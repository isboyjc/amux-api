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
import { Button, Form } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';

import { DATE_RANGE_PRESETS } from '../../../constants/console.constants';

const LogsFilters = ({
  formInitValues,
  setFormApi,
  refresh,
  setShowColumnSelector,
  formApi,
  setLogType,
  loading,
  isAdminUser,
  tokenOptions,
  tokenOptionsTruncated,
  groupOptions,
  modelOptions,
  t,
}) => {
  // 下拉候选只覆盖「当前存在」的令牌/分组/模型，而日志里的 token_name / group /
  // model_name 是写入当时的历史快照。令牌改名、模型下架后，那批旧日志的名字不在
  // 候选里 —— 所以下拉都开 allowCreate + filter，既能输入关键字快速过滤，也能
  // 直接输入候选之外的任意值，不让下拉化造成功能倒退。
  const searchableSelectProps = {
    filter: true,
    allowCreate: true,
    showClear: true,
    pure: true,
    size: 'small',
    className: 'w-full',
    // 下面的 key 会让 Select 在候选到达时重挂，而 Semi 的 Form 字段默认在卸载时
    // 删掉自己的值（form/foundation.js unRegister: !keepState 时 remove values）。
    // 不加 keepState，从工单跳转过来的 ?token_name=xxx 预填值会被这次重挂清空。
    keepState: true,
  };

  // Semi 的 allowCreate + 受控组件（Form.Select 由 Form 托管 value）组合下，
  // select/foundation.js 里 handleValueChange 有一条捷径：
  //     if (allowCreate && this._isControlledComponent()) {
  //         originalOptions = this.getState('options');   // 复用旧 options
  //     }
  // 它会跳过对 props.optionList 的重新收集。我们的候选值是异步拉回来的，首次
  // 挂载时是空数组，于是内部 options 永远停在空 —— 下拉打开什么都没有。
  // 用 key 绑定候选数量，让候选到达时强制重建 Select，绕开这条捷径。
  const optionsKey = (list) => (list?.length ? list.length : 0);

  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={refresh}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div className='flex flex-col gap-2'>
        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2'>
          {/* 时间选择器 */}
          <div className='col-span-1'>
            <Form.DatePicker
              field='dateRange'
              className='w-full'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size='small'
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
            />
          </div>

          {/* 其他搜索字段 */}
          {/* 管理员筛的是别人的令牌，拉不到候选值，仍走手输 */}
          {isAdminUser ? (
            <Form.Input
              field='token_name'
              prefix={<IconSearch />}
              placeholder={t('令牌名称')}
              showClear
              pure
              size='small'
            />
          ) : (
            <Form.Select
              key={`token-${optionsKey(tokenOptions)}`}
              field='token_name'
              placeholder={t('令牌名称')}
              optionList={tokenOptions}
              outerBottomSlot={
                tokenOptionsTruncated ? (
                  <div className='px-3 py-2 text-xs text-[var(--semi-color-text-2)]'>
                    {t('仅展示前 100 个令牌，可直接输入完整名称搜索')}
                  </div>
                ) : null
              }
              {...searchableSelectProps}
            />
          )}

          <Form.Select
            key={`model-${optionsKey(modelOptions)}`}
            field='model_name'
            placeholder={t('模型名称')}
            optionList={modelOptions}
            {...searchableSelectProps}
          />

          <Form.Select
            key={`group-${optionsKey(groupOptions)}`}
            field='group'
            placeholder={t('分组')}
            optionList={groupOptions}
            {...searchableSelectProps}
          />

          <Form.Input
            field='request_id'
            prefix={<IconSearch />}
            placeholder={t('Request ID')}
            showClear
            pure
            size='small'
          />

          {isAdminUser && (
            <>
              <Form.Input
                field='channel'
                prefix={<IconSearch />}
                placeholder={t('渠道 ID')}
                showClear
                pure
                size='small'
              />
              <Form.Input
                field='username'
                prefix={<IconSearch />}
                placeholder={t('用户名称')}
                showClear
                pure
                size='small'
              />
              <Form.Input
                field='user_id'
                prefix={<IconSearch />}
                placeholder={t('用户 ID')}
                showClear
                pure
                size='small'
              />
            </>
          )}
        </div>

        {/* 操作按钮区域 */}
        <div className='flex flex-col sm:flex-row justify-between items-start sm:items-center gap-3'>
          {/* 日志类型选择器 */}
          <div className='w-full sm:w-auto'>
            <Form.Select
              field='logType'
              placeholder={t('日志类型')}
              className='w-full sm:w-auto min-w-[120px]'
              showClear
              pure
              onChange={() => {
                // 延迟执行搜索，让表单值先更新
                setTimeout(() => {
                  refresh();
                }, 0);
              }}
              size='small'
            >
              <Form.Select.Option value='0'>{t('全部')}</Form.Select.Option>
              <Form.Select.Option value='1'>{t('充值')}</Form.Select.Option>
              <Form.Select.Option value='2'>{t('消费')}</Form.Select.Option>
              <Form.Select.Option value='3'>{t('管理')}</Form.Select.Option>
              <Form.Select.Option value='4'>{t('系统')}</Form.Select.Option>
              <Form.Select.Option value='5'>{t('错误')}</Form.Select.Option>
              <Form.Select.Option value='6'>{t('退款')}</Form.Select.Option>
            </Form.Select>
          </div>

          <div className='flex gap-2 w-full sm:w-auto justify-end'>
            <Button
              type='tertiary'
              htmlType='submit'
              loading={loading}
              size='small'
            >
              {t('查询')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => {
                if (formApi) {
                  formApi.reset();
                  setLogType(0);
                  setTimeout(() => {
                    refresh();
                  }, 100);
                }
              }}
              size='small'
            >
              {t('重置')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => setShowColumnSelector(true)}
              size='small'
            >
              {t('列设置')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default LogsFilters;
