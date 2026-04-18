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
        return {
          url: `data:image/png;base64,${it.b64_json}`,
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
    async ({ model, group, prompt, params = {}, extra }) => {
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
      ]);
      const topParams = {};
      const extraBody = {};
      for (const [k, v] of Object.entries(params || {})) {
        if (v === undefined || v === null || v === '') continue;
        if (OPENAI_STD_KEYS.has(k)) topParams[k] = v;
        else extraBody[k] = v;
      }
      const payload = {
        model,
        group,
        prompt: prompt.trim(),
        response_format: 'b64_json',
        ...topParams,
      };
      if (Object.keys(extraBody).length > 0) payload.extra_body = extraBody;
      if (extra && typeof extra === 'object') Object.assign(payload, extra);

      const requestTs = new Date().toISOString();
      onDebug?.({
        previewRequest: JSON.stringify(payload, null, 2),
        previewTimestamp: requestTs,
        request: JSON.stringify(payload, null, 2),
        timestamp: requestTs,
      });

      setLoading(true);
      try {
        const res = await API.post(IMAGE_ENDPOINT, payload);
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
