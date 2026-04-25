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

import React, { useState, useRef, useEffect } from 'react';
import { Button, Input, Popconfirm, Typography } from '@douyinfe/semi-ui';
import { Plus, Trash2, Edit3 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

// 操练场左侧会话列表，对齐 ChatGPT / Claude 的视觉骨架：
//   - 顶部：醒目的「新建会话」按钮，整行宽
//   - 中间：section 小标题「最近会话」
//   - 下方：会话条目列表，撑满剩余高度、超出滚动
//
// 列表外层容器需要 h-full 才能让内层 flex 算高度；调用方在 Layout.Sider
// 里把 SessionList 直接套在一个 h-full 的 flex 容器里即可。
const SessionList = ({
  sessions = [],
  activeId,
  onSwitch,
  onCreate,
  onRename,
  onDelete,
}) => {
  const { t } = useTranslation();
  const [renamingId, setRenamingId] = useState(null);
  const [renameDraft, setRenameDraft] = useState('');
  const renameInputRef = useRef(null);

  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      setTimeout(() => {
        try {
          renameInputRef.current.focus();
        } catch {}
      }, 0);
    }
  }, [renamingId]);

  const startRename = (s) => {
    setRenamingId(s.id);
    setRenameDraft(s.title || '');
  };
  const commitRename = async () => {
    const id = renamingId;
    const title = (renameDraft || '').trim() || t('未命名会话');
    setRenamingId(null);
    setRenameDraft('');
    if (id && onRename) await onRename(id, title);
  };
  const cancelRename = () => {
    setRenamingId(null);
    setRenameDraft('');
  };

  return (
    <div className='flex flex-col h-full min-h-0'>
      {/* 顶部「新建会话」按钮：整行宽，主色 light-default 背景，靠左带 +
          图标的现代聊天产品标准做法 */}
      <Button
        block
        theme='light'
        type='primary'
        icon={<Plus size={16} />}
        onClick={() => onCreate?.()}
        className='!justify-center !rounded-xl !h-9'
        style={{ flexShrink: 0 }}
      >
        {t('新建会话')}
      </Button>

      {/* 「最近会话」section 标题 */}
      <div
        className='flex items-center justify-between mt-4 mb-2 px-1'
        style={{ flexShrink: 0 }}
      >
        <Typography.Text
          type='tertiary'
          style={{
            fontSize: 11,
            fontWeight: 600,
            letterSpacing: 0.4,
            textTransform: 'uppercase',
          }}
        >
          {t('最近会话')}
        </Typography.Text>
        <Typography.Text type='tertiary' style={{ fontSize: 11 }}>
          {sessions.length}
        </Typography.Text>
      </div>

      {/* 会话列表：撑满剩余高度，溢出滚动；每条统一 36px 高 */}
      <div
        className='flex-1 min-h-0 overflow-y-auto pr-0.5'
        style={{ scrollbarWidth: 'thin' }}
      >
        {sessions.length === 0 ? (
          <Typography.Text
            type='tertiary'
            className='text-xs px-2 py-6 block text-center'
          >
            {t('暂无会话')}
          </Typography.Text>
        ) : (
          <div className='flex flex-col gap-0.5'>
            {sessions.map((s) => {
              const isActive = s.id === activeId;
              const isRenaming = renamingId === s.id;
              return (
                <div
                  key={s.id}
                  className={`group rounded-lg cursor-pointer transition-colors flex items-center gap-2 ${
                    isActive ? '' : 'hover:bg-semi-color-fill-1'
                  }`}
                  style={{
                    height: 36,
                    paddingLeft: 10,
                    paddingRight: 6,
                    backgroundColor: isActive
                      ? 'var(--semi-color-primary-light-default)'
                      : undefined,
                  }}
                  onClick={() => {
                    if (!isRenaming && onSwitch) onSwitch(s.id);
                  }}
                >
                  <div className='flex-1 min-w-0'>
                    {isRenaming ? (
                      <Input
                        ref={renameInputRef}
                        size='small'
                        value={renameDraft}
                        onChange={setRenameDraft}
                        onBlur={commitRename}
                        onEnterPress={commitRename}
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') cancelRename();
                        }}
                        onClick={(e) => e.stopPropagation()}
                        autoFocus
                      />
                    ) : (
                      <Typography.Text
                        ellipsis={{ showTooltip: true }}
                        className='text-sm'
                        strong={isActive}
                      >
                        {s.title || t('未命名会话')}
                      </Typography.Text>
                    )}
                  </div>
                  {!isRenaming && (
                    <div className='flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-0'>
                      <Button
                        size='small'
                        theme='borderless'
                        type='tertiary'
                        icon={<Edit3 size={12} />}
                        onClick={(e) => {
                          e.stopPropagation();
                          startRename(s);
                        }}
                        aria-label={t('重命名')}
                      />
                      <Popconfirm
                        title={t('删除该会话？')}
                        content={t('此操作不可恢复')}
                        onConfirm={(e) => {
                          if (e?.stopPropagation) e.stopPropagation();
                          if (onDelete) onDelete(s.id);
                        }}
                      >
                        <Button
                          size='small'
                          theme='borderless'
                          type='danger'
                          icon={<Trash2 size={12} />}
                          onClick={(e) => e.stopPropagation()}
                          aria-label={t('删除')}
                        />
                      </Popconfirm>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};

export default SessionList;
