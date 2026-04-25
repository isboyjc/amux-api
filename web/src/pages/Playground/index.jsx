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

import React, { useContext, useEffect, useCallback, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Layout, Toast } from '@douyinfe/semi-ui';

// Context
import { UserContext } from '../../context/User';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';

// hooks
import { usePlaygroundState } from '../../hooks/playground/usePlaygroundState';
import { useMessageActions } from '../../hooks/playground/useMessageActions';
import { useApiRequest } from '../../hooks/playground/useApiRequest';
import { useImageGeneration } from '../../hooks/playground/useImageGeneration';
import { useVideoGeneration } from '../../hooks/playground/useVideoGeneration';
import { useSyncMessageAndCustomBody } from '../../hooks/playground/useSyncMessageAndCustomBody';
import { useMessageEdit } from '../../hooks/playground/useMessageEdit';
import { useDataLoader } from '../../hooks/playground/useDataLoader';

// Constants and utils
import {
  MESSAGE_ROLES,
  ERROR_MESSAGES,
} from '../../constants/playground.constants';
import {
  getLogo,
  buildMessageContent,
  createMessage,
  createLoadingAssistantMessage,
  getTextContent,
  buildApiPayload,
  encodeToBase64,
} from '../../helpers';

// Components
import ChatArea from '../../components/playground/ChatArea';
import FloatingButtons from '../../components/playground/FloatingButtons';
import PlaygroundRightPanel, {
  RIGHT_PANEL_TABS,
} from '../../components/playground/PlaygroundRightPanel';
import SessionList from '../../components/playground/SessionList';
import {
  parseSchema,
  defaultsOf,
  splitSchema,
  filterSchemaByGroup,
  hasImageInputSlot,
} from '../../components/playground/SchemaParamsRenderer';
import { PlaygroundProvider } from '../../contexts/PlaygroundContext';
import { MODALITY } from '../../constants/playground.constants';
import { inferMessageModality } from '../../components/playground/messageModality';

// 生成用户头像（灰色背景 + 圆角矩形，与导航头像一致）
const generateAvatarDataUrl = (username) => {
  if (!username) {
    return 'https://lf3-static.bytednsdoc.com/obj/eden-cn/ptlz_zlp/ljhwZthlaukjlkulzlp/docs-icon.png';
  }
  const firstLetter = username[0].toUpperCase();
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
      <rect width="32" height="32" rx="6" ry="6" fill="#9ca3af" />
      <text x="50%" y="50%" dominant-baseline="central" text-anchor="middle" font-size="16" fill="#ffffff" font-family="Ubuntu, sans-serif">${firstLetter}</text>
    </svg>
  `;
  return `data:image/svg+xml;base64,${encodeToBase64(svg)}`;
};

// 生成默认 AI logo（与系统默认 SVG logo 一致，适配明暗色）
const generateDefaultLogoDataUrl = (isDark) => {
  const fillColor = isDark ? '#ffffff' : '#18181b';
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128" fill="none">
      <path d="M4 96 C4 96, 24 12, 64 12 C104 12, 124 96, 124 96 Q124 102, 118 102 C94 102, 92 64, 64 64 C36 64, 34 102, 10 102 Q4 102, 4 96 Z" fill="${fillColor}"/>
    </svg>
  `;
  return `data:image/svg+xml;base64,${encodeToBase64(svg)}`;
};

