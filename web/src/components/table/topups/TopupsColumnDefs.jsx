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
import { Typography, Tag, Space, Button } from '@douyinfe/semi-ui';
import { Coins } from 'lucide-react';
import { timestamp2string } from '../../../helpers';

const { Text } = Typography;

const STATUS_CONFIG = {
  success: { color: 'green', label: '成功' },
  pending: { color: 'orange', label: '待支付' },
  failed: { color: 'red', label: '失败' },
  expired: { color: 'grey', label: '过期' },
};

const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  alipay: '支付宝',
  wxpay: '微信',
};

const renderUserCell = (record, t) => {
  const username = record.username || '-';
  const email = record.email;
  return (
    <div className='flex flex-col'>
      <Text className='leading-tight'>{username}</Text>
      <Text
        type='secondary'
        size='small'
        className='leading-tight'
        style={{ fontSize: 12 }}
      >
        {email || t('未绑定邮箱')}
      </Text>
    </div>
  );
};

const renderStatusTag = (status, t) => {
  const cfg = STATUS_CONFIG[status] || { color: 'grey', label: status || '-' };
  return (
    <Tag color={cfg.color} shape='circle'>
      {t(cfg.label)}
    </Tag>
  );
};

const renderPaymentMethod = (pm, t) => {
  const display = PAYMENT_METHOD_MAP[pm];
  return <Text>{display ? t(display) : pm || '-'}</Text>;
};

const isSubscriptionTopup = (record) => {
  const tradeNo = (record?.trade_no || '').toLowerCase();
  return Number(record?.amount || 0) === 0 && tradeNo.startsWith('sub');
};

export const getTopupsColumns = ({ t, currencySymbol = '$', onComplete }) => {
  return [
    {
      title: t('订单号'),
      dataIndex: 'trade_no',
      render: (text) => <Text copyable>{text}</Text>,
    },
    {
      title: t('用户'),
      dataIndex: 'user',
      render: (_, record) => renderUserCell(record, t),
    },
    {
      title: t('支付方式'),
      dataIndex: 'payment_method',
      render: (text) => renderPaymentMethod(text, t),
    },
    {
      title: t('充值额度'),
      dataIndex: 'amount',
      render: (amount, record) => {
        if (isSubscriptionTopup(record)) {
          return (
            <Tag color='purple' shape='circle' size='small'>
              {t('订阅套餐')}
            </Tag>
          );
        }
        return (
          <Space spacing={4}>
            <Coins size={14} />
            <Text>{amount}</Text>
          </Space>
        );
      },
    },
    {
      title: t('支付金额'),
      dataIndex: 'money',
      render: (money, record) => {
        const symbol =
          record.payment_method === 'stripe' ? currencySymbol : '¥';
        const value = Number(money) || 0;
        return (
          <Text type='danger'>
            {symbol}
            {value.toFixed(2)}
          </Text>
        );
      },
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (status) => renderStatusTag(status, t),
    },
    {
      title: t('充值信息'),
      dataIndex: 'topup_seq',
      render: (_, record) => {
        if (record.status !== 'success') {
          return <Text type='tertiary'>-</Text>;
        }
        const seq = Number(record.topup_seq) || 0;
        const cumulative = Number(record.topup_cumulative) || 0;
        const symbol =
          record.payment_method === 'stripe' ? currencySymbol : '¥';
        const isFirst = seq === 1;
        return (
          <div className='flex flex-col'>
            {isFirst ? (
              <Tag color='amber' shape='circle' size='small'>
                {t('首充')}
              </Tag>
            ) : (
              <Tag color='blue' shape='circle' size='small'>
                {t('第 {{seq}} 次', { seq })}
              </Tag>
            )}
            <Text
              type='secondary'
              size='small'
              className='leading-tight mt-1'
              style={{ fontSize: 12 }}
            >
              {t('累计')}: {symbol}
              {cumulative.toFixed(2)}
            </Text>
          </div>
        );
      },
    },
    {
      title: t('创建时间'),
      dataIndex: 'create_time',
      render: (time) => (
        <Text>{time ? timestamp2string(time) : '-'}</Text>
      ),
    },
    {
      title: t('完成时间'),
      dataIndex: 'complete_time',
      render: (time) => (
        <Text>{time ? timestamp2string(time) : '-'}</Text>
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      fixed: 'right',
      width: 120,
      render: (_, record) => {
        if (record.status === 'pending') {
          return (
            <Button
              type='primary'
              size='small'
              theme='outline'
              onClick={() => onComplete && onComplete(record)}
            >
              {t('补单')}
            </Button>
          );
        }
        return null;
      },
    },
  ];
};
