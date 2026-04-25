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

import { useCallback, useEffect, useState } from 'react';
import {
  listSessions,
  putSession,
  patchSession,
  touchSession as dbTouchSession,
  deleteSession as dbDeleteSession,
  genSessionId,
} from '../../utils/playgroundDb';
import {
  getActiveSessionId,
  setActiveSessionId,
  migrateLegacyMessagesIfAny,
} from '../../components/playground/configStorage';
const DEFAULT_TITLE = '未命名会话';

/**
 * 会话 CRUD + active 追踪。独立成 hook 便于在 Playground 以外的地方复用，
 * 也便于测试。
 */
export const useSessions = () => {
  const [sessions, setSessions] = useState([]);
  const [activeId, setActiveIdState] = useState(() => getActiveSessionId());
  const [initialized, setInitialized] = useState(false);

  const refresh = useCallback(async () => {
    const list = await listSessions();
    setSessions(list);
    return list;
  }, []);

  // 首次挂载：加载列表、建默认会话。
  // 旧版本会话曾经带 workspace_type 字段，现已废弃；保留在 IDB 里不动，
  // 新代码读 session.modality 即可，向前向后都兼容。
  useEffect(() => {
    (async () => {
      let list = await listSessions();

      if (list.length === 0) {
        const id = genSessionId();
        const now = Date.now();
        const newSession = {
          id,
          title: DEFAULT_TITLE,
          modality: 'text',
          model: '',
          group: '',
          created_at: now,
          updated_at: now,
        };
        await putSession(newSession);
        await migrateLegacyMessagesIfAny(id);
        setSessions([newSession]);
        setActiveIdState(id);
        setActiveSessionId(id);
      } else {
        setSessions(list);
        const saved = getActiveSessionId();
        const stillExists = saved && list.some((s) => s.id === saved);
        const next = stillExists ? saved : list[0].id;
        setActiveIdState(next);
        setActiveSessionId(next);
      }
      setInitialized(true);
    })();
  }, []);

  const switchSession = useCallback((id) => {
    if (!id) return;
    setActiveIdState(id);
    setActiveSessionId(id);
  }, []);

  const createSession = useCallback(
    async ({ title, modality, model, group } = {}) => {
      const id = genSessionId();
      const now = Date.now();
      const record = {
        id,
        title: title || DEFAULT_TITLE,
        modality: modality || 'text',
        model: model || '',
        group: group || '',
        created_at: now,
        updated_at: now,
      };
      await putSession(record);
      await refresh();
      switchSession(id);
      return record;
    },
    [refresh, switchSession],
  );

  const renameSession = useCallback(
    async (id, title) => {
      await patchSession(id, { title });
      await refresh();
    },
    [refresh],
  );

  const updateSessionMeta = useCallback(
    async (id, patch) => {
      await patchSession(id, patch);
      await refresh();
    },
    [refresh],
  );

  const deleteSession = useCallback(
    async (id) => {
      await dbDeleteSession(id);
      const list = await listSessions();
      setSessions(list);
      if (id === activeId) {
        if (list.length > 0) {
          switchSession(list[0].id);
        } else {
          // 删完了：建一个空会话保证始终有 active
          await createSession({});
        }
      }
    },
    [activeId, switchSession, createSession],
  );

  // 仅在"有新消息活动"时调用：把会话排到顶部。
  // patchSession 默认不 touch，所以 meta 同步/迁移/重命名都不会改动顺序。
  const touchSession = useCallback(
    async (id) => {
      if (!id) return;
      await dbTouchSession(id);
      await refresh();
    },
    [refresh],
  );

  return {
    sessions,
    activeId,
    initialized,
    refresh,
    switchSession,
    createSession,
    renameSession,
    updateSessionMeta,
    deleteSession,
    touchSession,
  };
};
