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

import React, { useContext, useEffect, useState } from 'react';
import { Megaphone, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../context/Status';

// 用户关闭后落到 localStorage 的 key。比对 status.announcement_bar.version：
//   - 没存过      → 显示
//   - 存的与当前一致 → 隐藏（用户已 dismiss 当前内容）
//   - 不一致     → 显示（admin 改了内容，需要再次提示）
const DISMISSED_KEY = 'dismissed_announcement_bar_version';

// 横幅高度，要与 index.css 里 --announcement-bar-height 默认值一致
const BAR_HEIGHT_PX = 40;

// safeUrl 防御性校验：renderer 的 href 必须只允许 http(s)，其它一律 fallback
// 到 '#'，避免后端虽校验过但有人通过别的途径写入了 javascript:/data: 等
function safeUrl(raw) {
  if (typeof raw !== 'string') return '';
  const s = raw.trim();
  if (!s) return '';
  if (!/^https?:\/\//i.test(s)) return '';
  return s;
}

const AnnouncementBar = () => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  // 本地 dismissed flag——切换显隐不依赖路由 / 重渲染：
  // 用户点 X 后立刻 setHidden(true)，不再依赖 localStorage 反向同步
  const [hidden, setHidden] = useState(false);

  const ab = statusState?.status?.announcement_bar;
  const enabled = !!ab?.enabled;
  const content = (ab?.content || '').trim();
  const version = ab?.version || '';
  const link = safeUrl(ab?.link || '');
  const openInNewTab = !!ab?.open_in_new_tab;

  // 颜色变量：后端给空字符串时不写 inline style，让 CSS 默认值生效；
  // 给了 hex 就写到 :root 风格的 inline style 上覆盖默认。
  // 不在这里做 hex 校验——后端已经卡了，重复校验只会让前端跟规则
  const styleVars = {};
  if (ab?.bg_color) styleVars['--ab-bg'] = ab.bg_color;
  if (ab?.accent_color) styleVars['--ab-accent'] = ab.accent_color;
  if (ab?.text_color) styleVars['--ab-text'] = ab.text_color;

  // status 还没拉到 / 配置未启用 / 文案空 → 直接不渲染。
  // 还要看用户是否已 dismiss 当前 version
  let dismissedVersion = '';
  try {
    dismissedVersion = localStorage.getItem(DISMISSED_KEY) || '';
  } catch {
    /* ignore SSR / disabled storage */
  }
  const visible =
    enabled &&
    !!content &&
    !!version &&
    dismissedVersion !== version &&
    !hidden;

  // 同步 CSS 变量——driver 让 PageLayout 的 Header / Sider 自动让位 40px。
  // 必须用 effect：组件 mount/unmount + 显隐切换都要更新；走 documentElement
  // 是为了让 SiderBar 这种 portal 出去的元素也能读到（变量在根上）
  useEffect(() => {
    const root = document.documentElement;
    if (visible) {
      root.style.setProperty('--announcement-bar-height', `${BAR_HEIGHT_PX}px`);
    } else {
      root.style.setProperty('--announcement-bar-height', '0px');
    }
    return () => {
      // 组件卸载时复位，不在路由切换时残留
      root.style.setProperty('--announcement-bar-height', '0px');
    };
  }, [visible]);

  if (!visible) return null;

  const handleClose = (e) => {
    e.stopPropagation();
    try {
      localStorage.setItem(DISMISSED_KEY, version);
    } catch {
      /* ignore */
    }
    setHidden(true);
  };

  const handleClick = () => {
    if (!link) return;
    if (openInNewTab) {
      window.open(link, '_blank', 'noopener,noreferrer');
    } else {
      window.location.href = link;
    }
  };

  return (
    <div
      className='announcement-bar'
      role='region'
      aria-label={t('站点公告横幅')}
      style={styleVars}
    >
      <div
        className={`announcement-bar__inner${
          link ? ' announcement-bar__inner--clickable' : ''
        }`}
        onClick={link ? handleClick : undefined}
        onKeyDown={(e) => {
          if (link && (e.key === 'Enter' || e.key === ' ')) {
            e.preventDefault();
            handleClick();
          }
        }}
        role={link ? 'link' : undefined}
        tabIndex={link ? 0 : -1}
        title={content}
      >
        <Megaphone size={16} className='announcement-bar__icon' />
        <span className='announcement-bar__text'>{content}</span>
      </div>
      <button
        type='button'
        className='announcement-bar__close'
        aria-label={t('关闭公告')}
        onClick={handleClose}
      >
        <X size={14} />
      </button>
    </div>
  );
};

export default AnnouncementBar;
