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
import { Sparkles, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import ImageUrlInput from './ImageUrlInput';
import SessionList from './SessionList';
import {
  MODALITY,
  PLAYGROUND_SUPPORTED_MODALITIES,
} from '../../constants/playground.constants';
import {
  getModalityLongLabel,
  getModalityShortLabel,
  MODALITY_COLOR,
} from '../../constants/modalityLabels';

// (model, group) 组合在 Select 里用 'model@@group' 编码，方便单一 value
// 字段表达「同时选中模型和分组」。@@ 是双字符分隔，不会跟模型名里的 @latest
// 这种正常字符冲突。
const MODEL_GROUP_SEP = '@@';
const encodeMG = (model, group) => `${model || ''}${MODEL_GROUP_SEP}${group || ''}`;
const decodeMG = (value) => {
  if (typeof value !== 'string' || !value) return { model: '', group: '' };
  const idx = value.indexOf(MODEL_GROUP_SEP);
  if (idx < 0) return { model: value, group: '' };
  return {
    model: value.slice(0, idx),
    group: value.slice(idx + MODEL_GROUP_SEP.length),
  };
};

// 倍率徽标颜色：和 helpers/render.jsx 里的 renderRatio 保持一致的语义
// （倍率越高用越警示的色）。
const ratioColor = (ratio) => {
  if (typeof ratio !== 'number') return 'grey';
  if (ratio > 5) return 'red';
  if (ratio > 3) return 'orange';
  if (ratio > 1) return 'blue';
  return 'green';
};

// modality 在下拉里的展示顺序：用户最常用的对话/图片/视频靠前，
// 嵌入/重排/音频这些基本只有少数高级用户会用到的放后面。
const MODALITY_GROUP_ORDER = [
  MODALITY.TEXT,
  MODALITY.MULTIMODAL,
  MODALITY.IMAGE,
  MODALITY.VIDEO,
  MODALITY.AUDIO,
  MODALITY.EMBEDDING,
  MODALITY.RERANK,
];

// 模型行：左边图标 + 模型名 + modality 短标签，右边分组名 + 倍率
const ModelGroupRow = ({ entry, t, dim }) => {
  const { model, groupLabel, group, ratio, modality } = entry;
  const modalityShort = getModalityShortLabel(t, modality);
  const modalityTagColor = MODALITY_COLOR[modality] || 'grey';
  return (
    <div
      className='flex items-center w-full gap-2'
      style={{ minWidth: 0, opacity: dim ? 0.6 : 1 }}
    >
      <Typography.Text
        strong
        className='text-sm'
        style={{
          flex: '1 1 auto',
          minWidth: 0,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {model}
      </Typography.Text>
      <Tag
        color={modalityTagColor}
        size='small'
        shape='circle'
        className='flex-shrink-0'
      >
        {modalityShort}
      </Tag>
      <div className='flex items-center gap-1 flex-shrink-0'>
        <Typography.Text
          type='tertiary'
          className='text-xs'
          style={{ maxWidth: 120 }}
          ellipsis={{ showTooltip: true }}
        >
          {groupLabel || group}
        </Typography.Text>
        {typeof ratio === 'number' && (
          <Tag color={ratioColor(ratio)} size='small' shape='circle'>
            {ratio}x
          </Tag>
        )}
      </div>
    </div>
  );
};

const SettingsPanel = ({
  inputs,
  modelEntries = [],
  currentModality = MODALITY.TEXT,
  styleState,
  customRequestMode,
  onInputChange,
  onModelGroupChange,
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

  // 当前选中的 (model, group) 编码，给 Select 用作 value
  const selectedValue = inputs.model
    ? encodeMG(inputs.model, inputs.group)
    : undefined;

  // 把 modelEntries 按 modality 分组、并在每组内按 (模型名, 倍率) 排序。
  // 排序规则：模型名 a→z；同名模型按倍率升序，让"最便宜的分组"靠近顶部，
  // 符合"用户多半想直接用最划算那个"的心智。
  const groupedEntries = React.useMemo(() => {
    const buckets = new Map();
    modelEntries.forEach((e) => {
      const key = e.modality || MODALITY.TEXT;
      if (!buckets.has(key)) buckets.set(key, []);
      buckets.get(key).push(e);
    });
    buckets.forEach((arr) => {
      arr.sort((a, b) => {
        if (a.model !== b.model) return a.model.localeCompare(b.model);
        const ra = typeof a.ratio === 'number' ? a.ratio : Infinity;
        const rb = typeof b.ratio === 'number' ? b.ratio : Infinity;
        return ra - rb;
      });
    });
    // 按 MODALITY_GROUP_ORDER 输出；遇到没在白名单里的 modality 追加到末尾
    const ordered = [];
    MODALITY_GROUP_ORDER.forEach((m) => {
      if (buckets.has(m)) {
        ordered.push({ modality: m, entries: buckets.get(m) });
        buckets.delete(m);
      }
    });
    buckets.forEach((entries, modality) => {
      ordered.push({ modality, entries });
    });
    return ordered;
  }, [modelEntries]);

  // Semi 的 Select 自带 filter 走 label/value 字符串匹配。我们把 label
  // 设成 `${model} ${groupLabel}`，让搜索同时命中模型名和分组名/倍率
  // 关键字。
  const buildOptionLabel = (entry) =>
    `${entry.model}  ${entry.groupLabel || entry.group}  ${
      typeof entry.ratio === 'number' ? entry.ratio + 'x' : ''
    }`.trim();

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
        {/* 模型 + 分组：合并成一个选择器。下拉里按 modality 分段，每行
            展示「模型名 · modality 标签 · 分组名 · 倍率」，覆盖原先两个
            独立下拉的全部信息。 */}
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
            placeholder={t('选择模型与分组')}
            name='model'
            required
            selection
            filter
            autoClearSearchValue={false}
            value={selectedValue}
            onChange={(value) => {
              const { model, group } = decodeMG(value);
              if (onModelGroupChange) onModelGroupChange(model, group);
            }}
            style={{ width: '100%' }}
            dropdownStyle={{ width: '100%', maxWidth: '100%' }}
            className='!rounded-lg'
            disabled={customRequestMode}
            // 自定义渲染选中态：显示模型 + 分组 + 倍率，比单纯一个 model
            // 字符串信息更全，避免选完之后看不出在哪个分组。
            renderSelectedItem={() => {
              const cur = modelEntries.find(
                (e) => e.model === inputs.model && e.group === inputs.group,
              );
              if (!cur) {
                return (
                  <Typography.Text type='tertiary' className='text-sm'>
                    {inputs.model || t('选择模型与分组')}
                  </Typography.Text>
                );
              }
              return <ModelGroupRow entry={cur} t={t} />;
            }}
          >
            {groupedEntries.length === 0 && (
              <Select.Option value='' disabled>
                <Typography.Text type='tertiary' className='text-xs'>
                  {t('暂无可用模型')}
                </Typography.Text>
              </Select.Option>
            )}
            {groupedEntries.map((g) => (
              <Select.OptGroup
                key={g.modality}
                label={getModalityLongLabel(t, g.modality)}
              >
                {g.entries.map((entry) => {
                  const value = encodeMG(entry.model, entry.group);
                  const label = buildOptionLabel(entry);
                  return (
                    <Select.Option
                      key={value}
                      value={value}
                      label={label}
                      showTick={false}
                      style={{ padding: '8px 12px' }}
                    >
                      <ModelGroupRow entry={entry} t={t} />
                    </Select.Option>
                  );
                })}
              </Select.OptGroup>
            ))}
          </Select>
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
