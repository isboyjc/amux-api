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
import { Layout, Toast, Modal } from '@douyinfe/semi-ui';

// Context
import { UserContext } from '../../context/User';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';

// hooks
import { usePlaygroundState } from '../../hooks/playground/usePlaygroundState';
import { useMessageActions } from '../../hooks/playground/useMessageActions';
import { useApiRequest } from '../../hooks/playground/useApiRequest';
import { useImageGeneration } from '../../hooks/playground/useImageGeneration';
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
import {
  OptimizedSettingsPanel,
  OptimizedMessageContent,
  OptimizedMessageActions,
} from '../../components/playground/OptimizedComponents';
import ChatArea from '../../components/playground/ChatArea';
import FloatingButtons from '../../components/playground/FloatingButtons';
import PlaygroundRightPanel, {
  RIGHT_PANEL_TABS,
} from '../../components/playground/PlaygroundRightPanel';
import WorkspaceRouter from '../../components/playground/WorkspaceRouter';
import {
  parseSchema,
  defaultsOf,
} from '../../components/playground/SchemaParamsRenderer';
import { PlaygroundProvider } from '../../contexts/PlaygroundContext';
import { WORKSPACE } from '../../constants/workspaceTypes';

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
    models,
    groups,
    status,
    modalityMap,
    message,
    debugData,
    activeDebugTab,
    previewPayload,
    sseSourceRef,
    chatRef,
    handleInputChange,
    handleParameterToggle,
    debouncedSaveConfig,
    saveMessagesImmediately,
    handleConfigImport,
    handleConfigReset,
    handleNewChat,
    setShowSettings,
    setModels,
    setGroups,
    setStatus,
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
    currentWorkspaceType,
    switchSession,
    createSession,
    renameSession,
    deleteSession,
    touchSession,
  } = state;

  // 当前选中模型的 modality，用于 SettingsPanel 按模态自适应
  const currentModality =
    (modalityMap && modalityMap[inputs.model]?.modality) || 'text';

  // image workspace 的参数 schema：完全由管理员在后台为该模型配置的
  // param_schema 决定；没配就是 null（右栏显示"该模型未声明任何参数"）。
  // 想给某个模型默认列出"尺寸/质量/宽高比"等，去"模型管理"里填一份 schema。
  const imageParamSchema = React.useMemo(() => {
    const raw = modalityMap?.[inputs.model]?.param_schema;
    return parseSchema(raw);
  }, [inputs.model, modalityMap]);

  // image workspace 的参数值。schema 变化（切模型/切 workspace）时重置。
  const [imageParamValues, setImageParamValues] = React.useState({});
  const imageSchemaSig = React.useMemo(
    () => JSON.stringify(imageParamSchema || {}),
    [imageParamSchema],
  );
  React.useEffect(() => {
    setImageParamValues(imageParamSchema ? defaultsOf(imageParamSchema) : {});
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

  // 数据加载
  useDataLoader(
    userState,
    inputs,
    handleInputChange,
    setModels,
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

  // 首条 prompt 自动给会话命名：标题仍是默认名时，用新 prompt 的前若干
  // 字符填进去。后续 prompt 不改。
  const maybeAutoNameSession = useCallback(
    (promptText) => {
      if (!promptText || !activeSessionId) return;
      const trimmed = String(promptText).trim();
      if (!trimmed) return;
      const sess = sessions.find((s) => s.id === activeSessionId);
      if (!sess) return;
      const defaultTitles = ['', '未命名会话', '新会话'];
      if (!defaultTitles.includes(sess.title || '')) return;
      const singleLine = trimmed.replace(/\s+/g, ' ');
      const name =
        singleLine.length > 20 ? singleLine.slice(0, 20) + '…' : singleLine;
      renameSession(activeSessionId, name);
    },
    [activeSessionId, sessions, renameSession],
  );

  const handleGenerateImage = useCallback(
    async ({ prompt }) => {
      if (!prompt || !prompt.trim()) return;
      maybeAutoNameSession(prompt);
      // 真实消息活动：把当前会话顶到列表顶部
      if (activeSessionId) touchSession?.(activeSessionId);
      const params = { ...imageParamValues };
      const userMsg = createMessage(MESSAGE_ROLES.USER, prompt);
      const loadingMsg = createLoadingAssistantMessage();
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
            content: result.images.map((img) => ({
              type: 'image_url',
              image_url: { url: img.url },
              revised_prompt: img.revisedPrompt,
            })),
            meta: {
              model: inputs.model,
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

  // 发送消息
  function onMessageSend(content, _attachment) {
    // 用首条消息自动命名
    maybeAutoNameSession(typeof content === 'string' ? content : '');
    // 真实消息活动：把当前会话顶到列表顶部
    if (activeSessionId) touchSession?.(activeSessionId);

    // 创建用户消息和加载消息
    const userMessage = createMessage(MESSAGE_ROLES.USER, content);
    const loadingMessage = createLoadingAssistantMessage();

    // 如果是自定义请求体模式
    if (customRequestMode && customRequestBody) {
      try {
        const customPayload = JSON.parse(customRequestBody);

        setMessage((prevMessage) => {
          const newMessages = [...prevMessage, userMessage, loadingMessage];

          // 发送自定义请求体
          sendRequest(customPayload, customPayload.stream !== false);

          // 发送消息后保存，传入新消息列表
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

    // 默认模式
    const validImageUrls = inputs.imageUrls.filter((url) => url.trim() !== '');
    const messageContent = buildMessageContent(
      content,
      validImageUrls,
      inputs.imageEnabled,
    );
    const userMessageWithImages = createMessage(
      MESSAGE_ROLES.USER,
      messageContent,
    );

    setMessage((prevMessage) => {
      const newMessages = [...prevMessage, userMessageWithImages];

      const payload = buildApiPayload(
        newMessages,
        null,
        inputs,
        parameterEnabled,
        currentModality,
      );
      sendRequest(payload, inputs.stream);

      // 禁用图片模式
      if (inputs.imageEnabled) {
        setTimeout(() => {
          handleInputChange('imageEnabled', false);
        }, 100);
      }

      // 发送消息后保存，传入新消息列表（包含用户消息和加载消息）
      const messagesWithLoading = [...newMessages, loadingMessage];
      setTimeout(() => saveMessagesImmediately(messagesWithLoading), 0);

      return messagesWithLoading;
    });
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

  // 渲染函数
  const renderCustomChatContent = useCallback(
    ({ message, className }) => {
      const isCurrentlyEditing = editingMessageId === message.id;

      return (
        <OptimizedMessageContent
          message={message}
          className={className}
          styleState={styleState}
          onToggleReasoningExpansion={toggleReasoningExpansion}
          isEditing={isCurrentlyEditing}
          onEditSave={handleEditSave}
          onEditCancel={handleEditCancel}
          editValue={editValue}
          onEditValueChange={setEditValue}
        />
      );
    },
    [
      styleState,
      editingMessageId,
      editValue,
      handleEditSave,
      handleEditCancel,
      setEditValue,
      toggleReasoningExpansion,
    ],
  );

  const renderChatBoxAction = useCallback(
    (props) => {
      const { message: currentMessage } = props;
      const isAnyMessageGenerating = message.some(
        (msg) => msg.status === 'loading' || msg.status === 'incomplete',
      );
      const isCurrentlyEditing = editingMessageId === currentMessage.id;

      return (
        <OptimizedMessageActions
          message={currentMessage}
          styleState={styleState}
          onMessageReset={messageActions.handleMessageReset}
          onMessageCopy={messageActions.handleMessageCopy}
          onMessageDelete={messageActions.handleMessageDelete}
          onRoleToggle={messageActions.handleRoleToggle}
          onMessageEdit={handleMessageEdit}
          isAnyMessageGenerating={isAnyMessageGenerating}
          isEditing={isCurrentlyEditing}
        />
      );
    },
    [messageActions, styleState, message, editingMessageId, handleMessageEdit],
  );

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

  return (
    <PlaygroundProvider value={playgroundContextValue}>
      <div className='h-full'>
        <Layout className='h-full bg-transparent flex flex-col md:flex-row'>
          {(showSettings || !isMobile) && (
            <Layout.Sider
              className={`
              bg-transparent border-r-0 flex-shrink-0 overflow-auto mt-[60px]
              ${
                isMobile
                  ? 'fixed top-0 left-0 right-0 bottom-0 z-[1000] w-full h-auto bg-white shadow-lg'
                  : 'relative z-[1] w-80 h-[calc(100vh-66px)]'
              }
            `}
              width={isMobile ? '100%' : 320}
            >
              <OptimizedSettingsPanel
                inputs={inputs}
                models={models}
                groups={groups}
                currentModality={currentModality}
                currentWorkspaceType={currentWorkspaceType}
                styleState={styleState}
                showSettings={showSettings}
                customRequestMode={customRequestMode}
                onInputChange={handleInputChange}
                onCloseSettings={() => setShowSettings(false)}
                sessions={sessions}
                activeSessionId={activeSessionId}
                onSwitchSession={switchSession}
                onCreateSession={(wsType) => handleNewChat(wsType)}
                onRenameSession={renameSession}
                onDeleteSession={deleteSession}
              />
            </Layout.Sider>
          )}

          <Layout.Content className='relative flex-1 overflow-hidden'>
            <div className='overflow-hidden flex flex-col lg:flex-row h-[calc(100vh-66px)] mt-[60px]'>
              <div className='flex-1 flex flex-col'>
                <WorkspaceRouter
                  workspaceType={currentWorkspaceType}
                  currentModelName={inputs.model}
                  chatAreaProps={{
                    chatRef,
                    message,
                    inputs,
                    styleState,
                    showDebugPanel,
                    roleInfo,
                    onMessageSend,
                    onMessageCopy: messageActions.handleMessageCopy,
                    onMessageReset: messageActions.handleMessageReset,
                    onMessageDelete: messageActions.handleMessageDelete,
                    onStopGenerator,
                    onClearMessages: handleClearMessages,
                    onToggleDebugPanel: () => setShowDebugPanel(!showDebugPanel),
                    renderCustomChatContent,
                    renderChatBoxAction,
                  }}
                  imageWorkspaceProps={{
                    message,
                    inputs,
                    styleState,
                    loading: imageGenerating,
                    onGenerate: handleGenerateImage,
                    onDeleteGeneration: handleDeleteGeneration,
                    onClearAll: handleClearMessages,
                    showDebugPanel,
                    onToggleDebugPanel: () => setShowDebugPanel(!showDebugPanel),
                  }}
                  placeholderProps={{
                    modelName: inputs.model,
                    styleState,
                    onSwitchToCustomRequest: () => {
                      setCustomRequestMode(true);
                      setRightPanelTab(RIGHT_PANEL_TABS.PARAMS);
                      if (!showDebugPanel) setShowDebugPanel(true);
                    },
                  }}
                />
              </div>

              {/* 右侧面板 - 桌面端：参数/调试 Tab 切换，可通过右上角按钮收起 */}
              {showDebugPanel && !isMobile && (
                <div className='w-96 flex-shrink-0 h-full'>
                  <PlaygroundRightPanel
                    styleState={styleState}
                    activeTab={rightPanelTab}
                    onActiveTabChange={setRightPanelTab}
                    inputs={inputs}
                    parameterEnabled={parameterEnabled}
                    currentModality={currentModality}
                    currentWorkspaceType={currentWorkspaceType}
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
                  currentWorkspaceType={currentWorkspaceType}
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

            {/* 浮动按钮 */}
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
