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

import React from 'react';
import {
  Typography,
  Tag,
  Banner,
  Skeleton,
  Descriptions,
  Row,
  Col,
  Divider,
} from '@douyinfe/semi-ui';
import { Gift } from 'lucide-react';
import { Link } from 'react-router-dom';
import { renderQuota, timestamp2string } from '../../../helpers';

const { Text, Title } = Typography;

/**
 * 规则区块：把后台配置翻译成人话直接标注在管理页上，
 * 免得运营还要回运营设置页比对参数。
 *
 * 规则值来自后端内存中的 setting 实例（即定时任务真正用于结算的那份），
 * 而不是 options 表，避免"配置写库了但没热更新"时管理员看到假象。
 *
 * 本组件渲染为普通区块（不自带 Card 外壳）——它嵌在管理页内容卡的
 * 「规则统计」Tab 里，再套一层 Card 会出现双重边框。
 */
const BlindBoxRuleCard = ({ summary, loading, t }) => {
  if (loading || !summary) {
    return (
      <Skeleton placeholder={<Skeleton.Paragraph rows={4} />} loading active>
        <div />
      </Skeleton>
    );
  }

  const { setting, period, preview, health } = summary;
  const isPercent = setting.pool_mode === 'percent';

  const poolDesc = isPercent
    ? t('参与者总消耗 × {{rate}}% ÷ {{count}} 份（等额）', {
        rate: (setting.percent_rate * 100).toFixed(2).replace(/\.?0+$/, ''),
        count: setting.percent_count,
      })
    : t('固定奖池，共 {{count}} 个奖项', {
        count: (setting.prizes || []).length,
      });

  return (
    <div>
      <div className='flex items-center gap-2 mb-3'>
        <Gift size={18} className='text-amber-500' />
        <Title heading={6} className='!mb-0'>
          {t('当前生效规则')}
        </Title>
        {setting.enabled ? (
          <Tag color='green' shape='circle'>
            {t('已启用')}
          </Tag>
        ) : (
          <Tag color='grey' shape='circle'>
            {t('已停用')}
          </Tag>
        )}
        <Link
          to='/console/setting?tab=operation'
          className='text-xs ml-auto'
          style={{ color: 'var(--semi-color-link)' }}
        >
          {t('前往修改配置')}
        </Link>
      </div>

      {/* 健康告警：这两项不满足时盲盒会静默不工作，必须显式提示 */}
      {!health.log_consume_enabled && (
        <Banner
          fullMode={false}
          type='danger'
          className='!rounded-lg mb-3'
          closeIcon={null}
          description={t(
            '「记录消费日志」已关闭。达标判定完全依赖消费日志，当前状态下系统统计不到任何用户消耗，盲盒将永远无人达标。',
          )}
        />
      )}
      {!health.is_master_node && (
        <Banner
          fullMode={false}
          type='warning'
          className='!rounded-lg mb-3'
          closeIcon={null}
          description={t(
            '当前节点不是主节点，开奖定时任务不在本节点运行。若集群中没有主节点，将不会产生任何开奖。',
          )}
        />
      )}
      {setting.enabled && isPercent && setting.percent_rate === 0 && (
        <Banner
          fullMode={false}
          type='warning'
          className='!rounded-lg mb-3'
          closeIcon={null}
          description={t('奖池比例为 0，每期奖池均为空，不会产生中奖记录。')}
        />
      )}
      {setting.enabled && !isPercent && (setting.prizes || []).length === 0 && (
        <Banner
          fullMode={false}
          type='warning'
          className='!rounded-lg mb-3'
          closeIcon={null}
          description={t('固定奖池未配置任何奖项，不会产生中奖记录。')}
        />
      )}

      <Row gutter={[16, 16]}>
        <Col xs={24} md={14}>
          <Descriptions
            align='left'
            size='small'
            data={[
              { key: t('每日开奖时间'), value: setting.draw_time },
              {
                key: t('当前周期'),
                value: `${timestamp2string(period.period_start)} → ${timestamp2string(period.next_draw_at)}`,
              },
              {
                key: t('参与门槛'),
                value: `${setting.threshold_quota} (${renderQuota(setting.threshold_quota)})`,
              },
              {
                key: t('入池条件'),
                value: t(
                  '当期消耗达标后，用户还需主动点击「参与本期」；未点击的不进入奖池',
                ),
              },
              {
                key: t('奖池模式'),
                value: isPercent ? t('百分比奖池') : t('固定奖池'),
              },
              { key: t('奖池构成'), value: poolDesc },
              {
                key: t('领取窗口'),
                // 领取截止 = 下一次开奖时点，不是固定 24 小时（跨夏令时会是 23/25 小时）
                value: t('开奖后至下一次开奖时点（{{time}}），逾期作废', {
                  time: setting.draw_time,
                }),
              },
            ]}
          />

          {/* 固定模式把奖项逐条列出，百分比模式没有具名奖项，不必展示 */}
          {!isPercent && (setting.prizes || []).length > 0 && (
            <div className='mt-3'>
              <Text type='tertiary' className='text-xs'>
                {t('奖项列表')}
              </Text>
              <div className='mt-1.5 flex flex-wrap gap-1.5'>
                {setting.prizes.map((p, i) => (
                  <Tag key={i} color='amber' shape='circle'>
                    {p.name} · {renderQuota(p.quota)}
                  </Tag>
                ))}
              </div>
            </div>
          )}
        </Col>

        <Col xs={24} md={10}>
          {/* 本期实时预览：让运营在开奖前就能判断今天奖池是否合理 */}
          <div className='rounded-xl p-3 bg-amber-50 dark:bg-amber-950/30'>
            <Text strong className='text-sm'>
              {t('本期实时预览')}
            </Text>
            <Text type='tertiary' className='text-xs block mt-0.5'>
              {t('按当前配置推演，开奖时以实际数据为准')}
            </Text>
            <Divider margin='10px' />
            <div className='grid grid-cols-2 gap-y-2'>
              <PreviewItem
                label={t('已参与人数')}
                value={preview.entry_count}
              />
              <PreviewItem
                label={t('当前入池人数')}
                value={preview.participant_count}
              />
              <PreviewItem
                label={t('参与总消耗')}
                value={renderQuota(preview.total_consume)}
              />
              <PreviewItem
                label={t('预计中奖人数')}
                value={
                  preview.prize_count > preview.winner_count
                    ? t('{{n}}（{{lapse}} 个奖项将作废）', {
                        n: preview.winner_count,
                        lapse: preview.prize_count - preview.winner_count,
                      })
                    : preview.winner_count
                }
              />
              <PreviewItem
                label={t('预计奖池总额')}
                value={renderQuota(preview.total_prize_quota)}
              />
            </div>
          </div>
        </Col>
      </Row>
    </div>
  );
};

const PreviewItem = ({ label, value }) => (
  <div>
    <Text type='tertiary' className='text-xs block'>
      {label}
    </Text>
    <Text strong className='text-sm'>
      {value}
    </Text>
  </div>
);

export default BlindBoxRuleCard;
