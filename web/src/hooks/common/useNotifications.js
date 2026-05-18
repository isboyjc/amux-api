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

export const useNotifications = (statusState) => {
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);

  const announcements = statusState?.status?.announcements || [];

  // Helper functions
  const getAnnouncementKey = (a) =>
    `${a?.publishDate || ''}-${(a?.content || '').slice(0, 30)}`;

  const calculateUnreadCount = () => {
    if (!announcements.length) return 0;
    let readKeys = [];
    try {
      readKeys = JSON.parse(localStorage.getItem('notice_read_keys')) || [];
    } catch (_) {
      readKeys = [];
    }
    const readSet = new Set(readKeys);
    return announcements.filter((a) => !readSet.has(getAnnouncementKey(a)))
      .length;
  };

  const getUnreadKeys = () => {
    if (!announcements.length) return [];
    let readKeys = [];
    try {
      readKeys = JSON.parse(localStorage.getItem('notice_read_keys')) || [];
    } catch (_) {
      readKeys = [];
    }
    const readSet = new Set(readKeys);
    return announcements
      .filter((a) => !readSet.has(getAnnouncementKey(a)))
      .map(getAnnouncementKey);
  };

  // Effects
  useEffect(() => {
    setUnreadCount(calculateUnreadCount());
  }, [announcements]);

  // Home 页（以及任何想触发顶部公告 modal 的地方）通过 window 事件
  // 'notice:open' 打开本 hook 管控的 NoticeModal。原本 Home 自管一份本地
  // modal 但没接 noticeRaw，会出现"未登录首页自动弹是空白、Bell 点开有
  // 内容"的诡异 bug；统一到这一份 modal 后就只有一个真相源。
  useEffect(() => {
    const onOpen = () => setNoticeVisible(true);
    window.addEventListener('notice:open', onOpen);
    return () => window.removeEventListener('notice:open', onOpen);
  }, []);

  // Actions
  const handleNoticeOpen = () => {
    setNoticeVisible(true);
  };

  const handleNoticeClose = () => {
    setNoticeVisible(false);
    if (announcements.length) {
      let readKeys = [];
      try {
        readKeys = JSON.parse(localStorage.getItem('notice_read_keys')) || [];
      } catch (_) {
        readKeys = [];
      }
      const mergedKeys = Array.from(
        new Set([...readKeys, ...announcements.map(getAnnouncementKey)]),
      );
      localStorage.setItem('notice_read_keys', JSON.stringify(mergedKeys));
    }
    setUnreadCount(0);
  };

  return {
    noticeVisible,
    unreadCount,
    announcements,
    handleNoticeOpen,
    handleNoticeClose,
    getUnreadKeys,
  };
};
