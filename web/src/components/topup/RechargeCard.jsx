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

import React, { useEffect, useRef, useState } from 'react';
import {
  Avatar,
  Typography,
  Card,
  Button,
  Banner,
  Form,
  Space,
  Spin,
  Tabs,
  TabPane,
  Badge,
} from '@douyinfe/semi-ui';
import {
  SiAlipay,
  SiWechat,
  SiApplepay,
  SiBitcoin,
} from 'react-icons/si';
import {
  CreditCard,
  Coins,
  Wallet,
  BarChart2,
  TrendingUp,
  Receipt,
  Sparkles,
  Landmark,
} from 'lucide-react';
import { IconGift } from '@douyinfe/semi-icons';
import { useMinimumLoadingTime } from '../../hooks/common/useMinimumLoadingTime';
import { getCurrencyConfig } from '../../helpers/render';
import SubscriptionPlansCard from './SubscriptionPlansCard';

const { Text } = Typography;

// Google Pay 官方 logo（彩色 G + 灰色 Pay）。viewBox 437x174，按高度等比缩放。
const GooglePayLogo = ({ height = 20 }) => (
  <svg
    height={height}
    width={(height * 437) / 174}
    viewBox='0 0 437 174'
    xmlns='http://www.w3.org/2000/svg'
    aria-hidden='true'
  >
    <g fill='none' fillRule='nonzero'>
      <path
        fill='#5F6368'
        d='M207.2 84.6v50.8h-16.1V10h42.7c10.3-.2 20.2 3.7 27.7 10.9 7.5 6.7 11.7 16.4 11.5 26.4.2 10.1-4 19.8-11.5 26.6-7.5 7.1-16.7 10.7-27.6 10.7h-26.7zm0-59.2v43.8h27c6 .2 11.8-2.2 15.9-6.5 8.5-8.2 8.6-21.7.4-30.2l-.4-.4c-4.1-4.4-9.9-6.8-15.9-6.6l-27-.1zM310.1 46.8c11.9 0 21.3 3.2 28.2 9.5 6.9 6.4 10.3 15.1 10.3 26.2v52.8h-15.4v-11.9h-.7c-6.7 9.8-15.5 14.7-26.6 14.7-9.4 0-17.4-2.8-23.7-8.4-6.2-5.2-9.7-12.9-9.5-21 0-8.9 3.4-15.9 10.1-21.2 6.7-5.3 15.7-7.9 26.9-7.9 9.6 0 17.4 1.8 23.6 5.2v-3.7c0-5.5-2.4-10.7-6.6-14.2-4.3-3.8-9.8-5.9-15.5-5.9-9 0-16.1 3.8-21.4 11.4l-14.2-8.9c7.7-11.1 19.2-16.7 34.5-16.7zm-20.8 62.3c0 4.2 2 8.1 5.3 10.5 3.6 2.8 8 4.3 12.5 4.2 6.8 0 13.3-2.7 18.1-7.5 5.3-5 8-10.9 8-17.7-5-4-12-6-21-6-6.5 0-12 1.6-16.4 4.7-4.3 3.2-6.5 7.1-6.5 11.8zM437 49.6l-53.8 123.6h-16.6l20-43.2-35.4-80.3h17.5l25.5 61.6h.4l24.9-61.6z'
      />
      <path
        fill='#4285F4'
        d='M142.1 73.6c0-4.9-.4-9.8-1.2-14.6H73v27.7h38.9c-1.6 8.9-6.8 16.9-14.4 21.9v18h23.2c13.6-12.5 21.4-31 21.4-53z'
      />
      <path
        fill='#34A853'
        d='M73 144c19.4 0 35.8-6.4 47.7-17.4l-23.2-18c-6.5 4.4-14.8 6.9-24.5 6.9-18.8 0-34.7-12.7-40.4-29.7H8.7v18.6C20.9 128.6 45.8 144 73 144z'
      />
      <path
        fill='#FBBC04'
        d='M32.6 85.8c-3-8.9-3-18.6 0-27.6V39.7H8.7a71.39 71.39 0 0 0 0 64.6l23.9-18.5z'
      />
      <path
        fill='#EA4335'
        d='M73 28.5c10.3-.2 20.2 3.7 27.6 10.8l20.5-20.5C108.1 6.5 90.9-.2 73 0 45.8 0 20.9 15.4 8.7 39.7l23.9 18.6C38.3 41.2 54.2 28.5 73 28.5z'
      />
    </g>
  </svg>
);

