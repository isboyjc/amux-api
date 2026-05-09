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
import { Card, Button, Typography } from '@douyinfe/semi-ui';
import { BarChart3, RefreshCw } from 'lucide-react';
import { CARD_PROPS, FLEX_CENTER_GAP2 } from '../../constants/dashboard.constants';

const PRESET_RANGES = [
  { key: '1d', labelKey: '今日', seconds: 24 * 3600 },
  { key: '7d', labelKey: '近 7 天', seconds: 7 * 24 * 3600 },
  { key: '30d', labelKey: '近 30 天', seconds: 30 * 24 * 3600 },
  { key: '90d', labelKey: '近 90 天', seconds: 90 * 24 * 3600 },
  // "全部"：用一个足够大的窗口（50 年）覆盖任何站点的全生命周期；
  // 后端 maxOperationsRangeSeconds 也放宽到 50 年。
  { key: 'all', labelKey: '全部', seconds: 50 * 365 * 24 * 3600 },
];

const OperationsHeader = ({ rangeKey, onChangeRange, onRefresh, loading, t }) => {
  return (
    <Card
      {...CARD_PROPS}
      className='!rounded-2xl'
      style={{ marginBottom: 12 }}
    >
      <div className='flex flex-col lg:flex-row lg:items-center lg:justify-between gap-3'>
        <div className={FLEX_CENTER_GAP2}>
          <BarChart3 size={18} />
          <Typography.Title heading={5} style={{ margin: 0 }}>
            {t('运营统计')}
          </Typography.Title>
          <Typography.Text type='tertiary' style={{ marginLeft: 8 }}>
            {t('管理员视角的站点经营、营收与渠道健康概览')}
          </Typography.Text>
        </div>
        <div className='flex items-center gap-3 flex-wrap'>
          <div className='flex items-center gap-2'>
            {PRESET_RANGES.map((r) => {
              const active = rangeKey === r.key;
              return (
                <Button
                  key={r.key}
                  size='small'
                  theme={active ? 'solid' : 'borderless'}
                  type={active ? 'primary' : 'tertiary'}
                  onClick={() => onChangeRange(r.key, r.seconds)}
                >
                  {t(r.labelKey)}
                </Button>
              );
            })}
          </div>
          <Button
            theme='light'
            size='small'
            icon={<RefreshCw size={14} />}
            onClick={onRefresh}
            loading={loading}
          >
            {t('刷新')}
          </Button>
        </div>
      </div>
    </Card>
  );
};

export default OperationsHeader;
