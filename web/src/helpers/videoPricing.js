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

/**
 * 视频模型定价的前端镜像。
 *
 * 权威实现在后端 setting/billing_setting/video_pricing.go —— 这里只负责
 * 「配置编辑」和「定价页展示」两件事，改动价目表结构时两边必须同步。
 * 真正扣费一律以后端 ComputeVideoCost 为准，本文件的试算仅供预览。
 *
 * 两种数据形态：
 *   spec  —— 与后端 JSON 一致的配置对象，落库/展示用
 *   state —— 编辑器内部形态，分辨率档位是带稳定 id 的数组。编辑中必须以它
 *            为准：若直接编辑 spec 里的 map，清空分辨率名会让整条记录消失。
 */

let videoRowSeq = 0;
export const nextVideoRowId = () => `vrow_${(videoRowSeq += 1)}`;

/** 新建视频模型时的初始价目表，数值取 MiniMax-H3 官方价。 */
export const DEFAULT_VIDEO_MODEL_SPEC = {
  unit: 'second',
  default_resolution: '2K',
  output: { '768P': 0.08, '2K': 0.13 },
  input: {
    image: { per_image: 0.04, free_count: 0 },
    audio: { per_second: 0 },
    video: { per_second_by_output_resolution: { '768P': 0.08, '2K': 0.13 } },
  },
};

const toNumber = (value) => {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
};

const mapToRows = (map) =>
  Object.entries(map || {}).map(([resolution, price]) => ({
    id: nextVideoRowId(),
    resolution,
    price: toNumber(price),
  }));

const rowsToMap = (rows) => {
  const map = {};
  for (const row of rows || []) {
    const key = (row.resolution || '').trim();
    if (!key) continue;
    map[key] = toNumber(row.price);
  }
  return map;
};

/** spec → 编辑器状态。 */
export function specToVideoEditorState(spec) {
  const source = spec || DEFAULT_VIDEO_MODEL_SPEC;
  return {
    unit: source.unit || 'second',
    defaultResolution: source.default_resolution || '',
    outputRows: mapToRows(source.output),
    imagePerImage: toNumber(source.input?.image?.per_image),
    imageFreeCount: toNumber(source.input?.image?.free_count),
    audioPerSecond: toNumber(source.input?.audio?.per_second),
    videoRows: mapToRows(source.input?.video?.per_second_by_output_resolution),
  };
}

/** 编辑器状态 → spec，保存时调用。 */
export function videoEditorStateToSpec(state) {
  if (!state) return null;

  const input = {
    image: {
      per_image: toNumber(state.imagePerImage),
      free_count: toNumber(state.imageFreeCount),
    },
    audio: { per_second: toNumber(state.audioPerSecond) },
  };
  const videoMap = rowsToMap(state.videoRows);
  if (Object.keys(videoMap).length > 0) {
    input.video = { per_second_by_output_resolution: videoMap };
  }

  return {
    unit: state.unit || 'second',
    default_resolution: state.defaultResolution || '',
    output: rowsToMap(state.outputRows),
    input,
  };
}

/** 编辑器状态是否已经填了可用的输出定价。 */
export function isVideoPricingConfigured(state) {
  return Object.keys(rowsToMap(state?.outputRows)).length > 0;
}

/**
 * 把请求里的分辨率归一到价目表档位：精确匹配 → 大小写不敏感 → 默认档位。
 * 与后端 VideoPricing.ResolveResolution 同构。
 */
function resolveResolution(outputMap, resolution, defaultResolution) {
  const keys = Object.keys(outputMap || {});
  const target = (resolution || '').trim();
  if (target) {
    if (keys.includes(target)) return target;
    const fold = keys.find((k) => k.toLowerCase() === target.toLowerCase());
    if (fold) return fold;
  }
  if (defaultResolution && keys.includes(defaultResolution)) {
    return defaultResolution;
  }
  return '';
}

/**
 * 试算一单的价格明细，入参是 spec。
 * 算不出价时返回 error 而不是 0 —— 与后端一致，避免把「配错了」显示成「免费」。
 */
