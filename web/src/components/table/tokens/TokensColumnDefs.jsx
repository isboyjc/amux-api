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
  Button,
  Dropdown,
  Space,
  Tag,
  AvatarGroup,
  Avatar,
  Tooltip,
  Progress,
  Popover,
  Typography,
  Input,
  Modal,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  getModelCategories,
  stringToColor,
} from '../../../helpers';
import {
  IconTreeTriangleDown,
  IconCopy,
  IconEyeOpened,
  IconEyeClosed,
} from '@douyinfe/semi-icons';
import GroupChainPicker from './GroupChainPicker';

// progress color helper
const getProgressColor = (pct) => {
  if (pct === 100) return 'var(--semi-color-success)';
  if (pct <= 10) return 'var(--semi-color-danger)';
  if (pct <= 30) return 'var(--semi-color-warning)';
  return undefined;
};

// Render functions
function renderTimestamp(timestamp) {
  return <>{timestamp2string(timestamp)}</>;
}

// Render status column only (no usage)
const renderStatus = (text, record, t) => {
  const enabled = text === 1;

  let tagColor = 'black';
  let tagText = t('未知状态');
  if (enabled) {
    tagColor = 'green';
    tagText = t('已启用');
  } else if (text === 2) {
    tagColor = 'red';
    tagText = t('已禁用');
  } else if (text === 3) {
    tagColor = 'yellow';
    tagText = t('已过期');
  } else if (text === 4) {
    tagColor = 'grey';
    tagText = t('已耗尽');
  }

  return (
    <Tag color={tagColor} shape='circle' size='small'>
      {tagText}
    </Tag>
  );
};

// Render group column - merged into a single tag (group + ratio),
// with a clickable Dropdown to switch the token's group inline.
const renderGroupColumn = (
  text,
  record,
  t,
  groupRatios = {},
  groupOptions = [],
  updateTokenGroup,
  groupModelsCache = {},
  fetchGroupModels,
) => {
  const tagColors = {
    vip: 'yellow',
    pro: 'yellow',
    svip: 'red',
    premium: 'red',
  };

  const canChange =
    typeof updateTokenGroup === 'function' && (groupOptions || []).length > 0;

  // 令牌的分组链。为空时回退到 group 字段的旧语义：单分组 → 单元素链，
  // 空字符串 → 空链，auto → 保留旧版展示。
  const chain = Array.isArray(record?.groups) ? record.groups : [];

  // 多分组令牌：把整条链按顺序展示出来，序号即优先级。
  // 单选下拉表达不了「有序的多个分组」，所以点开的是勾选 + 排序面板。
  if (chain.length > 1) {
    const tagContent = (
      <Tooltip
        content={
          t('该令牌按顺序使用以下分组：') +
          chain.join(' → ') +
          (record?.cross_group_retry
            ? t('，失败时自动切换到下一个分组')
            : t('，未开启跨分组重试'))
        }
        position='top'
      >
        <span
          className='flex items-center gap-1 flex-wrap'
          style={canChange ? { cursor: 'pointer' } : undefined}
        >
          {chain.map((groupName, index) => (
            <React.Fragment key={groupName}>
              {index > 0 && <span className='opacity-50 text-xs'>→</span>}
              <Tag
                color={
                  index === 0
                    ? tagColors[groupName] || stringToColor(groupName)
                    : 'white'
                }
                shape='circle'
                size='small'
              >
                <span className='flex items-center gap-1'>
                  <span className='opacity-60'>{index + 1}</span>
                  {groupName}
                </span>
              </Tag>
            </React.Fragment>
          ))}
          {canChange && <IconTreeTriangleDown style={{ fontSize: 10 }} />}
        </span>
      </Tooltip>
    );
    return (
      <GroupChainPicker
        value={chain}
        crossGroupRetry={record?.cross_group_retry}
        groupOptions={groupOptions}
        groupModelsCache={groupModelsCache}
        fetchGroupModels={fetchGroupModels}
        disabled={!canChange}
        minSelected={1}
        onSave={(groups, crossGroupRetry) =>
          updateTokenGroup(record, groups, crossGroupRetry)
        }
      >
        {tagContent}
      </GroupChainPicker>
    );
  }

  // Render the merged tag content
  let tagContent;
  if (text === 'auto') {
    tagContent = (
      <Tooltip
        content={t(
          '当前分组为 auto，会自动选择最优分组，当一个组不可用时自动降级到下一个组（熔断机制）',
        )}
        position='top'
      >
        <Tag
          color='white'
          shape='circle'
          style={canChange ? { cursor: 'pointer' } : undefined}
        >
          <span className='flex items-center gap-1'>
            {t('智能熔断')}
            {record && record.cross_group_retry ? `(${t('跨分组')})` : ''}
            {canChange && <IconTreeTriangleDown style={{ fontSize: 10 }} />}
          </span>
        </Tag>
      </Tooltip>
    );
  } else {
    const ratio = groupRatios[text];
    const displayName = text === '' ? t('用户分组') : text;
    const color =
      text === '' ? 'white' : tagColors[text] || stringToColor(text);
    tagContent = (
      <Tag
        color={color}
        shape='circle'
        style={canChange ? { cursor: 'pointer' } : undefined}
      >
        <span className='flex items-center gap-1'>
          <span>{displayName}</span>
          {ratio !== undefined && (
            <span className='opacity-80'>· {ratio}x</span>
          )}
          {canChange && <IconTreeTriangleDown style={{ fontSize: 10 }} />}
        </span>
      </Tag>
    );
  }

  if (!canChange) {
    // Fallback: keep original non-interactive rendering using shared helper
    if (text === 'auto') return tagContent;
    return <span className='flex items-center gap-1'>{renderGroup(text)}</span>;
  }

  // 单分组 / 旧版 auto 令牌也走同一个选择器：用户随时可以把它改成多分组。
  // auto 令牌的当前链是运行时展开的，这里传空数组，让用户从零显式选择 ——
  // 与编辑弹窗一样，绝不静默把 auto 转成某条固定链。
  return (
    <GroupChainPicker
      value={text === 'auto' || !text ? [] : [text]}
      crossGroupRetry={record?.cross_group_retry}
      groupOptions={groupOptions}
      groupModelsCache={groupModelsCache}
      fetchGroupModels={fetchGroupModels}
      minSelected={1}
      onSave={(groups, crossGroupRetry) =>
        updateTokenGroup(record, groups, crossGroupRetry)
      }
    >
      {tagContent}
    </GroupChainPicker>
  );
};