// Link（link.com，Stripe 的 Link 支付）官方 logo：绿色圆 + 白色 link 符号。
const LinkLogo = ({ size = 26 }) => (
  <svg
    width={size}
    height={size}
    viewBox='0 0 40 40'
    xmlns='http://www.w3.org/2000/svg'
    fill='none'
    aria-hidden='true'
  >
    <circle cx='20' cy='20' r='20' fill='#00D66F' />
    <path
      fill='#000000'
      d='M19.08 8h-6.168c1.2 5.017 4.704 9.305 9.088 12-4.392 2.697-7.888 6.985-9.088 12h6.168c1.528-4.64 5.76-8.672 10.96-9.495v-5.017C24.832 16.672 20.6 12.64 19.08 8Z'
    />
  </svg>
);

// Stripe 是聚合支付，点「支付」按钮或任一图标都走 Stripe Checkout 自动列出全部可用方式。
// 图标做成 app 图标样式（圆角 tile，背景随主题且与主背景有差异），均可点击。
// label 为 i18n key，渲染时用 t() 翻译。
// 单色 logo(Card/Apple Pay/ACH) 不写死颜色 → 继承 tile 的主题文字色（亮深暗浅）；
// 品牌色 logo（Google Pay/支付宝/Crypto/Link）保留各自颜色。
const STRIPE_PAY_METHODS = [
  { key: 'card', label: 'Card', icon: <CreditCard size={24} /> },
  { key: 'applepay', label: 'Apple Pay', icon: <SiApplepay size={32} /> },
  { key: 'googlepay', label: 'Google Pay', icon: <GooglePayLogo height={15} /> },
  { key: 'alipay', label: 'Alipay', icon: <SiAlipay size={24} color='#1677FF' /> },
  { key: 'link', label: 'Link', icon: <LinkLogo size={26} /> },
  { key: 'crypto', label: 'Crypto', icon: <SiBitcoin size={24} color='#F7931A' /> },
  { key: 'ach', label: 'ACH', icon: <Landmark size={22} /> },
];

