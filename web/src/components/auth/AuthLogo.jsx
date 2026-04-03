import React from 'react';
import { Typography } from '@douyinfe/semi-ui';

const { Title } = Typography;

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

const AuthLogo = ({ logo, systemName, title }) => {
  return (
    <div className='flex items-center justify-center mb-8 gap-2'>
      {logo ? (
        <img src={logo} alt='Logo' className='h-10 rounded-full' />
      ) : (
        <DefaultSvgLogo />
      )}
      <Title heading={3} className='!mb-0 logo-text'>
        {systemName}
      </Title>
      {title && (
        <>
          <span className='text-gray-300 dark:text-zinc-600 font-light text-xl select-none'>/</span>
          <Title heading={3} className='!mb-0 !font-medium text-gray-800 dark:text-gray-200'>
            {title}
          </Title>
        </>
      )}
    </div>
  );
};

export default AuthLogo;
