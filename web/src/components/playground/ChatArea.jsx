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

import React from 'react';
import { Card, Chat, Typography, Button, Popconfirm } from '@douyinfe/semi-ui';
import { Eye, EyeOff, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { getLogo } from '../../helpers';
import { useActualTheme } from '../../context/Theme';
import { buildDefaultPlaygroundLogo } from './workspaceLogo';
import AssistantBubbleRouter from './bubbles/AssistantBubbleRouter';
import UnifiedInputBar from './UnifiedInputBar';
import { OptimizedMessageActions } from './OptimizedComponents';

// 统一对话窗口：一条消息流里同时承载文本/图片/视频气泡，按 modality 分发
// 渲染。输入区域是统一的 UnifiedInputBar——模型选择、新会话、参数、附件、
// 发送都集中在那里；ChatArea 自身只负责头部 + 消息流 + 空态。
//
// 发送时 onSubmit(text) 被调用，由父层根据当前模型 modality 路由到三套
// 后端 hook（chat / image / video），再把结果消息回写到 message 数组。
const ChatArea = ({
  chatRef,
  message,
  inputs,
  styleState,
  showDebugPanel,
  roleInfo,
  onMessageSend,        // (text) => void  父层按当前 modality 分派
  onMessageCopy,
  onMessageReset,
  onMessageDelete,
  onStopGenerator,
  onClearMessages,
  onToggleDebugPanel,
  onMessageEdit,
  editingMessageId,
  onEditValueChange,
  editValue,
  onEditSave,
  onEditCancel,
  onToggleReasoningExpansion,
  onImageContinueEdit,
  imageSupportsContinueEdit,

  // UnifiedInputBar 透传 props
  modelEntries,
  currentModelEntry,
  currentModality,
  onModelGroupChange,
  paramSchema,
  paramValues,
  onParamValuesChange,
  loading,
  acceptsReferenceImage,
  showUploadButton,
  referenceImages,
  onAddReferenceImage,
  onRemoveReferenceImage,
  onRetryUpload,
  // 视频「全能参考 / 首尾帧」模式控制（仅 modeSwitchAvailable 时启用）
  videoInputModeAvailable,
  videoInputMode,
  onVideoInputModeChange,
  firstLastFrameImages,
  onSwapFirstLastFrame,
  onRemoveFirstFrame,
  onRemoveLastFrame,
}) => {
  const { t } = useTranslation();

  const hasMessages = Array.isArray(message) && message.length > 0;
  const actualTheme = useActualTheme();
  const logoUrl =
    getLogo() || buildDefaultPlaygroundLogo(actualTheme === 'dark');

  // 整个聊天面板（含消息流）也接受拖拽上传——之前只有输入框监听 dragover/
  // drop，所以拖到上方的消息区松手时图片不会进入参考图，只会触发浏览器
  // 默认行为（在新标签打开）。这里把 drop 提到 Card 根，文件路由到
  // onAddReferenceImage（父层会按 modality 决定接受还是 toast 拒绝）。
  const [areaDragOver, setAreaDragOver] = React.useState(false);
  const dragDepthRef = React.useRef(0);
  const handleAreaDragEnter = (e) => {
    if (!e.dataTransfer?.types?.includes('Files')) return;
    e.preventDefault();
    dragDepthRef.current += 1;
    if (acceptsReferenceImage) setAreaDragOver(true);
  };
  const handleAreaDragOver = (e) => {
    if (e.dataTransfer?.types?.includes('Files')) {
      // 必须 preventDefault，否则浏览器拒绝触发后续 drop 事件
      e.preventDefault();
    }
  };
  const handleAreaDragLeave = (e) => {
    if (!e.dataTransfer?.types?.includes('Files')) return;
    dragDepthRef.current = Math.max(0, dragDepthRef.current - 1);
    if (dragDepthRef.current === 0) setAreaDragOver(false);
  };
  const handleAreaDrop = (e) => {
    if (!e.dataTransfer?.types?.includes('Files')) return;
    e.preventDefault();
    dragDepthRef.current = 0;
    setAreaDragOver(false);
    const files = e.dataTransfer?.files;
    if (!files || files.length === 0) return;
    Array.from(files).forEach((file) => {
      if (!file?.type?.startsWith('image/')) return;
      onAddReferenceImage?.(file);
    });
  };

  // assistant 消息按 modality 分发到 ImageBubble / VideoBubble / 文本气泡
  const renderCustomChatContent = React.useCallback(
    ({ message: msg, className }) => {
      const isCurrentlyEditing = editingMessageId === msg.id;
      // 用户消息走文本气泡（含 image_url[] 多模态附件）；助手消息走分发器。
      // 用户气泡里的 RefImageGrid 也接 onImageContinueEdit——hover 时浮出
      // 编辑按钮，让用户能把过往参考图复用为新参考图。
      if (msg.role !== 'assistant') {
        return (
          <AssistantBubbleRouter
            message={msg}
            className={className}
            styleState={styleState}
            onToggleReasoningExpansion={onToggleReasoningExpansion}
            isEditing={isCurrentlyEditing}
            onEditSave={onEditSave}
            onEditCancel={onEditCancel}
            editValue={editValue}
            onEditValueChange={onEditValueChange}
            onImageContinueEdit={onImageContinueEdit}
          />
        );
      }
      return (
        <AssistantBubbleRouter
          message={msg}
          className={className}
          styleState={styleState}
          onToggleReasoningExpansion={onToggleReasoningExpansion}
          isEditing={isCurrentlyEditing}
          onEditSave={onEditSave}
          onEditCancel={onEditCancel}
          editValue={editValue}
          onEditValueChange={onEditValueChange}
          onImageContinueEdit={onImageContinueEdit}
          imageSupportsContinueEdit={imageSupportsContinueEdit}
        />
      );
    },
    [
      styleState,
      editingMessageId,
      editValue,
      onEditSave,
      onEditCancel,
      onEditValueChange,
      onToggleReasoningExpansion,
      onImageContinueEdit,
      imageSupportsContinueEdit,
    ],
  );

  const renderChatBoxAction = React.useCallback(
    // 只用我们自己的 hover-toggle class（playground-message-actions），
    // 不挂 Semi 的 `.semi-chat-chatBox-action`——后者会带 flex/column-gap/
    // margin 等布局规则，会改变按钮排原本的样式。
    ({ message: currentMessage }) => {
      const isAnyMessageGenerating = (message || []).some(
        (m) => m.status === 'loading' || m.status === 'incomplete' || m.status === 'polling',
      );
      const isCurrentlyEditing = editingMessageId === currentMessage.id;
      return (
        <div className='playground-message-actions'>
          <OptimizedMessageActions
            message={currentMessage}
            styleState={styleState}
            onMessageReset={onMessageReset}
            onMessageCopy={onMessageCopy}
            onMessageDelete={onMessageDelete}
            onMessageEdit={onMessageEdit}
            isAnyMessageGenerating={isAnyMessageGenerating}
            isEditing={isCurrentlyEditing}
          />
        </div>
      );
    },
    [
      message,
      styleState,
      editingMessageId,
      onMessageReset,
      onMessageCopy,
      onMessageDelete,
      onMessageEdit,
    ],
  );

  return (
    <Card
      className='h-full'
      bordered={false}
      bodyStyle={{
        padding: 0,
        height: 'calc(100vh - 66px)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        position: 'relative',
      }}
      onDragEnter={handleAreaDragEnter}
      onDragOver={handleAreaDragOver}
      onDragLeave={handleAreaDragLeave}
      onDrop={handleAreaDrop}
    >
      {/* 头部：去掉左侧标题/模型名（参考 ChatGPT 风格，模型信息已经在
          输入栏的模型选择 pill 上展示），头部仅保留右侧的「清空 / 显示
          面板」操作按钮，渲染成轻量行而不是彩色 banner。 */}
      {styleState.isMobile ? (
        <div className='pt-4'></div>
      ) : (
        <div className='px-4 pt-3 pb-2'>
          <div className='flex items-center justify-end gap-1'>
            {hasMessages && onClearMessages && (
              <Popconfirm
                title={t('清空当前会话？')}
                content={t('此操作不可恢复')}
                onConfirm={onClearMessages}
              >
                <Button
                  icon={<Trash2 size={14} />}
                  theme='borderless'
                  type='tertiary'
                  size='small'
                  className='!rounded-lg'
                >
                  {t('清空')}
                </Button>
              </Popconfirm>
            )}
            {onToggleDebugPanel && (
              <Button
                icon={showDebugPanel ? <EyeOff size={14} /> : <Eye size={14} />}
                onClick={onToggleDebugPanel}
                theme='borderless'
                type='tertiary'
                size='small'
                className='!rounded-lg'
              >
                {showDebugPanel ? t('隐藏面板') : t('显示面板')}
              </Button>
            )}
          </div>
        </div>
      )}

      {/* 消息流：居中容器控制最大宽度。横向 padding 必须和输入栏
          UnifiedInputBar 一致（px-3 sm:px-4），否则气泡可触区会比输入框
          可见区域更宽，hover 高亮越界看着突兀。 */}
      <div className='flex-1 overflow-hidden flex flex-col items-stretch'>
        <div
          className='flex-1 min-h-0 w-full mx-auto relative px-3 sm:px-4'
          style={{ maxWidth: 860 }}
        >
          <Chat
            ref={chatRef}
            chatBoxRenderConfig={{
              renderChatBoxContent: renderCustomChatContent,
              renderChatBoxAction: renderChatBoxAction,
              renderChatBoxTitle: () => null,
            }}
            // 隐藏 Semi 自带的输入区，输入完全交给 UnifiedInputBar
            renderInputArea={() => null}
            roleConfig={roleInfo}
            style={{
              height: '100%',
              maxWidth: '100%',
              overflow: 'hidden',
            }}
            chats={message}
            onMessageSend={onMessageSend}
            onMessageCopy={onMessageCopy}
            onMessageReset={onMessageReset}
            onMessageDelete={onMessageDelete}
            showClearContext={false}
            // 关掉 Semi 自带的「输入框上方停止条」，改成由 UnifiedInputBar
            // 的发送按钮在 loading 时直接变成停止按钮，单一入口更清晰
            showStopGenerate={false}
            onClear={onClearMessages}
            className='h-full'
            placeholder={t('请输入您的问题...')}
          />
          {/* 空态：覆盖在 Chat 之上，不拦截点击。先看模型再看消息 */}
          {!hasMessages && (
            <div
              className='absolute inset-0 flex flex-col items-center justify-center pointer-events-none select-none px-6'
              style={{ paddingBottom: 140 }}
            >
              <img
                src={logoUrl}
                alt=''
                style={{
                  width: 80,
                  height: 80,
                  opacity: 0.75,
                }}
                className='mb-4'
              />
              <Typography.Text
                type='tertiary'
                className='text-sm text-center'
              >
                {inputs.model
                  ? t('使用 {{model}} 开始一段对话', { model: inputs.model })
                  : t('在下方输入框选择一个模型开始')}
              </Typography.Text>
            </div>
          )}
        </div>
      </div>

      {/* 统一输入条：模型选择 / 新会话 / 参考图 / 发送 都在这里 */}
      <UnifiedInputBar
        inputs={inputs}
        modelEntries={modelEntries}
        currentModelEntry={currentModelEntry}
        currentModality={currentModality}
        onModelGroupChange={onModelGroupChange}
        loading={loading}
        onSubmit={onMessageSend}
        onStop={onStopGenerator}
        paramSchema={paramSchema}
        paramValues={paramValues}
        onParamValuesChange={onParamValuesChange}
        acceptsReferenceImage={acceptsReferenceImage}
        showUploadButton={showUploadButton}
        referenceImages={referenceImages}
        onAddReferenceImage={onAddReferenceImage}
        onRemoveReferenceImage={onRemoveReferenceImage}
        onRetryUpload={onRetryUpload}
        videoInputModeAvailable={videoInputModeAvailable}
        videoInputMode={videoInputMode}
        onVideoInputModeChange={onVideoInputModeChange}
        firstLastFrameImages={firstLastFrameImages}
        onSwapFirstLastFrame={onSwapFirstLastFrame}
        onRemoveFirstFrame={onRemoveFirstFrame}
        onRemoveLastFrame={onRemoveLastFrame}
        styleState={styleState}
      />
    </Card>
  );
};

export default ChatArea;
