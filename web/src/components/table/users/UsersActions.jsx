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
import { Button, Popconfirm } from '@douyinfe/semi-ui';

const UsersActions = ({
  setShowAddUser,
  selectedUsers,
  setSelectedUsers,
  batchManageUsers,
  loading,
  t,
}) => {
  const selectedCount = selectedUsers.length;

  // Add new user
  const handleAddUser = () => {
    setShowAddUser(true);
  };

  return (
    <div className='flex flex-wrap gap-2 w-full md:w-auto order-2 md:order-1'>
      <Button
        className='flex-1 md:flex-initial'
        onClick={handleAddUser}
        size='small'
      >
        {t('添加用户')}
      </Button>

      {selectedCount > 0 && (
        <>
          <Popconfirm
            title={t('批量启用')}
            content={t('确定要批量启用选中的 {{count}} 个用户吗？', {
              count: selectedCount,
            })}
            okText={t('启用')}
            cancelText={t('取消')}
            onConfirm={() => batchManageUsers('enable')}
          >
            <Button
              type='primary'
              className='flex-1 md:flex-initial'
              loading={loading}
              size='small'
            >
              {t('批量启用')} ({selectedCount})
            </Button>
          </Popconfirm>

          <Popconfirm
            title={t('批量禁用')}
            content={t('确定要批量禁用选中的 {{count}} 个用户吗？', {
              count: selectedCount,
            })}
            okText={t('禁用')}
            cancelText={t('取消')}
            onConfirm={() => batchManageUsers('disable')}
          >
            <Button
              type='warning'
              className='flex-1 md:flex-initial'
              loading={loading}
              size='small'
            >
              {t('批量禁用')} ({selectedCount})
            </Button>
          </Popconfirm>

          <Popconfirm
            title={t('批量删除')}
            content={t(
              '确定要批量删除选中的 {{count}} 个用户吗？此操作不可撤销。',
              { count: selectedCount },
            )}
            okText={t('删除')}
            cancelText={t('取消')}
            onConfirm={() => batchManageUsers('delete')}
          >
            <Button
              type='danger'
              className='flex-1 md:flex-initial'
              loading={loading}
              size='small'
            >
              {t('批量删除')} ({selectedCount})
            </Button>
          </Popconfirm>

          <Button
            type='tertiary'
            className='flex-1 md:flex-initial'
            disabled={loading}
            onClick={() => setSelectedUsers([])}
            size='small'
          >
            {t('取消选择')}
          </Button>
        </>
      )}
    </div>
  );
};

export default UsersActions;
