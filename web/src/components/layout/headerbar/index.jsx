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

import React, { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { useHeaderBar } from '../../../hooks/common/useHeaderBar';
import { useNotifications } from '../../../hooks/common/useNotifications';
import { useTicketUnread } from '../../../hooks/common/useTicketUnread';
import { useInAppNoticeUnread } from '../../../hooks/common/useInAppNoticeUnread';
import { useNavigation } from '../../../hooks/common/useNavigation';
import NoticeModal from '../NoticeModal';
import MobileMenuButton from './MobileMenuButton';
import HeaderLogo from './HeaderLogo';
import Navigation from './Navigation';
import ActionButtons from './ActionButtons';
import { PanelLeft } from 'lucide-react';

const HeaderBar = ({ onMobileMenuToggle, drawerOpen }) => {
  const {
    userState,
    statusState,
    isMobile,
    collapsed,
    logoLoaded,
    currentLang,
    isLoading,
    systemName,
    logo,
    isNewYear,
    isSelfUseMode,
    docsLink,
    isDemoSiteMode,
    isConsoleRoute,
    theme,
    headerNavModules,
    pricingRequireAuth,
    toggleCollapsed,
    logout,
    handleLanguageChange,
    handleThemeToggle,
    handleMobileMenuToggle,
    navigate,
    t,
  } = useHeaderBar({ onMobileMenuToggle, drawerOpen });

  const {
    noticeVisible,
    unreadCount,
    handleNoticeOpen,
    handleNoticeClose,
    getUnreadKeys,
  } = useNotifications(statusState);

  // 工单未读由 useTicketUnread 统一管理（60s 轮询 + ticket:seen 事件），
  // 同时供顶部红点徽标和 NoticeModal 内的"我的工单"tab 使用。
  const { ticketUnread, ticketEnabled } = useTicketUnread(!!userState?.user);

  // 站内 markdown 公告未读（管理员主动 push 的内容）。透传出 noticeRaw 给
  // 弹层复用，省一次重复请求。
  const { unread: noticeUnread, noticeRaw } = useInAppNoticeUnread();

  const { mainNavLinks } = useNavigation(t, docsLink, headerNavModules);

  // Seedance 落地页触顶时让导航透明，以便露出全屏 hero 视频；滚动后恢复背景。
  const location = useLocation();
  const [atTop, setAtTop] = useState(true);
  useEffect(() => {
    // 桌面端页面实际滚动发生在内层 overflow:auto 容器，而非 window。
    // 用 capture 捕获任意后代滚动事件，并读取其 scrollTop 判断是否触顶。
    const onScroll = (e) => {
      const tgt = e && e.target;
      const st =
        tgt && typeof tgt.scrollTop === 'number'
          ? tgt.scrollTop
          : window.scrollY || document.documentElement.scrollTop || 0;
      setAtTop(st < 8);
    };
    window.addEventListener('scroll', onScroll, true);
    return () => window.removeEventListener('scroll', onScroll, true);
  }, []);
  const navTransparent = location.pathname === '/seedance2.0' && atTop;

  // 顶部 Bell 红点 = 三类未读之和：站内公告 + 系统公告（announcement
  // timeline）+ 工单。
  const totalUnread = (unreadCount || 0) + (noticeUnread || 0) + (ticketUnread || 0);

  // 打开弹窗时优先停在最值得看的 tab。优先级：
  //   1. inApp（管理员新公告，最 push 性）
  //   2. tickets（用户自己的事项）
  //   3. system（announcement timeline，相对被动）
  //   4. 默认 inApp
  const initialTab =
    noticeUnread > 0
      ? 'inApp'
      : ticketUnread > 0
        ? 'tickets'
        : unreadCount > 0
          ? 'system'
          : 'inApp';

  return (
    <header
      className={`text-semi-color-text-0 sticky top-0 z-50 transition-colors duration-300 ${
        navTransparent
          ? 'sd-nav-transparent'
          : 'bg-white/75 dark:bg-zinc-900/75 backdrop-blur-lg'
      }`}
    >
      <NoticeModal
        visible={noticeVisible}
        onClose={handleNoticeClose}
        isMobile={isMobile}
        defaultTab={initialTab}
        unreadKeys={getUnreadKeys()}
        ticketEnabled={ticketEnabled && !!userState?.user}
        ticketUnread={ticketUnread}
        noticeUnread={noticeUnread}
        noticeRaw={noticeRaw}
        navigate={navigate}
      />

      <div className='w-full px-4 md:px-4'>
        <div className='flex items-center justify-between h-16'>
          <div className='flex items-center'>
            <MobileMenuButton
              isConsoleRoute={isConsoleRoute}
              isMobile={isMobile}
              drawerOpen={drawerOpen}
              collapsed={collapsed}
              onToggle={handleMobileMenuToggle}
              t={t}
            />

            <HeaderLogo
              isMobile={isMobile}
              isConsoleRoute={isConsoleRoute}
              logo={logo}
              logoLoaded={logoLoaded}
              isLoading={isLoading}
              systemName={systemName}
              isSelfUseMode={isSelfUseMode}
              isDemoSiteMode={isDemoSiteMode}
              t={t}
            />

            {isConsoleRoute && !isMobile && (
              <button
                onClick={toggleCollapsed}
                className='ml-3 p-1.5 rounded-lg transition-colors duration-150'
                style={{
                  backgroundColor: 'var(--semi-color-fill-0)',
                  color: 'var(--semi-color-text-2)',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.backgroundColor = 'var(--semi-color-fill-1)';
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = 'var(--semi-color-fill-0)';
                }}
                title={collapsed ? t('展开侧边栏') : t('收起侧边栏')}
              >
                <PanelLeft size={15} />
              </button>
            )}
          </div>

          <div className='flex items-center'>
            <Navigation
              mainNavLinks={mainNavLinks}
              isMobile={isMobile}
              isLoading={isLoading}
              userState={userState}
              pricingRequireAuth={pricingRequireAuth}
            />

            <ActionButtons
            isNewYear={isNewYear}
            totalUnread={totalUnread}
            onNoticeOpen={handleNoticeOpen}
            theme={theme}
            onThemeToggle={handleThemeToggle}
            currentLang={currentLang}
            onLanguageChange={handleLanguageChange}
            userState={userState}
            isLoading={isLoading}
            isMobile={isMobile}
            isSelfUseMode={isSelfUseMode}
            logout={logout}
            navigate={navigate}
            t={t}
          />
          </div>
        </div>
      </div>
    </header>
  );
};

export default HeaderBar;