export function computeVideoCost(spec, usage) {
  const empty = { output: 0, image: 0, audio: 0, video: 0, total: 0 };
  if (!spec) return { ...empty, error: 'pricing not configured' };

  const resolution = resolveResolution(
    spec.output,
    usage?.resolution,
    spec.default_resolution,
  );
  if (!resolution) {
    return {
      ...empty,
      error: `no output tier for resolution "${usage?.resolution || ''}"`,
    };
  }

  const result = { ...empty, resolution };

  const outputSeconds = toNumber(usage?.outputSeconds);
  if (outputSeconds > 0) {
    result.output = toNumber(spec.output[resolution]) * outputSeconds;
  }

  const image = spec.input?.image;
  if (image) {
    const billable = toNumber(usage?.imageCount) - toNumber(image.free_count);
    if (billable > 0) {
      result.image = billable * toNumber(image.per_image);
    }
  }

  const audioSeconds = toNumber(usage?.audioSeconds);
  if (spec.input?.audio && audioSeconds > 0) {
    result.audio = toNumber(spec.input.audio.per_second) * audioSeconds;
  }

  const videoSeconds = toNumber(usage?.videoSeconds);
  const videoRates = spec.input?.video?.per_second_by_output_resolution;
  if (videoRates && videoSeconds > 0 && videoRates[resolution] !== undefined) {
    result.video = toNumber(videoRates[resolution]) * videoSeconds;
  }

  result.total = result.output + result.image + result.audio + result.video;
  return result;
}

/** 编辑器状态的试算，预览用。 */
export function computeVideoCostFromEditorState(state, usage) {
  return computeVideoCost(videoEditorStateToSpec(state), usage);
}

/**
 * 模型广场上的「每档分辨率每秒多少钱」。
 *
 * 视频模型的 model_price 只是哨兵基准价（$1），直接展示会误导用户，必须换成
 * 价目表里的真实单价并乘上分组倍率。
 *
 * displayPrice 由调用方传入（定价页那个 hook 提供）：它除了币种换算还负责
 * 「按充值价显示」等口径。自己用 getCurrencyConfig 换算的话，视频价格会和
 * 同一张卡片上的其它价格对不上。
 */
export function formatVideoOutputPrices(spec, groupRatio = 1, displayPrice) {
  const ratio = Number.isFinite(Number(groupRatio)) ? Number(groupRatio) : 1;
  const format =
    typeof displayPrice === 'function'
      ? displayPrice
      : (usd) => `$${Number(usd).toFixed(4)}`;

  return Object.entries(spec?.output || {}).map(([resolution, price]) => ({
    resolution,
    price: format(toNumber(price) * ratio),
  }));
}

/**
 * 分组价格表的「价格摘要」条目。
 *
 * 视频模型不能走 getModelPriceItems —— 它会落到按次兜底分支，显示成
 * 「模型价格 $1.00 / 次」，而那个 $1 只是哨兵基准价。这里按价目表列出
 * 每档分辨率的每秒单价与输入素材单价，全部乘上该分组的倍率。
 */
export function getVideoPriceItems(spec, groupRatio = 1, displayPrice, t) {
  const ratio = Number.isFinite(Number(groupRatio)) ? Number(groupRatio) : 1;
  const format =
    typeof displayPrice === 'function'
      ? displayPrice
      : (usd) => `$${Number(usd).toFixed(4)}`;
  const money = (usd) => format(toNumber(usd) * ratio);

  const items = Object.entries(spec?.output || {}).map(
    ([resolution, price]) => ({
      key: `output-${resolution}`,
      label: `${resolution} ${t('输出')}`,
      value: money(price),
      suffix: ` / ${t('秒')}`,
    }),
  );

  const videoRates = spec?.input?.video?.per_second_by_output_resolution;
  if (videoRates) {
    Object.entries(videoRates).forEach(([resolution, price]) => {
      items.push({
        key: `input-video-${resolution}`,
        label: `${resolution} ${t('输入视频')}`,
        value: money(price),
        suffix: ` / ${t('秒')}`,
      });
    });
  }

  const image = spec?.input?.image;
  if (image && toNumber(image.per_image) > 0) {
    const freeCount = toNumber(image.free_count);
    items.push({
      key: 'input-image',
      label: t('输入图片'),
      value: money(image.per_image),
      suffix:
        freeCount > 0
          ? ` / ${t('张')}${t('（前 {{count}} 张免费）', { count: freeCount })}`
          : ` / ${t('张')}`,
    });
  }

  const audio = spec?.input?.audio;
  if (audio && toNumber(audio.per_second) > 0) {
    items.push({
      key: 'input-audio',
      label: t('输入音频'),
      value: money(audio.per_second),
      suffix: ` / ${t('秒')}`,
    });
  }

  return items;
}

/** 模型列表里的一行价格摘要。 */
export function summarizeVideoPricing(state, t) {
  const parts = (state?.outputRows || [])
    .filter((r) => r.resolution)
    .map((r) => `${r.resolution} $${toNumber(r.price)}/${t('秒')}`);
  if (parts.length === 0) return t('未配置输出定价');

  const imagePrice = toNumber(state?.imagePerImage);
  if (imagePrice > 0) {
    parts.push(`${t('图片')} $${imagePrice}/${t('张')}`);
  }
  return parts.join(' · ');
}
