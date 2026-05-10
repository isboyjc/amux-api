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

// probeDuration：用 HTMLVideoElement / HTMLAudioElement 的 loadedmetadata
// 事件读取本地文件的时长（秒）。仅探测元数据，不下载/解码完整内容，体感几
// 十毫秒级别。
//
// 返回：
//   - 成功 → number（>0 的有限秒数）
//   - 元数据不可读 / 文件损坏 / 超时 / 类型不匹配 → null（调用方按需 fallback）
//
// 设计点：
//   - 仅吃 File 对象（前端拿到的就是 File）。URL 远端文件不在这里支持——
//     拿不到 file.size 也无法兜超时，留给上游服务校验
//   - 元素 + ObjectURL 用完立刻 revoke，避免 long-lived blob URL 累积
//   - 默认 8 秒超时：metadata 通常 <1s 完成，超时往往意味着文件损坏 /
//     编码不被浏览器识别，后续调用方应该当作"未知时长"处理
export const probeDuration = (file, { timeoutMs = 8000 } = {}) => {
  if (!(file instanceof File)) return Promise.resolve(null);
  const tp = (file.type || '').toLowerCase();
  const isAudio = tp.startsWith('audio/');
  const isVideo = tp.startsWith('video/');
  if (!isAudio && !isVideo) return Promise.resolve(null);

  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const el = document.createElement(isAudio ? 'audio' : 'video');
    let done = false;
    const cleanup = () => {
      if (done) return;
      done = true;
      try {
        el.removeAttribute('src');
        el.load();
      } catch {}
      try {
        URL.revokeObjectURL(url);
      } catch {}
    };
    const finish = (val) => {
      const ok = typeof val === 'number' && Number.isFinite(val) && val > 0;
      cleanup();
      resolve(ok ? val : null);
    };

    el.preload = 'metadata';
    // 必须 muted——video 元素带音轨时 autoplay/load 在某些浏览器下会被
    // 拦截。我们只读 metadata 不播放，但 muted 多一道保险
    el.muted = true;

    const timer = window.setTimeout(() => finish(null), timeoutMs);
    el.onloadedmetadata = () => {
      clearTimeout(timer);
      finish(el.duration);
    };
    el.onerror = () => {
      clearTimeout(timer);
      finish(null);
    };

    el.src = url;
  });
};
