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
import { Tag, Space, Tooltip } from '@douyinfe/semi-ui';
import { IconHelpCircle } from '@douyinfe/semi-icons';
import {
  renderModelTag,
  stringToColor,
  calculateModelPrice,
  getModelPriceItems,
  getLobeHubIcon,
} from '../../../../../helpers';
import {
  renderLimitedItems,
  renderDescription,
} from '../../../../common/ui/RenderUtils';
import { useIsMobile } from '../../../../../hooks/common/useIsMobile';

function renderQuotaType(type, t) {
  switch (type) {
    case 1:
      return (
        <Tag color='teal' shape='circle'>
          {t('按次计费')}
        </Tag>
      );
    case 0:
      return (
        <Tag color='violet' shape='circle'>
          {t('按量计费')}
        </Tag>
      );
    default:
      return t('未知');
  }
}

// Render vendor name
const renderVendor = (vendorName, vendorIcon, t) => {
  if (!vendorName) return '-';
  return (
    <Tag
      color='white'
      shape='circle'
      prefixIcon={getLobeHubIcon(vendorIcon || 'Layers', 14)}
    >
      {vendorName}
    </Tag>
  );
};

// Render tags list using RenderUtils
const renderTags = (text) => {
  if (!text) return '-';
  const tagsArr = text.split(',').filter((tag) => tag.trim());
  return renderLimitedItems({
    items: tagsArr,
    renderItem: (tag, idx) => (
      <Tag
        key={idx}
        color={stringToColor(tag.trim())}
        shape='circle'
        size='small'
      >
        {tag.trim()}
      </Tag>
    ),
    maxDisplay: 3,
  });
};

function renderSupportedEndpoints(endpoints) {
  if (!endpoints || endpoints.length === 0) {
    return null;
  }
  return (
    <Space wrap>
      {endpoints.map((endpoint, idx) => (
        <Tag key={endpoint} color={stringToColor(endpoint)} shape='circle'>
          {endpoint}
        </Tag>
      ))}
    </Space>
  );
}

