import React from 'react';
import { Card, Empty } from '@douyinfe/semi-ui';
import { Headset, Mail } from 'lucide-react';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';

const QrCodeItem = ({ qrcode, label }) => (
  <div className='flex flex-col items-center gap-2'>
    <div className='w-32 h-32 rounded-xl overflow-hidden bg-semi-color-fill-0'>
      <img
        src={qrcode}
        alt={label}
        className='w-full h-full object-cover'
      />
    </div>
    {label && (
      <span className='text-xs text-semi-color-text-2'>{label}</span>
    )}
  </div>
);

const SupportPanel = ({ supportData, CARD_PROPS, ILLUSTRATION_SIZE, t }) => {
  const items = supportData?.items?.filter((i) => i.qrcode) || [];
  const email = supportData?.email || '';
  const hasContent = items.length > 0 || email;

  return (
    <Card
      {...CARD_PROPS}
      className='shadow-sm !rounded-2xl lg:col-span-1'
      title={
        <div className='flex items-center gap-2'>
          <Headset size={16} />
          {t('用户支持')}
        </div>
      }
    >
      {hasContent ? (
        <div className='flex flex-col items-center gap-4 py-2'>
          {items.length > 0 && (
            <>
              <div className='flex items-start justify-center gap-5 flex-wrap'>
                {items.map((item, i) => (
                  <QrCodeItem
                    key={i}
                    qrcode={item.qrcode}
                    label={item.label}
                  />
                ))}
              </div>
              <span className='text-xs text-semi-color-text-3'>
                {t('扫码添加获取更多支持')}
              </span>
            </>
          )}

          {email && (
            <div className='flex items-center gap-1.5 text-semi-color-text-2'>
              <Mail size={14} />
              <a
                href={`mailto:${email}`}
                className='text-xs hover:text-semi-color-primary transition-colors'
              >
                {email}
              </a>
            </div>
          )}
        </div>
      ) : (
        <div className='flex justify-center items-center py-8'>
          <Empty
            image={<IllustrationConstruction style={ILLUSTRATION_SIZE} />}
            darkModeImage={
              <IllustrationConstructionDark style={ILLUSTRATION_SIZE} />
            }
            title={t('暂无用户支持信息')}
            description={t('请联系管理员在系统设置中配置')}
          />
        </div>
      )}
    </Card>
  );
};

export default SupportPanel;
