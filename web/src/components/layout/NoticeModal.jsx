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

import React, { useEffect, useState, useContext, useMemo } from 'react';
import { Button, Modal, Empty, Spin, Tag, Timeline } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, getRelativeTime } from '../../helpers';
import { stableStringHash } from '../../helpers/utils';
import { marked } from 'marked';
import {
  IllustrationNoContent,
  IllustrationNoContentDark,
} from '@douyinfe/semi-illustrations';
import { StatusContext } from '../../context/Status';
import {
  Bell,
  Check,
  ChevronRight,
  Inbox,
  Megaphone,
  Ticket as TicketIcon,
} from 'lucide-react';
import {
  STATUS_COLOR,
  tDynamicStatusLabel,
} from '../../pages/Ticket/constants';

// 工单 tab 显示的最大条数，前后端共用（请求 page_size 和副标题文案）。
const TICKET_PAGE_SIZE = 10;

// 弹层打开期间工单列表的自动刷新周期。低于 useTicketUnread 的 60s，
// 让"打开弹层挂着"的用户也能看到管理员的新回复反映到列表里。
const TICKET_REFRESH_INTERVAL_MS = 30 * 1000;

// 状态色块映射：用 Semi 的 *-light-default + 同色实色文字，色相和 Tag 保持
// 一致（蓝=进行中、橙=等待、绿=已解决、灰=已关闭）。注意不能用标准
// Tailwind 调色板，项目 tailwind.config 把 theme.colors 替换成了仅 semi-*
// token。提到模块顶层避免每次 render 重建。
const STATUS_PILL = {
  0: 'bg-semi-color-info-light-default text-semi-color-info',
  1: 'bg-semi-color-warning-light-default text-semi-color-warning',
  2: 'bg-semi-color-success-light-default text-semi-color-success',
  3: 'bg-semi-color-fill-1 text-semi-color-text-2',
};

const getKeyForAnnouncement = (item) =>
  `${item?.publishDate || ''}-${(item?.content || '').slice(0, 30)}`;

/**
 * 左侧自绘"pill"切换按钮。提到模块顶层避免每次父级 render 都重新创建
 * 组件类型，否则 React 会把它当成全新组件触发卸载 / 重挂载。
 */
function NavRow({ item, isActive, onSelect }) {
  const Icon = item.Icon;
  // outline-none 干掉浏览器默认蓝色焦点框（Modal 打开时 Semi 焦点陷阱会
  // 自动把焦点丢给第一个可聚焦元素，看起来就像"通知行被无故选中"）；
  // focus-visible 单独保留，键盘 Tab 导航仍有可见焦点反馈，鼠标 / 程序
  // 触发的焦点不显示。
  return (
    <button
      type='button'
      onClick={() => onSelect(item.key)}
      className={
        'group relative w-full flex items-center gap-2.5 px-3 py-2.5 rounded-xl text-sm transition-all duration-150 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-semi-color-primary-light-default ' +
        (isActive
          ? 'bg-semi-color-primary-light-default text-semi-color-primary font-medium'
          : 'text-semi-color-text-2 hover:bg-semi-color-fill-1 hover:text-semi-color-text-0')
      }
    >
      <Icon
        size={16}
        className={
          isActive
            ? 'text-semi-color-primary'
            : 'text-semi-color-text-2 group-hover:text-semi-color-text-1'
        }
      />
      <span className='flex-1 text-left truncate'>{item.label}</span>
      {item.unread > 0 && (
        /*
          tailwind.config.js 把 theme.colors 整个换成 semi-* token，
          bg-red-* 不会编译出来。这里用 semi-color-danger + 内联白色文字
          保险（避免被父级 text-semi-color-primary 串色）。
        */
        <span
          className='inline-flex items-center justify-center min-w-[18px] h-[18px] px-1.5 rounded-full text-[10px] font-semibold bg-semi-color-danger'
          style={{ color: '#fff' }}
        >
          {item.unread > 99 ? '99+' : item.unread}
        </span>
      )}
    </button>
  );
}

