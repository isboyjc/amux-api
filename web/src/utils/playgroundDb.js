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

// Playground 会话与消息的 IndexedDB 存储层。
//
// 为什么不用 localStorage：
//   - 5~10MB 硬上限，一张 base64 图片就能把 playground_messages 键写爆，
//     写失败时会整键丢失。
//   - 同步 API，所有读写都阻塞主线程。
//
// IndexedDB 默认配额通常在几百 MB 到几 GB，按 key 写入，单条消息异常不会
// 影响其它消息。
//
// Schema:
//   sessions(id, title, modality, model, group, created_at, updated_at)
//   messages(session_id [indexed], messages[] as blob)
// 消息以"整数组"为单位按会话保存；这样既与现有 UI 习惯（批量 setMessage）
// 兼容，也不需要精细的事务。后续如果需要流式追加，可再拆。

const DB_NAME = 'new_api_playground';
const DB_VERSION = 1;
const STORE_SESSIONS = 'sessions';
const STORE_MESSAGES = 'messages';

let dbPromise = null;

const openDb = () => {
  if (dbPromise) return dbPromise;
  if (typeof indexedDB === 'undefined') {
    return Promise.reject(new Error('IndexedDB not available'));
  }
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE_SESSIONS)) {
        db.createObjectStore(STORE_SESSIONS, { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains(STORE_MESSAGES)) {
        // 一行 = 一个会话的完整消息数组
        db.createObjectStore(STORE_MESSAGES, { keyPath: 'session_id' });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
    req.onblocked = () => reject(new Error('IndexedDB blocked'));
  });
  // 失败时清掉单例，下次调用允许重开（比如用户首次禁用后重新授权）
  dbPromise.catch(() => {
    dbPromise = null;
  });
  return dbPromise;
};

const tx = async (storeNames, mode, fn) => {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(storeNames, mode);
    let result;
    try {
      result = fn(transaction);
    } catch (e) {
      reject(e);
      return;
    }
    transaction.oncomplete = () => resolve(result);
    transaction.onabort = () => reject(transaction.error);
    transaction.onerror = () => reject(transaction.error);
  });
};

const request = (req) =>
  new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });

// ===================== sessions =====================

/**
 * 列出全部会话，按 updated_at 降序。
 * @returns {Promise<Array>}
 */
export const listSessions = async () => {
  try {
    const rows = await tx([STORE_SESSIONS], 'readonly', (t) =>
      request(t.objectStore(STORE_SESSIONS).getAll()),
    );
    return (rows || []).sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0));
  } catch (e) {
    console.error('[playgroundDb] listSessions failed:', e);
    return [];
  }
};

/**
 * 读取单个会话。
 */
export const getSession = async (id) => {
  if (!id) return null;
  try {
    return await tx([STORE_SESSIONS], 'readonly', (t) =>
      request(t.objectStore(STORE_SESSIONS).get(id)),
    );
  } catch (e) {
    console.error('[playgroundDb] getSession failed:', e);
    return null;
  }
};

/**
 * upsert 会话。session 必须含 id。
 */
export const putSession = async (session) => {
  if (!session || !session.id) throw new Error('session.id required');
  const now = Date.now();
  const record = {
    created_at: now,
    updated_at: now,
    ...session,
  };
  try {
    await tx([STORE_SESSIONS], 'readwrite', (t) => {
      t.objectStore(STORE_SESSIONS).put(record);
    });
    return record;
  } catch (e) {
    console.error('[playgroundDb] putSession failed:', e);
    throw e;
  }
};

/**
 * 局部更新会话字段。默认不刷新 updated_at —— 列表按 updated_at 排序，
 * 只有"发送新消息"这样的真实活动才应该让会话跳到顶部；切会话时的 model/
 * group/modality 同步、迁移、重命名等元数据变更都不应重新排序。需要刷新
 * 时显式传 `{ touch: true }` 或使用 `touchSession(id)`。
 *
 * 实现要点：get + put 放在同一个 readwrite IDB 事务里完成，避免两个并发
 * patchSession 之间发生 read-modify-write 竞态（例如 rename 刚写完
 * title，touchSession 后起但先读，又用老 title 覆盖回去）。IDB 天然对
 * 同一 store 上的事务串行化，合并成一笔能保证原子性。
 */
export const patchSession = async (id, patch, { touch = false } = {}) => {
  if (!id) return null;
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const transaction = db.transaction([STORE_SESSIONS], 'readwrite');
    const store = transaction.objectStore(STORE_SESSIONS);

    let merged = null;

    const getReq = store.get(id);
    getReq.onsuccess = () => {
      const current = getReq.result;
      if (!current) {
        merged = null;
        return;
      }
      merged = {
        ...current,
        ...patch,
        id,
        ...(touch ? { updated_at: Date.now() } : {}),
      };
      // 同一事务里接着 put。如果 put 失败，oncomplete 不会触发，
      // 由 onabort / onerror 走拒绝路径。
      store.put(merged);
    };
    getReq.onerror = () => reject(getReq.error);

    transaction.oncomplete = () => resolve(merged);
    transaction.onabort = () => reject(transaction.error);
    transaction.onerror = () => reject(transaction.error);
  });
};

/**
 * 显式把会话"推到顶部"。仅在真实消息活动（新发送/新生成）后调用。
 */
export const touchSession = async (id) => {
  return patchSession(id, {}, { touch: true });
};

/**
 * 删除会话及其消息。
 */
export const deleteSession = async (id) => {
  if (!id) return;
  try {
    await tx([STORE_SESSIONS, STORE_MESSAGES], 'readwrite', (t) => {
      t.objectStore(STORE_SESSIONS).delete(id);
      t.objectStore(STORE_MESSAGES).delete(id);
    });
  } catch (e) {
    console.error('[playgroundDb] deleteSession failed:', e);
    throw e;
  }
};

// ===================== messages =====================

/**
 * 读取会话的消息数组。不存在返回 null。
 */
export const getMessages = async (sessionId) => {
  if (!sessionId) return null;
  try {
    const row = await tx([STORE_MESSAGES], 'readonly', (t) =>
      request(t.objectStore(STORE_MESSAGES).get(sessionId)),
    );
    return row ? row.messages || null : null;
  } catch (e) {
    console.error('[playgroundDb] getMessages failed:', e);
    return null;
  }
};

/**
 * 整体覆盖会话的消息数组。
 */
export const putMessages = async (sessionId, messages) => {
  if (!sessionId) throw new Error('sessionId required');
  try {
    await tx([STORE_MESSAGES], 'readwrite', (t) => {
      t.objectStore(STORE_MESSAGES).put({
        session_id: sessionId,
        messages: messages || [],
        updated_at: Date.now(),
      });
    });
  } catch (e) {
    console.error('[playgroundDb] putMessages failed:', e);
    throw e;
  }
};

export const clearMessages = async (sessionId) => {
  if (!sessionId) return;
  try {
    await tx([STORE_MESSAGES], 'readwrite', (t) => {
      t.objectStore(STORE_MESSAGES).delete(sessionId);
    });
  } catch (e) {
    console.error('[playgroundDb] clearMessages failed:', e);
  }
};

// ===================== utils =====================

export const genSessionId = () => {
  const rnd = Math.random().toString(36).slice(2, 10);
  return `sess_${Date.now().toString(36)}_${rnd}`;
};
