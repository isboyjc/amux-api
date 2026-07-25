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
import { Typography, Row, Col, Skeleton } from '@douyinfe/semi-ui';
import { renderQuota } from '../../../helpers';

const { Text } = Typography;

/**
 * 全局概览。
 *
 * 领取率是这个玩法最关键的指标：盲盒刻意设计成"次日手动开盒、逾期作废"，
 * 领取率直接反映这套机制的实际转化，比单看发放额度有意义得多。
 *
 * 与规则区块一样渲染为普通区块，Card 外壳由外层「规则统计」Tab 提供。
 */
const BlindBoxOverview = ({ overview, loading, t }) => {
  if (loading || !overview) {
    return (
      <Skeleton placeholder={<Skeleton.Paragraph rows={2} />} loading active>
        <div />
      </Skeleton>
    );
  }

  const settledCount =
    overview.claimed_count + overview.pending_count + overview.expired_count;
  // 分母用已开出的中奖总数；为 0 时显示 "-"，不要显示 0% 误导成"没人领"
  const claimRate =
    settledCount > 0
      ? `${((overview.claimed_count / settledCount) * 100).toFixed(1)}%`
      : '-';

  const items = [
    { label: t('累计开奖期数'), value: overview.draw_count },
    { label: t('累计参与人次'), value: overview.total_participant },
    { label: t('累计中奖人次'), value: overview.total_winner },
    {
      label: t('累计奖池发放'),
      value: renderQuota(overview.total_prize_quota),
    },
    {
      label: t('已领取额度'),
      value: renderQuota(overview.claimed_quota),
      hint: t('{{n}} 人次', { n: overview.claimed_count }),
      color: 'text-emerald-600 dark:text-emerald-400',
    },
    {
      label: t('待开启额度'),
      value: renderQuota(overview.pending_quota),
      hint: t('{{n}} 人次', { n: overview.pending_count }),
      color: 'text-amber-600 dark:text-amber-400',
    },
    {
      label: t('已过期额度'),
      value: renderQuota(overview.expired_quota),
      hint: t('{{n}} 人次', { n: overview.expired_count }),
      color: 'text-gray-500',
    },
    { label: t('领取率'), value: claimRate },
  ];

  return (
    <div>
      <Row gutter={[16, 16]}>
        {items.map((it) => (
          <Col key={it.label} xs={12} sm={8} md={6} lg={3}>
            <Text type='tertiary' className='text-xs block'>
              {it.label}
            </Text>
            <div className={`text-lg font-semibold ${it.color || ''}`}>
              {it.value}
            </div>
            {it.hint ? (
              <Text type='tertiary' className='text-[11px]'>
                {it.hint}
              </Text>
            ) : null}
          </Col>
        ))}
      </Row>
    </div>
  );
};

export default BlindBoxOverview;
