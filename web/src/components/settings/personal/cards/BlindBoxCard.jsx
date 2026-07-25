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
import {
  Card,
  Button,
  Typography,
  Avatar,
  Spin,
  Tag,
  Collapsible,
  Modal,
  Pagination,
  Empty,
} from '@douyinfe/semi-ui';
import { Gift, ChevronDown, ChevronUp } from 'lucide-react';
import Turnstile from 'react-turnstile';
import { API, showError, showSuccess, renderQuota } from '../../../../helpers';
import BlindBoxRevealModal from './BlindBoxRevealModal';

// 用户需要主动完成的两个动作。都要过 Turnstile：这两下正是刷量脚本最想批量做的事。
const ACTION_ENTER = 'enter';
const ACTION_CLAIM = 'claim';

const MODAL_PAGE_SIZE = 10;

/**
 * 一条参与记录的结果展示。
 * 后端对 pending 记录不下发奖品字段，这里也绝不能编造内容。
 */
const recordOutcome = (r, t) => {
  switch (r.status) {
    case 'claimed':
      return {
        text: renderQuota(r.prize_quota),
        tag: (
          <Tag color='green' size='small'>
            {t('已领取')}
          </Tag>
        ),
        highlight: true,
      };
    case 'pending':
      return {
        text: t('待揭晓'),
        tag: (
          <Tag color='orange' size='small'>
            {t('待开启')}
          </Tag>
        ),
      };
    case 'expired':
      return {
        text: renderQuota(r.prize_quota),
        tag: (
          <Tag color='grey' size='small'>
            {t('已过期')}
          </Tag>
        ),
      };
    case 'waiting':
      return {
        text: '—',
        tag: (
          <Tag color='blue' size='small'>
            {t('待开奖')}
          </Tag>
        ),
      };
    default:
      return {
        text: '—',
        tag: (
          <Tag color='grey' size='small'>
            {t('未中奖')}
          </Tag>
        ),
      };
  }
};

const RecordRow = ({ record, t }) => {
  const outcome = recordOutcome(record, t);
  return (
    <div className='flex items-center justify-between py-2 px-2.5 bg-slate-50 dark:bg-slate-800 rounded-lg'>
      <Typography.Text className='text-xs text-gray-500'>
        {record.draw_date}
      </Typography.Text>
      <div className='flex items-center gap-2'>
        <Typography.Text
          className={`text-xs ${outcome.highlight ? 'font-semibold text-amber-600 dark:text-amber-400' : ''}`}
        >
          {outcome.text}
        </Typography.Text>
        {outcome.tag}
      </div>
    </div>
  );
};

