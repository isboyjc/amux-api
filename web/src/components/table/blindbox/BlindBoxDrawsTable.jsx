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

import React, { useState, useEffect } from 'react';
import {
  Table,
  Typography,
  Progress,
  Empty,
  Spin,
  Tag,
} from '@douyinfe/semi-ui';
import {
  API,
  showError,
  renderQuota,
  timestamp2string,
} from '../../../helpers';
import { renderDrawStatus, renderWinnerStatus } from './statusTag';
import { useIsMobile } from '../../../hooks/common/useIsMobile';

const { Text } = Typography;

/**
 * 单期中奖名单。行展开时才拉取，避免列表页一次性打 N 个请求。
 * 一期最多 10 个奖项（MaxBlindBoxPrizes），一次取全，不做二级分页。
 */
const DrawWinners = ({ drawDate, t }) => {
  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get(
          `/api/blindbox/winners?draw_date=${encodeURIComponent(drawDate)}&p=1&page_size=100`,
        );
        const { success, message, data } = res.data;
        if (cancelled) return;
        if (success) {
          setRows(data.items || []);
        } else {
          showError(message || t('获取中奖名单失败'));
        }
      } catch (e) {
        if (!cancelled) showError(t('获取中奖名单失败'));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [drawDate, t]);

  if (loading) {
    return (
      <div className='py-4 flex justify-center'>
        <Spin />
      </div>
    );
  }
  if (!rows.length) {
    return (
      <Empty description={t('本期无中奖记录')} style={{ padding: '16px 0' }} />
    );
  }

  return (
    <Table
      size='small'
      className='w-full'
      pagination={false}
      dataSource={rows}
      rowKey='id'
      columns={[
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
      ]}
    />
  );
};

const BlindBoxDrawsTable = ({ draws, loading, t }) => {
  const isMobile = useIsMobile();
  const columns = [
    {
      title: t('开奖日期'),
      dataIndex: 'draw_date',
      render: (v, r) => (
        <div>
          <div className='font-medium'>{v}</div>
          <Text type='tertiary' className='text-xs'>
            {timestamp2string(r.period_end)}
          </Text>
        </div>
      ),
    },
    {
      title: t('周期区间'),
      dataIndex: 'period_start',
      render: (v, r) => (
        <Text type='tertiary' className='text-xs'>
          {timestamp2string(v)}
          <br />→ {timestamp2string(r.period_end)}
        </Text>
      ),
    },
    {
      title: t('参与人数'),
      dataIndex: 'entry_count',
    },
    {
      // 参与时已达标，但开奖按整期口径重算；差值即"参与后消耗被退款/重算"的边缘情况
      title: t('入池人数'),
      dataIndex: 'participant_count',
    },
    {
      title: t('参与总消耗'),
      dataIndex: 'total_consume_quota',
      render: (v) => renderQuota(v),
    },
    {
      title: t('奖池模式'),
      dataIndex: 'pool_mode',
      render: (v) => (
        <Tag color={v === 'percent' ? 'blue' : 'violet'} shape='circle'>
          {v === 'percent' ? t('百分比') : t('固定')}
        </Tag>
      ),
    },
    {
      title: t('中奖人数'),
      dataIndex: 'winner_count',
    },
    {
      title: t('奖池总额'),
      dataIndex: 'total_prize_quota',
      render: (v) => (
        <Text strong className='text-amber-600 dark:text-amber-400'>
          {renderQuota(v)}
        </Text>
      ),
    },
    {
      title: t('领取情况'),
      dataIndex: 'claim_stat',
      render: (s, r) => {
        const total = r.winner_count || 0;
        if (!total) return <Text type='tertiary'>-</Text>;
        const claimed = s?.claimed_count || 0;
        return (
          <div style={{ minWidth: 110 }}>
            <Text className='text-xs'>
              {claimed} / {total} {t('已领取')}
            </Text>
            <Progress
              percent={Math.round((claimed / total) * 100)}
              stroke='#22c55e'
              size='small'
              aria-label='claim-rate'
            />
            {s?.pending_count ? (
              <Text type='warning' className='text-[11px]'>
                {t('{{n}} 个待开启', { n: s.pending_count })}
              </Text>
            ) : null}
            {s?.expired_count ? (
              <Text type='tertiary' className='text-[11px]'>
                {' '}
                {t('{{n}} 个已过期', { n: s.expired_count })}
              </Text>
            ) : null}
          </div>
        );
      },
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v, r) => renderDrawStatus(r, t),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={draws}
      loading={loading}
      pagination={false}
      rowKey='id'
      className='w-full'
      /* 桌面端不设 scroll.x：设成 max-content 后表格宽度只到内容宽度，
         容器右侧会留白撑不满。窄屏才需要横向滚动。 */
      scroll={isMobile ? { x: 'max-content' } : undefined}
      empty={<Empty description={t('暂无开奖记录')} />}
      // 只有真正开出奖的期次可展开；空期展开是一片空白，没有信息量
      expandedRowRender={(record) => (
        <DrawWinners drawDate={record.draw_date} t={t} />
      )}
      rowExpandable={(record) => record.winner_count > 0}
    />
  );
};

export default BlindBoxDrawsTable;
