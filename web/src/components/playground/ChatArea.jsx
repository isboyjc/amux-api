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
import CustomInputRender from './CustomInputRender';
import { getLogo } from '../../helpers';
import { useActualTheme } from '../../context/Theme';
import { buildDefaultPlaygroundLogo } from './workspaceLogo';

const ChatArea = ({
  chatRef,
  message,
  inputs,
  styleState,
  showDebugPanel,
  roleInfo,
  onMessageSend,
  onMessageCopy,
  onMessageReset,
  onMessageDelete,
  onStopGenerator,
  onClearMessages,
  onToggleDebugPanel,
  renderCustomChatContent,
  renderChatBoxAction,
}) => {
  const { t } = useTranslation();

  const renderInputArea = React.useCallback((props) => {
    return <CustomInputRender {...props} />;
  }, []);

  const hasMessages = Array.isArray(message) && message.length > 0;
  const actualTheme = useActualTheme();
  const logoUrl =
    getLogo() || buildDefaultPlaygroundLogo(actualTheme === 'dark');

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
      }}
    >
      {/* 头部：跨满宽度，左侧标题/副标题 + 右侧操作按钮（清空、显示面板） */}
      {styleState.isMobile ? (
        <div className='pt-4'></div>
      ) : (
        <div className='px-6 py-4 bg-gradient-to-r from-purple-500 to-blue-500 rounded-t-2xl'>
          <div className='flex items-center justify-between'>
            <div className='flex flex-col min-w-0'>
              <Typography.Title heading={5} className='!text-white mb-0'>
                {t('AI 对话')}
              </Typography.Title>
              <Typography.Text className='!text-white/80 text-sm hidden sm:inline truncate'>
                {inputs.model || t('选择模型开始对话')}
              </Typography.Text>
            </div>
            <div className='flex items-center gap-2'>
              {hasMessages && onClearMessages && (
                <Popconfirm
                  title={t('清空当前会话？')}
                  content={t('此操作不可恢复')}
                  onConfirm={onClearMessages}
                >
                  <Button
                    icon={<Trash2 size={14} />}
                    theme='borderless'
                    type='primary'
                    size='small'
                    className='!rounded-lg !text-white/80 hover:!text-white hover:!bg-white/10'
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
                  type='primary'
                  size='small'
                  className='!rounded-lg !text-white/80 hover:!text-white hover:!bg-white/10'
                >
                  {showDebugPanel ? t('隐藏面板') : t('显示面板')}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 聊天与输入内容：居中容器控制最大宽度，保持阅读舒适感 */}
      <div className='flex-1 overflow-hidden flex flex-col items-stretch'>
        <div
          className='flex-1 min-h-0 w-full mx-auto relative'
          style={{ maxWidth: 860 }}
        >
          <Chat
            ref={chatRef}
            chatBoxRenderConfig={{
              renderChatBoxContent: renderCustomChatContent,
              renderChatBoxAction: renderChatBoxAction,
              renderChatBoxTitle: () => null,
            }}
            renderInputArea={renderInputArea}
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
            showStopGenerate
            onStopGenerator={onStopGenerator}
            onClear={onClearMessages}
            className='h-full'
            placeholder={t('请输入您的问题...')}
          />
          {/* 空态：覆盖在 Chat 之上，不拦截点击，让下方输入区依然可用 */}
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
                {t('开始一段对话')}
              </Typography.Text>
            </div>
          )}
        </div>
      </div>
    </Card>
  );
};

export default ChatArea;
