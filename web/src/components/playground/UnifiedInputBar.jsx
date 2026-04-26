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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Dropdown,
  ImagePreview,
  Input,
  InputNumber,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  ArrowLeftRight,
  ArrowUp,
  ArrowUpDown,
  Binary,
  Check,
  ChevronDown,
  Image as ImageIcon,
  Mic,
  Plus,
  Sparkles,
  Square,
  Video as VideoIcon,
  X,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { MODALITY } from '../../constants/playground.constants';
import {
  getModalityLongLabel,
  getModalityShortLabel,
} from '../../constants/modalityLabels';
import { getLobeHubIcon } from '../../helpers/render';
// 缩略图渲染走 cdn-cgi/image：URL entry 的 dataUrl 是远程原图（常 1-4MB），
// 直接当 56×56 缩略既浪费带宽，swap 时还会触发 re-fetch。optimizeImageUrl
// 内部对 blob:/data: 是 no-op，所以可以无脑套——只对 http(s) URL 生效
import { optimizeImageUrl, pixelWidth } from '../../helpers';
import { inferVendorIconKey, inferVendorMeta } from './vendorIcon';

// (model, group) 组合在 Select 里的编码方式，与之前 SettingsPanel 沿用一致
const MODEL_GROUP_SEP = '@@';
const encodeMG = (model, group) => `${model || ''}${MODEL_GROUP_SEP}${group || ''}`;
const decodeMG = (value) => {
  if (typeof value !== 'string' || !value) return { model: '', group: '' };
  const idx = value.indexOf(MODEL_GROUP_SEP);
  if (idx < 0) return { model: value, group: '' };
  return {
    model: value.slice(0, idx),
    group: value.slice(idx + MODEL_GROUP_SEP.length),
  };
};

// 倍率徽标颜色：和 helpers/render.jsx 的 renderRatio 保持语义一致
const ratioColor = (ratio) => {
  if (typeof ratio !== 'number') return 'grey';
  if (ratio > 5) return 'red';
  if (ratio > 3) return 'orange';
  if (ratio > 1) return 'blue';
  return 'green';
};

// 模型行内的「类型角标」：只在 image / video / audio / embedding / rerank
// 出现，提示用户这条不是常规对话模型。text / multimodal 是默认形态，
// 不画角标，避免视觉噪音。
//
// 仅图标，不带文字；颜色直接用 modality 主题色（CSS var），让角标和会话
// 视觉语言一致。
const MODALITY_BADGE_ICON = {
  [MODALITY.IMAGE]: { Icon: ImageIcon, color: 'violet' },
  [MODALITY.VIDEO]: { Icon: VideoIcon, color: 'orange' },
  [MODALITY.AUDIO]: { Icon: Mic, color: 'pink' },
  [MODALITY.EMBEDDING]: { Icon: Binary, color: 'cyan' },
  [MODALITY.RERANK]: { Icon: ArrowUpDown, color: 'amber' },
};

// 单条模型选项：左厂商 logo + 中间两行 + 右选中标记
//   行 1：模型名
//   行 2：分组 · 倍率徽章 · 类型徽章（仅图/视/音 等非常规对话）
// 厂商 logo 即使在 section header 上有一份，每行也保留——列表滚到中部
// header 离开视野时，能立刻识别模型出处。选中态在最右挂一个 Check 图标。
const ModelGroupRow = ({ entry, isSelected = false }) => {
  const { t } = useTranslation();
  const { model, group, ratio, modality } = entry;
  const badge = MODALITY_BADGE_ICON[modality];
  const badgeLabel = badge ? getModalityShortLabel(t, modality) : '';
  const iconKey = inferVendorIconKey(model);
  return (
    <div className='flex items-center w-full gap-2.5' style={{ minWidth: 0 }}>
      <span
        className='flex-shrink-0 inline-flex items-center justify-center'
        style={{ width: 22, height: 22 }}
      >
        {getLobeHubIcon(iconKey, 18)}
      </span>
      <div
        className='flex-1 min-w-0 flex flex-col'
        style={{ lineHeight: 1.25 }}
      >
        <Typography.Text
          className='text-sm'
          style={{
            color: 'var(--semi-color-text-0)',
            fontWeight: 500,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {model}
        </Typography.Text>
        <div
          className='flex items-center gap-1.5'
          style={{ marginTop: 2 }}
        >
          {/* 倍率 + 分组标识合并到一个 Tag：倍率在前（按数值上色，传达
              价格信号），分组用 raw 标识（premium / default 这种），不再
              展示后台填的「高级稳定分组」描述——标识更短、更稳定，
              i18n 也无需特殊处理。 */}
          {(typeof ratio === 'number' || group) && (
            <Tag
              color={typeof ratio === 'number' ? ratioColor(ratio) : 'grey'}
              size='small'
              shape='circle'
              style={{
                fontSize: 10,
                lineHeight: 1,
                padding: '0 6px',
                height: 16,
                maxWidth: 200,
              }}
            >
              <span
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 4,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {typeof ratio === 'number' && <span>{ratio}x</span>}
                {group && (
                  <span style={{ opacity: 0.85 }}>{group}</span>
                )}
              </span>
            </Tag>
          )}
          {badge && (
            // 类型徽章：图标 + i18n 文本（图片/视频/音频/向量/重排）。
            // 高度和左侧倍率徽章一致（16px）；prefixIcon 让 Semi Tag 自己
            // 把图标垂直居中，文本 size 10px 与倍率 Tag 一致。
            <Tag
              size='small'
              shape='circle'
              color={badge.color}
              prefixIcon={<badge.Icon size={10} />}
              style={{
                fontSize: 10,
                lineHeight: 1,
                padding: '0 6px',
                height: 16,
              }}
              aria-label={modality}
            >
              {badgeLabel}
            </Tag>
          )}
        </div>
      </div>
      {isSelected && (
        <span
          className='flex-shrink-0 inline-flex items-center justify-center'
          style={{
            width: 18,
            height: 18,
            color: 'var(--semi-color-primary)',
          }}
          aria-label='selected'
        >
          <Check size={16} strokeWidth={2.5} />
        </span>
      )}
    </div>
  );
};

// Model picker pill：放在输入框工具栏，触发态是个紧凑按钮，点击弹出
// 「分组 + 模型」选择面板。
//
// 实现方式刻意和导航多语言下拉（LanguageSelector）保持一致——都是
// Semi `Dropdown` + 自定义 render。这样下拉外观（圆角、描边、阴影、
// 选中/悬停色）天然和站内其它下拉对齐，不再和 Select 内置的 OptGroup /
// tick 占位 / 搜索框结构打架。搜索框我们自己用 Input 拼，过滤逻辑也自己
// 控制，行为反而更可预期。
const ModelPickerPill = ({
  modelEntries,
  inputs,
  currentEntry,
  onChange,
  disabled,
  t,
}) => {
  const [open, setOpen] = useState(false);
  const [keyword, setKeyword] = useState('');
  const searchRef = useRef(null);

  // 打开下拉时自动聚焦搜索框；关闭时清搜索词。
  // Dropdown 通过 portal 异步挂载内容，autoFocus 不一定及时生效——用一个
  // 短延时 + 显式 focus() 兜底，对齐 macOS 原生菜单的「输入即过滤」体验。
  useEffect(() => {
    if (!open) {
      setKeyword('');
      return;
    }
    const id = setTimeout(() => {
      try {
        searchRef.current?.focus?.();
      } catch {}
    }, 50);
    return () => clearTimeout(id);
  }, [open]);

  // 按厂商分组：同模型 + 不同分组的多条记录会落到同一个厂商 section 下；
  // 厂商内部按「模型名 a→z + 倍率升序」排，让最便宜的同名条目靠前。
  // 厂商之间的展示顺序：「Other」固定垫底，其余按厂商名首字母排序。
  //
  // 搜索匹配域：模型名 + 分组（key + 描述）+ modality 的 raw key + 短标签
  // + 长标签 + 厂商名。这样不论用户输入「image / 图片 / 图片生成 / openai」
  // 任意一个都能命中。
  const groupedEntries = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    const matched = kw
      ? modelEntries.filter((e) => {
          const modalityKey = e.modality || '';
          const modalityShort = getModalityShortLabel(t, modalityKey);
          const modalityLong = getModalityLongLabel(t, modalityKey);
          const vendorName = inferVendorMeta(e.model).name;
          const hay = [
            e.model,
            e.groupLabel,
            e.group,
            modalityKey,
            modalityShort,
            modalityLong,
            vendorName,
          ]
            .filter(Boolean)
            .join(' ')
            .toLowerCase();
          return hay.includes(kw);
        })
      : modelEntries;

    const buckets = new Map();
    matched.forEach((e) => {
      const meta = inferVendorMeta(e.model);
      const key = meta.name;
      if (!buckets.has(key)) {
        buckets.set(key, { name: meta.name, iconKey: meta.iconKey, entries: [] });
      }
      buckets.get(key).entries.push(e);
    });
    buckets.forEach((b) => {
      b.entries.sort((a, b2) => {
        if (a.model !== b2.model) return a.model.localeCompare(b2.model);
        const ra = typeof a.ratio === 'number' ? a.ratio : Infinity;
        const rb = typeof b2.ratio === 'number' ? b2.ratio : Infinity;
        return ra - rb;
      });
    });
    const ordered = Array.from(buckets.values()).sort((a, b) => {
      if (a.name === 'Other') return 1;
      if (b.name === 'Other') return -1;
      return a.name.localeCompare(b.name);
    });
    return ordered;
  }, [modelEntries, keyword]);

  const triggerLabel = currentEntry
    ? `${currentEntry.model}`
    : inputs?.model || t('选择模型');
  const triggerRatio =
    typeof currentEntry?.ratio === 'number' ? `${currentEntry.ratio}x` : '';
  const triggerIconKey = currentEntry
    ? inferVendorIconKey(currentEntry.model)
    : inputs?.model
      ? inferVendorIconKey(inputs.model)
      : null;
  const currentValue = inputs?.model
    ? encodeMG(inputs.model, inputs.group)
    : '';

  // 下拉面板：列表在上，搜索框在下（贴近输入条/工具栏，符合「往上展开」
  // 时手指/鼠标的近端操作习惯；同时整个面板高度感更稳定，搜索时列表不会
  // 抖动）。
  //
  // 用 Dropdown.Menu 作为容器（不是裸 div），背景/边框/阴影 / 暗色适配
  // 直接复用站内导航多语言选择器的同款 class 串——这样视觉与 LanguageSelector
  // 完全对齐，不再自己 inline 写颜色变量。
  const menuContent = (
    <Dropdown.Menu
      className='!bg-semi-color-bg-overlay !border-semi-color-border !shadow-lg !rounded-xl dark:!bg-zinc-800 dark:!border-zinc-700'
      style={{
        width: 320,
        maxHeight: 480,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        padding: 0,
      }}
    >
      <div
        style={{
          flex: 1,
          overflowY: 'auto',
          padding: '6px 6px',
        }}
      >
        {groupedEntries.length === 0 && (
          <div className='px-3 py-8 text-center'>
            <Typography.Text type='tertiary' className='text-xs'>
              {t('没有匹配的模型')}
            </Typography.Text>
          </div>
        )}
        {groupedEntries.map((g, gi) => {
          return (
            <div key={g.name} style={{ marginTop: gi === 0 ? 0 : 6 }}>
              {/* Section header：厂商名 + 数量（紧贴名字右侧）。
                  厂商 logo 仅在每条 item 左侧出现一次，header 这里不再
                  重复，避免视觉冗余。 */}
              <div
                className='flex items-center select-none'
                style={{
                  padding: '8px 12px 4px',
                  gap: 6,
                  color: 'var(--semi-color-text-2)',
                }}
              >
                <span
                  style={{
                    fontSize: 12,
                    fontWeight: 600,
                    letterSpacing: 0.2,
                    color: 'var(--semi-color-text-1)',
                  }}
                >
                  {g.name}
                </span>
                <span
                  style={{
                    fontSize: 11,
                    color: 'var(--semi-color-text-2)',
                  }}
                >
                  {g.entries.length}
                </span>
              </div>
              {g.entries.map((entry) => {
                const value = encodeMG(entry.model, entry.group);
                const isSelected = value === currentValue;
                return (
                  // 直接用 <button> 而不是 Dropdown.Item：Semi 的 Item
                  // 内部有 wrapper / icon-slot 等结构，hover 背景会被
                  // 内层 box 限制住、铺不满。改成扁平的 button 后，
                  // hover 在自己 100% 宽的盒子上着色，圆角也直接受控。
                  <button
                    key={value}
                    type='button'
                    onClick={() => {
                      onChange?.(entry.model, entry.group);
                      setOpen(false);
                    }}
                    className={`text-left transition-colors ${
                      isSelected
                        ? 'bg-semi-color-primary-light-default dark:bg-zinc-700 font-medium'
                        : 'hover:bg-semi-color-fill-1 dark:hover:bg-zinc-700/70'
                    }`}
                    style={{
                      display: 'block',
                      width: '100%',
                      padding: '8px 12px',
                      border: 'none',
                      background: isSelected
                        ? 'var(--semi-color-primary-light-default)'
                        : 'transparent',
                      cursor: 'pointer',
                      borderRadius: 12,
                      color: 'var(--semi-color-text-0)',
                      outline: 'none',
                      fontFamily: 'inherit',
                    }}
                    onMouseEnter={(e) => {
                      if (!isSelected) {
                        e.currentTarget.style.background =
                          'var(--semi-color-fill-1)';
                      }
                    }}
                    onMouseLeave={(e) => {
                      if (!isSelected) {
                        e.currentTarget.style.background = 'transparent';
                      }
                    }}
                  >
                    <ModelGroupRow entry={entry} isSelected={isSelected} />
                  </button>
                );
              })}
            </div>
          );
        })}
      </div>
      {/* 搜索框放在底部、紧贴输入条侧。整体高度做到 36px（default size），
          配合 6px 内边距，比之前的 small 视觉上明显「更扎实」。 */}
      <div
        style={{
          padding: 8,
          borderTop: '1px solid var(--semi-color-border)',
        }}
      >
        <Input
          ref={searchRef}
          size='default'
          value={keyword}
          onChange={setKeyword}
          placeholder={t('搜索模型、厂商、类型或分组')}
          showClear
        />
      </div>
    </Dropdown.Menu>
  );

  return (
    <Dropdown
      trigger='click'
      position='topLeft'
      visible={open}
      onVisibleChange={setOpen}
      // Dropdown 接受 render 直接渲染面板内容，行为和 LanguageSelector
      // 完全一致；点击外部关闭由 Semi 自己处理。
      render={menuContent}
    >
      <button
        type='button'
        disabled={disabled}
        onClick={(e) => e.stopPropagation()}
        // 复用 .playground-toolbar-btn：无边框、hover 出 fill-1 灰底；
        // 左 padding 收紧到 4，因为按钮左侧紧跟厂商 logo，标准 10px 显得太空
        className='playground-toolbar-btn'
        style={{ maxWidth: 300, paddingLeft: 4 }}
      >
        <span
          className='inline-flex items-center justify-center flex-shrink-0'
          style={{ width: 16, height: 16 }}
        >
          {triggerIconKey ? (
            getLobeHubIcon(triggerIconKey, 14)
          ) : (
            <Sparkles size={13} />
          )}
        </span>
        <span
          className='text-xs font-medium'
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            maxWidth: 160,
          }}
        >
          {triggerLabel}
        </span>
        {triggerRatio && (
          <Tag
            size='small'
            shape='circle'
            color={ratioColor(currentEntry?.ratio)}
            style={{ fontSize: 10, lineHeight: 1, padding: '0 6px', height: 16 }}
          >
            {triggerRatio}
          </Tag>
        )}
        <ChevronDown
          size={12}
          className='flex-shrink-0'
          style={{ opacity: 0.6 }}
        />
      </button>
    </Dropdown>
  );
};

