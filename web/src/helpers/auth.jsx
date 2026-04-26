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

import React from 'react';
import { Navigate, useSearchParams } from 'react-router-dom';
import { history } from './history';

export function authHeader() {
  // return authorization header with jwt token
  let user = JSON.parse(localStorage.getItem('user'));

  if (user && user.token) {
    return { Authorization: 'Bearer ' + user.token };
  } else {
    return {};
  }
}

export const AuthRedirect = ({ children }) => {
  const user = localStorage.getItem('user');
  const [searchParams] = useSearchParams();
  const callback = searchParams.get('callback');

  if (user) {
    // If there's a valid callback, redirect there instead of /console
    // Must start with / but not // (protocol-relative URL) and not contain ://
    if (callback && callback.startsWith('/') && !callback.startsWith('//') && !callback.includes('://')) {
      return <Navigate to={callback} replace />;
    }
    return <Navigate to='/console' replace />;
  }

  return children;
};

// loginRedirectPath 把当前路径 + query 拼成 /login?callback=...
// LoginForm / AuthRedirect 都按 ?callback= 解析，state.from 在它们里没人读
// 是死代码——之前用户从 /console/playground?model=xxx 这种深链进来未登录
// 时会丢 query，登完直接掉到 /console。改用 ?callback 后能正确回到深链
function loginRedirectPath() {
  if (typeof window === 'undefined') return '/login';
  const cb = window.location.pathname + window.location.search;
  // 仅本站相对路径才透传，防止恶意 callback 注入外部 URL
  if (!cb || !cb.startsWith('/') || cb.startsWith('//') || cb.includes('://')) {
    return '/login';
  }
  return `/login?callback=${encodeURIComponent(cb)}`;
}

function PrivateRoute({ children }) {
  if (!localStorage.getItem('user')) {
    return <Navigate to={loginRedirectPath()} replace />;
  }
  return children;
}

export function AdminRoute({ children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to={loginRedirectPath()} replace />;
  }
  try {
    const user = JSON.parse(raw);
    if (user && typeof user.role === 'number' && user.role >= 10) {
      return children;
    }
  } catch (e) {
    // ignore
  }
  return <Navigate to='/forbidden' replace />;
}

export { PrivateRoute };
