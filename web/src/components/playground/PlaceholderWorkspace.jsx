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
import { Card, Tag, Typography, Button } from '@douyinfe/semi-ui';
import {
  Image as ImageIcon,
  Video as VideoIcon,
  Mic,
  Binary,
  ArrowUpDown,
  Sparkles,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  WORKSPACE,
  WORKSPACE_COLOR,
  getWorkspaceLabel,
} from '../../constants/workspaceTypes';

const WORKSPACE_ICON = {
  [WORKSPACE.IMAGE]: ImageIcon,
  [WORKSPACE.VIDEO]: VideoIcon,
  [WORKSPACE.AUDIO]: Mic,
  [WORKSPACE.EMBEDDING]: Binary,
  [WORKSPACE.RERANK]: ArrowUpDown,
};

/**
 * 尚未实现专属界面的 workspace 的占位工作区。核心消息：操练场把这类模型的
 * 专属 UI 列入路线图，现阶段可通过右栏"参数 → 自定义请求体"手动调试。
 */
const PlaygroundPlaceholderWorkspace = ({
  workspaceType = WORKSPACE.IMAGE,
  modelName,
  styleState,
  onSwitchToCustomRequest,
}) => {
  const { t } = useTranslation();
  const Icon = WORKSPACE_ICON[workspaceType] || Sparkles;
  const label = getWorkspaceLabel(t, workspaceType);
  const color = WORKSPACE_COLOR[workspaceType] || 'grey';

  return (
    <div className='h-full flex items-center justify-center p-6'>
      <Card
        className='!rounded-2xl max-w-md w-full shadow-sm'
        bodyStyle={{ padding: 32, textAlign: 'center' }}
      >
        <div
          className='inline-flex items-center justify-center w-14 h-14 rounded-full mb-4'
          style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
        >
          <Icon size={26} />
        </div>
        <div className='mb-2'>
          <Tag color={color} size='small' shape='circle'>
            {label}
          </Tag>
        </div>
        <Typography.Title heading={5} className='mb-2'>
          {modelName || t('该模型类别')}
        </Typography.Title>
        <Typography.Paragraph
          type='secondary'
          className='text-sm mb-4'
        >
          {t(
            '当前模型类别的专属操练场界面正在开发中，敬请期待。你可以在右侧「参数」标签里开启"自定义请求体"模式，按照官方 API 规范手动构造请求并调试。',
          )}
        </Typography.Paragraph>
        {onSwitchToCustomRequest && (
          <Button
            theme='solid'
            type='primary'
            className='!rounded-lg'
            icon={<Sparkles size={14} />}
            onClick={onSwitchToCustomRequest}
          >
            {t('切换到自定义请求体')}
          </Button>
        )}
      </Card>
    </div>
  );
};

export default PlaygroundPlaceholderWorkspace;
