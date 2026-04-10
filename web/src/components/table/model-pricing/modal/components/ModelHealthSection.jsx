import React from 'react';
import { Card, Avatar, Typography, RadioGroup, Radio } from '@douyinfe/semi-ui';
import { IconPulse } from '@douyinfe/semi-icons';
import ModelHealthTimeline from '../../view/common/ModelHealthTimeline';

const { Text } = Typography;

const ModelHealthSection = ({
  healthData,
  modelData,
  usableGroup = {},
  timeRange = '24h',
  onTimeRangeChange,
  t,
}) => {
  if (!healthData || !modelData) return null;

  const groups = modelData.enable_groups || [];
  const visibleGroups = groups.filter((g) => usableGroup[g]);

  if (visibleGroups.length === 0) return null;

  return (
    <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
      <div className='flex items-center justify-between mb-4'>
        <div className='flex items-center'>
          <Avatar size='small' color='green' className='mr-2 shadow-md'>
            <IconPulse size={16} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>
              {t('模型健康状态')}
            </Text>
            <div
              className='text-xs'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t('各分组健康状态')}
            </div>
          </div>
        </div>
        {onTimeRangeChange && (
          <RadioGroup
            type='button'
            size='small'
            value={timeRange}
            onChange={(e) => onTimeRangeChange(e.target.value)}
            style={{ flexShrink: 0 }}
          >
            <Radio value='24h'>{t('最近24小时')}</Radio>
            <Radio value='7d'>{t('最近7天')}</Radio>
          </RadioGroup>
        )}
      </div>
      <ModelHealthTimeline
        healthData={healthData}
        modelName={modelData.model_name}
        groups={visibleGroups}
        compact={false}
        t={t}
      />
    </Card>
  );
};

export default ModelHealthSection;
