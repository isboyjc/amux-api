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
import { API, processModelsData, processGroupsData } from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModels,
  setGroups,
  setModalityMap,
) => {
  const { t } = useTranslation();
  const prevGroupRef = useRef(inputs.group);

  const loadModels = useCallback(async (group) => {
    try {
      const qs = new URLSearchParams();
      if (group && group !== 'auto') qs.set('group', group);
      qs.set('detail', 'true');
      const res = await API.get(`${API_ENDPOINTS.USER_MODELS}?${qs.toString()}`);
      const { success, message, data } = res.data;

      if (success) {
        const { modelOptions, selectedModel, modalityMap } = processModelsData(
          data,
          inputs.model,
        );
        setModels(modelOptions);
        if (setModalityMap) setModalityMap(modalityMap || {});

        if (selectedModel !== inputs.model) {
          handleInputChange('model', selectedModel);
        }
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(t('加载模型失败'));
    }
  }, [inputs.model, handleInputChange, setModels, setModalityMap, t]);

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get(API_ENDPOINTS.USER_GROUPS);
      const { success, message, data } = res.data;

      if (success) {
        const userGroup =
          userState?.user?.group ||
          JSON.parse(localStorage.getItem('user'))?.group;
        const groupOptions = processGroupsData(data, userGroup);
        setGroups(groupOptions);

        const hasCurrentGroup = groupOptions.some(
          (option) => option.value === inputs.group,
        );
        if (!hasCurrentGroup) {
          const autoGroup = groupOptions.find((option) => option.value === 'auto');
          handleInputChange('group', autoGroup ? 'auto' : groupOptions[0]?.value || '');
        }
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(t('加载分组失败'));
    }
  }, [userState, inputs.group, handleInputChange, setGroups, t]);

  // 初始加载
  useEffect(() => {
    if (userState?.user) {
      loadModels(inputs.group);
      loadGroups();
    }
  }, [userState?.user]);

  // 分组变化时重新加载模型
  useEffect(() => {
    if (userState?.user && prevGroupRef.current !== inputs.group) {
      prevGroupRef.current = inputs.group;
      loadModels(inputs.group);
    }
  }, [inputs.group, userState?.user, loadModels]);

  return {
    loadModels,
    loadGroups,
  };
};
