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
import { Card, Select, Typography, Button, Tag, Banner } from '@douyinfe/semi-ui';
import { Sparkles, Users, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderGroupOption, renderModelOption, selectFilter } from '../../helpers';
import ImageUrlInput from './ImageUrlInput';
import SessionList from './SessionList';
import {
  MODALITY,
  PLAYGROUND_SUPPORTED_MODALITIES,
} from '../../constants/playground.constants';
import { getModalityShortLabel } from '../../constants/modalityLabels';
import {
  WORKSPACE_MODALITIES,
  isModalityInWorkspace,
} from '../../constants/workspaceTypes';

const SettingsPanel = ({
  inputs,
  models,
  groups,
  currentModality = MODALITY.TEXT,
  currentWorkspaceType,
  styleState,
  customRequestMode,
  onInputChange,
  onCloseSettings,
  // 会话
  sessions = [],
  activeSessionId,
  onSwitchSession,
  onCreateSession,
  onRenameSession,
  onDeleteSession,
}) => {
  const { t } = useTranslation();

  const showImageUrlInput = currentModality === MODALITY.MULTIMODAL;
  const isUnsupportedModality =
    !PLAYGROUND_SUPPORTED_MODALITIES.has(currentModality);
  const modalityLabel = getModalityShortLabel(t, currentModality);

  // 按当前会话的 workspace_type 过滤模型下拉
  const filteredModels = React.useMemo(() => {
    if (!currentWorkspaceType) return models;
    const allowed = WORKSPACE_MODALITIES[currentWorkspaceType];
    if (!allowed) return models;
    return models.filter((m) =>
      isModalityInWorkspace(m.modality || 'text', currentWorkspaceType),
    );
  }, [models, currentWorkspaceType]);

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
      {/* 移动端留一个关闭按钮，桌面端不需要标题栏 */}
      {styleState.isMobile && onCloseSettings && (
        <div className='flex items-center justify-end mb-3 flex-shrink-0'>
          <Button
            icon={<X size={16} />}
            onClick={onCloseSettings}
            theme='borderless'
            type='tertiary'
            size='small'
            className='!rounded-lg'
          />
        </div>
      )}

      <div className='space-y-6 overflow-y-auto flex-1 pr-2 model-settings-scroll'>
        {/* 分组选择（会话之上） */}
        <div className={customRequestMode ? 'opacity-50' : ''}>
          <div className='flex items-center gap-2 mb-2'>
            <Users size={16} className='text-gray-500' />
            <Typography.Text strong className='text-sm'>
              {t('分组')}
            </Typography.Text>
            {customRequestMode && (
              <Typography.Text className='text-xs text-orange-600'>
                ({t('已在自定义模式中忽略')})
              </Typography.Text>
            )}
          </div>
          <Select
            placeholder={t('请选择分组')}
            name='group'
            required
            selection
            filter={selectFilter}
            autoClearSearchValue={false}
            onChange={(value) => onInputChange('group', value)}
            value={inputs.group}
            autoComplete='new-password'
            optionList={groups}
            renderOptionItem={renderGroupOption}
            style={{ width: '100%' }}
            dropdownStyle={{ width: '100%', maxWidth: '100%' }}
            className='!rounded-lg'
            disabled={customRequestMode}
          />
        </div>

        {/* 模型选择（会话之上） */}
        <div className={customRequestMode ? 'opacity-50' : ''}>
          <div className='flex items-center gap-2 mb-2'>
            <Sparkles size={16} className='text-gray-500' />
            <Typography.Text strong className='text-sm'>
              {t('模型')}
            </Typography.Text>
            <Tag size='small' shape='circle' color='cyan'>
              {modalityLabel}
            </Tag>
            {customRequestMode && (
              <Typography.Text className='text-xs text-orange-600'>
                ({t('已在自定义模式中忽略')})
              </Typography.Text>
            )}
          </div>
          <Select
            placeholder={t('请选择模型')}
            name='model'
            required
            selection
            filter={selectFilter}
            autoClearSearchValue={false}
            onChange={(value) => onInputChange('model', value)}
            value={inputs.model}
            autoComplete='new-password'
            optionList={filteredModels}
            renderOptionItem={renderModelOption}
            style={{ width: '100%' }}
            dropdownStyle={{ width: '100%', maxWidth: '100%' }}
            className='!rounded-lg'
            disabled={customRequestMode}
          />
        </div>

        {/* 非支持 modality 的提示 */}
        {isUnsupportedModality && !customRequestMode && (
          <Banner
            type='info'
            closeIcon={null}
            description={t(
              '当前模型类别（{{modality}}）的专属参数界面即将上线，现阶段操练场仅提供基础请求入口。你可以切换到自定义请求体模式手动调参。',
              { modality: modalityLabel },
            )}
          />
        )}

        {/* 图片URL输入 - 仅多模态模型可见 */}
        {showImageUrlInput && (
          <div className={customRequestMode ? 'opacity-50' : ''}>
            <ImageUrlInput
              imageUrls={inputs.imageUrls}
              imageEnabled={inputs.imageEnabled}
              onImageUrlsChange={(urls) => onInputChange('imageUrls', urls)}
              onImageEnabledChange={(enabled) =>
                onInputChange('imageEnabled', enabled)
              }
              disabled={customRequestMode}
            />
          </div>
        )}

        {/* 会话列表（放到分组/模型之下） */}
        <SessionList
          sessions={sessions}
          activeId={activeSessionId}
          onSwitch={onSwitchSession}
          onCreate={onCreateSession}
          onRename={onRenameSession}
          onDelete={onDeleteSession}
        />
      </div>
    </Card>
  );
};

export default SettingsPanel;
