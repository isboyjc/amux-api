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
  Empty,
  Spin,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { AlertTriangle, ShieldCheck } from 'lucide-react';
import { CARD_PROPS } from '../../constants/dashboard.constants';
import { renderQuota } from '../../helpers/render';

const RISK_REASON_LABELS = {
  high_volume: '区间内邀请数偏高',
  disposable_email: '存在一次性邮箱',
};

const RiskTag = ({ row, t }) => {
  if (!row.risk) {
    return (
      <Tag color='green' prefixIcon={<ShieldCheck size={12} />}>
        {t('正常')}
      </Tag>
    );
  }
  const reasons = (row.risk_reasons || [])
    .map((r) => t(RISK_REASON_LABELS[r] || r))
    .join('、');
  return (
    <Tooltip content={reasons || t('命中风险规则')}>
      <Tag color='red' prefixIcon={<AlertTriangle size={12} />}>
        {t('风险')}
      </Tag>
    </Tooltip>
  );
};

const AffiliateTab = ({ data, loading, t }) => {
  const tops = data?.top_inviters || [];
  const domains = data?.email_domains || [];

  const columns = [
    {
      title: t('邀请人'),
      dataIndex: 'username',
      width: 200,
      render: (v, row) => (
        <span>
          <Typography.Text strong>{v || '-'}</Typography.Text>
          <Typography.Text
            type='tertiary'
            style={{ marginLeft: 6, fontSize: 12 }}
          >
            #{row.inviter_id}
          </Typography.Text>
          {row.email && (
            <div
              style={{
                fontSize: 11,
                color: 'var(--semi-color-text-2)',
                marginTop: 2,
              }}
            >
              {row.email}
            </div>
          )}
        </span>
      ),
    },
    {
      title: t('区间邀请数'),
      dataIndex: 'invites_in_range',
      width: 110,
      sorter: (a, b) => a.invites_in_range - b.invites_in_range,
    },
    {
      title: t('一次性邮箱'),
      dataIndex: 'disposable_hits',
      width: 130,
      render: (v, row) => {
        if (!v) return '0';
        const ratio = (row.disposable_ratio * 100).toFixed(0);
        return (
          <span style={{ color: 'var(--semi-color-danger)' }}>
            {v} ({ratio}%)
          </span>
        );
      },
    },
    {
      title: t('累计邀请数'),
      dataIndex: 'lifetime_aff_count',
      width: 110,
    },
    {
      title: t('累计返利'),
      dataIndex: 'lifetime_aff_history',
      width: 130,
      render: (v) => renderQuota(v || 0, 2),
    },
    {
      title: t('待领取返利'),
      dataIndex: 'pending_aff_quota',
      width: 130,
      render: (v) => renderQuota(v || 0, 2),
    },
    {
      title: t('被邀邮箱样本'),
      dataIndex: 'sample_invitees',
      render: (v) => (
        <div
          style={{
            fontSize: 11,
            color: 'var(--semi-color-text-2)',
            maxWidth: 280,
            wordBreak: 'break-all',
          }}
        >
          {(v || []).join(', ')}
        </div>
      ),
    },
    {
      title: t('风险'),
      dataIndex: 'risk',
      width: 90,
      fixed: 'right',
      render: (_, row) => <RiskTag row={row} t={t} />,
    },
  ];

  return (
    <div className='flex flex-col gap-4'>
      {/* 概览数字 */}
      <div className='grid grid-cols-2 md:grid-cols-4 gap-3'>
        <SmallStat
          label={t('区间内被邀注册')}
          value={data?.new_invited_users ?? '—'}
          hint={t('区间内 inviter_id != 0 的新用户数')}
        />
        <SmallStat
          label={t('区间内活跃邀请人')}
          value={data?.active_inviters ?? '—'}
          hint={t('区间内有过新增邀请的邀请人去重数')}
        />
        <SmallStat
          label={t('被邀邮箱后缀数')}
          value={domains.length}
          hint={t('区间内被邀新用户的不同邮箱后缀数量')}
        />
        <SmallStat
          label={t('一次性邮箱后缀数')}
          value={domains.filter((d) => d.disposable).length}
          hint={t('命中一次性邮箱黑名单的后缀数量；越多越可疑')}
          danger={domains.filter((d) => d.disposable).length > 0}
        />
      </div>

      {/* Top 邀请人表 */}
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={
          <div>
            <Typography.Text strong>{t('Top 邀请人（含风险标记）')}</Typography.Text>
            <Typography.Text
              type='tertiary'
              style={{ marginLeft: 8, fontSize: 12 }}
            >
              {t(
                '风险规则：区间内邀请数 ≥ 5 或 一次性邮箱占比 ≥ 30%。命中即标红供管理员复核。',
              )}
            </Typography.Text>
          </div>
        }
      >
        <Spin spinning={loading}>
          {tops.length === 0 ? (
            <Empty description={t('区间内暂无邀请数据')} />
          ) : (
            <Table
              columns={columns}
              dataSource={tops}
              pagination={false}
              size='small'
              rowKey='inviter_id'
              scroll={{ x: 1100 }}
            />
          )}
        </Spin>
      </Card>

      {/* 邮箱后缀分布 */}
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={t('被邀邮箱后缀分布')}
      >
        <Spin spinning={loading}>
          {domains.length === 0 ? (
            <Empty description={t('暂无数据')} />
          ) : (
            <div className='flex flex-wrap gap-2'>
              {domains.map((d) => (
                <Tag
                  key={d.domain}
                  color={d.disposable ? 'red' : 'blue'}
                  size='large'
                  type={d.disposable ? 'solid' : 'light'}
                >
                  {d.domain} · {d.count}
                </Tag>
              ))}
            </div>
          )}
        </Spin>
      </Card>
    </div>
  );
};

const SmallStat = ({ label, value, hint, danger }) => (
  <Card {...CARD_PROPS} className='!rounded-2xl' bodyStyle={{ padding: 12 }}>
    <Typography.Text
      type='tertiary'
      style={{ fontSize: 12, display: 'block', marginBottom: 4 }}
    >
      {hint ? (
        <Tooltip content={hint}>
          <span>{label}</span>
        </Tooltip>
      ) : (
        label
      )}
    </Typography.Text>
    <Typography.Title
      heading={5}
      style={{
        margin: 0,
        color: danger ? 'var(--semi-color-danger)' : undefined,
      }}
    >
      {value}
    </Typography.Title>
  </Card>
);

export default AffiliateTab;
