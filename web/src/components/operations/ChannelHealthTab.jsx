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

import React, { useMemo } from 'react';
import { Card, Empty, Spin, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { CARD_PROPS } from '../../constants/dashboard.constants';

const formatPct = (n) => `${(n * 100).toFixed(2)}%`;
const formatMs = (n) => {
  if (!n) return '-';
  if (n >= 1000) return `${(n / 1000).toFixed(2)}s`;
  return `${Math.round(n)}ms`;
};

const errorRateTag = (rate, t) => {
  let color = 'green';
  let label = t('健康');
  if (rate >= 0.5) {
    color = 'red';
    label = t('严重');
  } else if (rate >= 0.25) {
    color = 'orange';
    label = t('降级');
  } else if (rate >= 0.1) {
    color = 'amber';
    label = t('波动');
  } else if (rate >= 0.02) {
    color = 'lime';
    label = t('轻微');
  }
  return <Tag color={color}>{label}</Tag>;
};

const ChannelHealthTab = ({ rows, loading, t }) => {
  const enriched = useMemo(
    () =>
      (rows || []).map((r) => {
        const total = Number(r.total || 0);
        const errors = Number(r.errors || 0);
        const errorRate = total > 0 ? errors / total : 0;
        return {
          ...r,
          errorRate,
          successRate: 1 - errorRate,
        };
      }),
    [rows],
  );

  const columns = [
    {
      title: t('渠道'),
      dataIndex: 'channel_id',
      width: 220,
      render: (id, row) => (
        <span>
          <Typography.Text strong>
            {row.channel_name || `#${id}`}
          </Typography.Text>
          {row.channel_name && (
            <Typography.Text type='tertiary' style={{ marginLeft: 6, fontSize: 12 }}>
              #{id}
            </Typography.Text>
          )}
        </span>
      ),
    },
    {
      title: t('调用总数'),
      dataIndex: 'total',
      width: 100,
      sorter: (a, b) => a.total - b.total,
    },
    {
      title: t('失败数'),
      dataIndex: 'errors',
      width: 100,
      sorter: (a, b) => a.errors - b.errors,
      render: (v) => (v > 0 ? <span style={{ color: 'var(--semi-color-danger)' }}>{v}</span> : v),
    },
    {
      title: t('成功率'),
      dataIndex: 'successRate',
      width: 110,
      sorter: (a, b) => a.successRate - b.successRate,
      render: (v) => formatPct(v),
    },
    {
      title: t('失败率'),
      dataIndex: 'errorRate',
      width: 110,
      sorter: (a, b) => a.errorRate - b.errorRate,
      render: (v) => formatPct(v),
    },
    {
      title: t('平均耗时'),
      dataIndex: 'avg_use_time',
      width: 120,
      sorter: (a, b) => a.avg_use_time - b.avg_use_time,
      render: formatMs,
    },
    {
      title: t('健康度'),
      dataIndex: 'health',
      width: 90,
      render: (_, row) => errorRateTag(row.errorRate, t),
    },
  ];

  return (
    <Card
      {...CARD_PROPS}
      className='!rounded-2xl'
      title={
        <div>
          <Typography.Text strong>{t('渠道健康度')}</Typography.Text>
          <Typography.Text
            type='tertiary'
            style={{ marginLeft: 8, fontSize: 12 }}
          >
            {t('按调用量排序，仅展示当前区间内有调用的渠道')}
          </Typography.Text>
        </div>
      }
    >
      <Spin spinning={loading}>
        {enriched.length === 0 ? (
          <Empty description={t('区间内暂无渠道调用记录')} />
        ) : (
          <Table
            columns={columns}
            dataSource={enriched}
            pagination={false}
            size='middle'
            rowKey='channel_id'
          />
        )}
      </Spin>
    </Card>
  );
};

export default ChannelHealthTab;
