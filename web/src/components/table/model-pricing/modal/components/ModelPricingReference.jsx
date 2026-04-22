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

import React, { useMemo } from 'react';
import { Card, Avatar, Typography, Table } from '@douyinfe/semi-ui';
import { IconCreditCard } from '@douyinfe/semi-icons';
import { parsePricingReference } from '../../../../../helpers';

const { Text } = Typography;

const ModelPricingReference = ({ modelData, t }) => {
  const ref = useMemo(
    () => parsePricingReference(modelData?.pricing_reference),
    [modelData?.pricing_reference],
  );

  if (!ref) return null;

  const columns = [
    {
      title: t('场景'),
      dataIndex: 'scenario',
      key: 'scenario',
      render: (v) => <Text>{v || '-'}</Text>,
    },
    {
      title: t('官方价'),
      dataIndex: 'official',
      key: 'official',
      render: (v) => <Text type='tertiary'>{v || '-'}</Text>,
    },
    {
      title: t('本站价'),
      dataIndex: 'ours',
      key: 'ours',
      render: (v) => (
        <Text strong style={{ color: 'var(--semi-color-success)' }}>
          {v || '-'}
        </Text>
      ),
    },
    {
      title: t('折扣'),
      dataIndex: 'discount',
      key: 'discount',
      render: (v) => (v ? <Text type='success'>{v}</Text> : <Text>-</Text>),
    },
  ];

  return (
    <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='orange' className='mr-2 shadow-md'>
          <IconCreditCard size={16} />
        </Avatar>
        <div>
          <Text className='text-lg font-medium'>{t('价格参考')}</Text>
          <div className='text-xs text-gray-600'>
            {t('此模型按 token 计费，以下为常见场景的参考价格')}
          </div>
        </div>
      </div>
      {ref.note && (
        <div className='mb-3 text-sm text-gray-600'>{ref.note}</div>
      )}
      {ref.items.length > 0 && (
        <Table
          columns={columns}
          dataSource={ref.items.map((it, idx) => ({ key: idx, ...it }))}
          pagination={false}
          size='small'
        />
      )}
    </Card>
  );
};

export default ModelPricingReference;
