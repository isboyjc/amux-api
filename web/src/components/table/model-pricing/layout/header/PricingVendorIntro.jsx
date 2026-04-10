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

import React, { useState, useEffect, useMemo, useCallback, memo } from 'react';
import { Tag, Avatar, Typography, Tooltip, Modal } from '@douyinfe/semi-ui';
import { getLobeHubIcon } from '../../../../../helpers';
import SearchActions from './SearchActions';

const { Paragraph } = Typography;

const CONFIG = {
  CAROUSEL_INTERVAL: 2000,
  ICON_SIZE: 32,
  UNKNOWN_VENDOR: 'unknown',
};

const CONTENT_TEXTS = {
  unknown: {
    displayName: (t) => t('未知供应商'),
    description: (t) =>
      t(
        '包含来自未知或未标明供应商的AI模型，这些模型可能来自小型供应商或开源项目。',
      ),
  },
  all: {
    description: (t) =>
      t('查看所有可用的AI模型供应商，包括众多知名供应商的模型。'),
  },
  fallback: {
    description: (t) => t('该供应商提供多种AI模型，适用于不同的应用场景。'),
  },
};

const getVendorDisplayName = (vendorName, t) =>
  vendorName === CONFIG.UNKNOWN_VENDOR
    ? CONTENT_TEXTS.unknown.displayName(t)
    : vendorName;

const getAvatarText = (vendorName) =>
  vendorName === CONFIG.UNKNOWN_VENDOR
    ? '?'
    : vendorName.charAt(0).toUpperCase();

const createAvatarContent = (vendor) => {
  if (vendor.icon) {
    return (
      <div className='w-8 h-8 flex items-center justify-center'>
        {getLobeHubIcon(vendor.icon, CONFIG.ICON_SIZE)}
      </div>
    );
  }
  return (
    <Avatar
      size='large'
      style={{
        width: 48,
        height: 48,
        borderRadius: 12,
        fontSize: 16,
        fontWeight: 'bold',
      }}
    >
      {getAvatarText(vendor.name)}
    </Avatar>
  );
};

const renderVendorAvatar = (vendor, t) => {
  const containerClass =
    'w-12 h-12 rounded-xl flex items-center justify-center overflow-hidden flex-shrink-0';
  const containerStyle = {
    backgroundColor: 'var(--semi-color-fill-0)',
    border: '1px solid var(--semi-color-border)',
  };

  if (!vendor) {
    return (
      <div className={containerClass} style={containerStyle}>
        <Avatar size='large'>AI</Avatar>
      </div>
    );
  }

  const displayName = getVendorDisplayName(vendor.name, t);
  return (
    <Tooltip content={displayName} position='top'>
      <div className={containerClass} style={containerStyle}>
        {createAvatarContent(vendor)}
      </div>
    </Tooltip>
  );
};

