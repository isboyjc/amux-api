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
import ChatArea from './ChatArea';
import ImageWorkspace from './ImageWorkspace';
import PlaceholderWorkspace from './PlaceholderWorkspace';
import { WORKSPACE } from '../../constants/workspaceTypes';

/**
 * 主工作区按当前会话的 workspace_type 分发：
 *   - chat（text / multimodal）→ ChatArea
 *   - image → ImageWorkspace（画廊 + prompt 输入）
 *   - video / audio / embedding / rerank → PlaceholderWorkspace
 */
const WorkspaceRouter = ({
  workspaceType = WORKSPACE.CHAT,
  currentModelName,
  chatAreaProps,
  imageWorkspaceProps,
  placeholderProps,
}) => {
  if (workspaceType === WORKSPACE.CHAT) {
    return <ChatArea {...chatAreaProps} />;
  }
  if (workspaceType === WORKSPACE.IMAGE) {
    return <ImageWorkspace {...imageWorkspaceProps} />;
  }
  return (
    <PlaceholderWorkspace
      workspaceType={workspaceType}
      modelName={currentModelName}
      {...placeholderProps}
    />
  );
};

export default WorkspaceRouter;
