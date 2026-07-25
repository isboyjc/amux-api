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
import { Modal, Button, Typography, Tag } from '@douyinfe/semi-ui';
import { Gift, Sparkles, Check } from 'lucide-react';
import { renderQuota } from '../../../../helpers';

const { Text } = Typography;

/**
 * 开奖揭晓弹框。
 *
 * 开盒是整个玩法里唯一一次"揭晓"时刻，用 Modal.success 的话奖金会被挤在系统图标
 * 旁边，完全没有仪式感。这里改成自绘内容 + footer=null 的受控弹框：
 * 光晕 → 礼盒弹入 → 金额放大浮现，动画定义在 index.css，且尊重 prefers-reduced-motion。
 */
const BlindBoxRevealModal = ({ visible, prize, onClose, t }) => {
  if (!prize) return null;

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={null}
      centered
      closable
      width={380}
      maskClosable={false}
      bodyStyle={{ padding: 0 }}
    >
      <div className='relative overflow-hidden rounded-xl px-6 pt-8 pb-6 text-center'>
        {/* 背景光晕 */}
        <div
          className='blindbox-glow pointer-events-none absolute left-1/2 top-4 h-40 w-40 -translate-x-1/2 rounded-full'
          style={{
            background:
              'radial-gradient(circle, rgba(245,158,11,0.38) 0%, rgba(245,158,11,0) 70%)',
          }}
          aria-hidden='true'
        />

        {/* 礼盒徽标 + 环绕星光 */}
        <div className='relative mx-auto mb-5 h-20 w-20'>
          <div
            className='blindbox-emblem flex h-20 w-20 items-center justify-center rounded-2xl shadow-lg'
            style={{
              background: 'linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%)',
            }}
          >
            <Gift size={38} color='#fff' strokeWidth={2.2} />
          </div>
          <Sparkles
            size={16}
            className='blindbox-sparkle absolute -left-1 top-1 text-amber-400'
            style={{ animationDelay: '0.1s' }}
          />
          <Sparkles
            size={13}
            className='blindbox-sparkle absolute -right-2 top-6 text-amber-300'
            style={{ animationDelay: '0.6s' }}
          />
          <Sparkles
            size={11}
            className='blindbox-sparkle absolute left-2 -bottom-1 text-amber-300'
            style={{ animationDelay: '1.1s' }}
          />
        </div>

        <Text type='tertiary' className='block text-xs'>
          {t('恭喜，你开出了')}
        </Text>

        <div
          className='blindbox-amount my-1 font-bold'
          style={{ fontSize: 40, lineHeight: 1.2, color: '#f59e0b' }}
        >
          {renderQuota(prize.prize_quota)}
        </div>

        <div className='blindbox-meta'>
          {prize.prize_name ? (
            <Tag color='amber' shape='circle' className='mt-1'>
              {prize.prize_name}
            </Tag>
          ) : null}

          <div className='mt-4 flex items-center justify-center gap-1.5'>
            <Check size={14} className='text-emerald-500' />
            <Text type='tertiary' className='text-xs'>
              {t('奖励已存入你的账户余额')}
            </Text>
          </div>

          {prize.draw_date ? (
            <Text type='tertiary' className='mt-1 block text-[11px]'>
              {t('期号')} {prize.draw_date}
            </Text>
          ) : null}

          <Button
            theme='solid'
            type='primary'
            block
            size='large'
            className='!mt-5 !bg-amber-500 hover:!bg-amber-600'
            onClick={onClose}
          >
            {t('收下奖励')}
          </Button>
        </div>
      </div>
    </Modal>
  );
};

export default BlindBoxRevealModal;