// 输入框右侧悬浮的「参考图堆叠」+「上传按钮基座」。
//
// 关键设计：
// 1. 容器宽度恒定（按 4 张堆叠尺寸算）；超过 4 张的多余图叠在第 3 槽位
//    背后，hover 才能看见
// 2. hover 时容器自身的「热区盒子」会向上 / 向左扩展，正好覆盖整个抛物
//    线扇形——避免「鼠标走到扇形某张图时穿过空白触发 leave」造成闪动；
//    bottom-right 角保持不动，配合子元素 right:0+bottom:0 锚定，扩张
//    不会让任何元素位移
// 3. 上传按钮固定在 base 位置（z-index 最低，被堆叠图覆盖）；hover 扇开
//    后图片飞走、upload 按钮自然露出供再次上传。它本身不参与 hover 扇开
// 4. 扇开后悬浮某张图：该图 scale + 提到最高 z-index，左右相邻图沿弧线
//    各退让一档，给中心图留视觉留白
const THUMB_SIZE = 56;
const COLLAPSE_STEP = 6;
const MAX_COLLAPSED_OFFSETS = 3; // ≤4 张时错位 0/6/12/18
const ARC_ANGLE_START = -75;
const ARC_ANGLE_END = -165;

const ReferenceImageStack = ({
  images,
  onRemove,
  showUploadButton,
  onUpload,
  onRetryUpload,
  t,
}) => {
  const [expanded, setExpanded] = useState(false);
  const [hoveredIdx, setHoveredIdx] = useState(null);
  const [previewIdx, setPreviewIdx] = useState(-1);
  const previewUrls = useMemo(() => images.map((x) => x.dataUrl), [images]);

  // 容器自身用 pointer-events: auto，配合 expanded 时整体扩张到能罩住整个
  // 抛物线扇形——这样无论鼠标在「上传按钮 / 图片 / 它们之间的空白」哪一处，
  // 都还在容器 box 内，不会触发 mouseLeave。再叠 120ms 离开 dwell 容错
  // 「贴边滑动一瞬间穿出」之类的边缘情况。
  const leaveTimerRef = useRef(null);
  const handleContainerEnter = () => {
    if (leaveTimerRef.current) {
      clearTimeout(leaveTimerRef.current);
      leaveTimerRef.current = null;
    }
    setExpanded(true);
  };
  const handleContainerLeave = () => {
    if (leaveTimerRef.current) clearTimeout(leaveTimerRef.current);
    leaveTimerRef.current = setTimeout(() => {
      setExpanded(false);
      setHoveredIdx(null);
      leaveTimerRef.current = null;
    }, 120);
  };
  useEffect(() => {
    return () => {
      if (leaveTimerRef.current) clearTimeout(leaveTimerRef.current);
    };
  }, []);

  const n = images?.length || 0;
  const stackWidth =
    THUMB_SIZE +
    COLLAPSE_STEP * Math.min(MAX_COLLAPSED_OFFSETS, Math.max(n - 1, 0));
  const arcRadius = Math.min(72 + n * 4, 120);

  // 0 张且不展示上传按钮 → 完全不渲染
  if (n === 0 && !showUploadButton) return null;

  // 抛物线展开 + 邻居挤开 + 「径向外推」让 hovered 浮出弧线之外。
  //
  // 数量多（>4 张）时弧线密集，邻居挤开角度（22°）已超过相邻基础间距，
  // 仍然可能视觉重叠；为了让 hovered 永远「能完整看到、能点删除按钮」，
  // 引入两个机制：
  //   1) 沿径向往外推 12% R——把 hovered 抬出原弧线，从堆里浮出来
  //   2) zIndex 拉到最高，无论被谁覆盖都贴在最上层
  // 其它图维持基线弧线位置 + 紧邻 22° 挤开，给 hovered 让点呼吸空间。
  const expandedTransform = (i) => {
    const t01 = n <= 1 ? 0 : i / (n - 1);
    let angle = ARC_ANGLE_START + (ARC_ANGLE_END - ARC_ANGLE_START) * t01;
    if (hoveredIdx != null && hoveredIdx !== i) {
      const d = i - hoveredIdx;
      if (d === -1) angle += 22; // 紧邻右侧，往右挪
      else if (d === 1) angle -= 22; // 紧邻左侧，往左挪
    }
    const a = (angle * Math.PI) / 180;
    const isHovered = hoveredIdx === i;
    // hovered 沿径向外推 12%——视觉上「跳出」原弧线，不被邻居挡住
    const radius = isHovered ? arcRadius * 1.12 : arcRadius;
    return {
      dx: radius * Math.cos(a),
      dy: radius * Math.sin(a),
      rot: (angle + 105) / 3,
    };
  };

  const collapsedTransform = (i) => ({
    dx: -Math.min(i, MAX_COLLAPSED_OFFSETS) * COLLAPSE_STEP,
    dy: 0,
    rot: i % 2 === 0 ? -4 : 4,
  });

  // 容器 expanded 时向上 / 向左扩展，bottom-right 角不动；子元素都用
  // right:0 + bottom:0 锚定，所以扩张不会让任何元素位移。容器自带
  // pointer-events: auto，鼠标只要在它的 box 内（含元素之间的空白）就
  // 不会触发 leave。整体下移到 top: 18，整组观感更舒展不那么贴顶。
  const TOP_OFFSET = 18;
  const containerStyle = {
    position: 'absolute',
    right: 12,
    top: expanded ? TOP_OFFSET - arcRadius : TOP_OFFSET,
    width: expanded ? stackWidth + arcRadius : stackWidth,
    height: expanded ? THUMB_SIZE + arcRadius : THUMB_SIZE,
    zIndex: 5,
    transition:
      'top 220ms cubic-bezier(0.22, 1, 0.36, 1), width 220ms cubic-bezier(0.22, 1, 0.36, 1), height 220ms cubic-bezier(0.22, 1, 0.36, 1)',
    pointerEvents: 'auto',
  };

  return (
    <div
      style={containerStyle}
      onMouseEnter={handleContainerEnter}
      onMouseLeave={handleContainerLeave}
    >
      {/* 上传按钮：固定在 base 位置（z-index 0），微倾斜与堆叠图统一 */}
      {showUploadButton && (
        <button
          type='button'
          onClick={(e) => {
            e.stopPropagation();
            onUpload?.();
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.background = 'var(--semi-color-fill-1)';
            e.currentTarget.style.color = 'var(--semi-color-text-1)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.background = 'var(--semi-color-fill-0)';
            e.currentTarget.style.color = 'var(--semi-color-text-2)';
          }}
          aria-label={t('上传参考图')}
          style={{
            position: 'absolute',
            right: 0,
            bottom: 0,
            width: THUMB_SIZE,
            height: THUMB_SIZE,
            borderRadius: 10,
            border: '1.5px dashed var(--semi-color-border)',
            background: 'var(--semi-color-fill-0)',
            color: 'var(--semi-color-text-2)',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 0,
            zIndex: 0,
            transform: 'rotate(-3deg)',
            transformOrigin: 'center center',
            transition:
              'background-color 150ms, color 150ms, transform 200ms ease',
          }}
        >
          <Plus size={20} />
        </button>
      )}

      {images.map((img, i) => {
        const { dx, dy, rot } = expanded
          ? expandedTransform(i)
          : collapsedTransform(i);
        const isHovered = hoveredIdx === i;
        // 数量多时邻居挤开仍可能重叠，所以 hovered：
        //   - scale 1.2 放大更明显
        //   - zIndex 拉到 999 永远在最上层（保证删除按钮可点）
        //   - 沿径向外推 12%（在 expandedTransform 里实现）
        // 这三件事一起，确保任何数量下 hovered 都「完整可见、可操作」
        const scale = isHovered ? 1.2 : 1;
        return (
          <div
            key={img.key}
            className='group absolute cursor-zoom-in'
            style={{
              right: 0,
              bottom: 0,
              width: THUMB_SIZE,
              height: THUMB_SIZE,
              transform: `translate(${dx}px, ${dy}px) rotate(${rot}deg) scale(${scale})`,
              transformOrigin: 'center center',
              transition:
                'transform 220ms cubic-bezier(0.22, 1, 0.36, 1), box-shadow 200ms ease',
              zIndex: isHovered ? 999 : 100 - i,
              boxShadow: isHovered
                ? '0 8px 22px rgba(0,0,0,0.28)'
                : '0 2px 6px rgba(0,0,0,0.18)',
              background: 'var(--semi-color-fill-0)',
              borderRadius: 10,
            }}
            onMouseEnter={() => setHoveredIdx(i)}
            // 单图 leave 不清 hoveredIdx，避免相邻穿过时邻居复位再起跳
            // 抖动；统一交由容器 leave 的 dwell timer 处理
            onClick={(e) => {
              e.stopPropagation();
              setPreviewIdx(i);
            }}
          >
            <img
              src={optimizeImageUrl(img.dataUrl, {
                width: pixelWidth(THUMB_SIZE),
              })}
              alt=''
              draggable={false}
              style={{
                display: 'block',
                width: '100%',
                height: '100%',
                objectFit: 'cover',
                borderRadius: 10,
                filter: img.uploading
                  ? 'grayscale(0.6) brightness(0.85)'
                  : 'none',
                transition: 'filter 200ms',
                outline: img.failed
                  ? '1.5px solid var(--semi-color-danger)'
                  : 'none',
                outlineOffset: -1,
              }}
            />
            <UploadStatusBadge
              uploading={img.uploading}
              failed={img.failed}
              uploadError={img.uploadError}
              onRetry={() => img.file && onRetryUpload?.(img.file)}
              t={t}
            />
            {/* 悬浮显示「图片 N」角标——和 @imageN 引用语法对齐，
                让用户知道当前图片在 prompt 里如何称呼。pointer-events:none
                以免覆盖到 hover 区域，触发抖动。 */}
            <span
              className='opacity-0 group-hover:opacity-100 transition-opacity'
              style={{
                position: 'absolute',
                left: 4,
                top: 4,
                padding: '1px 5px',
                borderRadius: 6,
                background: 'rgba(0,0,0,0.55)',
                color: '#fff',
                fontSize: 10,
                fontWeight: 600,
                letterSpacing: 0.3,
                pointerEvents: 'none',
                backdropFilter: 'blur(4px)',
              }}
            >
              {t('图片{{n}}', { n: i + 1 })}
            </span>
            <button
              type='button'
              className='opacity-0 group-hover:opacity-100 transition-opacity'
              onClick={(e) => {
                e.stopPropagation();
                onRemove?.(img.key);
              }}
              aria-label={t('删除参考图')}
              style={{
                position: 'absolute',
                top: -6,
                right: -6,
                width: 18,
                height: 18,
                borderRadius: '50%',
                border: 'none',
                background: 'rgba(0,0,0,0.7)',
                color: '#fff',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 0,
                lineHeight: 0,
              }}
            >
              <X size={11} />
            </button>
          </div>
        );
      })}

      {previewIdx >= 0 && (
        <ImagePreview
          src={previewUrls}
          visible={previewIdx >= 0}
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

// 模式选择按钮：放在模型选择器左侧。三种模式：
//   - smart  智能模式（默认）：模型列表显示全部
//   - image  图片生成     ：模型列表只显示 image 类型
//   - video  视频生成     ：模型列表只显示 video 类型
// 非 smart 时：悬浮整个按钮就会把左侧图标切成 X 关闭图标（不必悬到图标上），
// 同时按钮本身带一抹半透明色背景以暗示「你处在特殊模式」。
// 点击 X 图标 → 恢复 smart；点击按钮其它区域 → 打开下拉换模式。
const MODE_DEFS = [
  { key: 'smart', icon: Sparkles, modality: null },
  { key: 'image', icon: ImageIcon, modality: MODALITY.IMAGE },
  { key: 'video', icon: VideoIcon, modality: MODALITY.VIDEO },
];
const getModeLabel = (t, key) => {
  switch (key) {
    case 'image':
      return t('图片生成');
    case 'video':
      return t('视频生成');
    default:
      return t('智能模式');
  }
};
// 非 smart 模式下按钮的着色背景 + 文字色，提示「特殊模式」
const MODE_TINT = {
  image: {
    bg: 'rgba(139, 92, 246, 0.14)',
    bgHover: 'rgba(139, 92, 246, 0.22)',
    color: 'var(--semi-color-violet-6, rgb(124, 58, 237))',
  },
  video: {
    bg: 'rgba(249, 115, 22, 0.14)',
    bgHover: 'rgba(249, 115, 22, 0.22)',
    color: 'var(--semi-color-orange-6, rgb(234, 88, 12))',
  },
};

const ModeSelector = ({ mode, onModeChange, disabled, t }) => {
  const [open, setOpen] = useState(false);
  const [btnHover, setBtnHover] = useState(false);
  const current = MODE_DEFS.find((m) => m.key === mode) || MODE_DEFS[0];
  const Icon = current.icon;
  const isCancellable = mode !== 'smart';
  const ShowIcon = isCancellable && btnHover ? X : Icon;
  const tint = MODE_TINT[mode];

  const menu = (
    <Dropdown.Menu
      className='!bg-semi-color-bg-overlay !border-semi-color-border !shadow-lg !rounded-xl dark:!bg-zinc-800 dark:!border-zinc-700'
      style={{
        width: 180,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        padding: 6,
      }}
    >
      {MODE_DEFS.map((m) => {
        const ItemIcon = m.icon;
        const selected = m.key === mode;
        return (
          <button
            key={m.key}
            type='button'
            onClick={() => {
              setOpen(false);
              if (m.key !== mode) onModeChange?.(m.key);
            }}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              width: '100%',
              padding: '8px 10px',
              border: 'none',
              background: selected
                ? 'var(--semi-color-primary-light-default)'
                : 'transparent',
              cursor: 'pointer',
              borderRadius: 10,
              color: 'var(--semi-color-text-0)',
              fontFamily: 'inherit',
              fontSize: 13,
              fontWeight: selected ? 500 : 400,
              textAlign: 'left',
            }}
            onMouseEnter={(e) => {
              if (!selected)
                e.currentTarget.style.background = 'var(--semi-color-fill-1)';
            }}
            onMouseLeave={(e) => {
              if (!selected) e.currentTarget.style.background = 'transparent';
            }}
          >
            <span
              className='inline-flex items-center justify-center'
              style={{
                width: 18,
                height: 18,
                color: 'var(--semi-color-text-1)',
              }}
            >
              <ItemIcon size={16} />
            </span>
            <span>{getModeLabel(t, m.key)}</span>
          </button>
        );
      })}
    </Dropdown.Menu>
  );

  return (
    <Dropdown
      trigger='click'
      position='topLeft'
      visible={open}
      onVisibleChange={setOpen}
      render={menu}
    >
      <button
        type='button'
        disabled={disabled}
        className='playground-toolbar-btn'
        aria-label={getModeLabel(t, current.key)}
        title={getModeLabel(t, current.key)}
        onMouseEnter={() => setBtnHover(true)}
        onMouseLeave={() => setBtnHover(false)}
        // 触发按钮只展示图标（紧凑），文字仅放在下拉条目里。
        // smart 用默认 hover 灰底；非 smart 用着色 tint，hover 加深一档。
        style={{
          width: 32,
          padding: 0,
          justifyContent: 'center',
          ...(tint
            ? {
                background: btnHover ? tint.bgHover : tint.bg,
                color: tint.color,
              }
            : null),
        }}
      >
        <span
          className='inline-flex items-center justify-center'
          // 图标区单独可点：显示 X（非 smart + hover）时点 X 一键回智能模式
          onClick={(e) => {
            if (isCancellable) {
              e.stopPropagation();
              onModeChange?.('smart');
            }
          }}
          style={{
            width: 18,
            height: 18,
            color:
              isCancellable && btnHover
                ? 'var(--semi-color-danger)'
                : 'inherit',
            cursor: isCancellable ? 'pointer' : 'inherit',
            transition: 'color 150ms ease',
          }}
        >
          <ShowIcon size={16} />
        </span>
      </button>
    </Dropdown>
  );
};

// 工具栏只暴露「最常调」的几类参数：分辨率 / 尺寸 / 质量 / 比例。其它
// 参数（背景、输出格式、内容审核、person_generation 等）只在右栏完整面板
// 里出现，避免工具栏被堆满。匹配用 normKey（小写 + 去空格/_/-）跨模型
// 命中：例如 "Aspect Ratio"、"aspect_ratio"、"aspectRatio" 都归一到
// "aspectratio"。
const normKey = (k) => String(k || '').toLowerCase().replace(/[\s_-]+/g, '');
const TOOLBAR_PARAM_KEYS = new Set([
  'size',
  'imagesize',
  'resolution',
  'dimensions',
  'quality',
  'imagequality',
  'aspectratio',
  'aspect',
  'ratio',
  'imageratio',
  'videoratio',
  // 视频模型：时长是高频参数，直接放工具栏，跟图片模型的 size/quality
  // 同等地位，省去用户进右栏改值
  'duration',
  'videoduration',
  'length',
]);
// 工具栏支持两类参数：
//   1) enum 字段（size / quality / aspect_ratio …）→ 选择型下拉
//   2) 有界数字字段（duration: int 0-16 …）→ 同样下拉，但内容是
//      「特殊值快捷按钮（来自 enumLabels）+ 数字输入」
// 第二类是为视频时长这种「整段范围合法 + 0 表示自动」场景设计的。
const isBoundedNumberParam = (def) =>
  (def?.type === 'integer' || def?.type === 'number') &&
  typeof def?.minimum === 'number' &&
  typeof def?.maximum === 'number';
const isToolbarParam = (key, def) => {
  if (!TOOLBAR_PARAM_KEYS.has(normKey(key))) return false;
  return Array.isArray(def?.enum) || isBoundedNumberParam(def);
};

// 把 schema 的 enumLabels 扩展字段标准化成「raw 值 → 显示文案」的查表函数。
// 约定：enumLabels 的 key 是字符串（JSON 限制），但 raw 值可能是数字/布尔，
// 比对时统一 String() 化。未声明 label 时回退原值的字符串形式。
const makeLabelFor = (def) => {
  const map =
    def?.enumLabels && typeof def.enumLabels === 'object' ? def.enumLabels : null;
  return (raw) => {
    if (raw === undefined || raw === null) return null;
    if (map && Object.prototype.hasOwnProperty.call(map, String(raw))) {
      return String(map[String(raw)]);
    }
    return String(raw);
  };
};

// 单个 schema 参数的快捷下拉。schema.title 形如「分辨率 (size)」或
// 「宽高比（aspectRatio）」（gemini 用全角括号），统一去掉末尾括号部分，
// 只保留前面的中文/英文标题，便于在 trigger 和下拉头部展示。
const getSchemaShortTitle = (def, key) => {
  const raw = def?.title || key || '';
  // 末尾的 ( ... ) 或 （ ... ） 整段裁掉，前面允许有空格
  return raw.replace(/\s*[(（][^()（）]*[)）]\s*$/u, '').trim() || raw;
};

const SchemaParamSelector = ({ paramKey, def, value, onChange, disabled, t }) => {
  const [open, setOpen] = useState(false);
  const enumValues = Array.isArray(def?.enum) ? def.enum : null;
  const isRange = !enumValues && isBoundedNumberParam(def);
  if (!enumValues && !isRange) return null;

  // trigger 与下拉头部都用清洗过的短标题——把 schema title 末尾的
  // 「(字段名)」/「（字段名）」整段裁掉，只展示中文/英文文字标题，
  // 与 gpt-image 这类 title 本身就没带字段名的 schema 视觉对齐。
  const title = getSchemaShortTitle(def, paramKey);
  const labelFor = makeLabelFor(def);
  const currentRaw = value !== undefined ? value : def?.default;
  const currentLabel =
    currentRaw === undefined || currentRaw === null
      ? t('未设置')
      : labelFor(currentRaw);

  // range 模式下，enumLabels 的 entries 渲染成「特殊值快捷按钮」（如
  // 「自动」对应 0）；下面再放一个 InputNumber 让用户输入区间内任意数。
  // 选中态比对统一走 String()，规避 0 ≠ "0" / true ≠ "true" 的坑。
  const specialEntries =
    isRange && def?.enumLabels && typeof def.enumLabels === 'object'
      ? Object.entries(def.enumLabels)
      : [];

  const optionButton = (rawVal, label) => {
    const valStr = String(rawVal);
    const selected = String(currentRaw) === valStr;
    return (
      <button
        key={valStr}
        type='button'
        onClick={() => {
          // enum 字段的值类型五花八门（int / float / string / bool），
          // 必须按 schema 原始类型回写——直接传 rawVal 即可。
          onChange?.(rawVal);
          setOpen(false);
        }}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          width: '100%',
          padding: '6px 10px',
          border: 'none',
          background: selected
            ? 'var(--semi-color-primary-light-default)'
            : 'transparent',
          cursor: 'pointer',
          borderRadius: 8,
          color: 'var(--semi-color-text-0)',
          fontFamily: 'inherit',
          fontSize: 13,
          fontWeight: selected ? 500 : 400,
          textAlign: 'left',
        }}
        onMouseEnter={(e) => {
          if (!selected)
            e.currentTarget.style.background = 'var(--semi-color-fill-1)';
        }}
        onMouseLeave={(e) => {
          if (!selected) e.currentTarget.style.background = 'transparent';
        }}
      >
        <span style={{ flex: 1 }}>{label}</span>
        {selected && (
          <Check
            size={14}
            style={{
              color: 'var(--semi-color-primary)',
              flexShrink: 0,
            }}
          />
        )}
      </button>
    );
  };

  const menu = (
    <Dropdown.Menu
      className='!bg-semi-color-bg-overlay !border-semi-color-border !shadow-lg !rounded-xl dark:!bg-zinc-800 dark:!border-zinc-700'
      style={{
        minWidth: 180,
        maxHeight: 320,
        overflowY: 'auto',
        display: 'flex',
        flexDirection: 'column',
        padding: 6,
      }}
    >
      <div
        style={{
          padding: '4px 10px 6px',
          fontSize: 11,
          fontWeight: 600,
          letterSpacing: 0.3,
          color: 'var(--semi-color-text-2)',
        }}
      >
        {title}
      </div>
      {enumValues
        ? enumValues.map((opt) => optionButton(opt, labelFor(opt)))
        : (
          <>
            {specialEntries.map(([raw, label]) => {
              // enumLabels 的 key 是字符串，按 schema type 还原成数字
              const numeric = Number(raw);
              const rawVal = Number.isFinite(numeric) ? numeric : raw;
              return optionButton(rawVal, label);
            })}
            {specialEntries.length > 0 && (
              <div
                style={{
                  height: 1,
                  background: 'var(--semi-color-border)',
                  margin: '4px 6px',
                }}
              />
            )}
            <div
              style={{
                padding: '6px 10px 4px',
                fontSize: 11,
                color: 'var(--semi-color-text-2)',
              }}
            >
              {t('自定义（{{min}}-{{max}}）', {
                min: def.minimum,
                max: def.maximum,
              })}
            </div>
            <div style={{ padding: '0 10px 8px' }}>
              <InputNumber
                value={
                  typeof currentRaw === 'number' ? currentRaw : def?.default
                }
                min={def.minimum}
                max={def.maximum}
                step={def.type === 'integer' ? 1 : def.step || 0.1}
                precision={def.type === 'integer' ? 0 : undefined}
                size='small'
                style={{ width: '100%' }}
                onChange={(v) => {
                  // InputNumber 在空字符串时回 ''；空值不写回，保留默认
                  if (v === '' || v === null || v === undefined) return;
                  const num = Number(v);
                  if (!Number.isFinite(num)) return;
                  onChange?.(def.type === 'integer' ? Math.round(num) : num);
                }}
              />
            </div>
          </>
        )}
    </Dropdown.Menu>
  );

  return (
    <Dropdown
      trigger='click'
      position='topLeft'
      visible={open}
      onVisibleChange={setOpen}
      render={menu}
    >
      <button
        type='button'
        disabled={disabled}
        className='playground-toolbar-btn'
        title={title}
      >
        <span
          className='text-xs'
          style={{
            color: 'var(--semi-color-text-2)',
            fontWeight: 500,
          }}
        >
          {title}
        </span>
        <span
          className='text-xs'
          style={{
            color: 'var(--semi-color-text-0)',
            fontWeight: 600,
            maxWidth: 120,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {currentLabel}
        </span>
      </button>
    </Dropdown>
  );
};

// 视频模型「全能参考 / 首尾帧」模式选择器。仅在 schema 同时声明了 reference
// 槽和 first_frame/last_frame 槽时由父层启用（videoInputModeAvailable=true）。
// 视觉上做成模型选择器右侧的小 pill 下拉：触发器只显示当前模式的图标 + 简短
// 文字，下拉里两条选项各带描述。
const VIDEO_INPUT_MODES = [
  { key: 'omni', icon: ImageIcon },
  { key: 'first_last', icon: ArrowLeftRight },
];
const getVideoInputModeLabel = (t, key) => {
  switch (key) {
    case 'first_last':
      return t('首尾帧');
    case 'omni':
    default:
      return t('全能参考');
  }
};
const getVideoInputModeDesc = (t, key) => {
  switch (key) {
    case 'first_last':
      return t('上传首帧 + 末帧锁定起止画面');
    case 'omni':
    default:
      return t('上传 1-9 张参考图，prompt 用 @imageN 引用');
  }
};

const VideoInputModeSelector = ({ mode, onModeChange, disabled, t }) => {
  const [open, setOpen] = useState(false);
  const current =
    VIDEO_INPUT_MODES.find((m) => m.key === mode) || VIDEO_INPUT_MODES[0];
  const Icon = current.icon;

  const menu = (
    <Dropdown.Menu
      className='!bg-semi-color-bg-overlay !border-semi-color-border !shadow-lg !rounded-xl dark:!bg-zinc-800 dark:!border-zinc-700'
      style={{
        width: 240,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        padding: 6,
      }}
    >
      {VIDEO_INPUT_MODES.map((m) => {
        const ItemIcon = m.icon;
        const selected = m.key === mode;
        return (
          <button
            key={m.key}
            type='button'
            onClick={() => {
              setOpen(false);
              if (m.key !== mode) onModeChange?.(m.key);
            }}
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 10,
              width: '100%',
              padding: '8px 10px',
              border: 'none',
              background: selected
                ? 'var(--semi-color-primary-light-default)'
                : 'transparent',
              cursor: 'pointer',
              borderRadius: 10,
              color: 'var(--semi-color-text-0)',
              fontFamily: 'inherit',
              textAlign: 'left',
            }}
            onMouseEnter={(e) => {
              if (!selected)
                e.currentTarget.style.background = 'var(--semi-color-fill-1)';
            }}
            onMouseLeave={(e) => {
              if (!selected) e.currentTarget.style.background = 'transparent';
            }}
          >
            <span
              className='inline-flex items-center justify-center'
              style={{
                width: 18,
                height: 18,
                color: 'var(--semi-color-text-1)',
                marginTop: 1,
              }}
            >
              <ItemIcon size={16} />
            </span>
            <div className='flex-1 min-w-0' style={{ lineHeight: 1.3 }}>
              <div
                style={{
                  fontSize: 13,
                  fontWeight: selected ? 500 : 400,
                  color: 'var(--semi-color-text-0)',
                }}
              >
                {getVideoInputModeLabel(t, m.key)}
              </div>
              <div
                style={{
                  fontSize: 11,
                  marginTop: 2,
                  color: 'var(--semi-color-text-2)',
                }}
              >
                {getVideoInputModeDesc(t, m.key)}
              </div>
            </div>
          </button>
        );
      })}
    </Dropdown.Menu>
  );

  return (
    <Dropdown
      trigger='click'
      position='topLeft'
      visible={open}
      onVisibleChange={setOpen}
      render={menu}
    >
      <button
        type='button'
        disabled={disabled}
        className='playground-toolbar-btn'
        title={getVideoInputModeLabel(t, current.key)}
        // 覆盖 .playground-toolbar-btn 默认 gap:6 + padding:0 10px——
        // 这里图标和文字之间用更紧凑的 4px，左右内边距也收一档
        style={{ height: 32, padding: '0 8px', gap: 4 }}
      >
        <Icon size={14} style={{ display: 'block' }} />
        <span
          className='text-xs'
          style={{ color: 'var(--semi-color-text-0)', fontWeight: 500 }}
        >
          {getVideoInputModeLabel(t, current.key)}
        </span>
        <ChevronDown
          size={12}
          style={{ color: 'var(--semi-color-text-2)' }}
        />
      </button>
    </Dropdown>
  );
};

