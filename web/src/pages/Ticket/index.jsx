/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Empty,
  Form,
  Modal,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Ticket as TicketIcon } from 'lucide-react';
import { API } from '../../helpers/api';
import { showError, showSuccess } from '../../helpers/utils';
import { createCardProPagination } from '../../helpers/utils';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import CardPro from '../../components/common/ui/CardPro';
import CardTable from '../../components/common/ui/CardTable';
import CompactModeToggle from '../../components/common/ui/CompactModeToggle';
import {
  PRIORITY_COLOR,
  REFUND_METHODS,
  REFUND_REASONS,
  STATUS_COLOR,
  buildCategoryOptions,
  fmtTime,
  tCategory,
  tDynamicStatusLabel,
  tPriorityLabel,
  tRefundMethod,
  tRefundReason,
  tStatusLabel,
  tType,
} from './constants';
import { StatusContext } from '../../context/Status';
import { TicketAttachmentsUploader } from './TicketAttachments';

const { Text } = Typography;

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

/**
 * 用户侧工单列表。沿用项目里"兑换码管理"/"使用日志"的 CardPro + CardTable
 * 模式：header 描述 + 操作/筛选行 + 表格 + 底部 createCardProPagination 分页。
 */
const Ticket = () => {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const isMobile = useIsMobile();

  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filter, setFilter] = useState({
    status: '',
    type: '',
    category: '',
    priority: '',
  });
  // 筛选区当前选中的 type，用于联动 category 下拉。
  // 只在筛选行内部使用，不进入实际请求；请求值仍走 filter.type。
  const [filterType, setFilterType] = useState('');
  const [setting, setSetting] = useState(null);
  const [compactMode, setCompactMode] = useState(false);
  const [newModalOpen, setNewModalOpen] = useState(false);
  const filterFormApi = useRef(null);

  const load = async (overrides = {}) => {
    setLoading(true);
    try {
      const eff = { page, pageSize, filter, ...overrides };
      const params = { page: eff.page, page_size: eff.pageSize };
      if (eff.filter.status !== '') params.status = eff.filter.status;
      if (eff.filter.type) params.type = eff.filter.type;
      if (eff.filter.category) params.category = eff.filter.category;
      if (eff.filter.priority !== '') params.priority = eff.filter.priority;
      const res = await API.get('/api/ticket', { params });
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setItems(data.items || []);
      setTotal(data.total || 0);
    } finally {
      setLoading(false);
    }
  };

  const loadSetting = async () => {
    const res = await API.get('/api/ticket/setting');
    const { success, data, message } = res.data;
    if (!success) {
      showError(message);
      return;
    }
    setSetting(data);
  };

  useEffect(() => {
    loadSetting();
  }, []);

  useEffect(() => {
    if (setting?.enabled) load();
    // eslint-disable-next-line
  }, [setting?.enabled]);

  const onSearch = (values) => {
    const next = {
      status: values.status === undefined || values.status === '' ? '' : Number(values.status),
      type: values.type || '',
      category: values.category || '',
      priority:
        values.priority === undefined || values.priority === ''
          ? ''
          : Number(values.priority),
    };
    setPage(1);
    setFilter(next);
    load({ page: 1, filter: next });
  };

  const onReset = () => {
    filterFormApi.current?.reset();
    const next = { status: '', type: '', category: '', priority: '' };
    setPage(1);
    setFilter(next);
    setFilterType('');
    load({ page: 1, filter: next });
  };

  const quickAction = (id, action) => {
    const titleMap = { close: '关闭工单', reopen: '重新打开' };
    Modal.confirm({
      title: t(titleMap[action]) + ` #${id}`,
      content: t('确认要更新该工单状态？'),
      onOk: async () => {
        const res = await API.put(`/api/ticket/${id}/${action}`);
        if (res.data?.success) {
          load();
        } else {
          showError(res.data?.message);
        }
      },
    });
  };

  const columns = useMemo(
    () => [
      { title: '#', dataIndex: 'id', width: 80 },
      {
        title: t('标题'),
        dataIndex: 'title',
        render: (text, record) => (
          <a
            onClick={() => navigate(`/console/ticket/${record.id}`)}
            className='cursor-pointer text-[var(--semi-color-link)] hover:underline'
          >
            {text}
          </a>
        ),
      },
      {
        title: t('类型'),
        dataIndex: 'type',
        width: 110,
        render: (v) => (
          <Tag color='white' shape='circle'>
            {tType(t, v)}
          </Tag>
        ),
      },
      {
        title: t('优先级'),
        dataIndex: 'priority',
        width: 100,
        render: (p) => (
          <Tag color={PRIORITY_COLOR[p] || 'white'} shape='circle'>
            {tPriorityLabel(t, p)}
          </Tag>
        ),
      },
      {
        title: t('分类'),
        dataIndex: 'category',
        width: 150,
        render: (v) => (
          <Tag color='grey' shape='circle'>
            {tCategory(t, v)}
          </Tag>
        ),
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 140,
        render: (s, r) => (
          <Tag color={STATUS_COLOR[s] || 'white'} shape='circle'>
            {tDynamicStatusLabel(t, s, r.last_reply_role, false)}
          </Tag>
        ),
      },
      {
        title: t('回复'),
        dataIndex: 'reply_count',
        width: 80,
      },
      {
        title: t('最近活动'),
        dataIndex: 'last_reply_at',
        width: 180,
        render: (v) => fmtTime(v),
      },
      {
        // 操作列：所有按钮直接平铺，不再藏进 IconMore 下拉。宽度按最多 2 个
        // 按钮估算（查看 + 关闭/重新打开），加少量缓冲。
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        width: 180,
        render: (_, r) => (
          <Space spacing={4}>
            <Button
              type='tertiary'
              size='small'
              onClick={() => navigate(`/console/ticket/${r.id}`)}
            >
              {t('查看')}
            </Button>
            {r.status !== 3 && r.status !== 2 && (
              <Button
                type='warning'
                size='small'
                onClick={() => quickAction(r.id, 'close')}
              >
                {t('关闭')}
              </Button>
            )}
            {(r.status === 3 || r.status === 2) && (
              <Button
                type='secondary'
                size='small'
                onClick={() => quickAction(r.id, 'reopen')}
              >
                {t('重开')}
              </Button>
            )}
          </Space>
        ),
      },
    ],
    // eslint-disable-next-line
    [navigate, t],
  );

  // compact 模式下移除 fixed，避免移动端窄屏不必要的横向滚动
  const tableColumns = useMemo(
    () =>
      compactMode
        ? columns.map((c) => {
            if (c.dataIndex === 'operate') {
              const { fixed, ...rest } = c;
              return rest;
            }
            return c;
          })
        : columns,
    [compactMode, columns],
  );

  if (setting && !setting.enabled) {
    return (
      <div className='mt-[60px] px-2'>
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          title={t('工单系统未启用')}
          style={{ padding: 60 }}
        />
      </div>
    );
  }

  return (
    <div className='mt-[60px] px-2'>
      <CardPro
        type='type1'
        descriptionArea={
          <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
            <div className='flex items-center text-orange-500'>
              <TicketIcon size={16} className='mr-2' />
              <Text>{t('我的工单')}</Text>
            </div>
            <CompactModeToggle
              compactMode={compactMode}
              setCompactMode={setCompactMode}
              t={t}
            />
          </div>
        }
        actionsArea={
          /*
            左侧主操作（新建工单）+ 右侧筛选区。justify-between 把两块推到两端；
            右侧 form 内部的子项用 gap-2 紧贴排布，整组靠 ml-auto/justify-end
            贴到右边缘——视觉上类型/状态/按钮之间不会再有意外空隙。
          */
          <div className='flex flex-col md:flex-row md:items-center justify-between gap-2 w-full'>
            <div className='order-2 md:order-1'>
              <Button
                type='primary'
                size='small'
                onClick={() => setNewModalOpen(true)}
              >
                {t('新建工单')}
              </Button>
            </div>
            <Form
              initValues={{
                status: '',
                type: '',
                category: '',
                priority: '',
              }}
              getFormApi={(api) => (filterFormApi.current = api)}
              onSubmit={onSearch}
              allowEmpty
              autoComplete='off'
              layout='horizontal'
              trigger='change'
              className='order-1 md:order-2 w-full md:w-auto'
            >
              <div className='flex flex-col md:flex-row md:items-center md:justify-end gap-2 w-full'>
                {/*
                  Semi 控件默认按内容宽度，套了固定宽度的外层 div 也不会撑满，
                  这里显式 style={{ width: '100%' }} 让 Select 填充 w-36 外层。
                */}
                <div className='w-full md:w-36'>
                  <Form.Select
                    field='type'
                    placeholder={t('类型')}
                    showClear
                    pure
                    size='small'
                    style={{ width: '100%' }}
                    onChange={(v) => {
                      // 切换 type 时清空 category，避免出现「support 工单 +
                      // feature 分类」这种不存在的组合。
                      setFilterType(v || '');
                      filterFormApi.current?.setValue('category', '');
                    }}
                    optionList={[
                      { label: tType(t, 'support'), value: 'support' },
                      { label: tType(t, 'feedback'), value: 'feedback' },
                    ]}
                  />
                </div>
                <div className='w-full md:w-40'>
                  <Form.Select
                    // 同一 type 内变更分类时无需重挂载；
                    // type 切换会因为 optionList 引用变化触发选项刷新。
                    field='category'
                    placeholder={t('分类')}
                    showClear
                    pure
                    size='small'
                    style={{ width: '100%' }}
                    optionList={buildCategoryOptions(
                      t,
                      filterType,
                      setting?.categories,
                    )}
                  />
                </div>
                <div className='w-full md:w-36'>
                  <Form.Select
                    field='priority'
                    placeholder={t('优先级')}
                    showClear
                    pure
                    size='small'
                    style={{ width: '100%' }}
                    optionList={[0, 1, 2, 3].map((p) => ({
                      label: tPriorityLabel(t, p),
                      value: String(p),
                    }))}
                  />
                </div>
                <div className='w-full md:w-36'>
                  <Form.Select
                    field='status'
                    placeholder={t('状态')}
                    showClear
                    pure
                    size='small'
                    style={{ width: '100%' }}
                    optionList={[0, 1, 2, 3].map((s) => ({
                      label: tStatusLabel(t, s),
                      value: String(s),
                    }))}
                  />
                </div>
                <div className='flex gap-2 w-full md:w-auto'>
                  <Button
                    type='tertiary'
                    htmlType='submit'
                    loading={loading}
                    className='flex-1 md:flex-initial'
                    size='small'
                  >
                    {t('查询')}
                  </Button>
                  <Button
                    type='tertiary'
                    onClick={onReset}
                    className='flex-1 md:flex-initial'
                    size='small'
                  >
                    {t('重置')}
                  </Button>
                </div>
              </div>
            </Form>
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: page,
          pageSize,
          total,
          onPageChange: (p) => {
            setPage(p);
            load({ page: p });
          },
          onPageSizeChange: (s) => {
            setPageSize(s);
            setPage(1);
            load({ page: 1, pageSize: s });
          },
          isMobile,
          pageSizeOpts: PAGE_SIZE_OPTIONS,
          t,
        })}
        t={t}
      >
        <CardTable
          rowKey='id'
          loading={loading}
          columns={tableColumns}
          dataSource={items}
          scroll={compactMode ? undefined : { x: 'max-content' }}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('搜索无结果')}
              style={{ padding: 30 }}
            />
          }
          pagination={{
            currentPage: page,
            pageSize,
            total,
            pageSizeOptions: PAGE_SIZE_OPTIONS,
            showSizeChanger: true,
          }}
          hidePagination
          className='rounded-xl overflow-hidden'
          size='middle'
        />
      </CardPro>

      {newModalOpen && setting && (
        <NewTicketModal
          setting={setting}
          onClose={() => setNewModalOpen(false)}
          onCreated={(id) => {
            setNewModalOpen(false);
            navigate(`/console/ticket/${id}`);
          }}
        />
      )}
    </div>
  );
};

