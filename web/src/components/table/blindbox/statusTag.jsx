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
import { Tag } from '@douyinfe/semi-ui';

// 中奖记录状态。与后端 model.BlindBoxStatus* 一一对应。
export const WINNER_STATUS_OPTIONS = (t) => [
  { label: t('全部状态'), value: '' },
  { label: t('已领取'), value: 'claimed' },
  { label: t('待开启'), value: 'pending' },
  { label: t('已过期'), value: 'expired' },
];

export const renderWinnerStatus = (status, t) => {
  switch (status) {
    case 'claimed':
      return (
        <Tag color='green' shape='circle'>
          {t('已领取')}
        </Tag>
      );
    case 'pending':
      return (
        <Tag color='orange' shape='circle'>
          {t('待开启')}
        </Tag>
      );
    case 'expired':
      return (
        <Tag color='grey' shape='circle'>
          {t('已过期')}
        </Tag>
      );
    default:
      return <Tag shape='circle'>{status || '-'}</Tag>;
  }
};

// 开奖记录状态。empty 分两种成因，靠 participant_count 区分后给出更具体的说明。
export const renderDrawStatus = (draw, t) => {
  // 当期是前端合成的行，没有真正结算过，单独标注
  if (draw.__current) {
    return (
      <Tag color='blue' shape='circle'>
        {t('进行中')}
      </Tag>
    );
  }
  if (draw.status === 'done') {
    return (
      <Tag color='green' shape='circle'>
        {t('已开奖')}
      </Tag>
    );
  }
  // 空期有多种成因，分开标注运营才知道该动哪个旋钮
  let reason;
  if (draw.participant_count > 0) {
    reason = t('奖池为空');
  } else if (draw.blocked_count > 0) {
    reason = t('达标者均被屏蔽');
  } else if (draw.entry_count > 0) {
    reason = t('参与者均未达标');
  } else {
    reason = t('无人参与');
  }
  return (
    <Tag color='grey' shape='circle'>
      {t('空期')} · {reason}
    </Tag>
  );
};