export const getPricingTableColumns = ({
  t,
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
}) => {
  const isMobile = useIsMobile();
  const priceDataCache = new WeakMap();

  const getPriceData = (record) => {
    let cache = priceDataCache.get(record);
    if (!cache) {
      cache = calculateModelPrice({
        record,
        selectedGroup,
        groupRatio,
        tokenUnit,
        displayPrice,
        currency,
        quotaDisplayType: siteDisplayType,
      });
      priceDataCache.set(record, cache);
    }
    return cache;
  };

  const endpointColumn = {
    title: t('可用端点类型'),
    dataIndex: 'supported_endpoint_types',
    render: (text, record, index) => {
      return renderSupportedEndpoints(text);
    },
  };

  const modelNameColumn = {
    title: t('模型名称'),
    dataIndex: 'model_name',
    render: (text, record, index) => {
      return renderModelTag(text, {
        onClick: () => {
          copyText(text);
        },
      });
    },
    onFilter: (value, record) =>
      record.model_name.toLowerCase().includes(value.toLowerCase()),
  };

  const quotaColumn = {
    title: t('计费类型'),
    dataIndex: 'quota_type',
    render: (text, record, index) => {
      return renderQuotaType(parseInt(text), t);
    },
    sorter: (a, b) => a.quota_type - b.quota_type,
  };

  const descriptionColumn = {
    title: t('描述'),
    dataIndex: 'description',
    render: (text) => renderDescription(text, 200),
  };

  const tagsColumn = {
    title: t('标签'),
    dataIndex: 'tags',
    render: renderTags,
  };

  const vendorColumn = {
    title: t('供应商'),
    dataIndex: 'vendor_name',
    render: (text, record) => renderVendor(text, record.vendor_icon, t),
  };

  const baseColumns = [
    modelNameColumn,
    vendorColumn,
    descriptionColumn,
    tagsColumn,
    quotaColumn,
  ];

  const ratioColumn = {
    title: () => (
      <div className='flex items-center space-x-1'>
        <span>{t('倍率')}</span>
        <Tooltip content={t('倍率是为了方便换算不同价格的模型')}>
          <IconHelpCircle
            className='text-blue-500 cursor-pointer'
            onClick={() => {
              setModalImageUrl('/ratio.png');
              setIsModalOpenurl(true);
            }}
          />
        </Tooltip>
      </div>
    ),
    dataIndex: 'model_ratio',
    render: (text, record, index) => {
      const completionRatio = parseFloat(record.completion_ratio.toFixed(3));
      const priceData = getPriceData(record);

      return (
        <div className='space-y-1'>
          <div className='text-gray-700'>
            {t('模型倍率')}：{record.quota_type === 0 ? text : t('无')}
          </div>
          <div className='text-gray-700'>
            {t('补全倍率')}：
            {record.quota_type === 0 ? completionRatio : t('无')}
          </div>
          <div className='text-gray-700'>
            {t('分组倍率')}：{priceData?.usedGroupRatio ?? '-'}
          </div>
        </div>
      );
    },
  };

  const priceColumn = {
    title: siteDisplayType === 'TOKENS' ? t('当前计费') : t('当前价格'),
    dataIndex: 'model_price',
    render: (text, record, index) => {
      const priceData = getPriceData(record);
      const priceItems = getModelPriceItems(priceData, t, siteDisplayType);
      return (
        <div className='space-y-1'>
          {priceItems.map((item) => (
            <div key={item.key} className='text-gray-700'>
              {item.label} {item.value}
              {item.suffix}
            </div>
          ))}
        </div>
      );
    },
  };

  const originalPriceColumn = {
    title: siteDisplayType === 'TOKENS' ? t('官方计费') : t('官方价格'),
    dataIndex: 'original_price',
    render: (text, record, index) => {
      const priceData = getPriceData(record);

      // 只有当倍率不为1时才显示
      if (priceData.usedGroupRatio === 1 || priceData.usedGroupRatio === undefined) {
        return '-';
      }

      const originalPriceData = calculateModelPrice({
        record,
        selectedGroup: 'all',
        groupRatio: { all: 1 },
        tokenUnit,
        displayPrice,
        currency,
        quotaDisplayType: siteDisplayType,
      });

      const priceItems = getModelPriceItems(originalPriceData, t, siteDisplayType);

      return (
        <div className='space-y-1'>
          {priceItems.map((item) => (
            <div key={item.key} className='text-gray-400 line-through text-xs'>
              {item.label} {item.value}
              {item.suffix}
            </div>
          ))}
        </div>
      );
    },
  };

  const discountColumn = {
    title: t('分组折扣'),
    dataIndex: 'discount',
    width: 200,
    ...(isMobile ? {} : { fixed: 'right' }),
    render: (text, record, index) => {
      // 获取该模型的折扣信息
      const modelEnableGroups = Array.isArray(record.enable_groups) ? record.enable_groups : [];
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

      if (groupDiscounts.length === 0) {
        return '-';
      }

      return (
        <div className='flex flex-wrap gap-1'>
          {groupDiscounts.map((item) => (
            <React.Fragment key={item.group}>
              {/* 当前用户分组的折扣 */}
              {item.showCurrentDiscount && (
                <Tag
                  size='small'
                  color={item.currentChangePercent > 0 ? 'red' : 'green'}
                  style={{ fontWeight: '600', fontSize: '11px' }}
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
                    fontSize: '11px',
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
                      fontSize: '9px',
                      fontWeight: '700',
                      letterSpacing: '0.3px',
                    }}
                  >
                    VIP
                  </span>
                  <span
                    style={{
                      padding: '2px 6px',
                      fontSize: '11px',
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
      );
    },
  };

  const columns = [...baseColumns];
  columns.push(endpointColumn);
  if (showRatio) {
    columns.push(ratioColumn);
  }
  columns.push(priceColumn);
  columns.push(originalPriceColumn);
  columns.push(discountColumn);
  return columns;
};
