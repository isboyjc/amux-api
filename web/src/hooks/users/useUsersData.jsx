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

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useUsersData = () => {
  const { t } = useTranslation();
  const [compactMode, setCompactMode] = useTableCompactMode('users');

  // State management
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [searching, setSearching] = useState(false);
  const [groupOptions, setGroupOptions] = useState([]);
  const [userCount, setUserCount] = useState(0);

  // Modal states
  const [showAddUser, setShowAddUser] = useState(false);
  const [showEditUser, setShowEditUser] = useState(false);
  const [editingUser, setEditingUser] = useState({
    id: undefined,
  });

  // Form initial values
  const formInitValues = {
    searchKeyword: '',
    searchGroup: '',
    searchRisk: '',
  };

  // Form API reference
  const [formApi, setFormApi] = useState(null);

  // Get form values helper function
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
      searchGroup: formValues.searchGroup || '',
      searchRisk: formValues.searchRisk || '',
    };
  };

  // Set user format with key field
  const setUserFormat = (users) => {
    // 后端在结果集为空时可能返回 items: null（GORM Scan + JSON 序列化默认行为）。
    // 这里统一兜底为空数组，避免 null.length 抛同步异常，进而导致外层 loading 永久卡住。
    const list = Array.isArray(users) ? users : [];
    for (let i = 0; i < list.length; i++) {
      list[i].key = list[i].id;
    }
    setUsers(list);
  };

  // Load users data
  const loadUsers = async (startIdx, pageSize, searchRisk = null) => {
    setLoading(true);
    try {
      if (searchRisk === null) {
        searchRisk = getFormValues().searchRisk;
      }
      const riskParam = searchRisk
        ? `&risk=${encodeURIComponent(searchRisk)}`
        : '';
      const res = await API.get(
        `/api/user/?p=${startIdx}&page_size=${pageSize}${riskParam}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = data.items;
        setActivePage(data.page);
        setUserCount(data.total);
        setUserFormat(newPageData);
      } else {
        showError(message);
      }
    } catch (err) {
      // 任何异常（网络/后端 4xx/5xx）都要兜底，否则按钮永久 loading
      showError(err?.message || t('加载用户列表失败'));
    } finally {
      setLoading(false);
    }
  };

  // Search users with keyword and group
  const searchUsers = async (
    startIdx,
    pageSize,
    searchKeyword = null,
    searchGroup = null,
    searchRisk = null,
  ) => {
    // If no parameters passed, get values from form
    if (searchKeyword === null || searchGroup === null || searchRisk === null) {
      const formValues = getFormValues();
      if (searchKeyword === null) searchKeyword = formValues.searchKeyword;
      if (searchGroup === null) searchGroup = formValues.searchGroup;
      if (searchRisk === null) searchRisk = formValues.searchRisk;
    }

    if (searchKeyword === '' && searchGroup === '' && searchRisk === '') {
      // If keyword is blank, load files instead
      await loadUsers(startIdx, pageSize, searchRisk);
      return;
    }
    setSearching(true);
    try {
      const riskParam = searchRisk
        ? `&risk=${encodeURIComponent(searchRisk)}`
        : '';
      // keyword 为空时仍然走 search，让后端按 group/risk 过滤；编码避免特殊字符断 URL
      const res = await API.get(
        `/api/user/search?keyword=${encodeURIComponent(
          searchKeyword,
        )}&group=${encodeURIComponent(searchGroup)}&p=${startIdx}&page_size=${pageSize}${riskParam}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = data.items;
        setActivePage(data.page);
        setUserCount(data.total);
        setUserFormat(newPageData);
      } else {
        showError(message);
      }
    } catch (err) {
      // 任何异常（网络/后端 4xx/5xx）都要兜底，否则按钮永久"查询中"
      showError(err?.message || t('搜索用户失败'));
    } finally {
      setSearching(false);
    }
  };

  // Manage user operations (promote, demote, enable, disable, delete)
  const manageUser = async (userId, action, record) => {
    // Trigger loading state to force table re-render
    setLoading(true);

    const res = await API.post('/api/user/manage', {
      id: userId,
      action,
    });

    const { success, message } = res.data;
    if (success) {
      showSuccess(t('操作成功完成！'));
      const user = res.data.data;

      // Create a new array and new object to ensure React detects changes
      const newUsers = users.map((u) => {
        if (u.id === userId) {
          if (action === 'delete') {
            return { ...u, DeletedAt: new Date() };
          }
          return { ...u, status: user.status, role: user.role };
        }
        return u;
      });

      setUsers(newUsers);
    } else {
      showError(message);
    }

    setLoading(false);
  };

  const resetUserPasskey = async (user) => {
    if (!user) {
      return;
    }
    try {
      const res = await API.delete(`/api/user/${user.id}/reset_passkey`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('Passkey 已重置'));
      } else {
        showError(message || t('操作失败，请重试'));
      }
    } catch (error) {
      showError(t('操作失败，请重试'));
    }
  };

  const resetUserTwoFA = async (user) => {
    if (!user) {
      return;
    }
    try {
      const res = await API.delete(`/api/user/${user.id}/2fa`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('二步验证已重置'));
      } else {
        showError(message || t('操作失败，请重试'));
      }
    } catch (error) {
      showError(t('操作失败，请重试'));
    }
  };

  // Handle page change
  const handlePageChange = (page) => {
    setActivePage(page);
    const { searchKeyword, searchGroup, searchRisk } = getFormValues();
    if (searchKeyword === '' && searchGroup === '' && searchRisk === '') {
      loadUsers(page, pageSize).then();
    } else {
      searchUsers(page, pageSize, searchKeyword, searchGroup, searchRisk).then();
    }
  };

  // Handle page size change
  // 修复：原实现存在两个 bug —
  //   1) 传入了 stale 的 activePage（setState 异步），实际应回到第 1 页；
  //   2) 直接调 loadUsers，会丢失 keyword/group 搜索条件（只有 risk 因为内部 getFormValues 还能保住）。
  // 这里复用与 handlePageChange / refresh 完全一致的分流逻辑，保证 page size 切换不破坏当前筛选上下文。
  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    const { searchKeyword, searchGroup, searchRisk } = getFormValues();
    try {
      if (searchKeyword === '' && searchGroup === '' && searchRisk === '') {
        await loadUsers(1, size);
      } else {
        await searchUsers(1, size, searchKeyword, searchGroup, searchRisk);
      }
    } catch (reason) {
      showError(reason);
    }
  };

  // Handle table row styling for disabled/deleted users
  const handleRow = (record, index) => {
    if (record.DeletedAt !== null || record.status !== 1) {
      return {
        style: {
          background: 'var(--semi-color-disabled-border)',
        },
      };
    } else {
      return {};
    }
  };

  // Refresh data
  const refresh = async (page = activePage) => {
    const { searchKeyword, searchGroup, searchRisk } = getFormValues();
    if (searchKeyword === '' && searchGroup === '' && searchRisk === '') {
      await loadUsers(page, pageSize);
    } else {
      await searchUsers(page, pageSize, searchKeyword, searchGroup, searchRisk);
    }
  };

  // Fetch groups data
  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      if (res === undefined) {
        return;
      }
      setGroupOptions(
        res.data.data.map((group) => ({
          label: group,
          value: group,
        })),
      );
    } catch (error) {
      showError(error.message);
    }
  };

  // Modal control functions
  const closeAddUser = () => {
    setShowAddUser(false);
  };

  const closeEditUser = () => {
    setShowEditUser(false);
    setEditingUser({
      id: undefined,
    });
  };

  // Initialize data on component mount
  useEffect(() => {
    loadUsers(0, pageSize)
      .then()
      .catch((reason) => {
        showError(reason);
      });
    fetchGroups().then();
  }, []);

  return {
    // Data state
    users,
    loading,
    activePage,
    pageSize,
    userCount,
    searching,
    groupOptions,

    // Modal state
    showAddUser,
    showEditUser,
    editingUser,
    setShowAddUser,
    setShowEditUser,
    setEditingUser,

    // Form state
    formInitValues,
    formApi,
    setFormApi,

    // UI state
    compactMode,
    setCompactMode,

    // Actions
    loadUsers,
    searchUsers,
    manageUser,
    resetUserPasskey,
    resetUserTwoFA,
    handlePageChange,
    handlePageSizeChange,
    handleRow,
    refresh,
    closeAddUser,
    closeEditUser,
    getFormValues,

    // Translation
    t,
  };
};
