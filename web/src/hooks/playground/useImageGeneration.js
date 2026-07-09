/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import { uploadToR2 } from '../../helpers/upload';

const IMAGE_ENDPOINT = '/pg/images/generations';
const IMAGE_EDITS_ENDPOINT = '/pg/images/edits';

/**
 * 将分辨率档位和宽高比转换成 WIDTHxHEIGHT 格式
 * 例如: "3K" + "9:16" → "1728x3072"
 *
 * @param {string} resolution - 分辨率档位 ("2K", "3K", "4K")
 * @param {string} aspectRatio - 宽高比 ("16:9", "9:16" 等)
 * @returns {string|null} - WIDTHxHEIGHT 格式或 null
 */
function resolutionAndRatioToSize(resolution, aspectRatio) {
  // 解析分辨率档位对应的基准像素
  const resolutionMap = {
    '2K': 2048,
    '3K': 3072,
    '4K': 4096,
  };

  const basePixels = resolutionMap[resolution];
  if (!basePixels) {
    return null; // 不支持的分辨率档位
  }

  // 解析宽高比 "W:H"
  const parts = aspectRatio.split(':');
  if (parts.length !== 2) {
    return null;
  }

  const widthRatio = parseFloat(parts[0]);
  const heightRatio = parseFloat(parts[1]);

  if (isNaN(widthRatio) || isNaN(heightRatio) || widthRatio <= 0 || heightRatio <= 0) {
    return null;
  }

  // 计算实际宽高:让较长边接近 basePixels
  let width, height;
  if (widthRatio >= heightRatio) {
    // 横向或方形:宽度为基准
    width = basePixels;
    height = Math.round(basePixels * heightRatio / widthRatio);
  } else {
    // 纵向:高度为基准
    height = basePixels;
    width = Math.round(basePixels * widthRatio / heightRatio);
  }

  // 确保是偶数(视频编码友好)
  if (width % 2 !== 0) width++;
  if (height % 2 !== 0) height++;

  return `${width}x${height}`;
}


// 模块作用域的「进行中请求」注册表：messageId -> Promise<result>。
// 用途：图片生成是同步请求，promise 句柄只活在调用它的组件里；用户切到
// 别的路由时 Playground 会 unmount，promise 仍在跑但没人接收结果，loading
// 气泡就永远卡住。把 promise 提到模块作用域后，重新进入 Playground 的实例
// 可以按 messageId 回查并续上 .then(...)，从而恢复出"完成 / 失败"状态。
//
// 局限：promise 活在 JS 堆里，硬刷新 / 关 tab 仍然会丢——这种情况由
// Playground 的恢复 effect 把孤儿 loading 消息标成 error。
const inFlightImageRequests = new Map();

export const getInFlightImageRequest = (messageId) =>
  messageId ? inFlightImageRequests.get(messageId) || null : null;

export const clearInFlightImageRequest = (messageId) => {
  if (messageId) inFlightImageRequests.delete(messageId);
};

// hasAnyFile 判断 inputs 里是否有真正的 File。用于决定走 JSON generations
// 还是 multipart edits 两条路径。空对象、空数组、null 都视为无图。
const hasAnyFile = (inputs) => {
  if (!inputs || typeof inputs !== 'object') return false;
  for (const v of Object.values(inputs)) {
    if (v instanceof File) return true;
    if (Array.isArray(v) && v.some((x) => x instanceof File)) return true;
  }
  return false;
};

/**
 * 规范化 OpenAI 兼容的图片生成响应：
 *   {
 *     created: int,
 *     data: [ { b64_json?, url?, revised_prompt? } ],
 *     usage?: { total_tokens, ... }
 *   }
 * 返回统一的 { images: [{ url, revisedPrompt? }], usage, raw }。
 */
