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
  ColorPicker,
  Col,
  Form,
  Input,
  Row,
  Space,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { Megaphone, X, RotateCcw } from 'lucide-react';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

// 与后端 setting/operation_setting/announcement_bar_setting.go 对齐的可见
// 字段（version 由后端自动派生，前端只读，不在表单里暴露）
const KEYS = {
  enabled: 'announcement_bar.enabled',
  content: 'announcement_bar.content',
  link: 'announcement_bar.link',
  openInNewTab: 'announcement_bar.open_in_new_tab',
  bgColor: 'announcement_bar.bg_color',
  accentColor: 'announcement_bar.accent_color',
  textColor: 'announcement_bar.text_color',
};

// Form.Field 的 field 字段如果含有 ".", Semi 会按"嵌套对象路径"解析；
// 我们的 key 本来就是 "announcement_bar.xxx" 这种带点的扁平 key，所以要
// 用 ['key.with.dots'] 这种字符串字面量绕过解析。
// (项目里 StorageSetting / DashboardSetting 都用同款 helper)
const ff = (key) => `['${key}']`;

// 默认色（与 setting/operation_setting/announcement_bar_setting.go 对齐）。
// 「重置默认」按钮回到这套；空字符串落库表示"用 CSS 默认"，但前端这里
// 直接展示这套颜色更直观
const COLOR_DEFAULTS = {
  bg: '#5a3f1f',
  accent: '#d4a13e',
  text: '#f4e4c1',
};

// 默认值常量；外面的 useEffect 直接基于它合并，避免依赖 useState 闭包里
// 的 inputs（防止"props 还没回种这些 key 时 next={} 把 inputs 清空"的 bug）
const DEFAULTS = {
  'announcement_bar.enabled': false,
  'announcement_bar.content': '',
  'announcement_bar.link': '',
  'announcement_bar.open_in_new_tab': false,
  'announcement_bar.bg_color': COLOR_DEFAULTS.bg,
  'announcement_bar.accent_color': COLOR_DEFAULTS.accent,
  'announcement_bar.text_color': COLOR_DEFAULTS.text,
};

// 简单 hex 校验（允许 #RGB / #RRGGBB / #RRGGBBAA），与后端正则保持一致
const HEX_RE = /^#([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$/i;
const isHex = (v) => typeof v === 'string' && HEX_RE.test(v);

// 紧凑色板字段：左侧 36×24 swatch（点开弹 ColorPicker），右侧 hex 文本
// 输入。两者双向绑定。直接在 JSX 里写一遍太啰嗦，抽成组件。
const ColorField = ({ value, onChange }) => {
  const safe = isHex(value) ? value : '#000000';
  const handlePicker = (next) => {
    // ColorPicker 给的是 { hex, rgba, hsva }；只取 hex 落到表单
    if (next?.hex) onChange(next.hex);
  };
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <ColorPicker
        value={ColorPicker.colorStringToValue(safe)}
        onChange={handlePicker}
        usePopover
        eyeDropper={false}
        alpha={false}
        defaultFormat='hex'
      >
        <div
          role='button'
          tabIndex={0}
          aria-label='choose color'
          style={{
            width: 36,
            height: 24,
            borderRadius: 4,
            border: '1px solid var(--semi-color-border)',
            background: safe,
            cursor: 'pointer',
            flexShrink: 0,
          }}
        />
      </ColorPicker>
      <Input
        value={value || ''}
        onChange={onChange}
        placeholder='#RRGGBB'
        style={{ width: 130 }}
        size='small'
      />
    </div>
  );
};

