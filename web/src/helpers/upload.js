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

import { API } from './api';

// 通用对象存储（Cloudflare R2）上传 helper。前端无论谁要上传文件——操练场
// 参考图、视频/音频素材、未来的头像等——都走这一个入口。
//
// 与旧版（multipart 走代理）的差异：
//   - 走"两步预签名直传"：先 POST /api/upload/presign 拿到 R2 PUT URL，
//     再用 XHR 直接 PUT 到 R2。文件流不经过 amux-api，省后端带宽，对大
//     文件友好；签名把 Content-Type / Content-Length 一起签进去，客户端
//     不能撒谎换文件
//   - 调用方接口完全不变：仍然返回 { url, key, size, content_type }，
//     onProgress / signal 行为一致
//
// 失败语义：抛 Error（带后端 / R2 message 优先）；调用方自己 try/catch
// 决定是走 Toast 还是回退。这里不调 showError，让上层有时机做"失败重试 / 回退
// data URL"之类的策略。
//
// scope 白名单（必须与后端 controller/upload.go 里的 allowedUploadScopes 同步）：
//   playground 内部:
//     - playground-video-image
//     - playground-video-video
//     - playground-video-audio
//     - playground-image-reference
//   通用上传（对外 API 也可用）:
//     - user-upload-image
//     - user-upload-video
//     - user-upload-audio
//     - user-upload-file

const PRESIGN_ENDPOINT = '/api/upload/presign';

/**
 * @param {File|Blob} file 要上传的文件
 * @param {string} scope 业务域（必须在后端白名单里）
 * @param {{ onProgress?: (percent:number)=>void, signal?: AbortSignal }} [opts]
 * @returns {Promise<{ url: string, key: string, size: number, content_type: string }>}
 */
export async function uploadToR2(file, scope, opts = {}) {
  if (!(file instanceof File) && !(file instanceof Blob)) {
    throw new Error('uploadToR2: file 参数必须是 File / Blob');
  }
  if (!scope || typeof scope !== 'string') {
    throw new Error('uploadToR2: 缺少 scope');
  }
  if (opts.signal?.aborted) {
    throw new DOMException('Aborted', 'AbortError');
  }

  // ---- Step 1: 向 amux-api 申请预签名 URL ----
  const filename = file instanceof File && file.name ? file.name : 'upload.bin';
  const contentType = file.type || 'application/octet-stream';

  const presignRes = await API.post(
    PRESIGN_ENDPOINT,
    {
      scope,
      filename,
      size: file.size,
      content_type: contentType,
    },
    {
      signal: opts.signal,
      // 业务错误自己 throw，避免全局 showError 抢解释权
      skipErrorHandler: true,
    },
  );

  const presignBody = presignRes?.data;
  if (!presignBody?.success || !presignBody?.data?.upload_url) {
    throw new Error(presignBody?.message || '获取上传签名失败');
  }
  const presign = presignBody.data;

  // ---- Step 2: 浏览器直接 PUT 到 R2，XHR 才能拿 upload progress ----
  await putToR2(file, presign, opts);

  return {
    url: presign.public_url,
    key: presign.key,
    size: presign.size,
    content_type: presign.content_type,
  };
}

// putToR2 用 XMLHttpRequest（不是 fetch）实现，因为只有 xhr.upload.onprogress
// 能给上传进度；fetch 的 Request body 没有进度事件。
//
// 失败要点：
//   - 网络层失败（断网 / DNS）走 onerror
//   - HTTP 4xx/5xx 走 onload 但 status 不在 2xx——R2 的错误体是 XML，不强行解析，
//     直接把 status + statusText 抛回去就够定位了
//   - AbortSignal 透传到 xhr.abort()，调用方可主动取消
function putToR2(file, presign, opts) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(presign.method || 'PUT', presign.upload_url, true);

    // 把后端返回的签名 header 原样写回（一般至少含 Content-Type）。
    // Host / Content-Length 服务端已经在过滤，这里不会出现；浏览器自己带。
    if (presign.headers && typeof presign.headers === 'object') {
      for (const [k, v] of Object.entries(presign.headers)) {
        if (typeof v === 'string') {
          xhr.setRequestHeader(k, v);
        }
      }
    }

    if (typeof opts.onProgress === 'function') {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && e.total) {
          opts.onProgress(Math.round((e.loaded / e.total) * 100));
        }
      };
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        // 进度事件 100% 不一定准时触发 onprogress（部分浏览器只到 99）；
        // onload 时手动补一次 100，UI 不会卡在 99
        if (typeof opts.onProgress === 'function') {
          try {
            opts.onProgress(100);
          } catch {
            /* swallow */
          }
        }
        resolve();
      } else {
        reject(
          new Error(
            `上传到对象存储失败 (HTTP ${xhr.status}${
              xhr.statusText ? ' ' + xhr.statusText : ''
            })`,
          ),
        );
      }
    };
    xhr.onerror = () => reject(new Error('网络错误，上传失败'));
    xhr.ontimeout = () => reject(new Error('上传超时'));
    xhr.onabort = () =>
      reject(new DOMException('Aborted', 'AbortError'));

    // 大文件慢网下走默认的 0（无超时）；R2 自己也不会无限挂着
    xhr.timeout = 0;

    if (opts.signal) {
      const onAbort = () => {
        try {
          xhr.abort();
        } catch {
          /* swallow */
        }
      };
      if (opts.signal.aborted) {
        onAbort();
        return;
      }
      opts.signal.addEventListener('abort', onAbort, { once: true });
    }

    xhr.send(file);
  });
}

// 上传成功后还需要把文件名 / 大小回放到 UI 时常用——再开一个轻量 helper，
// 顺手返回原始 file metadata（File API 自带的 name、size、type），调用方
// 不用自己合并字段。
export async function uploadToR2WithMeta(file, scope, opts) {
  const data = await uploadToR2(file, scope, opts);
  return {
    ...data,
    name: file.name || '',
    originalSize: file.size,
    originalType: file.type,
  };
}
