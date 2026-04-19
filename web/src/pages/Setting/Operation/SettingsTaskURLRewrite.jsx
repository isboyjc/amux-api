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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Form,
  Input,
  Space,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { Plus, Trash2 } from 'lucide-react';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import { useTranslation } from 'react-i18next';

/**
 * 任务结果 URL 脱敏/反向代理设置。
 *
 * 后端对应 setting/operation_setting.TaskURLRewriteSetting：
 *   task_url_rewrite_setting.enabled  bool
 *   task_url_rewrite_setting.rules    []{from,to}  —— 存储为 JSON 字符串
 *
 * 页面职责：
 *   1. 启用/禁用开关
 *   2. 规则列表的增删改
 *   3. 保存时把 rules 序列化成字符串 PUT 到 /api/option/
 */
const SETTING_KEY_ENABLED = 'task_url_rewrite_setting.enabled';
const SETTING_KEY_RULES = 'task_url_rewrite_setting.rules';

const parseRules = (raw) => {
  if (!raw) return [];
  if (Array.isArray(raw)) return raw;
  try {
    const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw;
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((r) => r && typeof r === 'object')
      .map((r) => ({ from: String(r.from || ''), to: String(r.to || '') }));
  } catch {
    return [];
  }
};

export default function SettingsTaskURLRewrite(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [enabled, setEnabled] = useState(false);
  // rules: [{from, to}]，初始来自 props.options
  const [rules, setRules] = useState([]);
  // 初始快照，用来判定"有没有改动"（鼓励只保存真的变更）
  const [origEnabled, setOrigEnabled] = useState(false);
  const [origRulesJSON, setOrigRulesJSON] = useState('[]');

  // 每次 props.options 变化（父级刷新）时，重置一次本地状态。
  useEffect(() => {
    const opts = props.options || {};
    const nextEnabled =
      typeof opts[SETTING_KEY_ENABLED] === 'boolean'
        ? opts[SETTING_KEY_ENABLED]
        : String(opts[SETTING_KEY_ENABLED] || '').toLowerCase() === 'true';
    const nextRules = parseRules(opts[SETTING_KEY_RULES]);

    setEnabled(nextEnabled);
    setRules(nextRules);
    setOrigEnabled(nextEnabled);
    setOrigRulesJSON(JSON.stringify(nextRules));
  }, [props.options]);

  const currentRulesJSON = useMemo(() => JSON.stringify(rules), [rules]);
  const dirty = enabled !== origEnabled || currentRulesJSON !== origRulesJSON;

  const updateRule = (idx, patch) => {
    setRules((prev) => {
      const next = prev.slice();
      next[idx] = { ...next[idx], ...patch };
      return next;
    });
  };

  const addRule = () => {
    setRules((prev) => [...prev, { from: '', to: '' }]);
  };

  const removeRule = (idx) => {
    setRules((prev) => prev.filter((_, i) => i !== idx));
  };

  const validate = () => {
    // 允许空列表（相当于只保留开关但无规则）；有条目则两列都不能空，from 要唯一
    const seen = new Set();
    for (let i = 0; i < rules.length; i++) {
      const r = rules[i];
      const from = (r.from || '').trim();
      const to = (r.to || '').trim();
      if (!from || !to) {
        showError(t('第 {{n}} 行的"从"或"到"不能为空', { n: i + 1 }));
        return false;
      }
      if (!/^https?:\/\//i.test(from)) {
        showError(t('第 {{n}} 行的"从"需以 http:// 或 https:// 开头', { n: i + 1 }));
        return false;
      }
      if (!/^https?:\/\//i.test(to)) {
        showError(t('第 {{n}} 行的"到"需以 http:// 或 https:// 开头', { n: i + 1 }));
        return false;
      }
      if (seen.has(from)) {
        showError(t('第 {{n}} 行的"从"前缀与上面的重复了', { n: i + 1 }));
        return false;
      }
      seen.add(from);
    }
    return true;
  };

  const onSubmit = async () => {
    if (!dirty) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }
    if (!validate()) return;

    const payloads = [];
    if (enabled !== origEnabled) {
      payloads.push({ key: SETTING_KEY_ENABLED, value: String(enabled) });
    }
    if (currentRulesJSON !== origRulesJSON) {
      // 保存一份干净的结构，去掉首尾空白
      const clean = rules.map((r) => ({
        from: (r.from || '').trim(),
        to: (r.to || '').trim(),
      }));
      payloads.push({ key: SETTING_KEY_RULES, value: JSON.stringify(clean) });
    }

    setLoading(true);
    try {
      await Promise.all(payloads.map((p) => API.put('/api/option/', p)));
      showSuccess(t('保存成功'));
      props.refresh?.();
    } catch (err) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <Form style={{ marginBottom: 15 }}>
        <Form.Section text={t('任务结果 URL 脱敏')}>
          <Banner
            type='info'
            closeIcon={null}
            description={t(
              '把上游厂商返回的视频 / 结果直链按前缀替换成自家反代地址，避免暴露上游源站。按"从"做严格前缀匹配，多条规则最长前缀优先命中；落库前一次性替换，操练场、任务日志、/v1/videos 代理都会用脱敏后的 URL。',
            )}
            style={{ marginBottom: 12 }}
          />

          <Form.Switch
            field='_enabled_placeholder'
            label={t('启用 URL 脱敏规则')}
            checked={enabled}
            onChange={setEnabled}
            size='default'
            checkedText='｜'
            uncheckedText='〇'
            extraText={t('关闭时所有规则失效，落库为上游原始 URL')}
          />

          <div style={{ marginTop: 16 }}>
            <Typography.Text strong>{t('前缀映射规则')}</Typography.Text>
            <Typography.Text
              type='tertiary'
              style={{ marginLeft: 8, fontSize: 12 }}
            >
              {t('示例：https://resource.xxx.com/prefix/ → https://r.amux.ai/alias/')}
            </Typography.Text>
          </div>

          <div style={{ marginTop: 8 }}>
            {rules.length === 0 ? (
              <Typography.Text
                type='tertiary'
                style={{
                  display: 'block',
                  padding: '12px 0',
                  textAlign: 'center',
                }}
              >
                {t('暂无规则，点击下方"添加规则"按钮新增')}
              </Typography.Text>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {rules.map((rule, idx) => (
                  <Space
                    key={idx}
                    align='center'
                    style={{ width: '100%' }}
                    spacing={8}
                  >
                    <Input
                      value={rule.from}
                      onChange={(v) => updateRule(idx, { from: v })}
                      placeholder={t('从（上游前缀，含协议和末尾 /）')}
                      style={{ flex: 1, minWidth: 320 }}
                      prefix={t('从')}
                    />
                    <Typography.Text type='tertiary'>→</Typography.Text>
                    <Input
                      value={rule.to}
                      onChange={(v) => updateRule(idx, { to: v })}
                      placeholder={t('到（自家反代前缀）')}
                      style={{ flex: 1, minWidth: 320 }}
                      prefix={t('到')}
                    />
                    <Button
                      icon={<Trash2 size={14} />}
                      type='danger'
                      theme='borderless'
                      size='small'
                      onClick={() => removeRule(idx)}
                      aria-label={t('删除')}
                    />
                  </Space>
                ))}
              </div>
            )}

            <Button
              icon={<Plus size={14} />}
              theme='borderless'
              size='small'
              onClick={addRule}
              style={{ marginTop: 8 }}
            >
              {t('添加规则')}
            </Button>
          </div>
        </Form.Section>

        <Button type='primary' onClick={onSubmit} disabled={!dirty}>
          {t('保存任务 URL 脱敏设置')}
        </Button>
      </Form>
    </Spin>
  );
}
