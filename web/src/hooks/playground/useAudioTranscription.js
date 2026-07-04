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

import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import { uploadToR2 } from '../../helpers/upload';

// 语音识别（STT）走**异步任务**：amux_stt 这类模型单次耗时长（几十秒~几分钟），
// 同步阻塞会被网关/上游 504。所以和视频生成一样：提交 → 拿 task_id → 轮询。
//   提交: POST /pg/audio/transcriptions/async (multipart) → { id: task_xxxx }
//   轮询: GET  /pg/audio/transcriptions/{task_id} → { code, data: TaskDto }
//         TaskDto.status ∈ SUBMITTED/QUEUED/IN_PROGRESS/SUCCESS/FAILURE
//         TaskDto.data 是上游转写结果（结构厂商各异，尽力抽文本）
const SUBMIT_ENDPOINT = '/pg/audio/transcriptions/async';
const FETCH_ENDPOINT = (taskId) => `/pg/audio/transcriptions/${taskId}`;
const POLL_INTERVAL_MS = 3000;
// STT 一般比视频快，10 分钟兜底停轮询
const POLL_TIMEOUT_MS = 10 * 60 * 1000;

const STT_DONE = 'SUCCESS';
const STT_FAIL = 'FAILURE';

// amux_stt 上游要的是 { audio_base64, audio_filename }（或 audio_url）的 JSON，
// 不是 multipart 文件。把选中的音频文件读成 base64（去掉 data: 前缀）。
const fileToBase64 = (file) =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result || '');
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });

// 从任务结果里尽力抽出转写文本。上游结构厂商各异（可能是 {text} /
// {result:{text}} / {utterances:[{text}]} / {segments:[{text}]} 等），这里
// 多路 + 递归兜底；实在找不到就返回整段 JSON，至少让用户看到内容。
const joinArr = (arr) =>
  Array.isArray(arr)
    ? arr
        .map((x) =>
          x && typeof x === 'object' ? x.text || x.transcript || '' : String(x || ''),
        )
        .filter(Boolean)
        .join('')
    : '';

