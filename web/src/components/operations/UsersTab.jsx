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
import { Card, Empty, Spin, Table, Typography } from '@douyinfe/semi-ui';
import { CARD_PROPS } from '../../constants/dashboard.constants';
import { renderQuota } from '../../helpers/render';

const formatTime = (ts) => {
  if (!ts) return '-';
  const d = new Date(ts * 1000);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
};

const UsersTab = ({ topConsumers, recentTopups, loading, t }) => {
  const consumerCols = [
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (v, row) => (
        <span>
          <Typography.Text strong>{v}</Typography.Text>
          <Typography.Text
            type='tertiary'
            style={{ marginLeft: 6, fontSize: 12 }}
          >
            #{row.user_id}
          </Typography.Text>
        </span>
      ),
    },
    {
      title: t('消耗（金额口径）'),
      dataIndex: 'quota',
      sorter: (a, b) => a.quota - b.quota,
      render: (v) => renderQuota(v || 0, 2),
    },
    {
      title: t('调用次数'),
      dataIndex: 'count',
      sorter: (a, b) => a.count - b.count,
    },
    {
      title: t('Tokens'),
      dataIndex: 'token_used',
      sorter: (a, b) => a.token_used - b.token_used,
      render: (v) => (v ? Number(v).toLocaleString() : 0),
    },
  ];

  const topupCols = [
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (v, row) => (
        <span>
          <Typography.Text strong>{v || '-'}</Typography.Text>
          <Typography.Text
            type='tertiary'
            style={{ marginLeft: 6, fontSize: 12 }}
          >
            #{row.user_id}
          </Typography.Text>
        </span>
      ),
    },
    {
      title: t('金额'),
      dataIndex: 'money',
      sorter: (a, b) => a.money - b.money,
      render: (v) => Number(v || 0).toFixed(2),
    },
    {
      title: t('额度'),
      dataIndex: 'amount',
      render: (v) => (v ? renderQuota(v * 1, 2) : '-'),
    },
    {
      title: t('支付方式'),
      dataIndex: 'payment_method',
    },
    {
      title: t('完成时间'),
      dataIndex: 'complete_time',
      render: formatTime,
    },
  ];

  return (
    <div className='grid grid-cols-1 lg:grid-cols-2 gap-4'>
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={
          <div>
            <Typography.Text strong>{t('Top 消耗用户')}</Typography.Text>
            <Typography.Text
              type='tertiary'
              style={{ marginLeft: 8, fontSize: 12 }}
            >
              {t('按区间内消耗金额排序')}
            </Typography.Text>
          </div>
        }
      >
        <Spin spinning={loading}>
          {(topConsumers || []).length === 0 ? (
            <Empty description={t('区间内暂无消耗')} />
          ) : (
            <Table
              columns={consumerCols}
              dataSource={topConsumers}
              pagination={false}
              size='small'
              rowKey='user_id'
            />
          )}
        </Spin>
      </Card>

      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={
          <div>
            <Typography.Text strong>{t('最近成功充值')}</Typography.Text>
            <Typography.Text
              type='tertiary'
              style={{ marginLeft: 8, fontSize: 12 }}
            >
              {t('用于关注大额或异常充值，独立于时间筛选')}
            </Typography.Text>
          </div>
        }
      >
        <Spin spinning={loading}>
          {(recentTopups || []).length === 0 ? (
            <Empty description={t('暂无充值记录')} />
          ) : (
            <Table
              columns={topupCols}
              dataSource={recentTopups}
              pagination={false}
              size='small'
              rowKey='id'
            />
          )}
        </Spin>
      </Card>
    </div>
  );
};

export default UsersTab;
