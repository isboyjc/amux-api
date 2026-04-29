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
import { useNavigate } from 'react-router-dom';
import {
  Card,
  Tag,
  Tooltip,
  Empty,
  Avatar,
  Modal,
} from '@douyinfe/semi-ui';
import { IconCreditCard } from '@douyinfe/semi-icons';
import {
  Type,
  Image as ImageIcon,
  AudioLines,
  Video,
  FileText,
  Boxes,
  ArrowUpDown,
} from 'lucide-react';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import {
  calculateModelPrice,
  getLobeHubIcon,
  parsePricingReference,
  formatGroupDiscount,
  buildPlaygroundDeepLink,
  MODEL_INPUT_CAPABILITIES,
  MODEL_OUTPUT_CAPABILITIES,
} from '../../../../../helpers';
import PricingCardSkeleton from './PricingCardSkeleton';
import ModelHealthTimeline from '../common/ModelHealthTimeline';
import { useMinimumLoadingTime } from '../../../../../hooks/common/useMinimumLoadingTime';
import { renderLimitedItems } from '../../../../common/ui/RenderUtils';

const CARD_STYLES = {
  container:
    'w-11 h-11 rounded-xl flex items-center justify-center relative flex-shrink-0',
  containerStyle: {
    backgroundColor: 'var(--semi-color-fill-0)',
    border: '1px solid var(--semi-color-border)',
  },
  icon: 'w-7 h-7 flex items-center justify-center',
};

const CAPABILITY_ICONS = {
  text: Type,
  image: ImageIcon,
  audio: AudioLines,
  video: Video,
  file: FileText,
  embedding: Boxes,
  rerank: ArrowUpDown,
};

// 工厂函数：让 i18next-cli 提取器能识别所有能力标签 key
const buildCapabilityLabels = (t) => ({
  text: t('文本'),
  image: t('图片'),
  audio: t('音频'),
  video: t('视频'),
  file: t('文件'),
  embedding: t('向量嵌入'),
  rerank: t('重排'),
});

const isDark = () =>
  document.body.getAttribute('theme-mode') === 'dark';

const getCardShadow = (type) => {
  if (isDark()) {
    switch (type) {
      case 'hover':
        return '0 4px 14px rgba(0,0,0,0.28), 0 0 0 1px rgba(255,255,255,0.08)';
      case 'selected':
        return '0 1px 4px rgba(0,0,0,0.2), 0 0 0 2px rgba(59,130,246,0.45)';
      default:
        return '0 1px 4px rgba(0,0,0,0.2), 0 0 0 1px rgba(255,255,255,0.06)';
    }
  }
  switch (type) {
    case 'hover':
      return '0 4px 12px rgba(0,0,0,0.06), 0 1px 4px rgba(0,0,0,0.04)';
    case 'selected':
      return '0 1px 4px rgba(0,0,0,0.08), 0 0 0 2px rgba(59,130,246,0.35)';
    default:
      return '0 1px 4px rgba(0,0,0,0.08), 0 0 0 1px rgba(0,0,0,0.04)';
  }
};

