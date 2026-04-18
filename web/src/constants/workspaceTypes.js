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

// Workspace 类型是"会话的 UI 形态"，由创建时确定、一旦创建不再变化。
// 同一 workspace 内部允许在"兼容 modality"之间自由切换模型，跨 workspace
// 只能另开新会话。
//
// 例：chat workspace 同时接纳 text（纯文本）+ multimodal（视觉对话），
// 两者 UI 都是聊天气泡，差异仅在后者能附图；image workspace 只接纳 image
// 家族，UI 是画廊；以此类推。

export const WORKSPACE = {
  CHAT: 'chat',
  IMAGE: 'image',
  VIDEO: 'video',
  AUDIO: 'audio',
  EMBEDDING: 'embedding',
  RERANK: 'rerank',
};

// 每个 workspace 包含的 modality 白名单
export const WORKSPACE_MODALITIES = {
  [WORKSPACE.CHAT]: ['text', 'multimodal'],
  [WORKSPACE.IMAGE]: ['image'],
  [WORKSPACE.VIDEO]: ['video'],
  [WORKSPACE.AUDIO]: ['audio'],
  [WORKSPACE.EMBEDDING]: ['embedding'],
  [WORKSPACE.RERANK]: ['rerank'],
};

// 反向映射：已知某个 modality，它属于哪个 workspace
export const MODALITY_TO_WORKSPACE = {
  text: WORKSPACE.CHAT,
  multimodal: WORKSPACE.CHAT,
  image: WORKSPACE.IMAGE,
  video: WORKSPACE.VIDEO,
  audio: WORKSPACE.AUDIO,
  embedding: WORKSPACE.EMBEDDING,
  rerank: WORKSPACE.RERANK,
};

// 一期 Playground 实际可以使用的 workspace。其他创建入口先灰化。
export const V1_ENABLED_WORKSPACES = new Set([WORKSPACE.CHAT, WORKSPACE.IMAGE]);

// "+新建会话"菜单里展示的顺序
export const WORKSPACE_PICK_ORDER = [
  WORKSPACE.CHAT,
  WORKSPACE.IMAGE,
  WORKSPACE.VIDEO,
  WORKSPACE.AUDIO,
  WORKSPACE.EMBEDDING,
  WORKSPACE.RERANK,
];

// Workspace 徽章颜色
export const WORKSPACE_COLOR = {
  [WORKSPACE.CHAT]: 'blue',
  [WORKSPACE.IMAGE]: 'violet',
  [WORKSPACE.VIDEO]: 'orange',
  [WORKSPACE.AUDIO]: 'pink',
  [WORKSPACE.EMBEDDING]: 'cyan',
  [WORKSPACE.RERANK]: 'amber',
};

// Workspace 显示标签（t() 把字面量暴露给 i18next-cli 静态提取）
export const getWorkspaceLabel = (t, workspace) => {
  switch (workspace) {
    case WORKSPACE.CHAT:
      return t('对话');
    case WORKSPACE.IMAGE:
      return t('图片');
    case WORKSPACE.VIDEO:
      return t('视频');
    case WORKSPACE.AUDIO:
      return t('音频');
    case WORKSPACE.EMBEDDING:
      return t('嵌入');
    case WORKSPACE.RERANK:
      return t('重排');
    default:
      return workspace || '';
  }
};

// 根据 modality 推断 workspace（用于数据迁移和"默认创建"场景）。
export const inferWorkspaceFromModality = (modality) =>
  MODALITY_TO_WORKSPACE[modality] || WORKSPACE.CHAT;

// 判断一个 modality 是否和某 workspace 兼容。
export const isModalityInWorkspace = (modality, workspace) => {
  if (!workspace) return true;
  const allowed = WORKSPACE_MODALITIES[workspace];
  if (!allowed) return true;
  return allowed.includes(modality || 'text');
};
