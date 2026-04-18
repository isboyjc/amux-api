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

import React, { useState, useEffect } from 'react';
import {
  Card,
  Typography,
  Tabs,
  TabPane,
  Button,
  Dropdown,
} from '@douyinfe/semi-ui';
import { Zap, Clock, X, Eye, Send } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import CodeViewer from './CodeViewer';
import SSEViewer from './SSEViewer';

const DebugPanel = ({
  debugData,
  activeDebugTab,
  onActiveDebugTabChange,
  styleState,
  onCloseDebugPanel,
  customRequestMode,
}) => {
  const { t } = useTranslation();

  const [activeKey, setActiveKey] = useState(activeDebugTab);

  useEffect(() => {
    setActiveKey(activeDebugTab);
  }, [activeDebugTab]);

  const handleTabChange = (key) => {
    setActiveKey(key);
    onActiveDebugTabChange(key);
  };

  const renderArrow = (items, pos, handleArrowClick, defaultNode) => {
    const style = {
      width: 32,
      height: 32,
      margin: '0 12px',
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      borderRadius: '100%',
      background: 'rgba(var(--semi-grey-1), 1)',
      color: 'var(--semi-color-text)',
      cursor: 'pointer',
    };

    return (
      <Dropdown
        render={
          <Dropdown.Menu>
            {items.map((item) => {
              return (
                <Dropdown.Item
                  key={item.itemKey}
                  onClick={() => handleTabChange(item.itemKey)}
                >
                  {item.tab}
                </Dropdown.Item>
              );
            })}
          </Dropdown.Menu>
        }
      >
        {pos === 'start' ? (
          <div style={style} onClick={handleArrowClick}>
            ←
          </div>
        ) : (
          <div style={style} onClick={handleArrowClick}>
            →
          </div>
        )}
      </Dropdown>
    );
  };

  return (
    <Card
      className='h-full flex flex-col'
      bordered={false}
      bodyStyle={{
        padding: styleState.isMobile ? '16px' : '24px',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {/* 移动端保留一个关闭按钮；桌面端由 PlaygroundRightPanel 的顶部 Tab 标识，
          这里不再重复展示"调试信息"标题以免冗余。 */}
      {styleState.isMobile && onCloseDebugPanel && (
        <div className='flex items-center justify-end mb-3 flex-shrink-0'>
          <Button
            icon={<X size={16} />}
            onClick={onCloseDebugPanel}
            theme='borderless'
            type='tertiary'
            size='small'
            className='!rounded-lg'
          />
        </div>
      )}

      <div className='flex-1 overflow-hidden debug-panel'>
        <Tabs
          renderArrow={renderArrow}
          type='card'
          collapsible
          className='h-full'
          style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
          activeKey={activeKey}
          onChange={handleTabChange}
        >
          <TabPane
            tab={
              <div className='flex items-center gap-2'>
                <Eye size={16} />
                {t('预览请求体')}
                {customRequestMode && (
                  <span className='px-1.5 py-0.5 text-xs bg-orange-100 text-orange-600 rounded-full'>
                    自定义
                  </span>
                )}
              </div>
            }
            itemKey='preview'
          >
            <CodeViewer
              content={debugData.previewRequest}
              title='preview'
              language='json'
            />
          </TabPane>

          <TabPane
            tab={
              <div className='flex items-center gap-2'>
                <Send size={16} />
                {t('实际请求体')}
              </div>
            }
            itemKey='request'
          >
            <CodeViewer
              content={debugData.request}
              title='request'
              language='json'
            />
          </TabPane>

          <TabPane
            tab={
              <div className='flex items-center gap-2'>
                <Zap size={16} />
                {t('响应')}
                {debugData.sseMessages && debugData.sseMessages.length > 0 && (
                  <span className='px-1.5 py-0.5 text-xs bg-blue-100 text-blue-600 rounded-full'>
                    SSE ({debugData.sseMessages.length})
                  </span>
                )}
              </div>
            }
            itemKey='response'
          >
            {debugData.sseMessages && debugData.sseMessages.length > 0 ? (
              <SSEViewer sseData={debugData.sseMessages} title='response' />
            ) : (
              <CodeViewer
                content={debugData.response}
                title='response'
                language='json'
              />
            )}
          </TabPane>
        </Tabs>
      </div>

      <div className='flex items-center justify-between mt-4 pt-4 flex-shrink-0'>
        {(debugData.timestamp || debugData.previewTimestamp) && (
          <div className='flex items-center gap-2'>
            <Clock size={14} className='text-gray-500' />
            <Typography.Text className='text-xs text-gray-500'>
              {activeKey === 'preview' && debugData.previewTimestamp
                ? `${t('预览更新')}: ${new Date(debugData.previewTimestamp).toLocaleString()}`
                : debugData.timestamp
                  ? `${t('最后请求')}: ${new Date(debugData.timestamp).toLocaleString()}`
                  : ''}
            </Typography.Text>
          </div>
        )}
      </div>
    </Card>
  );
};

export default DebugPanel;
