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

import {
  STORAGE_KEYS,
  DEFAULT_CONFIG,
} from '../../constants/playground.constants';
import {
  getMessages as dbGetMessages,
  putMessages as dbPutMessages,
  clearMessages as dbClearMessages,
} from '../../utils/playgroundDb';

const ACTIVE_SESSION_KEY = 'playground_active_session_id';

/**
 * 活动会话 id 存 localStorage，方便刷新后定位到同一个会话。
 */
export const getActiveSessionId = () => {
  try {
    return localStorage.getItem(ACTIVE_SESSION_KEY) || null;
  } catch {
    return null;
  }
};

export const setActiveSessionId = (id) => {
  try {
    if (id) localStorage.setItem(ACTIVE_SESSION_KEY, id);
    else localStorage.removeItem(ACTIVE_SESSION_KEY);
  } catch {
    // ignore
  }
};

/**
 * 保存配置到 localStorage
 * @param {Object} config - 要保存的配置对象
 */
export const saveConfig = (config) => {
  try {
    const configToSave = {
      ...config,
      timestamp: new Date().toISOString(),
    };
    localStorage.setItem(STORAGE_KEYS.CONFIG, JSON.stringify(configToSave));
  } catch (error) {
    console.error('保存配置失败:', error);
  }
};

/**
 * 保存消息。sessionId 缺省时使用当前 active session。已迁移到 IndexedDB，
 * localStorage 不再承载消息内容——解决 base64 超限导致的丢失问题。
 * @param {Array} messages
 * @param {string} [sessionId]
 */
export const saveMessages = async (messages, sessionId) => {
  const sid = sessionId || getActiveSessionId();
  if (!sid) return;
  try {
    await dbPutMessages(sid, messages || []);
  } catch (error) {
    console.error('保存消息失败:', error);
  }
};

/**
 * 从 localStorage 加载配置
 * @returns {Object} 配置对象，如果不存在则返回默认配置
 */
export const loadConfig = () => {
  try {
    const savedConfig = localStorage.getItem(STORAGE_KEYS.CONFIG);
    if (savedConfig) {
      const parsedConfig = JSON.parse(savedConfig);
      const parsedMaxTokens = parseInt(parsedConfig?.inputs?.max_tokens, 10);

      const mergedConfig = {
        inputs: {
          ...DEFAULT_CONFIG.inputs,
          ...parsedConfig.inputs,
          max_tokens: Number.isNaN(parsedMaxTokens)
            ? parsedConfig?.inputs?.max_tokens
            : parsedMaxTokens,
        },
        parameterEnabled: {
          ...DEFAULT_CONFIG.parameterEnabled,
          ...parsedConfig.parameterEnabled,
        },
        showDebugPanel:
          parsedConfig.showDebugPanel || DEFAULT_CONFIG.showDebugPanel,
        customRequestMode:
          parsedConfig.customRequestMode || DEFAULT_CONFIG.customRequestMode,
        customRequestBody:
          parsedConfig.customRequestBody || DEFAULT_CONFIG.customRequestBody,
      };

      return mergedConfig;
    }
  } catch (error) {
    console.error('加载配置失败:', error);
  }

  return DEFAULT_CONFIG;
};

/**
 * 从 IndexedDB 加载消息。sessionId 缺省时使用当前 active session。
 * @param {string} [sessionId]
 * @returns {Promise<Array|null>}
 */
export const loadMessages = async (sessionId) => {
  const sid = sessionId || getActiveSessionId();
  if (!sid) return null;
  try {
    return await dbGetMessages(sid);
  } catch (error) {
    console.error('加载消息失败:', error);
    return null;
  }
};

/**
 * 一次性迁移旧 localStorage 里的消息数据到 IndexedDB。返回迁移到的
 * session id（若有）。供 useSessions 初始化时调用。
 */