/**
 * 新建工单弹窗。当 category 选中 model_invocation / channel_issue 时
 * 展开"调用上下文"分组。提交走 Semi formApi，能拿到原生校验。
 */
function NewTicketModal({ setting, onClose, onCreated }) {
  const { t } = useTranslation();
  const formApi = useRef(null);
  const [type, setType] = useState('support');
  const [category, setCategory] = useState('model_invocation');
  const [priority, setPriority] = useState(1); // 默认"普通"，与后端 default 一致
  const [submitting, setSubmitting] = useState(false);
  const [logOptions, setLogOptions] = useState([]);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [rawLogCount, setRawLogCount] = useState(null);
  const [attachments, setAttachments] = useState([]);

  // 退款工单相关状态。
  //   refundMethod: platform / offline，影响订单下拉是否显示。
  //   refundReason: 控制 "其他原因" 文本框的显隐。
  //   topupOptions: 异步拉到的可退款订单（status=success），仅在用户首次切到
  //     refund 分类时拉一次，避免每次切换都打接口。
  const [statusState] = useContext(StatusContext);
  const currencySymbol =
    statusState?.status?.stripe_currency_symbol || '$';
  const [refundMethod, setRefundMethod] = useState('platform');
  const [refundReason, setRefundReason] = useState('wrong_amount');
  const [topupOptions, setTopupOptions] = useState([]);
  const [loadingTopups, setLoadingTopups] = useState(false);
  const [topupsLoadedOnce, setTopupsLoadedOnce] = useState(false);

  const categoryOptions = useMemo(() => {
    if (!setting?.categories) return [];
    return (setting.categories[type] || []).map((c) => ({
      label: tCategory(t, c),
      value: c,
    }));
  }, [type, setting, t]);

  // 退款工单走独立分支：不再显示 Request ID 字段，避免和 bug_context 混用。
  const showRefund = category === 'refund';
  // 哪些分类需要带 Request ID 字段。计费/额度也常常关联具体请求，所以也加上。
  const showBugCtx =
    !showRefund &&
    (category === 'model_invocation' ||
      category === 'channel_issue' ||
      category === 'billing');

  // 拉用户最近 100 条日志做 Request ID 下拉。沿用 UsageLogs hook 的请求格式
  // （p / page_size / 后端只认 p 而不是 page），保持一致。过滤掉没有 request_id
  // 的条目；同时记录原始条数，让用户能在下拉空态里看出是"完全没日志"还是
  // "有日志但都没 request_id（非 relay 调用）"。
  const fetchRecentLogs = async () => {
    setLoadingLogs(true);
    try {
      const res = await API.get('/api/log/self/', {
        params: { p: 1, page_size: 100, type: 0 },
      });
      if (!res.data?.success) {
        // eslint-disable-next-line no-console
        console.warn('[ticket] log fetch failed:', res.data);
        setLogOptions([]);
        setRawLogCount(0);
        return;
      }
      const payload = res.data.data;
      const raw = Array.isArray(payload?.items)
        ? payload.items
        : Array.isArray(payload)
          ? payload
          : [];

      // 按 request_id 去重：同一请求可能产生多条日志（consume + error），
      // 错误条优先保留，让用户在下拉里看到的状态准确。
      const byId = new Map();
      for (const l of raw) {
        if (!l?.request_id) continue;
        const cur = byId.get(l.request_id);
        if (!cur) {
          byId.set(l.request_id, l);
          continue;
        }
        // 已存在：若新条目是 error 且当前不是，则覆盖
        if (l.type === 5 && cur.type !== 5) {
          byId.set(l.request_id, l);
        }
      }

      const list = [...byId.values()]
        .sort((a, b) => (b.created_at || 0) - (a.created_at || 0))
        .map(buildLogOption);

      setLogOptions(list);
      setRawLogCount(raw.length);
    } finally {
      setLoadingLogs(false);
    }
  };

  // 打开新建工单弹窗时拉一次最近日志，仅一次。后续不再随分类切换或点击
  // 下拉重复请求。
  useEffect(() => {
    fetchRecentLogs();
    // eslint-disable-next-line
  }, []);

  // 拉最近 100 笔成功充值订单做退款下拉。后端 30 天窗口已闸住返回量。
  // 懒加载：第一次切到 refund 分类才请求；后续切换不重复打。
  const fetchRecentTopups = async () => {
    setLoadingTopups(true);
    try {
      const res = await API.get('/api/user/topup/self', {
        params: { p: 1, page_size: 100, status: 'success' },
      });
      if (!res.data?.success) {
        // eslint-disable-next-line no-console
        console.warn('[ticket] topup fetch failed:', res.data);
        setTopupOptions([]);
        return;
      }
      const payload = res.data.data;
      const raw = Array.isArray(payload?.items)
        ? payload.items
        : Array.isArray(payload)
          ? payload
          : [];
      setTopupOptions(
        raw.map((tp) => buildTopupOption(tp, currencySymbol, t)),
      );
    } finally {
      setLoadingTopups(false);
    }
  };

  useEffect(() => {
    if (showRefund && !topupsLoadedOnce) {
      setTopupsLoadedOnce(true);
      fetchRecentTopups();
    }
    // eslint-disable-next-line
  }, [showRefund]);

  const submit = async () => {
    if (!formApi.current) return;
    try {
      const values = await formApi.current.validate();
      setSubmitting(true);
      const payload = {
        type,
        category,
        title: (values.title || '').trim(),
        content: values.content || '',
        priority,
      };
      if (showBugCtx) {
        const requestId = (values.request_id || '').trim();
        if (requestId) {
          payload.bug_context = { request_id: requestId };
        }
      }
      if (showRefund) {
        // 表单校验已保证：reason 必选、其他原因必填、platform 必选至少一笔。
        const refundCtx = {
          method: refundMethod,
          reason: refundReason,
        };
        if (refundReason === 'other') {
          refundCtx.reason_other = (values._refund_reason_other || '').trim();
        }
        if (refundMethod === 'platform') {
          const tradeNos = values._refund_topups || [];
          refundCtx.topups = tradeNos.map((tn) => ({ trade_no: tn }));
        }
        payload.refund_context = refundCtx;
      }
      if (attachments.length > 0) {
        payload.attachments = attachments;
      }
      const res = await API.post('/api/ticket', payload);
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(t('工单已创建'));
      onCreated(data.id);
    } catch (e) {
      // Semi 自带高亮
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      visible
      onCancel={onClose}
      onOk={submit}
      okText={t('提交')}
      cancelText={t('取消')}
      confirmLoading={submitting}
      title={t('新建工单')}
      width={680}
    >
      {/*
        Semi Form 接管 field 的状态，外部传 value 只是受控代理；必须用
        initValues 给字段初始值才会回填。type 改变时需要同步把 category
        select 的值也通过 formApi 改掉，否则用户看到的还是旧选项。
      */}
      <Form
        initValues={{
          _type_select: type,
          _category_select: category,
          _priority_select: priority,
        }}
        getFormApi={(api) => (formApi.current = api)}
        labelPosition='top'
      >
        <div className='flex gap-3 mb-2'>
          <div className='flex-1'>
            <Form.Select
              field='_type_select'
              label={t('类型')}
              onChange={(v) => {
                setType(v);
                const list = setting.categories?.[v] || [];
                const nextCategory = list[0] || 'other';
                setCategory(nextCategory);
                // Semi Form 管状态，category 也要同步刷一下回填值。
                formApi.current?.setValue('_category_select', nextCategory);
              }}
              style={{ width: '100%' }}
              optionList={[
                { label: tType(t, 'support'), value: 'support' },
                { label: tType(t, 'feedback'), value: 'feedback' },
              ]}
            />
          </div>
          <div className='flex-1'>
            <Form.Select
              field='_category_select'
              label={t('分类')}
              onChange={setCategory}
              style={{ width: '100%' }}
              optionList={categoryOptions}
            />
          </div>
          <div className='flex-1'>
            {/*
              用户建议优先级。最终是否生效由管理员判断（后端校验 [0,3]），
              这里只把用户的预期透给后台，避免每次都默认"普通"导致紧急事件
              被埋。
            */}
            <Form.Select
              field='_priority_select'
              label={t('优先级')}
              onChange={setPriority}
              style={{ width: '100%' }}
              optionList={[0, 1, 2, 3].map((p) => ({
                label: tPriorityLabel(t, p),
                value: p,
              }))}
            />
          </div>
        </div>

        <Form.Input
          field='title'
          label={t('标题')}
          placeholder={t('一句话描述问题')}
          maxLength={setting.max_title_length || 200}
          rules={[{ required: true, message: t('请填写标题') }]}
        />
        <Form.TextArea
          field='content'
          label={t('描述')}
          placeholder={t('详细描述、复现步骤、预期与实际表现等')}
          rows={6}
          maxLength={setting.max_content_length || 32000}
          rules={[{ required: true, message: t('请填写内容') }]}
        />

        {/* 附件紧跟描述，让"输入文字 / 补充截图"的动作在视觉上连成一组 */}
        <div className='mt-3'>
          <div className='mb-1 text-sm text-[var(--semi-color-text-2)]'>
            {t('附件')}
          </div>
          <TicketAttachmentsUploader
            value={attachments}
            onChange={setAttachments}
            disabled={submitting}
            max={setting?.max_attachments_per_message || 6}
          />
        </div>

        {showBugCtx && (
          <>
            {/*
              label 是 ReactNode（双行展示状态/分组/模型/时间），Semi 默认
              filter 按 label.toString() 过滤会失效，所以自定义 filter 走
              searchText（id + group + model + ok/error 拼接的纯文本）。
              key 在 logOptions 长度从 0 变非 0 时切换，触发 Select 重挂载，
              fetch 完成时下拉立即可见（Semi Select 在 mount 时把 optionList
              快照进内部，仅靠后续 props 刷新偶尔不更新视图）。
            */}
            <div className='mt-2'>
              <Form.Select
                key={`req-${logOptions.length}`}
                field='request_id'
                label='Request ID'
                placeholder={t('选择或输入 Request ID')}
                showClear
                filter={(input, option) =>
                  !input ||
                  (option?.searchText || option?.value || '')
                    .toLowerCase()
                    .includes(input.toLowerCase())
                }
                allowCreate
                loading={loadingLogs}
                optionList={logOptions}
                renderSelectedItem={(option) =>
                  option?.requestId || option?.value || ''
                }
                emptyContent={
                  loadingLogs
                    ? t('加载中…')
                    : rawLogCount > 0
                      ? t('最近 100 条日志均无 Request ID，可手动输入')
                      : t('暂无可选请求记录')
                }
                style={{ width: '100%' }}
              />
            </div>
          </>
        )}

        {showRefund && (
          <div className='mt-3 p-3 rounded-md bg-[var(--semi-color-fill-0)]'>
            <div className='mb-2 text-sm text-[var(--semi-color-text-2)]'>
              {t('退款信息')}
            </div>
            {/*
              退款方式 / 原因走 Semi Form 字段，复用框架的 reset 和校验。
              方式切换时清空已选订单，避免离线模式残留 platform 的订单引用。
            */}
            <Form.RadioGroup
              field='_refund_method'
              label={t('退款方式')}
              initValue={refundMethod}
              onChange={(e) => {
                const v = e?.target?.value ?? e;
                setRefundMethod(v);
                if (v === 'offline') {
                  formApi.current?.setValue('_refund_topups', []);
                }
              }}
              rules={[
                { required: true, message: t('请选择退款方式') },
              ]}
            >
              {REFUND_METHODS.map((m) => (
                <Form.Radio key={m} value={m}>
                  {tRefundMethod(t, m)}
                </Form.Radio>
              ))}
            </Form.RadioGroup>

            {refundMethod === 'platform' && (
              <Form.Select
                key={`topup-${topupOptions.length}`}
                field='_refund_topups'
                label={t('退款订单')}
                placeholder={t('选择需要退款的订单（可多选）')}
                multiple
                showClear
                maxTagCount={3}
                loading={loadingTopups}
                optionList={topupOptions}
                filter={(input, option) =>
                  !input ||
                  (option?.searchText || option?.value || '')
                    .toLowerCase()
                    .includes(input.toLowerCase())
                }
                // 多选模式 Semi 要求返回 { isRenderInTag, content } 对象；
                // 返回裸字符串时输入框里不会显示任何 chip。
                renderSelectedItem={(option) => ({
                  isRenderInTag: true,
                  content: option?.tradeNo || option?.value || '',
                })}
                emptyContent={
                  loadingTopups
                    ? t('加载中…')
                    : t('近 30 天内暂无可退款的成功订单')
                }
                rules={[
                  {
                    required: true,
                    type: 'array',
                    min: 1,
                    message: t('请至少选择一笔订单'),
                  },
                  {
                    type: 'array',
                    max: 10,
                    message: t('一次最多选择 10 笔订单'),
                  },
                ]}
                style={{ width: '100%' }}
              />
            )}

            <Form.Select
              field='_refund_reason'
              label={t('退款原因')}
              initValue={refundReason}
              onChange={setRefundReason}
              rules={[{ required: true, message: t('请选择退款原因') }]}
              optionList={REFUND_REASONS.map((r) => ({
                label: tRefundReason(t, r),
                value: r,
              }))}
              style={{ width: '100%' }}
            />

            {refundReason === 'other' && (
              <Form.TextArea
                field='_refund_reason_other'
                label={t('补充说明')}
                placeholder={t('请描述具体原因')}
                rows={3}
                maxLength={512}
                rules={[
                  { required: true, message: t('请填写补充说明') },
                ]}
              />
            )}
          </div>
        )}
      </Form>
    </Modal>
  );
}

/**
 * Build a Semi Select option from a Log row.
 *
 * label 用 ReactNode 渲染双行：
 *   行 1：状态 Tag（成功 / 失败）+ request_id（等宽字体）
 *   行 2：分组 · 模型 · 发生时间
 * searchText 是可被自定义 filter 函数匹配的纯文本，使下拉支持
 *   按 id / group / model / "success" / "error" 关键字搜索。
 */
/**
 * 把一条充值订单组装成 Semi Select 选项。
 *   value: trade_no（提交时取这个字符串）
 *   label: 双行 —— 订单号 + 金额 / 时间 / 支付方式
 *   searchText: 用于自定义过滤（用户可能按金额或时间搜）
 */
function buildTopupOption(tp, currencySymbol, t) {
  const tradeNo = String(tp.trade_no || '');
  const money = Number(tp.money || 0);
  const completed = tp.complete_time
    ? fmtTime(tp.complete_time)
    : fmtTime(tp.create_time);
  const pay = tp.payment_method || '—';
  return {
    value: tradeNo,
    tradeNo,
    searchText: [tradeNo, pay, String(money), completed]
      .join(' ')
      .toLowerCase(),
    label: (
      <div className='flex flex-col py-1'>
        <div className='flex items-center gap-2'>
          <span className='font-mono text-xs truncate'>{tradeNo}</span>
          <span className='inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'>
            {currencySymbol}
            {money.toFixed(2)}
          </span>
        </div>
        <div className='mt-0.5 text-[11px] text-[var(--semi-color-text-2)]'>
          {t('完成时间')}: {completed} · {pay}
        </div>
      </div>
    ),
  };
}

function buildLogOption(l) {
  const id = String(l.request_id);
  const model = l.model_name || '—';
  const group = l.group || '—';
  const time = fmtTime(l.created_at);
  const isError = l.type === 5;
  return {
    value: id,
    requestId: id,
    searchText: [id, group, model, isError ? 'error' : 'success']
      .join(' ')
      .toLowerCase(),
    label: (
      <div className='flex flex-col py-1'>
        <div className='flex items-center gap-2'>
          <span
            className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium ${
              isError
                ? 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                : 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
            }`}
          >
            {isError ? 'ERROR' : 'OK'}
          </span>
          <span className='font-mono text-xs truncate'>{id}</span>
        </div>
        <div className='mt-0.5 text-[11px] text-[var(--semi-color-text-2)]'>
          {group} · {model} · {time}
        </div>
      </div>
    ),
  };
}

export default Ticket;
