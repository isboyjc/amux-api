/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Empty,
  Form,
  Modal,
  Skeleton,
  Space,
  Tag,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { IconSearch } from '@douyinfe/semi-icons';
import { API } from '../../../helpers/api';
import { showError, createCardProPagination } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { useMinimumLoadingTime } from '../../../hooks/common/useMinimumLoadingTime';
import CardPro from '../../../components/common/ui/CardPro';
import CardTable from '../../../components/common/ui/CardTable';
import CompactModeToggle from '../../../components/common/ui/CompactModeToggle';
import {
  PRIORITY_COLOR,
  STATUS_COLOR,
  fmtTime,
  tCategory,
  tDynamicStatusLabel,
  tPriorityLabel,
  tStatusLabel,
  tType,
} from '../constants';

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

/**
 * 管理员侧工单队列。type2（查询型）布局：顶部 stats 卡片，下方 filters 表单，
 * 表格 + CardPro 自带分页。对齐"使用日志"页风格。
 */
const TicketAdmin = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const filterFormApi = useRef(null);

  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filter, setFilter] = useState({
    status: '',
    type: '',
    priority: '',
    keyword: '',
    user_id: '',
  });
  const [stats, setStats] = useState(null);
  const [loadingStats, setLoadingStats] = useState(false);
  const [compactMode, setCompactMode] = useState(false);
  const [enabled, setEnabled] = useState(true);

  const load = async (overrides = {}) => {
    setLoading(true);
    try {
      const eff = { page, pageSize, filter, ...overrides };
      const params = { page: eff.page, page_size: eff.pageSize };
      Object.entries(eff.filter).forEach(([k, v]) => {
        if (v !== '' && v !== undefined && v !== null) params[k] = v;
      });
      const res = await API.get('/api/admin/ticket', { params });
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

  const loadStats = async () => {
    setLoadingStats(true);
    try {
      const res = await API.get('/api/admin/ticket/stats');
      const { success, data } = res.data;
      if (success) setStats(data);
    } finally {
      setLoadingStats(false);
    }
  };

  const loadEnabled = async () => {
    const res = await API.get('/api/ticket/setting');
    if (res.data?.success) setEnabled(!!res.data.data?.enabled);
  };

  useEffect(() => {
    loadEnabled();
    loadStats();
    load();
    // eslint-disable-next-line
  }, []);

  const onSearch = (values) => {
    const next = {
      status:
        values.status === undefined || values.status === ''
          ? ''
          : Number(values.status),
      type: values.type || '',
      priority:
        values.priority === undefined || values.priority === ''
          ? ''
          : Number(values.priority),
      keyword: values.keyword || '',
      user_id: values.user_id || '',
    };
    setPage(1);
    setFilter(next);
    load({ page: 1, filter: next });
  };

  const onReset = () => {
    filterFormApi.current?.reset();
    const next = {
      status: '',
      type: '',
      priority: '',
      keyword: '',
      user_id: '',
    };
    setPage(1);
    setFilter(next);
    load({ page: 1, filter: next });
  };

  const quickStatus = (id, status) => {
    const titleMap = { 2: '标记已解决', 3: '关闭工单', 0: '重新打开' };
    Modal.confirm({
      title: t(titleMap[status]) + ` #${id}`,
      content: t('确认要更新该工单状态？'),
      onOk: async () => {
        const res = await API.put(`/api/admin/ticket/${id}`, { status });
        if (res.data?.success) {
          load();
          loadStats();
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
        title: t('用户'),
        dataIndex: 'username',
        width: 140,
        render: (v, r) => v || `#${r.user_id}`,
      },
      {
        title: t('标题'),
        dataIndex: 'title',
        render: (v, r) => (
          <a
            onClick={() => navigate(`/console/ticket/admin/${r.id}`)}
            className='cursor-pointer text-[var(--semi-color-link)] hover:underline'
          >
            {v}
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
        width: 160,
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
            {tDynamicStatusLabel(t, s, r.last_reply_role, true)}
          </Tag>
        ),
      },
      { title: t('回复'), dataIndex: 'reply_count', width: 80 },
      {
        title: t('最近活动'),
        dataIndex: 'last_reply_at',
        width: 180,
        render: (v) => fmtTime(v),
      },
      {
        // 操作列：所有动作平铺，不再藏进 IconMore。最多同时显示 3 个按钮
        // （查看 + 解决 + 关闭），列宽按这种情况估算。
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        width: 260,
        render: (_, r) => (
          <Space spacing={4}>
            <Button
              type='tertiary'
              size='small'
              onClick={() => navigate(`/console/ticket/admin/${r.id}`)}
            >
              {t('查看')}
            </Button>
            {(r.status === 0 || r.status === 1) && (
              <Button
                type='secondary'
                size='small'
                onClick={() => quickStatus(r.id, 2)}
              >
                {t('解决')}
              </Button>
            )}
            {r.status !== 3 && (
              <Button
                type='warning'
                size='small'
                onClick={() => quickStatus(r.id, 3)}
              >
                {t('关闭')}
              </Button>
            )}
            {(r.status === 3 || r.status === 2) && (
              <Button
                type='secondary'
                size='small'
                onClick={() => quickStatus(r.id, 0)}
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

  const statsArea = (
    <StatsRow
      stats={stats}
      loading={loadingStats}
      compactMode={compactMode}
      setCompactMode={setCompactMode}
      t={t}
    />
  );

  const filtersArea = (
    <Form
      initValues={{
        type: '',
        status: '',
        priority: '',
        user_id: '',
        keyword: '',
      }}
      getFormApi={(api) => (filterFormApi.current = api)}
      onSubmit={onSearch}
      allowEmpty
      autoComplete='off'
      layout='horizontal'
      trigger='change'
    >
      {/*
        md+ 屏整体靠右：md:justify-end 把所有控件推到行末；窄屏 flex-col 自动
        撑满竖排。每个 Semi 控件被外层 div 限宽（Semi 内部固定 width:100%，
        所以靠包裹层定宽更稳）。
      */}
      <div className='flex flex-col md:flex-row md:items-center md:justify-end gap-2 w-full'>
        {/*
          每个控件外面套 w-XX div 控制行内占位宽度；Semi 控件默认贴内容宽度，
          这里显式 style={{ width: '100%' }} 让它撑满外层，避免看起来比包裹层窄。
        */}
        <div className='w-full md:w-36'>
          <Form.Select
            field='type'
            placeholder={t('类型')}
            showClear
            pure
            size='small'
            style={{ width: '100%' }}
            optionList={[
              { label: tType(t, 'support'), value: 'support' },
              { label: tType(t, 'feedback'), value: 'feedback' },
            ]}
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
        <div className='w-full md:w-32'>
          <Form.Input
            field='user_id'
            placeholder={t('用户 ID')}
            showClear
            pure
            size='small'
            style={{ width: '100%' }}
          />
        </div>
        <div className='w-full md:w-64'>
          <Form.Input
            field='keyword'
            prefix={<IconSearch />}
            placeholder={t('搜索标题')}
            showClear
            pure
            size='small'
            style={{ width: '100%' }}
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
  );

  return (
    <div className='mt-[60px] px-2'>
      {!enabled && (
        <div className='mb-4'>
          <Empty
            image={
              <IllustrationNoResult style={{ width: 120, height: 120 }} />
            }
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 120, height: 120 }} />
            }
            title={t('工单系统未启用')}
            description={t('请前往系统设置 - 工单 中打开开关')}
            style={{ padding: 40 }}
          />
        </div>
      )}

      <CardPro
        type='type2'
        statsArea={statsArea}
        searchArea={filtersArea}
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
    </div>
  );
};

/**
 * 顶部统计 4 个 Tag + 紧凑模式切换。对齐 LogsActions 视觉。
 * loading 时显示骨架避免数字跳动。
 */
function StatsRow({ stats, loading, compactMode, setCompactMode, t }) {
  const showSkeleton = useMinimumLoadingTime(loading);
  const placeholder = (
    <Space>
      <Skeleton.Title style={{ width: 88, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 88, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 88, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 88, height: 21, borderRadius: 6 }} />
    </Space>
  );
  return (
    <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
      <Skeleton loading={showSkeleton || !stats} active placeholder={placeholder}>
        <Space wrap>
          <Tag
            color='orange'
            className='!rounded-lg'
            style={{
              fontWeight: 500,
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
              padding: 13,
            }}
          >
            {t('待处理')}: {stats?.pending ?? 0}
          </Tag>
          <Tag
            color='blue'
            className='!rounded-lg'
            style={{
              fontWeight: 500,
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
              padding: 13,
            }}
          >
            {t('进行中')}: {stats?.open ?? 0}
          </Tag>
          <Tag
            color='green'
            className='!rounded-lg'
            style={{
              fontWeight: 500,
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
              padding: 13,
            }}
          >
            {t('已解决')}: {stats?.resolved ?? 0}
          </Tag>
          <Tag
            color='white'
            className='!rounded-lg'
            style={{
              border: 'none',
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
              fontWeight: 500,
              padding: 13,
            }}
          >
            {t('已关闭')}: {stats?.closed ?? 0}
          </Tag>
        </Space>
      </Skeleton>

      <CompactModeToggle
        compactMode={compactMode}
        setCompactMode={setCompactMode}
        t={t}
      />
    </div>
  );
}

export default TicketAdmin;
