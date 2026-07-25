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

import React, { useCallback, useEffect, useState } from 'react';
import {
  Table,
  Typography,
  Tag,
  Button,
  Input,
  Modal,
  Popconfirm,
  Empty,
  Space,
  Tooltip,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import {
  API,
  showError,
  showSuccess,
  renderQuota,
  timestamp2string,
} from '../../../helpers';
import { useIsMobile } from '../../../hooks/common/useIsMobile';

const { Text } = Typography;

const PAGE_SIZE = 10;

/**
 * 当期（尚未开奖）参与明细。
 *
 * 与「中奖名单」的关键区别：这里的数字全是实时推演出来的，不是存档——用户还在消耗，
 * 每次展开/翻页都会按最新消耗重算概率。概率口径与结算完全一致（后端同一个函数），
 * 但被屏蔽者会被移出分母，所以屏蔽某人之后其余人的概率会当场上浮，这是预期行为。
 */
const BlindBoxPeriodParticipants = ({ drawDate, onChanged, t }) => {
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState('');
  const [keywordDraft, setKeywordDraft] = useState('');
  // 屏蔽需要填原因，用弹窗承载；null 表示未在操作中
  const [blocking, setBlocking] = useState(null);
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(
    async (p = page, kw = keyword) => {
      setLoading(true);
      try {
        const params = new URLSearchParams({ p, page_size: PAGE_SIZE });
        if (kw) params.set('keyword', kw);
        const res = await API.get(`/api/blindbox/period?${params.toString()}`);
        const { success, message, data } = res.data;
        if (success) {
          setItems(data?.participants?.items || []);
          setTotal(data?.participants?.total || 0);
        } else {
          showError(message || t('获取当期参与明细失败'));
        }
      } catch (e) {
        showError(t('获取当期参与明细失败'));
      } finally {
        setLoading(false);
      }
    },
    [page, keyword, t],
  );

  useEffect(() => {
    load(1, '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drawDate]);

  const refresh = (p = page) => {
    load(p, keyword);
    // 屏蔽会改变入池人数与奖池，父级那一行的统计也要跟着刷新
    onChanged?.();
  };

  const submitBlock = async () => {
    if (!blocking) return;
    setSubmitting(true);
    try {
      const res = await API.post('/api/blindbox/blocks', {
        user_id: blocking.user_id,
        scope: blocking.scope,
        draw_date: blocking.scope === 'period' ? drawDate : '',
        reason: reason.trim(),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('已屏蔽该用户的中奖资格'));
        setBlocking(null);
        setReason('');
        refresh();
      } else {
        showError(message || t('屏蔽失败'));
      }
    } catch (e) {
      showError(t('屏蔽失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const unblock = async (blockId) => {
    try {
      const res = await API.delete(`/api/blindbox/blocks/${blockId}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('已解除屏蔽'));
        refresh();
      } else {
        showError(message || t('解除屏蔽失败'));
      }
    } catch (e) {
      showError(t('解除屏蔽失败'));
    }
  };

  const renderState = (r) => {
    if (r.blocked_permanent) {
      return (
        <Tag color='red' shape='circle'>
          {t('永久屏蔽')}
        </Tag>
      );
    }
    if (r.blocked_period) {
      return (
        <Tag color='orange' shape='circle'>
          {t('本期屏蔽')}
        </Tag>
      );
    }
    if (!r.qualified) {
      return (
        <Tag color='grey' shape='circle'>
          {t('未达标')}
        </Tag>
      );
    }
    return (
      <Tag color='green' shape='circle'>
        {t('已入池')}
      </Tag>
    );
  };

  const columns = [
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (v, r) => (
        <div>
          <span
            className={
              r.blocked_permanent || r.blocked_period ? 'opacity-60' : ''
            }
          >
            {v || '-'}{' '}
            <Text type='tertiary' className='text-xs'>
              #{r.user_id}
            </Text>
          </span>
          <div>
            <Text type='tertiary' className='text-xs'>
              {t('参与于')} {timestamp2string(r.joined_at)}
            </Text>
          </div>
        </div>
      ),
    },
    {
      title: t('本期消耗'),
      dataIndex: 'consume_quota',
      render: (v) => renderQuota(v),
    },
    {
      title: t('权重'),
      dataIndex: 'weight',
      render: (v) => (v > 0 ? v : <Text type='tertiary'>-</Text>),
    },
    {
      title: (
        <Tooltip content={t('单次抽取被抽中的概率，与开奖存档口径一致')}>
          <span>{t('首抽概率')}</span>
        </Tooltip>
      ),
      dataIndex: 'first_prob',
      render: (v) =>
        v > 0 ? `${(v * 100).toFixed(2)}%` : <Text type='tertiary'>0.00%</Text>,
    },
    {
      title: (
        <Tooltip
          content={t(
            '本期至少中一次的估算值。加权无放回抽样没有闭式解，按 1-(1-p)^名额数 近似，仅供参考',
          )}
        >
          <span>{t('综合中奖率')}</span>
        </Tooltip>
      ),
      dataIndex: 'any_prob',
      render: (v) =>
        v > 0 ? (
          <Text strong className='text-amber-600 dark:text-amber-400'>
            {(v * 100).toFixed(2)}%
          </Text>
        ) : (
          <Text type='tertiary'>0.00%</Text>
        ),
    },
    {
      title: t('状态'),
      dataIndex: 'qualified',
      render: (v, r) => (
        <div>
          {renderState(r)}
          {r.block_reason ? (
            <div>
              <Text type='tertiary' className='text-xs'>
                {r.block_reason}
              </Text>
            </div>
          ) : null}
        </div>
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      render: (_, r) => (
        <Space>
          {r.blocked_period ? (
            <Popconfirm
              title={t('解除本期屏蔽')}
              content={t('解除后该用户恢复本期中奖资格')}
              onConfirm={() => unblock(r.period_block_id)}
            >
              <Button size='small' theme='light' type='tertiary'>
                {t('解除本期')}
              </Button>
            </Popconfirm>
          ) : (
            <Button
              size='small'
              theme='light'
              type='warning'
              onClick={() => {
                setReason('');
                setBlocking({ ...r, scope: 'period' });
              }}
            >
              {t('屏蔽本期')}
            </Button>
          )}
          {r.blocked_permanent ? (
            <Popconfirm
              title={t('解除永久屏蔽')}
              content={t('解除后该用户恢复中奖资格')}
              onConfirm={() => unblock(r.permanent_block_id)}
            >
              <Button size='small' theme='light' type='tertiary'>
                {t('解除永久')}
              </Button>
            </Popconfirm>
          ) : (
            <Button
              size='small'
              theme='light'
              type='danger'
              onClick={() => {
                setReason('');
                setBlocking({ ...r, scope: 'permanent' });
              }}
            >
              {t('永久屏蔽')}
            </Button>
          )}
        </Space>
      ),
    },
  ];

  return (
    <div className='py-2'>
      <div className='flex items-center gap-2 mb-2'>
        <Input
          prefix={<IconSearch />}
          placeholder={t('用户名或用户 ID')}
          value={keywordDraft}
          onChange={setKeywordDraft}
          onEnterPress={() => {
            setKeyword(keywordDraft);
            setPage(1);
            load(1, keywordDraft);
          }}
          style={{ width: 220 }}
          showClear
          size='small'
        />
        <Button
          size='small'
          theme='light'
          onClick={() => {
            setKeyword(keywordDraft);
            setPage(1);
            load(1, keywordDraft);
          }}
        >
          {t('查询')}
        </Button>
        <Text type='tertiary' className='text-xs'>
          {t('屏蔽仅影响中奖资格，不影响用户参与、消耗与其它任何功能')}
        </Text>
      </div>

      <Table
        size='small'
        className='w-full'
        columns={columns}
        dataSource={items}
        loading={loading}
        rowKey='user_id'
        scroll={isMobile ? { x: 'max-content' } : undefined}
        empty={<Empty description={t('本期暂无用户参与')} />}
        pagination={{
          currentPage: page,
          pageSize: PAGE_SIZE,
          total,
          onPageChange: (p) => {
            setPage(p);
            load(p, keyword);
          },
        }}
      />

      <Modal
        title={
          blocking?.scope === 'permanent'
            ? t('永久屏蔽中奖资格')
            : t('屏蔽本期中奖资格')
        }
        visible={!!blocking}
        onCancel={() => setBlocking(null)}
        onOk={submitBlock}
        confirmLoading={submitting}
        okText={t('确认屏蔽')}
        cancelText={t('取消')}
      >
        <div className='mb-2'>
          <Text>
            {t('用户')}：{blocking?.username || '-'} #{blocking?.user_id}
          </Text>
        </div>
        <div className='mb-2'>
          <Text type='tertiary' className='text-xs'>
            {blocking?.scope === 'permanent'
              ? t(
                  '永久屏蔽后，只要不解除，该用户即使参与也一定不会中奖。仅影响中奖资格，不影响参与、消耗与其它功能。',
                )
              : t(
                  '仅屏蔽该用户在本期的中奖资格，开奖后自动失效。仅影响中奖资格，不影响参与、消耗与其它功能。',
                )}
          </Text>
        </div>
        <Input
          placeholder={t('屏蔽原因（选填，仅管理员可见）')}
          value={reason}
          onChange={setReason}
          maxLength={200}
        />
      </Modal>
    </div>
  );
};

export default BlindBoxPeriodParticipants;