const PricingVendorIntro = memo(
  ({
    filterVendor,
    models = [],
    allModels = [],
    t,
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    isMobile = false,
    searchValue = '',
    setShowFilterModal,
    showRatio,
    setShowRatio,
    viewMode,
    setViewMode,
    tokenUnit,
    setTokenUnit,
    timeRange,
    setTimeRange,
    currentPage,
    setCurrentPage,
    pageSize,
    setPageSize,
  }) => {
    const [currentOffset, setCurrentOffset] = useState(0);
    const [descModalVisible, setDescModalVisible] = useState(false);
    const [descModalContent, setDescModalContent] = useState('');

    const handleOpenDescModal = useCallback((content) => {
      setDescModalContent(content || '');
      setDescModalVisible(true);
    }, []);

    const handleCloseDescModal = useCallback(() => {
      setDescModalVisible(false);
    }, []);

    const renderDescriptionModal = useCallback(
      () => (
        <Modal
          title={t('供应商介绍')}
          visible={descModalVisible}
          onCancel={handleCloseDescModal}
          footer={null}
          width={isMobile ? '95%' : 600}
          bodyStyle={{
            maxHeight: isMobile ? '70vh' : '60vh',
            overflowY: 'auto',
          }}
        >
          <div className='text-sm mb-4'>{descModalContent}</div>
        </Modal>
      ),
      [descModalVisible, descModalContent, handleCloseDescModal, isMobile, t],
    );

    const vendorInfo = useMemo(() => {
      const vendors = new Map();
      let unknownCount = 0;

      const sourceModels =
        Array.isArray(allModels) && allModels.length > 0 ? allModels : models;

      sourceModels.forEach((model) => {
        if (model.vendor_name) {
          const existing = vendors.get(model.vendor_name);
          if (existing) {
            existing.count++;
          } else {
            vendors.set(model.vendor_name, {
              name: model.vendor_name,
              icon: model.vendor_icon,
              description: model.vendor_description,
              count: 1,
            });
          }
        } else {
          unknownCount++;
        }
      });

      const vendorList = Array.from(vendors.values()).sort((a, b) =>
        a.name.localeCompare(b.name),
      );

      if (unknownCount > 0) {
        vendorList.push({
          name: CONFIG.UNKNOWN_VENDOR,
          icon: null,
          description: CONTENT_TEXTS.unknown.description(t),
          count: unknownCount,
        });
      }

      return vendorList;
    }, [allModels, models, t]);

    const currentModelCount = models.length;

    useEffect(() => {
      if (filterVendor !== 'all' || vendorInfo.length <= 1) {
        setCurrentOffset(0);
        return;
      }

      const interval = setInterval(() => {
        setCurrentOffset((prev) => (prev + 1) % vendorInfo.length);
      }, CONFIG.CAROUSEL_INTERVAL);

      return () => clearInterval(interval);
    }, [filterVendor, vendorInfo.length]);

    const getVendorDescription = useCallback(
      (vendorKey) => {
        if (vendorKey === 'all') {
          return CONTENT_TEXTS.all.description(t);
        }
        if (vendorKey === CONFIG.UNKNOWN_VENDOR) {
          return CONTENT_TEXTS.unknown.description(t);
        }
        const vendor = vendorInfo.find((v) => v.name === vendorKey);
        return vendor?.description || CONTENT_TEXTS.fallback.description(t);
      },
      [vendorInfo, t],
    );

    const renderSearchActions = useCallback(
      () => (
        <SearchActions
          handleChange={handleChange}
          handleCompositionStart={handleCompositionStart}
          handleCompositionEnd={handleCompositionEnd}
          isMobile={isMobile}
          searchValue={searchValue}
          setShowFilterModal={setShowFilterModal}
          showRatio={showRatio}
          setShowRatio={setShowRatio}
          viewMode={viewMode}
          setViewMode={setViewMode}
          tokenUnit={tokenUnit}
          setTokenUnit={setTokenUnit}
          timeRange={timeRange}
          setTimeRange={setTimeRange}
          total={currentModelCount}
          currentPage={currentPage}
          setCurrentPage={setCurrentPage}
          pageSize={pageSize}
          setPageSize={setPageSize}
          t={t}
        />
      ),
      [
        handleChange,
        handleCompositionStart,
        handleCompositionEnd,
        isMobile,
        searchValue,
        setShowFilterModal,
        showRatio,
        setShowRatio,
        viewMode,
        setViewMode,
        tokenUnit,
        setTokenUnit,
        timeRange,
        setTimeRange,
        currentModelCount,
        currentPage,
        setCurrentPage,
        pageSize,
        setPageSize,
        t,
      ],
    );

    const renderHeaderSection = useCallback(
      ({ title, count, description, avatarContent }) => (
        <div className='flex flex-col gap-3'>
          <div className='flex items-center gap-3'>
            {avatarContent}
            <div className='flex-1 min-w-0'>
              <div className='flex items-center gap-2 flex-wrap mb-0.5'>
                <h2
                  className='text-base font-semibold'
                  style={{ color: 'var(--semi-color-text-0)' }}
                >
                  {title}
                </h2>
                <Tag
                  size='small'
                  shape='circle'
                  style={{
                    background: 'var(--semi-color-fill-1)',
                    color: 'var(--semi-color-text-1)',
                    border: '1px solid var(--semi-color-border)',
                    fontWeight: 500,
                  }}
                >
                  {t('共 {{count}} 个模型', { count })}
                </Tag>
              </div>
              <Paragraph
                className='text-xs leading-normal !mb-0 cursor-pointer'
                style={{ color: 'var(--semi-color-text-2)' }}
                ellipsis={{ rows: 1 }}
                onClick={() => handleOpenDescModal(description)}
              >
                {description}
              </Paragraph>
            </div>
          </div>
          {renderSearchActions()}
        </div>
      ),
      [renderSearchActions, handleOpenDescModal, t],
    );

    const renderAllVendorsAvatar = useCallback(() => {
      const currentVendor =
        vendorInfo.length > 0
          ? vendorInfo[currentOffset % vendorInfo.length]
          : null;
      return renderVendorAvatar(currentVendor, t);
    }, [vendorInfo, currentOffset, t]);

    if (filterVendor === 'all') {
      return (
        <>
          {renderHeaderSection({
            title: t('全部供应商'),
            count: currentModelCount,
            description: getVendorDescription('all'),
            avatarContent: renderAllVendorsAvatar(),
          })}
          {renderDescriptionModal()}
        </>
      );
    }

    const currentVendor = vendorInfo.find((v) => v.name === filterVendor);
    if (!currentVendor) {
      return null;
    }

    const vendorDisplayName = getVendorDisplayName(currentVendor.name, t);

    return (
      <>
        {renderHeaderSection({
          title: vendorDisplayName,
          count: currentModelCount,
          description:
            currentVendor.description ||
            getVendorDescription(currentVendor.name),
          avatarContent: renderVendorAvatar(currentVendor, t),
        })}
        {renderDescriptionModal()}
      </>
    );
  },
);

PricingVendorIntro.displayName = 'PricingVendorIntro';

export default PricingVendorIntro;
