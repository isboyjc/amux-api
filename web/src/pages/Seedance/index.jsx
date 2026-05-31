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

import React, { useContext, useEffect, useRef, useState } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { copy, showSuccess } from '../../helpers';
import { StatusContext } from '../../context/Status';
import {
  Play,
  ArrowUpRight,
  Volume2,
  VolumeX,
  Film,
  Check,
  Copy as CopyIcon,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
} from 'lucide-react';
import { SEEDANCE_I18N, SEEDANCE_SEO_TITLE, SEEDANCE_SEO_DESC } from './translations';

/* ============================ 媒体源（远程 CDN） ============================ */
const CDN = 'https://cdn.amux.ai/playground/video/video/demo';
const HERO_VIDEOS = [
  `${CDN}/01.mp4`,
  `${CDN}/02.mp4`,
  `${CDN}/03.mp4`,
  `${CDN}/04.mp4`,
  `${CDN}/05.mp4`,
  `${CDN}/06.mp4`,
];
const CTA_IMAGE = 'https://cdn.amux.ai/playground/video/image/1/2026/seedance.png';

// 文档地址（固定指向 Seedance 2.0 文档页）
const DOCS_LINK = 'https://www.amux.ai/zh/docs/amux-api/video/doubao-seedance-2';
// 体验/生成的操练场深链：预填模型、分组、时长与示例提示词
const PLAYGROUND_PROMPT =
  '夜晚东京街头，一位年轻女性在霓虹灯下行走，细雨飘落，地面有倒影，慢动作，电影感镜头，浅景深，光影对比强烈';
const PLAYGROUND_LINK = `/console/playground?model=doubao-seedance-2.0&group=premium&duration=5&prompt=${encodeURIComponent(
  PLAYGROUND_PROMPT,
)}`;

// 统一内容容器：居中 max-w-6xl + 左右内边距；内容一律左对齐。
// 滑块左缘与该容器左缘对齐 → GUTTER = 容器左边缘到视口的距离。
const CONTENT = 'max-w-[88rem] mx-auto px-6';
const GUTTER = 'max(1.5rem, calc((100vw - 88rem) / 2 + 1.5rem))';

/* ============================ 工具 ============================ */
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

// 视口内自动播放（静音），悬浮则开启声音、移出静音
const HoverSoundVideo = ({ src, className = '', style }) => {
  const ref = useRef(null);
  useEffect(() => {
    const v = ref.current;
    if (!v) return;
    const io = new IntersectionObserver(
      ([e]) => {
        if (e.isIntersecting) {
          const p = v.play();
          if (p) p.catch(() => {});
        } else {
          v.pause();
        }
      },
      { threshold: 0.25 },
    );
    io.observe(v);
    return () => io.disconnect();
  }, []);
  return (
    <video
      ref={ref}
      className={className}
      style={style}
      src={src}
      muted
      loop
      playsInline
      preload='metadata'
      onMouseEnter={() => {
        if (ref.current) ref.current.muted = false;
      }}
      onMouseLeave={() => {
        if (ref.current) ref.current.muted = true;
      }}
    />
  );
};

const Eyebrow = ({ children }) => (
  <span
    className='inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.2em] mb-4'
    style={{ color: 'var(--semi-color-primary)' }}
  >
    <span
      className='inline-block w-5 h-px'
      style={{ background: 'var(--semi-color-primary)', opacity: 0.5 }}
    />
    {children}
  </span>
);

// 把 i18n.language 归一到本页字典支持的语言代码
const pickLang = (l = '') => {
  const lc = l.toLowerCase();
  if (lc === 'zh-tw' || lc.startsWith('zh-hant') || lc.startsWith('zh-tw')) return 'zh-TW';
  if (lc.startsWith('zh')) return 'zh-CN';
  const base = lc.split('-')[0];
  return ['en', 'ja', 'ru', 'fr', 'vi'].includes(base) ? base : 'zh-CN';
};

