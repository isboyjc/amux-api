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

import React, {
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowRight, X } from 'lucide-react';
import { Modal } from '@douyinfe/semi-ui';
import { StatusContext } from '../../context/Status';
import { UserContext } from '../../context/User';
import { API } from '../../helpers';

// 自动轮播间隔（毫秒）。鼠标悬浮时暂停。
const AUTO_PLAY_INTERVAL = 5000;

// dismissed 存的就是 server 下发的 version；不一致即重新展示。空字符串 →
// 用户从未关闭过。和 AnnouncementBar 同款思路
const DISMISSED_KEY = 'dismissed_sidebar_carousel_version';

// 5 套预置渐变；管理员未填 bg_url 时按 bg_preset_index 选用。索引保持稳定，
// 0..4 与后端校验范围一致。后续如需调整配色，**只能改值不能改顺序**——
// admin 已经填的 index 对应的就是某个具体渐变，乱序会换掉用户视觉
export const SIDEBAR_CAROUSEL_GRADIENTS = [
  'linear-gradient(135deg, #6366f1 0%, #8b5cf6 50%, #ec4899 100%)', // 0 紫粉
  'linear-gradient(135deg, #f59e0b 0%, #ef4444 60%, #db2777 100%)', // 1 橙红
  'linear-gradient(135deg, #0ea5e9 0%, #14b8a6 50%, #22c55e 100%)', // 2 蓝青绿
  'linear-gradient(135deg, #1e293b 0%, #334155 60%, #64748b 100%)', // 3 石板灰
  'linear-gradient(135deg, #ec4899 0%, #f97316 50%, #facc15 100%)', // 4 粉橙金
];

