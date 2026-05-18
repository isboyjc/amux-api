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
import { Badge, Button } from '@douyinfe/semi-ui';
import { Bell } from 'lucide-react';

/**
 * 顶部"收件箱"入口。
 *
 * 点击打开 NoticeModal（其中含通知 / 系统公告 / 我的工单三 tab）。徽标的
 * 总数（站内公告 + 系统公告 + 工单未读之和）由 headerbar 层算好直接透传，
 * 这里只负责渲染。
 */
const NotificationButton = ({ total, onOpen, t }) => {
  const count = total || 0;

  const buttonProps = {
    icon: <Bell size={18} />,
    'aria-label': t('通知'),
    onClick: onOpen,
    theme: 'borderless',
    type: 'tertiary',
    className:
      '!p-1.5 !text-current !bg-transparent hover:!bg-semi-color-fill-1 focus:!bg-semi-color-fill-1',
  };

  if (count > 0) {
    return (
      <Badge count={count} type='danger' overflowCount={99}>
        <Button {...buttonProps} />
      </Badge>
    );
  }
  return <Button {...buttonProps} />;
};

export default NotificationButton;
