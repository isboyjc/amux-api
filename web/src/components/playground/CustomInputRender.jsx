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

import React, { useRef, useEffect, useCallback } from 'react';
import { Toast, Typography } from '@douyinfe/semi-ui';
import { ArrowUp } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { usePlayground } from '../../contexts/PlaygroundContext';

const CustomInputRender = (props) => {
  const { t } = useTranslation();
  const { onPasteImage, imageEnabled } = usePlayground();
  const { detailProps } = props;
  const { inputNode, sendNode, onClick } = detailProps;
  const containerRef = useRef(null);

  const handlePaste = useCallback(
    async (e) => {
      const items = e.clipboardData?.items;
      if (!items) return;

      for (let i = 0; i < items.length; i++) {
        const item = items[i];

        if (item.type.indexOf('image') !== -1) {
          e.preventDefault();
          const file = item.getAsFile();

          if (file) {
            try {
              if (!imageEnabled) {
                Toast.warning({
                  content: t('请先在设置中启用图片功能'),
                  duration: 3,
                });
                return;
              }

              const reader = new FileReader();
              reader.onload = (event) => {
                const base64 = event.target.result;

                if (onPasteImage) {
                  onPasteImage(base64);
                  Toast.success({
                    content: t('图片已添加'),
                    duration: 2,
                  });
                } else {
                  Toast.error({
                    content: t('无法添加图片'),
                    duration: 2,
                  });
                }
              };
              reader.onerror = () => {
                console.error('Failed to read image file:', reader.error);
                Toast.error({
                  content: t('粘贴图片失败'),
                  duration: 2,
                });
              };
              reader.readAsDataURL(file);
            } catch (error) {
              console.error('Failed to paste image:', error);
              Toast.error({
                content: t('粘贴图片失败'),
                duration: 2,
              });
            }
          }
          break;
        }
      }
    },
    [onPasteImage, imageEnabled, t],
  );

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    container.addEventListener('paste', handlePaste);
    return () => {
      container.removeEventListener('paste', handlePaste);
    };
  }, [handlePaste]);

  // 圆形发送按钮
  const styledSendNode = React.cloneElement(sendNode, {
    className: '!rounded-full flex-shrink-0 ' + (sendNode.props.className || ''),
    theme: 'solid',
    type: 'primary',
    style: {
      ...sendNode.props.style,
      width: 36,
      height: 36,
      minWidth: 36,
      padding: 0,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    },
    icon: <ArrowUp size={18} strokeWidth={2.5} />,
    children: null,
    'aria-label': t('发送'),
  });

  return (
    <div className='px-3 pb-3 sm:px-4 sm:pb-4' ref={containerRef}>
      <div
        className='rounded-2xl transition-colors focus-within:ring-2 focus-within:ring-offset-0'
        style={{
          border: '1px solid var(--semi-color-border)',
          backgroundColor: 'var(--semi-color-bg-0)',
          ['--tw-ring-color']:
            'var(--semi-color-primary-light-hover, rgba(129,140,248,0.35))',
        }}
        onClick={onClick}
        title={t('支持 Ctrl+V 粘贴图片')}
      >
        {/* 输入框（Semi Chat 注入的 inputNode）顶在上部 */}
        <div className='px-1 pt-1'>{inputNode}</div>
        {/* 底部 action 行：左侧快捷键提示，右侧圆形发送按钮 */}
        <div className='flex items-center justify-between px-2 pb-2'>
          <Typography.Text
            type='tertiary'
            className='text-xs select-none'
            style={{ paddingLeft: 4 }}
          >
            {t('Enter 发送, Shift + Enter 换行')}
          </Typography.Text>
          {styledSendNode}
        </div>
      </div>
    </div>
  );
};

export default CustomInputRender;
