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

import React, { useMemo } from 'react';
import { Progress, Tooltip, Typography } from '@douyinfe/semi-ui';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { inferAspectRatio } from './aspectRatio';
import { MODALITY } from '../../../constants/playground.constants';
import { getLobeHubIcon } from '../../../helpers/render';
import { inferVendorIconKey } from '../vendorIcon';

// 单条 assistant 消息的视频气泡。结构和 ImageBubble 对齐：
// 不渲染 prompt（上一条 user 消息已展示），只关心媒体内容和元信息。
//
// content 形如：
//   [{type:'video_url', video_url:{url}}, {type:'image_url', image_url:{url}}?]
// 第二个图片 part 是末帧预览（可选）。
const extractVideo = (content) => {
  if (!Array.isArray(content)) return null;
  const v = content.find((p) => p?.type === 'video_url' && p.video_url?.url);
  if (!v) return null;
  const last = content.find((p) => p?.type === 'image_url' && p.image_url?.url);
  return { url: v.video_url.url, lastFrameUrl: last?.image_url?.url };
};

const VideoBubble = ({ message }) => {
  const { t } = useTranslation();
  const status = message?.status || 'complete';
  const isLoading = status === 'loading' || status === 'polling';
  const isError = status === 'error';
  const progress = typeof message?.progress === 'number' ? message.progress : 0;
  const video = extractVideo(message?.content);
  const meta = message?.meta || {};
  const params = meta.params || {};

  // 模型单独走 vendor logo + 名字渲染；这里只收参数文本
  const paramPieces = useMemo(() => {
    const arr = [];
    ['resolution', 'ratio', 'duration'].forEach((k) => {
      if (params[k] !== undefined && params[k] !== '' && params[k] !== -1) {
        arr.push(`${k}=${params[k]}`);
      }
    });
    return arr;
  }, [params]);

  if (isLoading) {
    // 同 ImageBubble：shimmer 骨架自带「生成中」语义，不叠 spinner / 文字。
    // 进度条留下来——它是真实百分比信号，不是装饰；放到骨架底部贴边。
    const ar = inferAspectRatio(params, MODALITY.VIDEO);
    const targetArea = 320 * 320;
    const rawWidth = Math.sqrt(targetArea * ar);
    const width = Math.max(280, Math.min(480, rawWidth));
    return (
      <div
        className='playground-image-bubble playground-skeleton'
        style={{ width, aspectRatio: ar, position: 'relative' }}
      >
        {progress > 0 && (
          <div
            style={{
              position: 'absolute',
              left: 12,
              right: 12,
              bottom: 12,
            }}
          >
            <Progress percent={progress} size='small' showInfo={false} />
          </div>
        )}
      </div>
    );
  }

  if (isError) {
    return (
      <div className='flex items-center gap-2 py-1'>
        <X
          size={14}
          style={{ color: 'var(--semi-color-danger)', flexShrink: 0 }}
        />
        <Typography.Text type='danger' className='text-sm'>
          {message?.errorMessage || t('视频生成失败')}
        </Typography.Text>
      </div>
    );
  }

  if (!video) return null;

  return (
    <div className='playground-image-bubble'>
      <div className='playground-image-bubble-media flex justify-start'>
        <div
          style={{
            maxWidth: '100%',
            backgroundColor: 'var(--semi-color-fill-0)',
            display: 'inline-block',
          }}
        >
          <video
            src={video.url}
            controls
            preload='metadata'
            style={{
              display: 'block',
              maxWidth: '100%',
              maxHeight: 480,
              width: 'auto',
              height: 'auto',
            }}
          >
            {t('浏览器不支持播放视频')}
          </video>
        </div>
      </div>

      {video.lastFrameUrl && (
        <div style={{ padding: '10px 12px 0' }}>
          <Tooltip content={t('末帧预览')}>
            <img
              src={video.lastFrameUrl}
              alt={t('末帧')}
              style={{
                maxWidth: 120,
                maxHeight: 120,
                borderRadius: 6,
                opacity: 0.9,
                display: 'block',
              }}
            />
          </Tooltip>
        </div>
      )}

      {(meta.model || paramPieces.length > 0) && (
        <div
          className='playground-image-bubble-meta flex items-center flex-wrap'
          style={{
            gap: 6,
            lineHeight: 1,
            fontSize: 13,
            fontWeight: 600,
            letterSpacing: 0.4,
            textTransform: 'uppercase',
            color: 'var(--semi-color-text-1)',
          }}
        >
          {meta.model && (
            <span className='inline-flex items-center' style={{ gap: 6 }}>
              <span
                className='inline-flex items-center justify-center'
                style={{ width: 18, height: 18 }}
              >
                {getLobeHubIcon(inferVendorIconKey(meta.model), 16)}
              </span>
              <span>{meta.model}</span>
            </span>
          )}
          {paramPieces.map((p, i) => (
            <React.Fragment key={i}>
              <span style={{ opacity: 0.6 }}>·</span>
              <span>{p}</span>
            </React.Fragment>
          ))}
        </div>
      )}
    </div>
  );
};

export default VideoBubble;