// 视频扩展名探测——bg_url 命中即按 <video> 渲染（自动播放、静音、循环）；
// 其它（jpg/png/webp/gif）一律按 <img> 渲染。gif 走 img 即可，无需特殊处理
const VIDEO_EXT_RE = /\.(mp4|webm|mov|ogv|m4v)(\?|#|$)/i;

const safeUrl = (raw) => (typeof raw === 'string' ? raw.trim() : '');
const isExternal = (url) => /^https?:\/\//i.test(url);

// 从内部链接里抓 ?group=... 用于命中分组权限校验。
// 外链 / 没 query / 没 group 一律返回空串——调用方据此跳过校验
const parseGroupFromInternalLink = (url) => {
  if (!url || !url.startsWith('/')) return '';
  const qIdx = url.indexOf('?');
  if (qIdx < 0) return '';
  try {
    return (new URLSearchParams(url.slice(qIdx + 1)).get('group') || '').trim();
  } catch {
    return '';
  }
};

// 把后端 item 规范化成前端用的形状，缺字段时给默认值，避免每个渲染分支
// 都做 nullish 判断。**不要**在这里把 link / bg_url 当成"必填"过滤——
// 留空有合法语义（卡片不可点、用渐变背景）
const normalizeItem = (raw, idx) => ({
  id: `slide-${idx}`,
  title: typeof raw?.title === 'string' ? raw.title : '',
  description: typeof raw?.description === 'string' ? raw.description : '',
  ctaText: typeof raw?.cta_text === 'string' ? raw.cta_text : '',
  link: typeof raw?.link === 'string' ? raw.link : '',
  openInNewTab: !!raw?.open_in_new_tab,
  bgUrl: typeof raw?.bg_url === 'string' ? raw.bg_url.trim() : '',
  bgPresetIndex:
    Number.isInteger(raw?.bg_preset_index) &&
    raw.bg_preset_index >= 0 &&
    raw.bg_preset_index < SIDEBAR_CAROUSEL_GRADIENTS.length
      ? raw.bg_preset_index
      : 0,
  overlay: raw?.overlay === 'light' ? 'light' : 'dark',
});

const SidebarCarousel = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [statusState] = useContext(StatusContext);
  const [userState] = useContext(UserContext);
  const userGroup = userState?.user?.group || '';

  const sc = statusState?.status?.sidebar_carousel;
  const enabled = !!sc?.enabled;
  const version = sc?.version || '';
  const rawItems = Array.isArray(sc?.items) ? sc.items : [];

  // 用户可用分组缓存：null = 还没拉到 / 不需要拉，Set = 已加载。
  // 拉一次就够：靠 user.group 变化作为重拉触发（登录/续登/分组变更都会触发）。
  // /api/user/self/groups 返回的 map **不含**用户自身分组（后端为令牌创建场景
  // 故意剔除），所以下面命中判断要 OR 上 userGroup
  const [usableGroups, setUsableGroups] = useState(null);

  // version 变了视作新内容——清掉本地 dismissed 标记，前端立刻重新展示。
  // 不依赖 React.state 直接读 localStorage 比对，避免 mount 之前先闪一下
  let dismissedVersion = '';
  try {
    dismissedVersion = localStorage.getItem(DISMISSED_KEY) || '';
  } catch {
    /* ignore: 隐身 / 禁用存储 */
  }

  const [activeIndex, setActiveIndex] = useState(0);
  const [paused, setPaused] = useState(false);
  // 用户点 X 后立即收起，不再依赖 localStorage 反向同步
  const [hidden, setHidden] = useState(false);
  const timerRef = useRef(null);

  // version 变更时清掉本地 hidden 状态——这样 admin 改完内容用户再次刷新
  // 时无需手动操作就能看到新轮播
  useEffect(() => {
    setHidden(false);
  }, [version]);

  const items = rawItems.map(normalizeItem);
  const total = items.length;

  const goTo = useCallback(
    (index) => {
      if (total === 0) return;
      setActiveIndex(((index % total) + total) % total);
    },
    [total],
  );

  // activeIndex 越界保护：admin 把 items 减少时旧的 index 可能超界
  useEffect(() => {
    if (activeIndex >= total) setActiveIndex(0);
  }, [activeIndex, total]);

  useEffect(() => {
    if (paused || total <= 1) return undefined;
    timerRef.current = setInterval(() => {
      setActiveIndex((prev) => (prev + 1) % total);
    }, AUTO_PLAY_INTERVAL);
    return () => clearInterval(timerRef.current);
  }, [paused, total]);

  // 拉用户可用分组：仅在已登录、轮播实际会显示时执行，避免对游客 401。
  // user.group 变了重拉一次（切账号 / 升级分组场景）。失败也写一个空 Set
  // 而非保持 null——避免点击时一直走"未加载"的乐观放行
  useEffect(() => {
    if (!enabled || total === 0) return undefined;
    if (!userGroup) {
      setUsableGroups(null);
      return undefined;
    }
    let cancelled = false;
    API.get('/api/user/self/groups')
      .then((res) => {
        if (cancelled) return;
        const data = res?.data?.success ? res.data.data : null;
        setUsableGroups(
          data && typeof data === 'object'
            ? new Set(Object.keys(data))
            : new Set(),
        );
      })
      .catch(() => {
        if (!cancelled) setUsableGroups(new Set());
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, total, userGroup]);

  // 显隐判断集中在一处，方便阅读：必须 enabled、有 item、未被当前 version
  // dismiss、且组件级未 hidden
  const visible =
    enabled &&
    total > 0 &&
    !!version &&
    dismissedVersion !== version &&
    !hidden;

  if (!visible) return null;

  const current = items[Math.min(activeIndex, total - 1)];
  const link = safeUrl(current.link);

  const handleDismiss = (e) => {
    e.stopPropagation();
    try {
      localStorage.setItem(DISMISSED_KEY, version);
    } catch {
      /* ignore */
    }
    setHidden(true);
  };

  const handleNavigate = (e) => {
    e.stopPropagation();
    if (!link) return;
    if (isExternal(link)) {
      const target = current.openInNewTab ? '_blank' : '_self';
      window.open(link, target, 'noopener,noreferrer');
      return;
    }
    // 内链：检查目标 group 是否对当前用户开放。命中规则：
    //   1) 未登录 / 链接没带 group：跳过校验，直接 navigate（让登录拦截、
    //      或目标页自身的"无可用分组"逻辑接管）；
    //   2) usableGroups 还没加载完：乐观放行，避免拉接口期间点击没反应；
    //   3) 目标 group == 用户自身分组 或 在可用列表内：放行；
    //   4) 否则弹升级提示，引导到充值页 —— 不静默失败，把"看得见、用不了"
    //      转化成升级漏斗
    const targetGroup = parseGroupFromInternalLink(link);
    if (targetGroup && userGroup && usableGroups) {
      const allowed =
        targetGroup === userGroup || usableGroups.has(targetGroup);
      if (!allowed) {
        Modal.confirm({
          title: t('该内容需要升级分组'),
          content: t(
            '当前分组「{{userGroup}}」暂时无法访问分组「{{targetGroup}}」对应的内容。累计充值满 $20 即可自动升级，解锁所有高级分组。',
            { userGroup, targetGroup },
          ),
          okText: t('前往充值'),
          cancelText: t('稍后再说'),
          onOk: () => navigate('/console/topup'),
        });
        return;
      }
    }
    navigate(link);
  };

  const handleKeyDown = (e) => {
    if (link && (e.key === 'Enter' || e.key === ' ')) {
      e.preventDefault();
      handleNavigate(e);
    }
  };

  const renderMedia = () => {
    if (current.bgUrl) {
      if (VIDEO_EXT_RE.test(current.bgUrl)) {
        return (
          <video
            className='sidebar-carousel__media'
            src={current.bgUrl}
            autoPlay
            muted
            loop
            playsInline
          />
        );
      }
      return (
        <img
          className='sidebar-carousel__media'
          src={current.bgUrl}
          alt=''
          loading='lazy'
        />
      );
    }
    // 兜底：bg_url 留空时使用预置渐变
    return (
      <div
        className='sidebar-carousel__media'
        style={{ background: SIDEBAR_CAROUSEL_GRADIENTS[current.bgPresetIndex] }}
      />
    );
  };

  return (
    <div
      className='sidebar-carousel'
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
    >
      <div
        className={`sidebar-carousel__card sidebar-carousel__card--${
          current.overlay
        }${link ? ' sidebar-carousel__card--clickable' : ''}`}
        role={link ? 'link' : undefined}
        tabIndex={link ? 0 : -1}
        onClick={link ? handleNavigate : undefined}
        onKeyDown={handleKeyDown}
        aria-label={current.title}
      >
        <div className='sidebar-carousel__bg'>{renderMedia()}</div>
        <div className='sidebar-carousel__scrim' />

        <button
          type='button'
          className='sidebar-carousel__close'
          aria-label={t('关闭宣传位')}
          onClick={handleDismiss}
        >
          <X size={12} />
        </button>

        <div className='sidebar-carousel__content'>
          <div className='sidebar-carousel__top'>
            <div className='sidebar-carousel__title'>{current.title}</div>
            {current.description && (
              <div className='sidebar-carousel__desc'>{current.description}</div>
            )}
          </div>

          <div className='sidebar-carousel__bottom'>
            <span className='sidebar-carousel__cta'>
              {current.ctaText || t('了解更多')}
              <ArrowRight size={12} className='sidebar-carousel__cta-icon' />
            </span>
            {total > 1 && (
              <div
                className='sidebar-carousel__dots'
                role='tablist'
                aria-label={t('轮播指示器')}
              >
                {items.map((s, i) => (
                  <button
                    key={s.id}
                    type='button'
                    role='tab'
                    aria-selected={i === activeIndex}
                    aria-label={t('切换到第 {{n}} 项', { n: i + 1 })}
                    className={`sidebar-carousel__dot${
                      i === activeIndex ? ' sidebar-carousel__dot--active' : ''
                    }`}
                    onClick={(e) => {
                      e.stopPropagation();
                      goTo(i);
                    }}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default SidebarCarousel;
