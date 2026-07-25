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
import { Table, Typography, Empty } from '@douyinfe/semi-ui';
import { renderQuota, timestamp2string } from '../../../helpers';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { renderWinnerStatus } from './statusTag';

const { Text } = Typography;

const BlindBoxWinnersTable = ({ winners, loading, t }) => {
  const isMobile = useIsMobile();
  const columns = [
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (v, r) => (
        <div>
          <div className='font-medium'>{v || '-'}</div>
          <Text type='tertiary' className='text-xs'>
            ID: {r.user_id}
          </Text>
        </div>
      ),
    },
    { title: t('开奖日期'), dataIndex: 'draw_date' },
    {
      title: t('当期消耗'),
      dataIndex: 'consume_quota',
      render: (v) => renderQuota(v),
    },
    {
      title: t('中奖概率'),
      dataIndex: 'win_probability',
      render: (v) => `${((v || 0) * 100).toFixed(2)}%`,
    },
    { title: t('奖项'), dataIndex: 'prize_name', render: (v) => v || '-' },
    {
      title: t('奖励额度'),
      dataIndex: 'prize_quota',
      render: (v) => (
        <Text strong className='text-amber-600 dark:text-amber-400'>
          {renderQuota(v)}
        </Text>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v) => renderWinnerStatus(v, t),
    },
    {
      title: t('领取时间'),
      dataIndex: 'claimed_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    {
      title: t('过期时间'),
      dataIndex: 'expire_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={winners}
      loading={loading}
      pagination={false}
      rowKey='id'
      className='w-full'
      /* 桌面端不设 scroll.x：设成 max-content 后表格宽度只到内容宽度，
         容器右侧会留白撑不满。窄屏才需要横向滚动。 */
      scroll={isMobile ? { x: 'max-content' } : undefined}
      empty={<Empty description={t('暂无中奖记录')} />}
    />
  );
};

export default BlindBoxWinnersTable;