const normalizeImageResponse = (data) => {
  const items = Array.isArray(data?.data) ? data.data : [];
  const images = items
    .map((it) => {
      if (!it) return null;
      if (typeof it.b64_json === 'string' && it.b64_json.length > 0) {
        const url = it.b64_json.startsWith('data:')
          ? it.b64_json
          : `data:image/png;base64,${it.b64_json}`;
        return {
          url,
          revisedPrompt: it.revised_prompt,
        };
      }
      if (typeof it.url === 'string' && it.url.length > 0) {
        return { url: it.url, revisedPrompt: it.revised_prompt };
      }
      return null;
    })
    .filter(Boolean);
  return {
    images,
    usage: data?.usage || null,
    raw: data,
  };
};

/**
 * 封装图片生成请求：
 *  - POST /pg/images/generations
 *  - 走 playground 专用路径，复用现有 relay pipeline
 *  - 默认 response_format: b64_json（和对话路径一致，避免跨域外链问题）
 */
export const useImageGeneration = ({ onDebug } = {}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);

  const generate = useCallback(
    (args = {}) => {
      const {
        messageId,
        model,
        group,
        prompt,
        params = {},
        inputs = {},
        extra,
      } = args;
      if (!prompt || !prompt.trim()) {
        showError(t('请输入 Prompt'));
        return Promise.resolve(null);
      }
      if (!model) {
        showError(t('请先选择模型'));
        return Promise.resolve(null);
      }
      // 把 schema 产出的 params 分流：
      //   - OpenAI 标准字段（size/quality/n/...）放顶层，保留和对公 API
      //     一致的形状，已有 adapter 的逻辑（如 Imagen）可以直接消费；
      //   - 其它字段（aspect_ratio / image_size / seed / thinking_level /
      //     person_generation 等）收进 extra_body，作为"调用方 → adapter"
      //     的私有参数通道。adapter 只会读取它认识的 key，其余静默忽略，
      //     因此同一份 payload 对不同厂商/模型都安全。
      const OPENAI_STD_KEYS = new Set([
        'size',
        'quality',
        'n',
        'response_format',
        'style',
        'user',
        'background',
        'watermark',
        'output_format',
        'output_compression',
        'moderation',
        'partial_images',
      ]);
      const topParams = {};
      const extraBody = {};

      // Seedream 特殊处理: resolution + aspect_ratio → size
      // 用户可以同时选择档位和比例,前端计算最终 size 值
      let computedSize = null;
      const resolution = params?.resolution;
      const aspectRatio = params?.aspect_ratio;

      if (resolution && aspectRatio) {
        // 两者都有: 转换成 WIDTHxHEIGHT 格式 (如 "3K" + "9:16" → "1728x3072")
        // 这样既保持比例又保持分辨率档位
        computedSize = resolutionAndRatioToSize(resolution, aspectRatio);
      } else if (aspectRatio) {
        // 只有比例: 直接使用比例
        computedSize = aspectRatio;
      } else if (resolution) {
        // 只有档位: 直接使用档位
        computedSize = resolution;
      }

      for (const [k, v] of Object.entries(params || {})) {
        if (v === undefined || v === null || v === '') continue;
        // 跳过 resolution 和 aspect_ratio,已处理成 size
        if (k === 'resolution' || k === 'aspect_ratio') continue;
        if (OPENAI_STD_KEYS.has(k)) topParams[k] = v;
        else extraBody[k] = v;
      }

      // 如果计算出了 size,覆盖原有 size 参数
      if (computedSize) {
        topParams.size = computedSize;
      }

      const hasImages = hasAnyFile(inputs);
      const requestTs = new Date().toISOString();
      setLoading(true);
      // 真正的请求逻辑放进 IIFE，先拿到 promise 句柄再 set 进注册表，
      // 这样调用方在第一个 await 之前就能用 messageId 查到这条请求。
      const requestPromise = (async () => {
        try {
          let res;
          let previewForDebug;

          if (!hasImages) {
            // 路径 A：纯文生图。和改造前一字不差，JSON 打到 /pg/images/generations。
            const payload = {
              model,
              group,
              prompt: prompt.trim(),
              response_format: 'b64_json',
              ...topParams,
            };
            if (Object.keys(extraBody).length > 0) payload.extra_body = extraBody;
            if (extra && typeof extra === 'object') Object.assign(payload, extra);
            previewForDebug = JSON.stringify(payload, null, 2);
            onDebug?.({
              previewRequest: previewForDebug,
              previewTimestamp: requestTs,
              request: previewForDebug,
              timestamp: requestTs,
            });
            res = await API.post(IMAGE_ENDPOINT, payload);
          } else {
            // 路径 B：带参考图，先上传到 R2 获取公开 URL，再发送 JSON 请求。
            // 这是为了兼容 Poyo 等只接受 URL 的渠道（不支持 multipart 文件）。
            // 流程：File → uploadToR2(scope: "playground-image-reference") → URL → 请求体
            const imageUrls = [];
            const uploadErrors = [];

            // 收集所有需要上传的文件
            for (const [key, val] of Object.entries(inputs)) {
              if (val instanceof File) {
                try {
                  const result = await uploadToR2(val, 'playground-image-reference');
                  imageUrls.push(result.url);
                } catch (err) {
                  uploadErrors.push(`${val.name}: ${err?.message || '上传失败'}`);
                }
              } else if (Array.isArray(val)) {
                const usable = val.filter((x) => x instanceof File);
                for (const f of usable) {
                  try {
                    const result = await uploadToR2(f, 'playground-image-reference');
                    imageUrls.push(result.url);
                  } catch (err) {
                    uploadErrors.push(`${f.name}: ${err?.message || '上传失败'}`);
                  }
                }
              }
            }

            // 如果有上传失败，提示用户
            if (uploadErrors.length > 0) {
              const msg = `参考图上传失败：\n${uploadErrors.join('\n')}`;
              showError(msg);
              return { error: msg };
            }

            // 没有成功上传任何图片
            if (imageUrls.length === 0) {
              const msg = t('请上传参考图');
              showError(msg);
              return { error: msg };
            }

            // 构建 JSON 请求体，把图片 URL 放进 image 或 images 字段
            // 优先用 images（复数），OpenAI edits API 标准字段
            const payload = {
              model,
              group,
              prompt: prompt.trim(),
              response_format: 'b64_json',
              images: imageUrls, // 参考图 URL 数组
              ...topParams,
            };
            if (Object.keys(extraBody).length > 0) payload.extra_body = extraBody;
            if (extra && typeof extra === 'object') Object.assign(payload, extra);

            previewForDebug = JSON.stringify(
              {
                ...payload,
                images: imageUrls.map((url, i) => `<Uploaded: image_${i + 1}>`),
              },
              null,
              2,
            );
            onDebug?.({
              previewRequest: previewForDebug,
              previewTimestamp: requestTs,
              request: previewForDebug,
              timestamp: requestTs,
            });

            // 图生图也走 /pg/images/generations，adaptor 会根据有无 image/images 字段自动切换到 edit 模式
            res = await API.post(IMAGE_ENDPOINT, payload);
          }

          const body = res?.data;
          const responseTs = new Date().toISOString();
          onDebug?.({
            response: JSON.stringify(body, null, 2),
            timestamp: responseTs,
          });
          if (body?.error || res?.status >= 400) {
            const msg =
              body?.error?.message ||
              body?.message ||
              t('图片生成失败');
            showError(msg);
            return { error: msg, raw: body };
          }
          return normalizeImageResponse(body);
        } catch (err) {
          const msg = err?.response?.data?.error?.message || err?.message || t('网络错误');
          showError(msg);
          return { error: msg };
        } finally {
          setLoading(false);
        }
      })();

      // 注册到模块作用域 Map，方便组件 unmount/remount 后按 messageId 续上。
      // 不在这里清理：交给消费方（Playground 的恢复 effect）调用
      // clearInFlightImageRequest，避免清得太早导致后来的 attach 拿不到结果。
      if (messageId) {
        inFlightImageRequests.set(messageId, requestPromise);
      }
      return requestPromise;
    },
    [onDebug, t],
  );

  return { generate, loading };
};