// SEO：进入本页时设置 title / description / Open Graph / Twitter 卡片，
// 离开时还原（仅本页生效，不污染其它页面）。
const useSeo = ({ title, description, image, url }) => {
  useEffect(() => {
    const prevTitle = document.title;
    document.title = title;
    const created = [];
    const changed = [];
    const upsert = (selector, attrs, content) => {
      let el = document.head.querySelector(selector);
      if (el) {
        changed.push([el, el.getAttribute('content')]);
        el.setAttribute('content', content);
      } else {
        el = document.createElement('meta');
        Object.entries(attrs).forEach(([k, v]) => el.setAttribute(k, v));
        el.setAttribute('content', content);
        el.setAttribute('data-seedance-seo', '');
        document.head.appendChild(el);
        created.push(el);
      }
    };
    upsert('meta[name="description"]', { name: 'description' }, description);
    upsert('meta[property="og:title"]', { property: 'og:title' }, title);
    upsert('meta[property="og:description"]', { property: 'og:description' }, description);
    upsert('meta[property="og:image"]', { property: 'og:image' }, image);
    upsert('meta[property="og:type"]', { property: 'og:type' }, 'website');
    upsert('meta[property="og:url"]', { property: 'og:url' }, url);
    upsert('meta[name="twitter:card"]', { name: 'twitter:card' }, 'summary_large_image');
    upsert('meta[name="twitter:title"]', { name: 'twitter:title' }, title);
    upsert('meta[name="twitter:description"]', { name: 'twitter:description' }, description);
    upsert('meta[name="twitter:image"]', { name: 'twitter:image' }, image);
    return () => {
      document.title = prevTitle;
      created.forEach((el) => el.remove());
      changed.forEach(([el, val]) => {
        if (val === null) el.removeAttribute('content');
        else el.setAttribute('content', val);
      });
    };
  }, [title, description, image, url]);
};

