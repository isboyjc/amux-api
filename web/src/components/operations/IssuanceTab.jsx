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
import { Card, Empty, Spin, Tooltip, Typography } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { VChart } from '@visactor/react-vchart';
import { CARD_PROPS, CHART_CONFIG } from '../../constants/dashboard.constants';
import { renderQuota } from '../../helpers/render';

const SOURCE_LABELS = {
  real_topup_quota: '在线支付充值',
  admin_topup_quota: '管理员充值',
  signup_gift_quota: '注册赠送',
  inviter_rebate: '邀请人返利',
  topup_rebate: '充值返利',
  redemption_quota: '兑换码核销',
};

const IssuanceTab = ({ data, balance, loading, t }) => {
  const items = useMemo(() => {
    if (!data) return [];
    return Object.keys(SOURCE_LABELS).map((k) => ({
      key: k,
      label: t(SOURCE_LABELS[k]),
      value: Number(data[k] || 0),
    }));
  }, [data, t]);

  const total = items.reduce((s, x) => s + x.value, 0);

  const pieSpec = useMemo(() => {
    const values = items
      .filter((x) => x.value > 0)
      .map((x) => ({ type: x.label, value: x.value }));
    return {
      type: 'pie',
      data: { values },
      categoryField: 'type',
      valueField: 'value',
      outerRadius: 0.85,
      innerRadius: 0.55,
      padAngle: 0.6,
      pie: { style: { cornerRadius: 6 } },
      legends: { visible: true, orient: 'right' },
      label: { visible: true },
    };
  }, [items]);

  return (
    <div className='grid grid-cols-1 lg:grid-cols-3 gap-4'>
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl lg:col-span-2'
        title={
          <div>
            <Typography.Text strong>{t('额度发放来源占比')}</Typography.Text>
            <Typography.Text
              type='tertiary'
              style={{ marginLeft: 8, fontSize: 12 }}
            >
              {t('管理员手动调整不计入此图（仅暴露次数）')}
            </Typography.Text>
          </div>
        }
        bodyStyle={{ padding: 8 }}
      >
        <Spin spinning={loading}>
          <div style={{ height: 320 }}>
            {!data || total === 0 ? (
              <Empty description={t('区间内无额度发放')} />
            ) : (
              <VChart spec={pieSpec} option={CHART_CONFIG} />
            )}
          </div>
        </Spin>
      </Card>

      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={t('额度发放明细')}
        bodyStyle={{ padding: 16 }}
      >
        <Spin spinning={loading}>
          {!data ? (
            <Empty description={t('暂无数据')} />
          ) : (
            <div className='flex flex-col gap-3'>
              {items.map((it) => (
                <Row key={it.key} label={it.label} value={renderQuota(it.value, 2)} />
              ))}

              <div
                style={{
                  borderTop: '1px solid var(--semi-color-border)',
                  margin: '8px 0',
                }}
              />

              <Row
                label={t('管理员手动调整次数')}
                value={String(data.admin_adjust_count || 0)}
                hint={t(
                  '区间内 type=3 的日志条数（含增加/减少/覆盖三种操作）',
                )}
                t={t}
              />
              <Row
                label={t('注册赠送参数（当前）')}
                value={`${t('新用户')} ${renderQuota(
                  data.quota_for_new_user || 0,
                  2,
                )} · ${t('被邀请')} ${renderQuota(
                  data.quota_for_invitee || 0,
                  2,
                )}`}
                hint={t(
                  '系统当前的赠送配置；上方"注册赠送"金额是从日志聚合的实发金额，与配置无关',
                )}
                t={t}
              />
              <Row
                label={t('用户余额总和')}
                value={
                  balance ? renderQuota(balance.total_quota || 0, 2) : '—'
                }
                hint={t('当前所有用户剩余额度合计（系统快照）')}
                t={t}
              />
              <Row
                label={t('AFF 待领取池')}
                value={
                  balance ? renderQuota(balance.total_aff_quota || 0, 2) : '—'
                }
                hint={t('所有用户 AFF 钱包中尚未划转到主余额的累计返利')}
                t={t}
              />
            </div>
          )}
        </Spin>
      </Card>
    </div>
  );
};

const Row = ({ label, value, hint, t }) => (
  <div
    style={{
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      gap: 8,
    }}
  >
    <span
      style={{
        color: 'var(--semi-color-text-2)',
        fontSize: 13,
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
      }}
    >
      {label}
      {hint && (
        <Tooltip content={hint}>
          <Info size={12} style={{ opacity: 0.6 }} />
        </Tooltip>
      )}
    </span>
    <Typography.Text strong>{value}</Typography.Text>
  </div>
);

export default IssuanceTab;