// Render token key column with show/hide and copy functionality
const renderTokenKey = (
  text,
  record,
  showKeys,
  resolvedTokenKeys,
  loadingTokenKeys,
  toggleTokenVisibility,
  copyTokenKey,
  copyTokenConnectionString,
  t,
) => {
  const revealed = !!showKeys[record.id];
  const loading = !!loadingTokenKeys[record.id];
  const keyValue =
    revealed && resolvedTokenKeys[record.id]
      ? resolvedTokenKeys[record.id]
      : record.key || '';
  const displayedKey = keyValue ? `sk-${keyValue}` : '';

  return (
    <div className='w-[200px]'>
      <Input
        readOnly
        value={displayedKey}
        size='small'
        suffix={
          <div className='flex items-center'>
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={revealed ? <IconEyeClosed /> : <IconEyeOpened />}
              loading={loading}
              aria-label='toggle token visibility'
              onClick={async (e) => {
                e.stopPropagation();
                await toggleTokenVisibility(record);
              }}
            />
            <Dropdown
              trigger='click'
              position='bottomRight'
              clickToHide
              menu={[
                {
                  node: 'item',
                  name: t('复制密钥'),
                  onClick: () => copyTokenKey(record),
                },
                {
                  node: 'item',
                  name: t('复制连接信息'),
                  onClick: () => copyTokenConnectionString(record),
                },
              ]}
            >
              <Button
                theme='borderless'
                size='small'
                type='tertiary'
                icon={<IconCopy />}
                loading={loading}
                aria-label='copy token key'
                onClick={async (e) => {
                  e.stopPropagation();
                }}
              />
            </Dropdown>
          </div>
        }
      />
    </div>
  );
};

