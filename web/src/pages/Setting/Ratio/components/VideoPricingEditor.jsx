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

import React, { useState } from 'react';
import {
  Banner,
  Button,
  Divider,
  Empty,
  Input,
  InputNumber,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import {
  computeVideoCostFromEditorState,
  nextVideoRowId,
} from '../../../../helpers';

const { Text } = Typography;

/** 分辨率 → 单价 的可编辑表格，输出定价与输入视频定价共用。 */
function ResolutionPriceTable({ rows, unitLabel, onChange, t }) {
  const columns = [
    {
      title: t('分辨率'),
      dataIndex: 'resolution',
      render: (text, record) => (
        <Input
          value={text}
          placeholder='2K'
          onChange={(val) =>
            onChange(
              rows.map((r) =>
                r.id === record.id ? { ...r, resolution: val } : r,
              ),
            )
          }
        />
      ),
    },
    {
      title: `${t('单价')} (${unitLabel})`,
      dataIndex: 'price',
      width: 170,
      render: (val, record) => (
        <InputNumber
          value={val}
          min={0}
          step={0.01}
          precision={4}
          prefix='$'
          onChange={(v) =>
            onChange(
              rows.map((r) =>
                r.id === record.id ? { ...r, price: v ?? 0 } : r,
              ),
            )
          }
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: t('操作'),
      width: 60,
      render: (_, record) => (
        <Button
          icon={<IconDelete />}
          type='danger'
          theme='borderless'
          size='small'
          onClick={() => onChange(rows.filter((r) => r.id !== record.id))}
        />
      ),
    },
  ];

  return (
    <>
      <Table
        dataSource={rows}
        columns={columns}
        pagination={false}
        size='small'
        rowKey='id'
        empty={<Empty description={t('未配置分辨率档位')} />}
      />
      <Button
        icon={<IconPlus />}
        size='small'
        theme='borderless'
        style={{ marginTop: 8 }}
        onClick={() =>
          onChange([
            ...rows,
            { id: nextVideoRowId(), resolution: '', price: 0 },
          ])
        }
      >
        {t('添加分辨率')}
      </Button>
    </>
  );
}

/** 计费预览：按当前配置试算一单，配错了在这里就能看出来。 */
function CostPreview({ pricing, t }) {
  const [usage, setUsage] = useState({
    resolution: '',
    outputSeconds: 10,
    imageCount: 0,
    audioSeconds: 0,
    videoSeconds: 0,
  });

  const resolutions = (pricing.outputRows || [])
    .map((r) => r.resolution)
    .filter(Boolean);
  const resolution =
    usage.resolution || pricing.defaultResolution || resolutions[0] || '';
  const result = computeVideoCostFromEditorState(pricing, {
    ...usage,
    resolution,
  });

  const field = (label, key) => (
    <div>
      <div className='mb-1 text-xs text-gray-500'>{label}</div>
      <InputNumber
        value={usage[key]}
        min={0}
        onChange={(v) => setUsage({ ...usage, [key]: v ?? 0 })}
        style={{ width: 110 }}
      />
    </div>
  );

  return (
    <div style={{ marginTop: 8 }}>
      <Space wrap align='end'>
        <div>
          <div className='mb-1 text-xs text-gray-500'>{t('分辨率')}</div>
          <Select
            value={resolution}
            onChange={(v) => setUsage({ ...usage, resolution: v })}
            style={{ width: 110 }}
            optionList={resolutions.map((r) => ({ label: r, value: r }))}
          />
        </div>
        {field(t('输出秒数'), 'outputSeconds')}
        {field(t('输入图片(张)'), 'imageCount')}
        {field(t('输入音频(秒)'), 'audioSeconds')}
        {field(t('输入视频(秒)'), 'videoSeconds')}
      </Space>

      <div style={{ marginTop: 10 }}>
        {result.error ? (
          <Text type='danger'>{result.error}</Text>
        ) : (
          <Space wrap>
            <Tag color='blue'>
              {t('输出')} ${result.output.toFixed(4)}
            </Tag>
            <Tag color='blue'>
              {t('图片')} ${result.image.toFixed(4)}
            </Tag>
            <Tag color='blue'>
              {t('音频')} ${result.audio.toFixed(4)}
            </Tag>
            <Tag color='blue'>
              {t('视频')} ${result.video.toFixed(4)}
            </Tag>
            <Text strong>
              {t('合计')} ${result.total.toFixed(4)}
            </Text>
            <Text type='tertiary' size='small'>
              {t('（最终扣费还需乘以用户所在分组倍率）')}
            </Text>
          </Space>
        )}
      </div>
    </div>
  );
}

/**
 * 单个视频模型的价目表编辑器，挂在模型定价设置的「视频计费」计费方式下，
 * 与 TieredPricingEditor 同级。
 */
export default function VideoPricingEditor({ pricing, onChange, t }) {
  if (!pricing) return null;

  const patch = (fields) => onChange({ ...pricing, ...fields });
  const resolutions = (pricing.outputRows || [])
    .map((r) => r.resolution)
    .filter(Boolean);

  return (
    <div>
      <Banner
        type='info'
        description={t(
          '这里填的是不含毛利的单价，最终扣费 = 该价目表算出的金额 × 用户所在分组倍率。',
        )}
        style={{ marginBottom: 16 }}
      />

      <div style={{ marginBottom: 16 }}>
        <div className='mb-1 text-xs text-gray-500'>{t('默认分辨率')}</div>
        <Select
          value={pricing.defaultResolution}
          onChange={(v) => patch({ defaultResolution: v })}
          style={{ width: 160 }}
          optionList={resolutions.map((r) => ({ label: r, value: r }))}
          placeholder={t('请选择')}
        />
        <div className='mt-1 text-xs text-gray-500'>
          {t('请求未指定分辨率时使用')}
        </div>
      </div>

      <Text strong>{t('输出定价')}</Text>
      <div className='mt-1 mb-2 text-xs text-gray-500'>
        {t('按输出视频时长计费，不同分辨率不同单价')}
      </div>
      <ResolutionPriceTable
        rows={pricing.outputRows || []}
        unitLabel={`$/${t('秒')}`}
        onChange={(rows) => patch({ outputRows: rows })}
        t={t}
      />

      <div style={{ marginTop: 16 }}>
        <Text>{t('输出定价（输入含参考视频）')}</Text>
        <div className='mt-1 mb-2 text-xs text-gray-500'>
          {t(
            '部分上游对含参考视频的任务整单换一档更低的单价（如火山 Seedance）。留空表示该模型没有这条规则，一律用上面的输出定价。',
          )}
        </div>
        <ResolutionPriceTable
          rows={pricing.outputWithVideoRows || []}
          unitLabel={`$/${t('秒')}`}
          onChange={(rows) => patch({ outputWithVideoRows: rows })}
          t={t}
        />
      </div>

      <div style={{ marginTop: 16 }}>
        <div className='mb-1 text-xs text-gray-500'>
          {t('时长未知时的预扣秒数')}
        </div>
        <InputNumber
          value={pricing.maxOutputSeconds}
          min={0}
          precision={0}
          suffix={t('秒')}
          onChange={(v) => patch({ maxOutputSeconds: v ?? 0 })}
          style={{ width: 170 }}
        />
        <div className='mt-1 text-xs text-gray-500'>
          {t(
            '模型自选时长（duration=-1）时按这个秒数预扣，生成完成后按上游返回的真实用量多退少补。填 0 表示该模型时长总是已知。',
          )}
        </div>
      </div>

      <Divider margin='16px' />

      <Text strong>{t('输入素材定价')}</Text>
      <Space wrap align='end' style={{ marginTop: 8, marginBottom: 16 }}>
        <div>
          <div className='mb-1 text-xs text-gray-500'>{t('输入图片')}</div>
          <InputNumber
            value={pricing.imagePerImage}
            min={0}
            step={0.01}
            precision={4}
            prefix='$'
            suffix={`/${t('张')}`}
            onChange={(v) => patch({ imagePerImage: v ?? 0 })}
            style={{ width: 170 }}
          />
        </div>
        <div>
          <div className='mb-1 text-xs text-gray-500'>{t('免费张数')}</div>
          <InputNumber
            value={pricing.imageFreeCount}
            min={0}
            precision={0}
            onChange={(v) => patch({ imageFreeCount: v ?? 0 })}
            style={{ width: 110 }}
          />
        </div>
        <div>
          <div className='mb-1 text-xs text-gray-500'>{t('输入音频')}</div>
          <InputNumber
            value={pricing.audioPerSecond}
            min={0}
            step={0.01}
            precision={4}
            prefix='$'
            suffix={`/${t('秒')}`}
            onChange={(v) => patch({ audioPerSecond: v ?? 0 })}
            style={{ width: 170 }}
          />
        </div>
      </Space>

      <div>
        <Text>{t('输入视频')}</Text>
        <div className='mt-1 mb-2 text-xs text-gray-500'>
          {t(
            '按输入视频时长计费，单价取决于输出视频的分辨率。留空或填 0 表示该模型的参考视频免费。',
          )}
        </div>
        <ResolutionPriceTable
          rows={pricing.videoRows || []}
          unitLabel={`$/${t('秒')}`}
          onChange={(rows) => patch({ videoRows: rows })}
          t={t}
        />
      </div>

      <Divider margin='16px' />

      <Text strong>{t('计费预览')}</Text>
      <CostPreview pricing={pricing} t={t} />
    </div>
  );
}
