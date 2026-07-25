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
  Button,
  Col,
  Form,
  Row,
  Spin,
  Typography,
  Input,
  InputNumber,
  Space,
} from '@douyinfe/semi-ui';
import { Plus, Trash2 } from 'lucide-react';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  renderQuota,
  renderQuotaWithPrompt,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const MAX_PRIZES = 10;

const DEFAULT_INPUTS = {
  'blindbox_setting.enabled': false,
  'blindbox_setting.draw_time': '00:00',
  'blindbox_setting.threshold_quota': 500000,
  'blindbox_setting.pool_mode': 'percent',
  'blindbox_setting.percent_rate': 0.1,
  'blindbox_setting.percent_count': 3,
  'blindbox_setting.prizes': '[]',
};

// 配置项从 /api/option 取回一律是字符串（如 "false" / "5"）。必须按类型强转，
// 否则布尔字段的字符串 "false" 在 JS 里为真值，会让开关显示为"开"，且与基线比对
// 永远不相等 —— 用户以为已启用、实际从未保存。
const BOOL_KEYS = ['blindbox_setting.enabled'];
const INT_KEYS = [
  'blindbox_setting.threshold_quota',
  'blindbox_setting.percent_count',
];
const FLOAT_KEYS = ['blindbox_setting.percent_rate'];

function coerceOption(key, raw) {
  if (BOOL_KEYS.includes(key)) return String(raw) === 'true';
  if (INT_KEYS.includes(key)) {
    const n = parseInt(raw, 10);
    return Number.isNaN(n) ? 0 : n;
  }
  if (FLOAT_KEYS.includes(key)) {
    const n = parseFloat(raw);
    return Number.isNaN(n) ? 0 : n;
  }
  return raw;
}

