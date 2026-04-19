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
import {
  Button,
  Collapsible,
  Dropdown,
  Input,
  Popconfirm,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  ChevronDown,
  Plus,
  MessageSquare,
  Trash2,
  Edit3,
  MessageCircle,
  Image as ImageIcon,
  Video as VideoIcon,
  Mic,
  Binary,
  ArrowUpDown,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  WORKSPACE,
  WORKSPACE_PICK_ORDER,
  WORKSPACE_COLOR,
  V1_ENABLED_WORKSPACES,
  getWorkspaceLabel,
} from '../../constants/workspaceTypes';

const WORKSPACE_ICON = {
  [WORKSPACE.CHAT]: MessageCircle,
  [WORKSPACE.IMAGE]: ImageIcon,
  [WORKSPACE.VIDEO]: VideoIcon,
  [WORKSPACE.AUDIO]: Mic,
  [WORKSPACE.EMBEDDING]: Binary,
  [WORKSPACE.RERANK]: ArrowUpDown,
};

const SessionList = ({
  sessions = [],
  activeId,
  onSwitch,
  onCreate,
  onRename,
  onDelete,
  defaultOpen = true,
}) => {
  const { t } = useTranslation();
  const [open, setOpen] = useState(defaultOpen);
  const [renamingId, setRenamingId] = useState(null);
  const [renameDraft, setRenameDraft] = useState('');
  // 新建会话下拉受控：手动控制 visible 状态，点 Item 后主动收起——
  // Semi 的 Dropdown.Item 默认不会因为 Item 的 onClick 自动关闭 menu。
  const [createMenuVisible, setCreateMenuVisible] = useState(false);
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
    <div className='mb-4'>
      {/* 区块头：只有左侧标题区和右侧 chevron 可以切换折叠状态；
          "+" 按钮和它的下拉菜单保持独立点击，不会触发收起。 */}
      <div className='flex items-center gap-2 px-1 py-1'>
        <div
          className='flex items-center gap-1.5 flex-1 cursor-pointer select-none'
          onClick={() => setOpen(!open)}
        >
          <MessageSquare size={14} className='text-gray-500' />
          <Typography.Text strong className='text-sm'>
            {t('会话')}
          </Typography.Text>
          <Typography.Text type='tertiary' className='text-xs'>
            ({sessions.length})
          </Typography.Text>
        </div>
        <Dropdown
          trigger='click'
          position='bottomRight'
          visible={createMenuVisible}
          onVisibleChange={setCreateMenuVisible}
          render={
            <Dropdown.Menu>
              {WORKSPACE_PICK_ORDER.map((ws) => {
                const Icon = WORKSPACE_ICON[ws];
                const enabled = V1_ENABLED_WORKSPACES.has(ws);
                return (
                  <Dropdown.Item
                    key={ws}
                    disabled={!enabled}
                    onClick={() => {
                      if (!enabled || !onCreate) return;
                      // 选中后手动关闭下拉，否则菜单会停留在打开状态。
                      // 注意：这里不再 e.stopPropagation()——之前的阻止冒泡
                      // 反而干扰了 Semi 内部的关闭逻辑；Dropdown 渲染到 portal
                      // 里，事件不会冒泡到 SessionList 的折叠 header。
                      setCreateMenuVisible(false);
                      onCreate(ws);
                    }}
                  >
                    <span className='flex items-center gap-2'>
                      {Icon ? <Icon size={14} /> : null}
                      <span>{getWorkspaceLabel(t, ws)}</span>
                      {!enabled && (
                        <Typography.Text
                          type='tertiary'
                          className='ml-1 text-xs'
                        >
                          ({t('敬请期待')})
                        </Typography.Text>
                      )}
                    </span>
                  </Dropdown.Item>
                );
              })}
            </Dropdown.Menu>
          }
        >
          <Button
            size='small'
            type='primary'
            theme='borderless'
            icon={<Plus size={14} />}
            onClick={(e) => e.stopPropagation()}
            aria-label={t('新建会话')}
          />
        </Dropdown>
        <ChevronDown
          size={14}
          className='cursor-pointer'
          onClick={() => setOpen(!open)}
          style={{
            transform: open ? 'rotate(180deg)' : 'rotate(0deg)',
            transition: 'transform 200ms',
            color: 'var(--semi-color-text-2)',
          }}
        />
      </div>

      <Collapsible isOpen={open}>
        <div
          className='space-y-1 pt-2 overflow-y-auto'
          style={{ maxHeight: 260 }}
        >
          {sessions.length === 0 && (
            <Typography.Text
              type='tertiary'
              className='text-xs px-2 py-3 block text-center'
            >
              {t('暂无会话')}
            </Typography.Text>
          )}
          {sessions.map((s) => {
            const isActive = s.id === activeId;
            const isRenaming = renamingId === s.id;
            const workspace = s.workspace_type || WORKSPACE.CHAT;
            return (
              <div
                key={s.id}
                className={`group rounded-lg px-2 py-2 cursor-pointer transition-colors ${
                  isActive ? '' : 'hover:bg-gray-100'
                }`}
                style={{
                  backgroundColor: isActive
                    ? 'var(--semi-color-primary-light-default)'
                    : undefined,
                }}
                onClick={() => {
                  if (!isRenaming && onSwitch) onSwitch(s.id);
                }}
              >
                <div className='flex items-center gap-2 min-w-0'>
                  <Tag
                    size='small'
                    shape='circle'
                    color={WORKSPACE_COLOR[workspace] || 'grey'}
                  >
                    {getWorkspaceLabel(t, workspace)}
                  </Tag>
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
                    <div className='flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-0.5'>
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
              </div>
            );
          })}
        </div>
      </Collapsible>
    </div>
  );
};

export default SessionList;
