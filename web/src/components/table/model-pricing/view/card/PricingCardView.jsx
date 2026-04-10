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
import {
  Card,
  Tag,
  Tooltip,
  Empty,
  Button,
  Avatar,
} from '@douyinfe/semi-ui';
import { IconHelpCircle } from '@douyinfe/semi-icons';
import { Copy } from 'lucide-react';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import {
  stringToColor,
  calculateModelPrice,
  formatPriceInfo,
  getLobeHubIcon,
  getModelPriceItems,
} from '../../../../../helpers';
import PricingCardSkeleton from './PricingCardSkeleton';
import ModelHealthTimeline from '../common/ModelHealthTimeline';
import { useMinimumLoadingTime } from '../../../../../hooks/common/useMinimumLoadingTime';
import { renderLimitedItems } from '../../../../common/ui/RenderUtils';
import { useIsMobile } from '../../../../../hooks/common/useIsMobile';

const CARD_STYLES = {
  container:
    'w-12 h-12 rounded-xl flex items-center justify-center relative',
  containerStyle: {
    backgroundColor: 'var(--semi-color-fill-0)',
    border: '1px solid var(--semi-color-border)',
  },
  icon: 'w-8 h-8 flex items-center justify-center',
  default: '',
};

const isDark = () =>
  document.body.getAttribute('theme-mode') === 'dark';

const getCardShadow = (type) => {
  if (isDark()) {
    switch (type) {
      case 'hover':
        return '0 8px 25px rgba(0,0,0,0.3), 0 0 0 1px rgba(255,255,255,0.1)';
      case 'selected':
        return '0 1px 4px rgba(0,0,0,0.2), 0 0 0 2px rgba(59,130,246,0.45)';
      default:
        return '0 1px 4px rgba(0,0,0,0.2), 0 0 0 1px rgba(255,255,255,0.06)';
    }
  }
  switch (type) {
    case 'hover':
      return '0 8px 25px rgba(0,0,0,0.1), 0 4px 10px rgba(0,0,0,0.05)';
    case 'selected':
      return '0 1px 4px rgba(0,0,0,0.08), 0 0 0 2px rgba(59,130,246,0.35)';
    default:
      return '0 1px 4px rgba(0,0,0,0.08), 0 0 0 1px rgba(0,0,0,0.04)';
  }
};

