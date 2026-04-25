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

import { MODALITY } from '../../../constants/playground.constants';

// 不同生图/视频模型用的「比例参数」差异巨大，schema 是后台手填的，
// 字段名/格式都不统一，例如：
//   - Gemini 系列：aspect_ratio / aspectRatio / "Aspect Ratio"，值形如 "4:3"
//   - GPT 系列   ：size，值形如 "1536x1024"
//   - SeeDance 等：ratio / resolution，值形如 "16:9" / "1280x720"
//   - 少数模型把宽高写在 width / height 两个独立字段里
//
// 这里做一份「容错型」推断：先扫显式比例字段（任何近似于 ratio 的 key），
// 不行再扫尺寸字段（size / resolution / dimensions / image_size 之类），
// 再不行尝试 width + height 两元字段，最后按 modality 兜底默认值。

// 默认兜底比例：图片 1:1，视频 16:9，其它退到正方形。
const DEFAULT_BY_MODALITY = {
  [MODALITY.IMAGE]: 1,
  [MODALITY.VIDEO]: 16 / 9,
};

const normKey = (k) => String(k || '').toLowerCase().replace(/[\s_-]+/g, '');

// 形如 "16:9" / "16x9" / "4 / 3" / "1.5:1" → number；不合法返回 null
const parseRatioStr = (raw) => {
  if (raw == null) return null;
  const s = String(raw).trim();
  if (!s || s === 'auto') return null;
  const m = s.match(/^(\d+(?:\.\d+)?)\s*[:/x×]\s*(\d+(?:\.\d+)?)$/i);
  if (!m) return null;
  const w = Number(m[1]);
  const h = Number(m[2]);
  return w > 0 && h > 0 ? w / h : null;
};

// 形如 "1024x1024" / "1920×1080" / "1536X1024" → number；不合法返回 null
const parseSizeStr = (raw) => {
  if (raw == null) return null;
  const s = String(raw).trim();
  if (!s || s === 'auto') return null;
  const m = s.match(/^(\d+)\s*[xX×]\s*(\d+)$/);
  if (!m) return null;
  const w = Number(m[1]);
  const h = Number(m[2]);
  return w > 0 && h > 0 ? w / h : null;
};

// 把 params 的字段名规范化后做命中。aspect_ratio / aspectratio /
// AspectRatio / "Aspect Ratio" 都会归一到同一个 normKey="aspectratio"。
const RATIO_KEYS = new Set([
  'aspectratio',
  'aspect',
  'ratio',
  'imageratio',
  'videoratio',
]);
const SIZE_KEYS = new Set([
  'size',
  'imagesize',
  'resolution',
  'dimensions',
  'imagedimensions',
]);

export const inferAspectRatio = (params, modality) => {
  const fallback = DEFAULT_BY_MODALITY[modality] || 1;
  if (!params || typeof params !== 'object') return fallback;

  // 1) 优先用显式比例字段
  for (const [k, v] of Object.entries(params)) {
    if (!RATIO_KEYS.has(normKey(k))) continue;
    const r = parseRatioStr(v);
    if (r) return r;
  }

  // 2) 退而求其次：尺寸/分辨率字段
  for (const [k, v] of Object.entries(params)) {
    if (!SIZE_KEYS.has(normKey(k))) continue;
    const r = parseSizeStr(v);
    if (r) return r;
  }

  // 3) width + height 两元字段（少见但合法）
  let w = null;
  let h = null;
  for (const [k, v] of Object.entries(params)) {
    const nk = normKey(k);
    const num = typeof v === 'number' ? v : Number(v);
    if (!Number.isFinite(num) || num <= 0) continue;
    if (nk === 'width' || nk === 'imagewidth') w = num;
    else if (nk === 'height' || nk === 'imageheight') h = num;
  }
  if (w && h) return w / h;

  // 4) 都没拿到 → modality 兜底
  return fallback;
};