function EmptyIllustration({ description }) {
  return (
    <div className='flex flex-col items-center justify-center py-10'>
      <Empty
        image={<IllustrationNoContent style={{ width: 110, height: 110 }} />}
        darkModeImage={
          <IllustrationNoContentDark style={{ width: 110, height: 110 }} />
        }
        description={
          <span className='text-sm text-semi-color-text-2'>{description}</span>
        }
      />
    </div>
  );
}

/**
 * 顶部 Bell 打开的"消息中心"。左侧 3 个 pill 切换（通知 / 系统公告 / 我的
 * 工单），右侧渲染对应内容。完全自绘左栏避免 Semi line-Tabs 风格陈旧。
 *
 * 站内 markdown 公告（inApp tab）由父级 useInAppNoticeUnread 透传 noticeRaw，
 * 弹层不再单独发请求；管理员的"未读"也通过该 hook 反映到 Bell 红点。
 *
 * 工单 tab 只在登录 + 工单系统开启时显示；弹层打开期间挂一个 30s 轮询，
 * 让"开着弹层不关"的用户也能看到新回复进列表。点击工单行直接 navigate
 * 详情页并关闭弹层，符合"通知 → 跳详情"的预期。
 */
const NoticeModal = ({
  visible,
  onClose,
  isMobile,
  defaultTab = 'inApp',
  unreadKeys = [],
  ticketEnabled = false,
  ticketUnread = 0,
  noticeUnread = 0,
  noticeRaw = '',
  navigate,
}) => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState(defaultTab);

  const [statusState] = useContext(StatusContext);
  const announcements = statusState?.status?.announcements || [];

  const unreadSet = useMemo(() => new Set(unreadKeys), [unreadKeys]);

  // markdown 公告 HTML 由 noticeRaw 派生，本地 useMemo 一次，避免每次 render
  // 重复 parse。
  const noticeContent = useMemo(
    () => (noticeRaw ? marked.parse(noticeRaw) : ''),
    [noticeRaw],
  );

  const processedAnnouncements = useMemo(() => {
    return (announcements || []).slice(0, 20).map((item) => {
      const pubDate = item?.publishDate ? new Date(item.publishDate) : null;
      const absoluteTime =
        pubDate && !isNaN(pubDate.getTime())
          ? `${pubDate.getFullYear()}-${String(pubDate.getMonth() + 1).padStart(2, '0')}-${String(pubDate.getDate()).padStart(2, '0')} ${String(pubDate.getHours()).padStart(2, '0')}:${String(pubDate.getMinutes()).padStart(2, '0')}`
          : item?.publishDate || '';
      return {
        key: getKeyForAnnouncement(item),
        type: item.type || 'default',
        time: absoluteTime,
        content: item.content,
        extra: item.extra,
        relative: getRelativeTime(item.publishDate),
        isUnread: unreadSet.has(getKeyForAnnouncement(item)),
      };
    });
  }, [announcements, unreadSet]);

  /**
   * "我已知晓"：把当前公告的内容指纹写入 localStorage，下次 Home 自动弹层
   * 时对比指纹一致就跳过；并 dispatch 'notice:acknowledged' 让顶部徽标
   * hook 立刻重新拉一次清零，不必等下个 tick。
   * 同时清理旧的 notice_close_date —— 该 key 是上一版本"按天压制"机制，
   * 现已废弃，顺手 removeItem 避免 localStorage 留脏。
   */
  const handleAcknowledge = () => {
    if (noticeRaw) {
      localStorage.setItem('notice_ack_hash', stableStringHash(noticeRaw));
    }
    localStorage.removeItem('notice_close_date');
    window.dispatchEvent(new CustomEvent('notice:acknowledged'));
    onClose();
  };

  // 最近工单列表。弹层打开期间挂 30s 轮询；关闭立刻丢弃 interval。
  const [tickets, setTickets] = useState([]);
  const [ticketLoading, setTicketLoading] = useState(false);

  useEffect(() => {
    if (!visible) return;
    setActiveTab(defaultTab);
  }, [defaultTab, visible]);

  useEffect(() => {
    if (!(visible && activeTab === 'tickets' && ticketEnabled)) return;
    let stopped = false;
    const fetchOnce = async (showSpinner) => {
      if (showSpinner) setTicketLoading(true);
      try {
        const res = await API.get('/api/ticket', {
          params: { page: 1, page_size: TICKET_PAGE_SIZE },
          skipErrorHandler: true,
        });
        if (stopped) return;
        if (res?.data?.success) {
          setTickets(res.data.data?.items || []);
        }
      } catch (_) {
        // 静默
      } finally {
        if (!stopped && showSpinner) setTicketLoading(false);
      }
    };
    fetchOnce(true);
    const id = setInterval(() => fetchOnce(false), TICKET_REFRESH_INTERVAL_MS);
    return () => {
      stopped = true;
      clearInterval(id);
    };
  }, [visible, activeTab, ticketEnabled]);

  // ───── 渲染 ─────

  const renderMarkdownNotice = () => {
    if (!noticeContent) {
      // 父级 hook 异步初始化时短暂无内容，给个轻量 spinner 避免一闪空态。
      if (noticeRaw === '' && noticeUnread === 0) {
        return <EmptyIllustration description={t('暂无公告')} />;
      }
      return (
        <div className='flex items-center justify-center py-16'>
          <Spin />
        </div>
      );
    }
    return (
      <div
        className='notice-content-scroll prose prose-sm dark:prose-invert max-w-none text-sm leading-relaxed text-semi-color-text-0'
        dangerouslySetInnerHTML={{ __html: noticeContent }}
      />
    );
  };

  const renderAnnouncementTimeline = () => {
    if (processedAnnouncements.length === 0) {
      return <EmptyIllustration description={t('暂无系统公告')} />;
    }
    return (
      <div className='pl-1'>
        <Timeline mode='left'>
          {processedAnnouncements.map((item, idx) => {
            const htmlContent = marked.parse(item.content || '');
            const htmlExtra = item.extra ? marked.parse(item.extra) : '';
            return (
              <Timeline.Item
                key={idx}
                type={item.type}
                time={`${item.relative ? item.relative + ' ' : ''}${item.time}`}
                extra={
                  item.extra ? (
                    <div
                      className='text-xs text-semi-color-text-2 mt-1'
                      dangerouslySetInnerHTML={{ __html: htmlExtra }}
                    />
                  ) : null
                }
              >
                <div
                  className={
                    item.isUnread
                      ? 'shine-text text-sm'
                      : 'text-sm text-semi-color-text-0'
                  }
                  dangerouslySetInnerHTML={{ __html: htmlContent }}
                />
              </Timeline.Item>
            );
          })}
        </Timeline>
      </div>
    );
  };

  const handleTicketClick = (id) => {
    onClose();
    navigate?.(`/console/ticket/${id}`);
  };

  const renderTickets = () => {
    if (ticketLoading && tickets.length === 0) {
      return (
        <div className='flex items-center justify-center py-16'>
          <Spin />
        </div>
      );
    }
    if (tickets.length === 0) {
      return <EmptyIllustration description={t('暂无工单')} />;
    }
    return (
      <div className='flex flex-col gap-1.5'>
        {tickets.map((tk) => {
          // 未读判定：管理员是最后回复方，且 user_seen_at 还停在更早的时刻。
          // 用户自己/系统回复不算未读。
          const isUnread =
            tk.last_reply_role === 1 &&
            (tk.user_seen_at || 0) < (tk.last_reply_at || 0);
          const pillCls = STATUS_PILL[tk.status] || STATUS_PILL[0];
          return (
            <button
              key={tk.id}
              type='button'
              onClick={() => handleTicketClick(tk.id)}
              className='group flex items-center gap-3 w-full px-3 py-2.5 rounded-xl hover:bg-semi-color-fill-0 transition-colors text-left outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-semi-color-primary-light-default'
            >
              <div className='relative flex-shrink-0'>
                <div
                  className={`w-9 h-9 rounded-lg flex items-center justify-center ${pillCls}`}
                >
                  <TicketIcon size={16} />
                </div>
                {isUnread && (
                  <span className='absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-semi-color-danger ring-2 ring-semi-color-bg-2' />
                )}
              </div>

              <div className='flex-1 min-w-0'>
                <div className='text-sm font-medium text-semi-color-text-0 truncate'>
                  {tk.title || `#${tk.id}`}
                </div>
                <div className='text-xs text-semi-color-text-2 mt-0.5 truncate'>
                  {tDynamicStatusLabel(t, tk.status, tk.last_reply_role, false)}
                  {' · '}
                  {getRelativeTime(
                    (tk.last_reply_at || tk.created_at) * 1000,
                  )}
                </div>
              </div>

              <ChevronRight
                size={14}
                className='flex-shrink-0 text-semi-color-text-3 group-hover:text-semi-color-text-2 transition-colors'
              />
            </button>
          );
        })}
        <div className='pt-3 mt-1 border-t border-semi-color-border flex justify-end'>
          <button
            type='button'
            onClick={() => {
              onClose();
              navigate?.('/console/ticket');
            }}
            className='text-xs text-semi-color-text-2 hover:text-semi-color-primary inline-flex items-center gap-1 transition-colors outline-none focus:outline-none focus-visible:underline'
          >
            {t('查看全部')}
            <ChevronRight size={12} />
          </button>
        </div>
      </div>
    );
  };

  // ───── 左侧切换栏 ─────

  const announcementUnreadCount = useMemo(
    () => processedAnnouncements.filter((a) => a.isUnread).length,
    [processedAnnouncements],
  );

  const navItems = useMemo(() => {
    const base = [
      {
        key: 'inApp',
        label: t('通知'),
        Icon: Bell,
        unread: noticeUnread,
      },
      {
        key: 'system',
        label: t('系统公告'),
        Icon: Megaphone,
        unread: announcementUnreadCount,
      },
    ];
    if (ticketEnabled) {
      base.push({
        key: 'tickets',
        label: t('我的工单'),
        Icon: TicketIcon,
        unread: ticketUnread,
      });
    }
    return base;
  }, [t, ticketEnabled, ticketUnread, noticeUnread, announcementUnreadCount]);

  const currentNavItem = navItems.find((n) => n.key === activeTab);

  const renderActiveBody = () => {
    if (activeTab === 'inApp') return renderMarkdownNotice();
    if (activeTab === 'system') return renderAnnouncementTimeline();
    if (activeTab === 'tickets') return renderTickets();
    return null;
  };

  return (
    <Modal
      title={null}
      header={null}
      visible={visible}
      onCancel={onClose}
      closable={false}
      footer={null}
      width={isMobile ? undefined : 760}
      size={isMobile ? 'full-width' : undefined}
      bodyStyle={{ padding: 0 }}
      className='notice-modal-modern'
    >
      {/*
        高度策略：
        - 移动端 (md-): h-full 配合 index.css 里给 .semi-modal-body 设的
          100dvh，让整个容器拿到确定高度——否则 section 的 flex-1 不会
          有界限，内容区不滚动、footer 被挤出可视区。
        - 桌面端 (md+): min 480px / max 78vh，弹层根据内容自适应，封顶
          78vh 避免遮挡太多视窗。
      */}
      <div className='flex flex-col md:flex-row h-full md:h-auto md:min-h-[480px] md:max-h-[78vh] overflow-hidden rounded-[inherit]'>
        {/* ─── 左侧：标题 + 切换栏 ─── */}
        <aside className='shrink-0 md:w-[212px] border-b md:border-b-0 md:border-r border-semi-color-border p-3 flex flex-col gap-3'>
          <div className='flex items-center gap-2 px-1.5 py-0.5'>
            <div className='w-7 h-7 rounded-lg bg-semi-color-primary-light-default flex items-center justify-center'>
              <Inbox size={15} className='text-semi-color-primary' />
            </div>
            <div className='text-sm font-semibold text-semi-color-text-0'>
              {t('消息中心')}
            </div>
          </div>
          <nav className='flex md:flex-col gap-1 overflow-x-auto md:overflow-x-visible -mx-1 px-1'>
            {navItems.map((item) => (
              <div
                key={item.key}
                className='shrink-0 md:shrink min-w-[120px] md:min-w-0 md:w-full'
              >
                <NavRow
                  item={item}
                  isActive={activeTab === item.key}
                  onSelect={setActiveTab}
                />
              </div>
            ))}
          </nav>
        </aside>

        {/* ─── 右侧：当前 tab 内容 ───
            min-h-0 关键：flex 子项默认 min-height:auto 会按内容撑开，导致
            section 实际高度超出 flex-1 预算，把外层 .semi-modal-wrap 撑出
            滚动条（整个 modal 跟着滚而不是 section 内部滚）。显式 min-h-0
            才能让 flex-1 在父容器约束下被压缩到正确高度。 */}
        <section className='flex-1 flex flex-col min-w-0 min-h-0 bg-semi-color-bg-2'>
          {/* 内容顶部 header：tab 标题 + 副标题，提供视觉锚点 */}
          <header className='flex items-start justify-between gap-3 px-4 pt-4 pb-3 border-b border-semi-color-border'>
            <div className='min-w-0'>
              <div className='text-base font-semibold text-semi-color-text-0 truncate'>
                {currentNavItem?.label}
              </div>
              <div className='text-xs text-semi-color-text-2 mt-0.5'>
                {activeTab === 'inApp' && t('最新站内通知与版本变更')}
                {activeTab === 'system' && t('运营公告与重要事件')}
                {activeTab === 'tickets' &&
                  t('最近 {{n}} 条工单状态', { n: TICKET_PAGE_SIZE })}
              </div>
            </div>
            <button
              type='button'
              onClick={onClose}
              aria-label={t('关闭')}
              className='shrink-0 w-8 h-8 inline-flex items-center justify-center rounded-lg text-semi-color-text-2 hover:bg-semi-color-fill-1 hover:text-semi-color-text-0 transition-colors outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-semi-color-primary-light-default'
            >
              <svg
                width='16'
                height='16'
                viewBox='0 0 16 16'
                fill='none'
                xmlns='http://www.w3.org/2000/svg'
              >
                <path
                  d='M4 4L12 12M12 4L4 12'
                  stroke='currentColor'
                  strokeWidth='1.5'
                  strokeLinecap='round'
                />
              </svg>
            </button>
          </header>

          {/* 滚动内容区。同样需要 min-h-0 让 flex-1 真正可以压缩，
              否则内部内容超出会把整个 section 撑高。 */}
          <div className='flex-1 min-h-0 overflow-y-auto px-4 py-3 card-content-scroll'>
            {renderActiveBody()}
          </div>

          {/* 底部 footer 仅在"通知"tab 且有公告内容时渲染，单一动作
              "我已知晓"——把当前公告的内容指纹写入 localStorage，Home 下
              次自动检查时不会再弹同一份。
              其它 tab / 无内容时 footer 整条不显示，弹层视觉更轻；关闭
              动作由右上角 X 统一承担，不需要重复的底部"关闭"按钮。 */}
          {activeTab === 'inApp' && noticeContent && (
            <footer className='flex justify-end gap-2 px-4 py-3 border-t border-semi-color-border'>
              {/*
                按钮分两态：
                - 未知晓（noticeUnread > 0）：主色按钮 + Check 图标，等待用户操作
                - 已知晓：tertiary 灰按钮 + disabled，文案改"已知晓"过去时
                判定来源是父级 hook 的 noticeUnread——它本身就读 ack_hash 决定
                是不是 0，无需在弹层再访问 localStorage。
              */}
              {noticeUnread > 0 ? (
                <Button
                  type='primary'
                  icon={<Check size={14} />}
                  onClick={handleAcknowledge}
                >
                  {t('我已知晓')}
                </Button>
              ) : (
                <Button type='tertiary' disabled icon={<Check size={14} />}>
                  {t('已知晓')}
                </Button>
              )}
            </footer>
          )}
        </section>
      </div>
    </Modal>
  );
};

export default NoticeModal;