const RechargeCard = ({
  t,
  enableOnlineTopUp,
  enableStripeTopUp,
  enableCreemTopUp,
  creemProducts,
  creemPreTopUp,
  presetAmounts,
  selectedPreset,
  selectPresetAmount,
  formatLargeNumber,
  priceRatio,
  topUpCount,
  minTopUp,
  renderQuotaWithAmount,
  getAmount,
  setTopUpCount,
  setSelectedPreset,
  renderAmount,
  amountLoading,
  payMethods,
  preTopUp,
  paymentLoading,
  payWay,
  redemptionCode,
  setRedemptionCode,
  topUp,
  isSubmitting,
  topUpLink,
  openTopUpLink,
  userState,
  renderQuota,
  statusLoading,
  topupInfo,
  onOpenHistory,
  enableWaffoTopUp,
  waffoTopUp,
  waffoPayMethods,
  subscriptionLoading = false,
  subscriptionPlans = [],
  billingPreference,
  onChangeBillingPreference,
  activeSubscriptions = [],
  allSubscriptions = [],
  reloadSubscriptionSelf,
  stripeUnitPrice = 8.0,
  stripeCurrency = 'CNY',
  stripeCurrencySymbol = '¥',
}) => {
  const onlineFormApiRef = useRef(null);
  const redeemFormApiRef = useRef(null);
  const initialTabSetRef = useRef(false);
  const showAmountSkeleton = useMinimumLoadingTime(amountLoading);
  const [activeTab, setActiveTab] = useState('topup');
  const shouldShowSubscription =
    !subscriptionLoading && subscriptionPlans.length > 0;

  useEffect(() => {
    if (initialTabSetRef.current) return;
    if (subscriptionLoading) return;
    setActiveTab(shouldShowSubscription ? 'subscription' : 'topup');
    initialTabSetRef.current = true;
  }, [shouldShowSubscription, subscriptionLoading]);

  useEffect(() => {
    if (!shouldShowSubscription && activeTab !== 'topup') {
      setActiveTab('topup');
    }
  }, [shouldShowSubscription, activeTab]);
  const topupContent = (
    <Space vertical style={{ width: '100%' }}>
      {/* 统计数据 */}
      <Card
        className='!rounded-xl w-full'
        cover={
          <div
            className='relative h-30'
            style={{
              '--palette-primary-darkerChannel': '37 99 235',
              backgroundImage: `linear-gradient(0deg, rgba(var(--palette-primary-darkerChannel) / 80%), rgba(var(--palette-primary-darkerChannel) / 80%)), url('/cover-4.webp')`,
              backgroundSize: 'cover',
              backgroundPosition: 'center',
              backgroundRepeat: 'no-repeat',
            }}
          >
            <div className='relative z-10 h-full flex flex-col justify-between p-4'>
              <div className='flex justify-between items-center'>
                <Text strong style={{ color: 'white', fontSize: '16px' }}>
                  {t('账户统计')}
                </Text>
              </div>

              {/* 统计数据 */}
              <div className='grid grid-cols-3 gap-6 mt-4'>
                {/* 当前余额 */}
                <div className='text-center'>
                  <div
                    className='text-base sm:text-2xl font-bold mb-2'
                    style={{ color: 'white' }}
                  >
                    {renderQuota(userState?.user?.quota)}
                  </div>
                  <div className='flex items-center justify-center text-sm'>
                    <Wallet
                      size={14}
                      className='mr-1'
                      style={{ color: 'rgba(255,255,255,0.8)' }}
                    />
                    <Text
                      style={{
                        color: 'rgba(255,255,255,0.8)',
                        fontSize: '12px',
                      }}
                    >
                      {t('当前余额')}
                    </Text>
                  </div>
                </div>

                {/* 历史消耗 */}
                <div className='text-center'>
                  <div
                    className='text-base sm:text-2xl font-bold mb-2'
                    style={{ color: 'white' }}
                  >
                    {renderQuota(userState?.user?.used_quota)}
                  </div>
                  <div className='flex items-center justify-center text-sm'>
                    <TrendingUp
                      size={14}
                      className='mr-1'
                      style={{ color: 'rgba(255,255,255,0.8)' }}
                    />
                    <Text
                      style={{
                        color: 'rgba(255,255,255,0.8)',
                        fontSize: '12px',
                      }}
                    >
                      {t('历史消耗')}
                    </Text>
                  </div>
                </div>

                {/* 请求次数 */}
                <div className='text-center'>
                  <div
                    className='text-base sm:text-2xl font-bold mb-2'
                    style={{ color: 'white' }}
                  >
                    {userState?.user?.request_count || 0}
                  </div>
                  <div className='flex items-center justify-center text-sm'>
                    <BarChart2
                      size={14}
                      className='mr-1'
                      style={{ color: 'rgba(255,255,255,0.8)' }}
                    />
                    <Text
                      style={{
                        color: 'rgba(255,255,255,0.8)',
                        fontSize: '12px',
                      }}
                    >
                      {t('请求次数')}
                    </Text>
                  </div>
                </div>
              </div>
            </div>
          </div>
        }
      >
        {/* 在线充值表单 */}
        {statusLoading ? (
          <div className='py-8 flex justify-center'>
            <Spin size='large' />
          </div>
        ) : enableOnlineTopUp || enableStripeTopUp || enableCreemTopUp || enableWaffoTopUp ? (
          <Form
            getFormApi={(api) => (onlineFormApiRef.current = api)}
            initValues={{ topUpCount: topUpCount }}
          >
            <div className='space-y-6'>
              {/* 选择充值额度（无标题） */}
              {(enableOnlineTopUp || enableStripeTopUp || enableWaffoTopUp) && (
                <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-2'>
                    {presetAmounts.map((preset, index) => {
                      const discount =
                        preset.discount || topupInfo?.discount?.[preset.value] || 1.0;
                      
                      let unitPrice = priceRatio;
                      let paymentSymbol = '¥';
                      
                      if (payWay === 'stripe') {
                        unitPrice = stripeUnitPrice;
                        paymentSymbol = stripeCurrencySymbol;
                      }
                      
                      const originalPrice = preset.value * unitPrice;
                      const discountedPrice = originalPrice * discount;
                      const hasDiscount = discount < 1.0;
                      const actualPay = discountedPrice;
                      const save = originalPrice - discountedPrice;

                      return (
                        <div
                          key={index}
                          onClick={() => {
                            selectPresetAmount(preset);
                            onlineFormApiRef.current?.setValue(
                              'topUpCount',
                              preset.value,
                            );
                          }}
                          className='relative flex flex-col items-center justify-center rounded-xl cursor-pointer transition-all hover:shadow-sm'
                          style={{
                            minHeight: 76,
                            padding: '18px 8px 10px',
                            border:
                              selectedPreset === preset.value
                                ? '2px solid var(--semi-color-primary)'
                                : '1px solid var(--semi-color-border)',
                            background:
                              selectedPreset === preset.value
                                ? 'var(--semi-color-primary-light-default)'
                                : 'var(--semi-color-bg-1)',
                          }}
                        >
                          {hasDiscount && (
                            <span
                              style={{
                                position: 'absolute',
                                top: 0,
                                right: 0,
                                fontSize: 10,
                                lineHeight: '15px',
                                padding: '0 5px',
                                borderTopRightRadius: 11,
                                borderBottomLeftRadius: 8,
                                color: '#fff',
                                background: 'var(--semi-color-success)',
                                fontWeight: 600,
                              }}
                            >
                              {t('折').includes('off')
                                ? ((1 - parseFloat(discount)) * 100).toFixed(1)
                                : (discount * 10).toFixed(1)}
                              {t('折')}
                            </span>
                          )}
                          <div
                            className='flex items-center gap-1'
                            style={{ fontSize: 15, fontWeight: 600, lineHeight: 1.2 }}
                          >
                            <Coins size={14} />
                            <span>{formatLargeNumber(preset.value)}</span>
                          </div>
                          <div
                            style={{
                              color: 'var(--semi-color-text-2)',
                              fontSize: 11,
                              marginTop: 4,
                            }}
                          >
                            {t('实付')} {paymentSymbol}
                            {actualPay.toFixed(2)}
                          </div>
                        </div>
                      );
                    })}
                  </div>
              )}

              {/* 充值金额（原“充值数量”；1:1 兑换，避免“数量”造成误解，下方实付金额随之更新） */}
              {(enableOnlineTopUp || enableStripeTopUp || enableWaffoTopUp) && (
                <Form.InputNumber
                  field='topUpCount'
                  label={t('充值金额')}
                  disabled={!enableOnlineTopUp && !enableStripeTopUp && !enableWaffoTopUp}
                  placeholder={t('充值金额，最低 ') + renderQuotaWithAmount(minTopUp)}
                  value={topUpCount}
                  min={minTopUp}
                  max={999999999}
                  step={1}
                  precision={0}
                  onChange={async (value) => {
                    if (value && value >= 1) {
                      setTopUpCount(value);
                      setSelectedPreset(null);
                      await getAmount(value);
                    }
                  }}
                  onBlur={(e) => {
                    const value = parseInt(e.target.value);
                    if (!value || value < 1) {
                      setTopUpCount(1);
                      getAmount(1);
                    }
                  }}
                  formatter={(value) => (value ? `${value}` : '')}
                  parser={(value) => (value ? parseInt(value.replace(/[^\d]/g, '')) : 0)}
                  style={{ width: '100%' }}
                />
              )}

              {/* 支付：Stripe 聚合支付 —— 大按钮 + 可点击的方式图标(图标+名称)，点击都走默认 Stripe 跳转 */}
              {payMethods && payMethods.filter((m) => m.type !== 'waffo').length > 0 && (
                <div className='space-y-5'>
                  {payMethods
                    .filter((m) => m.type !== 'waffo')
                    .map((payMethod) => {
                      const minTopupVal = Number(payMethod.min_topup) || 0;
                      const isStripe = payMethod.type === 'stripe';
                      const disabled =
                        (!enableOnlineTopUp && !isStripe) ||
                        (!enableStripeTopUp && isStripe) ||
                        minTopupVal > Number(topUpCount || 0);

                      if (isStripe) {
                        return (
                          <div key={payMethod.type} className='flex flex-col gap-4'>
                            <Button
                              theme='solid'
                              type='primary'
                              block
                              size='large'
                              onClick={() => preTopUp(payMethod.type)}
                              disabled={disabled}
                              loading={paymentLoading && payWay === payMethod.type}
                              className='!rounded-xl !h-12 !text-base !font-semibold'
                            >
                              {t('支付')}
                              {!showAmountSkeleton && ` ${renderAmount()}`}
                            </Button>
                            <div className='flex flex-wrap justify-center gap-x-5 gap-y-4'>
                              {STRIPE_PAY_METHODS.map((m) => (
                                <button
                                  key={m.key}
                                  type='button'
                                  disabled={disabled}
                                  onClick={() => preTopUp(payMethod.type)}
                                  title={t(m.label)}
                                  className='group flex flex-col items-center gap-1.5 border-0 bg-transparent cursor-pointer disabled:cursor-not-allowed disabled:opacity-50'
                                >
                                  <span
                                    className='inline-flex h-14 w-14 items-center justify-center rounded-2xl shadow-sm transition-all group-hover:-translate-y-0.5 group-hover:shadow-md'
                                    style={{
                                      // 固定正方形(56x56)：宽 logo 在方形内居中，不再把按钮撑宽。
                                      // 背景用随主题的填充色，与主背景有差异（暗色偏黑、亮色偏浅）；
                                      // color 设为主题文字色，让未写死颜色的单色 logo 继承（亮深暗浅）。
                                      backgroundColor: 'var(--semi-color-fill-0)',
                                      color: 'var(--semi-color-text-0)',
                                      border: '1px solid var(--semi-color-border)',
                                    }}
                                  >
                                    {m.icon}
                                  </span>
                                  <span
                                    className='text-xs'
                                    style={{ color: 'var(--semi-color-text-2)' }}
                                  >
                                    {t(m.label)}
                                  </span>
                                </button>
                              ))}
                            </div>
                          </div>
                        );
                      }

                      // 非 stripe（支付宝/微信等本地直连）保持按钮
                      return (
                        <Button
                          key={payMethod.type}
                          theme='outline'
                          type='tertiary'
                          block
                          onClick={() => preTopUp(payMethod.type)}
                          disabled={disabled}
                          loading={paymentLoading && payWay === payMethod.type}
                          icon={
                            payMethod.type === 'alipay' ? (
                              <SiAlipay size={18} color='#1677FF' />
                            ) : payMethod.type === 'wxpay' ? (
                              <SiWechat size={18} color='#07C160' />
                            ) : (
                              <CreditCard
                                size={18}
                                color={payMethod.color || 'var(--semi-color-text-2)'}
                              />
                            )
                          }
                          className='!rounded-lg'
                        >
                          {payMethod.name}
                        </Button>
                      );
                    })}
                </div>
              )}

              {/* Waffo 充值区域 */}
              {enableWaffoTopUp &&
                waffoPayMethods &&
                waffoPayMethods.length > 0 && (
                  <Form.Slot label={t('Waffo 充值')}>
                    <Space wrap>
                      {waffoPayMethods.map((method, index) => (
                        <Button
                          key={index}
                          theme='outline'
                          type='tertiary'
                          onClick={() => waffoTopUp(index)}
                          loading={paymentLoading}
                          icon={
                            method.icon ? (
                              <img
                                src={method.icon}
                                alt={method.name}
                                style={{
                                  width: 36,
                                  height: 36,
                                  objectFit: 'contain',
                                }}
                              />
                            ) : (
                              <CreditCard
                                size={18}
                                color='var(--semi-color-text-2)'
                              />
                            )
                          }
                          className='!rounded-lg !px-4 !py-2'
                        >
                          {method.name}
                        </Button>
                      ))}
                    </Space>
                  </Form.Slot>
                )}

              {/* Creem 充值区域 */}
              {enableCreemTopUp && creemProducts.length > 0 && (
                <Form.Slot label={t('Creem 充值')}>
                  <div className='grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3'>
                    {creemProducts.map((product, index) => (
                      <Card
                        key={index}
                        onClick={() => creemPreTopUp(product)}
                        className='cursor-pointer !rounded-2xl transition-all hover:shadow-md border-gray-200 hover:border-gray-300'
                        bodyStyle={{ textAlign: 'center', padding: '16px' }}
                      >
                        <div className='font-medium text-lg mb-2'>
                          {product.name}
                        </div>
                        <div className='text-sm text-gray-600 mb-2'>
                          {t('充值额度')}: {product.quota}
                        </div>
                        <div className='text-lg font-semibold text-blue-600'>
                          {product.currency === 'EUR' ? '€' : '$'}
                          {product.price}
                        </div>
                      </Card>
                    ))}
                  </div>
                </Form.Slot>
              )}
            </div>
          </Form>
        ) : (
          <Banner
            type='info'
            description={t(
              '管理员未开启在线充值功能，请联系管理员开启或使用兑换码充值。',
            )}
            className='!rounded-xl'
            closeIcon={null}
          />
        )}
      </Card>

      {/* 兑换码充值 */}
      <Card
        className='!rounded-xl w-full'
        title={
          <Text type='tertiary' strong>
            {t('兑换码充值')}
          </Text>
        }
      >
        <Form
          getFormApi={(api) => (redeemFormApiRef.current = api)}
          initValues={{ redemptionCode: redemptionCode }}
        >
          <Form.Input
            field='redemptionCode'
            noLabel={true}
            placeholder={t('请输入兑换码')}
            value={redemptionCode}
            onChange={(value) => setRedemptionCode(value)}
            prefix={<IconGift />}
            suffix={
              <div className='flex items-center gap-2'>
                <Button
                  type='primary'
                  theme='solid'
                  onClick={topUp}
                  loading={isSubmitting}
                >
                  {t('兑换额度')}
                </Button>
              </div>
            }
            showClear
            style={{ width: '100%' }}
            extraText={
              topUpLink && (
                <Text type='tertiary'>
                  {t('在找兑换码？')}
                  <Text
                    type='secondary'
                    underline
                    className='cursor-pointer'
                    onClick={openTopUpLink}
                  >
                    {t('购买兑换码')}
                  </Text>
                </Text>
              )
            }
          />
        </Form>
      </Card>

      {/* 充值说明 */}
      <Card
        className='!rounded-xl w-full'
        title={<Text type='tertiary'>{t('充值说明')}</Text>}
      >
        <div className='space-y-3'>
          <div className='flex items-start gap-2'>
            <Badge dot type='warning' />
            <Text type='tertiary' className='text-sm'>
              {t('充值说明1')}
            </Text>
          </div>

          <div className='flex items-start gap-2'>
            <Badge dot type='primary' />
            <Text type='tertiary' className='text-sm'>
              {t('充值说明2')}
            </Text>
          </div>

          <div className='flex items-start gap-2'>
            <Badge dot type='primary' />
            <Text type='tertiary' className='text-sm'>
              {t('充值说明4')}
            </Text>
          </div>

          <div className='flex items-start gap-2'>
            <Badge dot type='primary' />
            <div className='flex flex-col gap-1'>
              <Text type='tertiary' className='text-sm'>
                {t('充值说明5')}
              </Text>
              <Text type='secondary' className='text-sm'>
                {t('充值说明联系方式')}
              </Text>
            </div>
          </div>
        </div>
      </Card>
    </Space>
  );

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      {/* 卡片头部 */}
      <div className='flex items-center justify-between mb-4'>
        <div className='flex items-center'>
          <Avatar size='small' color='blue' className='mr-3 shadow-md'>
            <CreditCard size={16} />
          </Avatar>
          <div>
            <Typography.Text className='text-lg font-medium'>
              {t('账户充值')}
            </Typography.Text>
            <div className='text-xs'>{t('多种充值方式，安全便捷')}</div>
          </div>
        </div>
        <Button
          icon={<Receipt size={16} />}
          theme='solid'
          onClick={onOpenHistory}
        >
          {t('账单')}
        </Button>
      </div>

      {shouldShowSubscription ? (
        <Tabs type='card' activeKey={activeTab} onChange={setActiveTab}>
          <TabPane
            tab={
              <div className='flex items-center gap-2'>
                <Sparkles size={16} />
                {t('订阅套餐')}
              </div>
            }
            itemKey='subscription'
          >
            <div className='py-2'>
              <SubscriptionPlansCard
                t={t}
                loading={subscriptionLoading}
                plans={subscriptionPlans}
                payMethods={payMethods}
                enableOnlineTopUp={enableOnlineTopUp}
                enableStripeTopUp={enableStripeTopUp}
                enableCreemTopUp={enableCreemTopUp}
                billingPreference={billingPreference}
                onChangeBillingPreference={onChangeBillingPreference}
                activeSubscriptions={activeSubscriptions}
                allSubscriptions={allSubscriptions}
                reloadSubscriptionSelf={reloadSubscriptionSelf}
                withCard={false}
              />
            </div>
          </TabPane>
          <TabPane
            tab={
              <div className='flex items-center gap-2'>
                <Wallet size={16} />
                {t('额度充值')}
              </div>
            }
            itemKey='topup'
          >
            <div className='py-2'>{topupContent}</div>
          </TabPane>
        </Tabs>
      ) : (
        topupContent
      )}
    </Card>
  );
};

export default RechargeCard;