// Render model limits column
const renderModelLimits = (text, record, t) => {
  if (record.model_limits_enabled && text) {
    const models = text.split(',').filter(Boolean);
    const categories = getModelCategories(t);

    const vendorAvatars = [];
    const matchedModels = new Set();
    Object.entries(categories).forEach(([key, category]) => {
      if (key === 'all') return;
      if (!category.icon || !category.filter) return;
      const vendorModels = models.filter((m) =>
        category.filter({ model_name: m }),
      );
      if (vendorModels.length > 0) {
        vendorAvatars.push(
          <Tooltip
            key={key}
            content={vendorModels.join(', ')}
            position='top'
            showArrow
          >
            <Avatar
              size='extra-extra-small'
              alt={category.label}
              color='transparent'
            >
              {category.icon}
            </Avatar>
          </Tooltip>,
        );
        vendorModels.forEach((m) => matchedModels.add(m));
      }
    });

    const unmatchedModels = models.filter((m) => !matchedModels.has(m));
    if (unmatchedModels.length > 0) {
      vendorAvatars.push(
        <Tooltip
          key='unknown'
          content={unmatchedModels.join(', ')}
          position='top'
          showArrow
        >
          <Avatar size='extra-extra-small' alt='unknown'>
            {t('其他')}
          </Avatar>
        </Tooltip>,
      );
    }

    return <AvatarGroup size='extra-extra-small'>{vendorAvatars}</AvatarGroup>;
  } else {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }
};

// Render IP restrictions column
const renderMaxConcurrency = (text, t) => {
  const value = Number(text);
  if (!Number.isFinite(value) || value <= 0) {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }
  return (
    <Tag color='blue' shape='circle'>
      {value}
    </Tag>
  );
};

const renderAllowIps = (text, t) => {
  if (!text || text.trim() === '') {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }

  const ips = text
    .split('\n')
    .map((ip) => ip.trim())
    .filter(Boolean);

  const displayIps = ips.slice(0, 1);
  const extraCount = ips.length - displayIps.length;

  const ipTags = displayIps.map((ip, idx) => (
    <Tag key={idx} shape='circle'>
      {ip}
    </Tag>
  ));

  if (extraCount > 0) {
    ipTags.push(
      <Tooltip
        key='extra'
        content={ips.slice(1).join(', ')}
        position='top'
        showArrow
      >
        <Tag shape='circle'>{'+' + extraCount}</Tag>
      </Tooltip>,
    );
  }

  return <Space wrap>{ipTags}</Space>;
};

// Render separate quota usage column
const renderQuotaUsage = (text, record, t) => {
  const { Paragraph } = Typography;
  const used = parseInt(record.used_quota) || 0;
  const remain = parseInt(record.remain_quota) || 0;
  const total = used + remain;
  if (record.unlimited_quota) {
    // 无限额度下没有「剩余/总额」可言，但消耗依然有意义 —— 之前这里只渲染一个
    // 「无限额度」标签，已用额度藏在 Popover 里，列表上完全看不到消耗。
    // 注意必须单行横排：Tag 高度固定，在里面堆两行会溢出遮挡相邻行。
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(used) }}>
          {t('已用额度')}: {renderQuota(used)}
        </Paragraph>
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        <Tag color='white' shape='circle'>
          <span className='text-xs whitespace-nowrap'>
            {t('无限额度')}
            <span className='mx-1 text-[var(--semi-color-text-3)]'>·</span>
            <span className='text-[var(--semi-color-text-2)]'>
              {t('已用')} {renderQuota(used)}
            </span>
          </span>
        </Tag>
      </Popover>
    );
  }
  const percent = total > 0 ? (remain / total) * 100 : 0;
  const popoverContent = (
    <div className='text-xs p-2'>
      <Paragraph copyable={{ content: renderQuota(used) }}>
        {t('已用额度')}: {renderQuota(used)}
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(remain) }}>
        {t('剩余额度')}: {renderQuota(remain)} ({percent.toFixed(0)}%)
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(total) }}>
        {t('总额度')}: {renderQuota(total)}
      </Paragraph>
    </div>
  );
  return (
    <Popover content={popoverContent} position='top'>
      <Tag color='white' shape='circle'>
        <div className='flex flex-col items-end'>
          <span className='text-xs leading-none'>{`${renderQuota(remain)} / ${renderQuota(total)}`}</span>
          <Progress
            percent={percent}
            stroke={getProgressColor(percent)}
            aria-label='quota usage'
            format={() => `${percent.toFixed(0)}%`}
            style={{ width: '100%', marginTop: '1px', marginBottom: 0 }}
          />
        </div>
      </Tag>
    </Popover>
  );
};