// 单个能力图标，亮/灰区分
const CapabilityIcon = ({ type, active, label, t }) => {
  const Icon = CAPABILITY_ICONS[type];
  if (!Icon) return null;
  return (
    <Tooltip content={label} position='top' showArrow={false}>
      <span
        className='inline-flex items-center justify-center rounded'
        style={{
          width: 18,
          height: 18,
          backgroundColor: active
            ? 'var(--semi-color-primary-light-default)'
            : 'transparent',
          color: active
            ? 'var(--semi-color-primary)'
            : 'var(--semi-color-text-3)',
          opacity: active ? 1 : 0.45,
        }}
      >
        <Icon size={11} strokeWidth={2.2} />
      </span>
    </Tooltip>
  );
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
  actuallyUsableGroups = null,
}) => {
  const navigate = useNavigate();
  const showSkeleton = useMinimumLoadingTime(loading);
  const startIndex = (currentPage - 1) * pageSize;
  const paginatedModels = filteredModels.slice(
    startIndex,
    startIndex + pageSize,
  );
  const capabilityLabels = buildCapabilityLabels(t);

  // 每张卡片的"激活分组"覆盖。key 为模型唯一标识，value 为分组名。
  // 未设置 / 'all' 时按 calculateModelPrice 的逻辑选最低倍率分组。
  // 全局 selectedGroup 不为 'all' 时优先级更高，per-card 覆盖被忽略。
  const [cardGroupOverride, setCardGroupOverride] = useState({});

  const toggleCardGroup = (modelKey, group) => {
    setCardGroupOverride((prev) => {
      const next = { ...prev };
      if (prev[modelKey] === group) {
        delete next[modelKey];
      } else {
        next[modelKey] = group;
      }
      return next;
    });
  };

  // 当前用户能否真正使用该分组：
  //   - 未登录（userGroup 空）：跳过校验，让 playground 自身的登录拦截兜底
  //   - 已登录但 actuallyUsableGroups 还没拉到：乐观放行
  //   - 已加载：以 Set 为准
  const isGroupUsable = (group) => {
    if (!group) return true;
    if (!userGroup) return true;
    if (!actuallyUsableGroups) return true;
    return actuallyUsableGroups.has(group);
  };

  // 不可用渠道分组弹升级提示，引导到充值页
  const showUpgradeModal = (targetGroup) => {
    Modal.confirm({
      title: t('该渠道分组需要升级用户分组'),
      content: t(
        '当前用户分组「{{userGroup}}」暂时无法访问「{{targetGroup}}」渠道分组对应的内容。累计充值满 $20 即可自动升级「VIP」用户分组，解锁所有高级稳定渠道分组以及更优惠的「special/**」渠道分组折扣。',
        { userGroup, targetGroup },
      ),
      okText: t('前往充值'),
      cancelText: t('稍后再说'),
      onOk: () => navigate('/console/topup'),
    });
  };

  // 模型图标（自定义图标 > 厂商图标 > 名称缩写）
  const renderModelAvatar = (model) => {
    if (!model || !model.model_name) {
      return (
        <div className={CARD_STYLES.container} style={CARD_STYLES.containerStyle}>
          <Avatar size='small'>?</Avatar>
        </div>
      );
    }
    if (model.icon) {
      return (
        <div className={CARD_STYLES.container} style={CARD_STYLES.containerStyle}>
          <div className={CARD_STYLES.icon}>{getLobeHubIcon(model.icon, 28)}</div>
        </div>
      );
    }
    if (model.vendor_icon) {
      return (
        <div className={CARD_STYLES.container} style={CARD_STYLES.containerStyle}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(model.vendor_icon, 28)}
          </div>
        </div>
      );
    }
    const avatarText = model.model_name.slice(0, 2).toUpperCase();
    return (
      <div className={CARD_STYLES.container}>
        <Avatar
          size='small'
          style={{
            width: 44,
            height: 44,
            borderRadius: 12,
            fontSize: 14,
            fontWeight: 'bold',
          }}
        >
          {avatarText}
        </Avatar>
      </div>
    );
  };

  // 计费类型标签：按次/按量统一为低饱和的主题色 chip，仅文本区分
  // 动态计费用 warning 色和按次/按量区分；多档/时间条件/请求条件以中性 chip 补充
  const renderBillingTag = (model) => {
    if (model.billing_mode === 'tiered_expr' && model.billing_expr) {
      return <PlainChip warning>{t('动态计费')}</PlainChip>;
    }
    if (model.quota_type === 1) {
      return <PlainChip accent>{t('按次计费')}</PlainChip>;
    }
    if (model.quota_type === 0) {
      return <PlainChip accent>{t('按量计费')}</PlainChip>;
    }
    return <PlainChip>-</PlainChip>;
  };

  // 动态计费的辅助标识：档位数 / 时间条件 / 请求条件
  const renderDynamicAuxChips = (model) => {
    if (model.billing_mode !== 'tiered_expr' || !model.billing_expr) return null;
    const exprBody = model.billing_expr.replace(/^v\d+:/, '');
    const tierCount = (exprBody.match(/tier\(/g) || []).length;
    const hasTimeCondition = /\b(?:hour|minute|weekday|month|day)\(/.test(exprBody);
    const hasRequestCondition = /\b(?:param|header)\(/.test(exprBody);
    const chips = [];
    if (tierCount > 1) chips.push(`${tierCount}${t('档')}`);
    if (hasTimeCondition) chips.push(t('含时间条件'));
    if (hasRequestCondition) chips.push(t('含请求条件'));
    return chips.map((label) => <PlainChip key={label}>{label}</PlainChip>);
  };

  // 标签行（计费类型 + 自定义标签合并到一行；自定义标签统一中性色）
  const renderTagsRow = (model) => {
    const tagArr = model.tags
      ? model.tags.split(',').filter(Boolean)
      : [];
    const customTags = tagArr.map((tg, idx) => (
      <PlainChip key={`custom-${idx}`}>{tg}</PlainChip>
    ));
    return (
      <div className='flex items-center flex-wrap gap-1'>
        {renderBillingTag(model)}
        {renderDynamicAuxChips(model)}
        {tagArr.length > 0 &&
          renderLimitedItems({
          items: customTags.map((tag, idx) => ({
            key: `custom-${idx}`,
            element: tag,
          })),
          renderItem: (item) => item.element,
          maxDisplay: 4,
        })}
      </div>
    );
  };

  // 取按量计费下的展示用值（价格字符串或倍率数值），缺失返回 null
  const getPerTokenDisplay = (priceData, key) => {
    if (!priceData || !priceData.isPerToken) return null;
    if (priceData.isTokensDisplay) {
      const map = {
        input: priceData.inputRatio,
        completion: priceData.completionRatio,
        cache: priceData.cacheRatio,
        createCache: priceData.createCacheRatio,
      };
      return map[key] !== undefined && map[key] !== null
        ? `${map[key]}x`
        : null;
    }
    const map = {
      input: priceData.inputPrice,
      completion: priceData.completionPrice,
      cache: priceData.cachePrice,
      createCache: priceData.createCachePrice,
    };
    return map[key] || null;
  };

  // 价格单位后缀
  const getPriceSuffix = (priceData) => {
    if (!priceData) return '';
    if (priceData.isTokensDisplay) return '';
    return ` / 1${priceData.unitLabel || 'M'} Tokens`;
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
          const modelKey = model.key || model.model_name || index;
          // 全局选了具体分组时优先；否则用 per-card 覆盖；最后回退到 'all'（自动选最低）
          const effectiveGroup =
            selectedGroup !== 'all'
              ? selectedGroup
              : cardGroupOverride[modelKey] || 'all';
          const allowCardSwitch = selectedGroup === 'all';

          const priceData = calculateModelPrice({
            record: model,
            selectedGroup: effectiveGroup,
            groupRatio,
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          });

          // 原价（倍率 1）用于折扣对比展示
          const originalPriceData = calculateModelPrice({
            record: model,
            selectedGroup: 'all',
            groupRatio: { all: 1 },
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          });
          const showPriceCompare =
            priceData.usedGroupRatio !== undefined &&
            priceData.usedGroupRatio !== 1;

          const isPerCall = model.quota_type === 1;
          const priceSuffix = getPriceSuffix(priceData);

          // 输入/输出能力集合（后端 ResolveCapabilities 已解析，前端只渲染）
          const inputCaps = new Set(model.input_modalities || []);
          const outputCaps = new Set(model.output_modalities || []);

          // 分组折扣（紧凑格式）
          const modelEnableGroups = Array.isArray(model.enable_groups)
            ? model.enable_groups
            : [];
          const availableGroups = modelEnableGroups.filter(
            (g) => g !== '' && g !== 'default' && g !== 'vip' && g !== 'auto',
          );
          const groupsToShow =
            selectedGroup === 'all'
              ? availableGroups
              : availableGroups.filter((g) => g === selectedGroup);
          const groupDiscounts = groupsToShow
            .map((group) => {
              const defaultRatio = defaultGroupRatio[group] || 1;
              const vipRatio = vipGroupRatio[group] || defaultRatio;
              const currentRatio = groupRatio[group] || defaultRatio;
              const isVipExclusive = vipRatio < defaultRatio;
              const isVipUser = userGroup === 'vip';
              const useRatio = isVipUser
                ? currentRatio
                : currentRatio !== 1
                  ? currentRatio
                  : isVipExclusive
                    ? vipRatio
                    : currentRatio;
              const discount = formatGroupDiscount(useRatio, t);
              if (!discount) return null;
              return {
                group,
                ratio: useRatio,
                discount,
              };
            })
            .filter(Boolean)
            // 按倍率从小到大（折扣力度从大到小）排序：低价分组在前
            .sort((a, b) => a.ratio - b.ratio);

          const referenceData = parsePricingReference(model.pricing_reference);
          const referenceItems = referenceData?.items || [];

          return (
            <Card
              key={model.key || model.model_name || index}
              className={`!rounded-2xl !border-0 cursor-pointer group`}
              style={{
                boxShadow: getCardShadow('base'),
                transition: 'box-shadow 0.2s ease',
              }}
              bodyStyle={{ height: '100%', padding: '14px 16px' }}
              onClick={() => openModelDetail && openModelDetail(model)}
              onMouseEnter={(e) => {
                e.currentTarget.style.boxShadow = getCardShadow('hover');
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.boxShadow = getCardShadow('base');
              }}
            >
              <div className='flex flex-col h-full'>
                {/* 头部：图标 + 名称(可点击复制) + 副标题 + 右上预留 */}
                <div className='flex items-start justify-between gap-2 mb-2'>
                  <div className='flex items-start gap-3 flex-1 min-w-0'>
                    {renderModelAvatar(model)}
                    <div className='flex-1 min-w-0'>
                      <h3
                        className='text-sm font-semibold truncate'
                        style={{ color: 'var(--semi-color-text-0)' }}
                      >
                        <Tooltip
                          content={t('点击复制模型名称')}
                          position='top'
                          showArrow={false}
                        >
                          <span
                            className='cursor-pointer hover:underline'
                            onClick={(e) => {
                              e.stopPropagation();
                              copyText(model.model_name);
                            }}
                          >
                            {model.model_name}
                          </span>
                        </Tooltip>
                      </h3>
                      {model.vendor_name && (
                        <div
                          className='text-xs mt-0.5 truncate'
                          style={{ color: 'var(--semi-color-text-2)' }}
                        >
                          {model.vendor_name}
                        </div>
                      )}
                    </div>
                  </div>
                  {/* 右上：悬停展示 Chat 跳转按钮（在 Playground 中聊天） */}
                  <div
                    className='flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity duration-150'
                    style={{ minWidth: 58, height: 28 }}
                  >
                    <Tooltip
                      content={t('在 Playground 中聊天')}
                      position='left'
                      showArrow={false}
                    >
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          const targetGroup = priceData.usedGroup;
                          if (
                            targetGroup &&
                            !isGroupUsable(targetGroup)
                          ) {
                            showUpgradeModal(targetGroup);
                            return;
                          }
                          const url = buildPlaygroundDeepLink({
                            model: model.model_name,
                            group: targetGroup,
                          });
                          if (url) {
                            window.open(url, '_blank', 'noopener,noreferrer');
                          }
                        }}
                        style={{
                          height: 28,
                          padding: '0 14px',
                          border: 'none',
                          borderRadius: 8,
                          background: 'var(--semi-color-text-0)',
                          color: 'var(--semi-color-bg-0)',
                          cursor: 'pointer',
                          fontSize: 12,
                          fontWeight: 600,
                          letterSpacing: 0.2,
                          transition: 'opacity 0.15s ease',
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.opacity = '0.85';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.opacity = '1';
                        }}
                      >
                        {t('聊天')}
                      </button>
                    </Tooltip>
                  </div>
                </div>

                {/* 描述（无内容时不留空间） */}
                {model.description && (
                  <p
                    className='text-xs line-clamp-2 leading-relaxed mb-3'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {model.description}
                  </p>
                )}

                {/* 双列：能力 + 价格 */}
                <div
                  className='grid grid-cols-2 gap-x-3 gap-y-1 text-xs mb-3'
                  style={{ color: 'var(--semi-color-text-1)' }}
                >
                  {/* 输入列 */}
                  <div className='min-w-0 space-y-1'>
                    <div className='flex items-center gap-1.5'>
                      <span style={{ color: 'var(--semi-color-text-2)' }}>
                        {t('输入类型')}:
                      </span>
                      <div className='flex items-center gap-0.5'>
                        {MODEL_INPUT_CAPABILITIES.map((cap) => (
                          <CapabilityIcon
                            key={cap}
                            type={cap}
                            active={inputCaps.has(cap)}
                            label={capabilityLabels[cap]}
                            t={t}
                          />
                        ))}
                      </div>
                    </div>
                    {!isPerCall && (
                      <>
                        <PriceLine
                          label={t('输入')}
                          value={getPerTokenDisplay(priceData, 'input')}
                          originalValue={
                            showPriceCompare
                              ? getPerTokenDisplay(originalPriceData, 'input')
                              : null
                          }
                          suffix={priceSuffix}
                        />
                        <PriceLine
                          label={t('缓存读取')}
                          value={getPerTokenDisplay(priceData, 'cache')}
                          originalValue={
                            showPriceCompare
                              ? getPerTokenDisplay(originalPriceData, 'cache')
                              : null
                          }
                          suffix={priceSuffix}
                        />
                      </>
                    )}
                  </div>
                  {/* 输出列 */}
                  <div className='min-w-0 space-y-1'>
                    <div className='flex items-center gap-1.5'>
                      <span style={{ color: 'var(--semi-color-text-2)' }}>
                        {t('输出类型')}:
                      </span>
                      <div className='flex items-center gap-0.5'>
                        {MODEL_OUTPUT_CAPABILITIES.map((cap) => (
                          <CapabilityIcon
                            key={cap}
                            type={cap}
                            active={outputCaps.has(cap)}
                            label={capabilityLabels[cap]}
                            t={t}
                          />
                        ))}
                      </div>
                    </div>
                    {!isPerCall && (
                      <>
                        <PriceLine
                          label={t('输出')}
                          value={getPerTokenDisplay(priceData, 'completion')}
                          originalValue={
                            showPriceCompare
                              ? getPerTokenDisplay(
                                  originalPriceData,
                                  'completion',
                                )
                              : null
                          }
                          suffix={priceSuffix}
                        />
                        <PriceLine
                          label={t('缓存写入')}
                          value={getPerTokenDisplay(priceData, 'createCache')}
                          originalValue={
                            showPriceCompare
                              ? getPerTokenDisplay(
                                  originalPriceData,
                                  'createCache',
                                )
                              : null
                          }
                          suffix={priceSuffix}
                        />
                      </>
                    )}
                  </div>
                </div>

                {/* 按次计费单价行 */}
                {isPerCall && (
                  <div
                    className='text-xs mb-3 px-2 py-1.5 rounded-md'
                    style={{
                      backgroundColor: 'var(--semi-color-fill-0)',
                      color: 'var(--semi-color-text-1)',
                    }}
                  >
                    <span style={{ color: 'var(--semi-color-text-2)' }}>
                      {t('模型价格')}:
                    </span>{' '}
                    <span className='font-semibold'>
                      {priceData.price ?? '-'}
                    </span>
                    {showPriceCompare &&
                      originalPriceData.price &&
                      originalPriceData.price !== priceData.price && (
                        <span
                          className='line-through ml-1'
                          style={{
                            color: 'var(--semi-color-text-3)',
                            fontSize: 10,
                          }}
                        >
                          {originalPriceData.price}
                        </span>
                      )}
                    <span style={{ color: 'var(--semi-color-text-2)' }}>
                      {' '}
                      / {t('次')}
                    </span>
                  </div>
                )}

                {/* 价格参考 */}
                {referenceItems.length > 0 && (
                  <PricingReferenceBlock data={referenceData} t={t} />
                )}

                {/* 底部：左半（标签行顶 + 分组贴底）+ 右半（健康时间线贴底） */}
                <div className='mt-auto pt-2'>
                  <div className='grid grid-cols-2 gap-3'>
                    {/* 左：标签顶部，分组底部对齐 */}
                    <div className='min-w-0 flex flex-col'>
                      {/* 标签行（计费类型 + 自定义标签） */}
                      <div>{renderTagsRow(model)}</div>

                      {/* 可用分组 + 折扣 chip（贴底对齐；最多 4 个，自动换行） */}
                      {availableGroups.length > 0 && (
                        <div className='mt-auto pt-2'>
                          <div
                            className='text-[11px] mb-1'
                            style={{ color: 'var(--semi-color-text-2)' }}
                          >
                            {t('可用于 {{n}} 个分组', {
                              n: availableGroups.length,
                            })}
                          </div>
                          {groupDiscounts.length > 0 && (
                            <div className='flex flex-wrap gap-1'>
                              {renderLimitedItems({
                                items: groupDiscounts.map((d) => {
                                  const isActive =
                                    d.group === priceData.usedGroup;
                                  const unavailable = !isGroupUsable(d.group);
                                  return {
                                    key: d.group,
                                    element: (
                                      <GroupChip
                                        group={d.group}
                                        discountText={d.discount.text}
                                        isActive={isActive}
                                        clickable={allowCardSwitch}
                                        unavailable={unavailable}
                                        unavailableTooltip={t(
                                          '当前用户分组无法访问此渠道分组，点击查看升级方式',
                                        )}
                                        onClick={() => {
                                          if (unavailable) {
                                            showUpgradeModal(d.group);
                                            return;
                                          }
                                          toggleCardGroup(modelKey, d.group);
                                        }}
                                      />
                                    ),
                                  };
                                }),
                                renderItem: (item) => item.element,
                                maxDisplay: 4,
                              })}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                    {/* 右：健康时间线（多状态时纵向堆叠；左列更高时整体贴底对齐） */}
                    <div className='min-w-0 flex flex-col justify-end'>
                      {healthData && (
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
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </Card>
          );
        })}
      </div>
    </div>
  );
};

// 通用质朴 chip：与 GroupChip 视觉一致，但更中性。
// - accent: 低饱和主题色底（用于计费类型这种"次重要但需要识别"的信息）
// - warning: 低饱和警告色底（用于动态计费这类需要强提示的信息）
// - 默认：填充中性灰底（用于自定义标签等装饰信息）
const PlainChip = ({ children, accent, warning }) => {
  let bg = 'var(--semi-color-fill-1)';
  let fg = 'var(--semi-color-text-2)';
  if (accent) {
    bg = 'var(--semi-color-primary-light-default)';
    fg = 'var(--semi-color-primary)';
  } else if (warning) {
    bg = 'var(--semi-color-warning-light-default)';
    fg = 'var(--semi-color-warning)';
  }
  return (
  <span
    style={{
      display: 'inline-flex',
      alignItems: 'center',
      height: 20,
      padding: '0 8px',
      borderRadius: 4,
      fontSize: 11,
      fontWeight: 500,
      lineHeight: 1,
      backgroundColor: bg,
      color: fg,
      whiteSpace: 'nowrap',
    }}
  >
    {children}
  </span>
  );
};

// 分组折扣 chip：可点击切换价格显示。统一用系统主题色（primary）。
// - 激活态（priceData.usedGroup === group）：实色主题色背景 + 白字
// - 未激活：浅主题色底 + 主题色字
// - 不可用（用户分组无访问权限）：灰系样式 + tooltip 解释，点击弹升级 Modal
const GroupChip = ({
  group,
  discountText,
  isActive,
  clickable,
  unavailable,
  unavailableTooltip,
  onClick,
}) => {
  const [hover, setHover] = React.useState(false);

  let bg, fg;
  if (unavailable) {
    bg =
      clickable && hover
        ? 'var(--semi-color-fill-2)'
        : isActive
          ? 'var(--semi-color-fill-2)'
          : 'var(--semi-color-fill-1)';
    fg = 'var(--semi-color-text-2)';
  } else if (isActive) {
    bg = 'var(--semi-color-primary)';
    fg = '#fff';
  } else {
    bg =
      clickable && hover
        ? 'var(--semi-color-primary-light-hover)'
        : 'var(--semi-color-primary-light-default)';
    fg = 'var(--semi-color-primary)';
  }

  const chip = (
    <span
      role={clickable ? 'button' : undefined}
      onMouseEnter={() => clickable && setHover(true)}
      onMouseLeave={() => clickable && setHover(false)}
      onClick={(e) => {
        if (!clickable) return;
        e.stopPropagation();
        onClick?.();
      }}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        height: 20,
        padding: '0 8px',
        borderRadius: 4,
        fontSize: 11,
        fontWeight: 600,
        lineHeight: 1,
        backgroundColor: bg,
        color: fg,
        cursor: clickable ? 'pointer' : 'default',
        userSelect: 'none',
        transition: 'background-color 0.15s ease, color 0.15s ease',
        whiteSpace: 'nowrap',
        textDecoration: unavailable ? 'line-through' : 'none',
      }}
    >
      <span>{group}</span>
      <span style={{ opacity: 0.85 }}>{discountText}</span>
    </span>
  );

  if (unavailable && unavailableTooltip) {
    return (
      <Tooltip content={unavailableTooltip} position='top' showArrow={false}>
        {chip}
      </Tooltip>
    );
  }
  return chip;
};

// 单行价格（缺失值显示 -；当 originalValue 与 value 不同时附带划线原价）
const PriceLine = ({ label, value, originalValue, suffix }) => {
  const empty = value === null || value === undefined || value === '';
  const showOriginal =
    !empty &&
    originalValue !== null &&
    originalValue !== undefined &&
    originalValue !== '' &&
    originalValue !== value;
  return (
    <div className='flex items-baseline gap-1 truncate'>
      <span style={{ color: 'var(--semi-color-text-2)' }}>{label}:</span>
      {empty ? (
        <span style={{ color: 'var(--semi-color-text-3)' }}>-</span>
      ) : (
        <>
          <span className='font-medium' style={{ color: 'var(--semi-color-text-0)' }}>
            {value}
          </span>
          {showOriginal && (
            <span
              className='line-through'
              style={{ color: 'var(--semi-color-text-3)', fontSize: 10 }}
            >
              {originalValue}
            </span>
          )}
          {suffix && (
            <span
              className='truncate'
              style={{ color: 'var(--semi-color-text-2)', fontSize: 10 }}
            >
              {suffix}
            </span>
          )}
        </>
      )}
    </div>
  );
};

// 价格参考紧凑展示（首条 + 悬浮全部）
const PricingReferenceBlock = ({ data, t }) => {
  if (!data || !data.items || data.items.length === 0) return null;
  const first = data.items[0];
  const rest = data.items.slice(1);
  const tooltip = (
    <div className='text-xs' style={{ minWidth: 180 }}>
      {data.note && <div className='mb-1 text-gray-400'>{data.note}</div>}
      {data.items.map((it, i) => (
        <div key={i} className='flex items-center gap-1 py-0.5'>
          <span className='font-medium'>{it.scenario || '-'}</span>
          {it.official && (
            <span className='line-through text-gray-400'>{it.official}</span>
          )}
          {it.ours && <span className='font-semibold text-green-500'>{it.ours}</span>}
          {it.discount && <span className='text-green-500'>({it.discount})</span>}
        </div>
      ))}
    </div>
  );
  return (
    <Tooltip content={tooltip} position='top'>
      <div
        className='mb-3 rounded-lg px-2 py-1.5 flex items-center gap-1.5 text-xs'
        style={{
          backgroundColor: 'var(--semi-color-success-light-default)',
          border: '1px solid var(--semi-color-success-light-active)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <IconCreditCard
          size={12}
          style={{ color: 'var(--semi-color-success)' }}
        />
        <div className='flex-1 min-w-0 flex items-center gap-1.5 flex-wrap'>
          {first.scenario && (
            <span
              className='font-medium'
              style={{ color: 'var(--semi-color-text-0)' }}
            >
              {first.scenario}
            </span>
          )}
          {first.official && (
            <span
              className='line-through'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {first.official}
            </span>
          )}
          {first.ours && (
            <span
              className='font-semibold'
              style={{ color: 'var(--semi-color-success)' }}
            >
              {first.ours}
            </span>
          )}
          {first.discount && (
            <Tag
              size='small'
              color='green'
              shape='circle'
              style={{
                fontSize: '10px',
                padding: '0 6px',
                height: 16,
                lineHeight: '16px',
              }}
            >
              {first.discount}
            </Tag>
          )}
        </div>
        {rest.length > 0 && (
          <Tag
            size='small'
            color='white'
            shape='circle'
            style={{
              fontSize: '10px',
              padding: '0 6px',
              height: 16,
              lineHeight: '16px',
              flexShrink: 0,
            }}
          >
            +{rest.length}
          </Tag>
        )}
      </div>
    </Tooltip>
  );
};

export default PricingCardView;