export const extractTranscript = (data) => {
  if (data == null) return '';
  let obj = data;
  if (typeof data === 'string') {
    const s = data.trim();
    if (!s) return '';
    try {
      obj = JSON.parse(s);
    } catch {
      return s; // 已经是纯文本
    }
  }
  if (typeof obj === 'string') return obj;
  if (typeof obj !== 'object') return String(obj);

  const direct =
    obj.text ?? obj.transcript ?? obj.transcription ?? obj.result_text;
  if (typeof direct === 'string' && direct.trim()) return direct;

  const result = obj.result ?? obj.data ?? obj.output;
  if (result && typeof result === 'object') {
    const nested = result.text ?? result.transcript ?? result.transcription;
    if (typeof nested === 'string' && nested.trim()) return nested;
  }

  const fromArr =
    joinArr(obj.utterances) ||
    joinArr(obj.segments) ||
    joinArr(result?.utterances) ||
    joinArr(result?.segments);
  if (fromArr.trim()) return fromArr;

  // 深度递归：找第一个 key 含 text/transcript 的非空字符串
  let found = '';
  const walk = (v, depth) => {
    if (found || depth > 4 || v == null) return;
    if (Array.isArray(v)) {
      v.forEach((x) => walk(x, depth + 1));
      return;
    }
    if (typeof v === 'object') {
      for (const [k, val] of Object.entries(v)) {
        if (found) break;
        if (typeof val === 'string' && /text|transcript/i.test(k) && val.trim()) {
          found = val;
          return;
        }
        walk(val, depth + 1);
      }
    }
  };
  walk(obj, 0);
  if (found.trim()) return found;

  try {
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
};

export const useAudioTranscription = ({ onDebug } = {}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const pollsRef = useRef(new Map());

  useEffect(() => {
    return () => {
      pollsRef.current.forEach(({ intervalId }) => clearInterval(intervalId));
      pollsRef.current.clear();
    };
  }, []);

  const stopPoll = useCallback((taskId) => {
    const poll = pollsRef.current.get(taskId);
    if (!poll) return;
    clearInterval(poll.intervalId);
    pollsRef.current.delete(taskId);
  }, []);

  const tick = useCallback(
    async (taskId, onUpdate) => {
      try {
        const res = await API.get(FETCH_ENDPOINT(taskId));
        const body = res?.data;
        // 后端返回 { code:"success", data: TaskDto }
        const task = body?.data || body;
        const status = task?.status;
        if (status === STT_DONE) {
          stopPoll(taskId);
          const text = extractTranscript(task?.data);
          onUpdate?.({ status: 'complete', text, raw: body });
        } else if (status === STT_FAIL) {
          stopPoll(taskId);
          onUpdate?.({
            status: 'error',
            errorMessage: task?.fail_reason || t('语音识别失败'),
            raw: body,
          });
        } else {
          onUpdate?.({ status: 'polling' });
        }
      } catch (err) {
        const code = err?.response?.status;
        if (code && code >= 400 && code < 500) {
          stopPoll(taskId);
          onUpdate?.({
            status: 'error',
            errorMessage:
              err?.response?.data?.message || err?.message || t('查询失败'),
          });
        }
      }
      const poll = pollsRef.current.get(taskId);
      if (poll && Date.now() - poll.startedAt > POLL_TIMEOUT_MS) {
        stopPoll(taskId);
        onUpdate?.({
          status: 'error',
          errorMessage: t('轮询超时，请稍后手动刷新'),
        });
      }
    },
    [stopPoll, t],
  );

  const startPolling = useCallback(
    (taskId, onUpdate) => {
      if (!taskId || pollsRef.current.has(taskId)) return;
      const intervalId = setInterval(
        () => tick(taskId, onUpdate),
        POLL_INTERVAL_MS,
      );
      pollsRef.current.set(taskId, {
        intervalId,
        startedAt: Date.now(),
        onUpdate,
      });
      tick(taskId, onUpdate); // 立刻跑一次
    },
    [tick],
  );

  const transcribe = useCallback(
    async ({ model, group, file, params = {}, onUpdate, onAudioUploaded } = {}) => {
      if (!model) {
        showError(t('请先选择模型'));
        return null;
      }
      if (!(file instanceof File) && !(file instanceof Blob)) {
        showError(t('请先选择要转写的音频文件'));
        return null;
      }

      const filename =
        file instanceof File && file.name ? file.name : 'audio.mp3';

      // 优先把音频传到 R2 换 URL：请求体只带一个 audio_url（很轻），上游自己
      // 去拉音频。避免把整段 base64 塞进 JSON —— 大文件会让 body 巨大、后端
      // 多次解析 + 上传上游都很慢，容易在到达上游前就把网关拖到 504。
      // R2 上传失败再回退到 audio_base64 内联。
      // 有些来源的 File.type 为空（或非 audio/*），会被 presign 的 audio/*
      // scope 拒掉 → 上传失败 → 没有永久链接。按扩展名补一个 audio 类型再传。
      let uploadFile = file;
      if (!file.type || !file.type.startsWith('audio/')) {
        const ext = (filename.split('.').pop() || '').toLowerCase();
        const typeByExt = {
          mp3: 'audio/mpeg',
          wav: 'audio/wav',
          m4a: 'audio/mp4',
          mp4: 'audio/mp4',
          ogg: 'audio/ogg',
          oga: 'audio/ogg',
          opus: 'audio/opus',
          flac: 'audio/flac',
          aac: 'audio/aac',
          webm: 'audio/webm',
          amr: 'audio/amr',
        }[ext] || 'audio/mpeg';
        try {
          uploadFile = new File([file], filename, { type: typeByExt });
        } catch {
          uploadFile = file;
        }
      }

      let audioUrl = '';
      let audioBase64 = '';
      try {
        const up = await uploadToR2(uploadFile, 'user-upload-audio');
        audioUrl = up.url;
        // 上传成功即回调：调用方可立刻把用户气泡换成 R2 永久链接的播放器，
        // 不用等转写提交(可能 504)。
        onAudioUploaded?.(audioUrl);
      } catch (e) {
        // R2 传失败(如 content-type 被拒)回退到 base64 内联
        try {
          audioBase64 = await fileToBase64(file);
        } catch (e2) {
          const msg = t('读取音频文件失败');
          showError(msg);
          return { error: msg };
        }
      }

      // 上游要 JSON：{ audio_url | audio_base64, audio_filename }。其余可调
      // 参数（language/prompt/... 由右栏 schema 面板提供）放 options。
      const payload = {
        model,
        group,
        audio_filename: filename,
      };
      if (audioUrl) payload.audio_url = audioUrl;
      else payload.audio_base64 = audioBase64;
      const options = {};
      ['language', 'prompt', 'temperature', 'response_format'].forEach((k) => {
        if (params[k] !== undefined && params[k] !== null && params[k] !== '') {
          options[k] = params[k];
        }
      });
      if (Object.keys(options).length > 0) payload.options = options;

      const requestTs = new Date().toISOString();
      const previewPayload = { ...payload };
      if (previewPayload.audio_base64) {
        previewPayload.audio_base64 = `<${filename}, base64 省略>`;
      }
      onDebug?.({
        previewRequest: JSON.stringify(previewPayload, null, 2),
        previewTimestamp: requestTs,
        request: `POST ${SUBMIT_ENDPOINT} (json, ${audioUrl ? 'audio_url' : 'audio_base64'})`,
        timestamp: requestTs,
      });

      setLoading(true);
      try {
        const res = await API.post(SUBMIT_ENDPOINT, payload, {
          skipErrorHandler: true,
        });
        const body = res?.data;
        onDebug?.({
          response: JSON.stringify(body, null, 2),
          timestamp: new Date().toISOString(),
        });
        // 注意：错误返回里也带上 audioUrl —— 音频已经在 R2 了，即便转写提交
        // 失败，用户气泡也该能挂播放器回放。
        if (res?.status >= 400 || body?.error || (body?.message && !body?.id)) {
          const msg =
            body?.error?.message ||
            body?.message ||
            t('语音识别任务提交失败');
          showError(msg);
          return { error: msg, audioUrl, raw: body };
        }
        const taskId =
          body?.id || body?.task_id || body?.data?.task_id || body?.data?.id;
        if (!taskId) {
          const msg = t('服务端未返回 task_id');
          showError(msg);
          return { error: msg, audioUrl, raw: body };
        }
        startPolling(taskId, onUpdate);
        // audioUrl 回传给调用方：STT 输入音频已经在 R2，用户气泡可直接挂播放器
        return { taskId, audioUrl, status: body?.status, raw: body };
      } catch (err) {
        const msg =
          err?.response?.data?.message ||
          err?.response?.data?.error?.message ||
          err?.message ||
          t('网络错误');
        showError(msg);
        // 504/网络错误时音频通常已上传 R2，一并回传让气泡挂播放器
        return { error: msg, audioUrl };
      } finally {
        setLoading(false);
      }
    },
    [onDebug, startPolling, t],
  );

  return { transcribe, loading, startPolling, stopPoll };
};

export default useAudioTranscription;
