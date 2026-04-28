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
import { Card, Skeleton } from '@douyinfe/semi-ui';

const PricingCardSkeleton = ({ skeletonCount = 12 }) => {
  const placeholder = (
    <div className='px-2 pt-2'>
      <div className='grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-3'>
        {Array.from({ length: skeletonCount }).map((_, index) => (
          <Card
            key={index}
            className='!rounded-2xl !border-0'
            style={{
              boxShadow:
                '0 1px 4px rgba(0,0,0,0.08), 0 0 0 1px var(--semi-color-border)',
            }}
            bodyStyle={{ padding: '14px 16px' }}
          >
            {/* 头部：图标 + 名称 + 副标题 */}
            <div className='flex items-start gap-3 mb-3'>
              <Skeleton.Avatar
                style={{ width: 44, height: 44, borderRadius: 12 }}
              />
              <div className='flex-1 min-w-0'>
                <Skeleton.Title
                  style={{
                    width: `${110 + (index % 3) * 30}px`,
                    height: 16,
                    marginBottom: 6,
                  }}
                />
                <Skeleton.Title
                  style={{ width: 120, height: 12, marginBottom: 0 }}
                />
              </div>
            </div>

            {/* 描述 */}
            <Skeleton.Paragraph rows={2} style={{ marginBottom: 12 }} title={false} />

            {/* 双列：能力 + 价格 */}
            <div className='grid grid-cols-2 gap-x-3 gap-y-2 mb-3'>
              <Skeleton.Title style={{ width: '90%', height: 12, marginBottom: 0 }} />
              <Skeleton.Title style={{ width: '90%', height: 12, marginBottom: 0 }} />
              <Skeleton.Title style={{ width: '70%', height: 12, marginBottom: 0 }} />
              <Skeleton.Title style={{ width: '70%', height: 12, marginBottom: 0 }} />
              <Skeleton.Title style={{ width: '60%', height: 12, marginBottom: 0 }} />
              <Skeleton.Title style={{ width: '60%', height: 12, marginBottom: 0 }} />
            </div>

            {/* 自定义标签 */}
            <div className='flex flex-wrap gap-1 mb-3'>
              {Array.from({ length: 2 + (index % 2) }).map((_, tagIndex) => (
                <Skeleton.Button
                  key={tagIndex}
                  size='small'
                  style={{ width: 48, height: 16, borderRadius: 10 }}
                />
              ))}
            </div>

            {/* 底部双列 */}
            <div className='grid grid-cols-2 gap-3 pt-2'>
              <Skeleton.Title style={{ width: '70%', height: 16, marginBottom: 0 }} />
              <Skeleton.Title style={{ width: '95%', height: 16, marginBottom: 0 }} />
            </div>
          </Card>
        ))}
      </div>

      {/* 分页骨架 */}
      <div className='flex justify-center mt-6 py-4 border-t pricing-pagination-divider'>
        <Skeleton.Button style={{ width: 300, height: 32 }} />
      </div>
    </div>
  );

  return <Skeleton loading={true} active placeholder={placeholder}></Skeleton>;
};

export default PricingCardSkeleton;
