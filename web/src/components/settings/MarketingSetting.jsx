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
  Banner,
  Button,
  Card,
  Col,
  Form,
  Popconfirm,
  Row,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

import { API, showError, showSuccess, toBoolean } from '../../helpers';

const { Text } = Typography;

/**
 * 邮件营销 / Resend 联系人同步设置。
 *
 * 业务逻辑见 docs/event-system-design.md 第 21 节。前端只负责：
 *   - 读 / 写 6 个 marketing 相关 option (MarketingEnabled / MarketingProvider /
 *     ResendAPIKey / ResendDefaultSegmentID / ResendVIPSegmentID / ResendDefaultTopicIDs)
 *   - 提供"测试令牌"按钮调 POST /api/option/test_resend
 *
 * 后端保存后会自动重建 Provider 并接管事件分发，不需要重启服务。
 */
const MarketingSetting = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const formApiRef = useRef(null);

  const [inputs, setInputs] = useState({
    MarketingEnabled: false,
    MarketingProvider: 'resend',
    ResendAPIKey: '',
    ResendDefaultSegmentID: '',
    ResendVIPSegmentID: '',
    ResendDefaultTopicIDs: '',
    MarketingExtraEligibleGroups: '',
  });

  // 历史付费用户回填任务的状态
  const [backfillStatus, setBackfillStatus] = useState({
    running: false,
    result: null,
  });
  const [backfillSubmitting, setBackfillSubmitting] = useState(false);
  const backfillTimerRef = useRef(null);

  const loadOptions = async () => {
    try {
      setLoading(true);
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(t(message));
        return;
      }
      const next = { ...inputs };
      data.forEach((item) => {
        if (!(item.key in next)) return;
        if (item.key === 'MarketingEnabled') {
          next[item.key] = toBoolean(item.value);
        } else {
          next[item.key] = item.value || '';
        }
      });
      setInputs(next);
      if (formApiRef.current) {
        formApiRef.current.setValues(next);
      }
    } catch (err) {
      showError(t('刷新失败'));
    } finally {
      setLoading(false);
    }
  };

  const fetchBackfillStatus = async () => {
    try {
      // skipErrorHandler：绕开 api.js 全局响应拦截器的 showError，避免后端
      // 端点暂时不可用（如旧版二进制、网络抖动）时反复弹 toast 打扰用户。
      const res = await API.get('/api/option/backfill_marketing/status', {
        skipErrorHandler: true,
      });
      const { success, data } = res.data;
      if (success) {
        setBackfillStatus(data);
        // 正在跑就每 3 秒轮询一次
        if (data.running) {
          backfillTimerRef.current = setTimeout(fetchBackfillStatus, 3000);
        }
      }
    } catch (err) {
      // 静默 —— 状态查询失败不影响主流程
    }
  };

  useEffect(() => {
    loadOptions();
    fetchBackfillStatus();
    return () => {
      if (backfillTimerRef.current) {
        clearTimeout(backfillTimerRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleFormChange = (values) => {
    setInputs({ ...values });
  };

  const handleSave = async () => {
    if (inputs.MarketingEnabled && !inputs.ResendAPIKey) {
      showError(t('启用前请先填写 Resend API Key'));
      return;
    }
    setLoading(true);
    try {
      const payload = [
        { key: 'MarketingEnabled', value: inputs.MarketingEnabled ? 'true' : 'false' },
        { key: 'MarketingProvider', value: inputs.MarketingProvider || 'resend' },
        { key: 'ResendDefaultSegmentID', value: inputs.ResendDefaultSegmentID || '' },
        { key: 'ResendVIPSegmentID', value: inputs.ResendVIPSegmentID || '' },
        { key: 'ResendDefaultTopicIDs', value: inputs.ResendDefaultTopicIDs || '' },
        { key: 'MarketingExtraEligibleGroups', value: inputs.MarketingExtraEligibleGroups || '' },
      ];
      // API Key 仅在用户填写了新值时才保存（避免空字符串覆盖已有 key）
      if (inputs.ResendAPIKey && inputs.ResendAPIKey.trim() !== '') {
        payload.push({ key: 'ResendAPIKey', value: inputs.ResendAPIKey });
      }
      const results = await Promise.all(
        payload.map((opt) => API.put('/api/option/', opt)),
      );
      const errors = results.filter((r) => !r.data.success);
      if (errors.length > 0) {
        errors.forEach((r) => showError(r.data.message));
      } else {
        showSuccess(t('更新成功'));
        // 重新加载，让 API Key 字段回到 mask 状态
        await loadOptions();
      }
    } catch (err) {
      showError(t('更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const handleBackfill = async () => {
    setBackfillSubmitting(true);
    try {
      const res = await API.post('/api/option/backfill_marketing');
      const { success, message } = res.data;
      if (success) {
        showSuccess(message || t('回填任务已启动'));
        // 立即拉一次状态启动轮询
        fetchBackfillStatus();
      } else {
        showError(message || t('启动回填任务失败'));
      }
    } catch (err) {
      showError(t('请求失败：') + (err?.message || ''));
    } finally {
      setBackfillSubmitting(false);
    }
  };

  const handleTestToken = async () => {
    setTesting(true);
    try {
      // 若用户在输入框里填了新 key 但还没保存，优先用新 key 测试；
      // 否则后端会回退到已保存的 key
      const body = {};
      if (inputs.ResendAPIKey && inputs.ResendAPIKey.trim() !== '') {
        body.api_key = inputs.ResendAPIKey;
      }
      const res = await API.post('/api/option/test_resend', body);
      const { success, message } = res.data;
      if (success) {
        showSuccess(message || t('令牌有效'));
      } else {
        showError(message || t('令牌验证失败'));
      }
    } catch (err) {
      showError(t('测试请求失败：') + (err?.message || ''));
    } finally {
      setTesting(false);
    }
  };

  return (
    <Spin spinning={loading} size='large'>
      <Card style={{ marginTop: '10px' }}>
        <Form
          initValues={inputs}
          onValueChange={handleFormChange}
          getFormApi={(api) => (formApiRef.current = api)}
        >
          <Form.Section text={t('邮件营销 (Resend)')}>
            <Banner
              type='info'
              fullMode={false}
              description={
                <Text>
                  {t(
                    '开启后，付费用户（充值过的 default 用户 + 所有 vip 用户）会自动同步到 Resend 联系人，并按分组放入对应 Segment。免费用户、企业组用户不会进入。',
                  )}
                  <br />
                  {t(
                    '修改配置后自动生效，无需重启。详细业务规则见 docs/event-system-design.md。',
                  )}
                </Text>
              }
            />

            <Row
              gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
              style={{ marginTop: 16 }}
            >
              <Col xs={24} sm={24} md={6} lg={6} xl={6}>
                <Form.Switch
                  field='MarketingEnabled'
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  label={t('启用邮件营销同步')}
                  extraText={t('关闭后事件继续接收但不调用 Resend')}
                />
              </Col>
              <Col xs={24} sm={24} md={6} lg={6} xl={6}>
                <Form.Select
                  field='MarketingProvider'
                  label={t('Provider')}
                  extraText={t('当前仅支持 Resend；未来可扩展其他平台')}
                >
                  <Select.Option value='resend'>{t('Resend')}</Select.Option>
                </Form.Select>
              </Col>
            </Row>

            <Row
              gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
              style={{ marginTop: 16 }}
            >
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field='ResendAPIKey'
                  label={t('Resend API Key')}
                  placeholder={t('re_xxx，敏感信息保存后不回显')}
                  type='password'
                  extraText={t('从 https://resend.com/api-keys 创建')}
                />
              </Col>
              <Col xs={24} sm={24} md={12} lg={12} xl={12} style={{ display: 'flex', alignItems: 'flex-end' }}>
                <Space>
                  <Button
                    theme='light'
                    type='primary'
                    loading={testing}
                    onClick={handleTestToken}
                  >
                    {t('测试令牌')}
                  </Button>
                  <Text type='tertiary' size='small'>
                    {t('调用 Resend API 验证令牌是否有效')}
                  </Text>
                </Space>
              </Col>
            </Row>

            <Row
              gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
              style={{ marginTop: 16 }}
            >
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field='ResendDefaultSegmentID'
                  label={t('Default User Segment ID')}
                  placeholder={t('UUID，例如 abc-123-...')}
                  extraText={t('付费的 default 分组用户会被加入这个 Segment')}
                />
              </Col>
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field='ResendVIPSegmentID'
                  label={t('VIP User Segment ID')}
                  placeholder={t('UUID，例如 def-456-...')}
                  extraText={t('VIP 分组用户会被加入这个 Segment')}
                />
              </Col>
            </Row>

            <Row
              gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
              style={{ marginTop: 16 }}
            >
              <Col xs={24}>
                <Form.Input
                  field='ResendDefaultTopicIDs'
                  label={t('默认 Topic IDs（逗号分隔）')}
                  placeholder={t('topic-id-1, topic-id-2')}
                  extraText={t('新联系人创建时会自动 opt_in 这些 Topic；留空则不订阅')}
                />
              </Col>
            </Row>

            <Row
              gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
              style={{ marginTop: 16 }}
            >
              <Col xs={24}>
                <Form.Input
                  field='MarketingExtraEligibleGroups'
                  label={t('额外允许自助订阅的用户组（逗号分隔）')}
                  placeholder={t('enterprise_a, enterprise_b')}
                  extraText={t(
                    '这些组的用户可以在个人设置里管理邮件订阅，但 amux 不会自动同步他们到 Resend。需要管理员先手动在 Resend 创建 contact；用户首次保存时也会自动尝试创建。',
                  )}
                />
              </Col>
            </Row>

            <div style={{ marginTop: 16 }}>
              <Button type='primary' onClick={handleSave} loading={loading}>
                {t('保存邮件营销设置')}
              </Button>
            </div>
          </Form.Section>

          <Form.Section text={t('历史付费用户回填')}>
            <Banner
              type='warning'
              fullMode={false}
              description={t(
                '一次性把当前所有付费用户（VIP + 充值过的 default）灌入 Resend。Provider.Sync 幂等，重复执行无害。建议仅在首次启用 Resend 或更换 Segment ID 之后跑一次。',
              )}
            />
            <Row style={{ marginTop: 16 }}>
              <Col span={24}>
                <Space>
                  <Popconfirm
                    title={t('确认回填')}
                    content={t(
                      '会遍历所有付费用户调用 Resend API；可能耗时数分钟到数十分钟。确定要继续吗？',
                    )}
                    onConfirm={handleBackfill}
                    disabled={backfillStatus.running || backfillSubmitting}
                  >
                    <Button
                      theme='solid'
                      type='primary'
                      loading={backfillSubmitting}
                      disabled={backfillStatus.running}
                    >
                      {backfillStatus.running
                        ? t('回填中...')
                        : t('回填历史付费用户')}
                    </Button>
                  </Popconfirm>
                  {backfillStatus.running && (
                    <Tag color='blue' shape='circle'>
                      {t('运行中')}
                    </Tag>
                  )}
                </Space>
              </Col>
            </Row>
            {backfillStatus.result && (
              <Row style={{ marginTop: 12 }}>
                <Col span={24}>
                  <Text type='tertiary' size='small'>
                    {t('上次结果：')}
                    {t('总数={{n}}', { n: backfillStatus.result.total })}
                    {' / '}
                    {t('成功={{n}}', { n: backfillStatus.result.synced })}
                    {' / '}
                    {t('跳过={{n}}', { n: backfillStatus.result.skipped })}
                    {' / '}
                    {t('失败={{n}}', { n: backfillStatus.result.failed })}
                    {backfillStatus.result.last_error
                      ? ' — ' +
                        t('最近错误：') +
                        backfillStatus.result.last_error
                      : ''}
                  </Text>
                </Col>
              </Row>
            )}
          </Form.Section>
        </Form>
      </Card>
    </Spin>
  );
};

export default MarketingSetting;
