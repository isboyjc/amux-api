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

import { useState, useCallback, useRef, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DEFAULT_CONFIG,
  DEBUG_TABS,
  MESSAGE_STATUS,
  MODALITY,
} from '../../constants/playground.constants';
import {
  loadConfig,
  saveConfig,
  loadMessages,
  saveMessages,
} from '../../components/playground/configStorage';
import { processIncompleteThinkTags } from '../../helpers';
import { useSessions } from './useSessions';

export const usePlaygroundState = () => {
  const { t } = useTranslation();

  // 配置仍然走 localStorage（小而快）
  const [savedConfig] = useState(() => loadConfig());

  // 会话与消息走 IndexedDB
  const sessionsApi = useSessions();
  const { activeId, initialized: sessionsReady } = sessionsApi;

  // 基础配置状态
  const [inputs, setInputs] = useState(
    savedConfig.inputs || DEFAULT_CONFIG.inputs,
  );
  const [parameterEnabled, setParameterEnabled] = useState(
    savedConfig.parameterEnabled || DEFAULT_CONFIG.parameterEnabled,
  );
  const [showDebugPanel, setShowDebugPanel] = useState(
    savedConfig.showDebugPanel || DEFAULT_CONFIG.showDebugPanel,
  );
  const [customRequestMode, setCustomRequestMode] = useState(
    savedConfig.customRequestMode || DEFAULT_CONFIG.customRequestMode,
  );
  const [customRequestBody, setCustomRequestBody] = useState(
    savedConfig.customRequestBody || DEFAULT_CONFIG.customRequestBody,
  );

  // UI状态
  const [showSettings, setShowSettings] = useState(false);
  // modelEntries 是「跨分组聚合后的扁平模型列表」，每个元素一条
  // (model, group) 记录，UI 一个下拉同时让用户选模型 + 分组：
  //   { model, group, groupLabel, ratio, modality, paramSchema }
  const [modelEntries, setModelEntries] = useState([]);
  const [groups, setGroups] = useState([]);
  const [status, setStatus] = useState({});
  // modalityMap[modelName] = { modality, param_schema }
  // 同一模型在不同分组下 modality / schema 是一致的（管理员配置在模型表上、
  // 与分组无关），所以按名字索引足够。
  const [modalityMap, setModalityMap] = useState({});

  // 消息状态：初始空，待 active session 就绪后异步拉取
  const [message, setMessage] = useState([]);
  // 标记当前 session 的消息是否已完成首次从 IDB 加载，
  // 用来避免"初始 []"被当作真实内容写回 IDB 覆盖已有数据。
  const messagesLoadedRef = useRef(false);
  const [messagesReadyTick, setMessagesReadyTick] = useState(0);

  // active session 变化时，拉该会话的消息。
  // 空会话统一从空态进入（chat / image / 其他 workspace 都一样），由页面
  // 的空态 UI 引导用户，不再塞"你好 / 欢迎"这类占位气泡。
  useEffect(() => {
    let cancelled = false;
    if (!sessionsReady || !activeId) return;
    messagesLoadedRef.current = false;
    (async () => {
      const loaded = await loadMessages(activeId);
      if (cancelled) return;
      setMessage(Array.isArray(loaded) && loaded.length > 0 ? loaded : []);
      messagesLoadedRef.current = true;
      setMessagesReadyTick((x) => x + 1);
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionsReady, activeId, t]);

  // 切会话 / modelEntries 加载完成时，把 inputs.model/group 还原到这条
  // session 上次记的值。统一对话窗口里没有 modality 限制，会话只是一个
  // 包含混合气泡的对话流，只需保证「打开会话能继续上次的模型」即可。
  useEffect(() => {
    if (!sessionsReady || !activeId) return;
    const sess = sessionsApi.sessions.find((s) => s.id === activeId);
    if (!sess) return;
    const hasEntries = Array.isArray(modelEntries) && modelEntries.length > 0;

    setInputs((prev) => {
      const next = { ...prev };
      // 优先精确匹配 (model, group)；不存在时退到「同名其他分组」的第一条
      if (hasEntries && sess.model) {
        const exact = modelEntries.find(
          (e) => e.model === sess.model && e.group === sess.group,
        );
        if (exact) {
          if (exact.model !== prev.model) next.model = exact.model;
          if (exact.group !== prev.group) next.group = exact.group;
          return next;
        }
        const sameName = modelEntries.find((e) => e.model === sess.model);
        if (sameName) {
          if (sameName.model !== prev.model) next.model = sameName.model;
          if (sameName.group !== prev.group) next.group = sameName.group;
          return next;
        }
      }
      // entries 还没就绪：先把会话自身记的回填，等 entries 到了再校准
      if (sess.model && sess.model !== prev.model) next.model = sess.model;
      if (sess.group && sess.group !== prev.group) next.group = sess.group;
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeId, sessionsReady, modelEntries]);

  // 调试状态
  const [debugData, setDebugData] = useState({
    request: null,
    response: null,
    timestamp: null,
    previewRequest: null,
    previewTimestamp: null,
  });
  const [activeDebugTab, setActiveDebugTab] = useState(DEBUG_TABS.PREVIEW);
  const [previewPayload, setPreviewPayload] = useState(null);

  // 编辑状态
  const [editingMessageId, setEditingMessageId] = useState(null);
  const [editValue, setEditValue] = useState('');

  // Refs
  const sseSourceRef = useRef(null);
  const chatRef = useRef(null);
  const saveConfigTimeoutRef = useRef(null);

  // 配置更新函数
  const handleInputChange = useCallback((name, value) => {
    setInputs((prev) => ({ ...prev, [name]: value }));
  }, []);

  // 选「模型 + 分组」组合：直接更新 inputs，并把会话「上次使用的模型」
  // 元数据同步过去，方便切回会话时还原。
  // 统一对话窗口里不再有 fork 概念——切到任意 modality 都允许，下一条消息
  // 用新模型回复，前面的图片/视频气泡照旧保留在历史里。
  const handleModelGroupChange = useCallback(
    (model, group) => {
      const entry = modelEntries.find(
        (e) => e.model === model && e.group === group,
      );
      const modality = entry?.modality || modalityMap[model]?.modality || 'text';
      setInputs((prev) => ({ ...prev, model, group }));
      if (activeId) {
        sessionsApi.updateSessionMeta(activeId, { model, group, modality });
      }
    },
    [activeId, modelEntries, modalityMap, sessionsApi],
  );

  const handleParameterToggle = useCallback((paramName) => {
    setParameterEnabled((prev) => ({
      ...prev,
      [paramName]: !prev[paramName],
    }));
  }, []);

  // 消息保存函数 - 写入当前 active session 的 IndexedDB 存储
  const saveMessagesImmediately = useCallback(
    (messagesToSave) => {
      if (!activeId) return;
      // 未完成首次加载前不写，避免用空数组覆盖已有数据
      if (!messagesLoadedRef.current) return;
      saveMessages(messagesToSave || message, activeId).catch((err) =>
        console.error('saveMessages failed:', err),
      );
    },
    [message, activeId],
  );

  // 配置保存
  const debouncedSaveConfig = useCallback(() => {
    if (saveConfigTimeoutRef.current) {
      clearTimeout(saveConfigTimeoutRef.current);
    }

    saveConfigTimeoutRef.current = setTimeout(() => {
      const configToSave = {
        inputs,
        parameterEnabled,
        showDebugPanel,
        customRequestMode,
        customRequestBody,
      };
      saveConfig(configToSave);
    }, 1000);
  }, [
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
  ]);

  // 配置导入/重置
  const handleConfigImport = useCallback((importedConfig) => {
    if (importedConfig.inputs) {
      const parsedMaxTokens = parseInt(importedConfig.inputs.max_tokens, 10);
      setInputs((prev) => ({
        ...prev,
        ...importedConfig.inputs,
        max_tokens: Number.isNaN(parsedMaxTokens)
          ? importedConfig.inputs.max_tokens
          : parsedMaxTokens,
      }));
    }
    if (importedConfig.parameterEnabled) {
      setParameterEnabled((prev) => ({
        ...prev,
        ...importedConfig.parameterEnabled,
      }));
    }
    if (typeof importedConfig.showDebugPanel === 'boolean') {
      setShowDebugPanel(importedConfig.showDebugPanel);
    }
    if (importedConfig.customRequestMode) {
      setCustomRequestMode(importedConfig.customRequestMode);
    }
    if (importedConfig.customRequestBody) {
      setCustomRequestBody(importedConfig.customRequestBody);
    }
    // 如果导入的配置包含消息，也恢复消息
    if (importedConfig.messages && Array.isArray(importedConfig.messages)) {
      setMessage(importedConfig.messages);
    }
  }, []);

  const handleConfigReset = useCallback((options = {}) => {
    const { resetMessages = false } = options;

    setInputs(DEFAULT_CONFIG.inputs);
    setParameterEnabled(DEFAULT_CONFIG.parameterEnabled);
    setShowDebugPanel(DEFAULT_CONFIG.showDebugPanel);
    setCustomRequestMode(DEFAULT_CONFIG.customRequestMode);
    setCustomRequestBody(DEFAULT_CONFIG.customRequestBody);

    // 只有在明确指定时才重置消息
    if (resetMessages) {
      setMessage([]);
    }
  }, []);

  // 新建会话：直接用当前 inputs 的 (model, group) 作为初始值。会话只是
  // 一个混合气泡的对话流，不再绑定单一 workspace 类型，所以也不需要弹窗
  // 让用户挑「这是什么类型的会话」——该选什么模型在输入框里现挑就行。
  //
  // 可选 overrides：{ model, group } 显式指定时跳过当前 inputs，用于深链
  // 快速试用（/console/playground?model=...&group=...）这类场景。给了
  // overrides 还会把 inputs 也同步更新，让 UnifiedInputBar 显示正确选中。
  const handleNewChat = useCallback(
    async (overrides) => {
      const targetModel = overrides?.model ?? inputs.model;
      const targetGroup = overrides?.group ?? inputs.group;
      const modality = modalityMap[targetModel]?.modality || 'text';
      await sessionsApi.createSession({
        title: '',
        modality,
        model: targetModel || '',
        group: targetGroup || '',
      });
      if (overrides && (overrides.model || overrides.group)) {
        setInputs((prev) => ({
          ...prev,
          ...(overrides.model ? { model: overrides.model } : null),
          ...(overrides.group ? { group: overrides.group } : null),
        }));
      }
    },
    [sessionsApi, modalityMap, inputs.model, inputs.group],
  );

  // 清理定时器
  useEffect(() => {
    return () => {
      if (saveConfigTimeoutRef.current) {
        clearTimeout(saveConfigTimeoutRef.current);
      }
    };
  }, []);

  // 每个会话首次加载完后，如果最后一条消息仍处于 LOADING/INCOMPLETE 状态
  // （通常因为上次刷新时正好在流式接收中），就自动收尾一次。
  useEffect(() => {
    if (!messagesReadyTick) return;
    if (!Array.isArray(message) || message.length === 0) return;

    const lastMsg = message[message.length - 1];
    if (
      lastMsg.status !== MESSAGE_STATUS.LOADING &&
      lastMsg.status !== MESSAGE_STATUS.INCOMPLETE
    ) {
      return;
    }
    // image / video 走各自的恢复路径：
    //   - image: Playground 的 in-flight Map + 恢复 effect 接管
    //   - video: useVideoGeneration 的 task 轮询接管
    // 这里如果按 chat 流式中断的方式收尾，会把 content 强行写成空字符串，
    // 把图/视频气泡打成"已完成的空消息"。
    if (
      lastMsg.modality === MODALITY.IMAGE ||
      lastMsg.modality === MODALITY.VIDEO
    ) {
      return;
    }
    const processed = processIncompleteThinkTags(
      lastMsg.content || '',
      lastMsg.reasoningContent || '',
    );
    const fixedLastMsg = {
      ...lastMsg,
      status: MESSAGE_STATUS.COMPLETE,
      content: processed.content,
      reasoningContent: processed.reasoningContent || null,
      isThinkingComplete: true,
    };
    const updatedMessages = [...message.slice(0, -1), fixedLastMsg];
    setMessage(updatedMessages);
    setTimeout(() => saveMessagesImmediately(updatedMessages), 0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messagesReadyTick]);

  // 提供一个稳定的「按当前 inputs 找 entry」的派生值，避免在子组件里反复
  // find。entries 还没到的时候返回 null，子组件可以借此渲染骨架/placeholder。
  const currentModelEntry = useMemo(() => {
    if (!Array.isArray(modelEntries) || modelEntries.length === 0) return null;
    return (
      modelEntries.find(
        (e) => e.model === inputs.model && e.group === inputs.group,
      ) || null
    );
  }, [modelEntries, inputs.model, inputs.group]);

  return {
    // 配置状态
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,

    // UI状态
    showSettings,
    modelEntries,
    currentModelEntry,
    groups,
    status,
    modalityMap,

    // 消息状态
    message,

    // 调试状态
    debugData,
    activeDebugTab,
    previewPayload,

    // 编辑状态
    editingMessageId,
    editValue,

    // Refs
    sseSourceRef,
    chatRef,
    saveConfigTimeoutRef,

    // 更新函数
    setInputs,
    setParameterEnabled,
    setShowDebugPanel,
    setCustomRequestMode,
    setCustomRequestBody,
    setShowSettings,
    setModelEntries,
    setGroups,
    setStatus,
    setModalityMap,
    setMessage,
    setDebugData,
    setActiveDebugTab,
    setPreviewPayload,
    setEditingMessageId,
    setEditValue,

    // 处理函数
    handleInputChange,
    handleModelGroupChange,
    handleParameterToggle,
    debouncedSaveConfig,
    saveMessagesImmediately,
    handleConfigImport,
    handleConfigReset,
    handleNewChat,

    // 会话管理（来自 useSessions）
    sessions: sessionsApi.sessions,
    activeSessionId: activeId,
    activeSession:
      sessionsApi.sessions.find((s) => s.id === activeId) || null,
    sessionsReady,
    switchSession: sessionsApi.switchSession,
    createSession: sessionsApi.createSession,
    renameSession: sessionsApi.renameSession,
    updateSessionMeta: sessionsApi.updateSessionMeta,
    deleteSession: sessionsApi.deleteSession,
    touchSession: sessionsApi.touchSession,
  };
};
