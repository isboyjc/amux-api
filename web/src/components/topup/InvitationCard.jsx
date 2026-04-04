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

import React, { useState, useEffect, useContext } from 'react';
import {
  Avatar,
  Typography,
  Card,
  Button,
  Input,
  Badge,
  Space,
  Table,
  Spin,
} from '@douyinfe/semi-ui';
import { Copy, Users, BarChart2, TrendingUp, Gift, Zap, FileText } from 'lucide-react';
import { API, showError } from '../../helpers';
import InviteeTopupDetailModal from './modals/InviteeTopupDetailModal';
import { StatusContext } from '../../context/Status';

const { Text } = Typography;

const InvitationCard = ({
  t,
  userState,
  renderQuota,
  setOpenTransfer,
  affLink,
  handleAffLinkClick,
  stripeCurrencySymbol = '¥',
}) => {
  const [statusState] = useContext(StatusContext);
  const [invitees, setInvitees] = useState([]);
  const [inviteesLoading, setInviteesLoading] = useState(false);
  const [inviteesTotal, setInviteesTotal] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [selectedInvitee, setSelectedInvitee] = useState(null);
  const [rebateStats, setRebateStats] = useState(null);
  
  // 从配置中获取是否允许查看受邀用户列表
  const affShowInvitees = statusState?.status?.AffShowInvitees === 'true';
  const affRebateRatio = parseFloat(statusState?.status?.AffRebateRatio || '0');
  
  // 从 StatusContext 获取 Stripe 币种符号（优先），否则使用 props 传入的
  const actualStripeCurrencySymbol = statusState?.status?.stripe_currency_symbol || stripeCurrencySymbol;
  const enableStripeTopup = statusState?.status?.enable_stripe_topup || false;

  // 加载返现统计
  const loadRebateStats = async () => {
    try {
      const res = await API.get('/api/user/aff_rebate_stats');
      const { success, data } = res.data;
      if (success) {
        setRebateStats(data);
      }
    } catch (error) {
      console.error('加载返现统计失败:', error);
    }
  };

  useEffect(() => {
    loadRebateStats();
  }, []);

  // 加载受邀请用户列表
  const loadInvitees = async (page = 1, size = 10) => {
    try {
      setInviteesLoading(true);
      const res = await API.get('/api/user/invitees', {
        params: {
          p: page,
          page_size: size,
        },
      });
      const { success, message, data } = res.data;
      if (success) {
        setInvitees(data.items || []);
        setInviteesTotal(data.total || 0);
      } else {
        showError(message);
        setInvitees([]);
        setInviteesTotal(0);
      }
    } catch (error) {
      console.error('加载受邀请用户失败:', error);
      showError(t('加载失败'));
      setInvitees([]);
      setInviteesTotal(0);
    } finally {
      setInviteesLoading(false);
    }
  };

  useEffect(() => {
    if (affShowInvitees && userState?.user?.aff_count > 0) {
      loadInvitees(currentPage, pageSize);
    }
  }, [affShowInvitees, userState?.user?.aff_count]);

  const handlePageChange = (page) => {
    setCurrentPage(page);
    loadInvitees(page, pageSize);
  };

  const handleViewDetail = (record) => {
    setSelectedInvitee(record);
    setDetailModalVisible(true);
  };

  const columns = [
    {
      title: t('ID'),
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: t('用户名'),
      dataIndex: 'username',
      key: 'username',
      render: (text, record) => record.display_name || text,
    },
    {
      title: t('充值金额'),
      dataIndex: 'topup_amount',
      key: 'topup_amount',
      width: 150,
      render: (amount, record) => {
        // 累计金额使用系统默认币种（根据是否启用Stripe判断）
        const symbol = enableStripeTopup ? actualStripeCurrencySymbol : '¥';
        return `${symbol}${amount?.toFixed(2) || '0.00'}`;
      },
    },
    {
      title: t('操作'),
      key: 'action',
      width: 120,
      render: (text, record) => (
        <Button
          size='small'
          type='tertiary'
          icon={<FileText size={14} />}
          onClick={() => handleViewDetail(record)}
        >
          {t('查看明细')}
        </Button>
      ),
    },
  ];

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      {/* 卡片头部 */}
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='green' className='mr-3 shadow-md'>
          <Gift size={16} />
        </Avatar>
        <div>
          <Typography.Text className='text-lg font-medium'>
            {t('邀请奖励')}
          </Typography.Text>
          <div className='text-xs'>{t('邀请好友获得额外奖励')}</div>
        </div>
      </div>

      {/* 收益展示区域 */}
      <Space vertical style={{ width: '100%' }}>
        {/* 统计数据统一卡片 */}
        <Card
          className='!rounded-xl w-full'
          cover={
            <div
              className='relative h-30'
              style={{
                '--palette-primary-darkerChannel': '0 75 80',
                backgroundImage: `linear-gradient(0deg, rgba(var(--palette-primary-darkerChannel) / 80%), rgba(var(--palette-primary-darkerChannel) / 80%)), url('/cover-4.webp')`,
                backgroundSize: 'cover',
                backgroundPosition: 'center',
                backgroundRepeat: 'no-repeat',
              }}
            >
              {/* 标题和按钮 */}
              <div className='relative z-10 h-full flex flex-col justify-between p-4'>
                <div className='flex justify-between items-center'>
                  <Text strong style={{ color: 'white', fontSize: '16px' }}>
                    {t('收益统计')}
                  </Text>
                  <Button
                    type='primary'
                    theme='solid'
                    size='small'
                    disabled={
                      !userState?.user?.aff_quota ||
                      userState?.user?.aff_quota <= 0
                    }
                    onClick={() => setOpenTransfer(true)}
                    className='!rounded-lg'
                  >
                    <Zap size={12} className='mr-1' />
                    {t('划转到余额')}
                  </Button>
                </div>

                {/* 统计数据 */}
                <div className='grid grid-cols-3 gap-6 mt-4'>
                  {/* 待使用收益 */}
                  <div className='text-center'>
                    <div
                      className='text-base sm:text-2xl font-bold mb-2'
                      style={{ color: 'white' }}
                    >
                      {renderQuota(userState?.user?.aff_quota || 0)}
                    </div>
                    <div className='flex items-center justify-center text-sm'>
                      <TrendingUp
                        size={14}
                        className='mr-1'
                        style={{ color: 'rgba(255,255,255,0.8)' }}
                      />
                      <Text
                        style={{
                          color: 'rgba(255,255,255,0.8)',
                          fontSize: '12px',
                        }}
                      >
                        {t('待使用收益')}
                      </Text>
                    </div>
                    {rebateStats && (rebateStats.reg_pending_quota > 0 || rebateStats.topup_pending_quota > 0) && (
                      <div className='flex items-center justify-center gap-2 mt-1'>
                        <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: '10px' }}>
                          {t('注册')}: {renderQuota(rebateStats.reg_pending_quota || 0)}
                        </span>
                        <span style={{ color: 'rgba(255,255,255,0.35)', fontSize: '10px' }}>|</span>
                        <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: '10px' }}>
                          {t('充值')}: {renderQuota(rebateStats.topup_pending_quota || 0)}
                        </span>
                      </div>
                    )}
                  </div>

                  {/* 总收益 */}
                  <div className='text-center'>
                    <div
                      className='text-base sm:text-2xl font-bold mb-2'
                      style={{ color: 'white' }}
                    >
                      {renderQuota(userState?.user?.aff_history_quota || 0)}
                    </div>
                    <div className='flex items-center justify-center text-sm'>
                      <BarChart2
                        size={14}
                        className='mr-1'
                        style={{ color: 'rgba(255,255,255,0.8)' }}
                      />
                      <Text
                        style={{
                          color: 'rgba(255,255,255,0.8)',
                          fontSize: '12px',
                        }}
                      >
                        {t('总收益')}
                      </Text>
                    </div>
                    {rebateStats && (rebateStats.reg_history_quota > 0 || rebateStats.topup_history_quota > 0) && (
                      <div className='flex items-center justify-center gap-2 mt-1'>
                        <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: '10px' }}>
                          {t('注册')}: {renderQuota(rebateStats.reg_history_quota || 0)}
                        </span>
                        <span style={{ color: 'rgba(255,255,255,0.35)', fontSize: '10px' }}>|</span>
                        <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: '10px' }}>
                          {t('充值')}: {renderQuota(rebateStats.topup_history_quota || 0)}
                        </span>
                      </div>
                    )}
                  </div>

                  {/* 邀请人数 */}
                  <div className='text-center'>
                    <div
                      className='text-base sm:text-2xl font-bold mb-2'
                      style={{ color: 'white' }}
                    >
                      {userState?.user?.aff_count || 0}
                    </div>
                    <div className='flex items-center justify-center text-sm'>
                      <Users
                        size={14}
                        className='mr-1'
                        style={{ color: 'rgba(255,255,255,0.8)' }}
                      />
                      <Text
                        style={{
                          color: 'rgba(255,255,255,0.8)',
                          fontSize: '12px',
                        }}
                      >
                        {t('邀请人数')}
                      </Text>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          }
        >
          {/* 邀请链接部分 */}
          <Input
            value={affLink}
            readonly
            className='!rounded-lg'
            prefix={t('邀请链接')}
            suffix={
              <Button
                type='primary'
                theme='solid'
                onClick={handleAffLinkClick}
                icon={<Copy size={14} />}
                className='!rounded-lg'
              >
                {t('复制')}
              </Button>
            }
          />
        </Card>

        {/* 受邀请用户列表 */}
        {affShowInvitees && userState?.user?.aff_count > 0 && (
          <Card
            className='!rounded-xl w-full'
            title={
              <div className='flex items-center justify-between w-full'>
                <div className='flex items-center gap-2'>
                  <Users size={16} />
                  <Text type='tertiary'>
                    {t('受邀用户')} ({inviteesTotal})
                  </Text>
                </div>
                {affRebateRatio > 0 && (
                  <Badge
                    count={`${t('返现比例')}：${affRebateRatio.toFixed(1)}%`}
                    type='primary'
                  />
                )}
              </div>
            }
          >
            <Spin spinning={inviteesLoading}>
              <Table
                columns={columns}
                dataSource={invitees}
                pagination={{
                  currentPage: currentPage,
                  pageSize: pageSize,
                  total: inviteesTotal,
                  onPageChange: handlePageChange,
                  showSizeChanger: false,
                }}
                size='small'
                empty={
                  <div className='py-8'>
                    <Text type='tertiary'>{t('暂无受邀用户')}</Text>
                  </div>
                }
              />
            </Spin>
          </Card>
        )}

        {/* 奖励说明 */}
        <Card
          className='!rounded-xl w-full'
          title={<Text type='tertiary'>{t('奖励说明')}</Text>}
        >
          <div className='space-y-3'>
            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('邀请好友注册可获得注册奖励')}
              </Text>
            </div>
            {affRebateRatio > 0 && (
              <div className='flex items-start gap-2'>
                <Badge dot type='primary' />
                <Text type='tertiary' className='text-sm'>
                  {t('好友充值时，您可获得充值金额')} {affRebateRatio.toFixed(1)}% {t('的返现奖励')}
                </Text>
              </div>
            )}

            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('通过划转功能将奖励额度转入到您的账户余额中')}
              </Text>
            </div>

            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('邀请的好友越多，获得的奖励越多')}
              </Text>
            </div>
          </div>
        </Card>
      </Space>

      {/* 充值明细弹框 */}
      <InviteeTopupDetailModal
        t={t}
        visible={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        inviteeId={selectedInvitee?.id}
        inviteeName={selectedInvitee?.display_name || selectedInvitee?.username}
        stripeCurrencySymbol={actualStripeCurrencySymbol}
      />
    </Card>
  );
};

export default InvitationCard;