export default function SettingsAnnouncementBar(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({ ...DEFAULTS });
  const [inputsRow, setInputsRow] = useState({ ...DEFAULTS });
  const refForm = useRef();

  function handleFieldChange(field) {
    return (value) => setInputs((prev) => ({ ...prev, [field]: value }));
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    // 链接为空时强制把"在新标签打开"也关掉，避免后续打开时拼出空 href + _blank
    // 的怪状态——后端不会 reject 这种组合，但前端体验更干净
    const linkBlank = (inputs[KEYS.link] || '').trim() === '';
    if (linkBlank && inputs[KEYS.openInNewTab]) {
      setInputs((prev) => ({ ...prev, [KEYS.openInNewTab]: false }));
      refForm.current?.setValue(ff(KEYS.openInNewTab), false);
    }
    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value: String(inputs[item.key]),
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
    // 以 DEFAULTS 为底，仅当 props.options 同 key 有值时覆盖。
    // 这样即便老部署还没重启过 InitOptionMap、API 没回种 announcement_bar.*
    // 的默认值，表单也能完整工作；保存时 compareObjects 也能正确比对所有 key
    const merged = { ...DEFAULTS };
    for (const key in props.options) {
      if (Object.prototype.hasOwnProperty.call(merged, key)) {
        merged[key] = props.options[key];
      }
    }
    setInputs(merged);
    setInputsRow(structuredClone(merged));
    refForm.current?.setValues(merged);
  }, [props.options]);

  const previewVisible =
    inputs[KEYS.enabled] && (inputs[KEYS.content] || '').trim().length > 0;
  const linkValid =
    !inputs[KEYS.link] ||
    /^https?:\/\//i.test(inputs[KEYS.link].trim());
  const contentLen = (inputs[KEYS.content] || '').length;

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('公告横幅')}>
          <Typography.Text
            type='tertiary'
            style={{ marginBottom: 12, display: 'block' }}
          >
            {t(
              '在站点顶部展示一行全站公告，登录前 / 登录后均可见。用户点击 X 关闭后该版本不再提示；只要你修改文案 / 链接，所有用户会再次看到。',
            )}
          </Typography.Text>

          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={ff(KEYS.enabled)}
                label={t('启用公告横幅')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange(KEYS.enabled)}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={ff(KEYS.openInNewTab)}
                label={t('在新标签页打开')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange(KEYS.openInNewTab)}
                disabled={
                  !inputs[KEYS.enabled] ||
                  (inputs[KEYS.link] || '').trim() === ''
                }
              />
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 4 }}>
            <Col xs={24}>
              <Form.TextArea
                field={ff(KEYS.content)}
                label={t('公告文案')}
                placeholder={t(
                  '建议一句话，简短醒目；最多 500 字，仅支持纯文本',
                )}
                rows={2}
                maxLength={500}
                showClear
                onChange={handleFieldChange(KEYS.content)}
                disabled={!inputs[KEYS.enabled]}
                extraText={
                  <Typography.Text type='tertiary' size='small'>
                    {contentLen} / 500
                  </Typography.Text>
                }
              />
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24}>
              <Form.Input
                field={ff(KEYS.link)}
                label={t('点击跳转链接（可选）')}
                placeholder='https://example.com/changelog'
                showClear
                onChange={handleFieldChange(KEYS.link)}
                disabled={!inputs[KEYS.enabled]}
                validateStatus={linkValid ? 'default' : 'error'}
                extraText={
                  !linkValid ? (
                    <Typography.Text type='danger' size='small'>
                      {t('链接必须以 http:// 或 https:// 开头')}
                    </Typography.Text>
                  ) : (
                    <Typography.Text type='tertiary' size='small'>
                      {t('留空则横幅仅展示文案、不可点击')}
                    </Typography.Text>
                  )
                }
              />
            </Col>
          </Row>

          {/* 颜色三件套：背景主色 / 高光 / 文字。深浅 stops 用 color-mix
              从主色派生，所以管理员只要选这 3 个色就够；不暴露调色盘 */}
          <Row
            gutter={16}
            style={{ marginTop: 4, alignItems: 'flex-end' }}
          >
            <Col xs={24} sm={8}>
              <Typography.Text style={{ display: 'block', marginBottom: 6 }}>
                {t('背景主色')}
              </Typography.Text>
              <ColorField
                value={inputs[KEYS.bgColor]}
                onChange={(v) => {
                  setInputs((prev) => ({ ...prev, [KEYS.bgColor]: v }));
                }}
              />
            </Col>
            <Col xs={24} sm={8}>
              <Typography.Text style={{ display: 'block', marginBottom: 6 }}>
                {t('高光色')}
              </Typography.Text>
              <ColorField
                value={inputs[KEYS.accentColor]}
                onChange={(v) => {
                  setInputs((prev) => ({ ...prev, [KEYS.accentColor]: v }));
                }}
              />
            </Col>
            <Col xs={24} sm={8}>
              <Typography.Text style={{ display: 'block', marginBottom: 6 }}>
                {t('文字色')}
              </Typography.Text>
              <ColorField
                value={inputs[KEYS.textColor]}
                onChange={(v) => {
                  setInputs((prev) => ({ ...prev, [KEYS.textColor]: v }));
                }}
              />
            </Col>
          </Row>

          <Row style={{ marginTop: 8 }}>
            <Col xs={24}>
              <Button
                size='small'
                theme='borderless'
                icon={<RotateCcw size={14} />}
                onClick={() => {
                  setInputs((prev) => ({
                    ...prev,
                    [KEYS.bgColor]: COLOR_DEFAULTS.bg,
                    [KEYS.accentColor]: COLOR_DEFAULTS.accent,
                    [KEYS.textColor]: COLOR_DEFAULTS.text,
                  }));
                }}
              >
                {t('重置为默认配色')}
              </Button>
              <Typography.Text type='tertiary' size='small'>
                {t(
                  '深浅渐变会按背景主色自动派生；高光色用于光泽带和左侧光池。',
                )}
              </Typography.Text>
            </Col>
          </Row>

          {/* 实时预览：复刻 AnnouncementBar 视觉，避免 admin 改完才通过把横幅
              真正打开来"试看"——尤其在已 dismiss 当前 version 的情况下，
              admin 自己也得开无痕窗口才能看到效果，体验糟糕。
              用 inline CSS 变量驱动颜色，与上线后逻辑完全一致 */}
          {previewVisible && (
            <Row gutter={16} style={{ marginTop: 12 }}>
              <Col xs={24}>
                <Typography.Text
                  type='tertiary'
                  size='small'
                  style={{ marginBottom: 6, display: 'block' }}
                >
                  {t('预览')}
                </Typography.Text>
                <div
                  className='announcement-bar announcement-bar--preview'
                  style={{
                    '--ab-bg': isHex(inputs[KEYS.bgColor])
                      ? inputs[KEYS.bgColor]
                      : COLOR_DEFAULTS.bg,
                    '--ab-accent': isHex(inputs[KEYS.accentColor])
                      ? inputs[KEYS.accentColor]
                      : COLOR_DEFAULTS.accent,
                    '--ab-text': isHex(inputs[KEYS.textColor])
                      ? inputs[KEYS.textColor]
                      : COLOR_DEFAULTS.text,
                  }}
                >
                  <div
                    className={`announcement-bar__inner${
                      inputs[KEYS.link]
                        ? ' announcement-bar__inner--clickable'
                        : ''
                    }`}
                  >
                    <Megaphone size={16} className='announcement-bar__icon' />
                    <span className='announcement-bar__text'>
                      {inputs[KEYS.content]}
                    </span>
                  </div>
                  <button
                    type='button'
                    className='announcement-bar__close'
                    aria-hidden='true'
                  >
                    <X size={14} />
                  </button>
                </div>
              </Col>
            </Row>
          )}

          <Row style={{ marginTop: 12 }}>
            <Space>
              <Button size='default' type='primary' onClick={onSubmit}>
                {t('保存公告横幅设置')}
              </Button>
            </Space>
          </Row>

          <Banner
            type='info'
            description={t(
              '版本号由系统按内容自动生成。任意可见字段变化都会触发所有用户重新看到横幅，包括之前已点击关闭的用户。',
            )}
            style={{ marginTop: 12 }}
            closeIcon={null}
          />
        </Form.Section>
      </Form>
    </Spin>
  );
}