// 视频「首尾帧」模式专用：两个独立 56×56 上传位 + 中间 swap 按钮。
// 设计要点（与 ReferenceImageStack 区别）：
//  - 每个 slot 独立、无堆叠扇出；空槽显示 + 上传，已上传显示图 + X 移除
//  - 中间 swap 按钮始终在场，点一下 first / last 对调，便于纠错
//  - 上传后整个 slot 区域变成图片（而不是再保留 + 按钮）
//  - 容器位置和 ReferenceImageStack 对齐（top-right），但宽度更大
const SLOT_SIZE = 56;
const SWAP_WIDTH = 24;
const SLOT_GAP = 6;
const FIRST_LAST_TOTAL_WIDTH = SLOT_SIZE * 2 + SWAP_WIDTH + SLOT_GAP * 2;

// 上传状态覆盖层：pending 显小 spinner，failed 显红色徽章 + 重试。两种
// 状态都不阻塞缩略图本体的展示——失败时仍能预览原图，方便用户判断是不是
// 网络抖动需要重试。
//
// 不放在缩略图 overflow:hidden 容器内：spinner / 重试图标位于角落，需要
// 露出在缩略图外，避免被裁切。
const UploadStatusBadge = ({
  uploading,
  failed,
  uploadError,
  onRetry,
  t,
}) => {
  if (!uploading && !failed) return null;
  if (uploading) {
    return (
      <div
        aria-label={t('上传中…')}
        title={t('上传中…')}
        style={{
          position: 'absolute',
          left: 4,
          bottom: 4,
          width: 16,
          height: 16,
          borderRadius: '50%',
          background: 'rgba(0,0,0,0.55)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 2,
          pointerEvents: 'none',
        }}
      >
        <span
          style={{
            width: 9,
            height: 9,
            borderRadius: '50%',
            border: '1.5px solid rgba(255,255,255,0.85)',
            borderTopColor: 'transparent',
            animation: 'spin 0.7s linear infinite',
          }}
        />
      </div>
    );
  }
  // failed
  return (
    <button
      type='button'
      onClick={(e) => {
        e.stopPropagation();
        onRetry?.();
      }}
      aria-label={t('上传失败，点击重试')}
      title={uploadError ? t('上传失败：') + uploadError : t('上传失败，点击重试')}
      style={{
        position: 'absolute',
        left: 4,
        bottom: 4,
        height: 16,
        padding: '0 6px',
        borderRadius: 8,
        border: 'none',
        background: 'var(--semi-color-danger)',
        color: '#fff',
        fontSize: 10,
        fontWeight: 600,
        letterSpacing: 0.3,
        cursor: 'pointer',
        display: 'inline-flex',
        alignItems: 'center',
        zIndex: 2,
      }}
    >
      {t('重试')}
    </button>
  );
};

