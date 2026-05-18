/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

import React, { useRef } from 'react';
import { Button, Tag, Tooltip } from '@douyinfe/semi-ui';
import {
  Image as ImageIcon,
  Music,
  Paperclip,
  Video as VideoIcon,
  X,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { uploadToR2 } from '../../helpers/upload';
import { showError } from '../../helpers/utils';

const DEFAULT_MAX = 6;

/** 根据 MIME 选 R2 scope，决定走哪个 ticket-attachment 桶/上限。 */
export function pickTicketAttachmentScope(file) {
  const ct = (file?.type || '').toLowerCase();
  if (ct.startsWith('image/')) return 'ticket-attachment-image';
  if (ct.startsWith('video/')) return 'ticket-attachment-video';
  if (ct.startsWith('audio/')) return 'ticket-attachment-audio';
  return 'ticket-attachment-file';
}

function iconFor(ct = '') {
  if (ct.startsWith('image/')) return <ImageIcon size={14} />;
  if (ct.startsWith('video/')) return <VideoIcon size={14} />;
  if (ct.startsWith('audio/')) return <Music size={14} />;
  return <Paperclip size={14} />;
}

/**
 * 工单附件上传器。
 *
 * 用法：受控组件，value/onChange 维护 TicketAttachment 数组。
 * 每个数组元素 shape：{ url, filename, content_type, size }，与后端 dto 对齐。
 *
 * 简化考虑：
 *   - 不用 Semi Upload。Semi Upload 自带的列表/状态机和我们后端 R2 预签名
 *     直传不易对齐；直接 <input type="file"> + uploadToR2 直观可控。
 *   - 不支持拖拽。v1 价值不高，需要时后续加。
 */
export function TicketAttachmentsUploader({
  value = [],
  onChange,
  max = DEFAULT_MAX,
  disabled = false,
}) {
  const { t } = useTranslation();
  const inputRef = useRef(null);
  const uploadingRef = useRef(0);
  const [uploadingCount, setUploadingCount] = React.useState(0);

  const remaining = Math.max(0, max - value.length);
  const canUpload = !disabled && remaining > 0;

  const handleFiles = async (files) => {
    if (!files || files.length === 0) return;
    const list = Array.from(files).slice(0, remaining);
    // 当前组件内的并发计数；上传期间禁用"添加"按钮，避免超过 max。
    uploadingRef.current += list.length;
    setUploadingCount(uploadingRef.current);

    const next = [...value];
    await Promise.all(
      list.map(async (f) => {
        try {
          const scope = pickTicketAttachmentScope(f);
          const res = await uploadToR2(f, scope);
          next.push({
            url: res.url,
            filename: f.name || 'attachment',
            content_type: res.content_type,
            size: res.size,
          });
        } catch (e) {
          showError(e?.message || t('附件上传失败'));
        } finally {
          uploadingRef.current -= 1;
          setUploadingCount(uploadingRef.current);
        }
      }),
    );
    onChange?.(next);
  };

  const remove = (idx) => {
    const next = value.filter((_, i) => i !== idx);
    onChange?.(next);
  };

  return (
    <div className='flex flex-col gap-2'>
      <div className='flex items-center gap-2'>
        <Button
          size='small'
          theme='light'
          icon={<Paperclip size={14} />}
          disabled={!canUpload || uploadingCount > 0}
          loading={uploadingCount > 0}
          onClick={() => inputRef.current?.click()}
        >
          {t('添加附件')} ({value.length}/{max})
        </Button>
        <input
          ref={inputRef}
          type='file'
          multiple
          hidden
          onChange={(e) => {
            handleFiles(e.target.files);
            // 重置以便同名文件可以再次选择
            e.target.value = '';
          }}
        />
      </div>
      {value.length > 0 && (
        <div className='flex flex-wrap gap-2'>
          {value.map((a, idx) => (
            <Tooltip key={`${a.url}-${idx}`} content={a.url}>
              <Tag
                closable
                onClose={(e) => {
                  e?.stopPropagation?.();
                  remove(idx);
                }}
                size='large'
                color='white'
                prefixIcon={iconFor(a.content_type)}
                className='!cursor-default max-w-[260px]'
              >
                <span className='truncate inline-block max-w-[200px] align-middle'>
                  {a.filename}
                </span>
              </Tag>
            </Tooltip>
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * 附件展示组件（消息流里用，只读）。图片缩略 + 行内预览，其他附件展示
 * 文件名 + 下载链接。点击图片在新标签页打开原图（v1 不上 lightbox）。
 */
export function TicketAttachmentsView({ attachments = [] }) {
  if (!attachments?.length) return null;
  return (
    <div className='flex flex-wrap gap-2 mt-2'>
      {attachments.map((a, idx) => {
        const ct = a.content_type || '';
        if (ct.startsWith('image/')) {
          return (
            <a
              key={`${a.url}-${idx}`}
              href={a.url}
              target='_blank'
              rel='noopener noreferrer'
              className='block rounded overflow-hidden border border-[var(--semi-color-border)] hover:opacity-80 transition-opacity'
            >
              <img
                src={a.url}
                alt={a.filename || 'attachment'}
                style={{ maxHeight: 160, maxWidth: 240, display: 'block' }}
              />
            </a>
          );
        }
        return (
          <a
            key={`${a.url}-${idx}`}
            href={a.url}
            target='_blank'
            rel='noopener noreferrer'
            className='inline-flex items-center gap-1.5 px-2 py-1 rounded border border-[var(--semi-color-border)] text-xs hover:bg-[var(--semi-color-fill-0)] transition-colors'
          >
            {iconFor(ct)}
            <span className='truncate max-w-[200px]'>
              {a.filename || a.url}
            </span>
          </a>
        );
      })}
    </div>
  );
}
