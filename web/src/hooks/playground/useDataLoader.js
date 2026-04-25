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

import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { API, processGroupsData, showError } from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

// 一次性把所有可用分组下的模型聚合起来，让前端的「模型选择器」能跨分组
// 展示同一个模型的多份 (model, group) 选项（带分组名 + 倍率）。
//
// 旧逻辑是「先选分组、再选模型」：分组变化触发 loadModels(group)，
// 列表只会包含该分组下的模型。这导致用户切到一个没有目标模型的分组时
// 模型下拉变空，而他并不知道要去别的分组找。新逻辑：把分组挪到模型条目
// 的元数据里，用户在一个下拉里直接选「模型 + 分组」组合。
export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModelEntries,
  setGroups,
  setModalityMap,
) => {
  const { t } = useTranslation();
  const initializedRef = useRef(false);

  // 单分组的 detail 模型列表 -> [{name, modality, param_schema}]
  const fetchModelsForGroup = useCallback(async (group) => {
    const qs = new URLSearchParams();
    qs.set('group', group);
    qs.set('detail', 'true');
    const res = await API.get(`${API_ENDPOINTS.USER_MODELS}?${qs.toString()}`);
    const { success, data } = res.data || {};
    if (!success || !Array.isArray(data)) return [];
    return data
      .map((it) => {
        if (typeof it === 'string') {
          return { name: it, modality: 'text', param_schema: '' };
        }
        if (it && it.name) {
          return {
            name: it.name,
            modality: it.modality || 'text',
            param_schema: it.param_schema || '',
          };
        }
        return null;
      })
      .filter(Boolean);
  }, []);

  // 把 (groups + per-group model lists) 拍平成 modelEntries：
  //   [{ model, group, groupLabel, ratio, modality, paramSchema }, ...]
  // 同时输出 modalityMap 兼容旧调用方（按模型名查 modality / param_schema）。
  const buildAggregated = useCallback((groupOptions, perGroupModels) => {
    const entries = [];
    const modalityMap = {};
    groupOptions.forEach((g) => {
      const list = perGroupModels[g.value] || [];
      list.forEach((m) => {
        entries.push({
          model: m.name,
          group: g.value,
          groupLabel: g.fullLabel || g.label || g.value,
          ratio: g.ratio,
          modality: m.modality || 'text',
          paramSchema: m.param_schema || '',
        });
        // modalityMap 同名模型在不同分组的 modality 应该一致；以最先出现的为准
        if (!modalityMap[m.name]) {
          modalityMap[m.name] = {
            modality: m.modality || 'text',
            param_schema: m.param_schema || '',
          };
        }
      });
    });
    return { entries, modalityMap };
  }, []);

  const loadAll = useCallback(async () => {
    try {
      const res = await API.get(API_ENDPOINTS.USER_GROUPS);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(t(message || '加载分组失败'));
        return;
      }
      const userGroup =
        userState?.user?.group ||
        JSON.parse(localStorage.getItem('user') || '{}')?.group;
      let groupOptions = processGroupsData(data, userGroup);

      // 过滤掉 'auto' 之类的合成入口；真正的分组才下钻拉模型
      const realGroups = groupOptions.filter(
        (g) => g.value && g.value !== 'auto',
      );
      setGroups(realGroups);

      const perGroupArrays = await Promise.all(
        realGroups.map((g) => fetchModelsForGroup(g.value)),
      );
      const perGroupModels = {};
      realGroups.forEach((g, idx) => {
        perGroupModels[g.value] = perGroupArrays[idx];
      });

      const { entries, modalityMap } = buildAggregated(
        realGroups,
        perGroupModels,
      );
      setModelEntries(entries);
      if (setModalityMap) setModalityMap(modalityMap);

      // 兜底初始选择：若 inputs.model/group 在 entries 里找不到，挑第一个
      // 可用条目设回 inputs。优先匹配用户的 primary group。
      const matchExact = entries.find(
        (e) => e.model === inputs.model && e.group === inputs.group,
      );
      if (!matchExact && entries.length > 0) {
        const matchModel = entries.find((e) => e.model === inputs.model);
        const fallback =
          matchModel ||
          entries.find((e) => e.group === userGroup) ||
          entries[0];
        if (fallback) {
          if (fallback.model !== inputs.model) {
            handleInputChange('model', fallback.model);
          }
          if (fallback.group !== inputs.group) {
            handleInputChange('group', fallback.group);
          }
        }
      }
    } catch (err) {
      showError(t('加载模型失败'));
      // eslint-disable-next-line no-console
      console.error('loadAll failed', err);
    }
  }, [
    userState,
    inputs.model,
    inputs.group,
    handleInputChange,
    setGroups,
    setModelEntries,
    setModalityMap,
    fetchModelsForGroup,
    buildAggregated,
    t,
  ]);

  // 用户就绪后只跑一次完整加载；后续刷新依赖外部显式调用 loadAll。
  // 旧实现里"分组变化触发 loadModels"的链路彻底取消——分组现在是模型条目
  // 的属性，不再是过滤器。
  useEffect(() => {
    if (!userState?.user) return;
    if (initializedRef.current) return;
    initializedRef.current = true;
    loadAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userState?.user]);

  return {
    reload: loadAll,
  };
};
