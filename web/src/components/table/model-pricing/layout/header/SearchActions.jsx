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

import React, { memo, useCallback } from 'react';
import { Input, Button, Divider, Pagination, Dropdown } from '@douyinfe/semi-ui';
import { IconSearch, IconFilter } from '@douyinfe/semi-icons';

const SearchActions = memo(
  ({
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    isMobile = false,
    searchValue = '',
    setShowFilterModal,
    viewMode,
    setViewMode,
    timeRange,
    setTimeRange,
    total = 0,
    currentPage = 1,
    setCurrentPage,
    pageSize = 20,
    setPageSize,
    t,
  }) => {
    const handleFilterClick = useCallback(() => {
      setShowFilterModal?.(true);
    }, [setShowFilterModal]);

    const handleViewModeToggle = useCallback(() => {
      setViewMode?.(viewMode === 'table' ? 'card' : 'table');
    }, [viewMode, setViewMode]);

    const handleTimeRangeToggle = useCallback(() => {
      setTimeRange?.(timeRange === '24h' ? '7d' : '24h');
    }, [timeRange, setTimeRange]);

    return (
      <div className='flex items-center gap-2 w-full'>
        {/* 左侧：搜索框 + 设置项 */}
        <div className='w-52 flex-shrink-0'>
          <Input
            prefix={<IconSearch />}
            placeholder={t('模糊搜索模型名称')}
            value={searchValue}
            onCompositionStart={handleCompositionStart}
            onCompositionEnd={handleCompositionEnd}
            onChange={handleChange}
            showClear
          />
        </div>

        {!isMobile && (
          <>
            <Divider layout='vertical' margin='4px' />

            <div
              className='flex items-center gap-1 px-2 py-1 rounded-xl flex-shrink-0'
              style={{ background: 'var(--semi-color-fill-0)' }}
            >
              {/* 视图模式切换按钮 */}
              <Button
                theme={viewMode === 'table' ? 'solid' : 'borderless'}
                type={viewMode === 'table' ? 'primary' : 'tertiary'}
                size='small'
                onClick={handleViewModeToggle}
              >
                {t('表格视图')}
              </Button>

              <div
                className='self-stretch w-px mx-1'
                style={{ background: 'var(--semi-color-border)' }}
              />

              {/* 健康状态时间范围切换 */}
              <Button
                theme='borderless'
                type='tertiary'
                size='small'
                onClick={handleTimeRangeToggle}
                style={{ minWidth: 40 }}
              >
                {timeRange === '24h' ? t('最近24小时') : t('最近7天')}
              </Button>
            </div>

            {/* 右侧：页大小 + 分页（推到最右） */}
            {total > 0 && setCurrentPage && (
              <div className='ml-auto flex-shrink-0 flex items-center gap-2'>
                <Dropdown
                  position='bottomLeft'
                  render={
                    <Dropdown.Menu>
                      {[10, 20, 50, 100].map((s) => (
                        <Dropdown.Item
                          key={s}
                          active={pageSize === s}
                          onClick={() => {
                            setPageSize(s);
                            setCurrentPage(1);
                          }}
                        >
                          {s} {t('条/页')}
                        </Dropdown.Item>
                      ))}
                    </Dropdown.Menu>
                  }
                >
                  <Button theme='outline' type='tertiary'>
                    {pageSize} {t('条/页')}
                  </Button>
                </Dropdown>
                <Pagination
                  currentPage={currentPage}
                  pageSize={pageSize}
                  total={total}
                  showSizeChanger={false}
                  onPageChange={setCurrentPage}
                />
              </div>
            )}
          </>
        )}

        {isMobile && (
          <Button
            theme='outline'
            type='tertiary'
            icon={<IconFilter />}
            onClick={handleFilterClick}
          >
            {t('筛选')}
          </Button>
        )}
      </div>
    );
  },
);

SearchActions.displayName = 'SearchActions';

export default SearchActions;
