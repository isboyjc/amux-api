/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Button,
  Modal,
  Input,
  Form,
  Tag,
  Typography,
  Empty,
  Tabs,
  TabPane,
  Spin,
  Avatar,
} from '@douyinfe/semi-ui';
import { IconKey, IconCopy } from '@douyinfe/semi-icons';
import {
  RotateCw,
  Trash2,
  Plus,
  Smartphone,
  Terminal,
  ShieldCheck,
} from 'lucide-react';
import { API, copy, showError, showSuccess, timestamp2string } from '../../../../helpers';

const SOURCE_PAT = 'manual';
const SOURCE_LEGACY = 'legacy';
const SOURCE_DEVICE_FLOW = 'device-flow';

// 状态码 → 标签 props
const STATUS_PROPS = {
  1: { color: 'green', text: '生效中' },   // active
  2: { color: 'red', text: '已撤销' },     // revoked
  3: { color: 'grey', text: '已过期' },    // expired
};

// 用户管理自己的 access token 列表（PAT + 已授权应用聚合到两个 tab）
export default function AccessTokenManagement() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [tokens, setTokens] = useState([]);
  const [createOpen, setCreateOpen] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [revealedToken, setRevealedToken] = useState(null); // 创建/旋转后唯一一次展示
  const [tab, setTab] = useState('pat');

  const refresh = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/user/access_tokens');
      if (res.data.success) {
        setTokens(res.data.data || []);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  // 按来源分组；已撤销（status === 2）的不展示
  const { patTokens, oauthTokens } = useMemo(() => {
    const pat = [];
    const oauth = [];
    (tokens || []).forEach((tk) => {
      if (tk.status === 2) return;
      if (tk.source === SOURCE_DEVICE_FLOW) oauth.push(tk);
      else pat.push(tk); // manual + legacy 都归到 PAT 视图
    });
    return { patTokens: pat, oauthTokens: oauth };
  }, [tokens]);

  const handleCreate = async (values) => {
    setCreateSubmitting(true);
    try {
      const payload = {
        name: values.name,
        description: values.description || '',
      };
      const exp = values.expires;
      if (exp === 'never') payload.expires_in_days = 0;
      else if (exp === '30') payload.expires_in_days = 30;
      else if (exp === '60') payload.expires_in_days = 60;
      else payload.expires_in_days = 90;
      const res = await API.post('/api/user/access_tokens', payload);
      if (res.data.success) {
        setRevealedToken(res.data.data.plaintext_token);
        setCreateOpen(false);
        await refresh();
      } else {
        showError(res.data.message || t('创建失败'));
      }
    } catch (e) {
      showError(t('创建失败'));
    } finally {
      setCreateSubmitting(false);
    }
  };

  const handleRevoke = async (id) => {
    Modal.confirm({
      title: t('撤销该令牌？'),
      content: t('撤销后立即失效，使用此令牌的脚本/客户端将无法继续工作。'),
      okText: t('撤销'),
      okType: 'danger',
      cancelText: t('取消'),
      onOk: async () => {
        try {
          const res = await API.delete(`/api/user/access_tokens/${id}`);
          if (res.data.success) {
            showSuccess(t('已撤销'));
            await refresh();
          } else {
            showError(res.data.message);
          }
        } catch (e) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const handleRotate = async (id) => {
    Modal.confirm({
      title: t('刷新该令牌？'),
      content: t('刷新后旧令牌立即失效，新令牌仅会展示一次，请立刻保存。'),
      okText: t('继续'),
      cancelText: t('取消'),
      onOk: async () => {
        try {
          const res = await API.post(`/api/user/access_tokens/${id}/rotate`);
          if (res.data.success) {
            setRevealedToken(res.data.data.plaintext_token);
            await refresh();
          } else {
            showError(res.data.message);
          }
        } catch (e) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const handleCopyToken = async (text) => {
    const ok = await copy(text);
    if (ok) showSuccess(t('已复制到剪贴板'));
  };

  const renderTokenRow = (tk, kind /* 'pat' | 'oauth' */) => {
    const sp = STATUS_PROPS[tk.status] || { color: 'grey', text: '-' };
    const isActive = tk.status === 1;
    const app = kind === 'oauth' ? tk.client_app : null;
    // OAuth tab 优先把 app 信息当主标题；PAT tab 直接用用户起的名字
    const displayName = app?.name || tk.name;
    return (
      <Card
        key={tk.id}
        className='!rounded-xl'
        bodyStyle={{ padding: '12px 16px' }}
      >
        <div className='flex items-start justify-between gap-3'>
          <div className='flex items-start flex-1 min-w-0 gap-3'>
            {kind === 'oauth' && app?.logo_url ? (
              <Avatar
                shape='square'
                src={app.logo_url}
                size='small'
                style={{ borderRadius: 10, flexShrink: 0 }}
              >
                {(app.name || '?').slice(0, 2).toUpperCase()}
              </Avatar>
            ) : (
              <div className='w-10 h-10 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center flex-shrink-0'>
                {kind === 'pat' ? (
                  <Terminal size={18} className='text-slate-600 dark:text-slate-300' />
                ) : (
                  <Smartphone size={18} className='text-slate-600 dark:text-slate-300' />
                )}
              </div>
            )}
            <div className='flex-1 min-w-0'>
              <div className='flex items-center gap-2 flex-wrap mb-1'>
                <Typography.Text strong className='truncate'>
                  {displayName}
                </Typography.Text>
                <Tag size='small' color={sp.color}>{t(sp.text)}</Tag>
                {kind === 'oauth' && app?.verified && (
                  <Tag
                    size='small'
                    color='blue'
                    prefixIcon={<ShieldCheck size={12} />}
                  >
                    {t('已认证')}
                  </Tag>
                )}
                {tk.source === SOURCE_LEGACY && (
                  <Tag size='small' color='orange'>{t('迁移自旧版')}</Tag>
                )}
              </div>
              {kind === 'pat' && (
                <div className='flex items-center gap-1 mb-1'>
                  <Typography.Text
                    type='tertiary'
                    className='text-xs'
                    style={{ fontFamily: 'monospace' }}
                  >
                    {tk.token_prefix}
                    {'••••••••'}
                  </Typography.Text>
                </div>
              )}
              <Typography.Text type='tertiary' className='text-xs block'>
                {t('创建于')} {timestamp2string(tk.created_at)}
                {tk.expires_at ? (
                  <>
                    <span className='mx-1'>·</span>
                    {t('过期于')} {timestamp2string(tk.expires_at)}
                  </>
                ) : (
                  <>
                    <span className='mx-1'>·</span>
                    {t('永不过期')}
                  </>
                )}
                {tk.last_used_at && (
                  <>
                    <span className='mx-1'>·</span>
                    {t('上次使用')} {timestamp2string(tk.last_used_at)}
                  </>
                )}
              </Typography.Text>
              {kind === 'oauth' && tk.authorized_ip && (
                <Typography.Text type='tertiary' className='text-xs block'>
                  {t('授权来自')} {tk.authorized_ip}
                </Typography.Text>
              )}
            </div>
          </div>
          <div className='flex gap-2 flex-shrink-0'>
            {kind === 'pat' && isActive && (
              <Button
                size='small'
                theme='outline'
                icon={<RotateCw size={14} />}
                onClick={() => handleRotate(tk.id)}
              >
                {t('刷新')}
              </Button>
            )}
            <Button
              size='small'
              type='danger'
              theme='outline'
              icon={<Trash2 size={14} />}
              onClick={() => handleRevoke(tk.id)}
            >
              {kind === 'oauth' ? t('解除授权') : t('撤销')}
            </Button>
          </div>
        </div>
      </Card>
    );
  };

  const renderEmpty = (text) => (
    <div className='py-10'>
      <Empty
        image={
          <div className='w-16 h-16 rounded-full bg-slate-100 flex items-center justify-center mx-auto'>
            <IconKey size='extra-large' className='text-slate-400' />
          </div>
        }
        description={
          <Typography.Text type='tertiary' className='text-sm'>
            {text}
          </Typography.Text>
        }
      />
    </div>
  );

  return (
    <Card className='!rounded-xl w-full'>
      {/* 卡片头部：图标 + 标题 + 描述（与同级安全设置卡片对齐） */}
      <div className='flex flex-col sm:flex-row items-start sm:justify-between gap-4 mb-4'>
        <div className='flex items-start w-full sm:w-auto'>
          <div className='w-12 h-12 rounded-full bg-slate-100 flex items-center justify-center mr-4 flex-shrink-0'>
            <IconKey size='large' className='text-slate-600' />
          </div>
          <div>
            <Typography.Title heading={6} className='mb-1'>
              {t('访问令牌与已授权应用')}
            </Typography.Title>
            <Typography.Text type='tertiary' className='text-sm'>
              {t('管理用于脚本调用的个人访问令牌，以及已授权访问账户的第三方应用')}
            </Typography.Text>
          </div>
        </div>
      </div>

      <Tabs
        type='line'
        activeKey={tab}
        onChange={setTab}
        tabBarExtraContent={
          tab === 'pat' && (
            <Button
              type='primary'
              theme='solid'
              icon={<Plus size={14} />}
              onClick={() => setCreateOpen(true)}
              className='!bg-slate-600 hover:!bg-slate-700'
            >
              {t('新建令牌')}
            </Button>
          )
        }
      >
        <TabPane
          tab={
            <span className='inline-flex items-center gap-1.5'>
              <Terminal size={14} /> {t('访问令牌')}
            </span>
          }
          itemKey='pat'
        >
          <Typography.Text type='tertiary' className='text-xs block mt-4 mb-3'>
            {t('用于脚本/CLI 调用平台管理 API。请妥善保存，不要写进公共代码仓库。')}
          </Typography.Text>
          {loading ? (
            <div className='py-10 flex justify-center'>
              <Spin />
            </div>
          ) : patTokens.length === 0 ? (
            renderEmpty(t('尚未创建任何访问令牌'))
          ) : (
            <div className='flex flex-col gap-3'>
              {patTokens.map((tk) => renderTokenRow(tk, 'pat'))}
            </div>
          )}
        </TabPane>
        <TabPane
          tab={
            <span className='inline-flex items-center gap-1.5'>
              <Smartphone size={14} /> {t('已授权应用')}
            </span>
          }
          itemKey='oauth'
        >
          <Typography.Text type='tertiary' className='text-xs block mt-4 mb-3'>
            {t('通过 OAuth 授权代你访问账户的第三方应用 / 桌面客户端。')}
          </Typography.Text>
          {loading ? (
            <div className='py-10 flex justify-center'>
              <Spin />
            </div>
          ) : oauthTokens.length === 0 ? (
            renderEmpty(t('尚未授权任何应用'))
          ) : (
            <div className='flex flex-col gap-3'>
              {oauthTokens.map((tk) => renderTokenRow(tk, 'oauth'))}
            </div>
          )}
        </TabPane>
      </Tabs>

      {/* 创建令牌 Modal */}
      <Modal
        title={t('新建访问令牌')}
        visible={createOpen}
        onCancel={() => setCreateOpen(false)}
        footer={null}
      >
        <Form onSubmit={handleCreate} initValues={{ expires: '90' }}>
          <Form.Input
            field='name'
            label={t('名称')}
            placeholder={t('如：My CLI')}
            rules={[{ required: true, message: t('请输入名称') }]}
            maxLength={64}
          />
          <Form.Input
            field='description'
            label={t('描述（可选）')}
            placeholder={t('用于备忘')}
            maxLength={255}
          />
          <Form.RadioGroup
            field='expires'
            label={t('有效期')}
            type='button'
          >
            <Form.Radio value='30'>{t('30 天')}</Form.Radio>
            <Form.Radio value='60'>{t('60 天')}</Form.Radio>
            <Form.Radio value='90'>{t('90 天')}</Form.Radio>
            <Form.Radio value='never'>{t('永不过期')}</Form.Radio>
          </Form.RadioGroup>
          <div className='flex justify-end gap-2 mt-4'>
            <Button onClick={() => setCreateOpen(false)}>{t('取消')}</Button>
            <Button
              theme='solid'
              type='primary'
              htmlType='submit'
              loading={createSubmitting}
            >
              {t('创建')}
            </Button>
          </div>
        </Form>
      </Modal>

      {/* 一次性展示新 token Modal */}
      <Modal
        title={t('请立即复制并保存令牌')}
        visible={!!revealedToken}
        onCancel={() => setRevealedToken(null)}
        closeOnEsc={false}
        maskClosable={false}
        footer={
          <Button
            theme='solid'
            type='primary'
            onClick={() => setRevealedToken(null)}
          >
            {t('我已保存')}
          </Button>
        }
      >
        <Typography.Text type='warning' className='block mb-3'>
          ⚠ {t('此令牌仅在本次显示。关闭后无法再次查看，请立刻复制保存。')}
        </Typography.Text>
        <Input
          value={revealedToken || ''}
          readonly
          size='large'
          suffix={
            <Button
              type='primary'
              theme='borderless'
              icon={<IconCopy />}
              onClick={() => handleCopyToken(revealedToken)}
            >
              {t('复制')}
            </Button>
          }
        />
      </Modal>
    </Card>
  );
}
