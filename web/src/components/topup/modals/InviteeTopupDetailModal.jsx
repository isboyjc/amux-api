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

import React, { useState, useEffect } from 'react';
import { Modal, Table, Spin, Tag, Typography } from '@douyinfe/semi-ui';
import { Receipt } from 'lucide-react';
import { API, showError, timestamp2string } from '../../../helpers';

const { Text } = Typography;

const InviteeTopupDetailModal = ({ t, visible, onCancel, inviteeId, inviteeName, stripeCurrencySymbol = '¥' }) => {
  const [topups, setTopups] = useState([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize] = useState(10);

  const loadTopups = async (page = 1) => {
    if (!inviteeId) return;
    
    try {
      setLoading(true);
      const res = await API.get(`/api/user/invitee/${inviteeId}/topups`, {
        params: {
          p: page,
          page_size: pageSize,
        },
      });
      const { success, message, data } = res.data;
      if (success) {
        setTopups(data.items || []);
        setTotal(data.total || 0);
      } else {
        showError(message);
      }
    } catch (error) {
      console.error('加载充值明细失败:', error);
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible && inviteeId) {
      setCurrentPage(1);
      loadTopups(1);
    } else if (!visible) {
      // 清理状态，防止内存泄漏
      setTopups([]);
      setTotal(0);
      setCurrentPage(1);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, inviteeId]);

  const handlePageChange = (page) => {
    setCurrentPage(page);
    loadTopups(page);
  };

  const getStatusTag = (status) => {
    const statusMap = {
      success: { color: 'green', text: t('成功') },
      pending: { color: 'amber', text: t('待支付') },
      failed: { color: 'red', text: t('失败') },
      expired: { color: 'grey', text: t('已过期') },
    };
    const statusInfo = statusMap[status] || { color: 'grey', text: status };
    return <Tag color={statusInfo.color}>{statusInfo.text}</Tag>;
  };

  const columns = [
    {
      title: t('充值时间'),
      dataIndex: 'complete_time',
      key: 'complete_time',
      render: (time) => timestamp2string(time),
    },
    {
      title: t('支付金额'),
      dataIndex: 'money',
      key: 'money',
      width: 120,
      render: (money, record) => {
        const symbol = record.payment_method === 'stripe' ? stripeCurrencySymbol : '¥';
        return `${symbol}${money?.toFixed(2) || 0}`;
      },
    },
    {
      title: t('订单状态'),
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => getStatusTag(status),
    },
  ];

  // 计算当前页总金额
  const currentPageTotal = topups.reduce((sum, item) => sum + (item.money || 0), 0);

  return (
    <Modal
      title={
        <div className='flex items-center gap-2'>
          <Receipt size={18} />
          <Text>
            {t('充值明细')} - {inviteeName}
          </Text>
        </div>
      }
      visible={visible}
      onCancel={onCancel}
      footer={
        topups.length > 0 && (
          <div className='text-right px-4 py-2 border-t'>
            <Text type='secondary'>
              {t('当前页合计')}：<Text strong>¥{currentPageTotal.toFixed(2)}</Text>
              {' '}|{' '}
              {t('共')} <Text strong>{total}</Text> {t('条记录')}
            </Text>
          </div>
        )
      }
      width={700}
      centered
    >
      <Spin spinning={loading}>
        <Table
          columns={columns}
          dataSource={topups}
          pagination={{
            currentPage: currentPage,
            pageSize: pageSize,
            total: total,
            onPageChange: handlePageChange,
            showSizeChanger: false,
          }}
          size='small'
          empty={<Text type='tertiary'>{t('暂无充值记录')}</Text>}
        />
      </Spin>
    </Modal>
  );
};

export default InviteeTopupDetailModal;