const FrameSlot = ({ image, onClick, onRemove, onRetryUpload, label, t }) => {
  const filled = !!image;
  const [previewOpen, setPreviewOpen] = useState(false);
  return (
    // 把 .group hover 触发挂在最外层；overflow:hidden 留给内层"图片容器"
    // 即可——X 删除按钮放在外层（不被裁切），它通过 group-hover 仍能响应
    // 鼠标在整个 slot 上的 hover。
    <div
      className='group'
      style={{
        position: 'relative',
        width: SLOT_SIZE,
        height: SLOT_SIZE,
      }}
    >
      {filled ? (
        <>
          <div
            className='cursor-zoom-in'
            onClick={() => setPreviewOpen(true)}
            style={{
              width: '100%',
              height: '100%',
              borderRadius: 10,
              overflow: 'hidden',
              background: 'var(--semi-color-fill-0)',
              boxShadow: '0 2px 6px rgba(0,0,0,0.18)',
              // 失败时叠红色描边，让"这张没成功传上去"的状态在远处也能看到
              border: image.failed ? '1.5px solid var(--semi-color-danger)' : 'none',
              position: 'relative',
            }}
            title={label}
          >
            <img
              src={optimizeImageUrl(image.dataUrl, {
                width: pixelWidth(SLOT_SIZE),
              })}
              alt={label}
              draggable={false}
              style={{
                display: 'block',
                width: '100%',
                height: '100%',
                objectFit: 'cover',
                // 上传中给一点灰度，让用户视觉上感知"这张还没准备好"
                filter: image.uploading ? 'grayscale(0.6) brightness(0.85)' : 'none',
                transition: 'filter 200ms',
              }}
            />
            {/* 角标：首/末（用户可以一眼区分两个 slot） */}
            <span
              style={{
                position: 'absolute',
                left: 4,
                top: 4,
                padding: '1px 5px',
                borderRadius: 6,
                background: 'rgba(0,0,0,0.55)',
                color: '#fff',
                fontSize: 10,
                fontWeight: 600,
                letterSpacing: 0.3,
                backdropFilter: 'blur(4px)',
              }}
            >
              {label}
            </span>
            <UploadStatusBadge
              uploading={image.uploading}
              failed={image.failed}
              uploadError={image.uploadError}
              onRetry={() => image.file && onRetryUpload?.(image.file)}
              t={t}
            />
          </div>
          {/* X 移到 overflow:hidden 容器之外，避免 top:-6 / right:-6 那
              一截被裁掉。仍贴在 .group 内，靠 group-hover 控制显隐。 */}
          <button
            type='button'
            className='opacity-0 group-hover:opacity-100 transition-opacity'
            onClick={(e) => {
              e.stopPropagation();
              onRemove?.();
            }}
            aria-label={t('删除')}
            style={{
              position: 'absolute',
              top: -6,
              right: -6,
              width: 18,
              height: 18,
              borderRadius: '50%',
              border: 'none',
              background: 'rgba(0,0,0,0.7)',
              color: '#fff',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: 0,
              lineHeight: 0,
              zIndex: 1,
            }}
          >
            <X size={11} />
          </button>
          {previewOpen && (
            <ImagePreview
              src={[image.dataUrl]}
              visible={previewOpen}
              currentIndex={0}
              onVisibleChange={setPreviewOpen}
              onClose={() => setPreviewOpen(false)}
              infinite={false}
            />
          )}
        </>
      ) : (
        <button
          type='button'
          onClick={onClick}
          aria-label={t('上传 {{label}}', { label })}
          style={{
            width: '100%',
            height: '100%',
            borderRadius: 10,
            border: '1.5px dashed var(--semi-color-border)',
            background: 'var(--semi-color-fill-0)',
            color: 'var(--semi-color-text-2)',
            cursor: 'pointer',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
            padding: 0,
            transition: 'background-color 150ms, color 150ms',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.background = 'var(--semi-color-fill-1)';
            e.currentTarget.style.color = 'var(--semi-color-text-1)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.background = 'var(--semi-color-fill-0)';
            e.currentTarget.style.color = 'var(--semi-color-text-2)';
          }}
        >
          <Plus size={18} />
          <span style={{ fontSize: 10, fontWeight: 600, letterSpacing: 0.3 }}>
            {label}
          </span>
        </button>
      )}
    </div>
  );
};

