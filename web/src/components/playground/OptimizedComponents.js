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
import MessageContent from './MessageContent';
import MessageActions from './MessageActions';
import SettingsPanel from './SettingsPanel';
import DebugPanel from './DebugPanel';

// 优化的消息内容组件。
//
// ⚠️ 这是"白名单"式 React.memo——只比下面列出的 prop，没列出的 prop 即便
// 变了也不会触发 re-render。所以**任何"运行时身份会变"的 callback prop**
// 都必须显式纳入比较，否则 inner DOM 上挂的会是陈旧闭包，行为偏离当前状态。
//
// 历史教训：onImageContinueEdit 一开始没列在这里 → 用户在视频 first_last
// 模式下点已发送气泡的 pencil 按钮，跑的是"切模式前的"handleContinueEdit
// 闭包，图片被路由到 reference 槽而不是首/末帧槽。把所有用户能交互到的
// callback 都纳进来才安全。
export const OptimizedMessageContent = React.memo(
  MessageContent,
  (prevProps, nextProps) => {
    return (
      prevProps.message.id === nextProps.message.id &&
      prevProps.message.content === nextProps.message.content &&
      prevProps.message.status === nextProps.message.status &&
      prevProps.message.role === nextProps.message.role &&
      prevProps.message.reasoningContent ===
        nextProps.message.reasoningContent &&
      prevProps.message.isReasoningExpanded ===
        nextProps.message.isReasoningExpanded &&
      prevProps.isEditing === nextProps.isEditing &&
      prevProps.editValue === nextProps.editValue &&
      prevProps.styleState.isMobile === nextProps.styleState.isMobile &&
      prevProps.onImageContinueEdit === nextProps.onImageContinueEdit &&
      prevProps.onToggleReasoningExpansion ===
        nextProps.onToggleReasoningExpansion &&
      prevProps.onEditSave === nextProps.onEditSave &&
      prevProps.onEditCancel === nextProps.onEditCancel &&
      prevProps.onEditValueChange === nextProps.onEditValueChange
    );
  },
);

// 优化的消息操作组件
export const OptimizedMessageActions = React.memo(
  MessageActions,
  (prevProps, nextProps) => {
    return (
      prevProps.message.id === nextProps.message.id &&
      prevProps.message.role === nextProps.message.role &&
      prevProps.isAnyMessageGenerating === nextProps.isAnyMessageGenerating &&
      prevProps.isEditing === nextProps.isEditing &&
      prevProps.onMessageReset === nextProps.onMessageReset
    );
  },
);

// 优化的设置面板组件
export const OptimizedSettingsPanel = React.memo(
  SettingsPanel,
  (prevProps, nextProps) => {
    return (
      JSON.stringify(prevProps.inputs) === JSON.stringify(nextProps.inputs) &&
      JSON.stringify(prevProps.parameterEnabled) ===
        JSON.stringify(nextProps.parameterEnabled) &&
      JSON.stringify(prevProps.modelEntries) ===
        JSON.stringify(nextProps.modelEntries) &&
      prevProps.currentModality === nextProps.currentModality &&
      prevProps.customRequestMode === nextProps.customRequestMode &&
      prevProps.customRequestBody === nextProps.customRequestBody &&
      prevProps.showDebugPanel === nextProps.showDebugPanel &&
      prevProps.showSettings === nextProps.showSettings &&
      prevProps.activeSessionId === nextProps.activeSessionId &&
      JSON.stringify(prevProps.sessions) ===
        JSON.stringify(nextProps.sessions) &&
      JSON.stringify(prevProps.previewPayload) ===
        JSON.stringify(nextProps.previewPayload) &&
      JSON.stringify(prevProps.messages) === JSON.stringify(nextProps.messages)
    );
  },
);

// 优化的调试面板组件
export const OptimizedDebugPanel = React.memo(
  DebugPanel,
  (prevProps, nextProps) => {
    return (
      prevProps.show === nextProps.show &&
      prevProps.activeTab === nextProps.activeTab &&
      JSON.stringify(prevProps.debugData) ===
        JSON.stringify(nextProps.debugData) &&
      JSON.stringify(prevProps.previewPayload) ===
        JSON.stringify(nextProps.previewPayload) &&
      prevProps.customRequestMode === nextProps.customRequestMode &&
      prevProps.showDebugPanel === nextProps.showDebugPanel
    );
  },
);
