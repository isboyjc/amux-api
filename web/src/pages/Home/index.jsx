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

import React, { useContext, useEffect, useState, useRef } from 'react';
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
  Layers,
  Zap,
  Shield,
  CreditCard,
  Users,
  Headset,
  ArrowRight,
  MessageSquare,
  Image,
  Headphones,
  Code2,
  FileText,
  Search,
} from 'lucide-react';
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

// Intersection Observer hook for scroll animations - defined outside components
const useInView = (options = {}) => {
  const ref = useRef(null);
  const [inView, setInView] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setInView(true);
          observer.unobserve(el);
        }
      },
      { threshold: 0.15, ...options },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return [ref, inView];
};

const FeaturesSection = ({ t }) => {
  const [ref, inView] = useInView();
  const features = [
    {
      icon: <Layers size={24} />,
      title: t('多供应商聚合'),
      desc: t('homepage_feature_aggregate_desc'),
      color: 'from-violet-500 to-purple-600',
    },
    {
      icon: <Zap size={24} />,
      title: t('智能熔断调度'),
      desc: t('homepage_feature_failover_desc'),
      color: 'from-amber-500 to-orange-600',
    },
    {
      icon: <Shield size={24} />,
      title: t('企业级稳定性'),
      desc: t('homepage_feature_enterprise_desc'),
      color: 'from-emerald-500 to-teal-600',
    },
    {
      icon: <CreditCard size={24} />,
      title: t('灵活计费'),
      desc: t('homepage_feature_billing_desc'),
      color: 'from-blue-500 to-indigo-600',
    },
    {
      icon: <Users size={24} />,
      title: t('多样化订阅'),
      desc: t('homepage_feature_subscription_desc'),
      color: 'from-pink-500 to-rose-600',
    },
    {
      icon: <Headset size={24} />,
      title: t('社群与企业支持'),
      desc: t('homepage_feature_support_desc'),
      color: 'from-cyan-500 to-blue-600',
    },
  ];

  return (
    <section ref={ref} className='py-20 md:py-32 px-4'>
      <div className='max-w-6xl mx-auto'>
        <div className={`text-center mb-16 transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
          <h2 className='text-3xl md:text-4xl font-bold text-semi-color-text-0 mb-4'>
            {t('为什么选择我们')}
          </h2>
          <p className='text-semi-color-text-2 text-lg max-w-2xl mx-auto'>
            {t('homepage_features_subtitle')}
          </p>
        </div>
        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'>
          {features.map((f, i) => (
            <div
              key={i}
              className='group relative rounded-2xl p-6 md:p-8 transition-all duration-700 cursor-default hover:scale-[1.02]'
              style={{
                background: 'var(--semi-color-fill-0)',
                transitionDelay: `${i * 80}ms`,
                opacity: inView ? 1 : 0,
                transform: inView ? 'translateY(0)' : 'translateY(32px)',
              }}
            >
              <div className={`w-12 h-12 rounded-xl bg-gradient-to-br ${f.color} flex items-center justify-center text-white mb-5 group-hover:scale-110 transition-transform duration-300`}>
                {f.icon}
              </div>
              <h3 className='text-xl font-semibold text-semi-color-text-0 mb-3'>
                {f.title}
              </h3>
              <p className='text-semi-color-text-2 leading-relaxed text-sm'>
                {f.desc}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

const EndpointsSection = ({ t }) => {
  const [ref, inView] = useInView();

  const unified = [
    {
      icon: <MessageSquare size={18} />,
      label: t('对话补全'),
      endpoints: ['/v1/chat/completions', '/v1/completions', '/v1/responses'],
    },
    {
      icon: <Search size={18} />,
      label: t('向量与重排'),
      endpoints: ['/v1/embeddings', '/v1/rerank'],
    },
    {
      icon: <Image size={18} />,
      label: t('图像生成'),
      endpoints: ['/v1/images/generations', '/v1/images/edits'],
    },
    {
      icon: <Headphones size={18} />,
      label: t('语音处理'),
      endpoints: ['/v1/audio/speech', '/v1/audio/transcriptions'],
    },
  ];

  const nativeFormats = [
    { label: 'Claude', path: '/v1/messages' },
    { label: 'Gemini', path: '/v1beta/models/{model}:{action}' },
    { label: 'Midjourney', path: '/mj/submit/*' },
    { label: 'Suno', path: '/suno/submit/*' },
    { label: 'Realtime', path: '/v1/realtime' },
  ];

  return (
    <section ref={ref} className='py-20 md:py-32 px-4'>
      <div className='max-w-6xl mx-auto'>
        <div className={`text-center mb-16 transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
          <h2 className='text-3xl md:text-4xl font-bold text-semi-color-text-0 mb-4'>
            {t('覆盖主流 API 端点')}
          </h2>
          <p className='text-semi-color-text-2 text-lg max-w-2xl mx-auto'>
            {t('homepage_endpoints_subtitle')}
          </p>
        </div>

        {/* OpenAI 统一格式 */}
        <div
          className={`mb-6 transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
          style={{ transitionDelay: '100ms' }}
        >
          <div className='flex items-center gap-2 mb-4'>
            <span className='text-xs font-medium px-2.5 py-1 rounded-full bg-gradient-to-r from-violet-500 to-purple-600 text-white'>
              OpenAI
            </span>
            <span className='text-sm text-semi-color-text-2'>{t('homepage_unified_format')}</span>
          </div>
          <div className='grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4'>
            {unified.map((g, i) => (
              <div
                key={i}
                className='rounded-xl p-5 transition-all duration-700'
                style={{
                  background: 'var(--semi-color-fill-0)',
                  transitionDelay: `${150 + i * 80}ms`,
                  opacity: inView ? 1 : 0,
                  transform: inView ? 'translateY(0)' : 'translateY(24px)',
                }}
              >
                <div className='flex items-center gap-2 mb-3'>
                  <span className='text-semi-color-text-2'>{g.icon}</span>
                  <span className='font-semibold text-sm text-semi-color-text-0'>{g.label}</span>
                </div>
                <div className='space-y-1.5'>
                  {g.endpoints.map((ep) => (
                    <div
                      key={ep}
                      className='text-xs font-mono px-2.5 py-1.5 rounded-lg truncate'
                      style={{ background: 'var(--semi-color-fill-1)', color: 'var(--semi-color-text-2)' }}
                    >
                      {ep}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* 原生格式 */}
        <div
          className={`transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
          style={{ transitionDelay: '500ms' }}
        >
          <div className='flex items-center gap-2 mb-4'>
            <span className='text-xs font-medium px-2.5 py-1 rounded-full bg-gradient-to-r from-emerald-500 to-teal-600 text-white'>
              Native
            </span>
            <span className='text-sm text-semi-color-text-2'>{t('homepage_native_format')}</span>
          </div>
          <div className='flex flex-wrap gap-3'>
            {nativeFormats.map((n, i) => (
              <div
                key={i}
                className='flex items-center gap-3 rounded-xl px-5 py-3 transition-all duration-700'
                style={{
                  background: 'var(--semi-color-fill-0)',
                  transitionDelay: `${550 + i * 60}ms`,
                  opacity: inView ? 1 : 0,
                  transform: inView ? 'translateY(0)' : 'translateY(16px)',
                }}
              >
                <span className='text-sm font-semibold text-semi-color-text-0'>{n.label}</span>
                <span className='text-xs font-mono px-2 py-1 rounded-md' style={{ background: 'var(--semi-color-fill-1)', color: 'var(--semi-color-text-2)' }}>
                  {n.path}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
};

const CodeSection = ({ t, serverAddress }) => {
  const [ref, inView] = useInView();
  const codeExample = `curl ${serverAddress}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "gpt-5.4",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'`;

  const handleCopyCode = async () => {
    const ok = await copy(codeExample);
    if (ok) showSuccess(t('已复制到剪切板'));
  };

  return (
    <section ref={ref} className='py-20 md:py-32 px-4'>
      <div className='max-w-4xl mx-auto'>
        <div className={`text-center mb-12 transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
          <h2 className='text-3xl md:text-4xl font-bold text-semi-color-text-0 mb-4'>
            {t('几行代码即可接入')}
          </h2>
          <p className='text-semi-color-text-2 text-lg'>
            {t('homepage_code_subtitle')}
          </p>
        </div>
        <div
          className={`relative rounded-2xl overflow-hidden transition-all duration-700 delay-200 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
          style={{ background: 'var(--semi-color-fill-0)' }}
        >
          <div className='flex items-center justify-between px-5 py-3' style={{ borderBottom: '1px solid var(--semi-color-border)' }}>
            <div className='flex items-center gap-2'>
              <Code2 size={16} className='text-semi-color-text-2' />
              <span className='text-sm font-medium text-semi-color-text-2'>cURL</span>
            </div>
            <button
              onClick={handleCopyCode}
              className='p-1.5 rounded-lg transition-colors duration-150 hover:opacity-80'
              style={{ background: 'var(--semi-color-fill-1)', color: 'var(--semi-color-text-2)' }}
            >
              <IconCopy size='small' />
            </button>
          </div>
          <pre className='p-5 md:p-6 overflow-x-auto text-sm leading-relaxed'>
            <code className='text-semi-color-text-1 font-mono whitespace-pre'>
              {codeExample}
            </code>
          </pre>
        </div>
      </div>
    </section>
  );
};

const CTASection = ({ t, docsLink }) => {
  const [ref, inView] = useInView();

  return (
    <section ref={ref} className='py-20 md:py-32 px-4'>
      <div
        className={`max-w-4xl mx-auto text-center rounded-3xl py-16 md:py-20 px-6 relative overflow-hidden transition-all duration-700 ${inView ? 'opacity-100 scale-100' : 'opacity-0 scale-95'}`}
        style={{ background: 'var(--semi-color-fill-0)' }}
      >
        <h2 className='text-3xl md:text-4xl font-bold text-semi-color-text-0 mb-4'>
          {t('准备好开始了吗？')}
        </h2>
        <p className='text-semi-color-text-2 text-lg mb-10 max-w-xl mx-auto'>
          {t('homepage_cta_desc')}
        </p>
        <div className='flex flex-col sm:flex-row gap-4 justify-center'>
          <Link to='/console'>
            <Button
              theme='solid'
              type='primary'
              size='large'
              className='px-8'
              icon={<ArrowRight size={18} />}
              iconPosition='right'
            >
              {t('免费开始使用')}
            </Button>
          </Link>
          {docsLink && (
            <Button
              size='large'
              className='px-8'
              icon={<FileText size={18} />}
              onClick={() => window.open(docsLink, '_blank')}
            >
              {t('查看文档')}
            </Button>
          )}
        </div>
      </div>
    </section>
  );
};

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

          {/* 核心优势 */}
          <FeaturesSection t={t} />

          {/* API 端点覆盖 */}
          <EndpointsSection t={t} />

          {/* 快速开始代码示例 */}
          <CodeSection t={t} serverAddress={serverAddress} />

          {/* 底部 CTA */}
          <CTASection t={t} docsLink={docsLink} />
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
