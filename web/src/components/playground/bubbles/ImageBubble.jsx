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

import React, { useMemo, useState } from 'react';
import {
  Button,
  ImagePreview,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { Pencil, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { inferAspectRatio } from './aspectRatio';
import { MODALITY } from '../../../constants/playground.constants';
import { getLobeHubIcon } from '../../../helpers/render';
import { inferVendorIconKey } from '../vendorIcon';

// 单条 assistant 消息的图片气泡。统一对话窗口里和文本/视频气泡并列出现。
//
// content 形如 [{type:'image_url', image_url:{url}, revised_prompt}, ...]
//
// 不在这里渲染 prompt——上一条 user 消息已经显示了；prompt 重复反而让阅读
// 节奏变碎。meta（model / 关键参数 / 时间）作为底部小注释保留。
const extractImages = (content) => {
  if (!Array.isArray(content)) return [];
  return content
    .filter((p) => p && p.type === 'image_url' && p.image_url?.url)
    .map((p) => ({ url: p.image_url.url, revisedPrompt: p.revised_prompt }));
};

const ImageBubble = ({ message, onContinueEdit, supportsContinueEdit }) => {
  const { t } = useTranslation();
  const status = message?.status || 'complete';
  const isLoading = status === 'loading';
  const isError = status === 'error';
  const images = extractImages(message?.content);
  const meta = message?.meta || {};
  const params = meta.params || {};

  const [previewIdx, setPreviewIdx] = useState(-1);
  const previewOpen = previewIdx >= 0;
  const previewUrls = useMemo(() => images.map((x) => x.url), [images]);

  // 元信息文本片段（model 单独走 vendor logo + 名字渲染，这里只放参数）
  const paramPieces = useMemo(() => {
    const arr = [];
    ['size', 'aspect_ratio', 'aspectRatio', 'quality'].forEach((k) => {
      if (params[k] !== undefined && params[k] !== '' && params[k] !== 'auto') {
        arr.push(String(params[k]));
      }
    });
    return arr;
  }, [params]);

  if (isLoading) {
    // 骨架按目标尺寸的同比例渲染。「面积常数 + 比例反推宽度」让
    // 1:1 / 16:9 / 9:16 视觉面积接近；shimmer 动画本身就是「生成中」
    // 的反馈，不再叠 spinner 和文字，让占位更安静。
    const ar = inferAspectRatio(params, MODALITY.IMAGE);
    const targetArea = 320 * 320;
    const rawWidth = Math.sqrt(targetArea * ar);
    const width = Math.max(240, Math.min(480, rawWidth));
    return (
      <div
        className='playground-image-bubble playground-skeleton'
        style={{ width, aspectRatio: ar }}
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
          {message?.errorMessage || t('生成失败')}
        </Typography.Text>
      </div>
    );
  }

  if (images.length === 0) return null;

  // 多图布局策略（最多 10 张）：列数 + 单图最大高度按张数自适应。
  // 1 张：占满；2 张：左右；3 张：一排；4 张：2x2；5-6 张：3 列；
  // 7-9 张：3 列填满；10 张：5 列两行。grid 用 1fr 等分宽度，单图按
  // capH 限高、保留原比例。
  const shown = images.slice(0, 10);
  const n = shown.length;
  const grid =
    n <= 1
      ? { cols: 1, capH: 480 }
      : n === 2
        ? { cols: 2, capH: 320 }
        : n === 3
          ? { cols: 3, capH: 240 }
          : n === 4
            ? { cols: 2, capH: 280 }
            : n <= 6
              ? { cols: 3, capH: 200 }
              : n <= 9
                ? { cols: 3, capH: 180 }
                : { cols: 5, capH: 140 };

  return (
    <div className='playground-image-bubble'>
      <div
        className='playground-image-bubble-media'
        style={
          n > 1
            ? {
                display: 'grid',
                gridTemplateColumns: `repeat(${grid.cols}, 1fr)`,
                gap: 4,
              }
            : undefined
        }
      >
        {shown.map((img, i) => {
          const canContinue =
            supportsContinueEdit &&
            typeof img.url === 'string' &&
            img.url.startsWith('data:');
          return (
            <div key={i} className='flex justify-center items-start'>
              <div
                className='group relative cursor-zoom-in'
                style={{
                  display: n > 1 ? 'block' : 'inline-block',
                  width: n > 1 ? '100%' : 'auto',
                  maxWidth: '100%',
                  backgroundColor: 'var(--semi-color-fill-0)',
                  // hover 时图片有 1.02 缩放动效，必须裁切掉溢出，否则
                  // 放大后的边缘会盖住下方 meta 行
                  overflow: 'hidden',
                }}
                onClick={() => setPreviewIdx(i)}
              >
                <img
                  src={img.url}
                  alt={`image-${i}`}
                  loading='lazy'
                  className='transition-transform duration-300 group-hover:scale-[1.02]'
                  style={{
                    display: 'block',
                    maxWidth: '100%',
                    maxHeight: grid.capH,
                    width: n > 1 ? '100%' : 'auto',
                    height: n > 1 ? grid.capH : 'auto',
                    objectFit: n > 1 ? 'cover' : 'contain',
                  }}
                />
                <div
                  className='absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none'
                  style={{
                    background:
                      'linear-gradient(to top, rgba(0,0,0,0.25) 0%, transparent 40%)',
                  }}
                />
                {canContinue && (
                  <Tooltip content={t('以此图继续编辑')}>
                    <Button
                      icon={<Pencil size={14} />}
                      size='small'
                      theme='solid'
                      type='tertiary'
                      onClick={(e) => {
                        e.stopPropagation();
                        onContinueEdit?.(img);
                      }}
                      className='!absolute opacity-0 group-hover:opacity-100 transition-opacity'
                      style={{
                        bottom: 8,
                        right: 8,
                        background: 'rgba(0,0,0,0.55)',
                        color: 'white',
                        border: 'none',
                        backdropFilter: 'blur(4px)',
                      }}
                    />
                  </Tooltip>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {(meta.model || paramPieces.length > 0) && (
        // 元信息行：厂商 logo + 大写模型名 + 大写参数，全部 baseline 对齐。
        // 整行字体属性统一在容器上声明（uppercase / letter-spacing / 字号），
        // 子元素继承——避免每段单独写一遍。
        <div
          className='playground-image-bubble-meta flex items-center flex-wrap'
          style={{
            gap: 6,
            lineHeight: 1,
            fontSize: 13,
            fontWeight: 600,
            letterSpacing: 0.4,
            textTransform: 'uppercase',
            // text-1 比 text-2 亮一档：依然是次要信息，但读起来更清晰
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

      {previewOpen && (
        <ImagePreview
          src={previewUrls}
          visible={previewOpen}
          currentIndex={previewIdx}
          onVisibleChange={(v) => {
            if (!v) setPreviewIdx(-1);
          }}
          onClose={() => setPreviewIdx(-1)}
          infinite={false}
        />
      )}
    </div>
  );
};

export default ImageBubble;
