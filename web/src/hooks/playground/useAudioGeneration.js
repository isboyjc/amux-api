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

// 语音合成（TTS）的最小 hook：POST /pg/audio/speech 拿到音频**二进制**，转存
// R2 换一个永久 URL（刷新不丢），返回 { url }。和图片/视频生成不同，speech
// 响应不是 JSON base64 而是 mp3 等二进制流，所以走 responseType:'blob'，不能
// 复用图片那套 JSON 解析。
const SPEECH_ENDPOINT = '/pg/audio/speech';
// 必须与后端 controller/upload.go allowedUploadScopes 里的同名 scope 一致。
const AUDIO_SCOPE = 'playground-audio-output';

// 出错时后端会把错误当 JSON 返回；responseType:'blob' 下拿到的是 Blob，
// 读成文本再解出 message。
const readBlobError = async (blob, fallback) => {
  try {
    const text = await blob.text();
    const j = JSON.parse(text);
    return j?.error?.message || j?.message || fallback;
  } catch {
    return fallback;
  }
};

export const useAudioGeneration = ({ onDebug } = {}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);

  const generate = useCallback(
    async ({ model, group, prompt, params = {} } = {}) => {
      if (!prompt || !prompt.trim()) {
        showError(t('请输入要合成的文本'));
        return null;
      }
      if (!model) {
        showError(t('请先选择模型'));
        return null;
      }

      // OpenAI /audio/speech 的字段：input(必填) / voice(必填) / response_format
      // / speed / instructions。voice 缺省给 alloy，保证没配 schema 也能直接测。
      const payload = {
        model,
        group,
        input: prompt.trim(),
        voice: params.voice || 'alloy',
      };
      ['response_format', 'speed', 'instructions'].forEach((k) => {
        if (params[k] !== undefined && params[k] !== null && params[k] !== '') {
          payload[k] = params[k];
        }
      });

      const requestTs = new Date().toISOString();
      const previewForDebug = JSON.stringify(payload, null, 2);
      onDebug?.({
        previewRequest: previewForDebug,
        previewTimestamp: requestTs,
        request: previewForDebug,
        timestamp: requestTs,
      });

      setLoading(true);
      try {
        const res = await API.post(SPEECH_ENDPOINT, payload, {
          responseType: 'blob',
          // 自己按 blob 语义解释错误，避免全局 showError 拿 blob 乱弹
          skipErrorHandler: true,
        });
        let blob = res?.data;

        // 兜底：后端把错误当 JSON 返回（HTTP 200 但 content-type 是 json）
        if (blob instanceof Blob && blob.type.includes('application/json')) {
          const msg = await readBlobError(blob, t('音频生成失败'));
          showError(msg);
          return { error: msg };
        }
        if (!(blob instanceof Blob) || blob.size === 0) {
          const msg = t('音频生成失败：响应为空');
          showError(msg);
          return { error: msg };
        }
        // R2 scope 校验 contentPrefixes:['audio/']，确保 blob 带 audio 类型
        if (!blob.type || !blob.type.startsWith('audio/')) {
          blob = new Blob([blob], { type: 'audio/mpeg' });
        }

        // 转存 R2 → 永久链接（刷新不丢）。上传失败回退临时 objectURL：能当场
        // 播、刷新丢失，但不影响已发生的计费（speech 是同步扣费，与上传解耦）。
        let url;
        let persisted = true;
        try {
          const up = await uploadToR2(blob, AUDIO_SCOPE);
          url = up.url;
        } catch (err) {
          persisted = false;
          url = URL.createObjectURL(blob);
          onDebug?.({
            response: `R2 上传失败，回退临时链接（刷新丢失）：${err?.message || err}`,
            timestamp: new Date().toISOString(),
          });
        }

        onDebug?.({
          response: JSON.stringify(
            { audio_url: url, bytes: blob.size, persisted },
            null,
            2,
          ),
          timestamp: new Date().toISOString(),
        });
        return { url, bytes: blob.size, persisted };
      } catch (err) {
        // 错误响应体在 responseType:'blob' 下也是 Blob
        let msg = t('音频生成失败');
        const errBlob = err?.response?.data;
        if (errBlob instanceof Blob) {
          msg = await readBlobError(errBlob, msg);
        } else {
          msg = err?.message || msg;
        }
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

export default useAudioGeneration;
