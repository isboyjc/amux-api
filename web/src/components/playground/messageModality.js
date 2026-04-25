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

import { MODALITY } from '../../constants/playground.constants';

// 单条消息的「实际形态」。统一对话窗口里用它决定渲染哪种气泡：
//   text / multimodal -> MessageContent（聊天气泡）
//   image             -> ImageBubble（图片卡）
//   video             -> VideoBubble（视频播放器）
//
// 优先用消息上显式标注的 modality 字段（新流程在创建消息时就会写）。
// 老消息没有这个字段，按 content 形态推断：包含 video_url part 的视为视频，
// 含 image_url 数组且没有可见文本的视为图片，其它退回到 text/multimodal。
export const inferMessageModality = (message) => {
  if (!message) return MODALITY.TEXT;
  if (message.modality) return message.modality;

  const c = message.content;
  if (Array.isArray(c)) {
    const hasVideo = c.some((p) => p?.type === 'video_url' && p.video_url?.url);
    if (hasVideo) return MODALITY.VIDEO;

    const imageParts = c.filter(
      (p) => p?.type === 'image_url' && p.image_url?.url,
    );
    const textPart = c.find(
      (p) => p?.type === 'text' && typeof p.text === 'string' && p.text.trim(),
    );

    // 助手侧：纯图片消息 = 图片生成结果
    if (imageParts.length > 0 && !textPart && message.role === 'assistant') {
      return MODALITY.IMAGE;
    }
    // 任意一侧带图带文 = 多模态对话（用户上传的视觉材料）
    if (imageParts.length > 0) return MODALITY.MULTIMODAL;
  }

  return MODALITY.TEXT;
};

// 把若干字段从 modalityMap / 当前 inputs 折叠成「一条消息的元数据」，
// 在创建用户/助手消息时附加进去。
export const messageMetaForCurrent = ({ model, group, modality }) => ({
  model: model || undefined,
  group: group || undefined,
  modality: modality || MODALITY.TEXT,
});
