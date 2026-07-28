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

import React, { useState } from 'react';
import { API, showError, getModelCategories } from '../../../helpers';
import CardPro from '../../common/ui/CardPro';
import TokensTable from './TokensTable';
import TokensActions from './TokensActions';
import TokensFilters from './TokensFilters';
import TokensDescription from './TokensDescription';
import EditTokenModal from './modals/EditTokenModal';
import CCSwitchModal from './modals/CCSwitchModal';
import TokenTestModal from './modals/TokenTestModal';
import { useTokenTest } from '../../../hooks/tokens/useTokenTest';
import { useTokensData } from '../../../hooks/tokens/useTokensData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

function TokensPage() {
  const tokensData = useTokensData();
  const isMobile = useIsMobile();
  // 令牌可用性测试。key 通过 tokensData.fetchTokenKey 现取（带缓存），
  // 只在弹窗生命周期内留在 hook 的 ref 里。
  const tokenTest = useTokenTest({ fetchTokenKey: tokensData.fetchTokenKey });
  const [modelOptions, setModelOptions] = useState([]);
  const [loadingModelOptions, setLoadingModelOptions] = useState(false);
  const [ccSwitchVisible, setCCSwitchVisible] = useState(false);
  const [ccSwitchKey, setCCSwitchKey] = useState('');
  const [ccSwitchTokenName, setCCSwitchTokenName] = useState('');

  // 某个令牌真正能调用的模型，由后端按 model_limits / 分组链算（同测试弹窗）。
  // 不能用 /api/user/models：那是「用户全部可用分组的并集」，会把该令牌调不到
  // 的模型也塞进 CC Switch 的下拉里，用户选了之后在客户端才报错。
  const loadTokenModels = async (record) => {
    // 先清空：否则在请求返回前，下拉里还挂着上一个令牌的模型，用户可能选中一个
    // 本令牌调不到的模型。
    setModelOptions([]);
    setLoadingModelOptions(true);
    try {
      const res = await API.get(
        `/api/token/${encodeURIComponent(record?.id)}/models`,
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(tokensData.t(message));
        setModelOptions([]);
        return;
      }
      const categories = getModelCategories(tokensData.t);
      const options = (data || []).map((item) => {
        // 该端点返回 [{name, modality}]，只取模型名
        const model = typeof item === 'string' ? item : item?.name;
        let icon = null;
        for (const [key, category] of Object.entries(categories)) {
          if (key !== 'all' && category.filter({ model_name: model })) {
            icon = category.icon;
            break;
          }
        }
        return {
          label: (
            <span className='flex items-center gap-1'>
              {icon}
              {model}
            </span>
          ),
          value: model,
        };
      });
      setModelOptions(options);
    } catch (e) {
      showError(e.message || 'Failed to load models');
      setModelOptions([]);
    } finally {
      setLoadingModelOptions(false);
    }
  };

  // CC Switch 需要令牌明文 key 和令牌名（默认名字用「令牌名 Amux API」），
  // key 通过 tokensData.fetchTokenKey 现取（带缓存），不在这里持久化。
  const openCCSwitchModal = async (record) => {
    try {
      const fullKey = await tokensData.fetchTokenKey(record);
      setCCSwitchKey(fullKey || '');
      setCCSwitchTokenName(record?.name || '');
      setCCSwitchVisible(true);
      // 每次都按当前令牌重新取：模型集合是「每令牌」的，缓存住第一个令牌的
      // 结果会让之后打开的令牌看到别人的模型列表。
      loadTokenModels(record);
    } catch (_) {
      // fetchTokenKey 失败已经在 hook 内部 toast 过了
    }
  };

  const {
    // Edit state
    showEdit,
    editingToken,
    closeEdit,
    refresh,

    // Actions state
    selectedKeys,
    setEditingToken,
    setShowEdit,
    batchCopyTokens,
    batchDeleteTokens,

    // Filters state
    formInitValues,
    setFormApi,
    searchTokens,
    loading,
    searching,

    // Description state
    compactMode,
    setCompactMode,

    // Translation
    t,
  } = tokensData;

  return (
    <>
      <EditTokenModal
        refresh={refresh}
        editingToken={editingToken}
        visiable={showEdit}
        handleClose={closeEdit}
      />

      <CCSwitchModal
        visible={ccSwitchVisible}
        onClose={() => setCCSwitchVisible(false)}
        tokenKey={ccSwitchKey}
        tokenName={ccSwitchTokenName}
        modelOptions={modelOptions}
        loadingModels={loadingModelOptions}
      />

      <TokenTestModal
        visible={tokenTest.visible}
        record={tokenTest.record}
        models={tokenTest.models}
        modalityMap={tokenTest.modalityMap}
        loadingModels={tokenTest.loadingModels}
        results={tokenTest.results}
        testing={tokenTest.testing}
        isBatchTesting={tokenTest.isBatchTesting}
        isStream={tokenTest.isStream}
        setIsStream={tokenTest.setIsStream}
        keyword={tokenTest.keyword}
        setKeyword={tokenTest.setKeyword}
        page={tokenTest.page}
        setPage={tokenTest.setPage}
        selectedKeys={tokenTest.selectedKeys}
        setSelectedKeys={tokenTest.setSelectedKeys}
        onClose={tokenTest.close}
        onTestModel={tokenTest.testModel}
        onBatchTest={tokenTest.batchTest}
        onStopBatch={tokenTest.stopBatch}
        isMobile={isMobile}
        t={tokensData.t}
      />

      <CardPro
        type='type1'
        descriptionArea={
          <TokensDescription
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        }
        actionsArea={
          <div className='flex flex-col md:flex-row justify-between items-center gap-2 w-full'>
            <TokensActions
              selectedKeys={selectedKeys}
              setEditingToken={setEditingToken}
              setShowEdit={setShowEdit}
              batchCopyTokens={batchCopyTokens}
              batchDeleteTokens={batchDeleteTokens}
              t={t}
            />

            <div className='w-full md:w-full lg:w-auto order-1 md:order-2'>
              <TokensFilters
                formInitValues={formInitValues}
                setFormApi={setFormApi}
                searchTokens={searchTokens}
                loading={loading}
                searching={searching}
                t={t}
              />
            </div>
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: tokensData.activePage,
          pageSize: tokensData.pageSize,
          total: tokensData.tokenCount,
          onPageChange: tokensData.handlePageChange,
          onPageSizeChange: tokensData.handlePageSizeChange,
          isMobile: isMobile,
          t: tokensData.t,
        })}
        t={tokensData.t}
      >
        <TokensTable
          {...tokensData}
          openTestModal={tokenTest.open}
          openCCSwitchModal={openCCSwitchModal}
        />
      </CardPro>
    </>
  );
}

export default TokensPage;
