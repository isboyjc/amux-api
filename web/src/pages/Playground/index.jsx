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

import React, { useContext, useEffect, useCallback, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Layout, Toast } from '@douyinfe/semi-ui';

// Context
import { UserContext } from '../../context/User';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';

// hooks
import { usePlaygroundState } from '../../hooks/playground/usePlaygroundState';
import { useMessageActions } from '../../hooks/playground/useMessageActions';
import { useApiRequest } from '../../hooks/playground/useApiRequest';
import {
  useImageGeneration,
  getInFlightImageRequest,
  clearInFlightImageRequest,
} from '../../hooks/playground/useImageGeneration';
import {
  getMessages as dbGetMessages,
  putMessages as dbPutMessages,
} from '../../utils/playgroundDb';
import { useVideoGeneration } from '../../hooks/playground/useVideoGeneration';
import { useSyncMessageAndCustomBody } from '../../hooks/playground/useSyncMessageAndCustomBody';
import { useMessageEdit } from '../../hooks/playground/useMessageEdit';
import { useDataLoader } from '../../hooks/playground/useDataLoader';

// Constants and utils
import {
  MESSAGE_ROLES,
  ERROR_MESSAGES,
} from '../../constants/playground.constants';
import {
  getLogo,
  buildMessageContent,
  createMessage,
  createLoadingAssistantMessage,
  getTextContent,
  buildApiPayload,
  encodeToBase64,
  showError,
  uploadToR2,
  parsePlaygroundDeepLink,
  coerceParam,
  TEXT_PARAM_SCHEMA,
  probeDuration,
} from '../../helpers';

// Components
import ChatArea from '../../components/playground/ChatArea';
import FloatingButtons from '../../components/playground/FloatingButtons';
import PlaygroundRightPanel, {
  RIGHT_PANEL_TABS,
} from '../../components/playground/PlaygroundRightPanel';
import SessionList from '../../components/playground/SessionList';
import {
  parseSchema,
  defaultsOf,
  splitSchema,
  filterSchemaByGroup,
  hasImageInputSlot,
} from '../../components/playground/SchemaParamsRenderer';
import { PlaygroundProvider } from '../../contexts/PlaygroundContext';
import { MODALITY } from '../../constants/playground.constants';
import { inferMessageModality } from '../../components/playground/messageModality';

// 生成用户头像（灰色背景 + 圆角矩形，与导航头像一致）
const generateAvatarDataUrl = (username) => {
  if (!username) {
    return 'https://lf3-static.bytednsdoc.com/obj/eden-cn/ptlz_zlp/ljhwZthlaukjlkulzlp/docs-icon.png';
  }
  const firstLetter = username[0].toUpperCase();
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
      <rect width="32" height="32" rx="6" ry="6" fill="#9ca3af" />
      <text x="50%" y="50%" dominant-baseline="central" text-anchor="middle" font-size="16" fill="#ffffff" font-family="Ubuntu, sans-serif">${firstLetter}</text>
    </svg>
  `;
  return `data:image/svg+xml;base64,${encodeToBase64(svg)}`;
};

// 生成默认 AI logo（与系统默认 SVG logo 一致，适配明暗色）
const generateDefaultLogoDataUrl = (isDark) => {
  const fillColor = isDark ? '#ffffff' : '#18181b';
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128" fill="none">
      <path d="M4 96 C4 96, 24 12, 64 12 C104 12, 124 96, 124 96 Q124 102, 118 102 C94 102, 92 64, 64 64 C36 64, 34 102, 10 102 Q4 102, 4 96 Z" fill="${fillColor}"/>
    </svg>
  `;
  return `data:image/svg+xml;base64,${encodeToBase64(svg)}`;
};

// imageInputsValues 槽位 entry 的两种形态：
//   - File：本地选 / 粘贴 / 拖拽得到的字节，需要走 R2 上传
//   - { url, name? }：「以此图继续编辑」点已经在 R2 的图时，直接持有 URL，
//     发送时无需上传
//
// 这套 typeguard 让所有读槽位的逻辑统一通过它判断，避免 instanceof File 散落
// 各处。URL entry 必须是非 File 的对象（File 也是对象但 typeof 'object'，
// instanceof File 为 true，所以判断时把 File 排除掉）
const isFileEntry = (v) => v instanceof File;
const isUrlEntry = (v) =>
  !!v &&
  typeof v === 'object' &&
  !(v instanceof File) &&
  typeof v.url === 'string' &&
  v.url.length > 0;
const isSlotEntry = (v) => isFileEntry(v) || isUrlEntry(v);
const getEntryName = (v) => {
  if (isFileEntry(v)) return v.name || '';
  if (isUrlEntry(v)) return v.name || '';
  return '';
};

// 推断 entry 的媒体类型：'image' | 'video' | 'audio' | null。
// File：看 file.type 前缀；URL entry：优先 entry.mediaType，缺省按后缀名兜底
const getEntryMediaType = (entry) => {
  if (isFileEntry(entry)) {
    const tp = (entry.type || '').toLowerCase();
    if (tp.startsWith('image/')) return 'image';
    if (tp.startsWith('video/')) return 'video';
    if (tp.startsWith('audio/')) return 'audio';
    return null;
  }
  if (isUrlEntry(entry)) {
    if (
      entry.mediaType === 'image' ||
      entry.mediaType === 'video' ||
      entry.mediaType === 'audio'
    ) {
      return entry.mediaType;
    }
    const u = (entry.url || '').toLowerCase().split('?')[0];
    if (/\.(png|jpe?g|webp|gif|bmp|avif|heic|heif)$/.test(u)) return 'image';
    if (/\.(mp4|mov|webm|mkv|avi|m4v)$/.test(u)) return 'video';
    if (/\.(mp3|wav|m4a|aac|flac|ogg|opus)$/.test(u)) return 'audio';
    return null;
  }
  return null;
};

