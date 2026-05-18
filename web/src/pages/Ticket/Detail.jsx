/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import React, { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Descriptions,
  Select,
  Space,
  Spin,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import ReactMarkdown from 'react-markdown';
import RemarkGfm from 'remark-gfm';
import RemarkBreaks from 'remark-breaks';
import { API } from '../../helpers/api';
import { showError, showSuccess } from '../../helpers/utils';
import {
  ROLE_COLOR,
  STATUS_COLOR,
  fmtTime,
  tCategory,
  tDynamicStatusLabel,
  tPriorityLabel,
  tRole,
  tStatusLabel,
  tType,
} from './constants';
import {
  TicketAttachmentsUploader,
  TicketAttachmentsView,
} from './TicketAttachments';

const { Title, Text } = Typography;

// label 文案通过 tStatusLabel / tPriorityLabel 在渲染时翻译；这里只列 value 顺序。
const STATUS_VALUES = [0, 1, 2, 3];
const PRIORITY_VALUES = [0, 1, 2, 3];

const SUPPORT_CATEGORIES = [
  'model_invocation',
  'channel_issue',
  'billing',
  'account',
  'abuse',
  'other',
];

const FEEDBACK_CATEGORIES = ['feature', 'ux', 'docs', 'other'];

/**
 * 工单详情：用户和管理员视图共用。
 * - 顶部：标题 + 状态 + 类型 + 调用上下文（如有）
 * - 中部：消息时间线，markdown 渲染前经 ReactMarkdown 默认（无 raw html）清洗
 * - 底部：回复输入 + 状态控制按钮
 */
const TicketDetail = ({ admin = false }) => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);
  const [reply, setReply] = useState('');
  const [replyAttachments, setReplyAttachments] = useState([]);
  const [sending, setSending] = useState(false);

  const apiBase = admin ? `/api/admin/ticket/${id}` : `/api/ticket/${id}`;

  const load = async () => {
    setLoading(true);
    try {
      const res = await API.get(apiBase);
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setData(data);
      // 用户进入详情即"已读"。后端已异步 bump user_seen_at；前端立刻通知顶栏
      // NotificationButton 刷新红点，省得等下次 60s 轮询。
      if (!admin) {
        window.dispatchEvent(new CustomEvent('ticket:seen'));
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line
  }, [id]);

  const send = async () => {
    if (!reply.trim()) {
      showError(t('回复内容不能为空'));
      return;
    }
    setSending(true);
    try {
      const url = admin
        ? `/api/admin/ticket/${id}/reply`
        : `/api/ticket/${id}/reply`;
      const res = await API.post(url, {
        content: reply,
        attachments: replyAttachments,
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(t('已回复'));
      setReply('');
      setReplyAttachments([]);
      load();
    } finally {
      setSending(false);
    }
  };

  const userAction = async (action) => {
    const res = await API.put(`/api/ticket/${id}/${action}`);
    const { success, message } = res.data;
    if (!success) {
      showError(message);
      return;
    }
    load();
  };

  const adminUpdate = async (patch) => {
    const res = await API.put(`/api/admin/ticket/${id}`, patch);
    const { success, message } = res.data;
    if (!success) {
      showError(message);
      return;
    }
    load();
  };

  if (loading || !data) {
    return (
      <div className='mt-[60px] px-4 flex justify-center'>
        <Spin />
      </div>
    );
  }

  const bug = data.metadata?.bug_context;
  const dynStatusLabel = tDynamicStatusLabel(
    t,
    data.status,
    data.last_reply_role,
    admin,
  );
  const categoryOptions =
    data.type === 'feedback' ? FEEDBACK_CATEGORIES : SUPPORT_CATEGORIES;

  return (
    <div className='mt-[60px] px-4 max-w-[960px] mx-auto pb-8'>
      <div className='mb-4'>
        <Button
          onClick={() =>
            navigate(admin ? '/console/ticket/admin' : '/console/ticket')
          }
        >
          {t('返回列表')}
        </Button>
      </div>

      <div className='flex flex-col gap-4'>
        <Card>
          <Title heading={4} className='!mb-3'>
            #{data.id} {data.title}
          </Title>
          <div className='flex flex-wrap items-center gap-2'>
            <Tag color={STATUS_COLOR[data.status] || 'white'}>
              {dynStatusLabel}
            </Tag>
            <Tag>{tType(t, data.type)}</Tag>
            <Tag>{tCategory(t, data.category)}</Tag>
            {admin && data.username && (
              <Tag color='violet'>{data.username}</Tag>
            )}
          </div>

          {admin && (
            <AdminControls
              data={data}
              onUpdate={adminUpdate}
              categoryOptions={categoryOptions}
              t={t}
            />
          )}

          <div className='mt-5'>
            <Descriptions
              data={[
                { key: t('创建时间'), value: fmtTime(data.created_at) },
                { key: t('最近活动'), value: fmtTime(data.last_reply_at) },
                { key: t('回复数'), value: data.reply_count },
              ]}
            />
          </div>

          {data.attachments?.length > 0 && (
            <div className='mt-4'>
              <div className='text-sm text-[var(--semi-color-text-2)] mb-2'>
                {t('附件')}
              </div>
              <TicketAttachmentsView attachments={data.attachments} />
            </div>
          )}

          {bug && <BugContextPanel bug={bug} admin={admin} t={t} />}
        </Card>

        <Card title={t('消息')}>
          {data.messages?.length ? (
            <div className='divide-y divide-[var(--semi-color-border)]'>
              {data.messages.map((m) => (
                <MessageRow key={m.id} msg={m} t={t} />
              ))}
            </div>
          ) : (
            <Text>{t('暂无消息')}</Text>
          )}
        </Card>

        {data.status !== 3 && (
          <Card title={t('回复')}>
            <TextArea
              rows={4}
              value={reply}
              onChange={setReply}
              placeholder={t('支持 Markdown')}
              maxLength={32000}
            />
            <div className='mt-3'>
              <TicketAttachmentsUploader
                value={replyAttachments}
                onChange={setReplyAttachments}
                disabled={sending}
              />
            </div>
            <div className='mt-3 flex flex-wrap gap-2'>
              <Button type='primary' loading={sending} onClick={send}>
                {t('发送')}
              </Button>
              {!admin && data.status !== 2 && (
                <Button onClick={() => userAction('close')}>
                  {t('关闭工单')}
                </Button>
              )}
              {admin && (
                <>
                  <Button onClick={() => adminUpdate({ status: 2 })}>
                    {t('标记已解决')}
                  </Button>
                  <Button onClick={() => adminUpdate({ status: 3 })}>
                    {t('关闭工单')}
                  </Button>
                </>
              )}
            </div>
          </Card>
        )}

        {data.status === 3 && !admin && (
          <Card>
            <div className='flex flex-wrap items-center gap-2'>
              <Text>{t('工单已关闭')}</Text>
              <Button onClick={() => userAction('reopen')}>
                {t('重新打开')}
              </Button>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
};

/**
 * 管理员侧的状态 / 优先级 / 分类调整。后端 AdminUpdateTicket 三字段都支持，
 * 之前只暴露了状态按钮，这里补齐 priority 和 category 下拉。
 */
function AdminControls({ data, onUpdate, categoryOptions, t }) {
  return (
    <div className='mt-4 flex flex-wrap items-center gap-3 p-3 rounded-md bg-[var(--semi-color-fill-0)]'>
      <Space spacing='tight'>
        <Text type='tertiary' size='small'>
          {t('状态')}
        </Text>
        <Select
          size='small'
          value={data.status}
          style={{ width: 120 }}
          onChange={(v) => onUpdate({ status: v })}
          optionList={STATUS_VALUES.map((v) => ({
            label: tStatusLabel(t, v),
            value: v,
          }))}
        />
      </Space>
      <Space spacing='tight'>
        <Text type='tertiary' size='small'>
          {t('优先级')}
        </Text>
        <Select
          size='small'
          value={data.priority ?? 1}
          style={{ width: 100 }}
          onChange={(v) => onUpdate({ priority: v })}
          optionList={PRIORITY_VALUES.map((v) => ({
            label: tPriorityLabel(t, v),
            value: v,
          }))}
        />
      </Space>
      <Space spacing='tight'>
        <Text type='tertiary' size='small'>
          {t('分类')}
        </Text>
        <Select
          size='small'
          value={data.category}
          style={{ width: 160 }}
          onChange={(v) => onUpdate({ category: v })}
          optionList={categoryOptions.map((c) => ({
            label: tCategory(t, c),
            value: c,
          }))}
        />
      </Space>
    </div>
  );
}

function BugContextPanel({ bug, admin, t }) {
  const rows = [];
  if (bug.request_id) {
    rows.push({
      key: 'Request ID',
      value: (
        <div className='flex flex-wrap items-center gap-2'>
          <Text copyable>{bug.request_id}</Text>
          {!bug.request_id_verified && (
            <Tag color='orange' size='small'>
              {t('未验证')}
            </Tag>
          )}
        </div>
      ),
    });
  }
  if (bug.channel_id) {
    rows.push({
      key: t('渠道'),
      value: `${bug.channel_name || ''} (#${bug.channel_id})`.trim(),
    });
  }
  if (bug.group) rows.push({ key: t('分组'), value: bug.group });
  if (bug.model) rows.push({ key: t('模型'), value: bug.model });
  if (bug.occurred_at)
    rows.push({ key: t('发生时间'), value: fmtTime(bug.occurred_at) });
  if (bug.http_status) rows.push({ key: 'HTTP', value: bug.http_status });
  if (bug.error_excerpt) {
    rows.push({
      key: t('报错信息'),
      value: (
        <pre className='m-0 whitespace-pre-wrap text-xs'>{bug.error_excerpt}</pre>
      ),
    });
  }

  const openLog = () => {
    const params = new URLSearchParams();
    if (bug.log_id) params.set('log_id', bug.log_id);
    if (bug.request_id) params.set('request_id', bug.request_id);
    window.open(`/console/log?${params.toString()}`, '_blank');
  };

  return (
    <div className='mt-6 pt-5 border-t border-[var(--semi-color-border)]'>
      <Title heading={6} className='!mb-3'>
        {t('调用上下文')}
      </Title>
      <Descriptions data={rows} />
      {admin && (bug.log_id > 0 || bug.request_id) && (
        <div className='mt-3 flex flex-wrap gap-2'>
          <Button onClick={openLog}>{t('查看日志')}</Button>
          {bug.channel_id > 0 && (
            <Button onClick={() => window.open('/console/channel', '_blank')}>
              {t('打开渠道页')}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

function MessageRow({ msg, t }) {
  return (
    <div className='py-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Tag color={ROLE_COLOR[msg.sender_role] || 'white'}>
          {tRole(t, msg.sender_role)}
        </Tag>
        <Text type='tertiary'>{fmtTime(msg.created_at)}</Text>
      </div>
      <div className='mt-2 ticket-md text-sm break-words'>
        <ReactMarkdown
          remarkPlugins={[RemarkGfm, RemarkBreaks]}
          components={{
            a: ({ node, ...props }) => (
              <a {...props} target='_blank' rel='noopener noreferrer' />
            ),
          }}
        >
          {msg.content || ''}
        </ReactMarkdown>
      </div>
      <TicketAttachmentsView attachments={msg.attachments} />
    </div>
  );
}

export default TicketDetail;
