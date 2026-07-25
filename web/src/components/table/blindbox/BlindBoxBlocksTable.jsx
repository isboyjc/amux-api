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
  Table,
  Typography,
  Tag,
  Button,
  Popconfirm,
  Empty,
} from '@douyinfe/semi-ui';
import { timestamp2string } from '../../../helpers';
import { useIsMobile } from '../../../hooks/common/useIsMobile';

const { Text } = Typography;

export const BLOCK_SCOPE_OPTIONS = (t) => [
  { label: t('全部范围'), value: '' },
  { label: t('永久屏蔽'), value: 'permanent' },
  { label: t('本期屏蔽'), value: 'period' },
];

/**
 * 屏蔽名单。
 *
 * 当期明细里只看得到"本期报名过"的人，永久屏蔽的用户一旦某期没参与就会从那张表消失、
 * 也就无从解除，所以屏蔽必须有一个独立的集中管理入口。
 */
const BlindBoxBlocksTable = ({ blocks, loading, onUnblock, t }) => {
  const isMobile = useIsMobile();

  const columns = [
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (v, r) => (
        <span>
          {v || '-'}{' '}
          <Text type='tertiary' className='text-xs'>
            #{r.user_id}
          </Text>
        </span>
      ),
    },
    {
      title: t('屏蔽范围'),
      dataIndex: 'scope',
      render: (v, r) =>
        v === 'permanent' ? (
          <Tag color='red' shape='circle'>
            {t('永久屏蔽')}
          </Tag>
        ) : (
          <Tag color='orange' shape='circle'>
            {t('本期屏蔽')} · {r.draw_date}
          </Tag>
        ),
    },
    {
      title: t('原因'),
      dataIndex: 'reason',
      render: (v) => v || <Text type='tertiary'>-</Text>,
    },
    {
      title: t('操作人'),
      dataIndex: 'operator_name',
      render: (v, r) =>
        v ? (
          <span>
            {v}{' '}
            <Text type='tertiary' className='text-xs'>
              #{r.operator_id}
            </Text>
          </span>
        ) : (
          <Text type='tertiary'>-</Text>
        ),
    },
    {
      title: t('屏蔽时间'),
      dataIndex: 'created_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      render: (_, r) => (
        <Popconfirm
          title={t('解除屏蔽')}
          content={t('解除后该用户恢复中奖资格')}
          onConfirm={() => onUnblock(r.id)}
        >
          <Button size='small' theme='light' type='tertiary'>
            {t('解除屏蔽')}
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={blocks}
      loading={loading}
      pagination={false}
      rowKey='id'
      className='w-full'
      scroll={isMobile ? { x: 'max-content' } : undefined}
      empty={<Empty description={t('暂无屏蔽记录')} />}
    />
  );
};

export default BlindBoxBlocksTable;
