import React from 'react';
import { Card, Empty } from '@douyinfe/semi-ui';
import { Headset, Mail, SquareArrowOutUpRight } from 'lucide-react';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';

const IMAGE_EXTENSIONS = /\.(jpg|jpeg|png|gif|webp|svg|bmp|ico|avif)(\?.*)?$/i;

const isImageUrl = (url) => {
  if (!url) return false;
  try {
    const pathname = new URL(url).pathname;
    return IMAGE_EXTENSIONS.test(pathname);
  } catch {
    return IMAGE_EXTENSIONS.test(url);
  }
};

const ImageItem = ({ url, label }) => (
  <div className='flex flex-col items-center gap-2'>
    <div className='w-32 h-32 rounded-xl overflow-hidden bg-semi-color-fill-0'>
      <img src={url} alt={label} className='w-full h-full object-cover' />
    </div>
    {label && (
      <span className='text-xs text-semi-color-text-2'>{label}</span>
    )}
  </div>
);

const LinkItem = ({ url, label, t }) => (
  <div className='flex flex-col items-center gap-2'>
    <a
      href={url}
      target='_blank'
      rel='noopener noreferrer'
      className='w-32 h-32 rounded-xl bg-semi-color-fill-0 flex flex-col items-center justify-center gap-2 hover:bg-semi-color-fill-1 transition-colors cursor-pointer'
    >
      <SquareArrowOutUpRight size={24} className='text-semi-color-text-2' />
      <span className='text-xs text-semi-color-text-2'>
        {t('在浏览器中打开')}
      </span>
    </a>
    {label && (
      <span className='text-xs text-semi-color-text-2'>{label}</span>
    )}
  </div>
);

const SupportPanel = ({ supportData, CARD_PROPS, ILLUSTRATION_SIZE, t }) => {
  const rawItems = supportData?.items || [];
  const items = rawItems.filter((i) => i.url || i.qrcode);
  const email = supportData?.email || '';
  const hasContent = items.length > 0;

  return (
    <Card
      {...CARD_PROPS}
      className='shadow-sm !rounded-2xl lg:col-span-1 !flex !flex-col [&>.semi-card-body]:!flex [&>.semi-card-body]:!flex-col [&>.semi-card-body]:!flex-1'
      title={
        <div className='flex items-center justify-between w-full'>
          <div className='flex items-center gap-2'>
            <Headset size={16} />
            {t('关于 Amux 社区')}
          </div>
          {email && (
            <a
              href={`mailto:${email}`}
              className='flex items-center gap-1 text-xs text-semi-color-text-2 hover:text-semi-color-primary transition-colors'
            >
              <Mail size={12} />
              {email}
            </a>
          )}
        </div>
      }
    >
      {hasContent ? (
        <div className='flex items-center justify-center gap-5 flex-wrap flex-1'>
          {items.map((item, i) => {
            const url = item.url || item.qrcode;
            return isImageUrl(url) ? (
              <ImageItem key={i} url={url} label={item.label} />
            ) : (
              <LinkItem key={i} url={url} label={item.label} t={t} />
            );
          })}
        </div>
      ) : (
        <div className='flex justify-center items-center py-8'>
          <Empty
            image={<IllustrationConstruction style={ILLUSTRATION_SIZE} />}
            darkModeImage={
              <IllustrationConstructionDark style={ILLUSTRATION_SIZE} />
            }
            title={t('暂无社区信息')}
            description={t('请联系管理员在系统设置中配置')}
          />
        </div>
      )}
    </Card>
  );
};

export default SupportPanel;
