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

import React, { useEffect, useState, useRef } from 'react';
import {
  Banner,
  Button,
  Form,
  Row,
  Col,
  Typography,
  Spin,
  Select,
} from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsPaymentGateway(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    StripeApiSecret: '',
    StripeWebhookSecret: '',
    StripePriceId: '',
    StripeUnitPrice: 8.0,
    StripeMinTopUp: 1,
    StripePromotionCodesEnabled: false,
    StripeCurrency: 'CNY',
    StripeUseDynamicPrice: false,
    StripeDisableAdaptivePricing: false,
    StripeProductName: 'new-api 充值',
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        StripeApiSecret: props.options.StripeApiSecret || '',
        StripeWebhookSecret: props.options.StripeWebhookSecret || '',
        StripePriceId: props.options.StripePriceId || '',
        StripeUnitPrice:
          props.options.StripeUnitPrice !== undefined
            ? parseFloat(props.options.StripeUnitPrice)
            : 8.0,
        StripeMinTopUp:
          props.options.StripeMinTopUp !== undefined
            ? parseFloat(props.options.StripeMinTopUp)
            : 1,
        StripePromotionCodesEnabled:
          props.options.StripePromotionCodesEnabled !== undefined
            ? props.options.StripePromotionCodesEnabled === true || props.options.StripePromotionCodesEnabled === 'true'
            : false,
        StripeCurrency: props.options.StripeCurrency || 'CNY',
        StripeUseDynamicPrice:
          props.options.StripeUseDynamicPrice !== undefined
            ? props.options.StripeUseDynamicPrice === true || props.options.StripeUseDynamicPrice === 'true'
            : false,
        StripeDisableAdaptivePricing:
          props.options.StripeDisableAdaptivePricing !== undefined
            ? props.options.StripeDisableAdaptivePricing === true || props.options.StripeDisableAdaptivePricing === 'true'
            : false,
        StripeProductName: props.options.StripeProductName || '',
      };
      
      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs({...values});
    
    // 开启动态价格时，异步清空 Price ID
    if (values.StripeUseDynamicPrice && values.StripePriceId) {
      setTimeout(() => {
        formApiRef.current?.setValue('StripePriceId', '');
      }, 0);
    }
  };

  const submitStripeSetting = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    if (!inputs.StripeUseDynamicPrice && !inputs.StripePriceId) {
      showError(t('固定价格模式下必须填写商品价格 ID，或开启动态价格模式'));
      return;
    }

    setLoading(true);
    try {
      const options = [];

      if (inputs.StripeApiSecret && inputs.StripeApiSecret !== '') {
        options.push({ key: 'StripeApiSecret', value: inputs.StripeApiSecret });
      }
      if (inputs.StripeWebhookSecret && inputs.StripeWebhookSecret !== '') {
        options.push({
          key: 'StripeWebhookSecret',
          value: inputs.StripeWebhookSecret,
        });
      }
      // 始终保存 StripePriceId，包括清空的情况
      if (inputs.StripePriceId !== undefined) {
        options.push({ 
          key: 'StripePriceId', 
          value: inputs.StripePriceId || '' 
        });
      }
      if (
        inputs.StripeUnitPrice !== undefined &&
        inputs.StripeUnitPrice !== null
      ) {
        options.push({
          key: 'StripeUnitPrice',
          value: inputs.StripeUnitPrice.toString(),
        });
      }
      if (
        inputs.StripeMinTopUp !== undefined &&
        inputs.StripeMinTopUp !== null
      ) {
        options.push({
          key: 'StripeMinTopUp',
          value: inputs.StripeMinTopUp.toString(),
        });
      }
      
      // Boolean 字段：始终保存，确保开关状态同步
      if (inputs.StripePromotionCodesEnabled !== undefined) {
        options.push({
          key: 'StripePromotionCodesEnabled',
          value: inputs.StripePromotionCodesEnabled ? 'true' : 'false',
        });
      }
      if (inputs.StripeUseDynamicPrice !== undefined) {
        options.push({
          key: 'StripeUseDynamicPrice',
          value: inputs.StripeUseDynamicPrice ? 'true' : 'false',
        });
      }
      if (inputs.StripeDisableAdaptivePricing !== undefined) {
        options.push({
          key: 'StripeDisableAdaptivePricing',
          value: inputs.StripeDisableAdaptivePricing ? 'true' : 'false',
        });
      }
      
      // 其他字段
      if (inputs.StripeCurrency && inputs.StripeCurrency !== '') {
        options.push({
          key: 'StripeCurrency',
          value: inputs.StripeCurrency,
        });
      }
      if (inputs.StripeProductName !== undefined) {
        options.push({
          key: 'StripeProductName',
          value: inputs.StripeProductName || '',
        });
      }

      // 并发保存所有配置
      const queue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );
      
      const results = await Promise.all(queue);

      // 检查所有请求是否成功
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        // 更新本地存储的原始值
        setOriginInputs({ ...inputs });
        props.refresh?.();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('Stripe 设置')}>
          <Text>
            Stripe 密钥、Webhook 等设置请
            <a
              href='https://dashboard.stripe.com/developers'
              target='_blank'
              rel='noreferrer'
            >
              点击此处
            </a>
            进行设置，最好先在
            <a
              href='https://dashboard.stripe.com/test/developers'
              target='_blank'
              rel='noreferrer'
            >
              测试环境
            </a>
            进行测试。
            <br />
          </Text>
          <Banner
            type='info'
            description={`Webhook 填：${props.options.ServerAddress ? removeTrailingSlash(props.options.ServerAddress) : t('网站地址')}/api/stripe/webhook`}
          />
          <Banner
            type='warning'
            description={`需要包含事件：checkout.session.completed 和 checkout.session.expired`}
          />
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='StripeApiSecret'
                label={t('API 密钥')}
                placeholder={t(
                  'sk_xxx 或 rk_xxx 的 Stripe 密钥，敏感信息不显示',
                )}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='StripeWebhookSecret'
                label={t('Webhook 签名密钥')}
                placeholder={t('whsec_xxx 的 Webhook 签名密钥，敏感信息不显示')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='StripePriceId'
                label={t('商品价格 ID')}
                placeholder={t('price_xxx 的商品价格 ID，新建产品后可获得')}
                disabled={inputs.StripeUseDynamicPrice}
                extraText={
                  inputs.StripeUseDynamicPrice
                    ? t('动态价格模式下无需填写')
                    : t('固定价格模式下必填')
                }
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={6} lg={6} xl={6}>
              <Form.Select
                field='StripeCurrency'
                label={t('Stripe 支付货币')}
              >
                <Select.Option value='USD'>{t('美元 (USD)')}</Select.Option>
                <Select.Option value='CNY'>{t('人民币 (CNY)')}</Select.Option>
              </Form.Select>
            </Col>
            <Col xs={24} sm={24} md={6} lg={6} xl={6}>
              <Form.InputNumber
                field='StripeUnitPrice'
                precision={2}
                label={
                  inputs.StripeCurrency === 'USD'
                    ? t('充值价格（x$/美元）')
                    : t('充值价格（x元/美元）')
                }
                placeholder={
                  inputs.StripeCurrency === 'USD'
                    ? t('例如：1，即 1$/美元')
                    : t('例如：7.3，即 7.3元/美元')
                }
              />
            </Col>
            <Col xs={24} sm={24} md={6} lg={6} xl={6}>
              <Form.InputNumber
                field='StripeMinTopUp'
                label={t('最低充值美元数量')}
                placeholder={t('例如：2，即最低充值 2 美元')}
              />
            </Col>
            <Col xs={24} sm={24} md={6} lg={6} xl={6}>
              <Form.Input
                field='StripeProductName'
                label={t('产品名称（可选）')}
                placeholder={t('留空则使用：系统名称 Credits')}
                extraText={t('用于在 Stripe 后台区分不同项目的流水')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='StripeUseDynamicPrice'
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                label={t('使用动态价格（支持折扣）')}
                extraText={t(
                  '开启后支持折扣功能，但不使用上方的固定价格 ID',
                )}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='StripeDisableAdaptivePricing'
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                label={t('禁用多货币展示')}
                extraText={t(
                  '开启后用户只能用配置的货币支付，不显示其他货币选项',
                )}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='StripePromotionCodesEnabled'
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                label={t('允许在 Stripe 支付中输入促销码')}
              />
            </Col>
          </Row>
          <Button onClick={submitStripeSetting}>{t('更新 Stripe 设置')}</Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
