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

import { useEffect, useRef, useState } from 'react';
import { fetchTokenInflight } from '../../helpers/token';

// 轮询间隔。后端上报间隔是 3s（service.InflightReportInterval），这里取 5s：
// 比上报慢一点，避免连续两次拉到同一份快照而白跑一趟。
const POLL_INTERVAL_MS = 5000;

/**
 * 轮询当前页令牌的在途请求数。
 *
 * 只查当页可见的令牌 id —— 令牌很多的用户翻页时不必为看不见的行付代价。
 *
 * 三条省开销的规则：
 *   1. 页面不可见（切到别的标签页）时暂停，回来立刻补一次
 *   2. 请求失败不重试、不弹错误：这是纯展示数据，网络抖一下下个周期自然恢复；
 *      弹 toast 会在断网时刷屏
 *   3. 前一次请求还没回来就跳过本轮，避免慢响应时请求堆积
 *
 * @param {number[]} tokenIds 当页令牌 id
 * @returns {{inflight: Record<number, number>, clusterWide: boolean}}
 */
export const useTokenInflight = (tokenIds) => {
  const [inflight, setInflight] = useState({});
  const [clusterWide, setClusterWide] = useState(true);

  const inFlightRequestRef = useRef(false);
  // id 列表放 ref 里给定时器读，这样翻页不会重建定时器（否则每次翻页都要
  // 等一个完整周期才出数字）。
  const idsRef = useRef(tokenIds);
  const idsKey = Array.isArray(tokenIds) ? tokenIds.join(',') : '';
  idsRef.current = tokenIds;

  useEffect(() => {
    let cancelled = false;

    // 没有可查的令牌（列表为空 / 还在加载）时根本不建定时器。
    // 早期版本在 poll 里对空列表调 setInflight({})，每 5 秒产生一个新对象引用，
    // 白白触发一轮列定义 useMemo 重算和表格重渲染。
    const ids = tokenIds;
    if (!Array.isArray(ids) || ids.length === 0) {
      setInflight((prev) => (Object.keys(prev).length === 0 ? prev : {}));
      return undefined;
    }

    const poll = async () => {
      if (inFlightRequestRef.current) return;
      inFlightRequestRef.current = true;
      try {
        const result = await fetchTokenInflight(idsRef.current);
        if (cancelled) return;
        setInflight(result.inflight);
        setClusterWide(result.clusterWide);
      } catch {
        // 静默：纯展示数据，下个周期自然恢复
      } finally {
        inFlightRequestRef.current = false;
      }
    };

    poll();
    let timer = setInterval(poll, POLL_INTERVAL_MS);

    // 后台标签页里不轮询。切回来时立刻补一次，否则用户要盯着旧数字等一个周期。
    const onVisibilityChange = () => {
      if (document.hidden) {
        clearInterval(timer);
        timer = null;
      } else if (!timer) {
        poll();
        timer = setInterval(poll, POLL_INTERVAL_MS);
      }
    };
    document.addEventListener('visibilitychange', onVisibilityChange);

    return () => {
      cancelled = true;
      if (timer) clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
    // idsKey 而不是 tokenIds：后者每次渲染都是新数组引用，会让 effect 每次
    // 重跑（等于每渲染一次就多发一个请求 + 重建定时器）。
  }, [idsKey]);

  return { inflight, clusterWide };
};
