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

// 「显示侧」图片 URL 优化：套一层 Cloudflare Image Resizing（cdn-cgi/image）
// 让 CF 边缘按需生成缩略 / WebP / AVIF，省客户端带宽和首屏时间。
//
// **核心约定：只在 <img src> / 缩略图渲染处调用**。送给上游模型的 URL、
// 持久化到 message attachments 的 URL、`resolveUploadedUrl` 返回的 URL，
// 全部用原图。物理上保证「显示=优化版本，发送=原图」边界清晰。
//
// 启用条件（任一不满足都返回原 URL，无副作用）：
//   1) admin 在「对象存储设置」打开了 ImageTransformEnabled 开关
//   2) 配置了 R2PublicBaseURL（自定义域名 + CF 代理；r2.dev 不行）
//   3) 入参 url 是 http(s) 字符串、且属于我们配置的 publicBaseURL
//
// 这两个条件由 /api/status 下发，前端在 setStatusData 时写到 localStorage。
// 这里同步读 localStorage——render path 调用，不能 await，也不该走 React
// hook（很多调用点是普通 JSX 表达式）。

// readSettings 从 localStorage 读两项配置；任何异常都视作未启用。
function readSettings() {
  try {
    const enabled =
      localStorage.getItem('storage_image_transform_enabled') === 'true';
    const base = (localStorage.getItem('storage_public_base_url') || '')
      .replace(/\/$/, ''); // 容错：去掉可能的尾斜杠
    return { enabled, base };
  } catch {
    return { enabled: false, base: '' };
  }
}

/**
 * 给一个图片 URL 套上 cdn-cgi/image 变换前缀。不改变 host，path 保持原样。
 *
 * 输入：原图 URL
 * 输出：可能优化过的 URL（不满足条件时原样返回）
 *
 * @param {string} url 原始图片地址（任意来源）
 * @param {{ width?: number, height?: number, quality?: number, fit?: string }} [opts]
 *   - width: 目标最大宽（px）。一般传 UI 容器宽 * devicePixelRatio
 *   - height: 目标最大高
 *   - quality: 1-100，默认 85
 *   - fit: scale-down(默认) / contain / cover / crop
 * @returns {string}
 */
export function optimizeImageUrl(url, opts = {}) {
  if (typeof url !== 'string' || url.length === 0) return url;
  // base64 / blob: / data: 不优化——CF 不接受、转出来也无意义
  if (!/^https?:\/\//i.test(url)) return url;

  const { enabled, base } = readSettings();
  if (!enabled || !base) return url;
  // 只对配置在 publicBaseURL 域下的 URL 套变换；其它来源（外部 CDN、用户
  // 自带 URL）不动，避免命中不存在的 cdn-cgi 路由
  if (!url.startsWith(base + '/') && url !== base) return url;

  let parsed;
  try {
    parsed = new URL(url);
  } catch {
    return url;
  }

  const params = ['format=auto', `fit=${opts.fit || 'scale-down'}`];
  if (Number.isFinite(opts.width) && opts.width > 0) {
    params.push(`width=${Math.round(opts.width)}`);
  }
  if (Number.isFinite(opts.height) && opts.height > 0) {
    params.push(`height=${Math.round(opts.height)}`);
  }
  // 默认 96——CF 文档默认 85，但对参考图这种带细节 / 文字 / 锐边的内容偏低；
  // AVIF 在 85-92 都会把高频细节抹平，看着像柔焦。96 几乎无损，仍能享受
  // AVIF/WebP 体积优势（比原 PNG 小 60-80%）
  const q = Number.isFinite(opts.quality) ? opts.quality : 96;
  params.push(`quality=${Math.round(q)}`);
  // sharpen：对缩略图加一个轻量 unsharp mask，专治下采样后的"模糊感"。
  // CF 接受 0-10，1.0 是常用的"轻微锐化"，再大就明显假。可被 opts.sharpen
  // 覆盖（lightbox 大图传 0 关掉）
  const s = Number.isFinite(opts.sharpen) ? opts.sharpen : 1;
  if (s > 0) params.push(`sharpen=${s}`);

  // path 在前面拼参数；CF 文档规定的格式：
  //   https://example.com/cdn-cgi/image/<options>/<path>
  // 注意 pathname 已经以 "/" 开头，options 段不带尾斜杠
  return `${parsed.origin}/cdn-cgi/image/${params.join(',')}${parsed.pathname}${parsed.search}`;
}

/**
 * 容器宽度按 devicePixelRatio 放大成"实际像素宽"，给 optimizeImageUrl 用。
 *
 * 经验值：
 *   - DPR 上限 3x：覆盖手机 / iPad，4x 设备少且会爆带宽
 *   - oversampling 2.0：`objectFit:cover` 会裁切一次，浏览器 lanczos 降采样
 *     需要充裕源像素才不糊。1.25 在多图小格子（cell≈85-110）下还是肉眼可
 *     辨柔焦，2.0 才彻底干净
 *
 * 例：240px 单图 + 2x DPR ⇒ 240 * 2 * 2 = 960 px 源；CF 配合 quality=96 +
 * sharpen=1，实际下行 ~80-150KB（原 PNG 通常 1-4MB）
 */
export function pixelWidth(cssWidth) {
  if (!Number.isFinite(cssWidth) || cssWidth <= 0) return cssWidth;
  const dpr =
    typeof window !== 'undefined' && window.devicePixelRatio
      ? Math.min(window.devicePixelRatio, 3)
      : 1;
  return Math.round(cssWidth * dpr * 2);
}
