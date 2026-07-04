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
import { Typography } from '@douyinfe/semi-ui';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { getLobeHubIcon } from '../../../helpers/render';
import { inferVendorIconKey } from '../vendorIcon';

// 单条 assistant 消息的音频气泡。结构和 VideoBubble 对齐：不渲染 prompt
// （上一条 user 消息已展示），只关心音频播放器和元信息。
//
// content 形如：[{ type:'audio_url', audio_url:{ url } }]
const extractAudio = (content) => {
  if (!Array.isArray(content)) return null;
  const a = content.find((p) => p?.type === 'audio_url' && p.audio_url?.url);
  return a ? { url: a.audio_url.url } : null;
};

const AUDIO_BUBBLE_WIDTH = 320;

const AudioBubble = ({ message }) => {
  const { t } = useTranslation();
  const status = message?.status || 'complete';
  const isLoading = status === 'loading' || status === 'polling';
  const isError = status === 'error';
  const audio = extractAudio(message?.content);
  const meta = message?.meta || {};
  const params = meta.params || {};

  const paramPieces = useMemo(() => {
    const arr = [];
    ['voice', 'response_format', 'speed'].forEach((k) => {
      if (params[k] !== undefined && params[k] !== '' && params[k] !== null) {
        arr.push(`${k}=${params[k]}`);
      }
    });
    return arr;
  }, [params]);

  if (isLoading) {
    // 同 ImageBubble/VideoBubble：shimmer 骨架自带「生成中」语义。
    return (
      <div
        className='playground-image-bubble playground-skeleton'
        style={{ width: AUDIO_BUBBLE_WIDTH, height: 56, borderRadius: 12 }}
      />
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
          {message?.errorMessage || t('音频生成失败')}
        </Typography.Text>
      </div>
    );
  }

  if (!audio) return null;

  return (
    <div className='playground-image-bubble' style={{ width: AUDIO_BUBBLE_WIDTH }}>
      <div style={{ padding: 10 }}>
        <audio
          src={audio.url}
          controls
          preload='metadata'
          style={{ width: '100%', display: 'block' }}
        >
          {t('浏览器不支持播放音频')}
        </audio>
      </div>

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
            overflowWrap: 'anywhere',
            padding: '0 12px 10px',
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

export default AudioBubble;
