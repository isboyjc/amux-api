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
import { Card, Skeleton, Tooltip, Typography } from '@douyinfe/semi-ui';
import {
  ArrowDownRight,
  ArrowUpRight,
  CircleDollarSign,
  Coins,
  Gift,
  Info,
  PiggyBank,
  TrendingUp,
  UserCheck,
  UserPlus,
  Users,
} from 'lucide-react';
import { CARD_PROPS } from '../../constants/dashboard.constants';
import { renderQuota } from '../../helpers/render';

const formatMoney = (n) => {
  if (n === null || n === undefined || isNaN(n)) return '0';
  if (Math.abs(n) >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M';
  if (Math.abs(n) >= 10_000) return (n / 1_000).toFixed(1) + 'K';
  return Number(n).toFixed(2);
};

const formatInt = (n) => {
  if (n === null || n === undefined || isNaN(n)) return '0';
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 10_000) return (n / 1_000).toFixed(1) + 'K';
  return String(Math.round(n));
};

const computeDelta = (current, previous) => {
  if (previous === undefined || previous === null) return null;
  if (previous === 0) {
    if (current === 0) return { pct: 0, dir: 'flat' };
    return { pct: null, dir: current > 0 ? 'up' : 'down' };
  }
  const pct = ((current - previous) / Math.abs(previous)) * 100;
  return {
    pct,
    dir: pct > 0.5 ? 'up' : pct < -0.5 ? 'down' : 'flat',
  };
};

const DeltaBadge = ({ delta, invertColor = false, t }) => {
  if (!delta) return null;
  const { pct, dir } = delta;
  const goodWhenUp = !invertColor;
  const isGood =
    (dir === 'up' && goodWhenUp) || (dir === 'down' && !goodWhenUp);
  const isBad =
    (dir === 'down' && goodWhenUp) || (dir === 'up' && !goodWhenUp);
  const color = isGood
    ? 'var(--semi-color-success)'
    : isBad
      ? 'var(--semi-color-danger)'
      : 'var(--semi-color-text-2)';
  const Icon =
    dir === 'up' ? ArrowUpRight : dir === 'down' ? ArrowDownRight : null;
  return (
    <span
      style={{
        color,
        fontSize: 12,
        display: 'inline-flex',
        alignItems: 'center',
        gap: 2,
      }}
    >
      {Icon && <Icon size={12} />}
      {pct === null
        ? t('无对比')
        : `${pct >= 0 ? '+' : ''}${pct.toFixed(1)}%`}
    </span>
  );
};

const KpiCard = ({ icon, title, value, hint, delta, invertColor, snapshot, t }) => (
  <Card {...CARD_PROPS} className='!rounded-2xl' bodyStyle={{ padding: 16 }}>
    <div className='flex items-start justify-between gap-2'>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            color: 'var(--semi-color-text-2)',
            fontSize: 12,
            marginBottom: 6,
          }}
        >
          {icon}
          <span>{title}</span>
          {hint && (
            <Tooltip content={hint}>
              <Info size={12} style={{ opacity: 0.6 }} />
            </Tooltip>
          )}
        </div>
        <Typography.Title heading={4} style={{ margin: 0 }}>
          {value}
        </Typography.Title>
        <div style={{ marginTop: 4, minHeight: 16 }}>
          {snapshot ? (
            <Typography.Text type='tertiary' style={{ fontSize: 11 }}>
              {t('系统快照')}
            </Typography.Text>
          ) : (
            <>
              <DeltaBadge delta={delta} invertColor={invertColor} t={t} />
              {delta && (
                <Typography.Text
                  type='tertiary'
                  style={{ fontSize: 11, marginLeft: 6 }}
                >
                  {t('对比上一周期')}
                </Typography.Text>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  </Card>
);

const OverviewKpis = ({ overview, balance, loading, t }) => {
  const cards = useMemo(() => {
    if (!overview) return null;
    const cur = overview.current || {};
    const prev = overview.previous || {};
    const bal = balance || null;
    return [
      {
        icon: <CircleDollarSign size={14} />,
        title: t('充值流水'),
        value: formatMoney(cur.topup_amount),
        delta: computeDelta(cur.topup_amount, prev.topup_amount),
        hint: t('区间内成功充值订单的总金额，按支付网关原币种汇总'),
      },
      {
        icon: <Coins size={14} />,
        title: t('站内消耗（金额口径）'),
        value: renderQuota(cur.consumption_quota || 0, 2),
        delta: computeDelta(cur.consumption_quota, prev.consumption_quota),
        invertColor: true,
        hint: t(
          '按站内定价从用户余额扣除的总额度，是面向用户的"售出额"，不是平台真实成本',
        ),
      },
      {
        icon: <PiggyBank size={14} />,
        title: t('用户余额总和'),
        value: bal ? renderQuota(bal.total_quota || 0, 2) : '—',
        delta: null,
        snapshot: true,
        hint: t(
          '当前所有用户的剩余额度合计（系统快照，不随时间筛选变化）。这是平台对用户的余额负债。',
        ),
      },
      {
        icon: <Users size={14} />,
        title: t('付费用户数'),
        value: formatInt(cur.paying_users),
        delta: computeDelta(cur.paying_users, prev.paying_users),
        hint: t('区间内有过成功充值的去重用户数'),
      },
      {
        icon: <TrendingUp size={14} />,
        title: t('客单价 ARPU'),
        value: formatMoney(cur.arpu),
        delta: computeDelta(cur.arpu, prev.arpu),
        hint: t('充值流水 ÷ 付费用户数'),
      },
      {
        icon: <UserPlus size={14} />,
        title: t('新增用户'),
        value: formatInt(cur.new_users),
        delta: computeDelta(cur.new_users, prev.new_users),
        hint: t('区间内注册的用户数'),
      },
      {
        icon: <UserCheck size={14} />,
        title: t('活跃用户'),
        value: formatInt(cur.active_users),
        delta: computeDelta(cur.active_users, prev.active_users),
        hint: t('区间内有过消耗记录的去重用户数'),
      },
      {
        icon: <Gift size={14} />,
        title: t('兑换码核销'),
        value: renderQuota(cur.redemption_quota || 0, 2),
        delta: computeDelta(cur.redemption_quota, prev.redemption_quota),
        hint: t('区间内被使用的兑换码总额度（金额口径）'),
      },
    ];
  }, [overview, balance, t]);

  if (loading || !cards) {
    return (
      <div className='grid grid-cols-2 md:grid-cols-4 gap-3 mb-4'>
        {[...Array(8)].map((_, i) => (
          <Card key={i} {...CARD_PROPS} className='!rounded-2xl'>
            <Skeleton placeholder={<Skeleton.Paragraph rows={2} />} loading />
          </Card>
        ))}
      </div>
    );
  }

  return (
    <div className='grid grid-cols-2 md:grid-cols-4 gap-3 mb-4'>
      {cards.map((c, i) => (
        <KpiCard key={i} {...c} t={t} />
      ))}
    </div>
  );
};

export default OverviewKpis;