// 首/末帧之间的视觉距离：[slot] gap [button] gap [slot]
// 也就是 slot1 中心到 slot2 中心的偏移量。给 swap 动画做位移用
const FRAME_SWAP_DISTANCE = SLOT_SIZE + SLOT_GAP * 2 + SWAP_WIDTH;
const FRAME_SWAP_DURATION_MS = 320;

const FirstLastFrameUpload = ({
  images,
  onClickFirst,
  onClickLast,
  onRemoveFirst,
  onRemoveLast,
  onSwap,
  onRetryUpload,
  t,
}) => {
  // swap 过渡状态：true 期间两个 slot 通过 translateX 互相滑过，按钮旋转 180°；
  // 动画结束后再翻 onSwap 让父级真正交换 state。这样视觉位置 ≡ state 位置，
  // 不会出现"动画结束→state 跳变"的闪烁
  const [swapping, setSwapping] = useState(false);
  // 用 ref 保存 timer 句柄；组件卸载时清掉，避免迟到的 onSwap 落到已 unmount
  const swapTimerRef = useRef(null);
  useEffect(() => {
    return () => {
      if (swapTimerRef.current) {
        clearTimeout(swapTimerRef.current);
        swapTimerRef.current = null;
      }
    };
  }, []);

  const handleSwapClick = () => {
    if (swapping) return; // 动画中再点一次直接吞掉，避免 timer 叠加
    // 两边都空：没什么可动的，直接把状态翻一下走人；动画也无意义
    if (!images?.first && !images?.last) {
      onSwap?.();
      return;
    }
    setSwapping(true);
    swapTimerRef.current = window.setTimeout(() => {
      // React 会把这两次 setState 合并提交：state 翻好的同一帧把 swapping
      // 设为 false → transition='none' + transform='0' 一起生效。新 state 下
      // images.first / images.last 已对调，slot1 渲染的是新 first（视觉上
      // 就是动画末尾刚滑到左边的那张）→ 0 视觉跳变
      onSwap?.();
      setSwapping(false);
      swapTimerRef.current = null;
    }, FRAME_SWAP_DURATION_MS);
  };

  // cubic-bezier(0.65, 0, 0.35, 1) ≈ ease-in-out-cubic：开头/结尾稍慢，中段
  // 加速；比纯 ease-in-out 更跟手，又比 linear 不那么"机械"
  const slotTransition = swapping
    ? `transform ${FRAME_SWAP_DURATION_MS}ms cubic-bezier(0.65, 0, 0.35, 1)`
    : 'none';

  return (
    <div
      style={{
        position: 'absolute',
        right: 12,
        top: 18,
        height: SLOT_SIZE,
        display: 'flex',
        alignItems: 'center',
        gap: SLOT_GAP,
        zIndex: 5,
      }}
    >
      <div
        style={{
          // 首帧滑去末帧位置：+distance（向右）。zIndex 在动画期间提一档
          // 让两个 slot 滑过中间 swap 按钮时不会被它压在下面
          transform: swapping
            ? `translateX(${FRAME_SWAP_DISTANCE}px)`
            : 'translateX(0)',
          transition: slotTransition,
          zIndex: swapping ? 2 : 'auto',
        }}
      >
        <FrameSlot
          image={images?.first}
          onClick={onClickFirst}
          onRemove={onRemoveFirst}
          onRetryUpload={onRetryUpload}
          label={t('首帧')}
          t={t}
        />
      </div>
      <button
        type='button'
        onClick={handleSwapClick}
        aria-label={t('交换首尾帧')}
        title={t('交换首尾帧')}
        disabled={swapping}
        style={{
          width: SWAP_WIDTH,
          height: SWAP_WIDTH,
          borderRadius: '50%',
          border: '1px solid var(--semi-color-border)',
          background: 'var(--semi-color-bg-0)',
          color: 'var(--semi-color-text-1)',
          cursor: swapping ? 'default' : 'pointer',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 0,
          // 旋转 180° 暗示"对调"动作；旋转曲线和 slot 滑动用同一组缓动，
          // 视觉一致
          transform: swapping ? 'rotate(180deg)' : 'rotate(0deg)',
          transition: `background-color 150ms, color 150ms, transform ${FRAME_SWAP_DURATION_MS}ms cubic-bezier(0.65, 0, 0.35, 1)`,
        }}
        onMouseEnter={(e) => {
          if (swapping) return;
          e.currentTarget.style.background = 'var(--semi-color-fill-1)';
          e.currentTarget.style.color = 'var(--semi-color-text-0)';
        }}
        onMouseLeave={(e) => {
          if (swapping) return;
          e.currentTarget.style.background = 'var(--semi-color-bg-0)';
          e.currentTarget.style.color = 'var(--semi-color-text-1)';
        }}
      >
        <ArrowLeftRight size={12} />
      </button>
      <div
        style={{
          // 末帧反向滑：-distance（向左）。和首帧对称
          transform: swapping
            ? `translateX(${-FRAME_SWAP_DISTANCE}px)`
            : 'translateX(0)',
          transition: slotTransition,
          zIndex: swapping ? 2 : 'auto',
        }}
      >
        <FrameSlot
          image={images?.last}
          onClick={onClickLast}
          onRemove={onRemoveLast}
          onRetryUpload={onRetryUpload}
          label={t('末帧')}
          t={t}
        />
      </div>
    </div>
  );
};

