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

import React, { useCallback, useEffect, useState } from 'react';
import { Card, TabPane, Tabs } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { API, showError } from '../../helpers';
import { CARD_PROPS } from '../../constants/dashboard.constants';

import OperationsHeader from './OperationsHeader';
import OverviewKpis from './OverviewKpis';
import RevenueTab from './RevenueTab';
import IssuanceTab from './IssuanceTab';
import ChannelHealthTab from './ChannelHealthTab';
import AffiliateTab from './AffiliateTab';
import UsersTab from './UsersTab';

const DEFAULT_RANGE_KEY = '7d';
const DEFAULT_RANGE_SECONDS = 7 * 24 * 3600;

const Operations = () => {
  const { t } = useTranslation();

  const [rangeKey, setRangeKey] = useState(DEFAULT_RANGE_KEY);
  const [rangeSeconds, setRangeSeconds] = useState(DEFAULT_RANGE_SECONDS);
  const [activeTab, setActiveTab] = useState('revenue');

  const [overview, setOverview] = useState(null);
  const [balance, setBalance] = useState(null);
  const [revenue, setRevenue] = useState(null);
  const [issuance, setIssuance] = useState(null);
  const [channels, setChannels] = useState([]);
  const [affiliate, setAffiliate] = useState(null);
  const [topConsumers, setTopConsumers] = useState([]);
  const [recentTopups, setRecentTopups] = useState([]);

  const [loadingOverview, setLoadingOverview] = useState(false);
  const [loadingRevenue, setLoadingRevenue] = useState(false);
  const [loadingIssuance, setLoadingIssuance] = useState(false);
  const [loadingChannels, setLoadingChannels] = useState(false);
  const [loadingAffiliate, setLoadingAffiliate] = useState(false);
  const [loadingUsers, setLoadingUsers] = useState(false);

  const buildParams = useCallback(() => {
    const end = Math.floor(Date.now() / 1000);
    const start = end - rangeSeconds;
    return { start_timestamp: start, end_timestamp: end };
  }, [rangeSeconds]);

  const handleApi = async (url, params, setter, setLoading) => {
    setLoading(true);
    try {
      const res = await API.get(url, { params, disableDuplicate: true });
      const { success, message, data } = res.data || {};
      if (success) {
        setter(data);
      } else {
        showError(message || t('加载失败'));
      }
    } catch (e) {
      showError(e?.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  const loadAll = useCallback(async () => {
    const params = buildParams();
    await Promise.all([
      handleApi(
        '/api/operations/overview',
        params,
        setOverview,
        setLoadingOverview,
      ),
      // 余额快照不带时间区间
      handleApi(
        '/api/operations/balance_snapshot',
        {},
        setBalance,
        () => {},
      ),
      handleApi(
        '/api/operations/revenue_trend',
        params,
        setRevenue,
        setLoadingRevenue,
      ),
      handleApi(
        '/api/operations/quota_issuance',
        params,
        setIssuance,
        setLoadingIssuance,
      ),
      handleApi(
        '/api/operations/channel_health',
        { ...params, limit: 30 },
        setChannels,
        setLoadingChannels,
      ),
      handleApi(
        '/api/operations/affiliate',
        { ...params, limit: 30 },
        setAffiliate,
        setLoadingAffiliate,
      ),
      handleApi(
        '/api/operations/top_consumers',
        { ...params, limit: 20 },
        setTopConsumers,
        setLoadingUsers,
      ),
      handleApi(
        '/api/operations/recent_topups',
        { limit: 20 },
        setRecentTopups,
        () => {},
      ),
    ]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [buildParams]);

  // VChart Semi 主题（亮/暗自适应）。多次调用安全，库内部会去重。
  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  useEffect(() => {
    loadAll();
  }, [loadAll]);

  const handleChangeRange = (key, seconds) => {
    setRangeKey(key);
    setRangeSeconds(seconds);
  };

  const anyLoading =
    loadingOverview ||
    loadingRevenue ||
    loadingIssuance ||
    loadingChannels ||
    loadingAffiliate ||
    loadingUsers;

  return (
    <div className='h-full'>
      <OperationsHeader
        rangeKey={rangeKey}
        onChangeRange={handleChangeRange}
        onRefresh={loadAll}
        loading={anyLoading}
        t={t}
      />

      <OverviewKpis
        overview={overview}
        balance={balance}
        loading={loadingOverview}
        t={t}
      />

      <Card {...CARD_PROPS} className='!rounded-2xl'>
        <Tabs
          type='line'
          activeKey={activeTab}
          onChange={setActiveTab}
          collapsible
        >
          <TabPane tab={t('营收')} itemKey='revenue'>
            <div className='pt-2'>
              <RevenueTab data={revenue} loading={loadingRevenue} t={t} />
            </div>
          </TabPane>
          <TabPane tab={t('额度发放')} itemKey='issuance'>
            <div className='pt-2'>
              <IssuanceTab
                data={issuance}
                balance={balance}
                loading={loadingIssuance}
                t={t}
              />
            </div>
          </TabPane>
          <TabPane tab={t('渠道健康')} itemKey='channel'>
            <div className='pt-2'>
              <ChannelHealthTab
                rows={channels}
                loading={loadingChannels}
                t={t}
              />
            </div>
          </TabPane>
          <TabPane tab={t('邀请分析')} itemKey='affiliate'>
            <div className='pt-2'>
              <AffiliateTab
                data={affiliate}
                loading={loadingAffiliate}
                t={t}
              />
            </div>
          </TabPane>
          <TabPane tab={t('重点用户')} itemKey='users'>
            <div className='pt-2'>
              <UsersTab
                topConsumers={topConsumers}
                recentTopups={recentTopups}
                loading={loadingUsers}
                t={t}
              />
            </div>
          </TabPane>
        </Tabs>
      </Card>
    </div>
  );
};

export default Operations;
