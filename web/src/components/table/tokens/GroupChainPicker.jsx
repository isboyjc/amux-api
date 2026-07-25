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

import React, { useEffect, useState } from 'react';
import {
  Button,
  Checkbox,
  Popover,
  Switch,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconArrowUp,
  IconArrowDown,
  IconClose,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

/**
 * 令牌分组链的行内编辑器。
 *
 * 列表里原来是个单选下拉，表达不了「有序的多个分组」：既看不出优先级，也没法
 * 调顺序。这里换成勾选 + 排序的面板，勾选顺序就是优先级，选中项单独列出来并
 * 提供上下移动，让「谁先谁后」一眼可见。
 *
 * 不选任何分组表示使用用户自身分组（与令牌 group 为空的语义一致）。
 */
const GroupChainPicker = ({
  value = [],
  crossGroupRetry = false,
  groupOptions = [],
  groupModelsCache = {},
  fetchGroupModels,
  onSave,
  disabled = false,
  children,
}) => {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const [draft, setDraft] = useState(value);
  const [draftCross, setDraftCross] = useState(crossGroupRetry);
  const [saving, setSaving] = useState(false);

  // 每次打开都从当前值重新起草，避免上一次未保存的改动残留
  useEffect(() => {
    if (visible) {
      setDraft(Array.isArray(value) ? value : []);
      setDraftCross(!!crossGroupRetry);
    }
  }, [visible]);

  const toggleGroup = (groupName) => {
    setDraft((prev) =>
      prev.includes(groupName)
        ? prev.filter((g) => g !== groupName)
        : // 新勾选的排在最后：勾选顺序即优先级顺序
          [...prev, groupName],
    );
  };

  const moveGroup = (from, to) => {
    setDraft((prev) => {
      if (to < 0 || to >= prev.length) return prev;
      const next = [...prev];
      [next[from], next[to]] = [next[to], next[from]];
      return next;
    });
  };

  const handleSave = async () => {
    if (typeof onSave !== 'function') return;
    setSaving(true);
    try {
      await onSave(draft, draft.length > 1 ? draftCross : false);
      setVisible(false);
    } finally {
      setSaving(false);
    }
  };

  const renderModelsPreview = (groupKey) => {
    const cache = groupModelsCache[groupKey];
    if (!cache || cache.status === 'loading') {
      return <div className='text-xs opacity-70'>{t('加载中...')}</div>;
    }
    if (cache.status === 'error') {
      return (
        <div className='text-xs' style={{ color: 'var(--semi-color-danger)' }}>
          {cache.error || t('加载失败')}
        </div>
      );
    }
    const models = cache.models || [];
    if (models.length === 0) {
      return <div className='text-xs opacity-70'>{t('该分组下暂无可用模型')}</div>;
    }
    const visibleModels = models.slice(0, 60);
    const extra = models.length - visibleModels.length;
    return (
      <div
        className='flex flex-wrap gap-1 overflow-auto'
        style={{ maxHeight: 180, maxWidth: 320 }}
      >
        {visibleModels.map((m) => (
          <Tag key={m} size='small' shape='circle' color='white'>
            {m}
          </Tag>
        ))}
        {extra > 0 && (
          <Tag size='small' shape='circle'>
            +{extra}
          </Tag>
        )}
      </div>
    );
  };

  const content = (
    <div className='p-3' style={{ width: 340 }}>
      <div className='mb-2'>
        <Text strong>{t('令牌分组')}</Text>
        <div className='text-xs text-[var(--semi-color-text-2)] mt-1'>
          {t('可多选，请求按顺序寻找拥有目标模型的分组；不选表示使用用户自身分组')}
        </div>
      </div>

      {/* 已选分组：单独列出来并可调顺序，优先级才看得见 */}
      {draft.length > 0 && (
        <div className='mb-2 p-2 rounded' style={{ background: 'var(--semi-color-fill-0)' }}>
          {draft.map((groupName, index) => (
            <div key={groupName} className='flex items-center gap-1 py-0.5'>
              <Tag color='blue' size='small' shape='circle'>
                {index + 1}
              </Tag>
              <span className='flex-1 truncate text-sm'>{groupName}</span>
              <Button
                size='small'
                theme='borderless'
                icon={<IconArrowUp />}
                disabled={index === 0}
                onClick={() => moveGroup(index, index - 1)}
              />
              <Button
                size='small'
                theme='borderless'
                icon={<IconArrowDown />}
                disabled={index === draft.length - 1}
                onClick={() => moveGroup(index, index + 1)}
              />
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={<IconClose />}
                onClick={() => toggleGroup(groupName)}
              />
            </div>
          ))}
        </div>
      )}

      <div className='overflow-auto' style={{ maxHeight: 220 }}>
        {groupOptions.map((opt) => {
          const checked = draft.includes(opt.value);
          return (
            <Tooltip
              key={opt.value}
              content={
                <div className='max-w-[340px]'>{renderModelsPreview(opt.value)}</div>
              }
              position='right'
              mouseEnterDelay={300}
              trigger='hover'
            >
              <div
                className='flex items-center gap-2 py-1 px-1 rounded cursor-pointer hover:bg-[var(--semi-color-fill-0)]'
                onMouseEnter={() => {
                  if (typeof fetchGroupModels === 'function') {
                    fetchGroupModels(opt.value);
                  }
                }}
                onClick={() => {
                  if (opt.disabled && !checked) return;
                  toggleGroup(opt.value);
                }}
              >
                <Checkbox
                  checked={checked}
                  disabled={opt.disabled && !checked}
                  onChange={() => {}}
                />
                <span className='flex flex-col min-w-0 flex-1'>
                  <span className='font-medium truncate text-sm'>{opt.value}</span>
                  {opt.label && opt.label !== opt.value && (
                    <span className='text-xs text-[var(--semi-color-text-2)] truncate'>
                      {opt.label}
                    </span>
                  )}
                </span>
                <span className='flex items-center gap-1 flex-shrink-0'>
                  {typeof opt.modelCount === 'number' && (
                    <Tag size='small' color='cyan' shape='circle'>
                      {opt.modelCount}
                    </Tag>
                  )}
                  {typeof opt.ratio === 'number' && (
                    <Tag size='small' color='green' shape='circle'>
                      {opt.ratio}x
                    </Tag>
                  )}
                </span>
              </div>
            </Tooltip>
          );
        })}
      </div>

      {draft.length > 1 && (
        <div className='flex items-center gap-2 mt-2 pt-2 border-t border-[var(--semi-color-border)]'>
          <Switch
            size='small'
            checked={draftCross}
            onChange={(v) => setDraftCross(v)}
          />
          <span className='text-xs'>{t('跨分组重试')}</span>
          <Text type='tertiary' size='small' className='flex-1'>
            {t('失败时按顺序尝试下一个拥有该模型的分组')}
          </Text>
        </div>
      )}

      <div className='flex justify-end gap-2 mt-3'>
        <Button size='small' theme='borderless' onClick={() => setVisible(false)}>
          {t('取消')}
        </Button>
        <Button size='small' theme='solid' loading={saving} onClick={handleSave}>
          {t('保存')}
        </Button>
      </div>
    </div>
  );

  if (disabled) return children;

  return (
    <Popover
      visible={visible}
      onVisibleChange={setVisible}
      trigger='click'
      position='bottomLeft'
      content={content}
    >
      <span className='inline-flex' onClick={(e) => e.stopPropagation()}>
        {children}
      </span>
    </Popover>
  );
};

export default GroupChainPicker;
