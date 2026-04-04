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

import React, { useEffect, useState, useMemo, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { Typography } from '@douyinfe/semi-ui';
import { getFooterHTML, getLogo, getSystemName } from '../../helpers';
import { StatusContext } from '../../context/Status';

const DefaultSvgLogo = () => (
  <svg
    width='128'
    height='128'
    viewBox='0 0 128 128'
    fill='none'
    xmlns='http://www.w3.org/2000/svg'
    className='w-8 h-8 text-zinc-900 dark:text-white'
  >
    <path
      d='M4 96 C4 96, 24 12, 64 12 C104 12, 124 96, 124 96 Q124 102, 118 102 C94 102, 92 64, 64 64 C36 64, 34 102, 10 102 Q4 102, 4 96 Z'
      fill='currentColor'
    />
  </svg>
);

const FooterBar = () => {
  const { t } = useTranslation();
  const [footer, setFooter] = useState(getFooterHTML());
  const systemName = getSystemName();
  const logo = getLogo();
  const [statusState] = useContext(StatusContext);
  const docsLink = statusState?.status?.docs_link || '';
  const version = statusState?.status?.version || '';

  const loadFooter = () => {
    let footer_html = localStorage.getItem('footer_html');
    if (footer_html) {
      setFooter(footer_html);
    }
  };

  const currentYear = new Date().getFullYear();

  const customFooter = useMemo(
    () => (
      <footer className='w-full border-t border-semi-color-border'>
        <div className='max-w-6xl mx-auto px-6 md:px-12 py-12 md:py-16'>
          <div className='flex flex-col md:flex-row justify-between gap-10 md:gap-16'>
            {/* 左侧：Logo + 标语 */}
            <div className='flex flex-col gap-3 md:max-w-xs'>
              <div className='flex items-center gap-2.5'>
                {logo ? (
                  <img
                    src={logo}
                    alt={systemName}
                    className='w-8 h-8 rounded-lg object-contain'
                  />
                ) : (
                  <DefaultSvgLogo />
                )}
                <span className='text-base font-bold text-semi-color-text-0 logo-text'>
                  {systemName}
                </span>
              </div>
              <Typography.Text className='text-sm !text-semi-color-text-2 leading-relaxed'>
                {t('为个人与企业用户提供更优价格与企业级稳定性，只需替换模型基址即可接入')}
              </Typography.Text>
            </div>

            {/* 右侧：链接列 */}
            <div className='grid grid-cols-2 sm:grid-cols-3 gap-8 md:gap-12'>
              {/* 产品 */}
              <div>
                <p className='text-sm font-semibold text-semi-color-text-0 mb-4'>
                  {t('产品')}
                </p>
                <div className='flex flex-col gap-2.5'>
                  <Link
                    to='/console'
                    className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                  >
                    {t('控制台')}
                  </Link>
                  <Link
                    to='/pricing'
                    className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                  >
                    {t('模型')}
                  </Link>
                  {docsLink && (
                    <a
                      href={docsLink}
                      target='_blank'
                      rel='noopener noreferrer'
                      className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                    >
                      {t('文档')}
                    </a>
                  )}
                </div>
              </div>

              {/* 支持 */}
              <div>
                <p className='text-sm font-semibold text-semi-color-text-0 mb-4'>
                  {t('支持')}
                </p>
                <div className='flex flex-col gap-2.5'>
                  <Link
                    to='/login'
                    className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                  >
                    {t('登录')}
                  </Link>
                  <Link
                    to='/register'
                    className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                  >
                    {t('注册')}
                  </Link>
                </div>
              </div>

              {/* 法律 */}
              <div>
                <p className='text-sm font-semibold text-semi-color-text-0 mb-4'>
                  {t('法律')}
                </p>
                <div className='flex flex-col gap-2.5'>
                  <Link
                    to='/user-agreement'
                    className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                  >
                    {t('用户协议')}
                  </Link>
                  <Link
                    to='/privacy-policy'
                    className='text-sm text-semi-color-text-2 hover:text-semi-color-text-0 transition-colors'
                  >
                    {t('隐私政策')}
                  </Link>
                </div>
              </div>
            </div>
          </div>

          {/* 底部版权 */}
          <div className='mt-10 pt-6 border-t border-semi-color-border flex flex-col sm:flex-row items-center justify-between gap-3'>
            <Typography.Text className='text-xs !text-semi-color-text-3'>
              © {currentYear} {systemName}. {t('版权所有')}
            </Typography.Text>
            <a
              href='mailto:support@amux.ai'
              className='text-xs text-semi-color-text-3 hover:text-semi-color-text-1 transition-colors'
            >
              support@amux.ai
            </a>
          </div>
        </div>
      </footer>
    ),
    [logo, systemName, t, currentYear, docsLink, version],
  );

  useEffect(() => {
    loadFooter();
  }, []);

  return (
    <div className='w-full'>
      {footer ? (
        <div className='relative'>
          <div
            className='custom-footer'
            dangerouslySetInnerHTML={{ __html: footer }}
          ></div>
        </div>
      ) : (
        customFooter
      )}
    </div>
  );
};

export default FooterBar;
