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

import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useTopupsData = () => {
  const { t } = useTranslation();
  const [compactMode, setCompactMode] = useTableCompactMode('topups');

  const [topups, setTopups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [topupCount, setTopupCount] = useState(0);

  // Default filter = success only
  const formInitValues = {
    searchKeyword: '',
    searchStatus: 'success',
  };

  const [formApi, setFormApi] = useState(null);

  const statusOptions = useMemo(
    () => [
      { label: t('全部'), value: 'all' },
      { label: t('成功'), value: 'success' },
      { label: t('待支付'), value: 'pending' },
      { label: t('失败'), value: 'failed' },
      { label: t('过期'), value: 'expired' },
    ],
    [t],
  );

  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
      searchStatus:
        formValues.searchStatus === undefined || formValues.searchStatus === null
          ? 'success'
          : formValues.searchStatus,
    };
  };

  const setTopupFormat = (items) => {
    for (let i = 0; i < items.length; i++) {
      items[i].key = items[i].id;
    }
    setTopups(items);
  };

  const buildQueryString = (startIdx, sizeValue, keyword, status) => {
    const params = [
      `p=${startIdx}`,
      `page_size=${sizeValue}`,
      `status=${encodeURIComponent(status || 'all')}`,
    ];
    if (keyword) {
      params.push(`keyword=${encodeURIComponent(keyword)}`);
    }
    return params.join('&');
  };

  // Initial / reset load — uses default status filter (success)
  const loadTopups = async (
    startIdx = 1,
    sizeValue = pageSize,
    overrideStatus = 'success',
  ) => {
    setLoading(true);
    try {
      const qs = buildQueryString(startIdx, sizeValue, '', overrideStatus);
      const res = await API.get(`/api/user/topup?${qs}`);
      const { success, message, data } = res.data;
      if (success) {
        const items = data.items || [];
        setActivePage(data.page || startIdx);
        setTopupCount(data.total || 0);
        setTopupFormat(items);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message || t('加载账单失败'));
    } finally {
      setLoading(false);
    }
  };

  // Search (keyword + status filter) — uses form values when called without args
  const searchTopups = async (
    startIdx = 1,
    sizeValue = pageSize,
    searchKeyword = null,
    searchStatus = null,
  ) => {
    if (searchKeyword === null || searchStatus === null) {
      const values = getFormValues();
      searchKeyword = values.searchKeyword;
      searchStatus = values.searchStatus;
    }

    setSearching(true);
    try {
      const qs = buildQueryString(
        startIdx,
        sizeValue,
        searchKeyword,
        searchStatus,
      );
      const res = await API.get(`/api/user/topup?${qs}`);
      const { success, message, data } = res.data;
      if (success) {
        const items = data.items || [];
        setActivePage(data.page || startIdx);
        setTopupCount(data.total || 0);
        setTopupFormat(items);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message || t('加载账单失败'));
    } finally {
      setSearching(false);
    }
  };

  const refresh = async (page = activePage) => {
    const { searchKeyword, searchStatus } = getFormValues();
    await searchTopups(page, pageSize, searchKeyword, searchStatus);
  };

  const handlePageChange = (page) => {
    setActivePage(page);
    const { searchKeyword, searchStatus } = getFormValues();
    searchTopups(page, pageSize, searchKeyword, searchStatus).then();
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    const { searchKeyword, searchStatus } = getFormValues();
    searchTopups(1, size, searchKeyword, searchStatus)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  const adminCompleteTopup = async (tradeNo) => {
    try {
      const res = await API.post('/api/user/topup/complete', {
        trade_no: tradeNo,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('补单成功'));
        await refresh();
      } else {
        showError(message || t('补单失败'));
      }
    } catch (error) {
      showError(error.message || t('补单失败'));
    }
  };

  useEffect(() => {
    loadTopups(1, pageSize, 'success')
      .then()
      .catch((reason) => {
        showError(reason);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    topups,
    loading,
    searching,
    activePage,
    pageSize,
    topupCount,

    formInitValues,
    formApi,
    setFormApi,

    compactMode,
    setCompactMode,

    statusOptions,

    loadTopups,
    searchTopups,
    handlePageChange,
    handlePageSizeChange,
    adminCompleteTopup,
    refresh,

    t,
  };
};
