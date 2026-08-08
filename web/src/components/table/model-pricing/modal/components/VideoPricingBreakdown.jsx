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
import { Card, Avatar, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { IconPriceTag } from '@douyinfe/semi-icons';
import { computeVideoCost, getCurrencyConfig } from '../../../../../helpers';

const { Text } = Typography;

const SAMPLE_SECONDS = 10;

/**
 * 视频模型的定价明细。与 DynamicPricingBreakdown 一样只展示基础单价，
 * 分组倍率由下方的分组价格表负责，这里仅作文字提示，避免两处口径打架。
 */
export default function VideoPricingBreakdown({
  videoPricing,
  displayPrice,
  t,
}) {
  // symbol 只用于列头单位标注；金额一律走 displayPrice，保证与同页其它价格
  // 同口径（含「按充值价显示」等设置）。displayPrice 缺失时退回纯美元。
  const { symbol } = getCurrencyConfig();
  const output = videoPricing?.output || {};
  const resolutions = Object.keys(output);

  if (resolutions.length === 0) {
    return null;
  }

  const money =
    typeof displayPrice === 'function'
      ? (usd) => displayPrice(Number(usd || 0))
      : (usd) => `$${Number(usd || 0).toFixed(4)}`;

  const videoRates =
    videoPricing?.input?.video?.per_second_by_output_resolution;
  const image = videoPricing?.input?.image;
  const audio = videoPricing?.input?.audio;

  const columns = [
    {
      title: t('分辨率'),
      dataIndex: 'resolution',
      render: (text) => (
        <Tag color='blue' size='small' shape='circle'>
          {text}
        </Tag>
      ),
    },
    {
      title: `${t('输出')} (${symbol}/${t('秒')})`,
      dataIndex: 'output',
      render: (v) => <Text strong>{money(v)}</Text>,
    },
    ...(videoRates
      ? [
          {
            title: `${t('输入视频')} (${symbol}/${t('秒')})`,
            dataIndex: 'video',
            render: (v) => (v === undefined ? '-' : money(v)),
          },
        ]
      : []),
    {
      title: `${SAMPLE_SECONDS} ${t('秒')}`,
      dataIndex: 'sample',
      render: (v) => <Text type='tertiary'>{money(v)}</Text>,
    },
  ];

  const data = resolutions.map((resolution) => ({
    key: resolution,
    resolution,
    output: output[resolution],
    video: videoRates ? videoRates[resolution] : undefined,
    sample: computeVideoCost(videoPricing, {
      resolution,
      outputSeconds: SAMPLE_SECONDS,
    }).total,
  }));

  const materialNotes = [];
  if (image && Number(image.per_image) > 0) {
    const freeCount = Number(image.free_count) || 0;
    materialNotes.push(
      freeCount > 0
        ? t('输入图片 {{price}}/张，前 {{count}} 张免费', {
            price: money(image.per_image),
            count: freeCount,
          })
        : t('输入图片 {{price}}/张', { price: money(image.per_image) }),
    );
  }
  materialNotes.push(
    audio && Number(audio.per_second) > 0
      ? t('输入音频 {{price}}/秒', { price: money(audio.per_second) })
      : t('输入音频免费'),
  );
  if (videoRates) {
    materialNotes.push(t('输入视频按其时长计费，单价取决于输出分辨率'));
  }

  return (
    <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='amber' className='mr-2 shadow-md'>
          <IconPriceTag size={16} />
        </Avatar>
        <div>
          <Text className='text-lg font-medium'>{t('视频计费')}</Text>
          <div className='text-xs text-gray-600'>
            {t('按输出分辨率与时长计费，输入素材单独计价')}
          </div>
        </div>
      </div>

      <Table
        columns={columns}
        dataSource={data}
        pagination={false}
        size='small'
        className='mb-3'
      />

      <div className='text-xs text-gray-500'>
        {materialNotes.map((note, i) => (
          <div key={i}>· {note}</div>
        ))}
        <div className='mt-2'>
          {t('以上为基础单价，实际扣费还需乘以你所在分组的倍率。')}
        </div>
      </div>
    </Card>
  );
}
