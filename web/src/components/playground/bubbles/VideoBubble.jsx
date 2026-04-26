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

// 气泡显示宽度推断：按 ar 反推 + 保面积一致 + 高/宽双 cap。
// 把这个宽度钉到气泡根上，video 用 100% 填满，底部 meta 行就在这个宽度
// 内 wrap，不会再被长 model 名 / 多个参数撑得比视频还宽。
//
// 设计点：
//   - targetArea 定一个"目标视觉面积"，让 1:1 / 16:9 / 9:16 在视觉上面积接近
//   - 高度 cap 480：瘦长视频（9:16）反推回宽度，避免气泡被拉得很高
//   - 宽度 cap 480：宽屏视频（21:9）也不会无限扩张
//   - 最小宽度 240：留够空间放 model 徽标 + 至少一两个参数
//   - 骨架与正片同一公式：loading → complete 不会跳尺寸
const VIDEO_TARGET_AREA = 320 * 320;
const VIDEO_MAX_DIM = 480;
const VIDEO_MIN_WIDTH = 240;
const computeBubbleWidth = (ar) => {
  let width = Math.sqrt(VIDEO_TARGET_AREA * ar);
  // 瘦长视频按高度 cap 反推回宽度
  if (width / ar > VIDEO_MAX_DIM) width = VIDEO_MAX_DIM * ar;
  width = Math.min(VIDEO_MAX_DIM, width);
  width = Math.max(VIDEO_MIN_WIDTH, width);
  return width;
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

  const ar = inferAspectRatio(params, MODALITY.VIDEO);
  const bubbleWidth = computeBubbleWidth(ar);

  if (isLoading) {
    // 同 ImageBubble：shimmer 骨架自带「生成中」语义，不叠 spinner / 文字。
    // 进度条留下来——它是真实百分比信号，不是装饰；放到骨架底部贴边。
    return (
      <div
        className='playground-image-bubble playground-skeleton'
        style={{ width: bubbleWidth, aspectRatio: ar, position: 'relative' }}
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
    // 钉死气泡宽度 = 推算出来的 bubbleWidth：video 用 width:100% 填满，
    // 下面 meta 行也在这个宽度里 wrap。这样不会再出现"meta 比视频长 →
    // 气泡被撑大 → 视频左侧 / 下方留空"的情况。
    <div className='playground-image-bubble' style={{ width: bubbleWidth }}>
      <div className='playground-image-bubble-media'>
        <div
          style={{
            width: '100%',
            backgroundColor: 'var(--semi-color-fill-0)',
          }}
        >
          <video
            src={video.url}
            controls
            preload='metadata'
            style={{
              display: 'block',
              width: '100%',
              height: 'auto',
              // maxHeight 兜底：万一 ar 推断与实际视频比例不一致（瘦长
              // 视频 + 推断成 16:9），不至于撑出非常高的气泡
              maxHeight: VIDEO_MAX_DIM,
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
            // 气泡宽度被钉死后，长 model 名 / 多参数会自然 wrap；这一行
            // 让超长不间断字符（如 doubao-seedance-2-0-260128）也能在
            // 任意位置断行，避免溢出气泡边缘
            overflowWrap: 'anywhere',
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
