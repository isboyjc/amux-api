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

import HeaderBar from './headerbar';
import { Layout } from '@douyinfe/semi-ui';
import SiderBar from './SiderBar';
import App from '../../App';
import FooterBar from './Footer';
import AnnouncementBar from '../AnnouncementBar';
import { ToastContainer } from 'react-toastify';
import ErrorBoundary from '../common/ErrorBoundary';
import React, { useContext, useEffect, useState } from 'react';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { useSidebarCollapsed } from '../../hooks/common/useSidebarCollapsed';
import { useTranslation } from 'react-i18next';
import {
  API,
  getLogo,
  getSystemName,
  showError,
  setStatusData,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useLocation } from 'react-router-dom';
import { normalizeLanguage } from '../../i18n/language';
const { Sider, Content, Header } = Layout;

const PageLayout = () => {
  const [userState, userDispatch] = useContext(UserContext);
  const [, statusDispatch] = useContext(StatusContext);
  const isMobile = useIsMobile();
  const [collapsed, , setCollapsed] = useSidebarCollapsed();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { i18n } = useTranslation();
  const location = useLocation();

  const shouldHideFooter = location.pathname.startsWith('/console') || location.pathname === '/pricing';

  const shouldInnerPadding =
    location.pathname.includes('/console') &&
    !location.pathname.startsWith('/console/chat') &&
    location.pathname !== '/console/playground';

  const isConsoleRoute = location.pathname.startsWith('/console');
  const showSider = isConsoleRoute && (!isMobile || drawerOpen);

  useEffect(() => {
    if (isMobile && drawerOpen && collapsed) {
      setCollapsed(false);
    }
  }, [isMobile, drawerOpen, collapsed, setCollapsed]);

  const loadUser = () => {
    let user = localStorage.getItem('user');
    if (user) {
      let data = JSON.parse(user);
      userDispatch({ type: 'login', payload: data });
    }
  };

  const loadStatus = async () => {
    try {
      const res = await API.get('/api/status');
      const { success, data } = res.data;
      if (success) {
        statusDispatch({ type: 'set', payload: data });
        setStatusData(data);
      } else {
        showError('Unable to connect to server');
      }
    } catch (error) {
      showError('Failed to load status');
    }
  };

  useEffect(() => {
    loadUser();
    loadStatus().catch(console.error);
    let systemName = getSystemName();
    if (systemName) {
      document.title = systemName;
    }
    let logo = getLogo();
    if (logo) {
      let linkElement = document.querySelector("link[rel~='icon']");
      if (linkElement) {
        linkElement.href = logo;
      }
    }
  }, []);

  useEffect(() => {
    let preferredLang;

    if (userState?.user?.setting) {
      try {
        const settings = JSON.parse(userState.user.setting);
        preferredLang = normalizeLanguage(settings.language);
      } catch (e) {
        // Ignore parse errors
      }
    }

    if (!preferredLang) {
      const savedLang = localStorage.getItem('i18nextLng');
      if (savedLang) {
        preferredLang = normalizeLanguage(savedLang);
      }
    }

    if (preferredLang) {
      localStorage.setItem('i18nextLng', preferredLang);
      if (preferredLang !== i18n.language) {
        i18n.changeLanguage(preferredLang);
      }
    }
  }, [i18n, userState?.user?.setting]);

  return (
    <Layout
      className='app-layout'
      style={{
        display: 'flex',
        flexDirection: 'column',
        overflow: isMobile ? 'visible' : 'hidden',
      }}
    >
      {/* 公告横幅：fixed 在最顶层，自身管理显隐。可见时通过 CSS 变量
          --announcement-bar-height 把 Header / Sider 整体下推；不可见时
          变量为 0px，下面的 top: var(...) 计算结果即为原值 */}
      <AnnouncementBar />
      <Header
        style={{
          padding: 0,
          height: 'auto',
          lineHeight: 'normal',
          position: 'fixed',
          width: '100%',
          top: 'var(--announcement-bar-height, 0px)',
          zIndex: 100,
        }}
      >
        <HeaderBar
          onMobileMenuToggle={() => setDrawerOpen((prev) => !prev)}
          drawerOpen={drawerOpen}
        />
      </Header>
      <Layout
        style={{
          overflow: isMobile ? 'visible' : 'auto',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        {showSider && (
          <Sider
            className='app-sider'
            style={{
              position: 'fixed',
              left: 0,
              // 64px = HeaderBar 高度；横幅可见时再额外下推自身高度
              top: 'calc(64px + var(--announcement-bar-height, 0px))',
              zIndex: 99,
              border: 'none',
              paddingRight: '0',
              width: 'var(--sidebar-current-width)',
            }}
          >
            <SiderBar
              onNavigate={() => {
                if (isMobile) setDrawerOpen(false);
              }}
            />
          </Sider>
        )}
        <Layout
          style={{
            marginLeft: isMobile
              ? '0'
              : showSider
                ? 'var(--sidebar-current-width)'
                : '0',
            flex: '1 1 auto',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <Content
            style={{
              flex: '1 0 auto',
              overflowY: isMobile ? 'visible' : 'hidden',
              WebkitOverflowScrolling: 'touch',
              // 上 / 右 / 下保留原有 24px（移动端 5px）；只去掉左侧 padding，
              // 让正文紧贴 Sider 右缘。Sider 自身已有右侧分隔线 / 阴影
              // 区分边界，再叠 24px 留白显得太空
              paddingTop: shouldInnerPadding ? (isMobile ? 5 : 24) : 0,
              paddingRight: shouldInnerPadding ? (isMobile ? 5 : 24) : 0,
              paddingBottom: shouldInnerPadding ? (isMobile ? 5 : 24) : 0,
              paddingLeft: 0,
              position: 'relative',
            }}
          >
            <ErrorBoundary>
              <App />
            </ErrorBoundary>
          </Content>
          {!shouldHideFooter && (
            <Layout.Footer
              style={{
                flex: '0 0 auto',
                width: '100%',
              }}
            >
              <FooterBar />
            </Layout.Footer>
          )}
        </Layout>
      </Layout>
      <ToastContainer />
    </Layout>
  );
};

export default PageLayout;
