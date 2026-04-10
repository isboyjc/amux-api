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

import React, { memo } from 'react';
import { Skeleton } from '@douyinfe/semi-ui';

const skeletonBlock = (style, key) => (
  <div
    key={key}
    className='animate-pulse'
    style={{
      background: 'var(--semi-color-fill-1)',
      borderRadius: 6,
      ...style,
    }}
  />
);

const PricingVendorIntroSkeleton = memo(
  ({ isMobile = false }) => {
    const placeholder = (
      <div className='flex flex-col gap-3'>
        {/* Title row */}
        <div className='flex items-center gap-3'>
          {/* Avatar */}
          {skeletonBlock(
            { width: 36, height: 36, borderRadius: 10, flexShrink: 0 },
            'avatar',
          )}
          <div className='flex-1 min-w-0'>
            {/* Title + tag */}
            <div className='flex items-center gap-2 mb-1.5'>
              {skeletonBlock({ width: 100, height: 18 }, 'title')}
              {skeletonBlock({ width: 72, height: 18, borderRadius: 9999 }, 'tag')}
            </div>
            {/* Description */}
            {skeletonBlock({ width: '80%', height: 13 }, 'desc')}
          </div>
        </div>

        {/* Search row */}
        <div className='flex items-center gap-2'>
          {skeletonBlock({ flex: 1, height: 32, borderRadius: 10 }, 'input')}
          {!isMobile && (
            skeletonBlock({ width: 200, height: 32, borderRadius: 10 }, 'controls')
          )}
          {isMobile && (
            skeletonBlock({ width: 72, height: 32, borderRadius: 10 }, 'filter')
          )}
        </div>
      </div>
    );

    return (
      <Skeleton loading={true} active placeholder={placeholder} />
    );
  },
);

PricingVendorIntroSkeleton.displayName = 'PricingVendorIntroSkeleton';

export default PricingVendorIntroSkeleton;
