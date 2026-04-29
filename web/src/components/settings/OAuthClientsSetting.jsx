/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Modal,
  Form,
  Input,
  Tag,
  Typography,
  Avatar,
  Empty,
  Spin,
} from '@douyinfe/semi-ui';
import { Plus, RotateCw, Trash2, Pencil, KeyRound, ShieldCheck } from 'lucide-react';
import { IconCopy } from '@douyinfe/semi-icons';
import { API, copy, showError, showSuccess, timestamp2string } from '../../helpers';

const STATUS = {
  1: { color: 'green', text: 'Active' },
  2: { color: 'red', text: 'Disabled' },
};

export default function OAuthClientsSetting() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState([]);
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [revealedSecret, setRevealedSecret] = useState(null); // {client_id, secret}

  const refresh = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/admin/oauth/clients');
      if (res.data.success) setRows(res.data.data || []);
      else showError(res.data.message || t('加载失败'));
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const handleCreate = async (values) => {
    setSubmitting(true);
    try {
      const res = await API.post('/api/admin/oauth/clients', values);
      if (res.data.success) {
        setCreateOpen(false);
        setRevealedSecret({
          client_id: res.data.data.client_id,
          secret: res.data.data.client_secret,
        });
        await refresh();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(t('创建失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleEdit = async (values) => {
    if (!editTarget) return;
    setSubmitting(true);
    try {
      const res = await API.patch(`/api/admin/oauth/clients/${editTarget.id}`, values);
      if (res.data.success) {
        setEditOpen(false);
        setEditTarget(null);
        await refresh();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(t('更新失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleRotate = async (id) => {
    Modal.confirm({
      title: t('轮换该应用的 client_secret？'),
      content: t('旧的 secret 立即失效；新的仅展示一次，请立刻保存。'),
      onOk: async () => {
        try {
          const res = await API.post(`/api/admin/oauth/clients/${id}/rotate`);
          if (res.data.success) {
            const r = rows.find((x) => x.id === id);
            setRevealedSecret({
              client_id: r?.client_id || '',
              secret: res.data.data.client_secret,
            });
          } else {
            showError(res.data.message);
          }
        } catch (e) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const handleDisable = async (row) => {
    Modal.confirm({
      title: t('禁用该应用？'),
      content: t('禁用后不再受理新的授权请求；已签发的 OAT 不在此撤销。'),
      okType: 'danger',
      onOk: async () => {
        try {
          const res = await API.delete(`/api/admin/oauth/clients/${row.id}`);
          if (res.data.success) {
            showSuccess(t('已禁用'));
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

  const handleEnable = async (row) => {
    try {
      const res = await API.patch(`/api/admin/oauth/clients/${row.id}`, { status: 1 });
      if (res.data.success) {
        await refresh();
        showSuccess(t('已启用'));
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  const handleCopySecret = async () => {
    if (!revealedSecret) return;
    const ok = await copy(revealedSecret.secret);
    if (ok) showSuccess(t('已复制'));
  };

  const handleCopyClientId = async (clientId) => {
    const ok = await copy(clientId);
    if (ok) showSuccess(t('已复制'));
  };

  const renderClientRow = (r) => {
    const sp = STATUS[r.status] || { color: 'grey', text: '-' };
    const isBuiltin = r.client_id === 'amux-desktop';
    return (
      <Card
        key={r.id}
        className='!rounded-xl h-full'
        bodyStyle={{ padding: '20px', height: '100%' }}
      >
        <div className='flex flex-col gap-4 h-full'>
          {/* 头部：Avatar + 名称 + 标签 */}
          <div className='flex items-start gap-3'>
            <Avatar
              shape='square'
              src={r.logo_url}
              size='large'
              style={{
                borderRadius: 12,
                backgroundColor: r.logo_url ? undefined : 'var(--semi-color-fill-1)',
                color: 'var(--semi-color-text-1)',
                flexShrink: 0,
              }}
            >
              {(r.name || '?').slice(0, 2).toUpperCase()}
            </Avatar>
            <div className='flex-1 min-w-0'>
              <Typography.Text
                strong
                className='block truncate text-base'
                style={{ lineHeight: '1.4' }}
              >
                {r.name}
              </Typography.Text>
              <div className='flex items-center gap-1.5 flex-wrap mt-1'>
                <Tag size='small' color={sp.color}>
                  {sp.text}
                </Tag>
                {r.verified && (
                  <Tag size='small' color='blue' prefixIcon={<ShieldCheck size={12} />}>
                    {t('已认证')}
                  </Tag>
                )}
                {isBuiltin && (
                  <Tag size='small' color='grey'>
                    {t('内置')}
                  </Tag>
                )}
              </div>
            </div>
          </div>

          {/* 元信息块：client_id + 创建时间 + 联系邮箱 */}
          <div
            className='rounded-lg px-3 py-2 flex flex-col gap-1'
            style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
          >
            <div className='flex items-center gap-1 min-w-0'>
              <span
                className='text-xs flex-shrink-0'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                client_id
              </span>
              <Typography.Text
                className='text-xs truncate flex-1'
                style={{ fontFamily: 'monospace' }}
                title={r.client_id}
              >
                {r.client_id}
              </Typography.Text>
              <Button
                type='tertiary'
                theme='borderless'
                size='small'
                icon={<IconCopy size='small' />}
                onClick={() => handleCopyClientId(r.client_id)}
                style={{ padding: '0 4px', height: 'auto', flexShrink: 0 }}
              />
            </div>
            <Typography.Text type='tertiary' className='text-xs block truncate'>
              {t('创建于')} {timestamp2string(r.created_at)}
              {r.contact_email && (
                <>
                  <span className='mx-1'>·</span>
                  {r.contact_email}
                </>
              )}
            </Typography.Text>
          </div>

          {/* 描述（可选） */}
          {r.description && (
            <Typography.Text
              type='tertiary'
              className='text-xs block'
              ellipsis={{ rows: 2, showTooltip: true }}
            >
              {r.description}
            </Typography.Text>
          )}

          {/* 底部操作区：分隔线 + 按钮，贴底 */}
          <div
            className='flex gap-2 flex-wrap mt-auto pt-3'
            style={{ borderTop: '1px solid var(--semi-color-border)' }}
          >
            <Button
              size='small'
              theme='outline'
              icon={<Pencil size={14} />}
              onClick={() => {
                setEditTarget(r);
                setEditOpen(true);
              }}
            >
              {t('编辑')}
            </Button>
            <Button
              size='small'
              theme='outline'
              icon={<RotateCw size={14} />}
              onClick={() => handleRotate(r.id)}
            >
              {t('轮换 secret')}
            </Button>
            {!isBuiltin &&
              (r.status === 1 ? (
                <Button
                  size='small'
                  type='danger'
                  theme='outline'
                  icon={<Trash2 size={14} />}
                  onClick={() => handleDisable(r)}
                >
                  {t('禁用')}
                </Button>
              ) : (
                <Button size='small' theme='outline' onClick={() => handleEnable(r)}>
                  {t('启用')}
                </Button>
              ))}
          </div>
        </div>
      </Card>
    );
  };

  return (
    <Card className='!rounded-2xl' style={{ marginTop: '10px' }}>
      {/* 卡片头部：标题 + 描述 + 新建按钮 */}
      <div className='flex flex-col sm:flex-row items-start sm:justify-between gap-4 mb-5'>
        <div className='flex items-start w-full sm:w-auto'>
          <div className='w-12 h-12 rounded-full bg-slate-100 flex items-center justify-center mr-4 flex-shrink-0'>
            <KeyRound size={22} className='text-slate-600' />
          </div>
          <div>
            <Typography.Title heading={6} className='mb-1'>
              {t('OAuth 应用管理')}
            </Typography.Title>
            <Typography.Text type='tertiary' className='text-sm'>
              {t('注册第三方应用以接入 OAuth Device Flow，授权页将根据 Logo / 名字渲染')}
            </Typography.Text>
          </div>
        </div>
        <Button
          type='primary'
          theme='solid'
          icon={<Plus size={14} />}
          onClick={() => setCreateOpen(true)}
          className='!bg-slate-600 hover:!bg-slate-700 w-full sm:w-auto'
        >
          {t('新建应用')}
        </Button>
      </div>

      {/* 列表 */}
      {loading ? (
        <div className='py-12 flex justify-center'>
          <Spin />
        </div>
      ) : rows.length === 0 ? (
        <div className='py-12'>
          <Empty
            image={
              <div className='w-16 h-16 rounded-full bg-slate-100 flex items-center justify-center mx-auto'>
                <KeyRound size={28} className='text-slate-400' />
              </div>
            }
            description={
              <Typography.Text type='tertiary' className='text-sm'>
                {t('尚无应用，点击右上角新建第一个 OAuth 应用')}
              </Typography.Text>
            }
          />
        </div>
      ) : (
        <div
          className='grid gap-4'
          style={{
            gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 380px), 1fr))',
          }}
        >
          {rows.map(renderClientRow)}
        </div>
      )}

      {/* 新建 */}
      <Modal
        title={t('新建 OAuth 应用')}
        visible={createOpen}
        onCancel={() => setCreateOpen(false)}
        footer={null}
      >
        <Form onSubmit={handleCreate}>
          <Form.Input
            field='name'
            label={t('应用名称')}
            placeholder={t('如：Notion AI 助手')}
            rules={[{ required: true, message: t('请输入名称') }]}
          />
          <Form.Input field='client_id' label={t('client_id（留空自动生成）')} />
          <Form.Input field='logo_url' label={t('Logo URL')} />
          <Form.Input field='homepage_url' label={t('主页 URL')} />
          <Form.Input field='contact_email' label={t('联系邮箱')} />
          <Form.TextArea
            field='description'
            label={t('描述')}
            placeholder={t('一句话介绍：把 X 接入 Y…')}
            rows={3}
          />
          <Form.Switch field='verified' label={t('标记为已认证')} />
          <div className='flex justify-end gap-2 mt-4'>
            <Button onClick={() => setCreateOpen(false)}>{t('取消')}</Button>
            <Button theme='solid' type='primary' htmlType='submit' loading={submitting}>
              {t('创建')}
            </Button>
          </div>
        </Form>
      </Modal>

      {/* 编辑 */}
      <Modal
        title={t('编辑 OAuth 应用')}
        visible={editOpen}
        onCancel={() => {
          setEditOpen(false);
          setEditTarget(null);
        }}
        footer={null}
      >
        {editTarget && (
          <Form onSubmit={handleEdit} initValues={editTarget}>
            <Form.Input field='name' label={t('应用名称')} />
            <Form.Input field='logo_url' label={t('Logo URL')} />
            <Form.Input field='homepage_url' label={t('主页 URL')} />
            <Form.Input field='contact_email' label={t('联系邮箱')} />
            <Form.TextArea field='description' label={t('描述')} rows={3} />
            <Form.Switch field='verified' label={t('标记为已认证')} />
            <div className='flex justify-end gap-2 mt-4'>
              <Button
                onClick={() => {
                  setEditOpen(false);
                  setEditTarget(null);
                }}
              >
                {t('取消')}
              </Button>
              <Button theme='solid' type='primary' htmlType='submit' loading={submitting}>
                {t('保存')}
              </Button>
            </div>
          </Form>
        )}
      </Modal>

      {/* 一次性展示 secret */}
      <Modal
        title={t('请立即复制并保存 client_secret')}
        visible={!!revealedSecret}
        onCancel={() => setRevealedSecret(null)}
        closeOnEsc={false}
        maskClosable={false}
        footer={
          <Button theme='solid' type='primary' onClick={() => setRevealedSecret(null)}>
            {t('我已保存')}
          </Button>
        }
      >
        <Typography.Text type='warning' className='block mb-2'>
          ⚠ {t('client_secret 仅在本次显示，关闭后无法再次查看。')}
        </Typography.Text>
        {revealedSecret && (
          <>
            <Typography.Text type='tertiary' className='text-xs block mb-1'>
              client_id:{' '}
              <code style={{ fontFamily: 'monospace' }}>{revealedSecret.client_id}</code>
            </Typography.Text>
            <Input
              value={revealedSecret.secret}
              readonly
              size='large'
              suffix={
                <Button
                  type='primary'
                  theme='borderless'
                  icon={<IconCopy />}
                  onClick={handleCopySecret}
                >
                  {t('复制')}
                </Button>
              }
            />
          </>
        )}
      </Modal>
    </Card>
  );
}
