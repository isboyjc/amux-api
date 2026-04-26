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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Plus,
  Trash2,
  X,
} from 'lucide-react';
import {
  Banner,
  Button,
  Col,
  Form,
  Input,
  Row,
  Space,
  Spin,
  Switch,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import { SIDEBAR_CAROUSEL_GRADIENTS } from '../../../components/layout/SidebarCarousel';

// 后端 setting/operation_setting/sidebar_carousel_setting.go 的字段对齐
const KEY_ENABLED = 'sidebar_carousel.enabled';
const KEY_ITEMS = 'sidebar_carousel.items';

const MAX_ITEMS = 5;

// 仅存 value，label 在渲染时调 t() 用字面量——i18next-cli 只能识别字面量
const OVERLAY_VALUES = ['dark', 'light'];

// 单条轮播项的初始值。新增时调用 → 字段齐全，避免渲染时 nullish 检查
const blankItem = () => ({
  title: '',
  description: '',
  cta_text: '',
  link: '',
  open_in_new_tab: false,
  bg_url: '',
  bg_preset_index: 0,
  overlay: 'dark',
});

// 站内 / http(s) 链接通过；其它（javascript:、相对路径但无前导 /）不通过
const isLinkAllowed = (s) => {
  const v = (s || '').trim();
  if (!v) return true;
  return /^https?:\/\//i.test(v) || v.startsWith('/');
};

const isVideoUrl = (url) => /\.(mp4|webm|mov|ogv|m4v)(\?|#|$)/i.test(url || '');

export default function SettingsSidebarCarousel(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [enabled, setEnabled] = useState(false);
  const [enabledRaw, setEnabledRaw] = useState(false);
  const [items, setItems] = useState([]);
  const [itemsRaw, setItemsRaw] = useState('[]');
  // 预览选中第几条；items 变更时越界保护
  const [previewIdx, setPreviewIdx] = useState(0);
  const formRef = useRef();

  // 把 props.options 同步到本地表单状态。**raw 版本**保留服务端原值用于
  // diff，避免没改也提交
  useEffect(() => {
    const en =
      props.options?.[KEY_ENABLED] === true ||
      props.options?.[KEY_ENABLED] === 'true';
    setEnabled(en);
    setEnabledRaw(en);
    const rawItems = props.options?.[KEY_ITEMS];
    setItemsRaw(typeof rawItems === 'string' ? rawItems : '[]');
    let parsed = [];
    try {
      parsed = rawItems ? JSON.parse(rawItems) : [];
      if (!Array.isArray(parsed)) parsed = [];
    } catch {
      parsed = [];
    }
    setItems(
      parsed.slice(0, MAX_ITEMS).map((it) => ({ ...blankItem(), ...it })),
    );
  }, [props.options]);

  // items 数量变化时矫正预览索引，避免删除最后一条后预览崩
  useEffect(() => {
    if (previewIdx >= items.length && items.length > 0) {
      setPreviewIdx(0);
    }
  }, [items.length, previewIdx]);

  const updateItem = (index, patch) => {
    setItems((prev) =>
      prev.map((it, i) => (i === index ? { ...it, ...patch } : it)),
    );
  };

  const removeItem = (index) => {
    setItems((prev) => prev.filter((_, i) => i !== index));
  };

  const moveItem = (index, dir) => {
    setItems((prev) => {
      const next = [...prev];
      const target = index + dir;
      if (target < 0 || target >= next.length) return prev;
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
  };

  const addItem = () => {
    if (items.length >= MAX_ITEMS) return;
    setItems((prev) => [...prev, blankItem()]);
  };

  // 提交前的本地校验：可选字段为空 OK，但如果填了就必须合法。后端会再校验
  // 一遍——本地这层只是给 admin 立刻反馈
  const validate = () => {
    if (enabled && items.length === 0) {
      return t('请至少添加一个轮播项，或关闭"启用宣传位轮播"');
    }
    for (let i = 0; i < items.length; i++) {
      const it = items[i];
      if (!it.title || !it.title.trim()) {
        return t('第 {{n}} 项的标题不能为空', { n: i + 1 });
      }
      if (!isLinkAllowed(it.link)) {
        return t(
          '第 {{n}} 项的跳转链接必须以 http(s):// 开头，或以 / 开头的站内路径',
          { n: i + 1 },
        );
      }
      if (it.bg_url && !isLinkAllowed(it.bg_url)) {
        return t(
          '第 {{n}} 项的背景 URL 必须以 http(s):// 开头，或以 / 开头的站内静态路径',
          { n: i + 1 },
        );
      }
      if (
        !Number.isInteger(it.bg_preset_index) ||
        it.bg_preset_index < 0 ||
        it.bg_preset_index >= SIDEBAR_CAROUSEL_GRADIENTS.length
      ) {
        return t('第 {{n}} 项的预置渐变下标无效', { n: i + 1 });
      }
    }
    return null;
  };

  const onSubmit = async () => {
    const err = validate();
    if (err) {
      showError(err);
      return;
    }
    const requests = [];
    const newItemsStr = JSON.stringify(items);
    if (enabled !== enabledRaw) {
      requests.push(
        API.put('/api/option/', {
          key: KEY_ENABLED,
          value: String(enabled),
        }),
      );
    }
    if (newItemsStr !== itemsRaw) {
      requests.push(
        API.put('/api/option/', { key: KEY_ITEMS, value: newItemsStr }),
      );
    }
    if (requests.length === 0) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }
    setLoading(true);
    try {
      const results = await Promise.all(requests);
      // 后端在 success=false 时返回 200 + message；统一兜底
      const failed = results.find((r) => r?.data && r.data.success === false);
      if (failed) {
        showError(failed.data.message || t('保存失败，请重试'));
      } else {
        showSuccess(t('保存成功'));
        props.refresh && props.refresh();
      }
    } catch {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const previewItem = items[previewIdx] || null;

  // 预览背景 inline style：bg_url 优先，否则用预置渐变
  const previewMedia = useMemo(() => {
    if (!previewItem) return null;
    if (previewItem.bg_url) {
      if (isVideoUrl(previewItem.bg_url)) {
        return (
          <video
            className='sidebar-carousel__media'
            src={previewItem.bg_url}
            autoPlay
            muted
            loop
            playsInline
          />
        );
      }
      return (
        <img
          className='sidebar-carousel__media'
          src={previewItem.bg_url}
          alt=''
        />
      );
    }
    return (
      <div
        className='sidebar-carousel__media'
        style={{
          background: SIDEBAR_CAROUSEL_GRADIENTS[previewItem.bg_preset_index],
        }}
      />
    );
  }, [previewItem]);

  return (
    <Spin spinning={loading}>
      <Form
        getFormApi={(api) => (formRef.current = api)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('侧边栏底部宣传位')}>
          <Typography.Text
            type='tertiary'
            style={{ marginBottom: 12, display: 'block' }}
          >
            {t(
              '在控制台侧边栏底部展示一张可轮播的宣传卡片，最多 5 项。用户点击 X 关闭后该版本不再提示；任意可见字段变更会自动 bump 版本，所有用户会再次看到。',
            )}
          </Typography.Text>

          <Row gutter={16} style={{ marginBottom: 8 }}>
            <Col xs={24} sm={12} md={8}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <Switch
                  checked={enabled}
                  onChange={setEnabled}
                  checkedText='｜'
                  uncheckedText='〇'
                />
                <Typography.Text>
                  {enabled ? t('已启用') : t('已禁用')}
                </Typography.Text>
              </div>
            </Col>
            <Col xs={24} sm={12} md={16}>
              <Typography.Text type='tertiary' size='small'>
                {t('已配置 {{n}} / {{m}} 项', {
                  n: items.length,
                  m: MAX_ITEMS,
                })}
              </Typography.Text>
            </Col>
          </Row>

          {/* 轮播项编辑列表 */}
          {items.map((it, idx) => (
            <div
              key={idx}
              style={{
                marginTop: 12,
                padding: 12,
                border: '1px solid var(--semi-color-border)',
                borderRadius: 8,
                background: 'var(--semi-color-fill-0)',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  marginBottom: 8,
                }}
              >
                <Tag color='blue' shape='circle'>
                  {t('第 {{n}} 项', { n: idx + 1 })}
                </Tag>
                <Space>
                  <Button
                    size='small'
                    theme='borderless'
                    icon={<ArrowUp size={14} />}
                    disabled={idx === 0}
                    onClick={() => moveItem(idx, -1)}
                  />
                  <Button
                    size='small'
                    theme='borderless'
                    icon={<ArrowDown size={14} />}
                    disabled={idx === items.length - 1}
                    onClick={() => moveItem(idx, 1)}
                  />
                  <Button
                    size='small'
                    type='danger'
                    theme='borderless'
                    icon={<Trash2 size={14} />}
                    onClick={() => removeItem(idx)}
                  >
                    {t('删除')}
                  </Button>
                </Space>
              </div>

              <Row gutter={16}>
                <Col xs={24} sm={12}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('标题')}
                  </Typography.Text>
                  <Input
                    value={it.title}
                    onChange={(v) => updateItem(idx, { title: v })}
                    placeholder={t('限时活动')}
                    maxLength={50}
                    showClear
                  />
                </Col>
                <Col xs={24} sm={12}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('副标题')}
                  </Typography.Text>
                  <Input
                    value={it.description}
                    onChange={(v) => updateItem(idx, { description: v })}
                    placeholder={t('一句话介绍，可留空')}
                    maxLength={80}
                    showClear
                  />
                </Col>
              </Row>

              <Row gutter={16} style={{ marginTop: 8 }}>
                <Col xs={24} sm={12}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('引导文案（CTA）')}
                  </Typography.Text>
                  <Input
                    value={it.cta_text}
                    onChange={(v) => updateItem(idx, { cta_text: v })}
                    placeholder={t('立即体验')}
                    maxLength={20}
                    showClear
                  />
                </Col>
                <Col xs={24} sm={12}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('跳转链接')}
                  </Typography.Text>
                  <Input
                    value={it.link}
                    onChange={(v) => updateItem(idx, { link: v })}
                    placeholder='/console/playground 或 https://...'
                    showClear
                    validateStatus={isLinkAllowed(it.link) ? 'default' : 'error'}
                  />
                </Col>
              </Row>

              <Row gutter={16} style={{ marginTop: 8 }}>
                <Col xs={24} sm={16}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('背景 URL（可选，支持图片 / GIF / 视频）')}
                  </Typography.Text>
                  <Input
                    value={it.bg_url}
                    onChange={(v) => updateItem(idx, { bg_url: v })}
                    placeholder='https://example.com/banner.webp'
                    showClear
                    validateStatus={
                      isLinkAllowed(it.bg_url) ? 'default' : 'error'
                    }
                  />
                </Col>
                <Col
                  xs={24}
                  sm={8}
                  style={{ display: 'flex', alignItems: 'flex-end' }}
                >
                  <Switch
                    checked={!!it.open_in_new_tab}
                    onChange={(v) => updateItem(idx, { open_in_new_tab: v })}
                    disabled={
                      !it.link ||
                      !/^https?:\/\//i.test((it.link || '').trim())
                    }
                  />
                  <Typography.Text style={{ marginLeft: 8 }}>
                    {t('在新标签页打开（仅外链生效）')}
                  </Typography.Text>
                </Col>
              </Row>

              <Row gutter={16} style={{ marginTop: 12 }}>
                <Col xs={24} sm={16}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('预置渐变背景（背景 URL 留空时使用）')}
                  </Typography.Text>
                  <Space>
                    {SIDEBAR_CAROUSEL_GRADIENTS.map((g, gi) => {
                      const selected = it.bg_preset_index === gi && !it.bg_url;
                      return (
                        <button
                          key={gi}
                          type='button'
                          aria-label={t('预置 {{n}}', { n: gi + 1 })}
                          onClick={() => updateItem(idx, { bg_preset_index: gi })}
                          style={{
                            width: 44,
                            height: 28,
                            borderRadius: 6,
                            border: selected
                              ? '2px solid var(--semi-color-primary)'
                              : '1px solid var(--semi-color-border)',
                            background: g,
                            cursor: 'pointer',
                            padding: 0,
                            opacity: it.bg_url ? 0.5 : 1,
                          }}
                        />
                      );
                    })}
                  </Space>
                </Col>
                <Col xs={24} sm={8}>
                  <Typography.Text style={{ display: 'block', marginBottom: 4 }}>
                    {t('文字蒙层')}
                  </Typography.Text>
                  <Space>
                    {OVERLAY_VALUES.map((val) => (
                      <Tag
                        key={val}
                        color={it.overlay === val ? 'blue' : 'grey'}
                        type={it.overlay === val ? 'solid' : 'light'}
                        onClick={() => updateItem(idx, { overlay: val })}
                        style={{ cursor: 'pointer' }}
                      >
                        {val === 'dark' ? t('深色蒙层') : t('浅色蒙层')}
                      </Tag>
                    ))}
                  </Space>
                </Col>
              </Row>
            </div>
          ))}

          <Row style={{ marginTop: 12 }}>
            <Col>
              <Button
                icon={<Plus size={14} />}
                onClick={addItem}
                disabled={items.length >= MAX_ITEMS}
                theme='light'
                type='primary'
              >
                {t('添加轮播项')}
              </Button>
              {items.length >= MAX_ITEMS && (
                <Typography.Text
                  type='tertiary'
                  size='small'
                  style={{ marginLeft: 8 }}
                >
                  {t('已达上限 {{m}} 项', { m: MAX_ITEMS })}
                </Typography.Text>
              )}
            </Col>
          </Row>

          {/* 实时预览：复刻 SidebarCarousel 视觉，没启用 / 没条目时 hide */}
          {items.length > 0 && (
            <Row gutter={16} style={{ marginTop: 16 }}>
              <Col xs={24}>
                <Typography.Text
                  type='tertiary'
                  size='small'
                  style={{ marginBottom: 6, display: 'block' }}
                >
                  {t('预览（侧边栏中的真实尺寸）')}
                </Typography.Text>
                <div style={{ width: 204 }}>
                  <div className='sidebar-carousel'>
                    <div
                      className={`sidebar-carousel__card sidebar-carousel__card--${
                        previewItem?.overlay || 'dark'
                      }${previewItem?.link ? ' sidebar-carousel__card--clickable' : ''}`}
                    >
                      <div className='sidebar-carousel__bg'>{previewMedia}</div>
                      <div className='sidebar-carousel__scrim' />
                      <button
                        type='button'
                        className='sidebar-carousel__close'
                        aria-hidden='true'
                        tabIndex={-1}
                      >
                        <X size={12} />
                      </button>
                      <div className='sidebar-carousel__content'>
                        <div className='sidebar-carousel__top'>
                          <div className='sidebar-carousel__title'>
                            {previewItem?.title || t('（标题预览）')}
                          </div>
                          {previewItem?.description && (
                            <div className='sidebar-carousel__desc'>
                              {previewItem.description}
                            </div>
                          )}
                        </div>
                        <div className='sidebar-carousel__bottom'>
                          <span className='sidebar-carousel__cta'>
                            {previewItem?.cta_text || t('了解更多')}
                            <ArrowRight
                              size={12}
                              className='sidebar-carousel__cta-icon'
                            />
                          </span>
                          {items.length > 1 && (
                            <div className='sidebar-carousel__dots'>
                              {items.map((_, i) => (
                                <button
                                  key={i}
                                  type='button'
                                  onClick={() => setPreviewIdx(i)}
                                  className={`sidebar-carousel__dot${
                                    i === previewIdx
                                      ? ' sidebar-carousel__dot--active'
                                      : ''
                                  }`}
                                />
                              ))}
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </Col>
            </Row>
          )}

          <Row style={{ marginTop: 16 }}>
            <Space>
              <Button type='primary' onClick={onSubmit}>
                {t('保存宣传位设置')}
              </Button>
            </Space>
          </Row>

          <Banner
            type='info'
            description={t(
              '版本号由系统按内容自动生成。任意可见字段变化都会触发所有用户重新看到宣传位，包括之前已点击关闭的用户。',
            )}
            style={{ marginTop: 12 }}
            closeIcon={null}
          />
        </Form.Section>
      </Form>
    </Spin>
  );
}
