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

const IMAGE_ENDPOINT = '/pg/images/generations';
const IMAGE_EDITS_ENDPOINT = '/pg/images/edits';

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
    async ({ model, group, prompt, params = {}, inputs = {}, extra }) => {
      if (!prompt || !prompt.trim()) {
        showError(t('请输入 Prompt'));
        return null;
      }
      if (!model) {
        showError(t('请先选择模型'));
        return null;
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
      for (const [k, v] of Object.entries(params || {})) {
        if (v === undefined || v === null || v === '') continue;
        if (OPENAI_STD_KEYS.has(k)) topParams[k] = v;
        else extraBody[k] = v;
      }

      const hasImages = hasAnyFile(inputs);
      const requestTs = new Date().toISOString();
      setLoading(true);
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
          // 路径 B：带参考图，走 /pg/images/edits multipart。
          // 约定：schema 声明了图像输入槽 → 这里的 inputs[key] 就是 File / File[]。
          //   - 单槽：直接以 key 为表单字段名追加
          //   - 多槽：单文件仍用 key，多文件用 key[]（贴合 OpenAI image[] 惯例）
          // 非图像标量参数和 generations 一条同样走 topParams + extra_body。
          const fd = new FormData();
          fd.append('model', model);
          if (group) fd.append('group', group);
          fd.append('prompt', prompt.trim());
          fd.append('response_format', 'b64_json');
          Object.entries(topParams).forEach(([k, v]) => {
            fd.append(k, typeof v === 'string' ? v : String(v));
          });
          if (Object.keys(extraBody).length > 0) {
            fd.append('extra_body', JSON.stringify(extraBody));
          }
          // 调试预览：把文件部分用占位符代替，避免把 base64 塞进面板
          const filesPreview = {};
          Object.entries(inputs).forEach(([key, val]) => {
            if (val instanceof File) {
              fd.append(key, val, val.name);
              filesPreview[key] = `<File: ${val.name} (${val.size} bytes)>`;
            } else if (Array.isArray(val)) {
              const usable = val.filter((x) => x instanceof File);
              if (usable.length === 0) return;
              const field = usable.length > 1 ? `${key}[]` : key;
              usable.forEach((f) => fd.append(field, f, f.name));
              filesPreview[key] = usable.map(
                (f) => `<File: ${f.name} (${f.size} bytes)>`,
              );
            }
          });
          if (extra && typeof extra === 'object') {
            Object.entries(extra).forEach(([k, v]) => {
              if (v === undefined || v === null) return;
              fd.append(k, typeof v === 'string' ? v : String(v));
            });
          }
          previewForDebug = JSON.stringify(
            {
              _endpoint: IMAGE_EDITS_ENDPOINT,
              _format: 'multipart/form-data',
              model,
              group,
              prompt: prompt.trim(),
              ...topParams,
              ...(Object.keys(extraBody).length > 0
                ? { extra_body: extraBody }
                : {}),
              ...filesPreview,
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
          // 不要设置 Content-Type：浏览器会自动带 boundary
          res = await API.post(IMAGE_EDITS_ENDPOINT, fd);
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
    },
    [onDebug, t],
  );

  return { generate, loading };
};
