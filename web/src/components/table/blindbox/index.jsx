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

import React, { useState } from 'react';
import {
  Tabs,
  TabPane,
  Button,
  Input,
  Select,
  Space,
  Divider,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh, IconSearch } from '@douyinfe/semi-icons';
import CardPro from '../../common/ui/CardPro';
import BlindBoxRuleCard from './BlindBoxRuleCard';
import BlindBoxOverview from './BlindBoxOverview';
import BlindBoxDrawsTable from './BlindBoxDrawsTable';
import BlindBoxWinnersTable from './BlindBoxWinnersTable';
import { WINNER_STATUS_OPTIONS } from './statusTag';
import { useBlindBoxAdmin } from '../../../hooks/blindbox/useBlindBoxAdmin';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const { Title } = Typography;

const TAB_SUMMARY = 'summary';
const TAB_DRAWS = 'draws';
const TAB_WINNERS = 'winners';

const BlindBoxAdminPage = () => {
  const data = useBlindBoxAdmin();
  const isMobile = useIsMobile();
  const [tab, setTab] = useState(TAB_SUMMARY);
  // 关键字输入本地暂存，回车/点查询才提交，避免每敲一个字符打一次接口
  const [keywordDraft, setKeywordDraft] = useState('');

  const {
    t,
    summary,
    summaryLoading,
    draws,
    drawsLoading,
    drawsPage,
    drawsPageSize,
    drawsTotal,
    handleDrawsPageChange,
    handleDrawsPageSizeChange,
    winners,
    winnersLoading,
    winnersPage,
    winnersPageSize,
    winnersTotal,
    winnerFilters,
    applyWinnerFilters,
    handleWinnersPageChange,
    handleWinnersPageSizeChange,
    refreshAll,
  } = data;

  // 规则统计是纯展示，没有分页；两张列表各自维护自己的分页状态
  let pagination = null;
  if (tab === TAB_DRAWS) {
    pagination = createCardProPagination({
      currentPage: drawsPage,
      pageSize: drawsPageSize,
      total: drawsTotal,
      onPageChange: handleDrawsPageChange,
      onPageSizeChange: handleDrawsPageSizeChange,
      isMobile,
      t,
    });
  } else if (tab === TAB_WINNERS) {
    pagination = createCardProPagination({
      currentPage: winnersPage,
      pageSize: winnersPageSize,
      total: winnersTotal,
      onPageChange: handleWinnersPageChange,
      onPageSizeChange: handleWinnersPageSizeChange,
      isMobile,
      t,
    });
  }

  const renderContent = () => {
    if (tab === TAB_DRAWS) {
      return <BlindBoxDrawsTable draws={draws} loading={drawsLoading} t={t} />;
    }
    if (tab === TAB_WINNERS) {
      return (
        <BlindBoxWinnersTable
          winners={winners}
          loading={winnersLoading}
          t={t}
        />
      );
    }
    return (
      <div>
        <BlindBoxRuleCard summary={summary} loading={summaryLoading} t={t} />
        <Divider margin='20px' />
        <Title heading={6} className='!mb-3'>
          {t('历史统计')}
        </Title>
        <BlindBoxOverview
          overview={summary?.overview}
          loading={summaryLoading}
          t={t}
        />
      </div>
    );
  };

  return (
    <CardPro
      type='type3'
      tabsArea={
        <Tabs type='line' activeKey={tab} onChange={setTab} className='mb-2'>
          <TabPane tab={t('规则统计')} itemKey={TAB_SUMMARY} />
          <TabPane tab={t('开奖记录')} itemKey={TAB_DRAWS} />
          <TabPane tab={t('中奖明细')} itemKey={TAB_WINNERS} />
        </Tabs>
      }
      actionsArea={
        <div className='flex flex-col md:flex-row justify-between items-center gap-2 w-full'>
          <Button
            icon={<IconRefresh />}
            theme='light'
            type='tertiary'
            onClick={refreshAll}
          >
            {t('刷新')}
          </Button>

          {/* 筛选器只对中奖明细有意义：规则统计是纯展示，开奖记录是按期倒序的完整流水 */}
          {tab === TAB_WINNERS && (
            <Space wrap>
              <Input
                prefix={<IconSearch />}
                placeholder={t('用户名或用户 ID')}
                value={keywordDraft}
                onChange={setKeywordDraft}
                onEnterPress={() =>
                  applyWinnerFilters({ keyword: keywordDraft })
                }
                style={{ width: 200 }}
                showClear
              />
              <Select
                value={winnerFilters.status}
                onChange={(v) => applyWinnerFilters({ status: v })}
                optionList={WINNER_STATUS_OPTIONS(t)}
                style={{ width: 140 }}
              />
              <Input
                placeholder={t('开奖日期 YYYY-MM-DD')}
                value={winnerFilters.draw_date}
                onChange={(v) => applyWinnerFilters({ draw_date: v })}
                style={{ width: 180 }}
                showClear
              />
              <Button
                theme='solid'
                onClick={() => applyWinnerFilters({ keyword: keywordDraft })}
              >
                {t('查询')}
              </Button>
            </Space>
          )}
        </div>
      }
      paginationArea={pagination}
      t={t}
    >
      {renderContent()}
    </CardPro>
  );
};

export default BlindBoxAdminPage;
