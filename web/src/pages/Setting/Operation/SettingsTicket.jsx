/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Col,
  Form,
  Row,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

/**
 * 工单系统配置面板。enabled=false 时其余字段灰显，避免误改。
 * 字段集合需与 setting/operation_setting/ticket_setting.go 保持同步：
 *   - 改这里时记得同步后端 default 与字段，否则 GlobalConfig 会丢字段。
 */
export default function SettingsTicket(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);

  const DEFAULTS = {
    'ticket_setting.enabled': false,
    'ticket_setting.user_daily_limit': 30,
    'ticket_setting.user_hourly_limit': 10,
    'ticket_setting.user_reply_per_minute': 6,
    'ticket_setting.max_title_length': 200,
    'ticket_setting.max_content_length': 32768,
    'ticket_setting.max_attachments_per_message': 6,
    'ticket_setting.require_verified_email': true,
    'ticket_setting.auto_resolve_days': 14,
    'ticket_setting.reopen_after_closed_days': 7,
    'ticket_setting.reopen_after_resolved_days': 30,
    'ticket_setting.notify_email_to_admin': true,
    'ticket_setting.notify_email_to_user': true,
    'ticket_setting.notify_telegram_to_admin': false,
    'ticket_setting.notify_in_app_enabled': true,
    'ticket_setting.telegram_bot_token': '',
    'ticket_setting.telegram_chat_id': '',
    'ticket_setting.admin_emails': '',
  };

  const [inputs, setInputs] = useState(DEFAULTS);
  const [inputsRow, setInputsRow] = useState(DEFAULTS);
  const refForm = useRef();

  const enabled = !!inputs['ticket_setting.enabled'];

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((prev) => ({ ...prev, [fieldName]: value }));
    };
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value:
          typeof inputs[item.key] === 'boolean'
            ? String(inputs[item.key])
            : String(inputs[item.key]),
      }),
    );
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
      .catch(() => showError(t('保存失败，请重试')))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    const next = {};
    for (let key in props.options) {
      if (Object.keys(DEFAULTS).includes(key)) {
        next[key] = props.options[key];
      }
    }
    // 缺失字段补默认，避免 Form 控件成为 uncontrolled
    for (let key in DEFAULTS) {
      if (!(key in next)) next[key] = DEFAULTS[key];
    }
    setInputs(next);
    setInputsRow(structuredClone(next));
    refForm.current?.setValues(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(api) => (refForm.current = api)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('工单系统设置')}>
          <Typography.Text type='tertiary' className='block mb-4'>
            {t(
              '工单系统允许用户提交求助与反馈，管理员在工单管理页面统一处理。关闭后用户侧"我的工单"和管理员侧"工单管理"入口都会被隐藏。',
            )}
          </Typography.Text>

          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Switch
                field='ticket_setting.enabled'
                label={t('启用工单系统')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange('ticket_setting.enabled')}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Switch
                field='ticket_setting.require_verified_email'
                label={t('要求绑定邮箱后才能建单')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.require_verified_email',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Switch
                field='ticket_setting.notify_in_app_enabled'
                label={t('启用站内未读红点')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.notify_in_app_enabled',
                )}
              />
            </Col>
          </Row>

          <Typography.Title heading={6} className='!mt-4 !mb-2'>
            {t('限流')}
          </Typography.Title>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.user_daily_limit'
                label={t('用户每日建单上限')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange('ticket_setting.user_daily_limit')}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.user_hourly_limit'
                label={t('用户每小时建单上限')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.user_hourly_limit',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.user_reply_per_minute'
                label={t('用户回复每分钟上限')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.user_reply_per_minute',
                )}
              />
            </Col>
          </Row>

          <Typography.Title heading={6} className='!mt-4 !mb-2'>
            {t('长度与附件')}
          </Typography.Title>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.max_title_length'
                label={t('标题最大字符数')}
                min={10}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.max_title_length',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.max_content_length'
                label={t('正文最大字符数')}
                min={100}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.max_content_length',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.max_attachments_per_message'
                label={t('每条消息附件上限')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.max_attachments_per_message',
                )}
              />
            </Col>
          </Row>

          <Typography.Title heading={6} className='!mt-4 !mb-2'>
            {t('自动关闭与重开')}
          </Typography.Title>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.auto_resolve_days'
                label={t('N 天无人回复自动标记已解决（0=不启用）')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.auto_resolve_days',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.reopen_after_closed_days'
                label={t('已关闭工单可在 N 天内重开（0=不限）')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.reopen_after_closed_days',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.InputNumber
                field='ticket_setting.reopen_after_resolved_days'
                label={t('已解决工单可在 N 天内重开（0=不限）')}
                min={0}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.reopen_after_resolved_days',
                )}
              />
            </Col>
          </Row>

          <Typography.Title heading={6} className='!mt-4 !mb-2'>
            {t('通知')}
          </Typography.Title>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Switch
                field='ticket_setting.notify_email_to_admin'
                label={t('新工单/用户回复邮件通知管理员')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.notify_email_to_admin',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Switch
                field='ticket_setting.notify_email_to_user'
                label={t('管理员回复邮件通知用户')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.notify_email_to_user',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Switch
                field='ticket_setting.notify_telegram_to_admin'
                label={t('Telegram 推送管理员')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.notify_telegram_to_admin',
                )}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} md={12}>
              <Form.Input
                field='ticket_setting.admin_emails'
                label={t('管理员邮箱列表（英文逗号分隔，留空使用 root 邮箱）')}
                disabled={!enabled}
                onChange={handleFieldChange('ticket_setting.admin_emails')}
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.Input
                field='ticket_setting.telegram_bot_token'
                label={t('Telegram Bot Token')}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.telegram_bot_token',
                )}
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.Input
                field='ticket_setting.telegram_chat_id'
                label={t('Telegram Chat ID')}
                disabled={!enabled}
                onChange={handleFieldChange(
                  'ticket_setting.telegram_chat_id',
                )}
              />
            </Col>
          </Row>

          <Row className='mt-4'>
            <Button size='default' onClick={onSubmit}>
              {t('保存工单设置')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
