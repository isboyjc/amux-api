import React, { useState, useRef, useEffect } from 'react';
import { useMinimumLoadingTime } from '../../../hooks/common/useMinimumLoadingTime';
import {
  Divider,
  Collapsible,
  Checkbox,
  Skeleton,
  Tooltip,
} from '@douyinfe/semi-ui';
import { IconChevronDown, IconChevronUp } from '@douyinfe/semi-icons';

const SelectableButtonGroup = ({
  title,
  items = [],
  activeValue,
  onChange,
  t = (v) => v,
  style = {},
  collapsible = true,
  collapseHeight = 200,
  withCheckbox = false,
  loading = false,
  variant,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const showSkeleton = useMinimumLoadingTime(loading);
  const needCollapse = collapsible && items.length > 8;

  const ConditionalTooltipText = ({ text }) => {
    const textRef = useRef(null);
    const [isOverflowing, setIsOverflowing] = useState(false);

    useEffect(() => {
      const el = textRef.current;
      if (!el) return;
      setIsOverflowing(el.scrollWidth > el.clientWidth);
    }, [text]);

    const textElement = (
      <span ref={textRef} className='sbg-ellipsis'>
        {text}
      </span>
    );

    return isOverflowing ? (
      <Tooltip content={text}>{textElement}</Tooltip>
    ) : (
      textElement
    );
  };

  const maskStyle = isOpen
    ? {}
    : {
        WebkitMaskImage:
          'linear-gradient(to bottom, black 0%, rgba(0,0,0,1) 60%, rgba(0,0,0,0.2) 80%, transparent 100%)',
      };

  const toggle = () => setIsOpen(!isOpen);

  const renderSkeletonItems = () => (
    <Skeleton loading={true} active placeholder={
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        {Array.from({ length: 6 }).map((_, i) => (
          <div
            key={i}
            style={{
              height: 34,
              borderRadius: 8,
              display: 'flex',
              alignItems: 'center',
              padding: '0 10px',
              gap: 8,
            }}
          >
            <Skeleton.Title active style={{ width: 16, height: 16, borderRadius: 4 }} />
            <Skeleton.Title active style={{ width: `${60 + (i % 3) * 25}px`, height: 13 }} />
          </div>
        ))}
      </div>
    } />
  );

  const renderItem = (item) => {
    const isActive = Array.isArray(activeValue)
      ? activeValue.includes(item.value)
      : activeValue === item.value;

    return (
      <div
        key={item.value}
        className={`sbg-nav-item ${isActive ? 'sbg-nav-item-active' : ''}`}
        onClick={() => onChange(item.value)}
        role='button'
        tabIndex={0}
      >
        {withCheckbox && (
          <Checkbox
            checked={isActive}
            onChange={(e) => {
              e.stopPropagation();
              onChange(item.value);
            }}
            style={{ pointerEvents: 'none', marginRight: 4 }}
          />
        )}
        {item.icon && <span className='sbg-nav-icon'>{item.icon}</span>}
        <ConditionalTooltipText text={item.label} />
        {item.tagCount !== undefined && item.tagCount !== '' && (
          <span className={`sbg-badge ${isActive ? 'sbg-badge-active' : ''}`}>
            {item.tagCount}
          </span>
        )}
      </div>
    );
  };

  const contentElement = showSkeleton ? (
    renderSkeletonItems()
  ) : (
    <div className='sbg-nav-list' style={style}>
      {items.map(renderItem)}
    </div>
  );

  const linkStyle = {
    textAlign: 'center',
    fontWeight: 400,
    cursor: 'pointer',
    fontSize: '12px',
    color: 'var(--semi-color-text-2)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 4,
    marginTop: 4,
  };

  return (
    <div
      className={`mb-6${variant ? ` sbg-variant-${variant}` : ''}`}
    >
      {title && (
        <div className='sbg-nav-title'>
          {showSkeleton ? (
            <Skeleton.Title active style={{ width: 60, height: 12 }} />
          ) : (
            title
          )}
        </div>
      )}
      {needCollapse && !showSkeleton ? (
        <div>
          <Collapsible
            isOpen={isOpen}
            collapseHeight={collapseHeight}
            style={maskStyle}
          >
            {contentElement}
          </Collapsible>
          {!isOpen && (
            <div onClick={toggle} style={linkStyle}>
              <IconChevronDown size='small' />
              <span>{t('展开更多')}</span>
            </div>
          )}
          {isOpen && (
            <div onClick={toggle} style={linkStyle}>
              <IconChevronUp size='small' />
              <span>{t('收起')}</span>
            </div>
          )}
        </div>
      ) : (
        contentElement
      )}
    </div>
  );
};

export default SelectableButtonGroup;
