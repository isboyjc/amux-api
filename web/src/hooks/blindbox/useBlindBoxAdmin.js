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

import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const DEFAULT_PAGE_SIZE = 20;

/**
 * 盲盒管理页数据源。
 *
 * 三个接口彼此独立：概要（规则/预览/统计）、开奖周期分页、中奖明细分页。
 * 刻意不做联动刷新——切 Tab 或翻页只打对应接口，避免一次操作触发三个请求。
 */
export const useBlindBoxAdmin = () => {
  const { t } = useTranslation();

  const [summary, setSummary] = useState(null);
  const [summaryLoading, setSummaryLoading] = useState(true);

  const [draws, setDraws] = useState([]);
  const [drawsLoading, setDrawsLoading] = useState(false);
  const [drawsPage, setDrawsPage] = useState(1);
  const [drawsPageSize, setDrawsPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [drawsTotal, setDrawsTotal] = useState(0);

  const [winners, setWinners] = useState([]);
  const [winnersLoading, setWinnersLoading] = useState(false);
  const [winnersPage, setWinnersPage] = useState(1);
  const [winnersPageSize, setWinnersPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [winnersTotal, setWinnersTotal] = useState(0);
  const [winnerFilters, setWinnerFilters] = useState({
    draw_date: '',
    status: '',
    keyword: '',
  });

  const [blocks, setBlocks] = useState([]);
  const [blocksLoading, setBlocksLoading] = useState(false);
  const [blocksPage, setBlocksPage] = useState(1);
  const [blocksPageSize, setBlocksPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [blocksTotal, setBlocksTotal] = useState(0);
  const [blockFilters, setBlockFilters] = useState({ scope: '', keyword: '' });

  const loadSummary = useCallback(async () => {
    setSummaryLoading(true);
    try {
      const res = await API.get('/api/blindbox/summary');
      const { success, message, data } = res.data;
      if (success) {
        setSummary(data);
      } else {
        showError(message || t('获取盲盒概要失败'));
      }
    } catch (e) {
      showError(t('获取盲盒概要失败'));
    } finally {
      setSummaryLoading(false);
    }
  }, [t]);

  const loadDraws = useCallback(
    async (page = drawsPage, size = drawsPageSize) => {
      setDrawsLoading(true);
      try {
        const res = await API.get(
          `/api/blindbox/draws?p=${page}&page_size=${size}`,
        );
        const { success, message, data } = res.data;
        if (success) {
          setDraws(data.items || []);
          setDrawsTotal(data.total || 0);
        } else {
          showError(message || t('获取开奖记录失败'));
        }
      } catch (e) {
        showError(t('获取开奖记录失败'));
      } finally {
        setDrawsLoading(false);
      }
    },
    [drawsPage, drawsPageSize, t],
  );

  const loadWinners = useCallback(
    async (page = winnersPage, size = winnersPageSize, filters) => {
      const f = filters || winnerFilters;
      setWinnersLoading(true);
      try {
        const params = new URLSearchParams({ p: page, page_size: size });
        if (f.draw_date) params.set('draw_date', f.draw_date);
        if (f.status) params.set('status', f.status);
        if (f.keyword) params.set('keyword', f.keyword);
        const res = await API.get(`/api/blindbox/winners?${params.toString()}`);
        const { success, message, data } = res.data;
        if (success) {
          setWinners(data.items || []);
          setWinnersTotal(data.total || 0);
        } else {
          showError(message || t('获取中奖明细失败'));
        }
      } catch (e) {
        showError(t('获取中奖明细失败'));
      } finally {
        setWinnersLoading(false);
      }
    },
    [winnersPage, winnersPageSize, winnerFilters, t],
  );

  const loadBlocks = useCallback(
    async (page = blocksPage, size = blocksPageSize, filters) => {
      const f = filters || blockFilters;
      setBlocksLoading(true);
      try {
        const params = new URLSearchParams({ p: page, page_size: size });
        if (f.scope) params.set('scope', f.scope);
        if (f.keyword) params.set('keyword', f.keyword);
        const res = await API.get(`/api/blindbox/blocks?${params.toString()}`);
        const { success, message, data } = res.data;
        if (success) {
          setBlocks(data.items || []);
          setBlocksTotal(data.total || 0);
        } else {
          showError(message || t('获取屏蔽名单失败'));
        }
      } catch (e) {
        showError(t('获取屏蔽名单失败'));
      } finally {
        setBlocksLoading(false);
      }
    },
    [blocksPage, blocksPageSize, blockFilters, t],
  );

  // 解除屏蔽后当期入池人数与奖池都会变，summary 一并刷新
  const unblockUser = async (blockId) => {
    try {
      const res = await API.delete(`/api/blindbox/blocks/${blockId}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('已解除屏蔽'));
        loadBlocks(blocksPage, blocksPageSize);
        loadSummary();
      } else {
        showError(message || t('解除屏蔽失败'));
      }
    } catch (e) {
      showError(t('解除屏蔽失败'));
    }
  };

  const handleDrawsPageChange = (page) => {
    setDrawsPage(page);
    loadDraws(page, drawsPageSize);
  };
  const handleDrawsPageSizeChange = (size) => {
    setDrawsPageSize(size);
    setDrawsPage(1);
    loadDraws(1, size);
  };
  const handleWinnersPageChange = (page) => {
    setWinnersPage(page);
    loadWinners(page, winnersPageSize);
  };
  const handleWinnersPageSizeChange = (size) => {
    setWinnersPageSize(size);
    setWinnersPage(1);
    loadWinners(1, size);
  };

  // 应用筛选：始终回到第 1 页，否则在第 5 页筛出 3 条会显示空表
  const applyWinnerFilters = (filters) => {
    const next = { ...winnerFilters, ...filters };
    setWinnerFilters(next);
    setWinnersPage(1);
    loadWinners(1, winnersPageSize, next);
  };

  const handleBlocksPageChange = (page) => {
    setBlocksPage(page);
    loadBlocks(page, blocksPageSize);
  };
  const handleBlocksPageSizeChange = (size) => {
    setBlocksPageSize(size);
    setBlocksPage(1);
    loadBlocks(1, size);
  };
  const applyBlockFilters = (filters) => {
    const next = { ...blockFilters, ...filters };
    setBlockFilters(next);
    setBlocksPage(1);
    loadBlocks(1, blocksPageSize, next);
  };

  const refreshAll = () => {
    loadSummary();
    loadDraws(drawsPage, drawsPageSize);
    loadWinners(winnersPage, winnersPageSize);
    loadBlocks(blocksPage, blocksPageSize);
  };

  useEffect(() => {
    loadSummary();
    loadDraws(1, DEFAULT_PAGE_SIZE);
    loadWinners(1, DEFAULT_PAGE_SIZE);
    loadBlocks(1, DEFAULT_PAGE_SIZE);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
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
    blocks,
    blocksLoading,
    blocksPage,
    blocksPageSize,
    blocksTotal,
    blockFilters,
    applyBlockFilters,
    handleBlocksPageChange,
    handleBlocksPageSizeChange,
    unblockUser,
    loadSummary,
    refreshAll,
  };
};
