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

import React, { useContext, useEffect, useState } from 'react';
import {
  Button,
  Typography,
  Input,
  ScrollList,
  ScrollItem,
} from '@douyinfe/semi-ui';
import { API, showError, copy, showSuccess } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { API_ENDPOINTS } from '../../constants/common.constant';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import {
  IconGithubLogo,
  IconPlay,
  IconFile,
  IconCopy,
} from '@douyinfe/semi-icons';
import { Link } from 'react-router-dom';
import NoticeModal from '../../components/layout/NoticeModal';
import {
  Moonshot,
  OpenAI,
  XAI,
  Zhipu,
  Volcengine,
  Cohere,
  Claude,
  Gemini,
  Suno,
  Minimax,
  Wenxin,
  Spark,
  Qingyan,
  DeepSeek,
  Qwen,
  Midjourney,
  Grok,
  AzureAI,
  Hunyuan,
  Xinference,
  Meta,
  Mistral,
  Perplexity,
  Groq,
  Stepfun,
  Baichuan,
  Doubao,
  Yi,
  Google,
  Anthropic,
} from '@lobehub/icons';
import LogoLoop from '../../components/common/ui/LogoLoop';
import Counter from '../../components/common/ui/Counter';

const { Text } = Typography;

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const isMobile = useIsMobile();
  const isDemoSiteMode = statusState?.status?.demo_site_enabled || false;
  const docsLink = statusState?.status?.docs_link || '';
  const serverAddress =
    statusState?.status?.server_address || `${window.location.origin}`;
  const endpointItems = API_ENDPOINTS.map((e) => ({ value: e }));
  const [endpointIndex, setEndpointIndex] = useState(0);
  const isChinese = i18n.language.startsWith('zh');
  const [tokenDisplay, setTokenDisplay] = useState({ value: '0', unit: '', ready: false });

  const formatTokens = (tokens) => {
    if (tokens >= 1_000_000_000_000) {
      return { value: (tokens / 1_000_000_000_000).toFixed(2), unit: 'T' };
    }
    if (tokens >= 1_000_000_000) {
      return { value: (tokens / 1_000_000_000).toFixed(2), unit: 'B' };
    }
    if (tokens >= 1_000_000) {
      return { value: (tokens / 1_000_000).toFixed(2), unit: 'M' };
    }
    if (tokens >= 1_000) {
      return { value: (tokens / 1_000).toFixed(2), unit: 'K' };
    }
    return { value: String(tokens), unit: '' };
  };

  const fetchTokenStats = async () => {
    try {
      const res = await API.get('/api/public/token_stats');
      if (res.data.success) {
        const tokens = res.data.data.total_tokens || 0;
        const formatted = formatTokens(tokens);
        setTokenDisplay({ ...formatted, ready: true });
      }
    } catch (e) {
      // silent
    }
  };

  const displayHomePageContent = async () => {
    setHomePageContent(localStorage.getItem('home_page_content') || '');
    const res = await API.get('/api/home_page_content');
    const { success, message, data } = res.data;
    if (success) {
      let content = data;
      if (!data.startsWith('https://')) {
        content = marked.parse(data);
      }
      setHomePageContent(content);
      localStorage.setItem('home_page_content', content);

      // 如果内容是 URL，则发送主题模式
      if (data.startsWith('https://')) {
        const iframe = document.querySelector('iframe');
        if (iframe) {
          iframe.onload = () => {
            iframe.contentWindow.postMessage({ themeMode: actualTheme }, '*');
            iframe.contentWindow.postMessage({ lang: i18n.language }, '*');
          };
        }
      }
    } else {
      showError(message);
      setHomePageContent('加载首页内容失败...');
    }
    setHomePageContentLoaded(true);
  };

  const handleCopyBaseURL = async () => {
    const ok = await copy(serverAddress);
    if (ok) {
      showSuccess(t('已复制到剪切板'));
    }
  };

  useEffect(() => {
    const checkNoticeAndShow = async () => {
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const today = new Date().toDateString();
      if (lastCloseDate !== today) {
        try {
          const res = await API.get('/api/notice');
          const { success, data } = res.data;
          if (success && data && data.trim() !== '') {
            setNoticeVisible(true);
          }
        } catch (error) {
          console.error('获取公告失败:', error);
        }
      }
    };

    checkNoticeAndShow();
    fetchTokenStats();
  }, []);

  useEffect(() => {
    displayHomePageContent().then();
  }, []);

  useEffect(() => {
    const timer = setInterval(() => {
      setEndpointIndex((prev) => (prev + 1) % endpointItems.length);
    }, 3000);
    return () => clearInterval(timer);
  }, [endpointItems.length]);

  return (
    <div className='w-full overflow-x-hidden'>
      <NoticeModal
        visible={noticeVisible}
        onClose={() => setNoticeVisible(false)}
        isMobile={isMobile}
      />
      {homePageContentLoaded && homePageContent === '' ? (
        <div className='w-full overflow-x-hidden'>
          {/* 首屏 Hero —— 全屏高度（减去导航栏 64px） */}
          <div className='w-full relative overflow-hidden flex flex-col' style={{ height: '100dvh', minHeight: '600px' }}>
            {/* 背景模糊晕染球 */}
            <div className='blur-ball blur-ball-indigo' />
            <div className='blur-ball blur-ball-teal' />

            {/* 主内容：垂直居中 */}
            <div className='flex-1 flex items-center justify-center px-4'>
              <div className='flex flex-col items-center justify-center text-center max-w-5xl mx-auto'>
                <h1
                  className={`text-4xl md:text-5xl lg:text-6xl xl:text-7xl font-bold text-semi-color-text-0 leading-tight ${isChinese ? 'tracking-wide md:tracking-wider' : ''}`}
                >
                  {t('统一、稳定的')}
                  <br />
                  <span className='shine-text'>{t('企业级大模型接口网关')}</span>
                </h1>

                <p className='text-base md:text-lg lg:text-xl text-semi-color-text-2 mt-6 md:mt-8 max-w-3xl'>
                  {t('为个人与企业用户提供更优价格与企业级稳定性，只需替换模型基址即可接入')}
                </p>

                {/* BASE URL */}
                <div className='flex items-center justify-center w-full mt-6 md:mt-8 max-w-lg'>
                  <Input
                    readonly
                    value={serverAddress}
                    className='flex-1'
                    size={isMobile ? 'default' : 'large'}
                    suffix={
                      <div className='flex items-center gap-2'>
                        <ScrollList
                          bodyHeight={32}
                          style={{ border: 'unset', boxShadow: 'unset' }}
                        >
                          <ScrollItem
                            mode='wheel'
                            cycled={true}
                            list={endpointItems}
                            selectedIndex={endpointIndex}
                            onSelect={({ index }) => setEndpointIndex(index)}
                          />
                        </ScrollList>
                        <Button
                          type='primary'
                          onClick={handleCopyBaseURL}
                          icon={<IconCopy />}
                        />
                      </div>
                    }
                  />
                </div>

                {/* 操作按钮 */}
                <div className='flex flex-row gap-4 justify-center items-center mt-8 md:mt-10'>
                  <Link to='/console'>
                    <Button
                      theme='solid'
                      type='primary'
                      size={isMobile ? 'default' : 'large'}
                      className='px-8 py-2'
                      icon={<IconPlay />}
                    >
                      {t('获取密钥')}
                    </Button>
                  </Link>
                  {isDemoSiteMode && statusState?.status?.version ? (
                    <Button
                      size={isMobile ? 'default' : 'large'}
                      className='flex items-center px-6 py-2'
                      icon={<IconGithubLogo />}
                      onClick={() =>
                        window.open(
                          'https://github.com/QuantumNous/new-api',
                          '_blank',
                        )
                      }
                    >
                      {statusState.status.version}
                    </Button>
                  ) : (
                    docsLink && (
                      <Button
                        size={isMobile ? 'default' : 'large'}
                        className='flex items-center px-6 py-2'
                        icon={<IconFile />}
                        onClick={() => window.open(docsLink, '_blank')}
                      >
                        {t('文档')}
                      </Button>
                    )
                  )}
                </div>
              </div>
            </div>

            {/* 供应商图标 —— 吸底滚动展示 */}
            <div className='w-full pb-4 md:pb-6'>
              <div className='flex items-center justify-center mb-4 md:mb-6 gap-1 flex-wrap'>
                <Text type='tertiary' className='text-sm md:text-base font-light'>
                  {t('支持全球众多的大模型供应商')}
                </Text>
                {tokenDisplay.ready && (
                  <>
                    <Text type='tertiary' className='text-sm md:text-base font-light'>
                      ·
                    </Text>
                    <Text type='tertiary' className='text-sm md:text-base font-light'>
                      {t('累计处理约')}
                    </Text>
                    <Counter
                      value={tokenDisplay.value}
                      className='text-base md:text-lg font-bold text-semi-color-text-0'
                    />
                    <Text type='tertiary' className='text-sm md:text-base font-light'>
                      {tokenDisplay.unit} Tokens
                    </Text>
                  </>
                )}
              </div>
              <div className='provider-icons max-w-5xl mx-auto px-4 md:px-10 space-y-4'>
                <LogoLoop
                  items={[
                    <OpenAI size={32} />,
                    <Claude.Color size={32} />,
                    <Gemini.Color size={32} />,
                    <Meta size={32} />,
                    <Mistral.Color size={32} />,
                    <DeepSeek.Color size={32} />,
                    <Qwen.Color size={32} />,
                    <Zhipu.Color size={32} />,
                    <Moonshot size={32} />,
                    <XAI size={32} />,
                    <Perplexity.Color size={32} />,
                    <Groq size={32} />,
                    <Google.Color size={32} />,
                    <Anthropic size={32} />,
                    <Cohere.Color size={32} />,
                  ]}
                  speed={30}
                  direction='left'
                  logoHeight={32}
                  gap={44}
                  pauseOnHover
                  fadeOut
                />
                <LogoLoop
                  items={[
                    <Volcengine.Color size={32} />,
                    <Wenxin.Color size={32} />,
                    <Spark.Color size={32} />,
                    <Doubao size={32} />,
                    <Baichuan.Color size={32} />,
                    <Yi.Color size={32} />,
                    <Stepfun.Color size={32} />,
                    <Minimax.Color size={32} />,
                    <Hunyuan.Color size={32} />,
                    <Qingyan.Color size={32} />,
                    <AzureAI.Color size={32} />,
                    <Midjourney size={32} />,
                    <Grok size={32} />,
                    <Suno size={32} />,
                    <Xinference.Color size={32} />,
                  ]}
                  speed={30}
                  direction='right'
                  logoHeight={32}
                  gap={44}
                  pauseOnHover
                  fadeOut
                />
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className='overflow-x-hidden w-full'>
          {homePageContent.startsWith('https://') ? (
            <iframe
              src={homePageContent}
              className='w-full h-screen border-none'
            />
          ) : (
            <div
              className='mt-[60px]'
              dangerouslySetInnerHTML={{ __html: homePageContent }}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
