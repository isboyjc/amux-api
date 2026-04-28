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

// Modality 的显示文案都从这里拿。字面量通过 t() 直接写出来，让
// i18next-cli 的静态提取器可以识别；其它地方按 modality key 查询，
// 保证每个文案只在此处声明一次。

export const getModalityLongLabel = (t, modality) => {
  switch (modality) {
    case 'text':
      return t('文本对话');
    case 'multimodal':
      return t('多模态');
    case 'image':
      return t('图片生成');
    case 'video':
      return t('视频生成');
    case 'audio':
      return t('音频');
    case 'embedding':
      return t('向量嵌入');
    case 'rerank':
      return t('重排序');
    default:
      return modality || '';
  }
};

export const getModalityShortLabel = (t, modality) => {
  switch (modality) {
    case 'text':
      return t('文本');
    case 'multimodal':
      return t('多模态');
    case 'image':
      return t('图片');
    case 'video':
      return t('视频');
    case 'audio':
      return t('音频');
    case 'embedding':
      return t('向量');
    case 'rerank':
      return t('重排');
    default:
      return modality || '';
  }
};

// 输入/输出能力（input_modalities / output_modalities）的显示文案。
// 与 Modality 枚举有重叠，但 capability 是更细粒度的多值集合，且多了 file。
export const getCapabilityLabel = (t, cap) => {
  switch (cap) {
    case 'text':
      return t('文本');
    case 'image':
      return t('图片');
    case 'audio':
      return t('音频');
    case 'video':
      return t('视频');
    case 'file':
      return t('文件');
    case 'embedding':
      return t('向量嵌入');
    case 'rerank':
      return t('重排');
    default:
      return cap || '';
  }
};

// 后备顺序：当后端没返回枚举时使用
export const INPUT_CAPABILITY_FALLBACK = [
  'text',
  'image',
  'audio',
  'video',
  'file',
];
export const OUTPUT_CAPABILITY_FALLBACK = [
  'text',
  'image',
  'audio',
  'video',
  'embedding',
  'rerank',
];

// Tag 色调
export const MODALITY_COLOR = {
  text: 'blue',
  multimodal: 'teal',
  image: 'violet',
  video: 'orange',
  audio: 'pink',
  embedding: 'cyan',
  rerank: 'amber',
};
