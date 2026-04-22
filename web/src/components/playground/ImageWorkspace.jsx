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

import React, { useMemo, useRef, useState, useEffect } from 'react';
import {
  Button,
  Card,
  ImagePreview,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  ArrowUp,
  ChevronDown,
  ChevronUp,
  Copy,
  Eye,
  EyeOff,
  Paperclip,
  Pencil,
  Trash2,
  X,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { MESSAGE_ROLES } from '../../constants/playground.constants';
import { getLogo, showError, showSuccess } from '../../helpers';
import { useActualTheme } from '../../context/Theme';
import { buildDefaultPlaygroundLogo } from './workspaceLogo';
import {
  SchemaInputsRenderer,
  hasImageInputSlot,
} from './SchemaParamsRenderer';

// 一次生成 = 一条 user + 一条 assistant
const groupIntoGenerations = (messages) => {
  const result = [];
  let pending = null;
  (messages || []).forEach((m) => {
    if (!m) return;
    if (m.role === MESSAGE_ROLES.USER) {
      if (pending) result.push(pending);
      pending = { id: m.id, promptMessage: m, assistantMessage: null };
    } else if (m.role === MESSAGE_ROLES.ASSISTANT) {
      if (pending) {
        pending.assistantMessage = m;
        result.push(pending);
        pending = null;
      } else {
        result.push({ id: m.id, promptMessage: null, assistantMessage: m });
      }
    }
  });
  if (pending) result.push(pending);
  return result.reverse();
};

const extractImages = (assistantMessage) => {
  if (!assistantMessage) return [];
  const c = assistantMessage.content;
  if (Array.isArray(c)) {
    return c
      .filter((p) => p && p.type === 'image_url' && p.image_url?.url)
      .map((p) => ({ url: p.image_url.url, revisedPrompt: p.revised_prompt }));
  }
  return [];
};

const getPromptText = (msg) => {
  if (!msg) return '';
  const c = msg.content;
  if (typeof c === 'string') return c;
  if (Array.isArray(c)) {
    return c
      .filter((p) => p?.type === 'text')
      .map((p) => p.text)
      .join(' ');
  }
  return '';
};

const formatRelativeTime = (ts, t) => {
  if (!ts) return '';
  const now = Date.now();
  const diff = now - ts;
  if (diff < 60 * 1000) return t('刚刚');
  if (diff < 60 * 60 * 1000) {
    return t('{{n}} 分钟前', { n: Math.floor(diff / 60000) });
  }
  return new Date(ts).toLocaleString();
};

const GenerationCard = ({
  generation,
  onCopyPrompt,
  onDelete,
  onContinueEdit,
  supportsContinueEdit,
  t,
}) => {
  const prompt = getPromptText(generation.promptMessage);
  const images = extractImages(generation.assistantMessage);
  const loading = generation.assistantMessage?.status === 'loading';
  const isError = generation.assistantMessage?.status === 'error';
  const meta = generation.assistantMessage?.meta || {};
  const ts =
    generation.assistantMessage?.createAt ||
    generation.promptMessage?.createAt;

  // 受控预览：每张卡片独立维护自己的大图预览状态，点缩略图展开，
  // 使用 Semi 内置的 ImagePreview（带缩放/旋转/前后翻/下载）。
  const [previewIdx, setPreviewIdx] = React.useState(-1);
  const previewOpen = previewIdx >= 0;
  const previewUrls = React.useMemo(() => images.map((x) => x.url), [images]);

  const metaPieces = [];
  if (meta.model) metaPieces.push({ tag: true, text: meta.model });
  const p = meta.params || {};
  ['size', 'aspect_ratio', 'aspectRatio', 'quality'].forEach((k) => {
    if (p[k] !== undefined && p[k] !== '' && p[k] !== 'auto') {
      metaPieces.push({ text: String(p[k]) });
    }
  });
  if (ts) metaPieces.push({ text: formatRelativeTime(ts, t) });

  return (
    <div
      className='rounded-xl mb-4 overflow-hidden transition-shadow hover:shadow-sm'
      style={{
        backgroundColor: 'var(--semi-color-bg-0)',
        border: '1px solid var(--semi-color-border)',
      }}
    >
      <div
        className='px-4 py-3 flex items-start gap-3'
        style={{ borderBottom: '1px solid var(--semi-color-fill-0)' }}
      >
        <div className='flex-1 min-w-0'>
          {prompt ? (
            <Typography.Paragraph
              ellipsis={{ rows: 2, showTooltip: { opts: { content: prompt } } }}
              className='!mb-1 text-sm font-medium'
              style={{ color: 'var(--semi-color-text-0)' }}
            >
              {prompt}
            </Typography.Paragraph>
          ) : (
            <Typography.Text type='tertiary' className='text-sm italic'>
              {t('(无 prompt)')}
            </Typography.Text>
          )}
          {metaPieces.length > 0 && (
            <div className='flex items-center gap-2 flex-wrap mt-1'>
              {metaPieces.map((m, i) =>
                m.tag ? (
                  <Tag key={i} size='small' shape='circle' color='white'>
                    {m.text}
                  </Tag>
                ) : (
                  <Typography.Text
                    key={i}
                    type='tertiary'
                    className='text-xs'
                  >
                    {i > 0 ? '· ' : ''}
                    {m.text}
                  </Typography.Text>
                ),
              )}
            </div>
          )}
        </div>
        <div className='flex items-center gap-1 flex-shrink-0'>
          {prompt && onCopyPrompt && (
            <Tooltip content={t('复制 Prompt')}>
              <Button
                icon={<Copy size={14} />}
                size='small'
                theme='borderless'
                type='tertiary'
                onClick={() => onCopyPrompt(prompt)}
              />
            </Tooltip>
          )}
          {onDelete && (
            <Tooltip content={t('删除')}>
              <Button
                icon={<Trash2 size={14} />}
                size='small'
                theme='borderless'
                type='tertiary'
                onClick={() => onDelete(generation)}
              />
            </Tooltip>
          )}
        </div>
      </div>

      {loading && (
        <div className='flex flex-col items-center justify-center py-10 gap-2'>
          <Spin size='middle' />
          <Typography.Text type='tertiary' className='text-sm'>
            {t('生成中…')}
          </Typography.Text>
        </div>
      )}

      {isError && !loading && (
        <div className='px-4 py-4 flex items-center gap-2'>
          <X
            size={16}
            style={{ color: 'var(--semi-color-danger)', flexShrink: 0 }}
          />
          <Typography.Text type='danger' className='text-sm'>
            {generation.assistantMessage?.errorMessage || t('生成失败')}
          </Typography.Text>
        </div>
      )}

      {!loading && !isError && images.length > 0 && (
        <div className='p-2'>
          {/* 保留图片原始比例：单图时按原比例居中展示（竖图不再占满宽度），
              多图时 2 列并排，每格内按原比例居中。用 max-height 统一封顶
              避免超长竖图撑破版面。 */}
          <div className={images.length > 1 ? 'grid grid-cols-2 gap-2' : ''}>
            {images.map((img, i) => {
              const capH = images.length > 1 ? 420 : 560;
              // "继续编辑"按钮只在：模型 schema 声明了 image 槽 + 图本身是
              // data URL（可以在浏览器里直接转 blob，没有 CORS 问题）时出现。
              // 远程 url 不显示，避免跨域拿不到 blob 的静默失败。
              const canContinue =
                supportsContinueEdit &&
                typeof img.url === 'string' &&
                img.url.startsWith('data:');
              return (
                <div
                  key={i}
                  className='flex justify-center items-start'
                >
                  <div
                    className='group relative rounded-lg overflow-hidden cursor-zoom-in'
                    style={{
                      display: 'inline-block',
                      maxWidth: '100%',
                      backgroundColor: 'var(--semi-color-fill-0)',
                    }}
                    onClick={() => setPreviewIdx(i)}
                  >
                    <img
                      src={img.url}
                      alt={prompt || `image-${i}`}
                      loading='lazy'
                      className='transition-transform duration-300 group-hover:scale-[1.02]'
                      style={{
                        display: 'block',
                        maxWidth: '100%',
                        maxHeight: capH,
                        width: 'auto',
                        height: 'auto',
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
                          className='!absolute'
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

          {/* 受控预览：Semi ImagePreview 自带缩放/旋转/前后翻/下载 */}
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
      )}
    </div>
  );
};

/**
 * ImageWorkspace 只负责画廊 + prompt 输入。参数（schema 驱动）由右栏"参数"
 * Tab 提供，调用 onGenerate 时由父层拼装完整 payload。
 *
 * 图像输入槽（format:image 的 schema 字段）由此处的 SchemaInputsRenderer
 * 渲染，值通过受控 props `inputsValues` / `onInputsChange` 和父层双向绑定。
 */
const ImageWorkspace = ({
  message = [],
  inputs,
  styleState,
  onGenerate,
  loading = false,
  onDeleteGeneration,
  onClearAll,
  showDebugPanel,
  onToggleDebugPanel,
  inputsSchema,
  inputsValues,
  onInputsChange,
}) => {
  const { t } = useTranslation();
  const [prompt, setPrompt] = useState('');
  const textareaRef = useRef(null);

  const generations = useMemo(() => groupIntoGenerations(message), [message]);
  const hasResults = generations.length > 0;
  const actualTheme = useActualTheme();
  const logoUrl =
    getLogo() || buildDefaultPlaygroundLogo(actualTheme === 'dark');
  const supportsImageInput = hasImageInputSlot(inputsSchema);

  // 附件面板折叠：默认折叠，带附件时顶部小徽标显示数量。
  // 自动展开时机：
  //   1) 数量从 0 → >0（比如"继续编辑"或首次拖拽上传）——让用户能立刻
  //      看到文件落位；
  //   2) 用户手动点顶部条目。
  // 自动收起时机：
  //   - 发送后 inputs 被父层清空，数量回到 0，自动折叠让界面回到纯净态。
  const [inputsExpanded, setInputsExpanded] = useState(false);
  const inputsFileCount = useMemo(() => {
    if (!inputsValues) return 0;
    let n = 0;
    for (const v of Object.values(inputsValues)) {
      if (v instanceof File) n += 1;
      else if (Array.isArray(v)) n += v.filter((x) => x instanceof File).length;
    }
    return n;
  }, [inputsValues]);
  const prevInputsCountRef = useRef(0);
  useEffect(() => {
    const prev = prevInputsCountRef.current;
    if (prev === 0 && inputsFileCount > 0) setInputsExpanded(true);
    if (prev > 0 && inputsFileCount === 0) setInputsExpanded(false);
    prevInputsCountRef.current = inputsFileCount;
  }, [inputsFileCount]);

  // 原生 textarea 的 auto-grow：在 value 变化时按 scrollHeight 调高度
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 192) + 'px';
  }, [prompt]);

  useEffect(() => {
    if (!loading && textareaRef.current?.focus) {
      try {
        textareaRef.current.focus();
      } catch {}
    }
  }, [loading]);

  const handleSubmit = async () => {
    if (!prompt.trim() || loading) return;
    const text = prompt;
    const capturedInputs = inputsValues;
    setPrompt('');
    // 清空附件：发送是一次性的，防止下一次意外带上上次的图
    onInputsChange?.({});
    await onGenerate?.({ prompt: text, inputs: capturedInputs || {} });
  };

  // 继续编辑：把生成出来的图转成 File 塞进 image 槽。
  // 仅支持 data URL（浏览器内存转换，无 CORS 风险）；远程 https URL 的
  // 按钮在 GenerationCard 那边已经被隐藏。
  const handleContinueEdit = async (img) => {
    if (!img?.url || !img.url.startsWith('data:')) return;
    try {
      const blob = await fetch(img.url).then((r) => r.blob());
      const mime = blob.type || 'image/png';
      const ext = mime.split('/')[1] || 'png';
      const file = new File([blob], `continue-${Date.now()}.${ext}`, {
        type: mime,
      });
      // 语义约定：schema 里有 image 字段时优先填它；否则退一步填第一个
      // 声明过的图像槽。这样对 OpenAI / Gemini 以及未来厂商都兼容。
      const props = inputsSchema?.properties || {};
      let targetKey = Object.prototype.hasOwnProperty.call(props, 'image')
        ? 'image'
        : null;
      let targetDef = targetKey ? props[targetKey] : null;
      if (!targetKey) {
        const firstEntry = Object.entries(props).find(([, def]) => {
          if (!def) return false;
          if (def.format === 'image') return true;
          if (def.type === 'array' && def.items?.format === 'image') return true;
          return false;
        });
        if (firstEntry) {
          targetKey = firstEntry[0];
          targetDef = firstEntry[1];
        }
      }
      if (!targetKey) {
        showError(t('当前模型未声明图像输入，无法继续编辑'));
        return;
      }
      const next = { ...(inputsValues || {}) };
      if (targetDef?.type === 'array') {
        const prev = Array.isArray(next[targetKey])
          ? next[targetKey].filter((x) => x instanceof File)
          : [];
        const maxItems = targetDef?.maxItems ?? 4;
        next[targetKey] = [...prev, file].slice(0, maxItems);
      } else {
        next[targetKey] = file;
      }
      onInputsChange?.(next);
      showSuccess(t('已填入参考图'));
      textareaRef.current?.focus?.();
    } catch (err) {
      showError(err?.message || t('填入参考图失败'));
    }
  };

  const handleCopyPrompt = async (text) => {
    try {
      await navigator.clipboard.writeText(text);
      showSuccess(t('已复制 Prompt'));
    } catch {}
  };

  return (
    <Card
      className='h-full'
      bordered={false}
      bodyStyle={{
        padding: 0,
        height: 'calc(100vh - 66px)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      {/* 头部：与 ChatArea 同款 */}
      {styleState?.isMobile ? (
        <div className='pt-4'></div>
      ) : (
        <div className='px-6 py-4 bg-gradient-to-r from-purple-500 to-blue-500 rounded-t-2xl'>
          <div className='flex items-center justify-between'>
            <div className='flex flex-col min-w-0'>
              <Typography.Title heading={5} className='!text-white mb-0'>
                {t('图片生成')}
              </Typography.Title>
              <Typography.Text className='!text-white/80 text-sm hidden sm:inline truncate'>
                {inputs?.model || t('选择图片模型开始生成')}
              </Typography.Text>
            </div>
            <div className='flex items-center gap-2'>
              {hasResults && onClearAll && (
                <Button
                  icon={<Trash2 size={14} />}
                  theme='borderless'
                  type='primary'
                  size='small'
                  className='!rounded-lg !text-white/80 hover:!text-white hover:!bg-white/10'
                  onClick={onClearAll}
                >
                  {t('清空')}
                </Button>
              )}
              {onToggleDebugPanel && (
                <Button
                  icon={
                    showDebugPanel ? <EyeOff size={14} /> : <Eye size={14} />
                  }
                  onClick={onToggleDebugPanel}
                  theme='borderless'
                  type='primary'
                  size='small'
                  className='!rounded-lg !text-white/80 hover:!text-white hover:!bg-white/10'
                >
                  {showDebugPanel ? t('隐藏面板') : t('显示面板')}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 画廊区：空态时整块垂直居中；有数据时按常规流式向下排 */}
      <div className='flex-1 overflow-y-auto'>
        {!hasResults ? (
          <div className='h-full w-full flex flex-col items-center justify-center select-none px-6'>
            <img
              src={logoUrl}
              alt=''
              style={{ width: 80, height: 80, opacity: 0.75 }}
              className='mb-4'
            />
            <Typography.Text type='tertiary' className='text-sm'>
              {t('开始生成图片')}
            </Typography.Text>
          </div>
        ) : (
          <div className='w-full mx-auto px-4 py-4' style={{ maxWidth: 860 }}>
            {generations.map((g) => (
              <GenerationCard
                key={g.id}
                generation={g}
                t={t}
                onCopyPrompt={handleCopyPrompt}
                onDelete={onDeleteGeneration}
                onContinueEdit={handleContinueEdit}
                supportsContinueEdit={supportsImageInput}
              />
            ))}
          </div>
        )}
      </div>

      {/* 输入区：与 chat 输入同款胶囊样式 + max-width 居中 */}
      <div
        className='flex-shrink-0 w-full mx-auto px-4 pb-4'
        style={{ maxWidth: 860 }}
      >
        <div
          className='rounded-2xl transition-colors focus-within:ring-2 focus-within:ring-offset-0'
          style={{
            border: '1px solid var(--semi-color-border)',
            background: 'var(--semi-color-bg-0)',
            ['--tw-ring-color']:
              'var(--semi-color-primary-light-hover, rgba(129,140,248,0.35))',
          }}
        >
          {/* 附件区：模型 schema 声明了 format:image 输入槽时才渲染。
              默认折叠成一条细条（只占一行），减少空态时的占位。点击或
              首次添加附件时自动展开；发送后 inputs 被清空自动折叠回去。 */}
          {supportsImageInput && (
            <div
              style={{
                borderBottom: inputsExpanded
                  ? '1px solid var(--semi-color-fill-0)'
                  : 'none',
              }}
            >
              <div
                role='button'
                tabIndex={0}
                onClick={() => setInputsExpanded((v) => !v)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setInputsExpanded((v) => !v);
                  }
                }}
                className='flex items-center gap-2 px-3 py-1.5 cursor-pointer select-none transition-colors'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                <Paperclip size={13} />
                <Typography.Text type='tertiary' className='text-xs'>
                  {t('参考图')}
                </Typography.Text>
                {inputsFileCount > 0 && (
                  <Tag size='small' shape='circle' color='blue'>
                    {inputsFileCount}
                  </Tag>
                )}
                <span style={{ marginLeft: 'auto', display: 'flex' }}>
                  {inputsExpanded ? (
                    <ChevronUp size={13} />
                  ) : (
                    <ChevronDown size={13} />
                  )}
                </span>
              </div>
              {inputsExpanded && (
                <div className='px-3 pb-2'>
                  <SchemaInputsRenderer
                    schema={inputsSchema}
                    values={inputsValues || {}}
                    onChange={onInputsChange}
                    disabled={loading}
                  />
                </div>
              )}
            </div>
          )}
          {/* 用原生 textarea + 手动 auto-grow。之前 Semi <TextArea> 的
              autosize 在初次挂载时偶发不测量，语言切换等再渲染才纠回，
              这里直接用 DOM，时序可控、视觉一致。 */}
          <textarea
            ref={textareaRef}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={t('描述你想要的图片…')}
            rows={1}
            disabled={loading}
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
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
              padding: '12px 14px 6px',
              fontSize: 14,
              lineHeight: 1.5,
              minHeight: 44,
              maxHeight: 192,
              color: 'var(--semi-color-text-0)',
              fontFamily: 'inherit',
            }}
          />
          <div className='flex items-center justify-between px-2 pb-2'>
            <Typography.Text
              type='tertiary'
              className='text-xs select-none'
              style={{ paddingLeft: 4 }}
            >
              {loading ? t('生成中…') : t('⌘/Ctrl + Enter 发送')}
            </Typography.Text>
            <Button
              theme='solid'
              type='primary'
              icon={<ArrowUp size={18} strokeWidth={2.5} />}
              onClick={handleSubmit}
              loading={loading}
              disabled={!prompt.trim()}
              className='!rounded-full'
              style={{
                width: 36,
                height: 36,
                minWidth: 36,
                padding: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
              aria-label={t('生成')}
            />
          </div>
        </div>
      </div>

    </Card>
  );
};

export default ImageWorkspace;
