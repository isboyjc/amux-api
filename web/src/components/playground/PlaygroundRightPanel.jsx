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
import {
  Card,
  Tabs,
  TabPane,
  Typography,
  Switch,
  Banner,
  Button,
} from '@douyinfe/semi-ui';
import { Sliders, Bug, ToggleLeft, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import ParameterControl from './ParameterControl';
import CustomRequestEditor from './CustomRequestEditor';
import DebugPanel from './DebugPanel';
import SchemaParamsRenderer from './SchemaParamsRenderer';
import {
  MODALITY,
  PLAYGROUND_SUPPORTED_MODALITIES,
} from '../../constants/playground.constants';
import { getModalityShortLabel } from '../../constants/modalityLabels';

const RIGHT_PANEL_TABS = {
  PARAMS: 'params',
  DEBUG: 'debug',
};

const PlaygroundRightPanel = ({
  // 通用
  styleState,
  activeTab = RIGHT_PANEL_TABS.PARAMS,
  onActiveTabChange,
  onClose,
  // 参数 tab
  inputs,
  parameterEnabled,
  currentModality = MODALITY.TEXT,
  customRequestMode,
  customRequestBody,
  onInputChange,
  onParameterToggle,
  onCustomRequestModeChange,
  onCustomRequestBodyChange,
  previewPayload,
  // image workspace 的 schema 驱动参数
  paramSchema,
  paramValues,
  onParamValuesChange,
  // 调试 tab
  debugData,
  activeDebugTab,
  onActiveDebugTabChange,
}) => {
  const { t } = useTranslation();

  // workspace 概念已被合并到「按消息 modality 自适应」的统一窗口里。
  // 右侧参数面板按当前选中的模型 modality 动态切换：
  //   text / multimodal → 显示 chat 标准参数（temp/top_p/...）+ 流式开关
  //   image            → 显示 schema 驱动的参数（size/quality/...）
  //   其它             → Banner 提示，建议用自定义请求体
  const isTextLike =
    currentModality === MODALITY.TEXT ||
    currentModality === MODALITY.MULTIMODAL;
  const isChatWorkspace = isTextLike;
  const isImageWorkspace = currentModality === MODALITY.IMAGE;
  const isUnsupportedModality =
    !PLAYGROUND_SUPPORTED_MODALITIES.has(currentModality);
  const modalityLabel = getModalityShortLabel(t, currentModality);

  const renderParamsTab = () => (
    <div className='space-y-6 p-4'>
      {/* 自定义请求体编辑器 */}
      <CustomRequestEditor
        customRequestMode={customRequestMode}
        customRequestBody={customRequestBody}
        onCustomRequestModeChange={onCustomRequestModeChange}
        onCustomRequestBodyChange={onCustomRequestBodyChange}
        defaultPayload={previewPayload}
      />

      {/* 不支持 modality 提示（image 已经支持，不会走到这里） */}
      {isUnsupportedModality && !customRequestMode && !isImageWorkspace && (
        <Banner
          type='info'
          closeIcon={null}
          description={t(
            '当前模型类别（{{modality}}）的专属参数界面即将上线，现阶段操练场仅提供基础请求入口。你可以切换到自定义请求体模式手动调参。',
            { modality: modalityLabel },
          )}
        />
      )}

      {/* 图片 workspace：按当前模型的 param_schema 动态渲染参数 */}
      {isImageWorkspace && (
        <div className={customRequestMode ? 'opacity-50 pointer-events-none' : ''}>
          <SchemaParamsRenderer
            schema={paramSchema}
            values={paramValues}
            onChange={onParamValuesChange}
            disabled={customRequestMode}
          />
        </div>
      )}

      {/* 文本类参数 - 仅 chat workspace (text / multimodal) */}
      {isChatWorkspace && isTextLike && (
        <div className={customRequestMode ? 'opacity-50' : ''}>
          <ParameterControl
            inputs={inputs}
            parameterEnabled={parameterEnabled}
            onInputChange={onInputChange}
            onParameterToggle={onParameterToggle}
            disabled={customRequestMode}
          />
        </div>
      )}

      {/* 流式输出开关 - chat workspace 专属 */}
      {isChatWorkspace && isTextLike && (
        <div className={customRequestMode ? 'opacity-50' : ''}>
          <div className='flex items-center justify-between'>
            <div className='flex items-center gap-2'>
              <ToggleLeft size={16} className='text-gray-500' />
              <Typography.Text strong className='text-sm'>
                {t('流式输出')}
              </Typography.Text>
              {customRequestMode && (
                <Typography.Text className='text-xs text-orange-600'>
                  ({t('已在自定义模式中忽略')})
                </Typography.Text>
              )}
            </div>
            <Switch
              checked={inputs.stream}
              onChange={(checked) => onInputChange('stream', checked)}
              checkedText={t('开')}
              uncheckedText={t('关')}
              size='small'
              disabled={customRequestMode}
            />
          </div>
        </div>
      )}
    </div>
  );

  const renderDebugTab = () => (
    <DebugPanel
      debugData={debugData}
      activeDebugTab={activeDebugTab}
      onActiveDebugTabChange={onActiveDebugTabChange}
      styleState={styleState}
      customRequestMode={customRequestMode}
      onCloseDebugPanel={undefined}
    />
  );

  return (
    <Card
      className='h-full flex flex-col'
      bordered={false}
      bodyStyle={{
        padding: 0,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <div
        className='flex items-center justify-between flex-shrink-0 border-b'
        style={{
          borderColor: 'var(--semi-color-border)',
          paddingRight: 12,
        }}
      >
        <Tabs
          type='line'
          activeKey={activeTab}
          onChange={onActiveTabChange}
          className='flex-1 min-w-0'
        >
          <TabPane
            itemKey={RIGHT_PANEL_TABS.PARAMS}
            tab={
              <span className='flex items-center gap-1.5'>
                <Sliders size={14} />
                {t('参数')}
              </span>
            }
          />
          <TabPane
            itemKey={RIGHT_PANEL_TABS.DEBUG}
            tab={
              <span className='flex items-center gap-1.5'>
                <Bug size={14} />
                {t('调试')}
              </span>
            }
          />
        </Tabs>
        {onClose && styleState?.isMobile && (
          <Button
            icon={<X size={16} />}
            onClick={onClose}
            theme='borderless'
            type='tertiary'
            size='small'
            className='!rounded-lg'
          />
        )}
      </div>

      <div className='flex-1 overflow-y-auto'>
        {activeTab === RIGHT_PANEL_TABS.DEBUG ? renderDebugTab() : renderParamsTab()}
      </div>
    </Card>
  );
};

export { RIGHT_PANEL_TABS };
export default PlaygroundRightPanel;
