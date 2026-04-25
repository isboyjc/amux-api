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

import React from 'react';
import { OptimizedMessageContent } from '../OptimizedComponents';
import ImageBubble from './ImageBubble';
import VideoBubble from './VideoBubble';
import { inferMessageModality } from '../messageModality';
import { MODALITY } from '../../../constants/playground.constants';

// 统一对话窗口的渲染分发器。
//
// 关键约束：图片 / 视频气泡是「助手输出」专属——它们从消息 content 里抽
// image_url[] / video_url 来渲染。用户消息的 content 永远是 prompt 字符串
// （或带 image_url 附件的多模态数组），抽不到生成结果，强行走 ImageBubble
// 会渲染成空气泡。
//
// 所以：role==='user' 一律走文本气泡（MessageContent 能正常渲染字符串
// prompt 和多模态附件）；role==='assistant' 才按 modality 派发到对应气泡。
const AssistantBubbleRouter = ({
  message,
  className,
  styleState,
  onToggleReasoningExpansion,
  isEditing,
  onEditSave,
  onEditCancel,
  editValue,
  onEditValueChange,
  // 图片气泡专属
  onImageContinueEdit,
  imageSupportsContinueEdit,
}) => {
  const modality = inferMessageModality(message);
  const isAssistant = message?.role === 'assistant';

  if (isAssistant && modality === MODALITY.IMAGE) {
    return (
      <div className={className}>
        <ImageBubble
          message={message}
          onContinueEdit={onImageContinueEdit}
          supportsContinueEdit={imageSupportsContinueEdit}
        />
      </div>
    );
  }

  if (isAssistant && modality === MODALITY.VIDEO) {
    return (
      <div className={className}>
        <VideoBubble message={message} />
      </div>
    );
  }

  // 其余情况（用户消息 + 文本/多模态助手消息）走文本气泡链路。
  // MessageContent 已能渲染 image_url[] 多模态附件 + markdown；用户消息里
  // 的参考图组也走 onImageContinueEdit，hover 出现编辑按钮。
  return (
    <OptimizedMessageContent
      message={message}
      className={className}
      styleState={styleState}
      onToggleReasoningExpansion={onToggleReasoningExpansion}
      isEditing={isEditing}
      onEditSave={onEditSave}
      onEditCancel={onEditCancel}
      editValue={editValue}
      onEditValueChange={onEditValueChange}
      onImageContinueEdit={onImageContinueEdit}
    />
  );
};

export default AssistantBubbleRouter;