const PricingCardView = ({
  filteredModels,
  loading,
  rowSelection,
  pageSize,
  setPageSize,
  currentPage,
  setCurrentPage,
  selectedGroup,
  groupRatio,
  defaultGroupRatio = {},
  vipGroupRatio = {},
  userGroup = '',
  copyText,
  setModalImageUrl,
  setIsModalOpenurl,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  showRatio,
  t,
  selectedRowKeys = [],
  setSelectedRowKeys,
  openModelDetail,
  healthData,
  usableGroup = {},
}) => {
  const showSkeleton = useMinimumLoadingTime(loading);
  const startIndex = (currentPage - 1) * pageSize;
  const paginatedModels = filteredModels.slice(
    startIndex,
    startIndex + pageSize,
  );
  const getModelKey = (model) => model.key ?? model.model_name ?? model.id;
  const isMobile = useIsMobile();

  const handleCheckboxChange = (model, checked) => {
    if (!setSelectedRowKeys) return;
    const modelKey = getModelKey(model);
    const newKeys = checked
      ? Array.from(new Set([...selectedRowKeys, modelKey]))
      : selectedRowKeys.filter((key) => key !== modelKey);
    setSelectedRowKeys(newKeys);
    rowSelection?.onChange?.(newKeys, null);
  };

  // 获取模型图标
  const getModelIcon = (model) => {
    if (!model || !model.model_name) {
      return (
        <div className={CARD_STYLES.container} style={CARD_STYLES.containerStyle}>
          <Avatar size='large'>?</Avatar>
        </div>
      );
    }
    // 1) 优先使用模型自定义图标
    if (model.icon) {
      return (
        <div className={CARD_STYLES.container} style={CARD_STYLES.containerStyle}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(model.icon, 32)}
          </div>
        </div>
      );
    }
    // 2) 退化为供应商图标
    if (model.vendor_icon) {
      return (
        <div className={CARD_STYLES.container} style={CARD_STYLES.containerStyle}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(model.vendor_icon, 32)}
          </div>
        </div>
      );
    }

    // 如果没有供应商图标，使用模型名称生成头像

    const avatarText = model.model_name.slice(0, 2).toUpperCase();
    return (
      <div className={CARD_STYLES.container}>
        <Avatar
          size='large'
          style={{
            width: 48,
            height: 48,
            borderRadius: 16,
            fontSize: 16,
            fontWeight: 'bold',
          }}
        >
          {avatarText}
        </Avatar>
      </div>
    );
  };

  // 获取模型描述
  const getModelDescription = (record) => {
    return record.description || '';
  };

  // 渲染标签
  const renderTags = (record) => {
    // 计费类型标签（左边）
    let billingTag = (
      <Tag key='billing' shape='circle' color='white' size='small'>
        -
      </Tag>
    );
    if (record.quota_type === 1) {
      billingTag = (
        <Tag key='billing' shape='circle' color='teal' size='small'>
          {t('按次计费')}
        </Tag>
      );
    } else if (record.quota_type === 0) {
      billingTag = (
        <Tag key='billing' shape='circle' color='violet' size='small'>
          {t('按量计费')}
        </Tag>
      );
    }

    // 自定义标签（右边）
    const customTags = [];
    if (record.tags) {
      const tagArr = record.tags.split(',').filter(Boolean);
      tagArr.forEach((tg, idx) => {
        customTags.push(
          <Tag
            key={`custom-${idx}`}
            shape='circle'
            color={stringToColor(tg)}
            size='small'
          >
            {tg}
          </Tag>,
        );
      });
    }

    return (
      <div className='flex items-center justify-between'>
        <div className='flex items-center gap-2'>{billingTag}</div>
        <div className='flex items-center gap-1'>
          {customTags.length > 0 &&
            renderLimitedItems({
              items: customTags.map((tag, idx) => ({
                key: `custom-${idx}`,
                element: tag,
              })),
              renderItem: (item, idx) => item.element,
              maxDisplay: 3,
            })}
        </div>
      </div>
    );
  };

  // 显示骨架屏
  if (showSkeleton) {
    return (
      <PricingCardSkeleton
        rowSelection={!!rowSelection}
        showRatio={showRatio}
      />
    );
  }

  if (!filteredModels || filteredModels.length === 0) {
    return (
      <div className='flex justify-center items-center py-20'>
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('搜索无结果')}
        />
      </div>
    );
  }

  return (
    <div className='px-2 pt-2'>
      <div className='grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-3'>
        {paginatedModels.map((model, index) => {
          const modelKey = getModelKey(model);
          const isSelected = selectedRowKeys.includes(modelKey);

          const priceData = calculateModelPrice({
            record: model,
            selectedGroup,
            groupRatio,
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          });

          // 计算原价（倍率为1）用于对比
          const originalPriceData = calculateModelPrice({
            record: model,
            selectedGroup: 'all',
            groupRatio: { all: 1 },
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          });

          // 获取该模型的折扣信息
          const modelEnableGroups = Array.isArray(model.enable_groups) ? model.enable_groups : [];
          const availableGroups = modelEnableGroups.filter(
            (g) => g !== '' && g !== 'default' && g !== 'vip' && g !== 'auto'
          );
          
          // 当选中特定分组时，只显示该分组的折扣；选择全部分组时，显示所有分组的折扣
          const groupsToShow = selectedGroup === 'all' 
            ? availableGroups 
            : availableGroups.filter((g) => g === selectedGroup);
          
          const groupDiscounts = groupsToShow.map((group) => {
            const defaultRatio = defaultGroupRatio[group] || 1;
            const vipRatio = vipGroupRatio[group] || defaultRatio;
            const currentRatio = groupRatio[group] || defaultRatio;
            
            const defaultChangePercent = ((defaultRatio - 1) * 100);
            const vipChangePercent = ((vipRatio - 1) * 100);
            
            // 判断是否为 VIP 独享折扣（VIP 倍率比 default 更优惠）
            const isVipExclusive = vipRatio < defaultRatio;
            
            // 显示逻辑：
            // 1. 未登录用户（userGroup = ''）或 default 用户：显示 default 折扣 + VIP 独享折扣（如果有）
            // 2. VIP 用户：只在 VIP 独享折扣的分组上显示 VIP 标识
            let showCurrentDiscount = false;
            let showVipDiscount = false;
            let currentChangePercent = 0;
            
            if (userGroup === 'vip') {
              // VIP 用户：只在 VIP 独享折扣上显示 VIP 标识，其他显示普通折扣
              if (isVipExclusive && currentRatio !== 1) {
                showVipDiscount = true;
              } else if (currentRatio !== 1) {
                showCurrentDiscount = true;
              }
              currentChangePercent = ((currentRatio - 1) * 100);
            } else {
              // 未登录或 default 用户：显示当前折扣 + VIP 独享折扣
              showCurrentDiscount = currentRatio !== 1;
              showVipDiscount = isVipExclusive;
              currentChangePercent = ((currentRatio - 1) * 100);
            }
            
            return {
              group,
              currentRatio,
              defaultRatio,
              vipRatio,
              currentChangePercent,
              defaultChangePercent,
              vipChangePercent,
              showCurrentDiscount,
              showVipDiscount,
            };
          }).filter((item) => item.showCurrentDiscount || item.showVipDiscount);

          // 计算当前分组的价格变化
          const shouldShowComparison = priceData.usedGroupRatio !== 1 && priceData.usedGroupRatio !== undefined;
          let priceChangePercent = 0;
          if (shouldShowComparison && priceData.usedGroupRatio) {
            priceChangePercent = ((priceData.usedGroupRatio - 1) * 100);
          }

          return (
            <Card
              key={modelKey || index}
              className={`!rounded-2xl !border-0 cursor-pointer ${CARD_STYLES.default}`}
              style={{
                boxShadow: getCardShadow('base'),
                transition: 'box-shadow 0.3s ease, transform 0.3s ease',
              }}
              bodyStyle={{ height: '100%', padding: '14px 16px' }}
              onClick={() => openModelDetail && openModelDetail(model)}
              onMouseEnter={(e) => {
                e.currentTarget.style.boxShadow = getCardShadow('hover');
                e.currentTarget.style.transform = 'translateY(-2px)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.boxShadow = getCardShadow('base');
                e.currentTarget.style.transform = 'translateY(0)';
              }}
            >
              <div className='flex flex-col h-full'>
                {/* 头部：图标 + 模型名称 + 操作按钮 */}
                <div className='flex items-start justify-between mb-2'>
                  <div className='flex items-start space-x-3 flex-1 min-w-0'>
                    {getModelIcon(model)}
                    <div className='flex-1 min-w-0'>
                      <h3
                        className='text-sm font-semibold truncate'
                        style={{ color: 'var(--semi-color-text-0)' }}
                      >
                        {model.model_name}
                      </h3>
                      <div className='flex flex-col gap-1 text-xs mt-1'>
                        {/* 价格信息（当前价格 + 官方价格对比） */}
                        <div className='flex flex-col gap-0.5'>
                          {(() => {
                            const currentPriceItems = getModelPriceItems(priceData, t, siteDisplayType);
                            const originalPriceItems = shouldShowComparison 
                              ? getModelPriceItems(originalPriceData, t, siteDisplayType)
                              : [];
                            
                            return currentPriceItems.map((item, index) => {
                              const originalItem = originalPriceItems[index];
                              return (
                                <span key={item.key} style={{ color: 'var(--semi-color-text-1)' }}>
                                  {item.label} {item.value}
                                  {shouldShowComparison && originalItem && (
                                    <span className='text-gray-400 line-through text-[10px] ml-1'>
                                      ({originalItem.value})
                                    </span>
                                  )}
                                  {item.suffix}
                                </span>
                              );
                            });
                          })()}
                        </div>
                        {/* 所有分组的折扣标签 */}
                        {groupDiscounts.length > 0 && (
                          <div className='flex flex-wrap gap-1 mt-1.5'>
                            {groupDiscounts.map((item) => (
                              <React.Fragment key={item.group}>
                                {/* 当前用户分组的折扣 */}
                                {item.showCurrentDiscount && (
                                  <Tag
                                    size='small'
                                    color={item.currentChangePercent > 0 ? 'red' : 'green'}
                                    style={{ 
                                      fontSize: '10px', 
                                      padding: '2px 6px',
                                      fontWeight: '600',
                                    }}
                                  >
                                    {item.group}: {item.currentChangePercent > 0 ? '↑' : '↓'}{' '}
                                    {Math.abs(item.currentChangePercent).toFixed(0)}%
                                  </Tag>
                                )}
                                {/* VIP 折扣对比（仅 default 用户且 VIP 有更优惠时显示） */}
                                {item.showVipDiscount && (
                                  <Tag
                                    size='small'
                                    color={item.vipChangePercent > 0 ? 'orange' : 'cyan'}
                                    style={{
                                      fontSize: '10px',
                                      padding: '0',
                                      fontWeight: '600',
                                      display: 'inline-flex',
                                      alignItems: 'center',
                                      overflow: 'hidden',
                                    }}
                                  >
                                    <span
                                      style={{
                                        backgroundColor: 'var(--semi-color-fill-1)',
                                        color: 'var(--semi-color-text-0)',
                                        padding: '2px 4px',
                                        fontSize: '8px',
                                        fontWeight: '700',
                                        letterSpacing: '0.3px',
                                      }}
                                    >
                                      VIP
                                    </span>
                                    <span
                                      style={{
                                        padding: '2px 6px',
                                        fontSize: '10px',
                                        fontWeight: '600',
                                      }}
                                    >
                                      {item.group}: {item.vipChangePercent > 0 ? '↑' : '↓'}{' '}
                                      {Math.abs(item.vipChangePercent).toFixed(0)}%
                                    </span>
                                  </Tag>
                                )}
                              </React.Fragment>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>

                  <div className='ml-2 flex-shrink-0'>
                    <Tooltip content={t('复制')} position='top' showArrow={false}>
                      <Button
                        size='small'
                        theme='borderless'
                        type='tertiary'
                        icon={<Copy size={13} />}
                        style={{
                          width: 28,
                          height: 28,
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          color: 'var(--semi-color-text-2)',
                        }}
                        onClick={(e) => {
                          e.stopPropagation();
                          copyText(model.model_name);
                        }}
                      />
                    </Tooltip>
                  </div>
                </div>

                {/* 模型描述 - 占据剩余空间 */}
                <div className='flex-1 mb-2'>
                  <p
                    className='text-xs line-clamp-1 leading-relaxed'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {getModelDescription(model)}
                  </p>
                </div>

                {/* 健康状态时间线 */}
                {healthData && (
                  <div className='mb-2'>
                    <ModelHealthTimeline
                      healthData={healthData}
                      modelName={model.model_name}
                      groups={
                        model.enable_groups
                          ? model.enable_groups.filter(
                              (g) => usableGroup[g],
                            )
                          : undefined
                      }
                      compact={true}
                      t={t}
                    />
                  </div>
                )}

                {/* 底部区域 */}
                <div className='mt-auto'>
                  {/* 标签区域 */}
                  {renderTags(model)}

                  {/* 倍率信息（可选） */}
                  {showRatio && (
                    <div className='pt-3'>
                      <div className='flex items-center space-x-1 mb-2'>
                        <span
                          className='text-xs font-medium'
                          style={{ color: 'var(--semi-color-text-1)' }}
                        >
                          {t('倍率信息')}
                        </span>
                        <Tooltip
                          content={t('倍率是为了方便换算不同价格的模型')}
                        >
                          <IconHelpCircle
                            className='text-blue-500 cursor-pointer'
                            size='small'
                            onClick={(e) => {
                              e.stopPropagation();
                              setModalImageUrl('/ratio.png');
                              setIsModalOpenurl(true);
                            }}
                          />
                        </Tooltip>
                      </div>
                      <div
                        className='grid grid-cols-3 gap-2 text-xs'
                        style={{ color: 'var(--semi-color-text-2)' }}
                      >
                        <div>
                          {t('模型')}:{' '}
                          {model.quota_type === 0 ? model.model_ratio : t('无')}
                        </div>
                        <div>
                          {t('补全')}:{' '}
                          {model.quota_type === 0
                            ? parseFloat(model.completion_ratio.toFixed(3))
                            : t('无')}
                        </div>
                        <div>
                          {t('分组')}: {priceData?.usedGroupRatio ?? '-'}
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </Card>
          );
        })}
      </div>

    </div>
  );
};

export default PricingCardView;
