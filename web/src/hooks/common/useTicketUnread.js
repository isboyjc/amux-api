/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import { useEffect, useState } from 'react';
import { API } from '../../helpers/api';

const POLL_INTERVAL_MS = 60 * 1000;

/**
 * 工单未读数轮询。原本散在 TicketButton 里，现在头部"收件箱"和 NoticeModal
 * 同时需要这个数，统一抽到 hook 避免重复轮询。
 *
 * 触发刷新的途径：
 *   1. 60s 周期；
 *   2. window 'ticket:seen' 事件 —— 详情页打开后即视为已读，立刻拉一次新
 *      未读数，体感比等下个 tick 更准。
 *
 * 工单总开关来自 localStorage 'ticket_enabled'（由 /api/status 同步过来）。
 * 关闭或用户未登录时直接返回 0，不发请求。
 */
export function useTicketUnread(isLoggedIn) {
  const ticketEnabled = localStorage.getItem('ticket_enabled') === 'true';
  const active = !!(isLoggedIn && ticketEnabled);
  const [ticketUnread, setTicketUnread] = useState(0);

  useEffect(() => {
    if (!active) {
      setTicketUnread(0);
      return;
    }
    let stopped = false;
    const tick = async () => {
      try {
        const res = await API.get('/api/ticket/unread', {
          skipErrorHandler: true,
        });
        if (stopped) return;
        if (res?.data?.success) {
          setTicketUnread(res.data.data?.count || 0);
        }
      } catch (_) {
        // 静默：未读红点不可靠不影响主流程。
      }
    };
    tick();
    const id = setInterval(tick, POLL_INTERVAL_MS);
    const onSeen = () => {
      if (!stopped) tick();
    };
    window.addEventListener('ticket:seen', onSeen);
    return () => {
      stopped = true;
      clearInterval(id);
      window.removeEventListener('ticket:seen', onSeen);
    };
  }, [active]);

  return { ticketUnread, ticketEnabled, active };
}