const Playground = () => {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const actualTheme = useActualTheme();
  const isMobile = useIsMobile();
  const styleState = { isMobile };
  const [searchParams, setSearchParams] = useSearchParams();
  // 深链一次性 prompt 注入：useEffect 解析完写到这里，传给 ChatArea →
  // UnifiedInputBar；UnifiedInputBar 一旦把它同步到本地 text，就回调清回 null
  const [pendingDeepLinkText, setPendingDeepLinkText] = React.useState(null);
  // 深链 model 参数：第一个 useEffect 解析后存这里，第二个 useEffect 等
  // inputs.model 切到目标 + schema 就绪再 coerce + 写入对应 state
  // 形如 { targetModel: 'gpt-5', raw: { temperature: '0.7', max_tokens: '2000' } }
  const [pendingDeepLinkParams, setPendingDeepLinkParams] = React.useState(null);

  const state = usePlaygroundState();
  const {
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
    showSettings,
    modelEntries,
    currentModelEntry,
    modalityMap,
    message,
    debugData,
    activeDebugTab,
    previewPayload,
    sseSourceRef,
    chatRef,
    handleInputChange,
    handleModelGroupChange,
    handleParameterToggle,
    debouncedSaveConfig,
    saveMessagesImmediately,
    handleNewChat,
    setShowSettings,
    setModelEntries,
    setGroups,
    setModalityMap,
    setMessage,
    setDebugData,
    setActiveDebugTab,
    setPreviewPayload,
    setShowDebugPanel,
    setCustomRequestMode,
    setCustomRequestBody,
    // 会话
    sessions,
    activeSessionId,
    activeSession,
    sessionsReady,
    switchSession,
    renameSession,
    deleteSession,
    touchSession,
  } = state;

  // 深链快速试用：/console/playground?model=...&group=...&prompt=...
  // 命中后：新建一个会话 + 切到指定 model/group + 预填 prompt（可选），
  // 立刻把 query 抹掉（replace 不留历史，浏览器后退也不会再次触发）。
  // 静默执行，无 Toast；用户期望"点链接进来即可用"，不打断。
  //
  // ref 守门两件事：
  //   1) React Strict Mode 双重渲染时同一份 query 只跑一次
  //   2) 消费完同步触发的 setSearchParams({}) 不会被当作新一次深链
  // 关键点：当 URL 没有深链参数时把 ref 复位 —— 否则用户已经在 playground
  // 页面、再次点击侧边栏/公告里的深链时（同标签 navigate，不走重挂载），
  // ref 永远是 true，effect 直接 return 掉，新链接形同失效
  const deepLinkConsumedRef = useRef(false);
  useEffect(() => {
    if (!sessionsReady) return; // 等会话子系统就绪，避免与"自动建空会话"竞态
    const link = parsePlaygroundDeepLink(searchParams);
    if (!link) {
      // query 已被清空（包括我们自己 replace 抹掉的）→ 复位标记，
      // 让下一次再点深链能重新进入消费分支
      deepLinkConsumedRef.current = false;
      return;
    }
    if (deepLinkConsumedRef.current) return;
    deepLinkConsumedRef.current = true;
    (async () => {
      try {
        // handleNewChat 内部 setInputs 同步 model/group，并落到新会话的元数据
        await handleNewChat({ model: link.model, group: link.group });
        if (link.prompt) {
          setPendingDeepLinkText(link.prompt);
        }
        // model 参数延后到下一个 useEffect 处理：那里会等 inputs.model
        // 切到目标模型 + schema 就绪再 coerce 写入，避免在旧 modality
        // 的 state 上写错地方
        if (link.params && Object.keys(link.params).length > 0) {
          setPendingDeepLinkParams({
            targetModel: link.model,
            raw: link.params,
          });
        }
      } catch (err) {
        console.error('[playground] deep link failed:', err);
      } finally {
        // 清理 query。不在乎是否成功——失败也别让 ?model= 残留导致刷新
        // 反复触发；用户可以从干净 URL 手动操作
        setSearchParams({}, { replace: true });
      }
    })();
  }, [sessionsReady, searchParams, handleNewChat, setSearchParams]);

  // 深链 model 参数应用 effect：定义在 imageParamSchema / setImageParamValues
  // 声明之后，搜 "deep-link-params-apply" 锚点

  // 当前选中模型的 modality，驱动 ChatArea / UnifiedInputBar / 右栏的形态
  const currentModality =
    (modalityMap && modalityMap[inputs.model]?.modality) || 'text';

  // image workspace 的参数 schema：完全由管理员在后台为该模型配置的
  // param_schema 决定；没配就是 null（右栏显示"该模型未声明任何参数"）。
  // 想给某个模型默认列出"尺寸/质量/宽高比"等，去"模型管理"里填一份 schema。
  //
  // schema 里 format:"image" 的字段会被 splitSchema 拆到 inputsSchema，
  // 由 ImageWorkspace 的附件条渲染；其余旋钮留在 paramsSchema 给右栏。
  const imageSchemaSplit = React.useMemo(() => {
    const raw = modalityMap?.[inputs.model]?.param_schema;
    const parsed = parseSchema(raw);
    if (!parsed) {
      const empty = { type: 'object', properties: {} };
      return { paramsSchema: empty, inputsSchema: empty, rawSchema: null };
    }
    // 先按当前分组过滤掉它不支持的字段（schema 里通过
    // `x-disabled-group-prefixes` 声明），再做 image-input 拆分。
    // 这样不仅 UI 上隐藏，imageSchemaSig 变化也会触发 imageParamValues
    // 重置，避免之前在 premium 设的值（比如 n=4）被偷偷发到 special。
    const filtered = filterSchemaByGroup(parsed, inputs.group);
    const split = splitSchema(filtered);
    return { ...split, rawSchema: filtered };
  }, [inputs.model, inputs.group, modalityMap]);
  const imageParamSchema = imageSchemaSplit.paramsSchema;
  const imageInputsSchema = imageSchemaSplit.inputsSchema;

  // 把 inputsSchema 里的媒体槽按 `x-content-role` 分组。返回
  //   { reference, firstFrame, lastFrame, video, audio }，每个 slot 形如
  //   { key, isArray, maxItems, contentRole, mediaType }。
  //
  // 设计点：
  //  - schema-driven：identifier 是 schema 字段的 `x-content-role`，不依赖
  //    具体模型名，将来 Veo / Kling / 其他视频模型只要 schema 里声明同样的
  //    role 字段，UI 就能照样工作
  //  - 向后兼容：旧的 image gen schema（gpt-image / dall-e / gemini-image
  //    等）没有 role 标记，会落到下面的"老 pickKey 兜底"分支，原样把第一个
  //    image 字段当 reference 槽 —— 行为和迁移前一致
  //  - first_frame / last_frame：单值（string + format:image），最多 1 张
  //  - reference_image：通常是 array + items.format=image，maxItems 由 schema
  //    决定（doubao-seedance 是 9）
  //  - reference_video：array + items.format=video，maxItems 默认 3（seedance）
  //  - reference_audio：array + items.format=audio，maxItems 默认 3（seedance）
  const imageInputSlots = React.useMemo(() => {
    const props = imageInputsSchema?.properties || {};
    let firstFrame = null;
    let lastFrame = null;
    let reference = null;
    let video = null;
    let audio = null;

    // 1) 优先用 x-content-role 标记的字段。视频/音频 slot 还会捕获 schema
    //    上的 x-* 限制扩展键（参考 doubao-seedance-2.0 的官方约束）：
    //      - x-max-mb-per-item（array slot）/ x-max-mb（标量 slot）：单文件
    //        大小上限（MB）。沿用 schema 里 reference_images 的命名风格
    //      - x-min-duration-seconds / x-max-duration-seconds：单文件时长 [min,max]
    //      - x-max-total-duration-seconds：所有文件总时长上限
    //    缺省值都是 undefined，缺则不强制——Veo / Kling 等其它视频模型只要
    //    在 schema 里声明对应字段就自动生效，不在 schema 声明则前端不拦
    const numOrUndef = (v) => (typeof v === 'number' && v > 0 ? v : undefined);
    const mbToBytes = (v) =>
      typeof v === 'number' && v > 0 ? Math.floor(v * 1024 * 1024) : undefined;
    Object.entries(props).forEach(([key, def]) => {
      if (!def) return;
      const isArr = def.type === 'array';
      const itemFmt = isArr ? def.items?.format : def.format;
      const mediaType =
        itemFmt === 'image'
          ? 'image'
          : itemFmt === 'video'
            ? 'video'
            : itemFmt === 'audio'
              ? 'audio'
              : null;
      if (!mediaType) return;
      const role = def['x-content-role'];
      // 大小限制 MB 风格优先：array 用 x-max-mb-per-item，标量用 x-max-mb；
      // 都缺时再 fallback 到 x-max-bytes（旧字段，可选保留）
      const maxBytes =
        mbToBytes(isArr ? def['x-max-mb-per-item'] : def['x-max-mb']) ??
        numOrUndef(def['x-max-bytes']);
      const slot = {
        key,
        isArray: isArr,
        maxItems: isArr ? def.maxItems || 9 : 1,
        contentRole: role || null,
        mediaType,
        maxBytes,
        minDurationSeconds: numOrUndef(def['x-min-duration-seconds']),
        maxDurationSeconds: numOrUndef(def['x-max-duration-seconds']),
        maxTotalDurationSeconds: numOrUndef(
          def['x-max-total-duration-seconds'],
        ),
      };
      if (role === 'first_frame' && mediaType === 'image') firstFrame = slot;
      else if (role === 'last_frame' && mediaType === 'image') lastFrame = slot;
      else if (role === 'reference_image' && mediaType === 'image') reference = slot;
      else if (role === 'reference_video' && mediaType === 'video') video = slot;
      else if (role === 'reference_audio' && mediaType === 'audio') audio = slot;
    });

    // 2) 兜底：schema 没声明任何 role → 老 image gen schema，按字段名优先级
    //    + 第一个 image 字段当 reference 槽
    if (!firstFrame && !lastFrame && !reference && !video && !audio) {
      const pickKey = ['image', 'reference_image', 'images'].find(
        (k) => k in props,
      );
      const key =
        pickKey ||
        Object.keys(props).find((k) => {
          const def = props[k];
          return (
            def?.format === 'image' ||
            (def?.type === 'array' && def?.items?.format === 'image')
          );
        });
      if (key) {
        const def = props[key];
        const isArr = def?.type === 'array';
        reference = {
          key,
          isArray: isArr,
          maxItems: isArr ? def?.maxItems || 9 : 1,
          contentRole: 'reference_image',
          mediaType: 'image',
        };
      }
    }

    return { firstFrame, lastFrame, reference, video, audio };
  }, [imageInputsSchema]);

  // 「视频模型同时支持首尾帧 + 全能参考」时，UI 给一个模式切换按钮。
  // 仅 video modality 下需要这个能力——image gen 不存在 mutex 的设计。
  const videoInputModeAvailable =
    currentModality === MODALITY.VIDEO &&
    !!(imageInputSlots.firstFrame || imageInputSlots.lastFrame) &&
    !!imageInputSlots.reference;

  // image workspace 的参数值。schema 变化（切模型/切 workspace）时重置。
  const [imageParamValues, setImageParamValues] = React.useState({});
  // image workspace 的图像输入槽值。
  //
  // 槽位 entry 有两种形态：
  //   1) File           —— 用户从本地拖入 / 粘贴 / 点上传时；后续走 R2 即时上传
  //   2) { url, name? } —— 「以此图继续编辑」点击历史里已经在 R2 的图时；
  //                        slot 直接持有现成的远程 URL，发送时无需再上传
  //
  // 用 isFileEntry / isUrlEntry / isSlotEntry 区分；下面所有读 imageInputsValues
  // 的派生 / 提交逻辑都通过这套 typeguard，不再裸 instanceof File 判断
  const [imageInputsValues, setImageInputsValues] = React.useState({});

  // ========== R2 即时上传（eager upload） ==========
  //
  // 视频模型粘贴/拖拽图片时立刻 fire-and-forget 上传到 R2，发送时不再等
  // 上传，避免点击发送后还要 sequentially 跑多张大图上传产生的卡顿。
  //
  // 数据结构：
  //   - uploadStatus（state）—— File→{status,url?,error?}，驱动 UI（缩略图
  //     上的 spinner / 红框 / 提示）。Map 不可变更新，每次 set 替换整个 Map
  //     触发 React re-render。
  //   - uploadPromisesRef（ref）—— File→Promise<UploadResult>，给发送时的
  //     await 用。用 ref 不进入 React 再渲染循环；Promise 的 then/catch 自
  //     己更新 uploadStatus。
  //
  // 生命周期：
  //   - 用户上传图片 → handleAddReferenceImage → startUpload(file, scope)
  //   - 上传中：uploadStatus[file] = {status:'pending'}
  //   - 完成：uploadStatus[file] = {status:'done', url}
  //   - 失败：uploadStatus[file] = {status:'failed', error}
  //   - 用户移除：清掉两个 Map 中对应条目，避免 File 引用泄漏
  //   - 发送：resolveUploadedUrl(file) await Promise，已完成的直接返回 url
  const [uploadStatus, setUploadStatus] = React.useState(() => new Map());
  const uploadPromisesRef = React.useRef(new Map());

  // 视频/音频时长探测缓存：File → Promise<number|null>。
  // 在 add-time 跑一次探测做单文件 [min,max] 校验；submit-time 再 await
  // 拿到的 number 算总时长。Promise 维度是 File 实例——同一 File 多次
  // 读永远拿到同一个 promise，避免重复构造 <video> 元素
  const durationProbesRef = React.useRef(new Map());

  // 探测时长。已在缓存里直接复用；缺失则启动新一轮 probe 并缓存。
  // 仅服务于视频/音频 File entry——URL entry 没有本地字节，不在这里
  // 处理（提交时 URL 时长按 0 计入，让上游自己兜约束）
  const ensureDurationProbe = React.useCallback((file) => {
    if (!(file instanceof File)) return Promise.resolve(null);
    if (durationProbesRef.current.has(file)) {
      return durationProbesRef.current.get(file);
    }
    const p = probeDuration(file);
    durationProbesRef.current.set(file, p);
    return p;
  }, []);

  // File 被移除时，把对应缓存条目也丢掉，避免 File 引用泄漏
  const forgetDuration = React.useCallback((file) => {
    if (!(file instanceof File)) return;
    durationProbesRef.current.delete(file);
  }, []);

  const startUpload = React.useCallback((file, scope) => {
    if (!(file instanceof File)) return;
    if (uploadPromisesRef.current.has(file)) return; // 已经在传或已完成
    setUploadStatus((prev) => {
      const next = new Map(prev);
      next.set(file, { status: 'pending' });
      return next;
    });
    const promise = uploadToR2(file, scope);
    uploadPromisesRef.current.set(file, promise);
    promise
      .then((r) => {
        setUploadStatus((prev) => {
          const next = new Map(prev);
          next.set(file, { status: 'done', url: r.url });
          return next;
        });
      })
      .catch((err) => {
        setUploadStatus((prev) => {
          const next = new Map(prev);
          next.set(file, {
            status: 'failed',
            error: err?.message || '',
          });
          return next;
        });
      });
  }, []);

  // 给 send 用：等当前 file 上传完成。已完成 → 直接返回 url；in-flight →
  // await；从未启动（理论上不该发生）→ 兜底重启。失败时返回 null，调用方
  // 决定 toast 还是 drop 这一项。
  const resolveUploadedUrl = React.useCallback(
    async (file, scope) => {
      if (!uploadPromisesRef.current.has(file)) {
        startUpload(file, scope);
      }
      try {
        const r = await uploadPromisesRef.current.get(file);
        return r?.url || null;
      } catch {
        return null;
      }
    },
    [startUpload],
  );

  // 用户移除一张图：把该 File 对应的上传跟踪也清掉。这样既释放 File 引用
  // （让浏览器 GC 回收 base64 内容），也保证 hash key 重复使用同一个 File
  // 时不会拿到陈旧的 done/failed 状态。视频/音频时长缓存一起清，避免缓存
  // 表无限增长
  const forgetUpload = React.useCallback(
    (file) => {
      if (!(file instanceof File)) return;
      uploadPromisesRef.current.delete(file);
      setUploadStatus((prev) => {
        if (!prev.has(file)) return prev;
        const next = new Map(prev);
        next.delete(file);
        return next;
      });
      forgetDuration(file);
    },
    [forgetDuration],
  );

  // 重试上传：缩略图 hover 出红色失败提示时点击该按钮，把当前文件重置为
  // pending 重新跑一次。先 forget 再 startUpload，避免命中"已存在 promise"
  // 的早返回逻辑。
  const retryUpload = React.useCallback(
    (file, scope = 'playground-video-image') => {
      if (!(file instanceof File)) return;
      uploadPromisesRef.current.delete(file);
      setUploadStatus((prev) => {
        const next = new Map(prev);
        next.delete(file);
        return next;
      });
      startUpload(file, scope);
    },
    [startUpload],
  );
  // 视频模型的「附件输入模式」：
  //   - 'omni'      → 全能参考（用 reference_image 槽，UI 与图片模型相同）
  //   - 'first_last'→ 首尾帧（用 first_frame / last_frame 单值槽）
  // 默认 omni；切换时清空对侧已填值（兑现 schema 的 mutex 规则）。
  // schema 变化时（切模型）也回到 omni。
  const [videoInputMode, setVideoInputMode] = React.useState('omni');
  const imageSchemaSig = React.useMemo(
    () => JSON.stringify(imageSchemaSplit.rawSchema || {}),
    [imageSchemaSplit.rawSchema],
  );
  React.useEffect(() => {
    setImageParamValues(
      imageSchemaSplit.rawSchema ? defaultsOf(imageParamSchema) : {},
    );
    // 切模型 / 切 schema 时附件一并清空，避免把上一个模型的图带到新模型上
    setImageInputsValues({});
    setVideoInputMode('omni');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [imageSchemaSig]);

  // ===== deep-link-params-apply =====
  // 深链 model 参数应用 effect。设计要点：
  //   1. 等 inputs.model 切到 targetModel 才写——避免在旧模型上下文里把
  //      temperature 误写到 inputs，然后用户切模型时被默认值重置
  //   2. text 模型走 inputs（handleInputChange + 自动启用对应开关）；
  //      image/video 走 imageParamValues（按 paramsSchema coerce）
  //   3. 必须放在上面"切 schema 重置 imageParamValues"effect 之后——
  //      React 按声明顺序执行 effect，先让默认值落定再叠加深链覆盖，
  //      最终 state 是 { ...defaults, ...deepLinkParams }
  //   4. 用 functional updater 防 setImageParamValues 与上面那个 effect
  //      争抢；image/video schema 异步加载时 properties 为空，return 等下次
  //   5. 诊断日志：URL 写错 key / 类型不匹配时 console.warn，方便用户在
  //      DevTools 自查。"applied"汇总用 console.info，可在调试期 grep
  useEffect(() => {
    if (!pendingDeepLinkParams) return;
    if (inputs.model !== pendingDeepLinkParams.targetModel) return;
    const raw = pendingDeepLinkParams.raw;
    const tag = '[playground deep-link]';

    if (currentModality === 'text') {
      // 用户的目标模型如果根本不在 modalityMap 里（拼错名 / 没权限），
      // 这里会 fallback 到 text 分支——给个警告让用户能 DevTools 自查
      if (
        modelEntries.length > 0 &&
        !modalityMap[pendingDeepLinkParams.targetModel]
      ) {
        console.warn(
          `${tag} model "${pendingDeepLinkParams.targetModel}" not in modelEntries; ` +
            `falling back to text-mode param coerce. Check the model name and permission.`,
        );
      }
      const applied = [];
      Object.entries(raw).forEach(([key, value]) => {
        const type = TEXT_PARAM_SCHEMA[key];
        if (!type) {
          console.warn(
            `${tag} dropped "${key}=${value}" — not a known text-mode param. ` +
              `Valid text params: ${Object.keys(TEXT_PARAM_SCHEMA).join(', ')}`,
          );
          return;
        }
        const coerced = coerceParam(value, type);
        if (coerced === undefined) {
          console.warn(
            `${tag} dropped "${key}=${value}" — coerce to ${type} failed.`,
          );
          return;
        }
        handleInputChange(key, coerced);
        if (
          Object.prototype.hasOwnProperty.call(parameterEnabled, key) &&
          !parameterEnabled[key]
        ) {
          handleParameterToggle(key);
        }
        applied.push(`${key}=${coerced}`);
      });
      if (applied.length > 0) {
        console.info(`${tag} applied ${applied.length} text params: ${applied.join(', ')}`);
      }
      setPendingDeepLinkParams(null);
      return;
    }

    // image / video：等 paramsSchema 加载就绪
    const schema = imageParamSchema;
    if (!schema?.properties || Object.keys(schema.properties).length === 0) {
      return; // schema 还在加载，下次 deps 变化再试
    }
    const next = {};
    const applied = [];
    Object.entries(raw).forEach(([key, value]) => {
      const def = schema.properties[key];
      if (!def) {
        console.warn(
          `${tag} dropped "${key}=${value}" — not in schema for "${pendingDeepLinkParams.targetModel}". ` +
            `Valid params: ${Object.keys(schema.properties).join(', ')}`,
        );
        return;
      }
      const coerced = coerceParam(value, def);
      if (coerced === undefined) {
        // enum 不命中 / 类型转失败都走这里——把 schema 期望也打出来好排查
        const expected = Array.isArray(def.enum)
          ? `enum: [${def.enum.join(', ')}]`
          : `type: ${def.type}`;
        console.warn(
          `${tag} dropped "${key}=${value}" — value didn't match ${expected}.`,
        );
        return;
      }
      next[key] = coerced;
      applied.push(`${key}=${JSON.stringify(coerced)}`);
    });
    if (Object.keys(next).length > 0) {
      setImageParamValues((prev) => ({ ...prev, ...next }));
    }
    if (applied.length > 0) {
      console.info(`${tag} applied ${applied.length} schema params: ${applied.join(', ')}`);
    }
    setPendingDeepLinkParams(null);
  }, [
    pendingDeepLinkParams,
    inputs.model,
    currentModality,
    imageParamSchema,
    modelEntries,
    modalityMap,
    handleInputChange,
    handleParameterToggle,
    parameterEnabled,
  ]);

  // 视频模式切换：清空对侧 slot 值，避免触发 mutex
  // - 切到 first_last：清空 reference_image（图）+ reference_video（视频）+
  //   reference_audio（音频）。首尾帧模式仅保留两个图片单值槽。
  // - 切到 omni：清空 first_frame / last_frame。视频/音频附件保留——它们
  //   不和 first/last 互斥，但通常切模式后用户会重新选附件，保留也无副作用
  const handleVideoInputModeChange = useCallback(
    (nextMode) => {
      if (nextMode === videoInputMode) return;
      setImageInputsValues((vals) => {
        const next = { ...vals };
        if (nextMode === 'first_last') {
          if (imageInputSlots.reference) {
            delete next[imageInputSlots.reference.key];
          }
          if (imageInputSlots.video) {
            delete next[imageInputSlots.video.key];
          }
          if (imageInputSlots.audio) {
            delete next[imageInputSlots.audio.key];
          }
        } else {
          if (imageInputSlots.firstFrame) {
            delete next[imageInputSlots.firstFrame.key];
          }
          if (imageInputSlots.lastFrame) {
            delete next[imageInputSlots.lastFrame.key];
          }
        }
        return next;
      });
      setVideoInputMode(nextMode);
    },
    [videoInputMode, imageInputSlots],
  );

  // 右侧面板 Tab 状态：params 或 debug
  const [rightPanelTab, setRightPanelTab] = React.useState(
    RIGHT_PANEL_TABS.PARAMS,
  );

  // API 请求相关
  const { sendRequest, onStopGenerator } = useApiRequest(
    setMessage,
    setDebugData,
    setActiveDebugTab,
    sseSourceRef,
    saveMessagesImmediately,
  );

  // 数据加载（一次性聚合所有可用分组下的模型，提供给合并后的模型选择器）
  useDataLoader(
    userState,
    inputs,
    handleInputChange,
    setModelEntries,
    setGroups,
    setModalityMap,
  );

  // 消息编辑
  const {
    editingMessageId,
    editValue,
    setEditValue,
    handleMessageEdit,
    handleEditSave,
    handleEditCancel,
  } = useMessageEdit(
    setMessage,
    inputs,
    parameterEnabled,
    sendRequest,
    saveMessagesImmediately,
    currentModality,
  );

  // 消息和自定义请求体同步
  const { syncMessageToCustomBody, syncCustomBodyToMessage } =
    useSyncMessageAndCustomBody(
      customRequestMode,
      customRequestBody,
      message,
      inputs,
      setCustomRequestBody,
      setMessage,
      debouncedSaveConfig,
    );

  // 角色信息
  const roleInfo = {
    user: {
      name: userState?.user?.username || 'User',
      avatar: generateAvatarDataUrl(userState?.user?.username),
    },
    assistant: {
      name: 'Assistant',
      avatar: getLogo() || generateDefaultLogoDataUrl(actualTheme === 'dark'),
    },
    system: {
      name: 'System',
      avatar: getLogo() || generateDefaultLogoDataUrl(actualTheme === 'dark'),
    },
  };

  // 图片生成（image workspace 专用）
  const { generate: generateImage, loading: imageGenerating } =
    useImageGeneration({
      onDebug: (patch) =>
        setDebugData((prev) => ({ ...prev, ...patch })),
    });

  // 视频生成（video workspace 专用）—— hook 内部维护"提交 → 轮询 → 状态
  // 上报"的闭环，Playground 只负责把每一轮 onUpdate(patch) 合并回对应
  // assistant 消息并触发保存。
  const {
    generate: generateVideo,
    loading: videoGenerating,
    startPolling: startVideoPolling,
  } = useVideoGeneration({
    onDebug: (patch) =>
      setDebugData((prev) => ({ ...prev, ...patch })),
  });

  // 本轮已自动命名过的 session id 集合。避免同一会话多次触发 rename。
  const autoNamedSessionsRef = useRef(new Set());

  // 首条 prompt 自动给会话命名的统一入口。
  //
  // 判断逻辑（放宽以确保三种 workspace 都能触发）：
  //   1) 当前 session 尚未在本轮 ref 里打标 → 允许尝试
  //   2) prompt 非空 → 允许尝试
  //   3) 不再依赖 activeSession.title 的精确字面量匹配（之前多个默认标题
  //      变体会漏网）。只要 (a) 标题是空/在默认集合内 或 (b) 这条会话当前
  //      没有任何 message（= 名副其实的"第一次发送"），就执行 rename。
  //
  // 用 ref 防重入，renameSession 失败再清除 ref 允许下一次重试。
  const maybeAutoNameSession = useCallback(
    (promptText) => {
      if (!promptText || !activeSessionId) return;
      if (autoNamedSessionsRef.current.has(activeSessionId)) return;
      const trimmed = String(promptText).trim();
      if (!trimmed) return;

      const defaultTitles = new Set([
        '',
        '未命名会话',
        '未命名會話',
        '未命名',
        '新会话',
        '新會話',
        '新对话',
        '新對話',
      ]);
      const currentTitle = String(activeSession?.title || '').trim();
      const titleLooksDefault = defaultTitles.has(currentTitle);
      // 在发消息前 message 还保持 pre-submit 状态；空即"此次是首发"。
      const isFirstMessage = Array.isArray(message) && message.length === 0;

      if (!titleLooksDefault && !isFirstMessage) {
        // 用户已手动命过名、且已有消息，视为定名，不再覆盖
        autoNamedSessionsRef.current.add(activeSessionId);
        return;
      }

      const singleLine = trimmed.replace(/\s+/g, ' ');
      const name =
        singleLine.length > 20 ? singleLine.slice(0, 20) + '…' : singleLine;
      // 先记标记再异步 rename，防止短时间内重复触发
      autoNamedSessionsRef.current.add(activeSessionId);
      try {
        const result = renameSession(activeSessionId, name);
        if (result && typeof result.catch === 'function') {
          result.catch((err) => {
            // rename 失败则撤销标记，下次还能再试
            autoNamedSessionsRef.current.delete(activeSessionId);
            // eslint-disable-next-line no-console
            console.warn('[playground] auto-rename failed', err);
          });
        }
      } catch (err) {
        autoNamedSessionsRef.current.delete(activeSessionId);
        // eslint-disable-next-line no-console
        console.warn('[playground] auto-rename threw', err);
      }
    },
    [activeSessionId, activeSession, message, renameSession],
  );

  const handleGenerateImage = useCallback(
    async ({ prompt, inputs: imageInputs, paramsOverride, modelOverride, groupOverride }) => {
      if (!prompt || !prompt.trim()) return;
      maybeAutoNameSession(prompt);
      // 真实消息活动：把当前会话顶到列表顶部
      if (activeSessionId) touchSession?.(activeSessionId);
      // resend 时复刻原次参数和模型/分组；正常发送沿用 UI state
      const params = paramsOverride ? { ...paramsOverride } : { ...imageParamValues };
      const usedModel = modelOverride || inputs.model;
      const usedGroup = groupOverride || inputs.group;

      // 收集本次发送的「参考图」并读成 base64 data URL，嵌入用户消息的
      // content 数组（OpenAI multimodal 同款形态：[{text}, ...{image_url}]）。
      // 之后 MessageContent / RefImageGrid 直接渲染这些图，气泡里展示
      // 「参考图 + 文字 prompt」的完整一次发送。
      const slotKey = imageInputSlots.reference?.key;
      const slotVal = slotKey ? imageInputs?.[slotKey] : null;
      const refFiles = Array.isArray(slotVal)
        ? slotVal.filter((x) => x instanceof File)
        : slotVal instanceof File
          ? [slotVal]
          : [];
      const refUrls =
        refFiles.length > 0
          ? await Promise.all(
              refFiles.map(
                (f) =>
                  new Promise((resolve, reject) => {
                    const reader = new FileReader();
                    reader.onload = () => resolve(reader.result);
                    reader.onerror = () => reject(reader.error);
                    reader.readAsDataURL(f);
                  }),
              ),
            )
          : [];

      const userContent =
        refUrls.length > 0
          ? [
              { type: 'text', text: prompt },
              ...refUrls.map((url) => ({
                type: 'image_url',
                image_url: { url },
              })),
            ]
          : prompt;

      // 在统一对话窗口里：用户消息和助手消息都打 modality='image' 标，
      // 让 AssistantBubbleRouter 渲染图片气泡，并把这些消息排除出后续
      // chat 模型的上下文。user 消息也存 params，给 resend 用。
      const userMsg = {
        ...createMessage(MESSAGE_ROLES.USER, userContent),
        modality: MODALITY.IMAGE,
        meta: { model: usedModel, group: usedGroup, params },
      };
      const loadingMsg = {
        ...createLoadingAssistantMessage(),
        modality: MODALITY.IMAGE,
        // 把当前参数（size / aspect_ratio / quality 等）带到 loading 消息上，
        // 让 ImageBubble 的骨架占位能按目标尺寸的同比例渲染。
        meta: { model: usedModel, group: usedGroup, params },
      };
      let newMessages = [];
      setMessage((prev) => {
        newMessages = [...prev, userMsg, loadingMsg];
        return newMessages;
      });
      setTimeout(() => saveMessagesImmediately(newMessages), 0);

      // 改 fire-and-forget：generateImage 内部把 promise 注册到模块作用域
      // 的 inFlightImageRequests Map（key=messageId）。结果由下面的"恢复
      // effect"统一接收并落到这条 loading 消息上——这样用户就算切到别的
      // 路由再回来，新的 Playground 实例仍能按 messageId 找到同一条请求并
      // 续上结果，不会再卡在 loading 占位上。
      generateImage({
        messageId: loadingMsg.id,
        model: usedModel,
        group: usedGroup,
        prompt,
        params,
        inputs: imageInputs,
      });
    },
    [
      generateImage,
      inputs.model,
      inputs.group,
      imageParamValues,
      imageInputSlots,
      maybeAutoNameSession,
      setMessage,
      saveMessagesImmediately,
      activeSessionId,
      touchSession,
    ],
  );

  const handleDeleteGeneration = useCallback(
    (generation) => {
      setMessage((prev) => {
        const next = prev.filter((m) => {
          if (generation.promptMessage && m.id === generation.promptMessage.id) return false;
          if (generation.assistantMessage && m.id === generation.assistantMessage.id) return false;
          return true;
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [setMessage, saveMessagesImmediately],
  );

  // ========== 视频生成（异步任务） ==========

  // 把 hook 返回的 patch 合并到对应的 assistant 消息上。
  // status=complete 时同时拼出 content（[{type:'video_url'}, {type:'image_url' last-frame}?]）。
  const applyVideoUpdate = useCallback(
    (assistantId, patch) => {
      setMessage((prev) => {
        const next = prev.map((m) => {
          if (m.id !== assistantId) return m;
          const merged = { ...m };
          if (patch.status) merged.status = patch.status;
          if (typeof patch.progress === 'number') merged.progress = patch.progress;
          if (patch.errorMessage) merged.errorMessage = patch.errorMessage;
          if (patch.status === 'complete') {
            if (patch.videoUrl) {
              const content = [
                {
                  type: 'video_url',
                  video_url: { url: patch.videoUrl },
                },
              ];
              // 末帧 URL 在 raw.metadata.last_frame_url 或 raw.content.last_frame_url
              // 上游可能放在不同字段里，这里两处都兜一下
              const lastFrame =
                patch.raw?.metadata?.last_frame_url ||
                patch.raw?.content?.last_frame_url;
              if (lastFrame) {
                content.push({
                  type: 'image_url',
                  image_url: { url: lastFrame },
                });
              }
              merged.content = content;
            } else {
              // 上游认为任务已完成，但响应里找不到 URL。把它视作 error 让
              // 用户能看到反馈（否则 UI 上什么都不显示，像"卡死"）。
              merged.status = 'error';
              merged.errorMessage = t('任务已完成但响应中未携带视频链接，请查看任务日志');
            }
          }
          return merged;
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [setMessage, saveMessagesImmediately],
  );

  const handleGenerateVideo = useCallback(
    async ({ prompt, attachments, paramsOverride, modelOverride, groupOverride }) => {
      const trimmedPrompt = (prompt || '').trim();
      const content = Array.isArray(attachments) ? attachments : [];
      // prompt 强制必填：上游不接受空 prompt（即便有首/末帧或参考图）。
      // 没 prompt 直接吞掉这次请求，由 onMessageSend 那一层负责给用户 toast
      if (!trimmedPrompt) return;
      maybeAutoNameSession(trimmedPrompt);
      if (activeSessionId) touchSession?.(activeSessionId);
      // resend 路径会把原次 params/model/group 透传过来，覆盖当前 UI state，
      // 保证「重试 = 复刻原次请求」而不是「按当前 UI 再发一遍」。
      const params = paramsOverride ? { ...paramsOverride } : { ...imageParamValues };
      const usedModel = modelOverride || inputs.model;
      const usedGroup = groupOverride || inputs.group;

      // 把图片/视频/音频附件都嵌进 user 消息的 content（与 image gen 对齐），
      // 让用户气泡视觉上能直接看到自己上传了哪些素材，不只是依赖 attachments
      // badge。没有文本时不塞空 text block，避免气泡里出现空段落
      const mediaParts = content.filter(
        (c) =>
          c?.type === 'image_url' ||
          c?.type === 'video_url' ||
          c?.type === 'audio_url',
      );
      const textParts = trimmedPrompt
        ? [{ type: 'text', text: prompt }]
        : [];
      const userContent =
        mediaParts.length > 0 ? [...textParts, ...mediaParts] : prompt;

      // user 消息也把 attachments（带 role）一并持久化，方便渲染成 badge，
      // 同时给 resend 路径留下还原参考图的种子。params 也存在 user 消息上，
      // 这样即使下游 assistant loading 还没创建（极端时序）resend 也能找到。
      const userMsg = {
        ...createMessage(MESSAGE_ROLES.USER, userContent),
        attachments: content,
        modality: MODALITY.VIDEO,
        meta: { model: usedModel, group: usedGroup, params },
      };
      const loadingMsg = {
        ...createLoadingAssistantMessage(),
        status: 'loading',
        progress: 0,
        modality: MODALITY.VIDEO,
        // 同 image：把 resolution / ratio / duration 带过来，骨架按比例渲染
        meta: { model: usedModel, group: usedGroup, params },
      };
      let newMessages = [];
      setMessage((prev) => {
        newMessages = [...prev, userMsg, loadingMsg];
        return newMessages;
      });
      setTimeout(() => saveMessagesImmediately(newMessages), 0);

      const result = await generateVideo({
        model: usedModel,
        group: usedGroup,
        prompt,
        params,
        content,
        onUpdate: (patch) => applyVideoUpdate(loadingMsg.id, patch),
      });

      if (!result || result.error) {
        setMessage((prev) => {
          const next = prev.map((m) => {
            if (m.id !== loadingMsg.id) return m;
            return {
              ...m,
              status: 'error',
              errorMessage: result?.error || t('视频生成任务提交失败'),
            };
          });
          setTimeout(() => saveMessagesImmediately(next), 0);
          return next;
        });
        return;
      }

      // 提交成功：把 taskId + meta 写回 assistant 消息，后续轮询靠
      // onUpdate 更新 status/content。
      setMessage((prev) => {
        const next = prev.map((m) => {
          if (m.id !== loadingMsg.id) return m;
          return {
            ...m,
            taskId: result.taskId,
            status: 'polling',
            modality: MODALITY.VIDEO,
            pollStartedAt: Date.now(),
            meta: {
              model: usedModel,
              group: usedGroup,
              params: params || {},
            },
          };
        });
        setTimeout(() => saveMessagesImmediately(next), 0);
        return next;
      });
    },
    [
      generateVideo,
      inputs.model,
      inputs.group,
      imageParamValues,
      maybeAutoNameSession,
      setMessage,
      saveMessagesImmediately,
      activeSessionId,
      touchSession,
      applyVideoUpdate,
      t,
    ],
  );

  // 扫一遍 message 里还在 polling 的视频任务，把没跑轮询的补上。
  //   - 切会话 → message 由 IDB 异步加载进来，这时触发一次恢复
  //   - 刷新页面 → 同上
  //   - 新提交任务 → message 已经包含 loading/polling 行，顺带触发也无害
  // startVideoPolling 内部按 taskId 去重，重复调用直接 no-op；所以这里
  // 依赖 message 整体即可——虽然轮询过程中 message 会频繁变更导致 effect
  // 多次执行，但都是 idempotent 的空跑。
  React.useEffect(() => {
    if (!Array.isArray(message) || message.length === 0) return;
    message.forEach((m) => {
      if (!m || m.role !== MESSAGE_ROLES.ASSISTANT) return;
      if (!m.taskId) return;
      if (m.status === 'complete' || m.status === 'error') return;
      startVideoPolling(m.taskId, (patch) => applyVideoUpdate(m.id, patch));
    });
  }, [message, startVideoPolling, applyVideoUpdate]);

  // ========== 图片生成结果恢复 ==========
  //
  // 图片生成是同步请求，promise 句柄通过 useImageGeneration 的模块作用域
  // Map 暴露出来。这里扫所有 modality=image && status=loading 的 assistant
  // 消息：
  //   - Map 里有：attach .then() 收尾，并直接写 IDB（不依赖 setMessage 在
  //     unmount 后的更新——React 18 在 unmounted 状态下会跳过 updater，所以
  //     这里同时用 dbPutMessages 兜底，保证回到页面时 IDB 已经是终态）
  //   - Map 里没有（硬刷新 / 关 tab 后重开）：把消息标 error，避免一直转
  //
  // attachedImageMsgIdsRef 记录"本次 mount 已 attach 过的 messageId"，
  // 防止 message 数组变更引起 effect 重跑时重复挂监听（Promise 本身允许多
  // 次 .then，但没必要）。组件 unmount 时 ref gc，下次 mount 重新 attach；
  // 此时 promise 通常已经 resolve，.then 会立即用缓存的值跑。
  const attachedImageMsgIdsRef = useRef(new Set());
  React.useEffect(() => {
    if (!Array.isArray(message) || message.length === 0) return;
    if (!activeSessionId) return;

    message.forEach((msg) => {
      if (!msg || msg.role !== MESSAGE_ROLES.ASSISTANT) return;
      if (msg.modality !== MODALITY.IMAGE) return;
      if (msg.status !== 'loading') return;
      if (attachedImageMsgIdsRef.current.has(msg.id)) return;

      const promise = getInFlightImageRequest(msg.id);
      const sessionAtAttach = activeSessionId;

      // 没找到 in-flight：当作"请求已断开"处理。常见于刷新或关 tab。
      if (!promise) {
        const errorMsg = {
          ...msg,
          status: 'error',
          errorMessage: t('请求已中断，请重新发送'),
        };
        setMessage((prev) => {
          const next = prev.map((m) => (m.id === msg.id ? errorMsg : m));
          setTimeout(() => saveMessagesImmediately(next), 0);
          return next;
        });
        // 当前会话可能已经切走，setMessage 不会触达；直接落 IDB 兜底
        if (sessionAtAttach) {
          dbGetMessages(sessionAtAttach)
            .then((stored) => {
              if (!Array.isArray(stored)) return;
              const next = stored.map((m) => (m.id === msg.id ? errorMsg : m));
              return dbPutMessages(sessionAtAttach, next);
            })
            .catch((e) =>
              console.error('[playground] mark image error in IDB failed', e),
            );
        }
        return;
      }

      attachedImageMsgIdsRef.current.add(msg.id);

      // 把"参数 / 模型 / 分组"从 loading 消息的 meta 里取，避免依赖外层
      // inputs（用户切到别的会话或别的模型时 inputs 已变）
      const metaModel = msg.meta?.model;
      const metaGroup = msg.meta?.group;
      const metaParams = msg.meta?.params || {};

      const buildResolvedMsg = (result) => {
        if (!result || result.error || !result.images?.length) {
          return {
            ...msg,
            status: 'error',
            errorMessage: result?.error || t('图片生成失败'),
          };
        }
        return {
          ...msg,
          status: 'complete',
          modality: MODALITY.IMAGE,
          content: result.images.map((img) => ({
            type: 'image_url',
            image_url: { url: img.url },
            revised_prompt: img.revisedPrompt,
          })),
          meta: {
            model: metaModel,
            group: metaGroup,
            params: metaParams,
            usage: result.usage,
          },
        };
      };

      promise
        .then(async (result) => {
          const resolvedMsg = buildResolvedMsg(result);

          // 1) 更新 React state（如果当前正显示这条会话的消息）
          setMessage((prev) => {
            // 仅当 prev 里有这条消息时才更新，否则说明用户已切到别的会话
            if (!prev.some((m) => m.id === msg.id)) return prev;
            const next = prev.map((m) => (m.id === msg.id ? resolvedMsg : m));
            setTimeout(() => saveMessagesImmediately(next), 0);
            return next;
          });

          // 2) 直接落 IDB 兜底——React 18 在组件 unmount 时会跳过 setState
          // 的 updater，所以即使 setMessage 没生效，这里也保证持久化。
          if (sessionAtAttach) {
            try {
              const stored = await dbGetMessages(sessionAtAttach);
              if (Array.isArray(stored)) {
                const next = stored.map((m) =>
                  m.id === msg.id ? resolvedMsg : m,
                );
                await dbPutMessages(sessionAtAttach, next);
              }
            } catch (e) {
              console.error('[playground] persist image result failed', e);
            }
          }
        })
        .finally(() => {
          clearInFlightImageRequest(msg.id);
          attachedImageMsgIdsRef.current.delete(msg.id);
        });
    });
  }, [message, activeSessionId, setMessage, saveMessagesImmediately, t]);

  // 消息操作
  const messageActions = useMessageActions(
    message,
    setMessage,
    onMessageSend,
    saveMessagesImmediately,
    onMessageResend,
  );

  // 构建预览请求体
  const constructPreviewPayload = useCallback(() => {
    try {
      // 如果是自定义请求体模式且有自定义内容，直接返回解析后的自定义请求体
      if (customRequestMode && customRequestBody && customRequestBody.trim()) {
        try {
          return JSON.parse(customRequestBody);
        } catch (parseError) {
          console.warn('自定义请求体JSON解析失败，回退到默认预览:', parseError);
        }
      }

      // 默认预览逻辑
      let messages = [...message];

      // 如果存在用户消息
      if (
        !(
          messages.length === 0 ||
          messages.every((msg) => msg.role !== MESSAGE_ROLES.USER)
        )
      ) {
        // 处理最后一个用户消息的图片
        for (let i = messages.length - 1; i >= 0; i--) {
          if (messages[i].role === MESSAGE_ROLES.USER) {
            if (inputs.imageEnabled && inputs.imageUrls) {
              const validImageUrls = inputs.imageUrls.filter(
                (url) => url.trim() !== '',
              );
              if (validImageUrls.length > 0) {
                const textContent = getTextContent(messages[i]) || '示例消息';
                const content = buildMessageContent(
                  textContent,
                  validImageUrls,
                  true,
                );
                messages[i] = { ...messages[i], content };
              }
            }
            break;
          }
        }
      }

      return buildApiPayload(
        messages,
        null,
        inputs,
        parameterEnabled,
        currentModality,
      );
    } catch (error) {
      console.error('构造预览请求体失败:', error);
      return null;
    }
  }, [
    inputs,
    parameterEnabled,
    message,
    customRequestMode,
    customRequestBody,
    currentModality,
  ]);

  // chat / multimodal 路径：拼 chat completions 请求
  // 把过往的 image / video 消息从上下文里剔除（它们是 side outputs，
  // 不参与对话语义）；保留的 image_url 视觉附件由 multimodal 模型消费。
  function handleChatSend(content) {
    const userMessage = {
      ...createMessage(MESSAGE_ROLES.USER, content),
      modality:
        currentModality === MODALITY.MULTIMODAL || inputs.imageEnabled
          ? MODALITY.MULTIMODAL
          : MODALITY.TEXT,
      meta: { model: inputs.model, group: inputs.group },
    };
    const loadingMessage = {
      ...createLoadingAssistantMessage(),
      modality:
        currentModality === MODALITY.MULTIMODAL ? MODALITY.MULTIMODAL : MODALITY.TEXT,
      meta: { model: inputs.model, group: inputs.group },
    };

    if (customRequestMode && customRequestBody) {
      try {
        const customPayload = JSON.parse(customRequestBody);
        setMessage((prevMessage) => {
          const newMessages = [...prevMessage, userMessage, loadingMessage];
          sendRequest(customPayload, customPayload.stream !== false);
          setTimeout(() => saveMessagesImmediately(newMessages), 0);
          return newMessages;
        });
        return;
      } catch (error) {
        console.error('自定义请求体JSON解析失败:', error);
        Toast.error(ERROR_MESSAGES.JSON_PARSE_ERROR);
        return;
      }
    }

    const validImageUrls = inputs.imageUrls.filter((url) => url.trim() !== '');
    const messageContent = buildMessageContent(
      content,
      validImageUrls,
      inputs.imageEnabled,
    );
    const userMessageWithImages = {
      ...createMessage(MESSAGE_ROLES.USER, messageContent),
      modality: inputs.imageEnabled ? MODALITY.MULTIMODAL : MODALITY.TEXT,
      meta: { model: inputs.model, group: inputs.group },
    };

    setMessage((prevMessage) => {
      const newMessages = [...prevMessage, userMessageWithImages];
      // 上下文裁剪：只把 text / multimodal 消息送给 chat 模型；图片/视频
      // 气泡是 side outputs，不参与对话上下文。
      const chatHistory = newMessages.filter((m) => {
        const mod = inferMessageModality(m);
        return mod === MODALITY.TEXT || mod === MODALITY.MULTIMODAL;
      });
      const payload = buildApiPayload(
        chatHistory,
        null,
        inputs,
        parameterEnabled,
        currentModality,
      );
      sendRequest(payload, inputs.stream);

      const messagesWithLoading = [...newMessages, loadingMessage];
      setTimeout(() => saveMessagesImmediately(messagesWithLoading), 0);
      return messagesWithLoading;
    });
  }

  // 统一发送入口：UnifiedInputBar 不知道当前选的是哪种模型，全部以
  // (text) 形式回调过来；这里按当前 modality 路由到对应处理。
  // 切到不同 modality 的模型不再 fork 新会话，直接「下一条用新模型」。
  //
  // 写成 function 声明（不是 useCallback）的原因：useMessageActions 在更
  // 前面就要拿到 onMessageSend（用于消息「重新生成」），function 声明会
  // 被提升到 Playground 函数顶部，避免 const TDZ 报错。
  function onMessageSend(content) {
    if (typeof content !== 'string') return;
    const trimmed = content.trim();
    // prompt 强制必填——视频 first_last / omni / image / chat 一视同仁。
    // 上游服务商即便有参考图也会拒收空 prompt，让用户立刻看到 toast 比绕到
    // 上游再失败体验好得多
    if (!trimmed) {
      Toast.warning({
        content: t('请输入 Prompt'),
        duration: 2,
      });
      return;
    }
    maybeAutoNameSession(trimmed);
    if (activeSessionId) touchSession?.(activeSessionId);

    if (currentModality === MODALITY.IMAGE) {
      handleGenerateImage({ prompt: content, inputs: imageInputsValues });
      // 一次性的图像输入槽：发完清空，避免下次意外带上。
      // 注意：imageParamValues（size/quality 等）保留，只在切模型/切 schema
      // 时才重置——用户调好的参数不该因为发送一次就被吞掉。
      setImageInputsValues({});
      return;
    }
    if (currentModality === MODALITY.VIDEO) {
      // 把当前 imageInputsValues 按 schema slot 的 x-content-role 拍平成
      // content 数组：图 → {type:'image_url', image_url:{url}, role}
      //               视频 → {type:'video_url', video_url:{url}, role}
      //               音频 → {type:'audio_url', audio_url:{url}, role}
      // 上游 doubao adapter 会把这串透传给 Volcengine。
      //
      // 上传策略：媒体在 paste/drag 时已通过 startUpload(...) 启动 R2 即时
      // 上传——这里只 await 已经在 in-flight 的 promise 拿到 URL，发送几乎
      // 不再阻塞。极端情况下（如用户敲 ⌘+Enter 在浏览器还没启动 upload 前）
      // resolveUploadedUrl 内部会兜底重启上传并 await，行为退化为旧的同步
      // 上传，仍然能完成请求。
      (async () => {
        const items = [];
        // 遍历顺序：图片（first/last_frame → reference） → 视频 → 音频。
        // 上游 doubao 不依赖顺序，但保持稳定排列方便调试 / 日志阅读。
        const slotsToWalk = [
          { slot: imageInputSlots.firstFrame, scope: 'playground-video-image', contentType: 'image_url', defaultRole: 'first_frame' },
          { slot: imageInputSlots.lastFrame, scope: 'playground-video-image', contentType: 'image_url', defaultRole: 'last_frame' },
          { slot: imageInputSlots.reference, scope: 'playground-video-image', contentType: 'image_url', defaultRole: 'reference_image' },
          { slot: imageInputSlots.video, scope: 'playground-video-video', contentType: 'video_url', defaultRole: 'reference_video' },
          { slot: imageInputSlots.audio, scope: 'playground-video-audio', contentType: 'audio_url', defaultRole: 'reference_audio' },
        ].filter((s) => s.slot);

        for (const { slot, scope, contentType, defaultRole } of slotsToWalk) {
          const v = imageInputsValues?.[slot.key];
          // 同时容纳 File（待上传）和 { url, name? }（已经在 R2 的资源）
          const entries = Array.isArray(v)
            ? v.filter(isSlotEntry)
            : isSlotEntry(v)
              ? [v]
              : [];
          if (entries.length === 0) continue;
          for (const entry of entries) {
            // URL entry：直接用现成 URL，零等待；File entry：await R2 上传完成
            let url;
            if (isUrlEntry(entry)) {
              url = entry.url;
            } else {
              url = await resolveUploadedUrl(entry, scope);
            }
            if (!url) {
              // resolveUploadedUrl 失败时返回 null（已在 startUpload 的
              // catch 把状态写到 uploadStatus，缩略图上会显示红角标）；
              // 这里再补一条 toast，避免用户漏看角标
              const label =
                contentType === 'video_url'
                  ? t('上传视频失败：')
                  : contentType === 'audio_url'
                    ? t('上传音频失败：')
                    : t('上传图片失败：');
              showError(label + getEntryName(entry));
              continue;
            }
            const role = slot.contentRole || defaultRole;
            if (contentType === 'image_url') {
              items.push({ type: 'image_url', image_url: { url }, role });
            } else if (contentType === 'video_url') {
              items.push({ type: 'video_url', video_url: { url }, role });
            } else {
              items.push({ type: 'audio_url', audio_url: { url }, role });
            }
          }
        }

        // 提交前最后一道校验：
        // 1) 音频必须配图或视频（火山硬约束）
        // 2) 视频/音频总时长 ≤ schema 声明的 x-max-total-duration-seconds。
        //    同 slot 的 entry 累加时长——File entry 用 add 时缓存的探测结果，
        //    URL entry 没探测过按 0 计入（让上游兜约束，避免误拦）
        const imgCount = items.filter((x) => x.type === 'image_url').length;
        const vidCount = items.filter((x) => x.type === 'video_url').length;
        const audCount = items.filter((x) => x.type === 'audio_url').length;
        if (audCount > 0 && imgCount === 0 && vidCount === 0) {
          showError(t('音频不能单独输入，请配合图片或视频一起上传'));
          return;
        }

        // 总时长校验（仅当 schema 声明了 x-max-total-duration-seconds 才跑）
        const sumDurationFor = async (slot) => {
          if (!slot) return { total: 0, hasUnknown: false };
          const v = imageInputsValues?.[slot.key];
          const entries = Array.isArray(v)
            ? v.filter(isSlotEntry)
            : isSlotEntry(v)
              ? [v]
              : [];
          let total = 0;
          let hasUnknown = false;
          for (const e of entries) {
            if (isFileEntry(e)) {
              const d = await ensureDurationProbe(e);
              if (d == null) hasUnknown = true;
              else total += d;
            } else {
              hasUnknown = true; // URL entry 拿不到时长，标记一下
            }
          }
          return { total, hasUnknown };
        };

        const vSlot = imageInputSlots.video;
        if (vSlot?.maxTotalDurationSeconds) {
          const { total } = await sumDurationFor(vSlot);
          if (total > vSlot.maxTotalDurationSeconds) {
            showError(
              t('视频总时长不能超过 {{n}} 秒，当前 {{cur}} 秒', {
                n: vSlot.maxTotalDurationSeconds,
                cur: total.toFixed(1),
              }),
            );
            return;
          }
        }
        const aSlot = imageInputSlots.audio;
        if (aSlot?.maxTotalDurationSeconds) {
          const { total } = await sumDurationFor(aSlot);
          if (total > aSlot.maxTotalDurationSeconds) {
            showError(
              t('音频总时长不能超过 {{n}} 秒，当前 {{cur}} 秒', {
                n: aSlot.maxTotalDurationSeconds,
                cur: total.toFixed(1),
              }),
            );
            return;
          }
        }

        handleGenerateVideo({ prompt: content, attachments: items });
        // 发完清空 slot 值（与图片模型对称）；videoInputMode 保持，下条
        // 消息让用户继续在同一模式里。uploadStatus 中对应条目随 File 引用
        // 一并丢掉——下次 paste 同一张图也会触发新一轮 startUpload，因为
        // 浏览器 File API 每次构造的实例都是不同对象。
        setImageInputsValues({});
      })();
      return;
    }
    handleChatSend(content);
    // 文本/多模态对话：发送后清空参考图（与图片模型 setImageInputsValues({})
    // 对称）。inputs 里的模型/分组/temperature 等不动，跨消息保留。
    if (
      inputs.imageEnabled ||
      (inputs.imageUrls && inputs.imageUrls.some((u) => u && u.trim()))
    ) {
      handleInputChange('imageUrls', ['']);
      handleInputChange('imageEnabled', false);
    }
  }

  // dataUrlToFile：把 user 消息里持久化的 base64 data URL 还原成 File 对象，
  // 喂回 handleGenerateImage 的 inputs 槽。data URL 没有原始文件名，按
  // MIME 类型补一个稳定的扩展名即可，上游不依赖文件名语义。
  async function dataUrlToFile(dataUrl, baseName = 'reference') {
    const res = await fetch(dataUrl);
    const blob = await res.blob();
    const ext = (blob.type && blob.type.split('/')[1]) || 'png';
    return new File([blob], `${baseName}.${ext}`, {
      type: blob.type || 'image/png',
    });
  }

  // urlToFile：从 http(s) 远程 URL 下载图片字节并包成 File。给「以此图继续
  // 编辑」用，让用户能把历史里已经在 R2 / 外部 CDN 的图片拿回输入框。
  //
  // CORS 注意事项：浏览器 fetch 跨域读响应体需要对方开 GET CORS。R2 桶上传
  // 直传需要的 PUT CORS 一般会顺带配 GET，对 r2.dev / 自定义域名通常默认
  // 允许；如果失败会抛异常，调用方负责 toast。
  async function urlToFile(url, baseName = 'reference') {
    const res = await fetch(url, { credentials: 'omit' });
    if (!res.ok) {
      throw new Error(`下载图片失败 (HTTP ${res.status})`);
    }
    const blob = await res.blob();
    const ext = ((blob.type && blob.type.split('/')[1]) || 'png').split(
      '+',
    )[0];
    return new File([blob], `${baseName}-${Date.now()}.${ext}`, {
      type: blob.type || 'image/png',
    });
  }

  // 重新发送：失败 / 任意一条历史消息点 ⟳ 重试时调用。和 onMessageSend 不同
  // 的是——这里不读当前 UI state（参考图槽 / 当前选中的模型 / param 面板），
  // 而是从原次 user 消息（attachments / content / meta）里把数据全量还原，
  // 保证「重试 = 复刻原次请求」。
  //
  // 关键差异：
  //   - VIDEO：attachments 直接持久化在 user 消息上，原样回传给
  //     handleGenerateVideo
  //   - IMAGE：参考图嵌在 user.content 的 image_url 里（base64 data URL），
  //     通过 dataUrlToFile 还原为 File，再按当前 schema 的 reference slot key
  //     塞回 handleGenerateImage（不能取原次 slot key——原次模型可能已被切，
  //     但 schema 的 reference 含义是稳定的）
  //   - TEXT/MULTIMODAL：现状 onMessageSend(text) 已够用
  function onMessageResend(userMessage, assistantMessage) {
    if (!userMessage) return;
    const text = getTextContent(userMessage) || '';
    // 模型/分组/参数：优先 assistant.meta（loading 时写入的更完整），其次
    // user.meta；都没有再退回当前 UI state。
    const savedMeta = assistantMessage?.meta || userMessage?.meta || {};
    const savedParams = savedMeta.params;
    const savedModel = savedMeta.model;
    const savedGroup = savedMeta.group;
    const modality = userMessage.modality || assistantMessage?.modality;

    if (modality === MODALITY.VIDEO) {
      const attachments = Array.isArray(userMessage.attachments)
        ? userMessage.attachments
        : [];
      handleGenerateVideo({
        prompt: text,
        attachments,
        paramsOverride: savedParams,
        modelOverride: savedModel,
        groupOverride: savedGroup,
      });
      return;
    }

    if (modality === MODALITY.IMAGE) {
      // user.content 是 string 或 [{type:'text'},{type:'image_url'}...]——
      // 后者是带参考图的形态，前者纯 prompt。把 image_url 项还原成 Files。
      //
      // model/group 这里不强制 override：图片 multipart 的 slot key 必须配
      // 当前 model 的 schema（slot key = upstream form field name），如果用户
      // 已切到别的 model 还硬塞旧 model 名上去会拼不出对的 multipart。仅当
      // 当前 model 与原次保存的 model 一致时才 override；否则随当前 UI。
      (async () => {
        const imgParts = Array.isArray(userMessage.content)
          ? userMessage.content.filter(
              (p) => p?.type === 'image_url' && p?.image_url?.url,
            )
          : [];
        const slotKey = imageInputSlots.reference?.key;
        let imageInputs = {};
        if (imgParts.length > 0 && slotKey) {
          try {
            const files = await Promise.all(
              imgParts.map((p, i) => dataUrlToFile(p.image_url.url, `ref-${i}`)),
            );
            imageInputs = { [slotKey]: files.length === 1 ? files[0] : files };
          } catch (e) {
            console.error('[playground] resend: 还原参考图失败', e);
          }
        }
        const sameModel = savedModel && savedModel === inputs.model;
        handleGenerateImage({
          prompt: text,
          inputs: imageInputs,
          paramsOverride: savedParams,
          modelOverride: sameModel ? savedModel : undefined,
          groupOverride: sameModel ? savedGroup : undefined,
        });
      })();
      return;
    }

    // 文本 / 多模态：走原始 send 路径
    onMessageSend(text);
  }

  // 切换推理展开状态
  const toggleReasoningExpansion = useCallback(
    (messageId) => {
      setMessage((prevMessages) =>
        prevMessages.map((msg) =>
          msg.id === messageId && msg.role === MESSAGE_ROLES.ASSISTANT
            ? { ...msg, isReasoningExpanded: !msg.isReasoningExpanded }
            : msg,
        ),
      );
    },
    [setMessage],
  );

  // 注：消息气泡 / action bar 的渲染逻辑已下沉到 ChatArea 内部，
  // 这里只透传需要的 handlers 与编辑状态。

  // Effects

  // 同步消息和自定义请求体
  useEffect(() => {
    syncMessageToCustomBody();
  }, [message, syncMessageToCustomBody]);

  useEffect(() => {
    syncCustomBodyToMessage();
  }, [customRequestBody, syncCustomBodyToMessage]);

  // 处理URL参数
  useEffect(() => {
    if (searchParams.get('expired')) {
      Toast.warning(t('登录过期，请重新登录！'));
    }
  }, [searchParams, t]);

  // Playground 组件无需再监听窗口变化，isMobile 由 useIsMobile Hook 自动更新

  // 构建预览payload
  useEffect(() => {
    const timer = setTimeout(() => {
      const preview = constructPreviewPayload();
      setPreviewPayload(preview);
      setDebugData((prev) => ({
        ...prev,
        previewRequest: preview ? JSON.stringify(preview, null, 2) : null,
        previewTimestamp: preview ? new Date().toISOString() : null,
      }));
    }, 300);

    return () => clearTimeout(timer);
  }, [
    message,
    inputs,
    parameterEnabled,
    customRequestMode,
    customRequestBody,
    constructPreviewPayload,
    setPreviewPayload,
    setDebugData,
  ]);

  // 自动保存配置
  useEffect(() => {
    debouncedSaveConfig();
  }, [
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
    debouncedSaveConfig,
  ]);

  // 清空对话的处理函数
  const handleClearMessages = useCallback(() => {
    setMessage([]);
    // 清空对话后保存，传入空数组
    setTimeout(() => saveMessagesImmediately([]), 0);
  }, [setMessage, saveMessagesImmediately]);

  // 处理粘贴图片
  const handlePasteImage = useCallback(
    (base64Data) => {
      if (!inputs.imageEnabled) {
        return;
      }
      // 添加图片到 imageUrls 数组
      const newUrls = [...(inputs.imageUrls || []), base64Data];
      handleInputChange('imageUrls', newUrls);
    },
    [inputs.imageEnabled, inputs.imageUrls, handleInputChange],
  );

  // Playground Context 值
  const playgroundContextValue = {
    onPasteImage: handlePasteImage,
    imageUrls: inputs.imageUrls || [],
    imageEnabled: inputs.imageEnabled || false,
  };

  // 参考图相关的几个 derived 标志：
  //   supportsImageAttach    —— 多模态对话模型（chat-multimodal），用 imageUrls
  //   supportsImageInputSlot —— image/video 模型且 schema 有 image 输入槽
  //   acceptsReferenceImage  —— UnifiedInputBar 是否处理 paste/drag/picker
  //                              事件（true 时会调 handleAddReferenceImage，
  //                              文本类模型也设 true 以便父层 toast 反馈）
  //   showUploadButton       —— 输入框右侧是否渲染「+」基座按钮
  //                              （仅 image/video 且有 image input slot）
  const supportsImageAttach = currentModality === MODALITY.MULTIMODAL;
  const supportsImageInputSlot =
    (currentModality === MODALITY.IMAGE ||
      currentModality === MODALITY.VIDEO) &&
    hasImageInputSlot(imageInputsSchema);
  const showUploadButton = supportsImageInputSlot;
  // 文本/audio/embedding 等也允许走 ingest，让 toast 在父层统一提示
  const acceptsReferenceImage = true;

  // 任一异步任务进行中 → 按钮 loading
  const isAnyGenerating =
    imageGenerating ||
    videoGenerating ||
    (Array.isArray(message) &&
      message.some(
        (m) => m.status === 'loading' || m.status === 'incomplete' || m.status === 'polling',
      ));

  // 用 useRef 缓存「File → object URL」，避免每次渲染都新建 URL；
  // 组件卸载时统一 revoke 防止内存泄漏。
  const objectUrlsRef = React.useRef(new Map());
  React.useEffect(() => {
    const cache = objectUrlsRef.current;
    return () => {
      cache.forEach((url) => URL.revokeObjectURL(url));
      cache.clear();
    };
  }, []);
  const fileToUrl = React.useCallback((file) => {
    const cache = objectUrlsRef.current;
    if (cache.has(file)) return cache.get(file);
    const url = URL.createObjectURL(file);
    cache.set(file, url);
    return url;
  }, []);

  // 把 File 的上传状态转成 UI 友好的扁平字段。done/missing 时不显示任何
  // 状态指示；pending 显 spinner；failed 显红色徽章 + 提示。
  const fileUploadFlags = React.useCallback(
    (file) => {
      if (!(file instanceof File)) return { uploading: false, failed: false };
      const s = uploadStatus.get(file);
      return {
        uploading: s?.status === 'pending',
        failed: s?.status === 'failed',
        uploadError: s?.error || '',
      };
    },
    [uploadStatus],
  );

  // 参考图统一展示数据：根据 modality + 当前模式派生 [{ key, dataUrl, name,
  // uploading, failed, uploadError }]。
  // 这条只服务"堆叠 / 全能参考"UI——视频的首尾帧模式由 firstLastFrameImages
  // 单独走一条独立的双上传 UI，不混在这个数组里。
  const referenceImages = React.useMemo(() => {
    if (currentModality === MODALITY.MULTIMODAL) {
      // multimodal 走 base64 imageUrls 通道，没有 R2 上传概念，flags 全 false
      return (inputs.imageUrls || [])
        .map((url, i) => ({
          key: `mm-${i}`,
          dataUrl: url,
          idx: i,
          uploading: false,
          failed: false,
        }))
        .filter((x) => x.dataUrl && x.dataUrl.trim() !== '');
    }
    // image 模型：用 reference slot（向后兼容老 schema 也走这一支）
    // video 模型：仅 omni 模式下用 reference slot；first_last 模式由 firstLastFrameImages 接管
    const useRefSlot =
      (currentModality === MODALITY.IMAGE && imageInputSlots.reference) ||
      (currentModality === MODALITY.VIDEO &&
        videoInputMode === 'omni' &&
        imageInputSlots.reference);
    if (useRefSlot) {
      const slot = imageInputSlots.reference;
      const v = imageInputsValues?.[slot.key];
      const arr = Array.isArray(v) ? v : isSlotEntry(v) ? [v] : [];
      return arr.filter(isSlotEntry).map((entry, i) => {
        // URL entry：缩略图直接用现成远程 URL（顺便走 cdn-cgi 优化）；
        // 没有上传中 / 失败状态。File entry：和原来一样
        if (isUrlEntry(entry)) {
          return {
            key: `url-${i}-${entry.url}`,
            dataUrl: entry.url,
            name: entry.name || '',
            idx: i,
            entry,
            uploading: false,
            failed: false,
          };
        }
        return {
          key: `img-${i}-${entry.name}-${entry.size}`,
          dataUrl: fileToUrl(entry),
          name: entry.name,
          idx: i,
          file: entry,
          entry,
          ...fileUploadFlags(entry),
        };
      });
    }
    return [];
  }, [
    currentModality,
    inputs.imageUrls,
    imageInputsValues,
    imageInputSlots,
    videoInputMode,
    fileToUrl,
    fileUploadFlags,
  ]);

  // 视频「首尾帧」模式专用：两个独立 slot 的当前展示数据，给输入栏的
  // FirstLastFrameUpload 组件消费。任一槽空时该字段为 null，UI 据此渲染
  // 上传按钮 vs 已上传图片。每张图也带上 uploading / failed 标记。
  const firstLastFrameImages = React.useMemo(() => {
    if (currentModality !== MODALITY.VIDEO || videoInputMode !== 'first_last') {
      return null;
    }
    const ff = imageInputSlots.firstFrame;
    const lf = imageInputSlots.lastFrame;
    const ffEntry = ff ? imageInputsValues?.[ff.key] : null;
    const lfEntry = lf ? imageInputsValues?.[lf.key] : null;
    // entry 可能是 File（待上传）或 { url, name? }（已经在 R2 的图，直接复用）
    const toFrame = (entry) => {
      if (!isSlotEntry(entry)) return null;
      if (isUrlEntry(entry)) {
        return {
          dataUrl: entry.url,
          name: entry.name || '',
          entry,
          uploading: false,
          failed: false,
        };
      }
      return {
        dataUrl: fileToUrl(entry),
        name: entry.name,
        file: entry,
        entry,
        ...fileUploadFlags(entry),
      };
    };
    return {
      first: toFrame(ffEntry),
      last: toFrame(lfEntry),
    };
  }, [
    currentModality,
    videoInputMode,
    imageInputSlots,
    imageInputsValues,
    fileToUrl,
    fileUploadFlags,
  ]);

  // 交换首/末帧位置：把两个 slot 的 File 对调。任一为空时也允许 swap，
  // 等价于把已填的那张移到对侧
  const handleSwapFirstLastFrame = useCallback(() => {
    const ff = imageInputSlots.firstFrame;
    const lf = imageInputSlots.lastFrame;
    if (!ff || !lf) return;
    setImageInputsValues((vals) => {
      const next = { ...vals };
      const ffVal = next[ff.key];
      const lfVal = next[lf.key];
      if (ffVal) next[lf.key] = ffVal;
      else delete next[lf.key];
      if (lfVal) next[ff.key] = lfVal;
      else delete next[ff.key];
      return next;
    });
  }, [imageInputSlots]);

  // 显式按 slot 移除（仅首尾帧模式 UI 调用）。同时清掉对应 File 在 R2
  // 上传跟踪表里的条目，避免 File 引用被永远握住。
  const handleRemoveFirstFrame = useCallback(() => {
    const ff = imageInputSlots.firstFrame;
    if (!ff) return;
    setImageInputsValues((vals) => {
      const next = { ...vals };
      const removed = next[ff.key];
      delete next[ff.key];
      // URL entry 没有进过上传通道，无需 forget；只对 File entry 调
      forgetUpload(isFileEntry(removed) ? removed : null);
      return next;
    });
  }, [imageInputSlots, forgetUpload]);
  const handleRemoveLastFrame = useCallback(() => {
    const lf = imageInputSlots.lastFrame;
    if (!lf) return;
    setImageInputsValues((vals) => {
      const next = { ...vals };
      const removed = next[lf.key];
      delete next[lf.key];
      // URL entry 没有进过上传通道，无需 forget；只对 File entry 调
      forgetUpload(isFileEntry(removed) ? removed : null);
      return next;
    });
  }, [imageInputSlots, forgetUpload]);

  // 添加参考素材：根据 modality + 当前 video 模式 + entry 的 MIME 类型路由
  //   - multimodal：走 imageUrls（base64 字符串），统一上限 9 张，仅图
  //   - image：写 reference slot，上限按 schema，仅图
  //   - video-omni：图 → reference_image，视频 → reference_video，
  //                 音频 → reference_audio。三类槽分别按 schema 限额
  //   - video-first_last：依次填 first_frame → last_frame；非图直接 toast 拒
  //   - 其它（text / audio / embedding / rerank）：toast 提示不支持
  //
  // 入参 entry 现在可以是两种形态：
  //   - File：本地选 / 粘贴 / 拖拽得来——视频/图片/音频走对应 R2 scope 上传
  //   - { url, name?, mediaType? }：来自历史复用 / 用户粘贴的现成 URL，
  //     直接持有 URL，不上传、不下载。mediaType 显式标注 'image'|'video'|'audio'，
  //     缺省时按 image 处理（向后兼容旧调用）
  // multimodal 路径只接受图片 File（要 FileReader 转 base64），其它形态
  // 在入口提示后丢弃
  //
  // 第二参 opts.targetRole：可选，仅 video-first_last 模式生效，强制把
  // entry 写入指定 slot（'first_frame' | 'last_frame'）
  const MULTIMODAL_MAX = 9;

  const handleAddReferenceImage = React.useCallback(
    async (entry, opts = {}) => {
      if (!isSlotEntry(entry)) return;
      const targetRole = opts?.targetRole;
      const mediaType = getEntryMediaType(entry) || 'image';

      const hasAnyImageSlot =
        !!imageInputSlots.reference ||
        !!imageInputSlots.firstFrame ||
        !!imageInputSlots.lastFrame;
      const hasVideoSlot = !!imageInputSlots.video;
      const hasAudioSlot = !!imageInputSlots.audio;

      // text / 其它类型：明确告知不支持
      if (
        currentModality !== MODALITY.MULTIMODAL &&
        !(
          (currentModality === MODALITY.IMAGE ||
            currentModality === MODALITY.VIDEO) &&
          (hasAnyImageSlot || hasVideoSlot || hasAudioSlot)
        )
      ) {
        Toast.warning({
          content: t('当前模型不支持图片输入，请切换到多模态、图片或视频模型'),
          duration: 2.5,
        });
        return;
      }

      // multimodal：cap 9 张，走 chat 多模态 imageUrls 通道——仅图片
      if (currentModality === MODALITY.MULTIMODAL) {
        if (mediaType !== 'image') {
          Toast.warning({
            content: t('多模态对话仅支持图片附件'),
            duration: 2,
          });
          return;
        }
        const cur = (inputs.imageUrls || []).filter(
          (u) => u && u.trim() !== '',
        );
        if (cur.length >= MULTIMODAL_MAX) {
          Toast.warning({
            content: t('多模态对话最多上传 {{n}} 张图片', {
              n: MULTIMODAL_MAX,
            }),
            duration: 2,
          });
          return;
        }
        // multimodal 槽是 base64 数组：File 走 FileReader；URL entry 不受
        // 这条路径欢迎——理论上 handleContinueEdit 在 multimodal 走的是
        // urlToFile→File 路径，不会送 URL entry 进来
        if (!isFileEntry(entry)) {
          Toast.warning({
            content: t('当前模型不支持远程图片直接复用'),
            duration: 2,
          });
          return;
        }
        const reader = new FileReader();
        reader.onload = (ev) => {
          const dataUrl = ev.target?.result;
          if (typeof dataUrl !== 'string') return;
          handleInputChange('imageUrls', [...cur, dataUrl]);
          handleInputChange('imageEnabled', true);
        };
        reader.readAsDataURL(entry);
        return;
      }

      // 视频「首尾帧」模式：纯图双 slot。视频/音频在此模式禁用，给明确
      // 引导让用户切到全能参考再上传
      if (
        currentModality === MODALITY.VIDEO &&
        videoInputMode === 'first_last'
      ) {
        if (mediaType !== 'image') {
          Toast.warning({
            content:
              mediaType === 'video'
                ? t('首尾帧模式仅支持图片，可切到"全能参考"上传视频')
                : mediaType === 'audio'
                  ? t('首尾帧模式仅支持图片，可切到"全能参考"上传音频')
                  : t('首尾帧模式仅支持图片'),
            duration: 2.5,
          });
          return;
        }
        const ff = imageInputSlots.firstFrame;
        const lf = imageInputSlots.lastFrame;

        // File 才需要走 R2 上传；URL entry 直接持有现成 URL
        const maybeStartUpload = (e) => {
          if (isFileEntry(e)) startUpload(e, 'playground-video-image');
        };

        // 显式 targetRole：直接覆盖该 slot（点空框上传时使用）
        if (targetRole === 'first_frame' && ff) {
          setImageInputsValues({ ...imageInputsValues, [ff.key]: entry });
          maybeStartUpload(entry);
          return;
        }
        if (targetRole === 'last_frame' && lf) {
          setImageInputsValues({ ...imageInputsValues, [lf.key]: entry });
          maybeStartUpload(entry);
          return;
        }

        // 自动填充：first 空 → 填 first；first 满且 last 空 → 填 last
        const ffFilled = isSlotEntry(imageInputsValues?.[ff?.key]);
        const lfFilled = isSlotEntry(imageInputsValues?.[lf?.key]);
        if (ff && !ffFilled) {
          setImageInputsValues({ ...imageInputsValues, [ff.key]: entry });
          maybeStartUpload(entry);
          return;
        }
        if (lf && !lfFilled) {
          setImageInputsValues({ ...imageInputsValues, [lf.key]: entry });
          maybeStartUpload(entry);
          return;
        }
        Toast.warning({
          content: t('首尾帧已满，请先移除一张再添加'),
          duration: 2,
        });
        return;
      }

      // 视频全能模式：根据 mediaType 路由到 reference / video / audio 槽
      if (currentModality === MODALITY.VIDEO) {
        // image gen modality 不应进到这里——它没有视频/音频 slot
        const targetSlot =
          mediaType === 'video'
            ? imageInputSlots.video
            : mediaType === 'audio'
              ? imageInputSlots.audio
              : imageInputSlots.reference;
        if (!targetSlot) {
          Toast.warning({
            content:
              mediaType === 'video'
                ? t('当前模型不支持视频参考')
                : mediaType === 'audio'
                  ? t('当前模型不支持音频参考')
                  : t('当前模型不支持图片参考'),
            duration: 2.5,
          });
          return;
        }
        const slotMax = targetSlot.isArray ? targetSlot.maxItems : 1;
        const curForCount = imageInputsValues?.[targetSlot.key];
        const curArrForCount = targetSlot.isArray
          ? Array.isArray(curForCount)
            ? curForCount.filter(isSlotEntry)
            : []
          : isSlotEntry(curForCount)
            ? [curForCount]
            : [];
        if (curArrForCount.length >= slotMax) {
          const overMsg =
            mediaType === 'video'
              ? t('视频数量已达上限 {{n}}', { n: slotMax })
              : mediaType === 'audio'
                ? t('音频数量已达上限 {{n}}', { n: slotMax })
                : t('该模型最多上传 {{n}} 张参考图', { n: slotMax });
          Toast.warning({ content: overMsg, duration: 2 });
          return;
        }

        // 视频/音频 File entry：add 前做客户端 [大小 / 时长] 校验。校验失败
        // 直接 toast + return，不入 slot 也不上传；避免"先出现再消失"的闪烁。
        // 图片 entry 不做时长校验（无意义），大小限制由 R2 scope 兜（30MB）
        if ((mediaType === 'video' || mediaType === 'audio') && isFileEntry(entry)) {
          if (targetSlot.maxBytes && entry.size > targetSlot.maxBytes) {
            const mb = Math.round(targetSlot.maxBytes / (1024 * 1024));
            Toast.warning({
              content:
                mediaType === 'video'
                  ? t('单个视频不能超过 {{n}} MB', { n: mb })
                  : t('单个音频不能超过 {{n}} MB', { n: mb }),
              duration: 2.5,
            });
            return;
          }
          const minD = targetSlot.minDurationSeconds;
          const maxD = targetSlot.maxDurationSeconds;
          if (minD || maxD) {
            // 探测有缓存：同一 File 多次 add 共用一份 promise
            const dur = await ensureDurationProbe(entry);
            if (dur != null) {
              const tooShort = minD && dur < minD;
              const tooLong = maxD && dur > maxD;
              if (tooShort || tooLong) {
                Toast.warning({
                  content:
                    mediaType === 'video'
                      ? t('单个视频时长需在 {{min}}-{{max}} 秒之间，当前 {{cur}} 秒', {
                          min: minD || 0,
                          max: maxD || '∞',
                          cur: dur.toFixed(1),
                        })
                      : t('单个音频时长需在 {{min}}-{{max}} 秒之间，当前 {{cur}} 秒', {
                          min: minD || 0,
                          max: maxD || '∞',
                          cur: dur.toFixed(1),
                        }),
                  duration: 3,
                });
                forgetDuration(entry);
                return;
              }
            }
            // dur === null：浏览器读不出（编码不识别 / 探测超时）。这里
            // 不阻断——让它入 slot，提交时上游会兜约束；总时长按 0 计入
            // 也避免错误拦截
          }
        }

        // 走到这里说明数量 / 大小 / 时长都过了。用 functional updater 而非
        // 闭包里的 imageInputsValues：当用户拖多个文件时各 promise 并行完
        // 成，依赖闭包快照会丢失中间项；functional updater 始终读最新 state
        setImageInputsValues((prev) => {
          const cur = prev?.[targetSlot.key];
          const curArr = targetSlot.isArray
            ? Array.isArray(cur)
              ? cur.filter(isSlotEntry)
              : []
            : isSlotEntry(cur)
              ? [cur]
              : [];
          // 二次防越界：promise 串行排队时前面几条可能已把 slot 撑满
          if (curArr.length >= slotMax) return prev;
          if (targetSlot.isArray) {
            return { ...prev, [targetSlot.key]: [...curArr, entry] };
          }
          return { ...prev, [targetSlot.key]: entry };
        });

        // File entry 走 R2 即时上传：scope 按 mediaType 选；URL entry
        // 已经持有远程 URL，无需上传
        if (isFileEntry(entry)) {
          const scope =
            mediaType === 'video'
              ? 'playground-video-video'
              : mediaType === 'audio'
                ? 'playground-video-audio'
                : 'playground-video-image';
          startUpload(entry, scope);
        }
        return;
      }

      // image gen modality：仅图片，写 reference slot
      const refSlot = imageInputSlots.reference;
      if (currentModality === MODALITY.IMAGE && refSlot) {
        if (mediaType !== 'image') {
          Toast.warning({
            content: t('图片生成仅支持图片参考'),
            duration: 2,
          });
          return;
        }
        const slotMax = refSlot.isArray ? refSlot.maxItems : 1;
        const cur = imageInputsValues?.[refSlot.key];
        const curArr = refSlot.isArray
          ? Array.isArray(cur)
            ? cur.filter(isSlotEntry)
            : []
          : isSlotEntry(cur)
            ? [cur]
            : [];
        if (curArr.length >= slotMax) {
          Toast.warning({
            content: t('该模型最多上传 {{n}} 张参考图', { n: slotMax }),
            duration: 2,
          });
          return;
        }
        if (refSlot.isArray) {
          setImageInputsValues({
            ...imageInputsValues,
            [refSlot.key]: [...curArr, entry],
          });
        } else {
          setImageInputsValues({
            ...imageInputsValues,
            [refSlot.key]: entry,
          });
        }
        // image gen 走 multipart edits（useImageGeneration 直接吃 File），
        // 不需要预先 R2 上传
      }
    },
    [
      currentModality,
      videoInputMode,
      inputs.imageUrls,
      imageInputsValues,
      imageInputSlots,
      handleInputChange,
      startUpload,
      ensureDurationProbe,
      forgetDuration,
      t,
    ],
  );

  // 视频参考素材展示数据：仅 video-omni 模式下 + 该模型 schema 声明了
  // reference_video 槽时返回数组，否则为 []。结构和 referenceImages 类似，
  // 但缩略图字段（dataUrl）含义降级为"用于显示的链接"——视频/音频在 chip
  // 区只展示文件名 + 类型图标，不渲染媒体预览，所以 dataUrl 主要给 tooltip。
  // previewUrl 是给输入栏 chip 点击预览用的：File entry 拿 ObjectURL（与
  // 图片缩略图共用 fileToUrl 缓存，组件卸载时统一 revoke）；URL entry 直接
  // 用现成的远程 URL。R2 上传完成后 entry 仍是同一个 File，previewUrl 不
  // 失效——播放是本地解码 ObjectURL，跟上传链路独立
  const referenceVideos = React.useMemo(() => {
    if (currentModality !== MODALITY.VIDEO) return [];
    if (videoInputMode !== 'omni') return [];
    const slot = imageInputSlots.video;
    if (!slot) return [];
    const v = imageInputsValues?.[slot.key];
    const arr = Array.isArray(v) ? v : isSlotEntry(v) ? [v] : [];
    return arr.filter(isSlotEntry).map((entry, i) => {
      if (isUrlEntry(entry)) {
        return {
          key: `vid-url-${i}-${entry.url}`,
          url: entry.url,
          previewUrl: entry.url,
          name: entry.name || '',
          idx: i,
          entry,
          uploading: false,
          failed: false,
        };
      }
      return {
        key: `vid-${i}-${entry.name}-${entry.size}`,
        url: '',
        previewUrl: fileToUrl(entry),
        name: entry.name,
        idx: i,
        file: entry,
        entry,
        ...fileUploadFlags(entry),
      };
    });
  }, [
    currentModality,
    videoInputMode,
    imageInputSlots,
    imageInputsValues,
    fileToUrl,
    fileUploadFlags,
  ]);

  const referenceAudios = React.useMemo(() => {
    if (currentModality !== MODALITY.VIDEO) return [];
    if (videoInputMode !== 'omni') return [];
    const slot = imageInputSlots.audio;
    if (!slot) return [];
    const v = imageInputsValues?.[slot.key];
    const arr = Array.isArray(v) ? v : isSlotEntry(v) ? [v] : [];
    return arr.filter(isSlotEntry).map((entry, i) => {
      if (isUrlEntry(entry)) {
        return {
          key: `aud-url-${i}-${entry.url}`,
          url: entry.url,
          previewUrl: entry.url,
          name: entry.name || '',
          idx: i,
          entry,
          uploading: false,
          failed: false,
        };
      }
      return {
        key: `aud-${i}-${entry.name}-${entry.size}`,
        url: '',
        previewUrl: fileToUrl(entry),
        name: entry.name,
        idx: i,
        file: entry,
        entry,
        ...fileUploadFlags(entry),
      };
    });
  }, [
    currentModality,
    videoInputMode,
    imageInputSlots,
    imageInputsValues,
    fileToUrl,
    fileUploadFlags,
  ]);

  // 删除视频/音频参考：与 handleRemoveReferenceImage 同构，按 mediaType 分发
  const removeMediaReferenceByKey = React.useCallback(
    (key, mediaType) => {
      const list = mediaType === 'video' ? referenceVideos : referenceAudios;
      const slot = mediaType === 'video' ? imageInputSlots.video : imageInputSlots.audio;
      const target = list.find((x) => x.key === key);
      if (!target || !slot) return;
      const cur = imageInputsValues?.[slot.key];
      if (slot.isArray) {
        const arr = Array.isArray(cur) ? cur.filter(isSlotEntry) : [];
        const removed = arr[target.idx];
        const next = arr.slice();
        next.splice(target.idx, 1);
        setImageInputsValues({ ...imageInputsValues, [slot.key]: next });
        forgetUpload(isFileEntry(removed) ? removed : null);
      } else {
        const removed = cur;
        const { [slot.key]: _, ...rest } = imageInputsValues || {};
        setImageInputsValues(rest);
        forgetUpload(isFileEntry(removed) ? removed : null);
      }
    },
    [
      referenceVideos,
      referenceAudios,
      imageInputSlots,
      imageInputsValues,
      forgetUpload,
    ],
  );

  const handleRemoveReferenceVideo = React.useCallback(
    (key) => removeMediaReferenceByKey(key, 'video'),
    [removeMediaReferenceByKey],
  );
  const handleRemoveReferenceAudio = React.useCallback(
    (key) => removeMediaReferenceByKey(key, 'audio'),
    [removeMediaReferenceByKey],
  );

  // 删除参考图：按 key 找到 idx，从对应 state 里 splice 出去
  // 仅服务"堆叠 / 全能参考"UI；首尾帧模式由 handleRemoveFirstFrame /
  // handleRemoveLastFrame 走独立路径，不会进到这里
  const handleRemoveReferenceImage = React.useCallback(
    (key) => {
      const target = referenceImages.find((x) => x.key === key);
      if (!target) return;
      if (currentModality === MODALITY.MULTIMODAL) {
        const cur = (inputs.imageUrls || []).filter((u) => u && u.trim() !== '');
        const next = cur.slice();
        next.splice(target.idx, 1);
        handleInputChange('imageUrls', next.length > 0 ? next : ['']);
        if (next.length === 0) handleInputChange('imageEnabled', false);
        return;
      }
      const refSlot = imageInputSlots.reference;
      if (
        (currentModality === MODALITY.IMAGE ||
          currentModality === MODALITY.VIDEO) &&
        refSlot
      ) {
        const cur = imageInputsValues?.[refSlot.key];
        if (refSlot.isArray) {
          // 数组里 File 和 URL entry 混存；删除时按 idx 直接 splice
          const arr = Array.isArray(cur) ? cur.filter(isSlotEntry) : [];
          const removed = arr[target.idx];
          const next = arr.slice();
          next.splice(target.idx, 1);
          setImageInputsValues({
            ...imageInputsValues,
            [refSlot.key]: next,
          });
          forgetUpload(isFileEntry(removed) ? removed : null);
        } else {
          const removed = cur;
          const { [refSlot.key]: _, ...rest } = imageInputsValues || {};
          setImageInputsValues(rest);
          forgetUpload(isFileEntry(removed) ? removed : null);
        }
      }
    },
    [
      currentModality,
      referenceImages,
      inputs.imageUrls,
      imageInputsValues,
      imageInputSlots,
      handleInputChange,
      forgetUpload,
    ],
  );

  // 「以此图继续编辑」：把历史 / 已生成图片作为参考图加入当前输入框堆叠。
  //
  // 源图有两种形态：
  //   - data:  base64 内嵌（image / multimodal 模型的历史记录是这种）
  //   - http(s) 远程 URL（视频模型 R2 直传后历史记录是这种）
  //
  // 决策矩阵（按当前选中模型的 modality 区分）：
  //
  //                  | 源 = data:                 | 源 = http(s):
  //   ---------------|----------------------------|--------------------------
  //   VIDEO          | dataUrlToFile → File entry | **直接以 URL entry 入槽**
  //   （目标=R2 URL） | → handleAddReferenceImage  | （0 下载、0 上传、立刻
  //                  | 内部 startUpload 触发上传  | 出缩略图）
  //   ---------------|----------------------------|--------------------------
  //   非 VIDEO        | dataUrlToFile（直接复用    | urlToFile（下载本地字节，
  //   （目标=本地     | 本地字节）                 | 喂回输入槽——MULTIMODAL
  //   字节）          |                            | 内部 FileReader 转 base64；
  //                  |                            | IMAGE 保留 File 给
  //                  |                            | multipart edits）
  const handleContinueEdit = React.useCallback(
    async (img) => {
      const src = img?.url;
      if (typeof src !== 'string' || src.length === 0) return;
      const isDataUrl = src.startsWith('data:');
      const isHttpUrl = /^https?:\/\//i.test(src);
      if (!isDataUrl && !isHttpUrl) {
        Toast.warning({
          content: t('不支持的图片地址，无法继续编辑'),
          duration: 2,
        });
        return;
      }
      // 远程 URL 但不属于我们 R2 公网域：浏览器 fetch/xhr 会被 CORS 拦掉，
      // 没意义让用户点了再失败。VIDEO 模态走零下载快路径不受影响——只复用
      // URL 字符串，不取字节。
      if (isHttpUrl && currentModality !== MODALITY.VIDEO) {
        const base = (
          localStorage.getItem('storage_public_base_url') || ''
        ).replace(/\/$/, '');
        if (!base || !(src.startsWith(base + '/') || src === base)) {
          Toast.warning({
            content: t('外部图片暂不支持继续编辑'),
            duration: 2,
          });
          return;
        }
      }

      try {
        // VIDEO + 远程 URL：**零下载快路径**——把 { url, name } 直接塞进
        // 槽位，发送时 send 路径直接复用 entry.url；点击 → 立刻出缩略图
        if (currentModality === MODALITY.VIDEO && isHttpUrl) {
          // 从 URL 末段抠一个文件名做缩略图旁的展示文本
          let name = '';
          try {
            const u = new URL(src);
            name = decodeURIComponent(u.pathname.split('/').pop() || '');
          } catch {
            name = '';
          }
          handleAddReferenceImage({ url: src, name });
          return;
        }

        // 其它分支：仍需要本地字节
        //   - VIDEO + data URL → File → 内部 startUpload 触发 R2 上传
        //   - MULTIMODAL → File → 内部 FileReader 转 base64 进 imageUrls
        //   - IMAGE       → File → 写入 reference slot 给 multipart edits
        const file = isDataUrl
          ? await dataUrlToFile(src, 'continue')
          : await urlToFile(src, 'continue');
        handleAddReferenceImage(file);
      } catch (err) {
        Toast.error({
          content:
            err?.message ||
            (isHttpUrl
              ? t('下载远程图片失败，请检查 R2 桶 CORS 是否放行 GET')
              : t('填入参考图失败')),
          duration: 2.5,
        });
      }
    },
    [currentModality, handleAddReferenceImage, t],
  );

  return (
    <PlaygroundProvider value={playgroundContextValue}>
      <div className='h-full'>
        <Layout className='h-full bg-transparent flex flex-col md:flex-row'>
          {/* 左侧：只剩会话列表（可在移动端通过 floating 按钮展开覆盖层）。
              模型选择 / 新会话 / 参数 / 附件全部下沉到 ChatArea 底部的输入栏。 */}
          {(showSettings || !isMobile) && (
            <Layout.Sider
              className={`
              bg-transparent border-r-0 flex-shrink-0 overflow-auto mt-[60px]
              ${
                isMobile
                  ? 'fixed top-0 left-0 right-0 bottom-0 z-[1000] w-full h-auto bg-white shadow-lg'
                  : 'relative z-[1] w-64 h-[calc(100vh-66px)]'
              }
            `}
              width={isMobile ? '100%' : 256}
            >
              {/* SessionList 自己处理内部滚动；外层只给 padding 和满高 */}
              <div className='h-full p-3 flex flex-col min-h-0'>
                <SessionList
                  sessions={sessions}
                  activeId={activeSessionId}
                  onSwitch={(id) => {
                    switchSession(id);
                    if (isMobile) setShowSettings(false);
                  }}
                  onCreate={() => {
                    handleNewChat();
                    if (isMobile) setShowSettings(false);
                  }}
                  onRename={renameSession}
                  onDelete={deleteSession}
                />
              </div>
            </Layout.Sider>
          )}

          <Layout.Content className='relative flex-1 overflow-hidden'>
            <div className='overflow-hidden flex flex-col lg:flex-row h-[calc(100vh-66px)] mt-[60px]'>
              <div className='flex-1 flex flex-col'>
                <ChatArea
                  chatRef={chatRef}
                  message={message}
                  inputs={inputs}
                  styleState={styleState}
                  showDebugPanel={showDebugPanel}
                  roleInfo={roleInfo}
                  onMessageSend={onMessageSend}
                  onMessageCopy={messageActions.handleMessageCopy}
                  onMessageReset={messageActions.handleMessageReset}
                  onMessageDelete={messageActions.handleMessageDelete}
                  onStopGenerator={onStopGenerator}
                  onClearMessages={handleClearMessages}
                  onToggleDebugPanel={() => setShowDebugPanel(!showDebugPanel)}
                  onMessageEdit={handleMessageEdit}
                  editingMessageId={editingMessageId}
                  editValue={editValue}
                  onEditValueChange={setEditValue}
                  onEditSave={handleEditSave}
                  onEditCancel={handleEditCancel}
                  onToggleReasoningExpansion={toggleReasoningExpansion}
                  onImageContinueEdit={handleContinueEdit}
                  imageSupportsContinueEdit={supportsImageInputSlot}
                  // UnifiedInputBar 透传
                  modelEntries={modelEntries}
                  currentModelEntry={currentModelEntry}
                  currentModality={currentModality}
                  onModelGroupChange={handleModelGroupChange}
                  paramSchema={imageParamSchema}
                  paramValues={imageParamValues}
                  onParamValuesChange={setImageParamValues}
                  loading={isAnyGenerating}
                  acceptsReferenceImage={acceptsReferenceImage}
                  showUploadButton={showUploadButton}
                  referenceImages={referenceImages}
                  onAddReferenceImage={handleAddReferenceImage}
                  onRemoveReferenceImage={handleRemoveReferenceImage}
                  onRetryUpload={retryUpload}
                  // 视频「全能参考 / 首尾帧」模式控制
                  videoInputModeAvailable={videoInputModeAvailable}
                  videoInputMode={videoInputMode}
                  onVideoInputModeChange={handleVideoInputModeChange}
                  firstLastFrameImages={firstLastFrameImages}
                  onSwapFirstLastFrame={handleSwapFirstLastFrame}
                  onRemoveFirstFrame={handleRemoveFirstFrame}
                  onRemoveLastFrame={handleRemoveLastFrame}
                  referenceVideos={referenceVideos}
                  referenceAudios={referenceAudios}
                  onRemoveReferenceVideo={handleRemoveReferenceVideo}
                  onRemoveReferenceAudio={handleRemoveReferenceAudio}
                  pendingText={pendingDeepLinkText}
                  onPendingTextConsumed={() => setPendingDeepLinkText(null)}
                />
              </div>

              {/* 右侧面板 - 桌面端：参数/调试 Tab 切换，可通过输入栏的 ⚙ 收起 */}
              {showDebugPanel && !isMobile && (
                <div className='w-96 flex-shrink-0 h-full'>
                  <PlaygroundRightPanel
                    styleState={styleState}
                    activeTab={rightPanelTab}
                    onActiveTabChange={setRightPanelTab}
                    inputs={inputs}
                    parameterEnabled={parameterEnabled}
                    currentModality={currentModality}
                    customRequestMode={customRequestMode}
                    customRequestBody={customRequestBody}
                    onInputChange={handleInputChange}
                    onParameterToggle={handleParameterToggle}
                    onCustomRequestModeChange={setCustomRequestMode}
                    onCustomRequestBodyChange={setCustomRequestBody}
                    previewPayload={previewPayload}
                    paramSchema={imageParamSchema}
                    paramValues={imageParamValues}
                    onParamValuesChange={setImageParamValues}
                    debugData={debugData}
                    activeDebugTab={activeDebugTab}
                    onActiveDebugTabChange={setActiveDebugTab}
                  />
                </div>
              )}
            </div>

            {/* 右侧面板 - 移动端覆盖层 */}
            {showDebugPanel && isMobile && (
              <div className='fixed top-0 left-0 right-0 bottom-0 z-[1000] bg-white overflow-auto shadow-lg'>
                <PlaygroundRightPanel
                  styleState={styleState}
                  activeTab={rightPanelTab}
                  onActiveTabChange={setRightPanelTab}
                  onClose={() => setShowDebugPanel(false)}
                  inputs={inputs}
                  parameterEnabled={parameterEnabled}
                  currentModality={currentModality}
                  customRequestMode={customRequestMode}
                  customRequestBody={customRequestBody}
                  onInputChange={handleInputChange}
                  onParameterToggle={handleParameterToggle}
                  onCustomRequestModeChange={setCustomRequestMode}
                  onCustomRequestBodyChange={setCustomRequestBody}
                  previewPayload={previewPayload}
                  paramSchema={imageParamSchema}
                  paramValues={imageParamValues}
                  onParamValuesChange={setImageParamValues}
                  debugData={debugData}
                  activeDebugTab={activeDebugTab}
                  onActiveDebugTabChange={setActiveDebugTab}
                />
              </div>
            )}

            {/* 浮动按钮：移动端用于切换会话列表 / 调试面板 */}
            <FloatingButtons
              styleState={styleState}
              showSettings={showSettings}
              showDebugPanel={showDebugPanel}
              onToggleSettings={() => setShowSettings(!showSettings)}
              onToggleDebugPanel={() => setShowDebugPanel(!showDebugPanel)}
            />
          </Layout.Content>
        </Layout>
      </div>
    </PlaygroundProvider>
  );
};

export default Playground;
