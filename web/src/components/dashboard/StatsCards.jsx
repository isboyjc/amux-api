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
import { Card, Avatar, Skeleton, Tag } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

const StatsCards = ({
  groupedStatsData,
  loading,
  getTrendSpec,
  CARD_PROPS,
  CHART_CONFIG,
}) => {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [chartViewItems, setChartViewItems] = useState(new Set());

  const toggleItemView = (key, e) => {
    e.stopPropagation();
    setChartViewItems((prev) => {
      const next = new Set(prev);
      next.has(key) ? next.delete(key) : next.add(key);
      return next;
    });
  };

  return (
    <div className='mb-4'>
      <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4'>
        {groupedStatsData.map((group, idx) => (
          <Card
            key={idx}
            {...CARD_PROPS}
            className={`${group.color} border-0 !rounded-2xl w-full`}
            title={group.title}
          >
            <div className='space-y-4'>
              {group.items.map((item, itemIdx) => {
                const itemKey = `${idx}-${itemIdx}`;
                const showChart = chartViewItems.has(itemKey);
                const hasBreakdown =
                  item.subtitleItems &&
                  item.subtitleItems.length > 0 &&
                  !loading;

                return (
                  <div
                    key={itemIdx}
                    className='flex items-center justify-between cursor-pointer'
                    onClick={item.onClick}
                  >
                    <div className='flex items-center min-w-0'>
                      <Avatar
                        className='mr-3 flex-shrink-0'
                        size='small'
                        shape='square'
                        color={item.avatarColor}
                        style={{ borderRadius: '10px' }}
                      >
                        {item.icon}
                      </Avatar>
                      <div className='min-w-0'>
                        <div className='text-xs text-gray-500'>{item.title}</div>
                        <div className='text-lg font-semibold'>
                          <Skeleton
                            loading={loading}
                            active
                            placeholder={
                              <Skeleton.Paragraph
                                active
                                rows={1}
                                style={{
                                  width: '65px',
                                  height: '24px',
                                  marginTop: '4px',
                                }}
                              />
                            }
                          >
                            {item.value}
                          </Skeleton>
                        </div>
                      </div>
                    </div>

                    {item.title === t('当前余额') ? (
                      <Tag
                        color='white'
                        shape='circle'
                        size='large'
                        onClick={(e) => {
                          e.stopPropagation();
                          navigate('/console/topup');
                        }}
                      >
                        {t('充值')}
                      </Tag>
                    ) : hasBreakdown && !showChart ? (
                      <div
                        className='flex-shrink-0 flex items-center gap-x-2.5 cursor-pointer hover:opacity-70 transition-opacity'
                        title={t('点击查看趋势图')}
                        onClick={(e) => toggleItemView(itemKey, e)}
                      >
                        {item.subtitleItems.map((si, i) => (
                          <div key={i} className='flex flex-col items-center'>
                            <div className='text-[9px] leading-none text-gray-400 mb-0.5'>
                              {si.label}
                            </div>
                            <div className='text-[11px] leading-none font-semibold text-gray-600'>
                              {si.value}
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      (loading ||
                        (item.trendData && item.trendData.length > 0)) && (
                        <div
                          className={`w-24 h-10 ${hasBreakdown ? 'cursor-pointer hover:opacity-70 transition-opacity' : ''}`}
                          title={hasBreakdown ? t('点击查看分项数据') : undefined}
                          onClick={
                            hasBreakdown
                              ? (e) => toggleItemView(itemKey, e)
                              : undefined
                          }
                        >
                          <VChart
                            spec={getTrendSpec(item.trendData, item.trendColor)}
                            option={CHART_CONFIG}
                          />
                        </div>
                      )
                    )}
                  </div>
                );
              })}
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
};

export default StatsCards;
