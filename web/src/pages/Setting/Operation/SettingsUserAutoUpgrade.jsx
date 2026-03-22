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
  Card,
  Space,
  Typography,
  InputNumber,
  Select,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';

const { Text } = Typography;

export default function SettingsUserAutoUpgrade(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    'user_upgrade_setting.auto_upgrade_enabled': false,
    'user_upgrade_setting.upgrade_rules': '[]',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const [upgradeRules, setUpgradeRules] = useState([]);
  const [availableGroups, setAvailableGroups] = useState([]);

  // 加载可用分组列表
  const loadGroups = async () => {
    try {
      const res = await API.get('/api/group/');
      const { success, data } = res.data;
      if (success && data && Array.isArray(data) && data.length > 0) {
        // API返回的是数组格式的分组名称列表，直接使用
        setAvailableGroups(data);
      }
    } catch (error) {
      console.error(t('加载分组列表失败，请检查网络或刷新页面'), error);
      showError(t('加载分组列表失败，请检查网络或刷新页面'));
    }
  };

  useEffect(() => {
    loadGroups();
  }, []);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
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
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }

    // 转换布尔值
    if (typeof currentInputs['user_upgrade_setting.auto_upgrade_enabled'] === 'string') {
      currentInputs['user_upgrade_setting.auto_upgrade_enabled'] =
        currentInputs['user_upgrade_setting.auto_upgrade_enabled'] === 'true';
    }

    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);

    // 解析升级规则
    try {
      const rulesStr = currentInputs['user_upgrade_setting.upgrade_rules'] || '[]';
      const rules = JSON.parse(rulesStr);
      if (Array.isArray(rules)) {
        setUpgradeRules(rules);
      } else {
        setUpgradeRules([]);
      }
    } catch (e) {
      console.error('Failed to parse upgrade rules:', e);
      setUpgradeRules([]);
    }
  }, [props.options]);

  const handleAddRule = () => {
    // 使用系统中已有的分组作为默认值
    const defaultFromGroup = availableGroups[0] || 'default';
    const defaultToGroup = availableGroups[1] || 'vip';
    
    const newRules = [
      ...upgradeRules,
      { from_group: defaultFromGroup, to_group: defaultToGroup, threshold: 100.0 },
    ];
    setUpgradeRules(newRules);
    const rulesStr = JSON.stringify(newRules);
    const newInputs = {
      ...inputs,
      'user_upgrade_setting.upgrade_rules': rulesStr,
    };
    setInputs(newInputs);
    refForm.current?.setValue(
      'user_upgrade_setting.upgrade_rules',
      rulesStr
    );
  };

  const handleRemoveRule = (index) => {
    const newRules = upgradeRules.filter((_, i) => i !== index);
    setUpgradeRules(newRules);
    const rulesStr = JSON.stringify(newRules);
    const newInputs = {
      ...inputs,
      'user_upgrade_setting.upgrade_rules': rulesStr,
    };
    setInputs(newInputs);
    refForm.current?.setValue(
      'user_upgrade_setting.upgrade_rules',
      rulesStr
    );
  };

  const handleRuleChange = (index, field, value) => {
    const newRules = [...upgradeRules];
    newRules[index][field] = value;
    setUpgradeRules(newRules);
    const rulesStr = JSON.stringify(newRules);
    const newInputs = {
      ...inputs,
      'user_upgrade_setting.upgrade_rules': rulesStr,
    };
    setInputs(newInputs);
    refForm.current?.setValue(
      'user_upgrade_setting.upgrade_rules',
      rulesStr
    );
  };

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('用户自动升级设置')}>
            <Row gutter={16}>
              <Col span={24}>
                <Form.Switch
                  field={'user_upgrade_setting.auto_upgrade_enabled'}
                  label={t('启用自动升级')}
                  checkedText={t('开')}
                  uncheckedText={t('关')}
                  onChange={(value) => {
                    const newInputs = {
                      ...inputs,
                      'user_upgrade_setting.auto_upgrade_enabled': value,
                    };
                    setInputs(newInputs);
                    refForm.current?.setValue(
                      'user_upgrade_setting.auto_upgrade_enabled',
                      value
                    );
                  }}
                />
                <Text type="tertiary" style={{ fontSize: 12, marginTop: 4 }}>
                  {t(
                    '启用后，用户充值达到设定金额时将自动升级到对应分组'
                  )}
                </Text>
              </Col>
            </Row>
          </Form.Section>

          <Form.Section text={t('升级规则配置')}>
            <Row gutter={16}>
              <Col span={24}>
                <Space vertical style={{ width: '100%' }} spacing={12}>
                  {upgradeRules.map((rule, index) => (
                    <Card
                      key={index}
                      style={{ width: '100%' }}
                      bodyStyle={{ padding: 16 }}
                    >
                      <Space
                        style={{ width: '100%', justifyContent: 'space-between' }}
                      >
                        <Space spacing={12} align="center">
                          <div style={{ minWidth: 80 }}>
                            <Text strong>{t('从分组')}</Text>
                          </div>
                          <Select
                            style={{ minWidth: 150 }}
                            value={rule.from_group}
                            onChange={(value) =>
                              handleRuleChange(index, 'from_group', value)
                            }
                            placeholder={t('选择或输入源分组')}
                            filter
                            allowCreate
                          >
                            {availableGroups.map((group) => (
                              <Select.Option key={group} value={group}>
                                {group}
                              </Select.Option>
                            ))}
                          </Select>

                          <Text type="tertiary">{t('升级到')}</Text>

                          <Select
                            style={{ minWidth: 150 }}
                            value={rule.to_group}
                            onChange={(value) =>
                              handleRuleChange(index, 'to_group', value)
                            }
                            placeholder={t('选择或输入目标分组')}
                            filter
                            allowCreate
                          >
                            {availableGroups.map((group) => (
                              <Select.Option key={group} value={group}>
                                {group}
                              </Select.Option>
                            ))}
                          </Select>

                          <Text type="tertiary">{t('当累计充值 ≥')}</Text>

                          <InputNumber
                            style={{ minWidth: 120 }}
                            value={rule.threshold}
                            onChange={(value) =>
                              handleRuleChange(index, 'threshold', value || 0)
                            }
                            suffix="USD"
                            min={0}
                            step={10}
                            precision={2}
                            placeholder={t('充值金额阈值')}
                          />

                          <Text type="tertiary">{t('美元')}</Text>
                        </Space>

                        <Button
                          type="danger"
                          size="small"
                          onClick={() => handleRemoveRule(index)}
                        >
                          {t('删除')}
                        </Button>
                      </Space>
                    </Card>
                  ))}

                  <Button
                    type="dashed"
                    block
                    onClick={handleAddRule}
                    disabled={!inputs['user_upgrade_setting.auto_upgrade_enabled']}
                  >
                    {t('+ 添加升级规则')}
                  </Button>

                  <Text type="tertiary" style={{ fontSize: 12 }}>
                    {t(
                      '规则按顺序匹配，用户充值成功后会自动检查是否满足升级条件。支持链式升级（如 default → vip → svip）。'
                    )}
                  </Text>
                </Space>
              </Col>
            </Row>
          </Form.Section>

          <Form.Section>
            <Row>
              <Col span={24}>
                <Button
                  size={'large'}
                  type={'primary'}
                  htmlType={'submit'}
                  className="btn-margin-right"
                  onClick={onSubmit}
                  loading={loading}
                >
                  {t('保存用户自动升级设置')}
                </Button>
              </Col>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
