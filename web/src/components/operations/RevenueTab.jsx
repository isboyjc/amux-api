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
import { Card, Empty, Spin, Typography } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { CARD_PROPS, CHART_CONFIG } from '../../constants/dashboard.constants';

// 站内消耗（quota 单位）转金额口径（原币种数值，方便在同一坐标轴上和充值对比）
// 注意：quota_per_unit 是「quota / 1 USD」的换算系数，未做 CNY/CUSTOM 二次换算，
// 跨多支付货币的部署只能近似看趋势。
const quotaToMoney = (quota) => {
  const raw = parseFloat(localStorage.getItem('quota_per_unit') || '1');
  if (!raw || isNaN(raw)) return 0;
  return Number(quota || 0) / raw;
};

const formatBucketLabel = (ts, bucket) => {
  const d = new Date(ts * 1000);
  if (bucket === 'hour') {
    return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:00`;
  }
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

const SEGMENT_LABEL = {
  new: '新付费用户',
  returning: '复购用户',
};

const RevenueTab = ({ data, loading, t }) => {
  const series = data?.series || [];
  const payerSplit = (data?.payer_split || []).filter((s) => s.users > 0);

  const lineSpec = useMemo(() => {
    const points = [];
    series.forEach((p) => {
      const label = formatBucketLabel(p.bucket, data?.bucket);
      points.push({
        time: label,
        type: t('充值流水'),
        value: Number(p.topup_sum || 0),
      });
      points.push({
        time: label,
        type: t('站内消耗（金额口径）'),
        value: quotaToMoney(p.consume),
      });
    });
    return {
      type: 'line',
      data: { values: points },
      xField: 'time',
      yField: 'value',
      seriesField: 'type',
      point: { visible: false },
      tooltip: { mark: { visible: true } },
      legends: { visible: true, orient: 'top', position: 'end' },
      axes: [
        { orient: 'bottom', label: { visible: true } },
        { orient: 'left', label: { visible: true } },
      ],
    };
  }, [series, data?.bucket, t]);

  // 新老付费用户：饼图按"用户数"切片，tooltip / legend 上同时展示金额，
  // 这样运营既能看到人数占比，也能看到金额占比。
  const pieSpec = useMemo(() => {
    const values = payerSplit.map((s) => ({
      type: t(SEGMENT_LABEL[s.segment] || s.segment),
      value: Number(s.users || 0),
      amount: Number(s.amount || 0),
    }));
    return {
      type: 'pie',
      data: { values },
      categoryField: 'type',
      valueField: 'value',
      outerRadius: 0.8,
      innerRadius: 0.5,
      padAngle: 0.6,
      pie: { style: { cornerRadius: 6 } },
      legends: { visible: true, orient: 'right' },
      label: { visible: true },
      tooltip: {
        mark: {
          title: { value: (datum) => datum?.type },
          content: [
            { key: t('用户数'), value: (datum) => datum?.value },
            {
              key: t('金额'),
              value: (datum) => Number(datum?.amount || 0).toFixed(2),
            },
          ],
        },
      },
    };
  }, [payerSplit, t]);

  return (
    <div className='grid grid-cols-1 lg:grid-cols-3 gap-4'>
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl lg:col-span-2'
        title={
          <div>
            <Typography.Text strong>{t('充值与消耗趋势')}</Typography.Text>
            <Typography.Text type='tertiary' style={{ marginLeft: 8, fontSize: 12 }}>
              {t('两条曲线均为金额口径，便于对比')}
            </Typography.Text>
          </div>
        }
        bodyStyle={{ padding: 8 }}
      >
        <Spin spinning={loading}>
          <div style={{ height: 320 }}>
            {series.length === 0 ? (
              <Empty description={t('区间内暂无数据')} />
            ) : (
              <VChart spec={lineSpec} option={CHART_CONFIG} />
            )}
          </div>
        </Spin>
      </Card>

      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={
          <div>
            <Typography.Text strong>{t('新老付费用户占比')}</Typography.Text>
            <Typography.Text
              type='tertiary'
              style={{ marginLeft: 8, fontSize: 12 }}
            >
              {t('按用户数切片，hover 看金额')}
            </Typography.Text>
          </div>
        }
        bodyStyle={{ padding: 8 }}
      >
        <Spin spinning={loading}>
          <div style={{ height: 320 }}>
            {payerSplit.length === 0 ? (
              <Empty description={t('区间内暂无付费用户')} />
            ) : (
              <VChart spec={pieSpec} option={CHART_CONFIG} />
            )}
          </div>
        </Spin>
      </Card>
    </div>
  );
};

export default RevenueTab;