/* ============================ Hero ============================ */
// 背景视频用 position:fixed 钉在视口（合成层处理，零延迟、滚动/回弹都不动），
// 下方内容用不透明背景在其上滚动遮盖；hero 离开视口时淡出并暂停。
const Hero = ({ t }) => {
  const [active, setActive] = useState(0);
  const [muted, setMuted] = useState(true);
  const [inView, setInView] = useState(true);
  const vids = useRef([]);
  const spacerRef = useRef(null);

  useEffect(() => {
    vids.current.forEach((v, i) => {
      if (!v) return;
      if (i === active && inView) {
        v.muted = muted;
        const p = v.play();
        if (p) p.catch(() => {});
      } else {
        v.pause();
      }
    });
  }, [active, muted, inView]);

  useEffect(() => {
    const el = spacerRef.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([e]) => {
        setInView(e.isIntersecting);
        if (!e.isIntersecting) setMuted(true);
      },
      { threshold: 0.04 },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  const next = () => setActive((a) => (a + 1) % HERO_VIDEOS.length);

  const pillStyle = {
    color: '#fff',
    background: 'rgba(255,255,255,0.1)',
    border: '1px solid rgba(255,255,255,0.22)',
    backdropFilter: 'blur(10px)',
  };

  return (
    <>
      <div
        className='fixed inset-0 bg-black pointer-events-none'
        style={{ zIndex: 0, opacity: inView ? 1 : 0, transition: 'opacity 0.4s ease' }}
      >
        {HERO_VIDEOS.map((src, i) => (
          <video
            key={src}
            ref={(el) => (vids.current[i] = el)}
            className='absolute inset-0 w-full h-full object-cover transition-opacity duration-1000 ease-out'
            style={{ opacity: i === active ? 1 : 0 }}
            muted
            playsInline
            preload={i === 0 ? 'auto' : 'none'}
            onEnded={i === active ? next : undefined}
          >
            <source src={src} type='video/mp4' />
          </video>
        ))}
        <div
          className='absolute inset-0'
          style={{
            background:
              'linear-gradient(180deg, rgba(0,0,0,0.45) 0%, transparent 18%, transparent 52%, rgba(0,0,0,0.82) 100%)',
          }}
        />
      </div>

      <section
        ref={spacerRef}
        className='relative w-full'
        style={{ height: '100dvh', minHeight: '640px', zIndex: 1 }}
        onMouseEnter={() => setMuted(false)}
        onMouseLeave={() => setMuted(true)}
      >
        <div className='absolute bottom-0 left-0 right-0 pb-10 md:pb-14' style={{ paddingLeft: GUTTER, paddingRight: GUTTER }}>
          <div className='flex flex-col lg:flex-row lg:items-end lg:justify-between gap-8'>
            <div>
              <h1
                className='text-white font-bold tracking-tight leading-none whitespace-normal md:whitespace-nowrap'
                style={{ fontSize: 'clamp(2.5rem, 6vw, 5rem)' }}
              >
                Seedance 2.0 for Amux
              </h1>
              <p
                className='mt-4 text-sm md:text-base leading-relaxed max-w-xl'
                style={{ color: 'rgba(255,255,255,0.8)' }}
              >
                {t('Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。')}
              </p>
            </div>

            <div className='flex flex-wrap items-center gap-3 shrink-0'>
              <Link to={PLAYGROUND_LINK}>
                <button
                  className='px-5 py-2.5 rounded-full text-sm font-medium transition-transform hover:scale-105'
                  style={pillStyle}
                >
                  {t('立即体验')}
                </button>
              </Link>
              <button
                className='px-5 py-2.5 rounded-full text-sm font-medium transition-transform hover:scale-105'
                style={pillStyle}
                onClick={() => window.open(DOCS_LINK, '_blank')}
              >
                {t('获取 API 文档')}
              </button>
              <button
                aria-label='toggle sound'
                className='w-11 h-11 rounded-full flex items-center justify-center transition-transform hover:scale-105'
                style={pillStyle}
                onClick={() => setMuted((m) => !m)}
              >
                {muted ? <VolumeX size={18} /> : <Volume2 size={18} />}
              </button>
            </div>
          </div>
        </div>
      </section>
    </>
  );
};

/* ===================== 概述（左对齐大陈述） ===================== */
const OverviewSection = ({ t }) => {
  const [ref, inView] = useInView();
  return (
    <section className='py-24 md:py-32'>
      <div
        ref={ref}
        className={`${CONTENT} transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
      >
        <Eyebrow>{t('在 Amux API 上体验')}</Eyebrow>
        <h2 className='text-3xl md:text-5xl font-bold text-semi-color-text-0 leading-snug max-w-4xl'>
          {t('精准控制，生成连贯的电影级 AI 视频')}
        </h2>
        <p className='text-semi-color-text-2 text-lg md:text-xl mt-6 leading-relaxed max-w-3xl'>
          {t('通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。')}
        </p>
      </div>
    </section>
  );
};

/* ===================== 常见问题 FAQ（含模式区别 / 渠道计费） ===================== */
const FAQSection = ({ t }) => {
  const [ref, inView] = useInView();
  const [open, setOpen] = useState(0);
  const faqs = [
    {
      q: t('Seedance 2.0 是什么？'),
      a: t('Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。'),
    },
    {
      q: t('标准模式和极速模式有什么区别？'),
      a: t('标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。'),
    },
    {
      q: t('支持哪些输入与生成方式？'),
      a: t('支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。'),
    },
    {
      q: t('可以用它创作什么？'),
      a: t('短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。'),
    },
    {
      q: t('如何接入调用？'),
      a: t('提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。'),
    },
    {
      q: t('价格与渠道是怎样的？'),
      a: t('premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。'),
    },
    {
      q: t('使用 premium 渠道有什么要求？'),
      a: t('premium 渠道模型需累计充值满 $20 后解锁使用。'),
    },
    {
      q: t('需要视频剪辑经验吗？'),
      a: t('不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。'),
    },
  ];

  return (
    <section className='py-16 md:py-24'>
      <div ref={ref} className={CONTENT}>
        <div className={`mb-10 md:mb-12 transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
          <Eyebrow>{t('常见问题')}</Eyebrow>
          <h2 className='text-3xl md:text-5xl font-bold text-semi-color-text-0 mb-3'>
            {t('还有什么疑问吗？')}
          </h2>
          <p className='text-semi-color-text-2 text-base md:text-lg'>
            {t('我们整理了最常被问到的问题。')}
          </p>
        </div>

        <div className='border-t' style={{ borderColor: 'var(--semi-color-border)' }}>
          {faqs.map((f, i) => {
            const expanded = open === i;
            return (
              <div key={i} className='border-b' style={{ borderColor: 'var(--semi-color-border)' }}>
                <button
                  onClick={() => setOpen(expanded ? -1 : i)}
                  className='w-full flex items-center justify-between gap-4 py-5 text-left'
                >
                  <span className='text-base md:text-lg font-semibold text-semi-color-text-0'>
                    {f.q}
                  </span>
                  <ChevronDown
                    size={18}
                    className={`shrink-0 transition-transform duration-300 ${expanded ? 'rotate-180' : ''}`}
                    style={{ color: 'var(--semi-color-text-2)' }}
                  />
                </button>
                <div
                  className='grid transition-all duration-300 ease-out'
                  style={{ gridTemplateRows: expanded ? '1fr' : '0fr' }}
                >
                  <div className='overflow-hidden'>
                    <p className='text-sm md:text-base text-semi-color-text-2 leading-relaxed pb-5 pr-6 md:pr-10'>
                      {f.a}
                    </p>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
};

/* ===================== 效果展示（全宽出血大滑块，左缘对齐内容） ===================== */
const ShowcaseSection = ({ t }) => {
  const [ref, inView] = useInView();
  const scroller = useRef(null);

  const items = [
    {
      title: t('短片与电影叙事。'),
      desc: t('多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。'),
      src: `${CDN}/short-film-mini.mp4`,
    },
    {
      title: t('电影级动态运镜。'),
      desc: t('复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。'),
      src: `${CDN}/03.mp4`,
    },
    {
      title: t('逼真物理与动作连贯。'),
      desc: t('密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。'),
      src: `${CDN}/action-v2-mini.mp4`,
    },
    {
      title: t('活动推广视频。'),
      desc: t('一致品牌形象与强叙事性的宣传视频，无需制作团队。'),
      src: `${CDN}/compaign-mini.mp4`,
    },
    {
      title: t('高影响力视频广告。'),
      desc: t('由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。'),
      src: `${CDN}/high-impact-mini.mp4`,
    },
    {
      title: t('音频节奏引导。'),
      desc: t('以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。'),
      src: `${CDN}/1770627047985_WYEvEd7j.mp4`,
    },
  ];

  const scrollByCards = (dir) => {
    const el = scroller.current;
    if (!el) return;
    const card = el.querySelector('[data-card]');
    const step = card ? card.offsetWidth + 20 : el.clientWidth * 0.8;
    el.scrollBy({ left: dir * step, behavior: 'smooth' });
  };

  return (
    <section className='py-16 md:py-24'>
      {/* 标题：与常规内容容器左缘对齐 */}
      <div
        ref={ref}
        className={`${CONTENT} mb-10 md:mb-12 transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
      >
        <Eyebrow>{t('效果展示')}</Eyebrow>
        <h2 className='text-3xl md:text-5xl font-bold text-semi-color-text-0 leading-tight mb-4 max-w-2xl'>
          {t('为每个场景而生的专业视频')}
        </h2>
        <p className='text-semi-color-text-2 text-base md:text-lg max-w-2xl'>
          {t('无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。')}
        </p>
      </div>

      {/* 全宽出血滑轨：首张左缘与标题对齐，向右溢出屏幕边缘 */}
      <div
        ref={scroller}
        className='hide-scrollbar flex gap-5 overflow-x-auto snap-x snap-mandatory'
        style={{ paddingLeft: GUTTER, paddingRight: GUTTER, scrollPaddingLeft: GUTTER }}
      >
        {items.map((it, i) => (
          <div
            key={i}
            data-card
            className='shrink-0 snap-start'
            style={{ width: 'clamp(320px, 52vw, 980px)' }}
          >
            <div
              className='group relative overflow-hidden rounded-2xl bg-black'
              style={{ aspectRatio: '16 / 9' }}
            >
              <HoverSoundVideo
                src={it.src}
                className='absolute inset-0 w-full h-full object-cover transition-transform duration-700 group-hover:scale-[1.04]'
              />
              <div
                className='absolute bottom-3 right-3 flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full pointer-events-none opacity-0 group-hover:opacity-100 transition-opacity duration-300'
                style={{ background: 'rgba(0,0,0,0.5)', color: '#fff', backdropFilter: 'blur(6px)' }}
              >
                <Volume2 size={13} />
                {t('悬浮播放声音')}
              </div>
            </div>
            <p className='mt-4 text-sm md:text-[15px] leading-relaxed'>
              <span className='font-semibold text-semi-color-text-0'>{it.title}</span>
              <span className='text-semi-color-text-2'> {it.desc}</span>
            </p>
          </div>
        ))}
      </div>

      {/* 箭头：与内容左缘对齐放在滑轨下方 */}
      <div className={`${CONTENT} flex gap-3 mt-10`}>
        <button
          onClick={() => scrollByCards(-1)}
          aria-label='prev'
          className='lp-glass lp-glass-hover w-11 h-11 rounded-full flex items-center justify-center text-semi-color-text-1'
        >
          <ChevronLeft size={18} />
        </button>
        <button
          onClick={() => scrollByCards(1)}
          aria-label='next'
          className='lp-glass lp-glass-hover w-11 h-11 rounded-full flex items-center justify-center text-semi-color-text-1'
        >
          <ChevronRight size={18} />
        </button>
      </div>
    </section>
  );
};

/* ===================== 快速接入（左对齐，OpenAI / 火山 V3 可切换） ===================== */
const QuickStartSection = ({ t, serverAddress }) => {
  const [ref, inView] = useInView();
  const [tab, setTab] = useState('openai');
  const [copied, setCopied] = useState(false);

  const openaiSubmit = `curl ${serverAddress}/v1/video/generations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "doubao-seedance-2.0",
    "prompt": "一只柴犬在樱花树下奔跑，电影感运镜，阳光透过花瓣",
    "resolution": "1080p",
    "duration": 5
  }'`;
  const openaiPoll = `curl ${serverAddress}/v1/video/generations/{task_id} \\
  -H "Authorization: Bearer YOUR_API_KEY"`;

  const arkSubmit = `curl ${serverAddress}/api/v3/contents/generations/tasks \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "doubao-seedance-2.0",
    "content": [
      {"type": "text", "text": "一只柴犬在樱花树下奔跑，电影感运镜 --resolution 1080p --duration 5"}
    ]
  }'`;
  const arkPoll = `curl ${serverAddress}/api/v3/contents/generations/tasks/{task_id} \\
  -H "Authorization: Bearer YOUR_API_KEY"`;

  const cur =
    tab === 'openai'
      ? { submit: openaiSubmit, poll: openaiPoll, label: 'OpenAI' }
      : { submit: arkSubmit, poll: arkPoll, label: 'Volcengine V3' };

  const handleCopy = async () => {
    const ok = await copy(`${cur.submit}\n\n${cur.poll}`);
    if (ok) {
      setCopied(true);
      showSuccess(t('已复制到剪切板'));
      setTimeout(() => setCopied(false), 1800);
    }
  };

  const tabs = [
    { k: 'openai', label: 'OpenAI' },
    { k: 'ark', label: t('火山 V3') },
  ];

  return (
    <section ref={ref} className='py-20 md:py-28'>
      <div className={CONTENT}>
        <div className='grid lg:grid-cols-2 gap-10 lg:gap-16 items-start'>
          {/* 左：说明 + 风格切换 */}
          <div className={`transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
            <Eyebrow>{t('快速接入')}</Eyebrow>
            <h2 className='text-3xl md:text-4xl font-bold text-semi-color-text-0 mb-4'>
              {t('两步生成你的第一条视频')}
            </h2>
            <p className='text-semi-color-text-2 text-lg mb-8'>
              {t('同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。')}
            </p>

            <div className='flex justify-start mb-5'>
              <div className='inline-flex p-1 rounded-full lp-glass'>
                {tabs.map((o) => (
                  <button
                    key={o.k}
                    onClick={() => setTab(o.k)}
                    className='px-5 py-2 rounded-full text-sm font-medium transition-all duration-200'
                    style={
                      tab === o.k
                        ? { background: 'var(--semi-color-primary)', color: 'var(--semi-color-bg-0)' }
                        : { color: 'var(--semi-color-text-2)' }
                    }
                  >
                    {o.label}
                  </button>
                ))}
              </div>
            </div>

            {tab === 'ark' && (
              <p className='text-sm text-semi-color-text-2 mb-3 max-w-md'>
                {t('支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。')}
              </p>
            )}
            <p className='text-sm text-semi-color-text-2'>
              {t('提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。')}
            </p>
          </div>

          {/* 右：代码块 */}
          <div className={`lp-glass overflow-hidden transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
            <div className='flex items-center justify-between px-5 py-3' style={{ borderBottom: '1px solid var(--semi-color-border)' }}>
              <div className='flex items-center gap-2'>
                <span className='flex items-center gap-1.5 mr-1'>
                  <span className='w-3 h-3 rounded-full' style={{ background: '#ff5f57' }} />
                  <span className='w-3 h-3 rounded-full' style={{ background: '#febc2e' }} />
                  <span className='w-3 h-3 rounded-full' style={{ background: '#28c840' }} />
                </span>
                <Film size={16} className='text-semi-color-text-2' />
                <span className='text-sm font-medium text-semi-color-text-2'>{cur.label} · cURL</span>
              </div>
              <button
                onClick={handleCopy}
                className='w-8 h-8 shrink-0 rounded-lg flex items-center justify-center transition-colors duration-150 hover:opacity-80'
                style={{ background: 'var(--semi-color-fill-1)', color: 'var(--semi-color-text-2)' }}
              >
                {copied ? <Check size={15} /> : <CopyIcon size={15} />}
              </button>
            </div>
            <div className='p-5 md:p-6 space-y-4'>
              <div>
                <div className='text-xs font-semibold text-semi-color-text-2 mb-2'>{t('① 提交生成任务')}</div>
                <pre className='overflow-x-auto text-sm leading-relaxed'>
                  <code className='text-semi-color-text-1 font-mono whitespace-pre'>{cur.submit}</code>
                </pre>
              </div>
              <div style={{ borderTop: '1px dashed var(--semi-color-border)' }} className='pt-4'>
                <div className='text-xs font-semibold text-semi-color-text-2 mb-2'>{t('② 轮询任务结果')}</div>
                <pre className='overflow-x-auto text-sm leading-relaxed'>
                  <code className='text-semi-color-text-1 font-mono whitespace-pre'>{cur.poll}</code>
                </pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

/* ===================== 底部 CTA（左对齐） ===================== */
const CTASection = ({ t }) => {
  const [ref, inView] = useInView();
  return (
    <section className='py-20 md:py-28'>
      <div ref={ref} className={CONTENT}>
        <div
          className={`lp-glass relative overflow-hidden transition-all duration-700 ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
        >
          <div
            className='absolute inset-0 -z-10 pointer-events-none'
            style={{
              background:
                'radial-gradient(ellipse 50% 80% at 0% 0%, var(--semi-color-primary-light-default), transparent 65%)',
              opacity: 0.55,
            }}
          />
          <div className='grid lg:grid-cols-2 items-stretch'>
            {/* 左：文案 + 按钮 */}
            <div className='py-14 md:py-20 px-8 md:px-14 flex flex-col justify-center'>
              <Eyebrow>{t('即刻开始')}</Eyebrow>
              <h2 className='text-3xl md:text-4xl font-bold text-semi-color-text-0 mb-4 max-w-md'>
                {t('用 Seedance 2.0 开始你的创作')}
              </h2>
              <p className='text-semi-color-text-2 text-lg mb-10 max-w-md'>
                {t('Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。')}
              </p>
              <div className='flex flex-col sm:flex-row gap-4 justify-start'>
                <Link to={PLAYGROUND_LINK}>
                  <Button theme='solid' type='primary' size='large' className='px-8' icon={<Play size={18} />}>
                    {t('立即生成视频')}
                  </Button>
                </Link>
                <Link to='/pricing'>
                  <Button size='large' className='px-8' icon={<ArrowUpRight size={18} />} iconPosition='right'>
                    {t('查看模型与价格')}
                  </Button>
                </Link>
              </div>
            </div>
            {/* 右：图片 */}
            <div className='relative min-h-[240px] lg:min-h-[440px] order-first lg:order-last'>
              <img
                src={CTA_IMAGE}
                alt='Seedance 2.0'
                className='absolute inset-0 w-full h-full object-cover'
              />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

const Seedance = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const serverAddress =
    statusState?.status?.server_address || `${window.location.origin}`;

  // 本页多语言：优先取本页字典，缺失再回退全局 t()；zh-CN 直接用中文键。
  const lang = pickLang(i18n.language || 'zh-CN');
  const tt = React.useCallback(
    (key) => {
      if (lang === 'zh-CN') return t(key);
      const dict = SEEDANCE_I18N[lang];
      return (dict && dict[key]) || t(key);
    },
    [lang, t],
  );

  useSeo({
    title: tt(SEEDANCE_SEO_TITLE),
    description: tt(SEEDANCE_SEO_DESC),
    image: CTA_IMAGE,
    url: `${window.location.origin}/seedance2.0`,
  });

  return (
    <div className='w-full overflow-x-hidden'>
      <Hero t={tt} />
      <div
        className='relative'
        style={{ zIndex: 1, background: 'var(--semi-color-bg-0)' }}
      >
        <OverviewSection t={tt} />
        <ShowcaseSection t={tt} />
        <QuickStartSection t={tt} serverAddress={serverAddress} />
        <FAQSection t={tt} />
        <CTASection t={tt} />
      </div>
    </div>
  );
};

export default Seedance;
