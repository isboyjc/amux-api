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

import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showInfo } from '../../helpers';
import { getServerAddress } from '../../helpers/token';
import { resolveTestProfile } from '../../constants/tokenTest';

// 批量测试的串行间隔。/v1/* 上挂了 ModelRequestRateLimit（按分组限流），
// 渠道测试走的 /api/channel/test/:id 没有这个中间件，所以那边可以 5 路并发无
// 间隔地打；这里必须串行 + 留间隔，否则连打 N 个模型会撞限流返回 429，用户
// 看到一片失败还以为令牌坏了。
const BATCH_INTERVAL_MS = 300;

/**
 * 令牌连通性测试。
 *
 * 请求用裸 fetch 而不是 helpers/api.js 的 API 实例：后者带 New-Api-User 头和
 * 全局响应拦截器（会把错误弹成 toast）。这里要的恰恰是把上游/网关的原始错误
 * 原封不动展示在弹窗里 —— 用户要看的就是他自己的客户端会收到什么。
 */
export const useTokenTest = ({ fetchTokenKey }) => {
  const { t } = useTranslation();

  const [visible, setVisible] = useState(false);
  const [record, setRecord] = useState(null);
  // { [model]: { modality, param_schema } }
  const [modalityMap, setModalityMap] = useState({});
  const [models, setModels] = useState([]);
  const [loadingModels, setLoadingModels] = useState(false);
  // { [model]: { success, message, time, detail, status } }
  const [results, setResults] = useState({});
  const [testing, setTesting] = useState(() => new Set());
  const [isBatchTesting, setIsBatchTesting] = useState(false);
  const [isStream, setIsStream] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [selectedKeys, setSelectedKeys] = useState([]);

  const stopBatchRef = useRef(false);
  // 令牌明文只在弹窗生命周期内留在 ref 里，不进 state、不持久化。
  const keyRef = useRef('');

  const open = useCallback(async (target) => {
    setRecord(target);
    setVisible(true);
    setResults({});
    setKeyword('');
    setPage(1);
    setSelectedKeys([]);
    setIsStream(false);
    keyRef.current = '';
    // close() 会把它置为 true 来中断在跑的批量测试，这里必须清掉，
    // 否则重新打开弹窗后 testModel 一进来就直接 return，点什么都没反应。
    stopBatchRef.current = false;

    setLoadingModels(true);
    try {
      // 令牌能用哪些模型，按令牌自身的配置来算，而不是列全站模型。
      //
      // 1) 开了 model_limits：白名单就是全集，直接用。注意不要在前端拿它
      //    反过来预判「这个模型会被拒」—— 后端校验前会过
      //    ratio_setting.FormatMatchingModelName 归一化（match gpts &
      //    thinking-*），字面比较会算错，判断留给后端。
      // 2) 没开：查分组链的模型并集。链是有序的、失败会 fallback
      //    （service.ResolveGroupChain），所以能用的是整条链的并集而不是
      //    链首那一个 —— 后端 ?groups=a,b,c 就是干这个的。
      // 3) auto 分组：链由 GetUserAutoGroup 运行时算，前端算不出来，不传
      //    groups 参数退回全部可用分组的并集（同 EditTokenModal）。
      const chain = Array.isArray(target?.groups) ? target.groups : [];
      const usable = chain.filter((g) => g && g !== 'auto');
      const params = usable.length
        ? `?detail=true&groups=${encodeURIComponent(usable.join(','))}`
        : '?detail=true';

      const res = await API.get(`/api/user/models${params}`);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message);
        return;
      }

      const map = {};
      (Array.isArray(data) ? data : []).forEach((item) => {
        if (typeof item === 'string') {
          map[item] = { modality: 'text' };
        } else if (item?.name) {
          map[item.name] = { modality: item.modality || 'text' };
        }
      });
      setModalityMap(map);

      if (target?.model_limits_enabled && target?.model_limits) {
        // 白名单模型可能不在 detail 结果里（比如分组下已下架），modality
        // 查不到就按 text 兜底。
        setModels(
          target.model_limits
            .split(',')
            .map((m) => m.trim())
            .filter(Boolean),
        );
      } else {
        setModels(Object.keys(map));
      }
    } finally {
      setLoadingModels(false);
    }
  }, []);

  const close = useCallback(() => {
    stopBatchRef.current = true;
    setVisible(false);
    setRecord(null);
    setModels([]);
    setModalityMap({});
    setResults({});
    setIsBatchTesting(false);
    keyRef.current = '';
  }, []);

  const ensureKey = useCallback(async () => {
    if (keyRef.current) return keyRef.current;
    const fullKey = await fetchTokenKey(record);
    keyRef.current = fullKey;
    return fullKey;
  }, [fetchTokenKey, record]);

  const testModel = useCallback(
    async (model) => {
      // 只看 ref：isBatchTesting 是 state，batchTest 里拿到的 testModel 闭包
      // 捕获的是本轮渲染的旧值（一直是 false），拿它做与运算会让「停止测试」
      // 完全失效。ref 没有这个问题。
      if (stopBatchRef.current) return;

      const modality = modalityMap[model]?.modality || 'text';
      const { profile, untestable } = resolveTestProfile(model, modality);
      if (!profile) {
        setResults((prev) => ({
          ...prev,
          [model]: { success: false, untestable, message: '' },
        }));
        return;
      }

      setTesting((prev) => new Set([...prev, model]));
      const startedAt = Date.now();

      try {
        const fullKey = await ensureKey();
        const body = profile.body(model);
        // 流式只对 chat 类端点有意义，其余 profile 不声明 stream。
        if (profile.stream && isStream) body.stream = true;

        const res = await fetch(`${getServerAddress()}${profile.path}`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer sk-${fullKey}`,
          },
          body: JSON.stringify(body),
        });

        const elapsed = (Date.now() - startedAt) / 1000;
        // TTS 返回的是音频二进制。必须在读取前就分流：body 只能消费一次，
        // 先 text() 再想拿字节数就晚了，而且二进制按 UTF-8 解码出来的字符数
        // 和真实字节数对不上（多字节序列会被合并），报出来的数字是错的。
        if (res.ok && profile.binary) {
          const buf = await res.arrayBuffer();
          setResults((prev) => ({
            ...prev,
            [model]: {
              success: true,
              time: elapsed,
              summary: profile.extract({ bytes: buf.byteLength }),
              detail: '',
            },
          }));
          return;
        }

        const raw = await res.text();

        if (!res.ok) {
          let message = raw;
          try {
            const parsed = JSON.parse(raw);
            message = parsed?.error?.message || parsed?.message || raw;
          } catch (_) {
            // 非 JSON 错误体（网关 HTML 之类），原样展示
          }
          setResults((prev) => ({
            ...prev,
            [model]: {
              success: false,
              status: res.status,
              // 429 是限流而不是令牌/模型问题，单独标出来免得误判
              rateLimited: res.status === 429,
              message: message || t('请求失败'),
              detail: raw,
              time: elapsed,
            },
          }));
          return;
        }

        // 流式返回是 SSE 文本，不是 JSON；能拿到 data: 帧就算通了。
        if (profile.stream && isStream) {
          setResults((prev) => ({
            ...prev,
            [model]: {
              success: true,
              time: elapsed,
              summary: raw.includes('data:') ? t('已收到流式响应') : '',
              detail: raw.slice(0, 2000),
              async: false,
            },
          }));
          return;
        }

        let parsed = null;
        try {
          parsed = JSON.parse(raw);
        } catch (_) {
          parsed = null;
        }
        setResults((prev) => ({
          ...prev,
          [model]: {
            success: true,
            time: elapsed,
            summary: parsed ? profile.extract(parsed) : '',
            detail: raw.slice(0, 2000),
            async: Boolean(profile.async),
          },
        }));
      } catch (error) {
        setResults((prev) => ({
          ...prev,
          [model]: {
            success: false,
            message: error?.message || t('网络错误'),
            time: (Date.now() - startedAt) / 1000,
          },
        }));
      } finally {
        setTesting((prev) => {
          const next = new Set(prev);
          next.delete(model);
          return next;
        });
      }
    },
    [ensureKey, isStream, modalityMap, t],
  );

  // 批量测试只跑传进来的这批（调用方传当前页 / 已选），串行 + 留间隔。
  const batchTest = useCallback(
    async (targets) => {
      const list = (targets || []).filter((m) => {
        const modality = modalityMap[m]?.modality || 'text';
        return Boolean(resolveTestProfile(m, modality).profile);
      });
      if (list.length === 0) {
        showError(t('没有可自动测试的模型'));
        return;
      }

      setIsBatchTesting(true);
      stopBatchRef.current = false;
      setResults((prev) => {
        const next = { ...prev };
        list.forEach((m) => delete next[m]);
        return next;
      });

      try {
        for (let i = 0; i < list.length; i++) {
          if (stopBatchRef.current) {
            showInfo(t('批量测试已停止'));
            break;
          }
          await testModel(list[i]);
          if (i < list.length - 1) {
            await new Promise((r) => setTimeout(r, BATCH_INTERVAL_MS));
          }
        }
      } finally {
        setIsBatchTesting(false);
        stopBatchRef.current = false;
      }
    },
    [modalityMap, t, testModel],
  );

  const stopBatch = useCallback(() => {
    stopBatchRef.current = true;
  }, []);

  return {
    visible,
    record,
    models,
    modalityMap,
    loadingModels,
    results,
    testing,
    isBatchTesting,
    isStream,
    setIsStream,
    keyword,
    setKeyword,
    page,
    setPage,
    selectedKeys,
    setSelectedKeys,
    open,
    close,
    testModel,
    batchTest,
    stopBatch,
  };
};