const UnifiedInputBar = ({
  // 模型/分组
  inputs,
  modelEntries = [],
  currentModelEntry,
  currentModality,
  onModelGroupChange,

  // 行为
  loading,
  onSubmit,           // (text) => void  按当前 modality 由父层路由
  onStop,             // () => void       loading 时点发送按钮触发停止

  // image / video 模型的快捷参数：schema 里所有 enum 字段（size, quality,
  // aspect_ratio…）暴露成工具栏小下拉，方便用户不开右栏就改主要参数。
  paramSchema,            // { properties: { ... } } 不含 image input slots
  paramValues = {},
  onParamValuesChange,    // (next) => void

  // 「是否处理参考图请求」：true 时 paste/drag/picker 会调 onAddReferenceImage
  // （父层决定接受还是 toast 拒绝）；false 时所有上传通道直接静默
  acceptsReferenceImage = false,
  // 「是否在右侧渲染上传按钮基座」：仅图片/视频模型且 schema 有 image
  // input slot 时为 true。多模态模型 acceptsReferenceImage=true 但
  // showUploadButton=false（仅 paste/drag）
  showUploadButton = false,

  // 参考图统一表示：[{ key, dataUrl, name?, file?, uploading?, failed?,
  // uploadError? }]，由父层按 modality 派生。uploading/failed 标记仅在视频
  // 模型 R2 即时上传场景下有意义；其它场景全是 false。
  referenceImages = [],
  onAddReferenceImage,    // (file: File, opts?: { targetRole }) => void
  onRetryUpload,          // (file: File) => void —— 失败缩略图的重试回调
  onRemoveReferenceImage, // (key: string) => void

  // 视频「全能参考 / 首尾帧」模式（仅 schema 同时含两类槽位时启用）
  videoInputModeAvailable = false,
  videoInputMode = 'omni',
  onVideoInputModeChange,
  // 仅 first_last 模式生效：替换 ReferenceImageStack 的双上传 UI
  firstLastFrameImages = null,   // { first: {dataUrl,name}|null, last: {...}|null }
  onSwapFirstLastFrame,
  onRemoveFirstFrame,
  onRemoveLastFrame,

  // 外部注入的预填文案：深链 ?prompt= 用。父层 set 一次后立即 reset 到 null,
  // 子内部 useEffect 监听非 null 时把本地 text 同步过去；之后父层不再干涉
  pendingText = null,
  onPendingTextConsumed,

  styleState,
}) => {
  const { t } = useTranslation();
  const [text, setText] = useState('');

  // pendingText 接力：外部一次性注入文案到本地 text，并立刻通知父层 reset
  // 这个 prop（避免把 pendingText 当成 controlled value 反复同步）
  useEffect(() => {
    if (pendingText == null) return;
    setText(pendingText);
    onPendingTextConsumed?.();
  }, [pendingText, onPendingTextConsumed]);
  const [isDragOver, setIsDragOver] = useState(false);
  // 模型选择「模式」过滤：smart / image / video。仅影响下拉里展示哪些
  // 模型；非 smart 时切换还会顺手把当前选中模型自动换成第一条匹配项，
  // 让「快速选图/选视频」体验更顺。
  const [selectedMode, setSelectedMode] = useState('smart');
  // 视频「全能参考」模式下的 @-mention 状态：
  //   null              → 未触发，正常输入
  //   { atIdx, filter, selectedIdx } → 用户在 atIdx 处键入了 @ 并继续打字
  // atIdx 指 @ 字符在文本里的偏移；filter 是 @ 之后到光标之间的文字（用于
  // 后续过滤候选，目前未启用过滤——图片很少，全部展示）；selectedIdx 是
  // 候选列表里的高亮项，用于键盘 ↑↓ 选择和 Enter 插入。
  const [mentionState, setMentionState] = useState(null);
  const textareaRef = useRef(null);
  const fileInputRef = useRef(null);
  const mentionPopupRef = useRef(null);
  // @-mention 仅在视频「全能参考」+ 已上传至少一张参考图时启用
  const mentionEnabled =
    currentModality === MODALITY.VIDEO &&
    videoInputMode === 'omni' &&
    referenceImages.length > 0;

  const filteredModelEntries = useMemo(() => {
    if (selectedMode === 'smart') return modelEntries;
    const def = MODE_DEFS.find((m) => m.key === selectedMode);
    if (!def?.modality) return modelEntries;
    return modelEntries.filter((e) => e.modality === def.modality);
  }, [modelEntries, selectedMode]);

  const handleModeChange = (nextMode) => {
    setSelectedMode(nextMode);
    if (nextMode === 'smart') return;
    // 非 smart：如果当前模型不在新筛选范围里，自动选第一条
    const def = MODE_DEFS.find((m) => m.key === nextMode);
    const targetModality = def?.modality;
    if (!targetModality) return;
    const currentOk = modelEntries.some(
      (e) =>
        e.model === inputs?.model &&
        e.group === inputs?.group &&
        e.modality === targetModality,
    );
    if (currentOk) return;
    const firstMatch = modelEntries.find((e) => e.modality === targetModality);
    if (firstMatch) onModelGroupChange?.(firstMatch.model, firstMatch.group);
  };

  // 原生 textarea + 手动 auto-grow（与 ImageWorkspace 等保持一致）
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 192) + 'px';
  }, [text]);

  // 从光标位置往前找最近的 @，若该 @ 位于行首或前一字符是空白，且 @ 到光标
  // 之间没有空白/换行，则认为正在写 mention，返回 { atIdx, filter }。
  // 否则返回 null。让 "email@x" 这种中段 @ 不触发弹层。
  const detectMention = (textVal, caretPos) => {
    let i = caretPos - 1;
    while (i >= 0) {
      const ch = textVal[i];
      if (ch === '@') {
        if (i === 0 || /\s/.test(textVal[i - 1])) {
          const filter = textVal.slice(i + 1, caretPos);
          if (/[\s\n]/.test(filter)) return null;
          return { atIdx: i, filter };
        }
        return null;
      }
      if (/[\s\n]/.test(ch)) return null;
      i--;
    }
    return null;
  };

  const handleTextareaChange = (e) => {
    const newText = e.target.value;
    setText(newText);
    if (!mentionEnabled) {
      if (mentionState) setMentionState(null);
      return;
    }
    const caret = e.target.selectionStart ?? newText.length;
    const m = detectMention(newText, caret);
    if (m) {
      setMentionState((prev) => ({
        atIdx: m.atIdx,
        filter: m.filter,
        // 保留原来的 selectedIdx；首次打开时从 0 开始
        selectedIdx:
          prev && prev.atIdx === m.atIdx
            ? Math.min(prev.selectedIdx, referenceImages.length - 1)
            : 0,
      }));
    } else if (mentionState) {
      setMentionState(null);
    }
  };

  // 切模态/切模式/参考图数量变化时关闭弹层，避免引用过期 idx
  useEffect(() => {
    setMentionState(null);
  }, [currentModality, videoInputMode, referenceImages.length]);

  // 点击 textarea / popup 外部 → 关闭
  useEffect(() => {
    if (!mentionState) return;
    const handler = (e) => {
      if (mentionPopupRef.current?.contains(e.target)) return;
      if (textareaRef.current?.contains(e.target)) return;
      setMentionState(null);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [mentionState]);

  // 选中候选 → 把 @<filter> 整段替换成 @image{N}（N 从 1 起）
  const insertMention = (n) => {
    if (!mentionState) return;
    const before = text.slice(0, mentionState.atIdx);
    const after = text.slice(
      mentionState.atIdx + 1 + mentionState.filter.length,
    );
    const inserted = `@image${n} `;
    const newText = before + inserted + after;
    setText(newText);
    setMentionState(null);
    const newCaret = before.length + inserted.length;
    // 等下一帧 textarea 用新值 re-render 后再设置光标
    requestAnimationFrame(() => {
      const el = textareaRef.current;
      if (!el) return;
      try {
        el.focus();
        el.setSelectionRange(newCaret, newCaret);
      } catch {}
    });
  };

  // loading 结束后回焦到输入框
  useEffect(() => {
    if (!loading && textareaRef.current?.focus) {
      try {
        textareaRef.current.focus();
      } catch {}
    }
  }, [loading]);

  // 文件选择器最近一次点击的目标 slot（'first_frame' / 'last_frame'）。
  // 仅 first_last 模式下两个独立上传位用到——点空首/末帧框时把 ref 置上
  // 对应 role，handleFilePick 读到后传给父层强制写入指定 slot；其它路径
  // （paste / drag / 全能参考下的 + 按钮）保持 ref=null，走父层的默认填充
  // 顺序。
  const pendingTargetRoleRef = useRef(null);

  const ingestFile = (file) => {
    if (!file) return;
    if (!file.type || !file.type.startsWith('image/')) {
      Toast.warning({ content: t('仅支持图片格式'), duration: 2 });
      return;
    }
    // 由父层决定接受还是 toast 拒绝（按当前 modality 决策）。这里不再
    // 早 return，确保「文本模型上传时也能给用户反馈」。
    const targetRole = pendingTargetRoleRef.current;
    onAddReferenceImage?.(file, targetRole ? { targetRole } : undefined);
  };

  // 不在这里 gate「acceptsReferenceImage」——让文本模型的 paste/drag 也
  // 能进入 ingestFile，由父层的 handleAddReferenceImage 给出 toast 反馈。
  const handlePaste = (e) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      if (item.type.indexOf('image') !== -1) {
        e.preventDefault();
        const file = item.getAsFile();
        if (file) ingestFile(file);
        break;
      }
    }
  };

  const handleDragOver = (e) => {
    if (e.dataTransfer?.types?.includes('Files')) {
      e.preventDefault();
      // 只在「真的支持」时高亮，给 UI 反馈；不支持的模型也允许 drop
      // 但视觉上不强提示
      if (acceptsReferenceImage) setIsDragOver(true);
    }
  };
  const handleDragLeave = () => setIsDragOver(false);
  const handleDrop = (e) => {
    setIsDragOver(false);
    const files = e.dataTransfer?.files;
    if (!files || files.length === 0) return;
    e.preventDefault();
    Array.from(files).forEach(ingestFile);
  };

  const triggerFilePicker = () => {
    pendingTargetRoleRef.current = null;
    fileInputRef.current?.click();
  };
  // 首尾帧专用：点空 slot → 触发文件选择器，并把 targetRole 暂存到 ref；
  // ingestFile 读到后会 forward 给 onAddReferenceImage，父层据此把图写
  // 入指定 slot 而不是按"先 first 后 last"的默认顺序填
  const triggerFilePickerForFrame = (role) => {
    pendingTargetRoleRef.current = role;
    fileInputRef.current?.click();
  };
  const handleFilePick = (e) => {
    const files = e.target.files;
    if (files && files.length > 0) Array.from(files).forEach(ingestFile);
    e.target.value = ''; // 允许连续选择同一张
    pendingTargetRoleRef.current = null;
  };

  // 全 modality 一视同仁：必须有非空 prompt 才能发送。先前曾允许"视频
  // first_last 仅传首/末帧不写文本"，但实测上游服务商（Ali / Sora 等）
  // 即便参考图齐全也会因缺 prompt 拒收——发送按钮维持可点反而误导用户
  const canSend =
    !loading &&
    !!inputs?.model &&
    text.trim().length > 0;
  const handleSubmit = async () => {
    if (!canSend) return;
    const value = text;
    setText('');
    await onSubmit?.(value);
  };

  // 输入提示文案随 modality + 视频模式调整
  const placeholder = useMemo(() => {
    if (currentModality === MODALITY.VIDEO) {
      if (videoInputMode === 'first_last') {
        return t(
          '首帧图和尾帧图，尽量保持同样的图片比例，尽量都包含同样的主体，并用文字描述两张图之间如何过渡。',
        );
      }
      // omni（默认）：暗示用户可以用 @ 快捷引用上传过的参考图
      return t('使用 @ 快速调用参考内容');
    }
    switch (currentModality) {
      case MODALITY.IMAGE:
        return t('描述你想要的图片…');
      case MODALITY.MULTIMODAL:
        return t('输入消息（支持图片附件）…');
      default:
        return t('给模型发条消息…');
    }
  }, [currentModality, videoInputMode, t]);

  return (
    <div
      className='flex-shrink-0 w-full mx-auto px-3 pb-3 sm:px-4 sm:pb-4'
      style={{ maxWidth: 860 }}
    >
      <div
        // 默认就有 2px 实色边框，把输入框从同色背景里清晰拎出来；
        // focus-within 才叠一道整体阴影传达「聚焦」状态。
        className='rounded-2xl transition-shadow focus-within:shadow-[0_4px_16px_rgba(0,0,0,0.08),0_2px_6px_rgba(0,0,0,0.06)]'
        style={{
          position: 'relative',
          border: isDragOver
            ? '2px dashed var(--semi-color-primary)'
            : '2px solid var(--semi-color-border)',
          background: 'var(--semi-color-bg-0)',
        }}
        onPaste={handlePaste}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        {/* @-mention 弹层：仅视频全能参考 + 至少 1 张参考图时由
            handleTextareaChange 唤出。绝对定位贴在输入框正上方，候选条目
            是 referenceImages 里的图缩略图 + 「图片 N」。键盘 ↑↓/Enter/Tab
            导航；点候选或回车把 @<filter> 整段替换为 @image{N}。 */}
        {mentionState && referenceImages.length > 0 && (
          <div
            ref={mentionPopupRef}
            style={{
              position: 'absolute',
              left: 12,
              bottom: 'calc(100% + 6px)',
              minWidth: 200,
              maxWidth: 280,
              maxHeight: 240,
              overflowY: 'auto',
              zIndex: 20,
              background: 'var(--semi-color-bg-overlay)',
              border: '1px solid var(--semi-color-border)',
              borderRadius: 12,
              boxShadow: '0 8px 24px rgba(0,0,0,0.16)',
              padding: 6,
            }}
          >
            <div
              style={{
                padding: '4px 8px 6px',
                fontSize: 11,
                fontWeight: 600,
                letterSpacing: 0.3,
                color: 'var(--semi-color-text-2)',
              }}
            >
              {t('选择参考图')}
            </div>
            {referenceImages.map((img, i) => {
              const selected = i === mentionState.selectedIdx;
              return (
                <button
                  key={img.key}
                  type='button'
                  onClick={() => insertMention(i + 1)}
                  onMouseEnter={() =>
                    setMentionState((s) =>
                      s ? { ...s, selectedIdx: i } : s,
                    )
                  }
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                    width: '100%',
                    padding: '6px 8px',
                    border: 'none',
                    background: selected
                      ? 'var(--semi-color-primary-light-default)'
                      : 'transparent',
                    cursor: 'pointer',
                    borderRadius: 8,
                    color: 'var(--semi-color-text-0)',
                    fontFamily: 'inherit',
                    fontSize: 13,
                    fontWeight: selected ? 500 : 400,
                    textAlign: 'left',
                  }}
                >
                  <img
                    src={optimizeImageUrl(img.dataUrl, {
                      width: pixelWidth(28),
                    })}
                    alt=''
                    draggable={false}
                    style={{
                      width: 28,
                      height: 28,
                      borderRadius: 4,
                      objectFit: 'cover',
                      flexShrink: 0,
                    }}
                  />
                  <span style={{ flex: 1, minWidth: 0 }}>
                    {t('图片{{n}}', { n: i + 1 })}
                  </span>
                  <span
                    style={{
                      fontSize: 11,
                      color: 'var(--semi-color-text-2)',
                      fontFamily:
                        'ui-monospace, SFMono-Regular, Menlo, monospace',
                    }}
                  >
                    @image{i + 1}
                  </span>
                </button>
              );
            })}
          </div>
        )}

        {/* 文本区：默认 4 行；右侧给参考图堆叠 / 首尾帧双上传预留空间 */}
        <textarea
          ref={textareaRef}
          value={text}
          onChange={handleTextareaChange}
          placeholder={placeholder}
          rows={3}
          disabled={loading}
          onKeyDown={(e) => {
            // mention 弹层打开时优先拦截方向键 / 选择键，否则 Enter 会
            // 触发提交、Tab 会跳焦
            if (mentionState && referenceImages.length > 0) {
              if (e.key === 'ArrowDown') {
                e.preventDefault();
                setMentionState((s) =>
                  s
                    ? {
                        ...s,
                        selectedIdx: Math.min(
                          s.selectedIdx + 1,
                          referenceImages.length - 1,
                        ),
                      }
                    : s,
                );
                return;
              }
              if (e.key === 'ArrowUp') {
                e.preventDefault();
                setMentionState((s) =>
                  s
                    ? { ...s, selectedIdx: Math.max(s.selectedIdx - 1, 0) }
                    : s,
                );
                return;
              }
              if (e.key === 'Enter' || e.key === 'Tab') {
                e.preventDefault();
                insertMention(mentionState.selectedIdx + 1);
                return;
              }
              if (e.key === 'Escape') {
                e.preventDefault();
                setMentionState(null);
                return;
              }
            }
            // Enter 发送，Shift+Enter 换行；和系统全局习惯保持一致。
            // 避免 IME 编辑中的 Enter 误发：keyCode===229 时跳过。
            if (e.key === 'Enter' && !e.shiftKey && e.keyCode !== 229) {
              e.preventDefault();
              handleSubmit();
            }
          }}
          className='w-full block'
          style={{
            resize: 'none',
            border: 'none',
            outline: 'none',
            boxShadow: 'none',
            background: 'transparent',
            padding: `14px ${
              // 右内边距按"右侧浮元素"的实际宽度变化：
              //  - 视频 first_last 模式 → 双上传整体（2 框 + 中间 swap） + 24 留白
              //  - 全能参考 / 图片堆叠 → 堆叠宽度（按张数增长，封顶 4 档） + 24
              //  - 不支持上传 → 16（最小 padding）
              videoInputMode === 'first_last' && firstLastFrameImages
                ? FIRST_LAST_TOTAL_WIDTH + 24
                : acceptsReferenceImage || referenceImages.length > 0
                  ? THUMB_SIZE +
                    COLLAPSE_STEP *
                      Math.min(
                        MAX_COLLAPSED_OFFSETS,
                        Math.max(referenceImages.length - 1, 0),
                      ) +
                    24
                  : 16
            }px 8px 16px`,
            fontSize: 14,
            lineHeight: 1.6,
            minHeight: 84,
            maxHeight: 240,
            color: 'var(--semi-color-text-0)',
            fontFamily: 'inherit',
          }}
        />

        {/* 右上角浮元素：根据 videoInputMode 二选一渲染
            - first_last：FirstLastFrameUpload 双独立 slot
            - 其它：ReferenceImageStack 堆叠 + 上传基座 */}
        {videoInputMode === 'first_last' && firstLastFrameImages ? (
          <FirstLastFrameUpload
            images={firstLastFrameImages}
            onClickFirst={() => triggerFilePickerForFrame('first_frame')}
            onClickLast={() => triggerFilePickerForFrame('last_frame')}
            onRemoveFirst={onRemoveFirstFrame}
            onRemoveLast={onRemoveLastFrame}
            onSwap={onSwapFirstLastFrame}
            onRetryUpload={onRetryUpload}
            t={t}
          />
        ) : (
          <ReferenceImageStack
            images={referenceImages}
            onRemove={onRemoveReferenceImage}
            showUploadButton={showUploadButton}
            onUpload={triggerFilePicker}
            onRetryUpload={onRetryUpload}
            t={t}
          />
        )}

        {/* 隐藏文件选择器：「+ 上传参考图」点击触发 */}
        <input
          ref={fileInputRef}
          type='file'
          accept='image/*'
          multiple
          hidden
          onChange={handleFilePick}
        />

        {/* 工具栏：所有按钮无边框 + hover fill-1 灰底（除右侧发送）。
            模式选择放在模型选择左侧；模式过滤后的 entries 喂给模型选择器。 */}
        <div className='flex items-center gap-1 px-2 pb-2'>
          <ModeSelector
            mode={selectedMode}
            onModeChange={handleModeChange}
            disabled={loading}
            t={t}
          />

          <ModelPickerPill
            modelEntries={filteredModelEntries}
            inputs={inputs}
            currentEntry={currentModelEntry}
            onChange={onModelGroupChange}
            disabled={loading}
            t={t}
          />

          {/* 视频「全能参考 / 首尾帧」模式选择器：仅当 schema 同时声明
              reference_image 和 first/last_frame 槽位时父层置 true。
              紧贴模型选择器右侧，让用户能用同一片视觉区域控制"附件形态"。 */}
          {videoInputModeAvailable && (
            <VideoInputModeSelector
              mode={videoInputMode}
              onModeChange={onVideoInputModeChange}
              disabled={loading}
              t={t}
            />
          )}

          {/* 仅 image / video 模型展示的快捷参数下拉条。schema 里所有
              enum 字段一字排开；多了横向滚动，滚动条隐藏。flex:1 + min-w:0
              让它占据 ModelPickerPill 和 send 之间的剩余空间且可压缩。 */}
          {(currentModality === MODALITY.IMAGE ||
            currentModality === MODALITY.VIDEO) &&
            paramSchema?.properties && (
              <div
                className='playground-toolbar-params flex items-center gap-1 overflow-x-auto'
                style={{ flex: 1, minWidth: 0 }}
              >
                {Object.entries(paramSchema.properties)
                  .filter(([key, def]) => isToolbarParam(key, def))
                  .map(([key, def]) => (
                    <SchemaParamSelector
                      key={key}
                      paramKey={key}
                      def={def}
                      value={paramValues?.[key]}
                      onChange={(v) =>
                        onParamValuesChange?.({ ...paramValues, [key]: v })
                      }
                      disabled={loading}
                      t={t}
                    />
                  ))}
              </div>
            )}

          <div style={{ flex: currentModality === MODALITY.IMAGE || currentModality === MODALITY.VIDEO ? 0 : 1 }} />

          {/* 视频模型工具栏已经塞满了模式选择器 + 多个参数下拉，再加一行
              提示文字会把整条工具栏挤到换行；这里直接隐藏，保留 image 和
              text/multimodal 模型的提示。 */}
          {currentModality !== MODALITY.VIDEO && (
            <Typography.Text
              type='tertiary'
              className='text-xs select-none hidden sm:inline'
              style={{ paddingRight: 6 }}
            >
              {loading ? t('生成中…') : t('Enter 发送 / Shift+Enter 换行')}
            </Typography.Text>
          )}

          {/* 发送按钮：loading 时变成停止按钮（实心方块），点击调用 onStop。
              避免 loading 同时还出 Semi 自带的「输入框上方停止条」造成两个
              入口；用一个按钮承载「发送 → 停止」两态。 */}
          <Button
            theme='solid'
            type={loading ? 'danger' : 'primary'}
            icon={
              loading ? (
                <Square size={14} strokeWidth={0} fill='currentColor' />
              ) : (
                <ArrowUp size={18} strokeWidth={2.5} />
              )
            }
            onClick={loading ? () => onStop?.() : handleSubmit}
            disabled={loading ? !onStop : !canSend}
            style={{
              ...TOOLBAR_BTN_STYLE,
              padding: 0,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
            aria-label={loading ? t('停止') : t('发送')}
          />
        </div>
      </div>
    </div>
  );
};

// 工具栏按钮统一尺寸 + 圆角，给所有按钮（含发送按钮）共用，避免高度
// 不一致带来的视觉抖动。圆角 10px：和站内通用按钮风格一致，不做全圆角胶囊。
const TOOLBAR_BTN_STYLE = {
  width: 32,
  height: 32,
  minWidth: 32,
  borderRadius: 10,
};

export default UnifiedInputBar;
