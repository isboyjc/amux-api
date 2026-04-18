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

import { useState, useCallback, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DEFAULT_CONFIG,
  DEBUG_TABS,
  MESSAGE_STATUS,
} from '../../constants/playground.constants';
import {
  loadConfig,
  saveConfig,
  loadMessages,
  saveMessages,
} from '../../components/playground/configStorage';
import { processIncompleteThinkTags } from '../../helpers';
import { useSessions } from './useSessions';
import {
  WORKSPACE,
  inferWorkspaceFromModality,
  isModalityInWorkspace,
} from '../../constants/workspaceTypes';

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
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [status, setStatus] = useState({});
  // modalityMap[modelName] = { modality, param_schema }
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

  // 切会话 / 模型列表加载完 / workspace 变化时，把 inputs 的 model+group
  // 调整到当前 session 合法的值：
  //   1) 如果会话记住的 model 在当前 workspace 允许的模态列表里，优先还原
  //   2) 否则挑下拉里第一个兼容的模型
  //   3) 都没有就置空
  //   group 直接用会话记住的（没记就保持现状）。
  // 这一个 effect 同时负责"恢复上次选择"和"workspace 过滤兜底"，避免两个
  // 同 tick 触发的 effect 互相竞争覆盖。
  useEffect(() => {
    if (!sessionsReady || !activeId) return;
    const sess = sessionsApi.sessions.find((s) => s.id === activeId);
    if (!sess) return;

    const ws = sess.workspace_type;
    const hasModelList = Array.isArray(models) && models.length > 0;
    const isCompatible = (name) => {
      if (!name) return false;
      if (!ws) return true;
      const mod = modalityMap[name]?.modality || 'text';
      return isModalityInWorkspace(mod, ws);
    };

    // 如果 modelList 还没加载完，不做 workspace 过滤判断，先尽量还原 sess.model
    let targetModel;
    if (!hasModelList) {
      targetModel = sess.model || undefined;
    } else if (isCompatible(sess.model)) {
      targetModel = sess.model;
    } else {
      const first = models.find((m) => isCompatible(m.value));
      targetModel = first ? first.value : '';
    }

    setInputs((prev) => {
      const next = { ...prev };
      if (targetModel !== undefined && targetModel !== prev.model) {
        next.model = targetModel;
      }
      if (sess.group && sess.group !== prev.group) next.group = sess.group;
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeId, sessionsReady, models, modalityMap]);

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

  // inputs.model / inputs.group 变化时，把变化同步回 active session 的
  // 元数据，这样下次切回这个会话还能还原；同时让侧栏徽章跟随当前真实
  // modality 变化。
  // 注意：deps 里故意不放 activeId —— 切会话本身不算"用户改了模型"，
  // 如果把 activeId 放进来，切会话那一 tick inputs.model 还没被恢复 effect
  // 改过（React 还没重渲），此处就会用旧 session 的 inputs 去反向覆写新
  // session 的 model，进而"污染"新会话的上次选择。
  useEffect(() => {
    if (!sessionsReady || !activeId) return;
    if (!inputs.model) return;
    const sess = sessionsApi.sessions.find((s) => s.id === activeId);
    if (!sess) return;
    const resolvedModality = modalityMap[inputs.model]?.modality || 'text';
    // 只同步和该 workspace 兼容的模型。如果模型不属于当前 workspace（理论
    // 上下拉已过滤不会出现），不要污染 session 记录。
    if (
      sess.workspace_type &&
      !isModalityInWorkspace(resolvedModality, sess.workspace_type)
    ) {
      return;
    }
    if (
      sess.model !== inputs.model ||
      sess.group !== inputs.group ||
      sess.modality !== resolvedModality
    ) {
      sessionsApi.updateSessionMeta(activeId, {
        model: inputs.model,
        group: inputs.group,
        modality: resolvedModality,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inputs.model, inputs.group, modalityMap]);

  // 新建会话。workspaceType 由调用方（SessionList "+" 下拉）指定；
  // 不传则沿用当前会话的 workspace_type，或根据当前模型的 modality 推断，
  // 最终兜底 chat。
  const handleNewChat = useCallback(
    async (workspaceType) => {
      let ws = workspaceType;
      if (!ws) {
        const current = sessionsApi.sessions.find(
          (s) => s.id === sessionsApi.activeId,
        );
        ws =
          current?.workspace_type ||
          inferWorkspaceFromModality(modalityMap[inputs.model]?.modality) ||
          WORKSPACE.CHAT;
      }
      // 如果当前选中的模型不属于目标 workspace，就不把它塞进新会话
      const currentModelModality = modalityMap[inputs.model]?.modality || 'text';
      const carryModel =
        isModalityInWorkspace(currentModelModality, ws) ? inputs.model : '';
      await sessionsApi.createSession({
        title: '',
        workspaceType: ws,
        modality: carryModel ? currentModelModality : undefined,
        model: carryModel,
        group: inputs.group,
      });
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

  return {
    // 配置状态
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,

    // UI状态
    showSettings,
    models,
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
    setModels,
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
    currentWorkspaceType:
      sessionsApi.sessions.find((s) => s.id === activeId)?.workspace_type ||
      WORKSPACE.CHAT,
    sessionsReady,
    switchSession: sessionsApi.switchSession,
    createSession: sessionsApi.createSession,
    renameSession: sessionsApi.renameSession,
    updateSessionMeta: sessionsApi.updateSessionMeta,
    deleteSession: sessionsApi.deleteSession,
    touchSession: sessionsApi.touchSession,
  };
};
