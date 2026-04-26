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

import React, { useState } from 'react';
import { Button, ImagePreview, Tooltip } from '@douyinfe/semi-ui';
import { Pencil } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { optimizeImageUrl, pixelWidth } from '../../../helpers';

// 用户气泡里展示「参考图组」的网格。1-12 张自适应：
//   - 1 张：单图最大 240px，原比例
//   - 2-3 张：2/3 列，方格
//   - 4 张：2×2
//   - 5-9 张：3 列
//   - 10-12 张：4 列（≥13 张被截断展示前 12 张）
// 点击任一图打开 Semi ImagePreview 大图查看（带翻页）；hover 时右下角浮出
// 「以此图继续编辑」按钮——和 ImageBubble 的同款交互打通，让用户既能复用
// 自己上传过的参考图，也能复用模型生成的图。
const getGridConfig = (n) => {
  if (n <= 1) return { cols: 1, cell: 240, fit: 'contain' };
  if (n === 2) return { cols: 2, cell: 140, fit: 'cover' };
  if (n === 3) return { cols: 3, cell: 110, fit: 'cover' };
  if (n === 4) return { cols: 2, cell: 140, fit: 'cover' };
  if (n <= 6) return { cols: 3, cell: 110, fit: 'cover' };
  if (n <= 9) return { cols: 3, cell: 95, fit: 'cover' };
  return { cols: 4, cell: 85, fit: 'cover' };
};

const RefImageGrid = ({ urls, onContinueEdit }) => {
  const { t } = useTranslation();
  const [previewIdx, setPreviewIdx] = useState(-1);
  if (!urls || urls.length === 0) return null;

  const shown = urls.slice(0, 12);
  const n = shown.length;
  const { cols, cell, fit } = getGridConfig(n);

  return (
    <>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: `repeat(${cols}, ${cell}px)`,
          gap: 4,
        }}
      >
        {shown.map((url, i) => {
          // 父级 handleContinueEdit 现在同时支持 data: URL 和 http(s) 远程
          // 链接（视频模型走 R2 直传后历史里全是远程 URL），只要 url 是
          // 字符串、handler 存在就挂按钮——具体的 download / seed-as-done /
          // 重新上传选择由父级按当前 modality 决策。
          const canContinue =
            typeof url === 'string' &&
            url.length > 0 &&
            typeof onContinueEdit === 'function';
          return (
            <div
              key={i}
              className='group relative'
              style={{
                width: cell,
                height: n > 1 ? cell : 'auto',
                borderRadius: 8,
                overflow: 'hidden',
                cursor: 'zoom-in',
                backgroundColor: 'var(--semi-color-fill-0)',
              }}
              onClick={() => setPreviewIdx(i)}
            >
              <img
                src={optimizeImageUrl(url, { width: pixelWidth(cell) })}
                alt=''
                loading='lazy'
                style={{
                  display: 'block',
                  width: '100%',
                  height: n > 1 ? '100%' : 'auto',
                  objectFit: fit,
                }}
              />
              {canContinue && (
                <Tooltip content={t('以此图继续编辑')}>
                  <Button
                    icon={<Pencil size={12} />}
                    size='small'
                    theme='solid'
                    type='tertiary'
                    onClick={(e) => {
                      e.stopPropagation();
                      onContinueEdit?.({ url });
                    }}
                    className='!absolute opacity-0 group-hover:opacity-100 transition-opacity'
                    style={{
                      bottom: 6,
                      right: 6,
                      background: 'rgba(0,0,0,0.55)',
                      color: 'white',
                      border: 'none',
                      backdropFilter: 'blur(4px)',
                    }}
                  />
                </Tooltip>
              )}
            </div>
          );
        })}
      </div>
      {previewIdx >= 0 && (
        <ImagePreview
          src={shown}
          visible={previewIdx >= 0}
          currentIndex={previewIdx}
          onVisibleChange={(v) => {
            if (!v) setPreviewIdx(-1);
          }}
          onClose={() => setPreviewIdx(-1)}
          infinite={false}
        />
      )}
    </>
  );
};

export default RefImageGrid;