// Render operations column
const renderOperations = (
  text,
  record,
  openTestModal,
  openCCSwitchModal,
  setEditingToken,
  setShowEdit,
  manageToken,
  refresh,
  t,
) => {
  // 原来的「聊天 + 聊天下拉」整体让位给「测试」：聊天链接依赖管理员在设置里
  // 配置，没配就只能报错，而用户创建完令牌真正需要的是确认这个 key 能不能用。
  // CC Switch 单独留一个直达按钮（往桌面客户端投递密钥，和聊天链接配置无关）。
  return (
    <Space wrap>
      <Button
        size='small'
        type='tertiary'
        onClick={() => openTestModal(record)}
      >
        {t('测试')}
      </Button>

      <Button
        size='small'
        type='tertiary'
        onClick={() => openCCSwitchModal(record)}
      >
        {/* CC Switch 是产品名，不进翻译词表 */}
        {'CC Switch'}
      </Button>

      {record.status === 1 ? (
        <Button
          type='danger'
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'disable', record);
            await refresh();
          }}
        >
          {t('禁用')}
        </Button>
      ) : (
        <Button
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'enable', record);
            await refresh();
          }}
        >
          {t('启用')}
        </Button>
      )}

      <Button
        type='tertiary'
        size='small'
        onClick={() => {
          setEditingToken(record);
          setShowEdit(true);
        }}
      >
        {t('编辑')}
      </Button>

      <Button
        type='danger'
        size='small'
        onClick={() => {
          Modal.confirm({
            title: t('确定是否要删除此令牌？'),
            content: t('此修改将不可逆'),
            onOk: () => {
              (async () => {
                await manageToken(record.id, 'delete', record);
                await refresh();
              })();
            },
          });
        }}
      >
        {t('删除')}
      </Button>
    </Space>
  );
};

export const getTokensColumns = ({
  t,
  showKeys,
  resolvedTokenKeys,
  loadingTokenKeys,
  toggleTokenVisibility,
  copyTokenKey,
  copyTokenConnectionString,
  manageToken,
  openTestModal,
  openCCSwitchModal,
  setEditingToken,
  setShowEdit,
  refresh,
  groupRatios = {},
  groupOptions = [],
  updateTokenGroup,
  groupModelsCache = {},
  fetchGroupModels,
}) => {
  return [
    {
      title: t('名称'),
      dataIndex: 'name',
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: (text, record) => renderStatus(text, record, t),
    },
    {
      title: t('剩余额度/总额度'),
      key: 'quota_usage',
      render: (text, record) => renderQuotaUsage(text, record, t),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      key: 'group',
      render: (text, record) =>
        renderGroupColumn(
          text,
          record,
          t,
          groupRatios,
          groupOptions,
          updateTokenGroup,
          groupModelsCache,
          fetchGroupModels,
        ),
    },
    {
      title: t('密钥'),
      key: 'token_key',
      render: (text, record) =>
        renderTokenKey(
          text,
          record,
          showKeys,
          resolvedTokenKeys,
          loadingTokenKeys,
          toggleTokenVisibility,
          copyTokenKey,
          copyTokenConnectionString,
          t,
        ),
    },
    {
      title: t('可用模型'),
      dataIndex: 'model_limits',
      render: (text, record) => renderModelLimits(text, record, t),
    },
    {
      title: t('IP限制'),
      dataIndex: 'allow_ips',
      render: (text) => renderAllowIps(text, t),
    },
    {
      title: t('最大并发数'),
      dataIndex: 'max_concurrency',
      render: (text) => renderMaxConcurrency(text, t),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      render: (text, record, index) => {
        return <div>{renderTimestamp(text)}</div>;
      },
    },
    {
      title: t('最后使用时间'),
      dataIndex: 'accessed_time',
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      title: t('过期时间'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        return (
          <div>
            {record.expired_time === -1 ? t('永不过期') : renderTimestamp(text)}
          </div>
        );
      },
    },
    {
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) =>
        renderOperations(
          text,
          record,
          openTestModal,
          openCCSwitchModal,
          setEditingToken,
          setShowEdit,
          manageToken,
          refresh,
          t,
        ),
    },
  ];
};