export default function SettingsBlindBox(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({ ...DEFAULT_INPUTS });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  // 固定奖池奖项本地数组，与 inputs['blindbox_setting.prizes'] 的 JSON 字符串同步
  const [prizes, setPrizes] = useState([]);

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((inputs) => ({ ...inputs, [fieldName]: value }));
    };
  }

  function syncPrizes(next) {
    setPrizes(next);
    setInputs((prev) => ({
      ...prev,
      'blindbox_setting.prizes': JSON.stringify(
        next.map((p) => ({
          name: (p.name || '').trim(),
          quota: Number(p.quota) || 0,
        })),
      ),
    }));
  }

  function addPrize() {
    if (prizes.length >= MAX_PRIZES) {
      showWarning(t('最多支持 {{count}} 个奖项', { count: MAX_PRIZES }));
      return;
    }
    syncPrizes([...prizes, { name: '', quota: 0 }]);
  }

  function removePrize(index) {
    syncPrizes(prizes.filter((_, i) => i !== index));
  }

  function updatePrize(index, key, value) {
    const next = prizes.map((p, i) =>
      i === index ? { ...p, [key]: value } : p,
    );
    syncPrizes(next);
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      const value = String(inputs[item.key]);
      return API.put('/api/option/', { key: item.key, value });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    // 从 DEFAULT_INPUTS 出发，用 options 覆盖并按类型强转，避免字符串布尔/数值污染。
    const merged = { ...DEFAULT_INPUTS };
    for (let key in props.options) {
      if (key in DEFAULT_INPUTS) {
        merged[key] = coerceOption(key, props.options[key]);
      }
    }
    setInputs(merged);
    setInputsRow(structuredClone(merged));
    if (refForm.current) {
      refForm.current.setValues(merged);
    }
    // 解析固定奖池
    try {
      const parsed = JSON.parse(merged['blindbox_setting.prizes'] || '[]');
      if (Array.isArray(parsed)) {
        setPrizes(
          parsed.map((p) => ({ name: p.name || '', quota: p.quota || 0 })),
        );
      } else {
        setPrizes([]);
      }
    } catch (e) {
      setPrizes([]);
    }
  }, [props.options]);

  const enabled = inputs['blindbox_setting.enabled'];
  const poolMode = inputs['blindbox_setting.pool_mode'];
  // 「额度设置」里的单位额度（多少 quota 折合一个货币单位）。管理员可改，不能写死 500000。
  const quotaPerUnit = (() => {
    const raw = parseFloat(localStorage.getItem('quota_per_unit'));
    return Number.isFinite(raw) && raw > 0 ? raw : null;
  })();

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('盲盒抽奖设置')}>
            <Typography.Text
              type='tertiary'
              style={{ marginBottom: 16, display: 'block' }}
            >
              {t(
                '用户在开奖周期内消耗达到门槛后，需主动点击「参与本期」才会进入奖池；消耗越多中奖概率越大。开奖时点统一结算，中奖后仍需用户主动开盒领取，到下次开奖仍未开则作废。未开启的盲盒会阻止用户参与新一期。',
              )}
            </Typography.Text>
            {/* 本页所有额度字段填的都是 quota 原始值，不是金额。换算比例取自
                「额度设置」里的单位额度，管理员改过就跟着变，不能写死。 */}
            <Typography.Text
              type='tertiary'
              style={{ marginBottom: 16, display: 'block' }}
            >
              {t(
                '下方所有额度字段填写的都是额度（quota）原始值，不是金额。当前 {{unit}} 额度 = {{money}}。',
                {
                  unit: quotaPerUnit ?? '—',
                  money: quotaPerUnit ? renderQuota(quotaPerUnit) : '—',
                },
              )}
            </Typography.Text>
            <Typography.Text
              type='warning'
              style={{ marginBottom: 16, display: 'block' }}
            >
              {t(
                '注意：达标判定依赖消费日志。若「日志设置」中的「记录消费日志」处于关闭状态，系统将无法统计任何用户消耗，盲盒会一直无人达标。',
              )}
            </Typography.Text>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'blindbox_setting.enabled'}
                  label={t('启用盲盒功能')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('blindbox_setting.enabled')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  field={'blindbox_setting.draw_time'}
                  label={t('每日开奖时间 (HH:MM)')}
                  placeholder='00:00'
                  onChange={handleFieldChange('blindbox_setting.draw_time')}
                  disabled={!enabled}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'blindbox_setting.threshold_quota'}
                  label={
                    <span>
                      {t('参与门槛额度')}{' '}
                      {renderQuotaWithPrompt(
                        inputs['blindbox_setting.threshold_quota'],
                      )}
                    </span>
                  }
                  placeholder={t('达到该消耗额度即可参与')}
                  onChange={handleFieldChange(
                    'blindbox_setting.threshold_quota',
                  )}
                  min={0}
                  disabled={!enabled}
                />
              </Col>
            </Row>

            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Select
                  field={'blindbox_setting.pool_mode'}
                  label={t('奖池模式')}
                  onChange={handleFieldChange('blindbox_setting.pool_mode')}
                  disabled={!enabled}
                  style={{ width: '100%' }}
                  optionList={[
                    { label: t('百分比奖池'), value: 'percent' },
                    { label: t('固定奖池'), value: 'fixed' },
                  ]}
                />
              </Col>
              {poolMode === 'percent' && (
                <>
                  <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                    <Form.InputNumber
                      field={'blindbox_setting.percent_rate'}
                      label={t('奖池比例 (0~1，0.1=10%)')}
                      onChange={handleFieldChange(
                        'blindbox_setting.percent_rate',
                      )}
                      min={0}
                      max={1}
                      step={0.01}
                      disabled={!enabled}
                    />
                  </Col>
                  <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                    <Form.InputNumber
                      field={'blindbox_setting.percent_count'}
                      label={t('奖项数量 (1~10)')}
                      onChange={handleFieldChange(
                        'blindbox_setting.percent_count',
                      )}
                      min={1}
                      max={MAX_PRIZES}
                      disabled={!enabled}
                    />
                  </Col>
                </>
              )}
            </Row>

            {poolMode === 'fixed' && (
              <div style={{ marginTop: 8 }}>
                <Typography.Text
                  strong
                  style={{ display: 'block', marginBottom: 8 }}
                >
                  {t('固定奖池奖项')}（{prizes.length}/{MAX_PRIZES}）
                </Typography.Text>
                <Typography.Text
                  type='tertiary'
                  style={{ marginBottom: 12, display: 'block' }}
                >
                  {t(
                    '每个奖项设置名称与中奖额度。额度填 quota 原始值（右侧实时显示折算金额），参与人数少于奖项数时，多余奖项作废。',
                  )}
                </Typography.Text>
                {prizes.map((p, index) => (
                  <Space
                    key={index}
                    align='center'
                    style={{ marginBottom: 8, display: 'flex' }}
                  >
                    <Input
                      value={p.name}
                      placeholder={t('奖项名称')}
                      style={{ width: 200 }}
                      disabled={!enabled}
                      onChange={(v) => updatePrize(index, 'name', v)}
                    />
                    <InputNumber
                      value={p.quota}
                      placeholder={t('中奖额度（quota）')}
                      min={1}
                      step={quotaPerUnit || 1}
                      style={{ width: 180 }}
                      disabled={!enabled}
                      onChange={(v) => updatePrize(index, 'quota', v)}
                    />
                    {/* 实时折算：填的是 quota 原始值，没有这行提示很容易把 1 当成 $1 */}
                    <Typography.Text
                      type={Number(p.quota) > 0 ? 'success' : 'tertiary'}
                      style={{ minWidth: 96, display: 'inline-block' }}
                    >
                      {Number(p.quota) > 0 ? `= ${renderQuota(p.quota)}` : '—'}
                    </Typography.Text>
                    <Button
                      type='danger'
                      theme='borderless'
                      icon={<Trash2 size={16} />}
                      disabled={!enabled}
                      onClick={() => removePrize(index)}
                    />
                  </Space>
                ))}
                <Button
                  icon={<Plus size={16} />}
                  disabled={!enabled || prizes.length >= MAX_PRIZES}
                  onClick={addPrize}
                  style={{ marginTop: 4 }}
                >
                  {t('添加奖项')}
                </Button>
              </div>
            )}

            <Row style={{ marginTop: 16 }}>
              <Button size='default' onClick={onSubmit}>
                {t('保存盲盒设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
