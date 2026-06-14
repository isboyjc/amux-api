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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Descriptions,
  Empty,
  SideSheet,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import {
  API,
  renderNumber,
  renderQuota,
  showError,
  timestamp2string,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import CardTable from '../../../common/ui/CardTable';

const { Text, Title } = Typography;

const AFF_RISK_REASON_LABELS = {
  invites_24h: '24h邀请数异常',
  invites_7d: '7天邀请数偏高',
  disposable_ratio: '一次性邮箱占比高',
  activation_ratio: '被邀者活跃率低',
  banned_ratio: '被邀者封禁率高',
  email_pattern: '邮箱前缀批量特征',
};

const parseRiskReason = (raw) => {
  const key = String(raw || '').split(':')[0];
  return AFF_RISK_REASON_LABELS[key] || raw;
};

const renderRiskTag = (level, t) => {
  if (!level) {
    return (
      <Tag color='grey' shape='circle' size='small'>
        {t('未评估')}
      </Tag>
    );
  }
  const config = {
    normal: { color: 'green', label: '正常' },
    suspect: { color: 'orange', label: '可疑' },
    danger: { color: 'red', label: '高危' },
  };
  const { color, label } = config[level] || config.normal;
  return (
    <Tag color={color} shape='circle' size='small'>
      {t(label)}
    </Tag>
  );
};

const renderRiskDetailTag = (risk, t) => {
  if (!risk || !risk.risk_level) {
    return (
      <Tag color='grey' shape='circle' size='small'>
        {t('未评估')}
      </Tag>
    );
  }
  let reasons = [];
  if (risk.risk_reasons) {
    try {
      const parsed =
        typeof risk.risk_reasons === 'string'
          ? JSON.parse(risk.risk_reasons)
          : risk.risk_reasons;
      if (Array.isArray(parsed)) reasons = parsed;
    } catch {
      // ignore
    }
  }
  const tooltipLines = reasons.map((r) => t(parseRiskReason(r))).join('、');
  const computedAt = risk.computed_at
    ? timestamp2string(risk.computed_at)
    : null;
  const tooltipContent = (
    <div className='text-xs' style={{ maxWidth: 280 }}>
      {tooltipLines && <div>{tooltipLines}</div>}
      {computedAt && (
        <div className='opacity-70 mt-1'>
          {t('评估时间')}: {computedAt}
        </div>
      )}
      {!tooltipLines && !computedAt && <div>{t('无风险信号')}</div>}
    </div>
  );
  return (
    <Tooltip content={tooltipContent} position='top'>
      <span>{renderRiskTag(risk.risk_level, t)}</span>
    </Tooltip>
  );
};

const renderUserStatus = (status, t) => {
  if (status === 1) {
    return (
      <Tag color='green' shape='circle' size='small'>
        {t('已启用')}
      </Tag>
    );
  }
  if (status === 2) {
    return (
      <Tag color='red' shape='circle' size='small'>
        {t('已禁用')}
      </Tag>
    );
  }
  return (
    <Tag color='grey' shape='circle' size='small'>
      {t('未知状态')}
    </Tag>
  );
};

const UserSummaryCard = ({ title, user, emptyTip, t }) => {
  if (!user) {
    return (
      <div className='border border-[var(--semi-color-border)] rounded-md p-3 bg-[var(--semi-color-fill-0)]'>
        <div className='font-medium mb-2'>{title}</div>
        <Text type='tertiary' className='text-xs'>
          {emptyTip}
        </Text>
      </div>
    );
  }
  const data = [
    { key: 'ID', value: user.id },
    { key: t('用户名'), value: user.username || '-' },
    { key: t('昵称'), value: user.display_name || '-' },
    { key: t('邮箱'), value: user.email || '-' },
    { key: t('状态'), value: renderUserStatus(user.status, t) },
    { key: t('邀请码'), value: user.aff_code || '-' },
    { key: t('已邀请人数'), value: renderNumber(user.aff_count || 0) },
    {
      key: t('邀请历史收益'),
      value: renderQuota(user.aff_history_quota || 0),
    },
    { key: t('风险等级'), value: renderRiskDetailTag(user.risk, t) },
    {
      key: t('注册时间'),
      value: user.created_time
        ? timestamp2string(user.created_time)
        : '-',
    },
    {
      key: t('最后登录'),
      value: user.last_login_at
        ? timestamp2string(user.last_login_at)
        : '-',
    },
  ];
  return (
    <div className='border border-[var(--semi-color-border)] rounded-md p-3 bg-[var(--semi-color-bg-1)]'>
      <div className='font-medium mb-2'>{title}</div>
      <Descriptions
        data={data}
        size='small'
        row
        align='left'
        layout='horizontal'
      />
    </div>
  );
};

const AffiliateRelationModal = ({ visible, onCancel, user, t }) => {
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [view, setView] = useState(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const loadData = async (nextPage = page, nextPageSize = pageSize) => {
    if (!user?.id) return;
    setLoading(true);
    try {
      const res = await API.get(
        `/api/user/${user.id}/affiliate?page=${nextPage}&page_size=${nextPageSize}`,
      );
      if (res.data?.success) {
        setView(res.data.data || null);
      } else {
        showError(res.data?.message || t('加载失败'));
        setView(null);
      }
    } catch (e) {
      showError(t('请求失败'));
      setView(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!visible) return;
    setPage(1);
    setPageSize(10);
    loadData(1, 10);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, user?.id]);

  const handlePageChange = (p) => {
    setPage(p);
    loadData(p, pageSize);
  };

  const handlePageSizeChange = (s) => {
    setPageSize(s);
    setPage(1);
    loadData(1, s);
  };

  const inviteeColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      {
        title: t('用户名'),
        dataIndex: 'username',
        render: (text, record) => (
          <div className='min-w-0'>
            <div className='truncate'>{text || '-'}</div>
            {record.display_name && (
              <div className='text-xs text-[var(--semi-color-text-2)] truncate'>
                {record.display_name}
              </div>
            )}
          </div>
        ),
      },
      {
        title: t('邮箱'),
        dataIndex: 'email',
        render: (text) => <span>{text || '-'}</span>,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 90,
        render: (status) => renderUserStatus(status, t),
      },
      {
        title: t('风险等级'),
        dataIndex: 'risk_level',
        width: 100,
        render: (level) => renderRiskTag(level, t),
      },
      {
        title: t('累计充值'),
        dataIndex: 'topup_amount',
        width: 110,
        render: (amount) => (
          <Text>{Number(amount || 0).toFixed(2)}</Text>
        ),
      },
      {
        title: t('注册时间'),
        dataIndex: 'created_time',
        width: 160,
        render: (ts) => (ts ? timestamp2string(ts) : '-'),
      },
      {
        title: t('最后登录'),
        dataIndex: 'last_login_at',
        width: 160,
        render: (ts) => (ts ? timestamp2string(ts) : '-'),
      },
    ],
    [t],
  );

  return (
    <SideSheet
      visible={visible}
      placement='right'
      width={isMobile ? '100%' : 960}
      bodyStyle={{ padding: 0 }}
      onCancel={onCancel}
      title={
        <Space>
          <Tag color='blue' shape='circle'>
            {t('详情')}
          </Tag>
          <Title heading={4} className='m-0'>
            {t('邀请关系')}
          </Title>
          <Text type='tertiary' className='ml-2'>
            {user?.username || '-'} (ID: {user?.id || '-'})
          </Text>
        </Space>
      }
    >
      <div className='p-4'>
        <Spin spinning={loading}>
          <div className='grid grid-cols-1 md:grid-cols-2 gap-3 mb-4'>
            <UserSummaryCard
              title={t('目标用户')}
              user={view?.target}
              emptyTip={t('未找到用户信息')}
              t={t}
            />
            <UserSummaryCard
              title={t('邀请人')}
              user={view?.inviter}
              emptyTip={t('该用户无邀请人')}
              t={t}
            />
          </div>

          <div className='mb-2 flex items-center justify-between'>
            <div className='font-medium'>
              {t('受邀用户列表')}
              <Text type='tertiary' className='ml-2 text-xs'>
                {t('共')} {view?.total || 0} {t('人')}
              </Text>
            </div>
          </div>

          <CardTable
            columns={inviteeColumns}
            dataSource={view?.invitees || []}
            rowKey={(row) => row.id}
            scroll={{ x: 'max-content' }}
            hidePagination={false}
            pagination={{
              currentPage: page,
              pageSize,
              total: view?.total || 0,
              pageSizeOpts: [10, 20, 50],
              showSizeChanger: true,
              onPageChange: handlePageChange,
              onPageSizeChange: handlePageSizeChange,
            }}
            empty={
              <Empty
                image={
                  <IllustrationNoResult style={{ width: 150, height: 150 }} />
                }
                darkModeImage={
                  <IllustrationNoResultDark
                    style={{ width: 150, height: 150 }}
                  />
                }
                description={t('暂无受邀用户')}
                style={{ padding: 30 }}
              />
            }
            size='middle'
          />
        </Spin>
      </div>
    </SideSheet>
  );
};

export default AffiliateRelationModal;
