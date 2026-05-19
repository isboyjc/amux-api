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

import React, { useEffect, useState } from 'react';
import {
  Avatar,
  Banner,
  Button,
  Card,
  Checkbox,
  Spin,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { Mail } from 'lucide-react';

import { API, showError, showSuccess } from '../../../../helpers';

const { Text } = Typography;

/**
 * 用户个人设置 → 邮件订阅卡片。
 *
 * 设计原则（详见 docs/event-system-design.md 第 23 节）：
 *   - amux 不持久化用户的订阅状态；每次进入都实时查 provider（Resend）
 *   - 非付费用户：表单 disabled + 顶部提示，告诉用户充值后可解锁
 *   - 全局退订开关 = Resend 的 unsubscribed 字段
 *   - 下方 checkbox 列表 = admin 在后台配置的 topic 列表，按用户当前订阅状态预勾
 */
const MarketingSubscriptions = ({ t }) => {
  // loaded：首次 load 完成前不渲染任何内容，避免"先闪一下卡片不存在再出现"
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [eligible, setEligible] = useState(false);
  const [providerConfigured, setProviderConfigured] = useState(false);
  const [topics, setTopics] = useState([]);
  const [globalUnsubscribed, setGlobalUnsubscribed] = useState(false);
  // subscribedMap: { [topic_id]: boolean }，未在 provider 返回里的视为未订阅
  const [subscribedMap, setSubscribedMap] = useState({});

  const load = async () => {
    setLoading(true);
    setLoadError(false);
    try {
      // skipErrorHandler：provider 未开 / 未配置时后端返回 success 但 eligible=false，
      // 不应该用全局 error toast 打扰用户
      const res = await API.get('/api/user/self/marketing_subscriptions', {
        skipErrorHandler: true,
      });
      const { success, data } = res.data || {};
      if (!success || !data) {
        setEligible(false);
        setProviderConfigured(false);
        setLoadError(true);
        return;
      }
      setEligible(!!data.eligible);
      setProviderConfigured(!!data.provider_configured);
      const availableTopics = Array.isArray(data.available_topics)
        ? data.available_topics
        : [];
      setTopics(availableTopics);
      const current = data.current || {};
      setGlobalUnsubscribed(!!current.global_unsubscribed);
      const map = {};
      (current.topics || []).forEach((s) => {
        map[s.topic_id] = !!s.subscribed;
      });
      setSubscribedMap(map);
    } catch (err) {
      // 网络层失败 → 显示 banner 让用户知道，而不是静默
      setLoadError(true);
    } finally {
      setLoading(false);
      setLoaded(true);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const handleToggleGlobal = (checked) => {
    // Semi Switch 的 checked 表示"接收邮件"，对应后端 globalUnsubscribed=!checked
    setGlobalUnsubscribed(!checked);
  };

  const handleToggleTopic = (topicId, checked) => {
    setSubscribedMap((prev) => ({ ...prev, [topicId]: checked }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const payload = {
        global_unsubscribed: globalUnsubscribed,
        topics: topics.map((tp) => ({
          topic_id: tp.id,
          subscribed: !!subscribedMap[tp.id],
        })),
      };
      const res = await API.put(
        '/api/user/self/marketing_subscriptions',
        payload,
      );
      const { success, message } = res.data || {};
      if (success) {
        showSuccess(message || t('订阅设置已更新'));
      } else {
        showError(message || t('更新失败'));
      }
    } catch (err) {
      showError(t('请求失败：') + (err?.message || ''));
    } finally {
      setSaving(false);
    }
  };

  // 首次 load 尚未完成 → 不渲染（避免闪烁）
  if (!loaded) {
    return null;
  }
  // provider 未配置 → 不展示该卡片（不显示给用户营销系统的存在）
  if (!providerConfigured && !loadError) {
    return null;
  }

  const disabled = !eligible || loading || saving;

  return (
    <Card className='!rounded-2xl'>
      {/* 卡片头部 */}
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='pink' className='mr-3 shadow-md'>
          <Mail size={16} />
        </Avatar>
        <div>
          <Typography.Text className='text-lg font-medium'>
            {t('邮件订阅')}
          </Typography.Text>
          <div className='text-xs text-gray-600 dark:text-gray-400'>
            {t('管理你接收的营销邮件和订阅细分')}
          </div>
        </div>
      </div>

      <Spin spinning={loading}>
        {loadError && (
          <Banner
            type='warning'
            fullMode={false}
            description={t(
              '暂时无法读取订阅状态，请稍后刷新重试',
            )}
            style={{ marginBottom: 16 }}
          />
        )}
        {!loadError && !eligible && (
          <Banner
            type='info'
            fullMode={false}
            description={t(
              '邮件订阅功能仅对付费用户开放，充值任意金额后即可设置个人订阅偏好。',
            )}
            style={{ marginBottom: 16 }}
          />
        )}

        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '8px 0',
          }}
        >
          <div>
            <Text strong>{t('接收营销邮件')}</Text>
            <div>
              <Text type='tertiary' size='small'>
                {t('关闭后下方所有 topic 订阅都不会生效')}
              </Text>
            </div>
          </div>
          <Switch
            checked={!globalUnsubscribed}
            onChange={handleToggleGlobal}
            disabled={disabled}
          />
        </div>

        {topics.length > 0 && (
          <>
            <div style={{ margin: '12px 0 8px 0' }}>
              <Text type='secondary'>{t('订阅细分')}</Text>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {topics.map((tp) => (
                <div
                  key={tp.id}
                  style={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    gap: 12,
                    padding: '8px 12px',
                    border: '1px solid var(--semi-color-border)',
                    borderRadius: 8,
                  }}
                >
                  <Checkbox
                    checked={!!subscribedMap[tp.id]}
                    onChange={(e) =>
                      handleToggleTopic(tp.id, e.target.checked)
                    }
                    disabled={disabled || globalUnsubscribed}
                  />
                  <div style={{ flex: 1 }}>
                    <Text strong>{tp.name || tp.id}</Text>
                    {tp.description && (
                      <div>
                        <Text type='tertiary' size='small'>
                          {tp.description}
                        </Text>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </>
        )}

        {eligible && topics.length === 0 && !loading && (
          <Text type='tertiary' size='small'>
            {t('暂未配置可订阅的 topic，请联系管理员')}
          </Text>
        )}

        <div style={{ marginTop: 16, textAlign: 'right' }}>
          <Button
            type='primary'
            theme='solid'
            loading={saving}
            disabled={disabled}
            onClick={handleSave}
          >
            {t('保存订阅设置')}
          </Button>
        </div>
      </Spin>
    </Card>
  );
};

export default MarketingSubscriptions;