export const migrateLegacyMessagesIfAny = async (newSessionId) => {
  try {
    const raw = localStorage.getItem(STORAGE_KEYS.MESSAGES);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    const list = parsed?.messages;
    if (Array.isArray(list) && list.length > 0 && newSessionId) {
      await dbPutMessages(newSessionId, list);
    }
    localStorage.removeItem(STORAGE_KEYS.MESSAGES);
    return newSessionId;
  } catch (e) {
    console.warn('迁移旧消息失败:', e);
    return null;
  }
};

/**
 * 清除保存的配置
 */
export const clearConfig = () => {
  try {
    localStorage.removeItem(STORAGE_KEYS.CONFIG);
    localStorage.removeItem(STORAGE_KEYS.MESSAGES); // 兼容旧 key
  } catch (error) {
    console.error('清除配置失败:', error);
  }
};

/**
 * 清除指定会话的消息。
 */
export const clearMessages = async (sessionId) => {
  const sid = sessionId || getActiveSessionId();
  if (!sid) return;
  try {
    await dbClearMessages(sid);
  } catch (error) {
    console.error('清除消息失败:', error);
  }
};

/**
 * 检查是否有保存的配置
 * @returns {boolean} 是否存在保存的配置
 */
export const hasStoredConfig = () => {
  try {
    return localStorage.getItem(STORAGE_KEYS.CONFIG) !== null;
  } catch (error) {
    console.error('检查配置失败:', error);
    return false;
  }
};

/**
 * 获取配置的最后保存时间
 * @returns {string|null} 最后保存时间的 ISO 字符串
 */
export const getConfigTimestamp = () => {
  try {
    const savedConfig = localStorage.getItem(STORAGE_KEYS.CONFIG);
    if (savedConfig) {
      const parsedConfig = JSON.parse(savedConfig);
      return parsedConfig.timestamp || null;
    }
  } catch (error) {
    console.error('获取配置时间戳失败:', error);
  }
  return null;
};

/**
 * 导出配置为 JSON 文件（包含消息）
 * @param {Object} config - 要导出的配置
 * @param {Array} messages - 要导出的消息
 */
export const exportConfig = (config, messages = null) => {
  try {
    const configToExport = {
      ...config,
      // 如果调用方没传 messages，这里不再自动去 IndexedDB 取（异步开销
      // 且 exportConfig 本身是同步 API）；调用方应传入当前消息数组。
      messages: messages || [],
      exportTime: new Date().toISOString(),
      version: '1.0',
    };

    const dataStr = JSON.stringify(configToExport, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });

    const link = document.createElement('a');
    link.href = URL.createObjectURL(dataBlob);
    link.download = `playground-config-${new Date().toISOString().split('T')[0]}.json`;
    link.click();

    URL.revokeObjectURL(link.href);
  } catch (error) {
    console.error('导出配置失败:', error);
  }
};

/**
 * 从文件导入配置（包含消息）
 * @param {File} file - 包含配置的 JSON 文件
 * @returns {Promise<Object>} 导入的配置对象
 */
export const importConfig = (file) => {
  return new Promise((resolve, reject) => {
    try {
      const reader = new FileReader();
      reader.onload = (e) => {
        try {
          const importedConfig = JSON.parse(e.target.result);

          if (importedConfig.inputs && importedConfig.parameterEnabled) {
            // 如果导入的配置包含消息，写入当前 active session
            if (
              importedConfig.messages &&
              Array.isArray(importedConfig.messages)
            ) {
              saveMessages(importedConfig.messages).catch((err) =>
                console.error('导入消息失败:', err),
              );
            }

            resolve(importedConfig);
          } else {
            reject(new Error('配置文件格式无效'));
          }
        } catch (parseError) {
          reject(new Error('解析配置文件失败: ' + parseError.message));
        }
      };
      reader.onerror = () => reject(new Error('读取文件失败'));
      reader.readAsText(file);
    } catch (error) {
      reject(new Error('导入配置失败: ' + error.message));
    }
  });
};
