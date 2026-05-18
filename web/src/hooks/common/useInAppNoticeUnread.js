/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import { useEffect, useState } from 'react';
import { API } from '../../helpers/api';
import { stableStringHash } from '../../helpers/utils';

const POLL_INTERVAL_MS = 60 * 1000;

/**
 * 站内 markdown 公告未读检测。/api/notice 只是单条 markdown，没"已读"接口；
 * 这里靠"当前内容指纹 vs localStorage.notice_ack_hash"判断算不算未读，让
 * Bell 顶部红点也能反映管理员新发的公告（不只是 Home 自动弹）。
 *
 * 触发刷新：
 *   1. 60s 周期；
 *   2. window 'notice:acknowledged' 事件 —— 用户在弹层里点了"我已知晓"，
 *      立刻拉一次让徽标清零，不必等下个 tick。
 *
 * 同时把拿到的原始 markdown 透传出去（noticeRaw），NoticeModal 可以直接
 * 复用，避免弹层打开时重复请求；空内容时返回 ''。
 */
export function useInAppNoticeUnread() {
  const [state, setState] = useState({ unread: 0, noticeRaw: '' });

  useEffect(() => {
    let stopped = false;
    const tick = async () => {
      try {
        const res = await API.get('/api/notice', { skipErrorHandler: true });
        if (stopped) return;
        const data = res?.data?.data || '';
        if (!data || !data.trim()) {
          setState({ unread: 0, noticeRaw: '' });
          return;
        }
        const ack = localStorage.getItem('notice_ack_hash');
        const unread = ack === stableStringHash(data) ? 0 : 1;
        setState({ unread, noticeRaw: data });
      } catch (_) {
        // 静默：徽标不准比起报错好。
      }
    };
    tick();
    const id = setInterval(tick, POLL_INTERVAL_MS);
    const onAck = () => {
      if (!stopped) tick();
    };
    window.addEventListener('notice:acknowledged', onAck);
    return () => {
      stopped = true;
      clearInterval(id);
      window.removeEventListener('notice:acknowledged', onAck);
    };
  }, []);

  return state;
}