const Playground = () => {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const actualTheme = useActualTheme();
  const isMobile = useIsMobile();
  const styleState = { isMobile };
  const [searchParams] = useSearchParams();

  const state = usePlaygroundState();
  const {
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
    showSettings,
    modelEntries,
    currentModelEntry,
    modalityMap,
    message,
    debugData,
    activeDebugTab,
    previewPayload,
    sseSourceRef,
    chatRef,
    handleInputChange,
    handleModelGroupChange,
    handleParameterToggle,
    debouncedSaveConfig,
    saveMessagesImmediately,
    handleNewChat,
    setShowSettings,
    setModelEntries,
    setGroups,
    setModalityMap,
    setMessage,
    setDebugData,
    setActiveDebugTab,
    setPreviewPayload,
    setShowDebugPanel,
    setCustomRequestMode,
    setCustomRequestBody,
    // 会话
    sessions,
    activeSessionId,
    activeSession,
    switchSession,
    renameSession,
    deleteSession,
    touchSession,
  } = state;

  // 当前选中模型的 modality，驱动 ChatArea / UnifiedInputBar / 右栏的形态
  const currentModality =
    (modalityMap && modalityMap[inputs.model]?.modality) || 'text';

  // image workspace 的参数 schema：完全由管理员在后台为该模型配置的
  // param_schema 决定；没配就是 null（右栏显示"该模型未声明任何参数"）。
  // 想给某个模型默认列出"尺寸/质量/宽高比"等，去"模型管理"里填一份 schema。
  //
  // schema 里 format:"image" 的字段会被 splitSchema 拆到 inputsSchema，
  // 由 ImageWorkspace 的附件条渲染；其余旋钮留在 paramsSchema 给右栏。
  const imageSchemaSplit = React.useMemo(() => {
    const raw = modalityMap?.[inputs.model]?.param_schema;
    const parsed = parseSchema(raw);
    if (!parsed) {
      const empty = { type: 'object', properties: {} };
      return { paramsSchema: empty, inputsSchema: empty, rawSchema: null };
    }
    // 先按当前分组过滤掉它不支持的字段（schema 里通过
    // `x-disabled-group-prefixes` 声明），再做 image-input 拆分。
    // 这样不仅 UI 上隐藏，imageSchemaSig 变化也会触发 imageParamValues
    // 重置，避免之前在 premium 设的值（比如 n=4）被偷偷发到 special。
    const filtered = filterSchemaByGroup(parsed, inputs.group);
    const split = splitSchema(filtered);
    return { ...split, rawSchema: filtered };
  }, [inputs.model, inputs.group, modalityMap]);
  const imageParamSchema = imageSchemaSplit.paramsSchema;
  const imageInputsSchema = imageSchemaSplit.inputsSchema;

  // 「参考图槽位 key」：优先 'image' / 'reference_image' / 'images'，否则
  // schema 里第一个 format:image 的字段。返回 { key, isArray }。
  // 提到 useState/useCallback 之前，避免 handleGenerateImage 闭包引用时
  // 出现 const TDZ 报错。
  const imageSlotInfo = React.useMemo(() => {
    const props = imageInputsSchema?.properties || {};
    const pickKey = ['image', 'reference_image', 'images'].find(
      (k) => k in props,
    );
    const key =
      pickKey ||
      Object.keys(props).find((k) => {
        const def = props[k];
        return (
          def?.format === 'image' ||
          (def?.type === 'array' && def?.items?.format === 'image')
        );
      });
    if (!key) return null;
    const def = props[key];
    return { key, isArray: def?.type === 'array' };
  }, [imageInputsSchema]);

  // image workspace 的参数值。schema 变化（切模型/切 workspace）时重置。
  const [imageParamValues, setImageParamValues] = React.useState({});
  // image workspace 的图像输入槽值。{ [key]: File | File[] | null }
  const [imageInputsValues, setImageInputsValues] = React.useState({});
  const imageSchemaSig = React.useMemo(
    () => JSON.stringify(imageSchemaSplit.rawSchema || {}),
    [imageSchemaSplit.rawSchema],
  );
  React.useEffect(() => {
    setImageParamValues(
      imageSchemaSplit.rawSchema ? defaultsOf(imageParamSchema) : {},
    );
    // 切模型 / 切 schema 时附件一并清空，避免把上一个模型的图带到新模型上
    setImageInputsValues({});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [imageSchemaSig]);

  // 右侧面板 Tab 状态：params 或 debug
  const [rightPanelTab, setRightPanelTab] = React.useState(
    RIGHT_PANEL_TABS.PARAMS,
  );

  // API 请求相关
  const { sendRequest, onStopGenerator } = useApiRequest(
    setMessage,
    setDebugData,
    setActiveDebugTab,
    sseSourceRef,
    saveMessagesImmediately,
  );

  // 数据加载（一次性聚合所有可用分组下的模型，提供给合并后的模型选择器）
  useDataLoader(
    userState,
    inputs,
    handleInputChange,
    setModelEntries,
    setGroups,
    setModalityMap,
  );

  // 消息编辑
  const {
    editingMessageId,
    editValue,
    setEditValue,
    handleMessageEdit,
    handleEditSave,
    handleEditCancel,
  } = useMessageEdit(
    setMessage,
    inputs,
    parameterEnabled,
    sendRequest,
    saveMessagesImmediately,
    currentModality,
  );

  // 消息和自定义请求体同步
  const { syncMessageToCustomBody, syncCustomBodyToMessage } =
    useSyncMessageAndCustomBody(
      customRequestMode,
      customRequestBody,
      message,
      inputs,
      setCustomRequestBody,
      setMessage,
      debouncedSaveConfig,
    );

  // 角色信息
  const roleInfo = {
    user: {
      name: userState?.user?.username || 'User',
      avatar: generateAvatarDataUrl(userState?.user?.username),
    },
    assistant: {
      name: 'Assistant',
      avatar: getLogo() || generateDefaultLogoDataUrl(actualTheme === 'dark'),
    },
    system: {
      name: 'System',
      avatar: getLogo() || generateDefaultLogoDataUrl(actualTheme === 'dark'),
    },
  };

  // 图片生成（image workspace 专用）
  const { generate: generateImage, loading: imageGenerating } =
    useImageGeneration({
      onDebug: (patch) =>
        setDebugData((prev) => ({ ...prev, ...patch })),
    });

  // 视频生成（video workspace 专用）—— hook 内部维护"提交 → 轮询 → 状态
  // 上报"的闭环，Playground 只负责把每一轮 onUpdate(patch) 合并回对应
  // assistant 消息并触发保存。
  const {
    generate: generateVideo,
    loading: videoGenerating,
    startPolling: startVideoPolling,
  } = useVideoGeneration({
    onDebug: (patch) =>
      setDebugData((prev) => ({ ...prev, ...patch })),
  });

  // 本轮已自动命名过的 session id 集合。避免同一会话多次触发 rename。
  const autoNamedSessionsRef = useRef(new Set());

  // 首条 prompt 自动给会话命名的统一入口。
  //
  // 判断逻辑（放宽以确保三种 workspace 都能触发）：
  //   1) 当前 session 尚未在本轮 ref 里打标 → 允许尝试
  //   2) prompt 非空 → 允许尝试
  //   3) 不再依赖 activeSession.title 的精确字面量匹配（之前多个默认标题
  //      变体会漏网）。只要 (a) 标题是空/在默认集合内 或 (b) 这条会话当前
  //      没有任何 message（= 名副其实的"第一次发送"），就执行 rename。
  //
  // 用 ref 防重入，renameSession 失败再清除 ref 允许下一次重试。
  const maybeAutoNameSession = useCallback(
    (promptText) => {
      if (!promptText || !activeSessionId) return;
      if (autoNamedSessionsRef.current.has(activeSessionId)) return;
      const trimmed = String(promptText).trim();
      if (!trimmed) return;

      const defaultTitles = new Set([
        '',
        '未命名会话',
        '未命名會話',
        '未命名',
        '新会话',
        '新會話',
        '新对话',
        '新對話',
      ]);
      const currentTitle = String(activeSession?.title || '').trim();
      const titleLooksDefault = defaultTitles.has(currentTitle);
      // 在发消息前 message 还保持 pre-submit 状态；空即"此次是首发"。
      const isFirstMessage = Array.isArray(message) && message.length === 0;

      if (!titleLooksDefault && !isFirstMessage) {
        // 用户已手动命过名、且已有消息，视为定名，不再覆盖
        autoNamedSessionsRef.current.add(activeSessionId);
        return;
      }

      const singleLine = trimmed.replace(/\s+/g, ' ');
      const name =
        singleLine.length > 20 ? singleLine.slice(0, 20) + '…' : singleLine;
      // 先记标记再异步 rename，防止短时间内重复触发
      autoNamedSessionsRef.current.add(activeSessionId);
      try {
        const result = renameSession(activeSessionId, name);
        if (result && typeof result.catch === 'function') {
          result.catch((err) => {
            // rename 失败则撤销标记，下次还能再试
            autoNamedSessionsRef.current.delete(activeSessionId);
            // eslint-disable-next-line no-console
            console.warn('[playground] auto-rename failed', err);
          });
        }
      } catch (err) {
        autoNamedSessionsRef.current.delete(activeSessionId);
        // eslint-disable-next-line no-console
        console.warn('[playground] auto-rename threw', err);
      }
    },
    [activeSessionId, activeSession, message, renameSession],
  );

  const handleGenerateImage = useCallback(
    async ({ prompt, inputs: imageInputs }) => {
      if (!prompt || !prompt.trim()) return;
      maybeAutoNameSession(prompt);
      // 真实消息活动：把当前会话顶到列表顶部
      if (activeSessionId) touchSession?.(activeSessionId);
      const params = { ...imageParamValues };

      // 收集本次发送的「参考图」并读成 base64 data URL，嵌入用户消息的
      // content 数组（OpenAI multimodal 同款形态：[{text}, ...{image_url}]）。
      // 之后 MessageContent / RefImageGrid 直接渲染这些图，气泡里展示
      // 「参考图 + 文字 prompt」的完整一次发送。
      const slotKey = imageSlotInfo?.key;
      const slotVal = slotKey ? imageInputs?.[slotKey] : null;
      const refFiles = Array.isArray(slotVal)
        ? slotVal.filter((x) => x instanceof File)
        : slotVal instanceof File
          ? [slotVal]
          : [];
      const refUrls =
        refFiles.length > 0
          ? await Promise.all(
              refFiles.map(
                (f) =>
                  new Promise((resolve, reject) => {
                    const reader = new FileReader();
                    reader.onload = () => resolve(reader.result);
                    reader.onerror = () => reject(reader.error);
                    reader.readAsDataURL(f);
                  }),
              ),
            )
          : [];

      const userContent =
        refUrls.length > 0
          ? [
              { type: 'text', text: prompt },
              ...refUrls.map((url) => ({
                type: 'image_url',
                image_url: { url },
              })),
            ]
          : prompt;

      // 在统一对话窗口里：用户消息和助手消息都打 modality='image' 标，
      // 让 AssistantBubbleRouter 渲染图片气泡，并把这些消息排除出后续
      // chat 模型的上下文。
      const userMsg = {
        ...createMessage(MESSAGE_ROLES.USER, userContent),
        modality: MODALITY.IMAGE,
        meta: { model: inputs.model, group: inputs.group },
      };
      const loadingMsg = {
        ...createLoadingAssistantMessage(),
        modality: MODALITY.IMAGE,
        // 把当前参数（size / aspect_ratio / quality 等）带到 loading 消息上，
        // 让 ImageBubble 的骨架占位能按目标尺寸的同比例渲染。
        meta: { model: inputs.model, group: inputs.group, params },
      };
      let newMessages = [];
      setMessage((prev) => {
        newMessages = [...prev, userMsg, loadingMsg];
        return newMessages;
      });
      setTimeout(() => saveMessagesImmediately(newMessages), 0);

      const result = await generateImage({
        model: inputs.model,
        group: inputs.group,
        prompt,
        params,
        inputs: imageInputs,
      });

      setMessage((prev) => {
        const next = prev.map((m) => {
          if (m.id !== loadingMsg.id) return m;
          if (!result || result.error || !result.images?.length) {
            return {
              ...m,
              status: 'error',
              errorMessage: result?.error || t('图片生成失败'),
            };
          }
          return {
            ...m,
            status: 'complete',
            modality: MODALITY.IMAGE,
            content: result.images.map((img) => ({
              type: 'image_url',
              image_url: { url: img.url },
              revised_prompt: img.revisedPrompt,
            })),
            meta: {
              model: inputs.model,
              group: inputs.group,
              params: params || {},
              usage: result.usage,
            },
          };
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [
      generateImage,
      inputs.model,
      inputs.group,
      imageParamValues,
      imageSlotInfo,
      maybeAutoNameSession,
      setMessage,
      saveMessagesImmediately,
      activeSessionId,
      touchSession,
      t,
    ],
  );

  const handleDeleteGeneration = useCallback(
    (generation) => {
      setMessage((prev) => {
        const next = prev.filter((m) => {
          if (generation.promptMessage && m.id === generation.promptMessage.id) return false;
          if (generation.assistantMessage && m.id === generation.assistantMessage.id) return false;
          return true;
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [setMessage, saveMessagesImmediately],
  );

  // ========== 视频生成（异步任务） ==========

  // 把 hook 返回的 patch 合并到对应的 assistant 消息上。
  // status=complete 时同时拼出 content（[{type:'video_url'}, {type:'image_url' last-frame}?]）。
  const applyVideoUpdate = useCallback(
    (assistantId, patch) => {
      setMessage((prev) => {
        const next = prev.map((m) => {
          if (m.id !== assistantId) return m;
          const merged = { ...m };
          if (patch.status) merged.status = patch.status;
          if (typeof patch.progress === 'number') merged.progress = patch.progress;
          if (patch.errorMessage) merged.errorMessage = patch.errorMessage;
          if (patch.status === 'complete') {
            if (patch.videoUrl) {
              const content = [
                {
                  type: 'video_url',
                  video_url: { url: patch.videoUrl },
                },
              ];
              // 末帧 URL 在 raw.metadata.last_frame_url 或 raw.content.last_frame_url
              // 上游可能放在不同字段里，这里两处都兜一下
              const lastFrame =
                patch.raw?.metadata?.last_frame_url ||
                patch.raw?.content?.last_frame_url;
              if (lastFrame) {
                content.push({
                  type: 'image_url',
                  image_url: { url: lastFrame },
                });
              }
              merged.content = content;
            } else {
              // 上游认为任务已完成，但响应里找不到 URL。把它视作 error 让
              // 用户能看到反馈（否则 UI 上什么都不显示，像"卡死"）。
              merged.status = 'error';
              merged.errorMessage = t('任务已完成但响应中未携带视频链接，请查看任务日志');
            }
          }
          return merged;
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [setMessage, saveMessagesImmediately],
  );

  const handleGenerateVideo = useCallback(
    async ({ prompt, attachments }) => {
      if (!prompt || !prompt.trim()) return;
      maybeAutoNameSession(prompt);
      if (activeSessionId) touchSession?.(activeSessionId);
      const params = { ...imageParamValues };

      // attachments 是 [{type:'image_url', image_url:{url}, role}, ...]，
      // 直接当作 metadata.content 数组发给后端。
      const content = Array.isArray(attachments) ? attachments : [];

      // user 消息也把 attachments 一并持久化，方便渲染成 badge。
      const userMsg = {
        ...createMessage(MESSAGE_ROLES.USER, prompt),
        attachments: content,
        modality: MODALITY.VIDEO,
        meta: { model: inputs.model, group: inputs.group },
      };
      const loadingMsg = {
        ...createLoadingAssistantMessage(),
        status: 'loading',
        progress: 0,
        modality: MODALITY.VIDEO,
        // 同 image：把 resolution / ratio / duration 带过来，骨架按比例渲染
        meta: { model: inputs.model, group: inputs.group, params },
      };
      let newMessages = [];
      setMessage((prev) => {
        newMessages = [...prev, userMsg, loadingMsg];
        return newMessages;
      });
      setTimeout(() => saveMessagesImmediately(newMessages), 0);

      const result = await generateVideo({
        model: inputs.model,
        group: inputs.group,
        prompt,
        params,
        content,
        onUpdate: (patch) => applyVideoUpdate(loadingMsg.id, patch),
      });

      if (!result || result.error) {
        setMessage((prev) => {
          const next = prev.map((m) => {
            if (m.id !== loadingMsg.id) return m;
            return {
              ...m,
              status: 'error',
              errorMessage: result?.error || t('视频生成任务提交失败'),
            };
          });
          setTimeout(() => saveMessagesImmediately(next), 0);
          return next;
        });
        return;
      }

      // 提交成功：把 taskId + meta 写回 assistant 消息，后续轮询靠
      // onUpdate 更新 status/content。
      setMessage((prev) => {
        const next = prev.map((m) => {
          if (m.id !== loadingMsg.id) return m;
          return {
            ...m,
            taskId: result.taskId,
            status: 'polling',
            modality: MODALITY.VIDEO,
            pollStartedAt: Date.now(),
            meta: {
              model: inputs.model,
              group: inputs.group,
              params: params || {},
            },
          };
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [
      generateVideo,
      inputs.model,
      inputs.group,
      imageParamValues,
      maybeAutoNameSession,
      setMessage,
      saveMessagesImmediately,
      activeSessionId,
      touchSession,
      applyVideoUpdate,
      t,
    ],
  );

  // 扫一遍 message 里还在 polling 的视频任务，把没跑轮询的补上。
  //   - 切会话 → message 由 IDB 异步加载进来，这时触发一次恢复
  //   - 刷新页面 → 同上
  //   - 新提交任务 → message 已经包含 loading/polling 行，顺带触发也无害
  // startVideoPolling 内部按 taskId 去重，重复调用直接 no-op；所以这里
  // 依赖 message 整体即可——虽然轮询过程中 message 会频繁变更导致 effect
  // 多次执行，但都是 idempotent 的空跑。
  React.useEffect(() => {
    if (!Array.isArray(message) || message.length === 0) return;
    message.forEach((m) => {
      if (!m || m.role !== MESSAGE_ROLES.ASSISTANT) return;
      if (!m.taskId) return;
      if (m.status === 'complete' || m.status === 'error') return;
      startVideoPolling(m.taskId, (patch) => applyVideoUpdate(m.id, patch));
    });
  }, [message, startVideoPolling, applyVideoUpdate]);

  // 消息操作
  const messageActions = useMessageActions(
    message,
    setMessage,
    onMessageSend,
    saveMessagesImmediately,
  );

  // 构建预览请求体
  const constructPreviewPayload = useCallback(() => {
    try {
      // 如果是自定义请求体模式且有自定义内容，直接返回解析后的自定义请求体
      if (customRequestMode && customRequestBody && customRequestBody.trim()) {
        try {
          return JSON.parse(customRequestBody);
        } catch (parseError) {
          console.warn('自定义请求体JSON解析失败，回退到默认预览:', parseError);
        }
      }

      // 默认预览逻辑
      let messages = [...message];

      // 如果存在用户消息
      if (
        !(
          messages.length === 0 ||
          messages.every((msg) => msg.role !== MESSAGE_ROLES.USER)
        )
      ) {
        // 处理最后一个用户消息的图片
        for (let i = messages.length - 1; i >= 0; i--) {
          if (messages[i].role === MESSAGE_ROLES.USER) {
            if (inputs.imageEnabled && inputs.imageUrls) {
              const validImageUrls = inputs.imageUrls.filter(
                (url) => url.trim() !== '',
              );
              if (validImageUrls.length > 0) {
                const textContent = getTextContent(messages[i]) || '示例消息';
                const content = buildMessageContent(
                  textContent,
                  validImageUrls,
                  true,
                );
                messages[i] = { ...messages[i], content };
              }
            }
            break;
          }
        }
      }

      return buildApiPayload(
        messages,
        null,
        inputs,
        parameterEnabled,
        currentModality,
      );
    } catch (error) {
      console.error('构造预览请求体失败:', error);
      return null;
    }
  }, [
    inputs,
    parameterEnabled,
    message,
    customRequestMode,
    customRequestBody,
    currentModality,
  ]);

  // chat / multimodal 路径：拼 chat completions 请求
  // 把过往的 image / video 消息从上下文里剔除（它们是 side outputs，
  // 不参与对话语义）；保留的 image_url 视觉附件由 multimodal 模型消费。
  function handleChatSend(content) {
    const userMessage = {
      ...createMessage(MESSAGE_ROLES.USER, content),
      modality:
        currentModality === MODALITY.MULTIMODAL || inputs.imageEnabled
          ? MODALITY.MULTIMODAL
          : MODALITY.TEXT,
      meta: { model: inputs.model, group: inputs.group },
    };
    const loadingMessage = {
      ...createLoadingAssistantMessage(),
      modality:
        currentModality === MODALITY.MULTIMODAL ? MODALITY.MULTIMODAL : MODALITY.TEXT,
      meta: { model: inputs.model, group: inputs.group },
    };

    if (customRequestMode && customRequestBody) {
      try {
        const customPayload = JSON.parse(customRequestBody);
        setMessage((prevMessage) => {
          const newMessages = [...prevMessage, userMessage, loadingMessage];
          sendRequest(customPayload, customPayload.stream !== false);
          setTimeout(() => saveMessagesImmediately(newMessages), 0);
          return newMessages;
        });
        return;
      } catch (error) {
        console.error('自定义请求体JSON解析失败:', error);
        Toast.error(ERROR_MESSAGES.JSON_PARSE_ERROR);
        return;
      }
    }

    const validImageUrls = inputs.imageUrls.filter((url) => url.trim() !== '');
    const messageContent = buildMessageContent(
      content,
      validImageUrls,
      inputs.imageEnabled,
    );
    const userMessageWithImages = {
      ...createMessage(MESSAGE_ROLES.USER, messageContent),
      modality: inputs.imageEnabled ? MODALITY.MULTIMODAL : MODALITY.TEXT,
      meta: { model: inputs.model, group: inputs.group },
    };

    setMessage((prevMessage) => {
      const newMessages = [...prevMessage, userMessageWithImages];
      // 上下文裁剪：只把 text / multimodal 消息送给 chat 模型；图片/视频
      // 气泡是 side outputs，不参与对话上下文。
      const chatHistory = newMessages.filter((m) => {
        const mod = inferMessageModality(m);
        return mod === MODALITY.TEXT || mod === MODALITY.MULTIMODAL;
      });
      const payload = buildApiPayload(
        chatHistory,
        null,
        inputs,
        parameterEnabled,
        currentModality,
      );
      sendRequest(payload, inputs.stream);

      const messagesWithLoading = [...newMessages, loadingMessage];
      setTimeout(() => saveMessagesImmediately(messagesWithLoading), 0);
      return messagesWithLoading;
    });
  }

  // 统一发送入口：UnifiedInputBar 不知道当前选的是哪种模型，全部以
  // (text) 形式回调过来；这里按当前 modality 路由到对应处理。
  // 切到不同 modality 的模型不再 fork 新会话，直接「下一条用新模型」。
  //
  // 写成 function 声明（不是 useCallback）的原因：useMessageActions 在更
  // 前面就要拿到 onMessageSend（用于消息「重新生成」），function 声明会
  // 被提升到 Playground 函数顶部，避免 const TDZ 报错。
  function onMessageSend(content) {
    if (typeof content !== 'string' || !content.trim()) return;
    maybeAutoNameSession(content);
    if (activeSessionId) touchSession?.(activeSessionId);

    if (currentModality === MODALITY.IMAGE) {
      handleGenerateImage({ prompt: content, inputs: imageInputsValues });
      // 一次性的图像输入槽：发完清空，避免下次意外带上。
      // 注意：imageParamValues（size/quality 等）保留，只在切模型/切 schema
      // 时才重置——用户调好的参数不该因为发送一次就被吞掉。
      setImageInputsValues({});
      return;
    }
    if (currentModality === MODALITY.VIDEO) {
      // 视频附件目前未通过 UnifiedInputBar 透传，发空数组；下一迭代
      // 把附件管理整合进 UnifiedInputBar 后再补
      handleGenerateVideo({ prompt: content, attachments: [] });
      return;
    }
    handleChatSend(content);
    // 文本/多模态对话：发送后清空参考图（与图片模型 setImageInputsValues({})
    // 对称）。inputs 里的模型/分组/temperature 等不动，跨消息保留。
    if (
      inputs.imageEnabled ||
      (inputs.imageUrls && inputs.imageUrls.some((u) => u && u.trim()))
    ) {
      handleInputChange('imageUrls', ['']);
      handleInputChange('imageEnabled', false);
    }
  }

  // 切换推理展开状态
  const toggleReasoningExpansion = useCallback(
    (messageId) => {
      setMessage((prevMessages) =>
        prevMessages.map((msg) =>
          msg.id === messageId && msg.role === MESSAGE_ROLES.ASSISTANT
            ? { ...msg, isReasoningExpanded: !msg.isReasoningExpanded }
            : msg,
        ),
      );
    },
    [setMessage],
  );

  // 注：消息气泡 / action bar 的渲染逻辑已下沉到 ChatArea 内部，
  // 这里只透传需要的 handlers 与编辑状态。

  // Effects

  // 同步消息和自定义请求体
  useEffect(() => {
    syncMessageToCustomBody();
  }, [message, syncMessageToCustomBody]);

  useEffect(() => {
    syncCustomBodyToMessage();
  }, [customRequestBody, syncCustomBodyToMessage]);

  // 处理URL参数
  useEffect(() => {
    if (searchParams.get('expired')) {
      Toast.warning(t('登录过期，请重新登录！'));
    }
  }, [searchParams, t]);

  // Playground 组件无需再监听窗口变化，isMobile 由 useIsMobile Hook 自动更新

  // 构建预览payload
  useEffect(() => {
    const timer = setTimeout(() => {
      const preview = constructPreviewPayload();
      setPreviewPayload(preview);
      setDebugData((prev) => ({
        ...prev,
        previewRequest: preview ? JSON.stringify(preview, null, 2) : null,
        previewTimestamp: preview ? new Date().toISOString() : null,
      }));
    }, 300);

    return () => clearTimeout(timer);
  }, [
    message,
    inputs,
    parameterEnabled,
    customRequestMode,
    customRequestBody,
    constructPreviewPayload,
    setPreviewPayload,
    setDebugData,
  ]);

  // 自动保存配置
  useEffect(() => {
    debouncedSaveConfig();
  }, [
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
    debouncedSaveConfig,
  ]);

  // 清空对话的处理函数
  const handleClearMessages = useCallback(() => {
    setMessage([]);
    // 清空对话后保存，传入空数组
    setTimeout(() => saveMessagesImmediately([]), 0);
  }, [setMessage, saveMessagesImmediately]);

  // 处理粘贴图片
  const handlePasteImage = useCallback(
    (base64Data) => {
      if (!inputs.imageEnabled) {
        return;
      }
      // 添加图片到 imageUrls 数组
      const newUrls = [...(inputs.imageUrls || []), base64Data];
      handleInputChange('imageUrls', newUrls);
    },
    [inputs.imageEnabled, inputs.imageUrls, handleInputChange],
  );

  // Playground Context 值
  const playgroundContextValue = {
    onPasteImage: handlePasteImage,
    imageUrls: inputs.imageUrls || [],
    imageEnabled: inputs.imageEnabled || false,
  };

  // 参考图相关的几个 derived 标志：
  //   supportsImageAttach    —— 多模态对话模型（chat-multimodal），用 imageUrls
  //   supportsImageInputSlot —— image/video 模型且 schema 有 image 输入槽
  //   acceptsReferenceImage  —— UnifiedInputBar 是否处理 paste/drag/picker
  //                              事件（true 时会调 handleAddReferenceImage，
  //                              文本类模型也设 true 以便父层 toast 反馈）
  //   showUploadButton       —— 输入框右侧是否渲染「+」基座按钮
  //                              （仅 image/video 且有 image input slot）
  const supportsImageAttach = currentModality === MODALITY.MULTIMODAL;
  const supportsImageInputSlot =
    (currentModality === MODALITY.IMAGE ||
      currentModality === MODALITY.VIDEO) &&
    hasImageInputSlot(imageInputsSchema);
  const showUploadButton = supportsImageInputSlot;
  // 文本/audio/embedding 等也允许走 ingest，让 toast 在父层统一提示
  const acceptsReferenceImage = true;

  // 任一异步任务进行中 → 按钮 loading
  const isAnyGenerating =
    imageGenerating ||
    videoGenerating ||
    (Array.isArray(message) &&
      message.some(
        (m) => m.status === 'loading' || m.status === 'incomplete' || m.status === 'polling',
      ));

  // 用 useRef 缓存「File → object URL」，避免每次渲染都新建 URL；
  // 组件卸载时统一 revoke 防止内存泄漏。
  const objectUrlsRef = React.useRef(new Map());
  React.useEffect(() => {
    const cache = objectUrlsRef.current;
    return () => {
      cache.forEach((url) => URL.revokeObjectURL(url));
      cache.clear();
    };
  }, []);
  const fileToUrl = React.useCallback((file) => {
    const cache = objectUrlsRef.current;
    if (cache.has(file)) return cache.get(file);
    const url = URL.createObjectURL(file);
    cache.set(file, url);
    return url;
  }, []);

  // 参考图统一展示数据：根据 modality 从 imageUrls / imageInputsValues
  // 派生，结构 [{ key, dataUrl, name }]。key 用于增删定位。
  const referenceImages = React.useMemo(() => {
    if (currentModality === MODALITY.MULTIMODAL) {
      return (inputs.imageUrls || [])
        .map((url, i) => ({ key: `mm-${i}`, dataUrl: url, idx: i }))
        .filter((x) => x.dataUrl && x.dataUrl.trim() !== '');
    }
    if (currentModality === MODALITY.IMAGE && imageSlotInfo) {
      const v = imageInputsValues?.[imageSlotInfo.key];
      const arr = Array.isArray(v) ? v : v instanceof File ? [v] : [];
      return arr
        .filter((f) => f instanceof File)
        .map((f, i) => ({
          key: `img-${i}-${f.name}-${f.size}`,
          dataUrl: fileToUrl(f),
          name: f.name,
          idx: i,
        }));
    }
    return [];
  }, [
    currentModality,
    inputs.imageUrls,
    imageInputsValues,
    imageSlotInfo,
    fileToUrl,
  ]);

  // 添加参考图：根据 modality 路由 + 数量限制 + 文本/不支持模型 toast 拒绝
  //   - multimodal：走 imageUrls（base64 字符串），统一上限 9 张
  //   - image / video（带 image input slot）：File 对象写到对应 slot，
  //     上限按 schema 的 maxItems（数组型）或 1（单值型）
  //   - 其它（text / audio / embedding / rerank）：toast 提示不支持
  const MULTIMODAL_MAX = 9;
  const handleAddReferenceImage = React.useCallback(
    (file) => {
      if (!file) return;

      // text / 其它类型：明确告知不支持
      if (
        currentModality !== MODALITY.MULTIMODAL &&
        !(
          (currentModality === MODALITY.IMAGE ||
            currentModality === MODALITY.VIDEO) &&
          imageSlotInfo
        )
      ) {
        Toast.warning({
          content: t('当前模型不支持图片输入，请切换到多模态、图片或视频模型'),
          duration: 2.5,
        });
        return;
      }

      // multimodal：cap 9 张，走 chat 多模态 imageUrls 通道
      if (currentModality === MODALITY.MULTIMODAL) {
        const cur = (inputs.imageUrls || []).filter(
          (u) => u && u.trim() !== '',
        );
        if (cur.length >= MULTIMODAL_MAX) {
          Toast.warning({
            content: t('多模态对话最多上传 {{n}} 张图片', {
              n: MULTIMODAL_MAX,
            }),
            duration: 2,
          });
          return;
        }
        const reader = new FileReader();
        reader.onload = (ev) => {
          const dataUrl = ev.target?.result;
          if (typeof dataUrl !== 'string') return;
          handleInputChange('imageUrls', [...cur, dataUrl]);
          handleInputChange('imageEnabled', true);
        };
        reader.readAsDataURL(file);
        return;
      }

      // image / video gen：写 schema slot，上限按 schema 决定
      if (
        (currentModality === MODALITY.IMAGE ||
          currentModality === MODALITY.VIDEO) &&
        imageSlotInfo
      ) {
        const def = imageInputsSchema?.properties?.[imageSlotInfo.key];
        const slotMax = imageSlotInfo.isArray
          ? def?.maxItems || 4
          : 1;
        const cur = imageInputsValues?.[imageSlotInfo.key];
        const curArr = imageSlotInfo.isArray
          ? Array.isArray(cur)
            ? cur.filter((x) => x instanceof File)
            : []
          : cur instanceof File
            ? [cur]
            : [];
        if (curArr.length >= slotMax) {
          Toast.warning({
            content: t('该模型最多上传 {{n}} 张参考图', { n: slotMax }),
            duration: 2,
          });
          return;
        }
        if (imageSlotInfo.isArray) {
          setImageInputsValues({
            ...imageInputsValues,
            [imageSlotInfo.key]: [...curArr, file],
          });
        } else {
          setImageInputsValues({
            ...imageInputsValues,
            [imageSlotInfo.key]: file,
          });
        }
      }
    },
    [
      currentModality,
      inputs.imageUrls,
      imageInputsValues,
      imageInputsSchema,
      imageSlotInfo,
      handleInputChange,
      t,
    ],
  );

  // 删除参考图：按 key 找到 idx，从对应 state 里 splice 出去
  const handleRemoveReferenceImage = React.useCallback(
    (key) => {
      const target = referenceImages.find((x) => x.key === key);
      if (!target) return;
      if (currentModality === MODALITY.MULTIMODAL) {
        const cur = (inputs.imageUrls || []).filter((u) => u && u.trim() !== '');
        const next = cur.slice();
        next.splice(target.idx, 1);
        handleInputChange('imageUrls', next.length > 0 ? next : ['']);
        if (next.length === 0) handleInputChange('imageEnabled', false);
        return;
      }
      if (currentModality === MODALITY.IMAGE && imageSlotInfo) {
        const cur = imageInputsValues?.[imageSlotInfo.key];
        if (imageSlotInfo.isArray) {
          const arr = Array.isArray(cur)
            ? cur.filter((x) => x instanceof File)
            : [];
          const next = arr.slice();
          next.splice(target.idx, 1);
          setImageInputsValues({
            ...imageInputsValues,
            [imageSlotInfo.key]: next,
          });
        } else {
          const { [imageSlotInfo.key]: _, ...rest } = imageInputsValues || {};
          setImageInputsValues(rest);
        }
      }
    },
    [
      currentModality,
      referenceImages,
      inputs.imageUrls,
      imageInputsValues,
      imageSlotInfo,
      handleInputChange,
    ],
  );

  // 「以此图继续编辑」：把已生成图片作为参考图加入输入框堆叠。
  // 走和 paste/drag 同一条 handleAddReferenceImage 路径——所以 schema
  // 数量上限、modality 校验、Toast 反馈全都自然继承。可点多次累计。
  const handleContinueEdit = React.useCallback(
    async (img) => {
      if (!img?.url) return;
      if (typeof img.url !== 'string' || !img.url.startsWith('data:')) {
        Toast.warning({
          content: t('远程图片暂不支持作为参考图'),
          duration: 2,
        });
        return;
      }
      try {
        const blob = await fetch(img.url).then((r) => r.blob());
        const mime = blob.type || 'image/png';
        const ext = (mime.split('/')[1] || 'png').split('+')[0];
        const file = new File([blob], `continue-${Date.now()}.${ext}`, {
          type: mime,
        });
        handleAddReferenceImage(file);
      } catch (err) {
        Toast.error({
          content: err?.message || t('填入参考图失败'),
          duration: 2,
        });
      }
    },
    [handleAddReferenceImage, t],
  );

  return (
    <PlaygroundProvider value={playgroundContextValue}>
      <div className='h-full'>
        <Layout className='h-full bg-transparent flex flex-col md:flex-row'>
          {/* 左侧：只剩会话列表（可在移动端通过 floating 按钮展开覆盖层）。
              模型选择 / 新会话 / 参数 / 附件全部下沉到 ChatArea 底部的输入栏。 */}
          {(showSettings || !isMobile) && (
            <Layout.Sider
              className={`
              bg-transparent border-r-0 flex-shrink-0 overflow-auto mt-[60px]
              ${
                isMobile
                  ? 'fixed top-0 left-0 right-0 bottom-0 z-[1000] w-full h-auto bg-white shadow-lg'
                  : 'relative z-[1] w-64 h-[calc(100vh-66px)]'
              }
            `}
              width={isMobile ? '100%' : 256}
            >
              {/* SessionList 自己处理内部滚动；外层只给 padding 和满高 */}
              <div className='h-full p-3 flex flex-col min-h-0'>
                <SessionList
                  sessions={sessions}
                  activeId={activeSessionId}
                  onSwitch={(id) => {
                    switchSession(id);
                    if (isMobile) setShowSettings(false);
                  }}
                  onCreate={() => {
                    handleNewChat();
                    if (isMobile) setShowSettings(false);
                  }}
                  onRename={renameSession}
                  onDelete={deleteSession}
                />
              </div>
            </Layout.Sider>
          )}

          <Layout.Content className='relative flex-1 overflow-hidden'>
            <div className='overflow-hidden flex flex-col lg:flex-row h-[calc(100vh-66px)] mt-[60px]'>
              <div className='flex-1 flex flex-col'>
                <ChatArea
                  chatRef={chatRef}
                  message={message}
                  inputs={inputs}
                  styleState={styleState}
                  showDebugPanel={showDebugPanel}
                  roleInfo={roleInfo}
                  onMessageSend={onMessageSend}
                  onMessageCopy={messageActions.handleMessageCopy}
                  onMessageReset={messageActions.handleMessageReset}
                  onMessageDelete={messageActions.handleMessageDelete}
                  onStopGenerator={onStopGenerator}
                  onClearMessages={handleClearMessages}
                  onToggleDebugPanel={() => setShowDebugPanel(!showDebugPanel)}
                  onMessageEdit={handleMessageEdit}
                  editingMessageId={editingMessageId}
                  editValue={editValue}
                  onEditValueChange={setEditValue}
                  onEditSave={handleEditSave}
                  onEditCancel={handleEditCancel}
                  onToggleReasoningExpansion={toggleReasoningExpansion}
                  onImageContinueEdit={handleContinueEdit}
                  imageSupportsContinueEdit={supportsImageInputSlot}
                  // UnifiedInputBar 透传
                  modelEntries={modelEntries}
                  currentModelEntry={currentModelEntry}
                  currentModality={currentModality}
                  onModelGroupChange={handleModelGroupChange}
                  paramSchema={imageParamSchema}
                  paramValues={imageParamValues}
                  onParamValuesChange={setImageParamValues}
                  loading={isAnyGenerating}
                  acceptsReferenceImage={acceptsReferenceImage}
                  showUploadButton={showUploadButton}
                  referenceImages={referenceImages}
                  onAddReferenceImage={handleAddReferenceImage}
                  onRemoveReferenceImage={handleRemoveReferenceImage}
                />
              </div>

              {/* 右侧面板 - 桌面端：参数/调试 Tab 切换，可通过输入栏的 ⚙ 收起 */}
              {showDebugPanel && !isMobile && (
                <div className='w-96 flex-shrink-0 h-full'>
                  <PlaygroundRightPanel
                    styleState={styleState}
                    activeTab={rightPanelTab}
                    onActiveTabChange={setRightPanelTab}
                    inputs={inputs}
                    parameterEnabled={parameterEnabled}
                    currentModality={currentModality}
                    customRequestMode={customRequestMode}
                    customRequestBody={customRequestBody}
                    onInputChange={handleInputChange}
                    onParameterToggle={handleParameterToggle}
                    onCustomRequestModeChange={setCustomRequestMode}
                    onCustomRequestBodyChange={setCustomRequestBody}
                    previewPayload={previewPayload}
                    paramSchema={imageParamSchema}
                    paramValues={imageParamValues}
                    onParamValuesChange={setImageParamValues}
                    debugData={debugData}
                    activeDebugTab={activeDebugTab}
                    onActiveDebugTabChange={setActiveDebugTab}
                  />
                </div>
              )}
            </div>

            {/* 右侧面板 - 移动端覆盖层 */}
            {showDebugPanel && isMobile && (
              <div className='fixed top-0 left-0 right-0 bottom-0 z-[1000] bg-white overflow-auto shadow-lg'>
                <PlaygroundRightPanel
                  styleState={styleState}
                  activeTab={rightPanelTab}
                  onActiveTabChange={setRightPanelTab}
                  onClose={() => setShowDebugPanel(false)}
                  inputs={inputs}
                  parameterEnabled={parameterEnabled}
                  currentModality={currentModality}
                  customRequestMode={customRequestMode}
                  customRequestBody={customRequestBody}
                  onInputChange={handleInputChange}
                  onParameterToggle={handleParameterToggle}
                  onCustomRequestModeChange={setCustomRequestMode}
                  onCustomRequestBodyChange={setCustomRequestBody}
                  previewPayload={previewPayload}
                  paramSchema={imageParamSchema}
                  paramValues={imageParamValues}
                  onParamValuesChange={setImageParamValues}
                  debugData={debugData}
                  activeDebugTab={activeDebugTab}
                  onActiveDebugTabChange={setActiveDebugTab}
                />
              </div>
            )}

            {/* 浮动按钮：移动端用于切换会话列表 / 调试面板 */}
            <FloatingButtons
              styleState={styleState}
              showSettings={showSettings}
              showDebugPanel={showDebugPanel}
              onToggleSettings={() => setShowSettings(!showSettings)}
              onToggleDebugPanel={() => setShowDebugPanel(!showDebugPanel)}
            />
          </Layout.Content>
        </Layout>
      </div>
    </PlaygroundProvider>
  );
};

export default Playground;
