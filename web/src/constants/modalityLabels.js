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
