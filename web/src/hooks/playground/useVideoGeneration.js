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

const SUBMIT_ENDPOINT = '/pg/video/generations';
const FETCH_ENDPOINT = (taskId) => `/pg/video/generations/${taskId}`;

// OpenAIVideo 的四种状态（后端 dto.VideoStatus*）
const VIDEO_STATUS = {
  QUEUED: 'queued',
  IN_PROGRESS: 'in_progress',
  COMPLETED: 'completed',
  FAILED: 'failed',
};

// 轮询节奏：任务一般 60~300s 完成，3s 一次是个相对经济的折中。
const POLL_INTERVAL_MS = 3000;
// 兜底最长轮询时长：官方默认执行超时是 172800s（48 小时），我们前端不
// 奢求把超长任务跑到结束——过 30 分钟就停轮询，让用户手动刷新或下次
// 进来重新拉取。
const POLL_TIMEOUT_MS = 30 * 60 * 1000;

/**
 * useVideoGeneration 负责"提交 → 轮询 → 上报状态"。
 * 本 hook 不直接管消息数组，它把每一轮变化通过 onUpdate(patch) 发给外部；
 * 消息数组的更新、持久化、UI 由 Playground 主编排处理。
 *
 * @param {object} cfg
 * @param {(patch) => void} cfg.onDebug  调试面板回调（同 useImageGeneration）
 */
export const useVideoGeneration = ({ onDebug } = {}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);

  // taskId -> { intervalId, startedAt, onUpdate }。组件卸载时统一清理。
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
        const status = body?.status;
        const progress =
          typeof body?.progress === 'number' ? body.progress : undefined;
        const errorMsg = body?.error?.message || '';
        // 视频 URL 兜底：优先 metadata.url（OpenAIVideo 标准位置）；若上游/
        // 适配器把 URL 放在别处（例如老路径的 result_url、或者 data.output.url
        // 透传没被 ConvertToOpenAIVideo 归一），再从其他可能位置拾取，避免
        // 前端看到 status=completed 但拿不到 URL 导致面板空白。
        const videoUrl =
          body?.metadata?.url ||
          body?.result_url ||
          body?.url ||
          body?.data?.output?.url ||
          body?.content?.video_url ||
          '';

        if (status === VIDEO_STATUS.COMPLETED) {
          stopPoll(taskId);
          if (!videoUrl) {
            // 帮助调试：响应里 status=completed 但找不到可用 URL
            console.warn(
              '[useVideoGeneration] task completed but no url found in response',
              body,
            );
          }
          onUpdate?.({
            status: 'complete',
            progress: 100,
            videoUrl,
            raw: body,
          });
        } else if (status === VIDEO_STATUS.FAILED) {
          stopPoll(taskId);
          onUpdate?.({
            status: 'error',
            errorMessage: errorMsg || t('视频生成失败'),
            raw: body,
          });
        } else {
          onUpdate?.({
            status: 'polling',
            progress: progress ?? undefined,
          });
        }
      } catch (err) {
        // 网络抖动时不立刻标记错误；但如果是 4xx（任务不存在/被删除），
        // 停轮询并上报错误。
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

      // 超时兜底
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

  /**
   * 启动一个任务的轮询。重复调用同一个 taskId 会被忽略。
   * 主要在"会话切换时恢复还没完成的任务轮询"等场景使用。
   */
  const startPolling = useCallback(
    (taskId, onUpdate) => {
      if (!taskId) return;
      if (pollsRef.current.has(taskId)) return;
      const intervalId = setInterval(() => tick(taskId, onUpdate), POLL_INTERVAL_MS);
      pollsRef.current.set(taskId, {
        intervalId,
        startedAt: Date.now(),
        onUpdate,
      });
      // 立刻跑一次，避免用户等够 3s 才看到状态变化
      tick(taskId, onUpdate);
    },
    [tick],
  );

  /**
   * 提交新任务。返回 { taskId, status, raw }，由外部把这些信息写回 assistant
   * 消息；随后 hook 自动启动轮询，每次变化通过 onUpdate(patch) 通知外部。
   *
   * params: schema 渲染出来的参数（resolution/ratio/duration/...）
   * content: 可选的内容数组（image_url / video_url / audio_url），支持
   *          first_frame / last_frame / reference_image / reference_video /
   *          reference_audio 等 role；由调用方构造。
   */
  const generate = useCallback(
    async ({ model, group, prompt, params = {}, content, onUpdate }) => {
      if (!model) {
        showError(t('请先选择模型'));
        return null;
      }
      if (!prompt || !prompt.trim()) {
        showError(t('请输入 Prompt'));
        return null;
      }

      // metadata 承载"上游私有参数 + 富内容数组"；Doubao adapter 会把
      // metadata 反序列化到 requestPayload，content 也会被合并进去。
      const metadata = { ...(params || {}) };
      if (Array.isArray(content) && content.length > 0) {
        metadata.content = content;
      }

      const payload = {
        model,
        group,
        prompt: prompt.trim(),
        metadata,
      };

      const requestTs = new Date().toISOString();
      onDebug?.({
        previewRequest: JSON.stringify(payload, null, 2),
        previewTimestamp: requestTs,
        request: JSON.stringify(payload, null, 2),
        timestamp: requestTs,
      });

      setLoading(true);
      try {
        const res = await API.post(SUBMIT_ENDPOINT, payload);
        const body = res?.data;
        const responseTs = new Date().toISOString();
        onDebug?.({ response: JSON.stringify(body, null, 2), timestamp: responseTs });

        if (res?.status >= 400 || body?.error) {
          const msg =
            body?.error?.message ||
            body?.message ||
            t('视频生成任务提交失败');
          showError(msg);
          return { error: msg, raw: body };
        }

        const taskId = body?.id || body?.task_id;
        if (!taskId) {
          const msg = t('服务端未返回 task_id');
          showError(msg);
          return { error: msg, raw: body };
        }

        startPolling(taskId, onUpdate);
        return { taskId, status: body?.status || VIDEO_STATUS.QUEUED, raw: body };
      } catch (err) {
        const msg =
          err?.response?.data?.message ||
          err?.response?.data?.error?.message ||
          err?.message ||
          t('网络错误');
        showError(msg);
        return { error: msg };
      } finally {
        setLoading(false);
      }
    },
    [onDebug, startPolling, t],
  );

  return { generate, loading, startPolling, stopPoll };
};
