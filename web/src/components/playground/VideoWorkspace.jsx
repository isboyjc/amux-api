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

import React, { useCallback, useMemo, useRef, useState, useEffect } from 'react';
import {
  Button,
  Card,
  Input,
  Progress,
  Select,
  Spin,
  Tag,
  Tooltip,
  Typography,
  Popover,
} from '@douyinfe/semi-ui';
import {
  ArrowUp,
  Copy,
  Eye,
  EyeOff,
  Film,
  Image as ImageIcon,
  Music,
  Paperclip,
  Plus,
  Trash2,
  X,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { MESSAGE_ROLES } from '../../constants/playground.constants';
import { getLogo, showError, showSuccess, uploadToR2 } from '../../helpers';
import { useActualTheme } from '../../context/Theme';
import { buildDefaultPlaygroundLogo } from './workspaceLogo';

// 视频消息的 content 形如 [{ type:'video_url', video_url:{url} }]。
// 把它提取为 { url, lastFrameUrl? }。
const extractVideo = (assistantMessage) => {
  const c = assistantMessage?.content;
  if (!Array.isArray(c)) return null;
  const v = c.find((p) => p?.type === 'video_url' && p.video_url?.url);
  if (!v) return null;
  const last = c.find((p) => p?.type === 'image_url' && p.image_url?.url);
  return { url: v.video_url.url, lastFrameUrl: last?.image_url?.url };
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

// 一次生成 = 一条 user + 一条 assistant。和 ImageWorkspace 一致。
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

const formatRelativeTime = (ts, t) => {
  if (!ts) return '';
  const now = Date.now();
  const diff = now - ts;
  if (diff < 60 * 1000) return t('刚刚');
  if (diff < 60 * 60 * 1000) return t('{{n}} 分钟前', { n: Math.floor(diff / 60000) });
  return new Date(ts).toLocaleString();
};

// 用于预览的附件列表——来自 user 消息的 attachments 字段（见 Playground
// handleGenerateVideo 里的塞入）。
const renderAttachmentBadges = (attachments, t) => {
  if (!Array.isArray(attachments) || attachments.length === 0) return null;
  return (
    <div className='flex items-center gap-1 flex-wrap mt-1'>
      {attachments.map((a, i) => {
        const Icon =
          a.type === 'image_url'
            ? ImageIcon
            : a.type === 'video_url'
              ? Film
              : Music;
        // 图片类型附件在 role 已知时，展示本地化 role 名（"图片 · 首帧"）
        const roleLabel = a.type === 'image_url' && a.role ? getImageRoleLabel(t, a.role) : '';
        const label =
          a.type === 'image_url'
            ? t('图片') + (roleLabel ? ` · ${roleLabel}` : '')
            : a.type === 'video_url'
              ? t('视频')
              : t('音频');
        return (
          <span
            key={i}
            className='inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full text-xs'
            style={{
              background: 'var(--semi-color-fill-1)',
              color: 'var(--semi-color-text-2)',
            }}
          >
            <Icon size={12} />
            {label}
          </span>
        );
      })}
    </div>
  );
};

const GenerationCard = ({ generation, onCopyPrompt, onDelete, t }) => {
  const prompt = getPromptText(generation.promptMessage);
  const assistant = generation.assistantMessage || {};
  const status = assistant.status || 'loading';
  const progress = typeof assistant.progress === 'number' ? assistant.progress : 0;
  const video = extractVideo(assistant);
  const meta = assistant.meta || {};
  const ts = assistant.createAt || generation.promptMessage?.createAt;
  const attachments = generation.promptMessage?.attachments;

  const metaPieces = [];
  if (meta.model) metaPieces.push({ tag: true, text: meta.model });
  const p = meta.params || {};
  ['resolution', 'ratio', 'duration', 'seed'].forEach((k) => {
    if (p[k] !== undefined && p[k] !== '' && p[k] !== -1) {
      metaPieces.push({ text: `${k}=${p[k]}` });
    }
  });
  if (ts) metaPieces.push({ text: formatRelativeTime(ts, t) });

  const isLoading = status === 'loading' || status === 'polling';
  const isError = status === 'error';

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
                  <Typography.Text key={i} type='tertiary' className='text-xs'>
                    {i > 0 ? '· ' : ''}
                    {m.text}
                  </Typography.Text>
                ),
              )}
            </div>
          )}
          {renderAttachmentBadges(attachments, t)}
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

      {isLoading && (
        <div className='flex flex-col items-center justify-center py-10 gap-3 px-4'>
          <Spin size='middle' />
          <Typography.Text type='tertiary' className='text-sm'>
            {status === 'polling' ? t('生成中…') : t('正在提交…')}
          </Typography.Text>
          {progress > 0 && (
            <div className='w-full max-w-xs'>
              <Progress percent={progress} size='small' showInfo={false} />
            </div>
          )}
        </div>
      )}

      {isError && (
        <div className='px-4 py-4 flex items-center gap-2'>
          <X size={16} style={{ color: 'var(--semi-color-danger)', flexShrink: 0 }} />
          <Typography.Text type='danger' className='text-sm'>
            {assistant.errorMessage || t('视频生成失败')}
          </Typography.Text>
        </div>
      )}

      {!isLoading && !isError && video && (
        <div className='p-2'>
          <div className='flex justify-center'>
            <div
              className='rounded-lg overflow-hidden'
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
                  maxHeight: 560,
                  width: 'auto',
                  height: 'auto',
                }}
              >
                {t('浏览器不支持播放视频')}
              </video>
            </div>
          </div>
          {video.lastFrameUrl && (
            <div className='mt-2 flex justify-center'>
              <Tooltip content={t('末帧预览')}>
                <img
                  src={video.lastFrameUrl}
                  alt={t('末帧')}
                  style={{
                    maxWidth: 160,
                    maxHeight: 160,
                    borderRadius: 6,
                    opacity: 0.9,
                  }}
                />
              </Tooltip>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// ===== 附件抽屉 =====
//
// 支持三类附件：
//  - 图片：本地选择（base64 data URL），每张可选 role（reference_image /
//    first_frame / last_frame）。最多 9 张。
//  - 视频：URL 或 asset:// 直链——官方明确不支持 base64。最多 3 条。
//  - 音频：URL 或 base64 均可；此处只接 URL，够常用场景。最多 3 条。
//
// 校验在提交前跑（见 validateAttachments）。

// 三类图片 role 的 i18n 展示名。**不要**把字面量 Chinese 写进数组里再用
// t(r.label) 读取——那是动态调用，i18next-cli 的静态提取识别不到。这里
// 用 switch 里的字面量 t('...') 调用，保证提取工具能收进 locales。
const IMAGE_ROLES = [
  { value: 'reference_image' },
  { value: 'first_frame' },
  { value: 'last_frame' },
];

const getImageRoleLabel = (t, role) => {
  switch (role) {
    case 'reference_image':
      return t('参考图');
    case 'first_frame':
      return t('首帧');
    case 'last_frame':
      return t('末帧');
    default:
      return role || '';
  }
};

const MAX_IMAGES = 9;
const MAX_VIDEOS = 3;
const MAX_AUDIOS = 3;

const AttachmentsPanel = ({
  attachments,
  setAttachments,
  disabled,
  t,
}) => {
  const fileInputRef = useRef(null);
  const videoFileInputRef = useRef(null);
  const audioFileInputRef = useRef(null);
  const counts = useMemo(() => {
    const c = { image: 0, video: 0, audio: 0 };
    attachments.forEach((a) => {
      if (a.type === 'image_url') c.image++;
      else if (a.type === 'video_url') c.video++;
      else if (a.type === 'audio_url') c.audio++;
    });
    return c;
  }, [attachments]);

  const [videoUrl, setVideoUrl] = useState('');
  const [audioUrl, setAudioUrl] = useState('');
  // 上传中标记，多类型同时按各自 type 区分；防止用户重复点上传
  const [uploading, setUploading] = useState({
    image: false,
    video: false,
    audio: false,
  });

  // 图片：选完文件 → 逐张上传到 R2 → 拿回 https URL 落进 attachments。
  // 单张失败不阻断后续，仅 toast。R2 失败极端兜底是空 attachments，
  // 由提交侧的 hasContent 判断决定要不要发送。
  const handleAddImages = async (files) => {
    if (!files || files.length === 0) return;
    const left = MAX_IMAGES - counts.image;
    if (left <= 0) {
      showError(t('图片数量已达上限'));
      return;
    }
    const picked = Array.from(files).slice(0, left);
    setUploading((s) => ({ ...s, image: true }));
    const items = [];
    for (const f of picked) {
      if (f.size > 30 * 1024 * 1024) {
        showError(t('单张图片不能超过 30MB'));
        continue;
      }
      try {
        const r = await uploadToR2(f, 'playground-video-image');
        items.push({
          type: 'image_url',
          image_url: { url: r.url },
          role: 'reference_image',
        });
      } catch (err) {
        showError(t('上传图片失败：') + (err?.message || ''));
      }
    }
    setUploading((s) => ({ ...s, image: false }));
    if (items.length > 0) setAttachments([...attachments, ...items]);
  };

  const handleAddVideoUrl = () => {
    const url = videoUrl.trim();
    if (!url) return;
    if (counts.video >= MAX_VIDEOS) {
      showError(t('视频数量已达上限'));
      return;
    }
    if (url.startsWith('data:')) {
      showError(t('视频不支持 base64，请使用直链 URL'));
      return;
    }
    setAttachments([
      ...attachments,
      {
        type: 'video_url',
        video_url: { url },
        role: 'reference_video',
      },
    ]);
    setVideoUrl('');
  };

  // 视频文件上传：火山引擎对视频字段只接 URL（不接 base64），所以走 R2
  // 落盘后回写 https URL。视频普遍大，超时给宽一点（uploadToR2 默认已 5min）。
  const handleAddVideoFile = async (files) => {
    if (!files || files.length === 0) return;
    const left = MAX_VIDEOS - counts.video;
    if (left <= 0) {
      showError(t('视频数量已达上限'));
      return;
    }
    const picked = Array.from(files).slice(0, left);
    setUploading((s) => ({ ...s, video: true }));
    const items = [];
    for (const f of picked) {
      try {
        const r = await uploadToR2(f, 'playground-video-video');
        items.push({
          type: 'video_url',
          video_url: { url: r.url },
          role: 'reference_video',
        });
      } catch (err) {
        showError(t('上传视频失败：') + (err?.message || ''));
      }
    }
    setUploading((s) => ({ ...s, video: false }));
    if (items.length > 0) setAttachments([...attachments, ...items]);
  };

  const handleAddAudioUrl = () => {
    const url = audioUrl.trim();
    if (!url) return;
    if (counts.audio >= MAX_AUDIOS) {
      showError(t('音频数量已达上限'));
      return;
    }
    setAttachments([
      ...attachments,
      {
        type: 'audio_url',
        audio_url: { url },
        role: 'reference_audio',
      },
    ]);
    setAudioUrl('');
  };

  const handleAddAudioFile = async (files) => {
    if (!files || files.length === 0) return;
    const left = MAX_AUDIOS - counts.audio;
    if (left <= 0) {
      showError(t('音频数量已达上限'));
      return;
    }
    const picked = Array.from(files).slice(0, left);
    setUploading((s) => ({ ...s, audio: true }));
    const items = [];
    for (const f of picked) {
      try {
        const r = await uploadToR2(f, 'playground-video-audio');
        items.push({
          type: 'audio_url',
          audio_url: { url: r.url },
          role: 'reference_audio',
        });
      } catch (err) {
        showError(t('上传音频失败：') + (err?.message || ''));
      }
    }
    setUploading((s) => ({ ...s, audio: false }));
    if (items.length > 0) setAttachments([...attachments, ...items]);
  };

  const removeAt = (idx) => {
    const next = attachments.slice();
    next.splice(idx, 1);
    setAttachments(next);
  };

  const updateImageRole = (idx, role) => {
    const next = attachments.slice();
    next[idx] = { ...next[idx], role };
    setAttachments(next);
  };

  return (
    <div className='p-3 w-80'>
      <Typography.Text strong className='text-sm block mb-2'>
        {t('图片')}（{counts.image}/{MAX_IMAGES}）
      </Typography.Text>
      <div className='flex gap-2 mb-2'>
        <Button
          icon={<Plus size={14} />}
          size='small'
          loading={uploading.image}
          disabled={
            disabled || uploading.image || counts.image >= MAX_IMAGES
          }
          onClick={() => fileInputRef.current?.click()}
        >
          {uploading.image ? t('上传中…') : t('选择图片')}
        </Button>
        <input
          ref={fileInputRef}
          type='file'
          accept='image/*'
          multiple
          hidden
          onChange={(e) => {
            handleAddImages(e.target.files);
            e.target.value = '';
          }}
        />
      </div>
      {attachments.filter((a) => a.type === 'image_url').length > 0 && (
        <div className='space-y-1 mb-3'>
          {attachments.map((a, i) => {
            if (a.type !== 'image_url') return null;
            return (
              <div
                key={i}
                className='flex items-center gap-2 p-1 rounded'
                style={{ background: 'var(--semi-color-fill-0)' }}
              >
                <img
                  src={a.image_url.url}
                  alt=''
                  style={{
                    width: 32,
                    height: 32,
                    borderRadius: 4,
                    objectFit: 'cover',
                  }}
                />
                <Select
                  size='small'
                  value={a.role}
                  onChange={(v) => updateImageRole(i, v)}
                  optionList={IMAGE_ROLES.map((r) => ({
                    label: getImageRoleLabel(t, r.value),
                    value: r.value,
                  }))}
                  style={{ flex: 1 }}
                />
                <Button
                  icon={<X size={12} />}
                  size='small'
                  theme='borderless'
                  onClick={() => removeAt(i)}
                />
              </div>
            );
          })}
        </div>
      )}

      <Typography.Text strong className='text-sm block mb-2'>
        {t('视频')}（{counts.video}/{MAX_VIDEOS}）
      </Typography.Text>
      {/* 两条入口：本地选择文件（走 R2）+ 已有直链。文件选完触发 onChange，
          完成后会把对应 https URL 自动追加进 attachments；URL 输入则一直
          是手填粘贴模式，免得改动用户已习惯的工作流。 */}
      <div className='flex gap-2 mb-2'>
        <Button
          icon={<Plus size={14} />}
          size='small'
          loading={uploading.video}
          disabled={
            disabled || uploading.video || counts.video >= MAX_VIDEOS
          }
          onClick={() => videoFileInputRef.current?.click()}
        >
          {uploading.video ? t('上传中…') : t('选择视频')}
        </Button>
        <input
          ref={videoFileInputRef}
          type='file'
          accept='video/*'
          hidden
          onChange={(e) => {
            handleAddVideoFile(e.target.files);
            e.target.value = '';
          }}
        />
      </div>
      <div className='flex gap-1 mb-2'>
        <Input
          size='small'
          placeholder={t('或粘贴视频直链 URL')}
          value={videoUrl}
          onChange={setVideoUrl}
          disabled={disabled || counts.video >= MAX_VIDEOS}
        />
        <Button
          icon={<Plus size={14} />}
          size='small'
          onClick={handleAddVideoUrl}
          disabled={disabled || !videoUrl.trim() || counts.video >= MAX_VIDEOS}
        />
      </div>
      {attachments.filter((a) => a.type === 'video_url').map((a, idxReal) => {
        const i = attachments.indexOf(a);
        return (
          <div
            key={i}
            className='flex items-center gap-2 p-1 rounded mb-1'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <Film size={14} />
            <Typography.Text
              ellipsis={{ rows: 1, showTooltip: { opts: { content: a.video_url.url } } }}
              className='text-xs flex-1 min-w-0'
            >
              {a.video_url.url}
            </Typography.Text>
            <Button
              icon={<X size={12} />}
              size='small'
              theme='borderless'
              onClick={() => removeAt(i)}
            />
          </div>
        );
      })}

      <Typography.Text strong className='text-sm block mb-2 mt-1'>
        {t('音频')}（{counts.audio}/{MAX_AUDIOS}）
      </Typography.Text>
      <div className='flex gap-2 mb-2'>
        <Button
          icon={<Plus size={14} />}
          size='small'
          loading={uploading.audio}
          disabled={
            disabled || uploading.audio || counts.audio >= MAX_AUDIOS
          }
          onClick={() => audioFileInputRef.current?.click()}
        >
          {uploading.audio ? t('上传中…') : t('选择音频')}
        </Button>
        <input
          ref={audioFileInputRef}
          type='file'
          accept='audio/*'
          hidden
          onChange={(e) => {
            handleAddAudioFile(e.target.files);
            e.target.value = '';
          }}
        />
      </div>
      <div className='flex gap-1 mb-2'>
        <Input
          size='small'
          placeholder={t('或粘贴音频 URL')}
          value={audioUrl}
          onChange={setAudioUrl}
          disabled={disabled || counts.audio >= MAX_AUDIOS}
        />
        <Button
          icon={<Plus size={14} />}
          size='small'
          onClick={handleAddAudioUrl}
          disabled={disabled || !audioUrl.trim() || counts.audio >= MAX_AUDIOS}
        />
      </div>
      {attachments.filter((a) => a.type === 'audio_url').map((a) => {
        const i = attachments.indexOf(a);
        return (
          <div
            key={i}
            className='flex items-center gap-2 p-1 rounded mb-1'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <Music size={14} />
            <Typography.Text
              ellipsis={{ rows: 1, showTooltip: { opts: { content: a.audio_url.url } } }}
              className='text-xs flex-1 min-w-0'
            >
              {a.audio_url.url}
            </Typography.Text>
            <Button
              icon={<X size={12} />}
              size='small'
              theme='borderless'
              onClick={() => removeAt(i)}
            />
          </div>
        );
      })}

      <Typography.Text type='tertiary' className='text-xs block mt-2'>
        {t('规则：音频不能单独输入；首帧/末帧与参考图互斥。')}
      </Typography.Text>
    </div>
  );
};

// 运行时附件校验，匹配火山官方的硬约束。
const validateAttachments = (atts, t) => {
  if (!Array.isArray(atts) || atts.length === 0) return null;
  const images = atts.filter((a) => a.type === 'image_url');
  const videos = atts.filter((a) => a.type === 'video_url');
  const audios = atts.filter((a) => a.type === 'audio_url');

  // audio 不能单独输入
  if (audios.length > 0 && images.length === 0 && videos.length === 0) {
    return t('音频不能单独输入，请配合图片或视频一起上传');
  }

  // first/last_frame 与 reference_image 互斥
  const hasFrame = images.some(
    (a) => a.role === 'first_frame' || a.role === 'last_frame',
  );
  const hasRef = images.some((a) => a.role === 'reference_image');
  if (hasFrame && hasRef) {
    return t('首帧/末帧 与 参考图 不能同时使用');
  }

  return null;
};

const VideoWorkspace = ({
  message = [],
  inputs,
  styleState,
  onGenerate,
  loading = false,
  onDeleteGeneration,
  onClearAll,
  showDebugPanel,
  onToggleDebugPanel,
}) => {
  const { t } = useTranslation();
  const [prompt, setPrompt] = useState('');
  const [attachments, setAttachments] = useState([]);
  const [attachOpen, setAttachOpen] = useState(false);
  const textareaRef = useRef(null);

  const generations = useMemo(() => groupIntoGenerations(message), [message]);
  const hasResults = generations.length > 0;
  const actualTheme = useActualTheme();
  const logoUrl = getLogo() || buildDefaultPlaygroundLogo(actualTheme === 'dark');

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

  const handleSubmit = useCallback(async () => {
    if (!prompt.trim() || loading) return;
    const err = validateAttachments(attachments, t);
    if (err) {
      showError(err);
      return;
    }
    const text = prompt;
    const atts = attachments.slice();
    setPrompt('');
    setAttachments([]);
    await onGenerate?.({ prompt: text, attachments: atts });
  }, [prompt, attachments, loading, onGenerate, t]);

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
      {styleState?.isMobile ? (
        <div className='pt-4'></div>
      ) : (
        <div className='px-6 py-4 bg-gradient-to-r from-purple-500 to-blue-500 rounded-t-2xl'>
          <div className='flex items-center justify-between'>
            <div className='flex flex-col min-w-0'>
              <Typography.Title heading={5} className='!text-white mb-0'>
                {t('视频生成')}
              </Typography.Title>
              <Typography.Text className='!text-white/80 text-sm hidden sm:inline truncate'>
                {inputs?.model || t('选择视频模型开始生成')}
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
                  icon={showDebugPanel ? <EyeOff size={14} /> : <Eye size={14} />}
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
              {t('开始生成视频')}
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
              />
            ))}
          </div>
        )}
      </div>

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
          <textarea
            ref={textareaRef}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={t('描述你想要的视频…')}
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
            <div className='flex items-center gap-1'>
              <Popover
                trigger='click'
                visible={attachOpen}
                onVisibleChange={setAttachOpen}
                content={
                  <AttachmentsPanel
                    attachments={attachments}
                    setAttachments={setAttachments}
                    disabled={loading}
                    t={t}
                  />
                }
                position='topLeft'
              >
                <Button
                  icon={<Paperclip size={16} />}
                  size='small'
                  theme='borderless'
                  type={attachments.length > 0 ? 'primary' : 'tertiary'}
                  style={{ borderRadius: 8 }}
                >
                  {attachments.length > 0 ? `${attachments.length}` : ''}
                </Button>
              </Popover>
              <Typography.Text
                type='tertiary'
                className='text-xs select-none'
                style={{ marginLeft: 4 }}
              >
                {loading ? t('提交/生成中…') : t('⌘/Ctrl + Enter 发送')}
              </Typography.Text>
            </div>
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

export default VideoWorkspace;