const BlindBoxCard = ({ t, status, turnstileEnabled, turnstileSiteKey }) => {
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(null);
  const [turnstileModalVisible, setTurnstileModalVisible] = useState(false);
  const [turnstileWidgetKey, setTurnstileWidgetKey] = useState(0);
  // Turnstile 校验通过后要继续执行哪个动作
  const [pendingAction, setPendingAction] = useState(null);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(true);
  const [data, setData] = useState({
    threshold_quota: 0,
    qualified: false,
    joined: false,
    pending: null,
    total_won_quota: 0,
    records: [],
    records_total: 0,
  });

  // 开奖揭晓弹框（开盒成功后展示中奖内容）
  const [reveal, setReveal] = useState(null);

  // 「查看更多」分页弹框
  const [moreVisible, setMoreVisible] = useState(false);
  const [moreLoading, setMoreLoading] = useState(false);
  const [morePage, setMorePage] = useState(1);
  const [moreRecords, setMoreRecords] = useState([]);
  const [moreTotal, setMoreTotal] = useState(0);

  const fetchStatus = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/user/blindbox');
      const { success, data: d, message } = res.data;
      if (success) {
        setData(d);
      } else {
        showError(message || t('获取盲盒状态失败'));
      }
    } catch (error) {
      showError(t('获取盲盒状态失败'));
    } finally {
      setLoading(false);
      setInitialLoaded(true);
    }
  };

  const fetchMoreRecords = async (page) => {
    setMoreLoading(true);
    try {
      const res = await API.get(
        `/api/user/blindbox/records?p=${page}&page_size=${MODAL_PAGE_SIZE}`,
      );
      const { success, data: d, message } = res.data;
      if (success) {
        setMoreRecords(d.items || []);
        setMoreTotal(d.total || 0);
      } else {
        showError(message || t('获取参与记录失败'));
      }
    } catch (error) {
      showError(t('获取参与记录失败'));
    } finally {
      setMoreLoading(false);
    }
  };

  const openMore = () => {
    setMoreVisible(true);
    setMorePage(1);
    fetchMoreRecords(1);
  };

  const shouldTriggerTurnstile = (message) => {
    if (!turnstileEnabled) return false;
    if (typeof message !== 'string') return true;
    return message.includes('Turnstile');
  };

  const runAction = async (action, token) => {
    const base =
      action === ACTION_ENTER
        ? '/api/user/blindbox/enter'
        : '/api/user/blindbox';
    const url = token ? `${base}?turnstile=${encodeURIComponent(token)}` : base;

    setActionLoading(action);
    try {
      const res = await API.post(url);
      const { success, data: d, message } = res.data;
      if (success) {
        setTurnstileModalVisible(false);
        setPendingAction(null);
        if (action === ACTION_CLAIM) {
          setReveal(d);
        } else {
          showSuccess(message || t('已参与本期抽奖'));
        }
        fetchStatus();
      } else {
        // 未带 token 时被 Turnstile 拦下 → 弹验证码，通过后重放同一个动作
        if (!token && shouldTriggerTurnstile(message)) {
          if (!turnstileSiteKey) {
            showError('Turnstile is enabled but site key is empty.');
            return;
          }
          setPendingAction(action);
          setTurnstileModalVisible(true);
          return;
        }
        if (token && shouldTriggerTurnstile(message)) {
          setTurnstileWidgetKey((v) => v + 1);
        }
        showError(
          message || (action === ACTION_ENTER ? t('参与失败') : t('开盒失败')),
        );
      }
    } catch (error) {
      showError(action === ACTION_ENTER ? t('参与失败') : t('开盒失败'));
    } finally {
      setActionLoading(null);
    }
  };

  useEffect(() => {
    if (status?.blindbox_enabled) {
      fetchStatus();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status?.blindbox_enabled]);

  if (!status?.blindbox_enabled) {
    return null;
  }

  const hasPending = !!data.pending;
  const joined = !!data.joined;

  const subtitle = () => {
    if (!initialLoaded) return t('正在加载...');
    if (hasPending) return t('你有一个待开启的盲盒');
    if (joined) return t('已参与本期，等待开奖');
    if (data.qualified) return t('已达参与门槛，点击右侧参与本期');
    return t('本期消耗达到门槛即可参与抽奖，消耗越多中奖率越高');
  };

  /**
   * 卡片只留这一个按钮，状态互斥、按优先级取第一个成立的：
   *   待开盲盒 > 已参与 > 可参与 > 未达标
   * 有未开的盲盒时不给下一期入口 —— 必须先把手上的盒子开掉。
   */
  const renderActionButton = () => {
    if (!initialLoaded) {
      return (
        <Button type='primary' theme='solid' loading disabled>
          {t('加载中')}
        </Button>
      );
    }
    if (hasPending) {
      return (
        <Button
          type='primary'
          theme='solid'
          icon={<Gift size={16} />}
          onClick={() => runAction(ACTION_CLAIM)}
          loading={actionLoading === ACTION_CLAIM}
          className='!bg-amber-500 hover:!bg-amber-600'
        >
          {t('开启盲盒')}
        </Button>
      );
    }
    if (joined) {
      return (
        <Button type='primary' theme='solid' disabled>
          {t('已参与本期 · 待开奖')}
        </Button>
      );
    }
    if (data.qualified) {
      return (
        <Button
          type='primary'
          theme='solid'
          icon={<Gift size={16} />}
          onClick={() => runAction(ACTION_ENTER)}
          loading={actionLoading === ACTION_ENTER}
          className='!bg-amber-500 hover:!bg-amber-600'
        >
          {t('参与本期')}
        </Button>
      );
    }
    return (
      <Button type='primary' theme='solid' disabled>
        {t('消耗 {{amount}} 即可参与', {
          amount: renderQuota(data.threshold_quota),
        })}
      </Button>
    );
  };

  return (
    <Card className='!rounded-2xl'>
      <Modal
        title='Security Check'
        visible={turnstileModalVisible}
        footer={null}
        centered
        onCancel={() => {
          setTurnstileModalVisible(false);
          setPendingAction(null);
          setTurnstileWidgetKey((v) => v + 1);
        }}
      >
        <div className='flex justify-center py-2'>
          <Turnstile
            key={turnstileWidgetKey}
            sitekey={turnstileSiteKey}
            onVerify={(token) => {
              if (pendingAction) runAction(pendingAction, token);
            }}
            onExpire={() => {
              setTurnstileWidgetKey((v) => v + 1);
            }}
          />
        </div>
      </Modal>

      <BlindBoxRevealModal
        visible={!!reveal}
        prize={reveal}
        onClose={() => setReveal(null)}
        t={t}
      />

      {/* 全部参与记录 */}
      <Modal
        title={t('参与记录')}
        visible={moreVisible}
        footer={null}
        centered
        onCancel={() => setMoreVisible(false)}
      >
        <Spin spinning={moreLoading}>
          {moreRecords.length === 0 ? (
            <Empty description={t('暂无参与记录')} style={{ padding: 24 }} />
          ) : (
            <div className='flex flex-col gap-1.5'>
              {moreRecords.map((r) => (
                <RecordRow key={r.draw_date} record={r} t={t} />
              ))}
            </div>
          )}
          {moreTotal > MODAL_PAGE_SIZE && (
            <div className='flex justify-center mt-4'>
              <Pagination
                currentPage={morePage}
                pageSize={MODAL_PAGE_SIZE}
                total={moreTotal}
                onPageChange={(p) => {
                  setMorePage(p);
                  fetchMoreRecords(p);
                }}
              />
            </div>
          )}
        </Spin>
      </Modal>

      {/* 卡片头部：一个折叠区 + 一个按钮 */}
      <div className='flex items-center justify-between'>
        <div
          className='flex items-center flex-1 cursor-pointer min-w-0'
          onClick={() => setIsCollapsed(!isCollapsed)}
        >
          <Avatar size='small' color='amber' className='mr-3 shadow-md'>
            <Gift size={16} />
          </Avatar>
          <div className='flex-1 min-w-0'>
            <div className='flex items-center gap-2'>
              <Typography.Text className='text-lg font-medium'>
                {t('每日盲盒')}
              </Typography.Text>
              {isCollapsed ? (
                <ChevronDown size={16} className='text-gray-400' />
              ) : (
                <ChevronUp size={16} className='text-gray-400' />
              )}
            </div>
            <div className='text-xs text-gray-500 dark:text-gray-400 truncate'>
              {subtitle()}
            </div>
          </div>
        </div>
        <div className='ml-3 shrink-0'>{renderActionButton()}</div>
      </div>

      {/* 展开后：近期参与 */}
      <Collapsible isOpen={!isCollapsed} keepDOM>
        <Spin spinning={loading}>
          <div className='mt-4'>
            <div className='flex items-center justify-between mb-2'>
              <Typography.Text strong className='text-sm'>
                {t('近期参与')}
              </Typography.Text>
              <div className='flex items-center gap-1'>
                <Typography.Text type='tertiary' className='text-xs'>
                  {t('总中奖额度')}
                </Typography.Text>
                <Typography.Text
                  strong
                  className='text-sm text-amber-600 dark:text-amber-400'
                >
                  {renderQuota(data.total_won_quota || 0)}
                </Typography.Text>
              </div>
            </div>

            {(data.records || []).length === 0 ? (
              <div className='text-xs text-gray-400 py-4 text-center'>
                {t('暂无参与记录')}
              </div>
            ) : (
              <div className='flex flex-col gap-1.5'>
                {data.records.map((r) => (
                  <RecordRow key={r.draw_date} record={r} t={t} />
                ))}
              </div>
            )}

            {data.records_total > (data.records || []).length && (
              <div className='flex justify-center mt-2'>
                <Button size='small' theme='borderless' onClick={openMore}>
                  {t('查看更多')}
                </Button>
              </div>
            )}
          </div>
        </Spin>
      </Collapsible>
    </Card>
  );
};

export default BlindBoxCard;
