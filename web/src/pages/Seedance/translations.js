/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

For commercial licensing, please contact support@quantumnous.com
*/

// Seedance 落地页专属多语言字典。
// 约定：键为中文源串（与组件内 t('中文') 完全一致）；zh-CN 直接用键本身，
// 故此处只提供 zh-TW / en / ja / ru / fr / vi。SEO 标题/描述同样多语言。
export const SEEDANCE_SEO_TITLE = 'Seedance 2.0 for Amux —— 多模态 AI 视频生成 API';
export const SEEDANCE_SEO_DESC =
  '在 Amux API 上调用 Seedance 2.0（含 Fast）多模态视频生成模型：支持文生视频、图生视频，最高 1080p 电影级画质，兼容 OpenAI 与火山方舟 V3 协议端点。';

export const SEEDANCE_I18N = {
  'zh-TW': {
    'Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。':
      'Seedance 2.0 採用統一的多模態音視訊聯合生成架構，支援文字、圖像、音訊與影片輸入，具備業界領先的多模態內容參考與編輯能力。',
    '立即体验': '立即體驗',
    '获取 API 文档': '取得 API 文件',
    '在 Amux API 上体验': '在 Amux API 上體驗',
    '精准控制，生成连贯的电影级 AI 视频': '精準控制，生成連貫的電影級 AI 影片',
    '通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。':
      '透過精準控制生成連貫的電影級 AI 影片（含擬真人像），完美適用於行銷推廣、應用開發及專業製作工作流程；使用 Seedance 2.0 Fast，可實現更快的生成速度、更低的成本以及大規模快速迭代。',
    '效果展示': '效果展示',
    '为每个场景而生的专业视频': '為每個場景而生的專業影片',
    '无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。':
      '無論是拍攝短片、剪輯音樂影片，還是規模化產出廣告內容，Seedance 2.0 都能勝任。',
    '短片与电影叙事。': '短片與電影敘事。',
    '多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。':
      '多鏡頭敘事，人物形象一致，電影級攝影機拍攝，原生音訊同步。',
    '电影级动态运镜。': '電影級動態運鏡。',
    '复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。':
      '複刻參考影片中的跟蹤、環繞與快速轉場，畫面運動流暢清晰。',
    '逼真物理与动作连贯。': '擬真物理與動作連貫。',
    '密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。':
      '密集打鬥、碰撞與子彈時間下，時序、重量感與動量保持一致。',
    '活动推广视频。': '活動推廣影片。',
    '一致品牌形象与强叙事性的宣传视频，无需制作团队。':
      '一致品牌形象與強敘事性的宣傳影片，無需製作團隊。',
    '高影响力视频广告。': '高影響力影片廣告。',
    '由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。':
      '由產品照片生成精美廣告，動態演示與多種變體，鎖定每一幀的品牌一致性。',
    '音频节奏引导。': '音訊節奏引導。',
    '以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。':
      '以音樂為節奏參考，畫面動作與剪輯對齊節拍情緒，輕鬆做卡點影片。',
    '悬浮播放声音': '懸停播放聲音',
    '快速接入': '快速接入',
    '两步生成你的第一条视频': '兩步生成你的第一支影片',
    '同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。':
      '同時相容 OpenAI 風格與火山方舟 V3 官方協定端點，統一封裝為非同步任務介面，提交即走、輪詢取回。',
    '火山 V3': '火山 V3',
    '支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。':
      '支援火山方舟 V3 官方協定端點：只需把 Base URL 換成本站位址，即可將現有火山 SDK / 用戶端無縫遷移接入。',
    '提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。':
      '提示：將 model 換成 doubao-seedance-2.0-fast 即可使用極速版。',
    '① 提交生成任务': '① 提交生成任務',
    '② 轮询任务结果': '② 輪詢任務結果',
    '常见问题': '常見問題',
    '还有什么疑问吗？': '還有什麼疑問嗎？',
    '我们整理了最常被问到的问题。': '我們整理了最常被問到的問題。',
    'Seedance 2.0 是什么？': 'Seedance 2.0 是什麼？',
    'Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。':
      'Seedance 2.0 是字節跳動推出的多模態 AI 影片生成模型，支援文字、圖像、音訊與影片等多模態參考輸入，能生成具備多鏡頭一致性與原生音訊的電影級連貫影片，並支援擬真人像。',
    '标准模式和极速模式有什么区别？': '標準模式和極速模式有什麼區別？',
    '标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。':
      '標準模式（doubao-seedance-2.0）面向高品質成片，支援複雜運動與多鏡頭生成，最高 1080p，適合專業製作；極速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，適合提示詞測試、批次生成與快速迭代。兩者共用同一套介面，切換 model 即可。',
    '支持哪些输入与生成方式？': '支援哪些輸入與生成方式？',
    '支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。':
      '支援文生影片與圖生影片，可用文字、圖像、音訊、影片等多模態素材作為參考；解析度提供 720p / 1080p（極速版固定 720p）。',
    '可以用它创作什么？': '可以用它創作什麼？',
    '短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。':
      '短片與電影敘事、動作與視覺特效、活動推廣影片、高影響力影片廣告、音樂卡點 MV 等，涵蓋行銷推廣、應用開發與專業製作工作流程。',
    '如何接入调用？': '如何接入呼叫？',
    '提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。':
      '提供 OpenAI 風格（/v1/video/generations）與火山方舟 V3 官方協定端點（/api/v3/contents/generations/tasks）兩種方式，均為非同步任務：提交後輪詢取回結果。火山 V3 只需替換 Base URL 即可遷移現有用戶端。',
    '价格与渠道是怎样的？': '價格與渠道是怎樣的？',
    'premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。':
      'premium/doubao 渠道享官方 8 折，低至約 $0.12 / 秒，性價比首選（該渠道真人解限不能 100% 成功）；商業應用如需穩定真人解限，可使用 premium/doubao_video_max 渠道，價格為官方的 1.2 倍。',
    '使用 premium 渠道有什么要求？': '使用 premium 渠道有什麼要求？',
    'premium 渠道模型需累计充值满 $20 后解锁使用。':
      'premium 渠道模型需累計儲值滿 $20 後解鎖使用。',
    '需要视频剪辑经验吗？': '需要影片剪輯經驗嗎？',
    '不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。':
      '不需要。寫一句提示詞或上傳參考素材即可生成；進階使用者還能進一步控制運鏡、轉場與時長等，獲得更深度的創作掌控。',
    '即刻开始': '即刻開始',
    '用 Seedance 2.0 开始你的创作': '用 Seedance 2.0 開始你的創作',
    'Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。':
      'Seedance 2.0 由 premium 渠道提供，累計儲值滿 $20 即可解鎖使用，隨後幾分鐘內即可生成你的第一支 AI 影片。',
    '立即生成视频': '立即生成影片',
    '查看模型与价格': '檢視模型與價格',
    [SEEDANCE_SEO_TITLE]: 'Seedance 2.0 for Amux —— 多模態 AI 影片生成 API',
    [SEEDANCE_SEO_DESC]:
      '在 Amux API 上呼叫 Seedance 2.0（含 Fast）多模態影片生成模型：支援文生影片、圖生影片，最高 1080p 電影級畫質，相容 OpenAI 與火山方舟 V3 協定端點。',
  },

  en: {
    'Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。':
      'Seedance 2.0 uses a unified multimodal audio-video joint generation architecture, accepting text, image, audio, and video inputs, with industry-leading multimodal reference and editing capabilities.',
    '立即体验': 'Try Now',
    '获取 API 文档': 'Get API Docs',
    '在 Amux API 上体验': 'Experience on Amux API',
    '精准控制，生成连贯的电影级 AI 视频': 'Precise control for coherent, cinematic AI video',
    '通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。':
      'Generate coherent, cinematic AI video — including lifelike humans — with precise control, perfect for marketing, app development, and professional production. With Seedance 2.0 Fast, get faster generation, lower cost, and large-scale rapid iteration.',
    '效果展示': 'Showcase',
    '为每个场景而生的专业视频': 'Professional video for every use case',
    '无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。':
      "Whether you're shooting a short film, cutting a music video, or scaling ad content — Seedance 2.0 delivers.",
    '短片与电影叙事。': 'Short Film & Cinematic Storytelling.',
    '多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。':
      'Multi-shot narratives with consistent characters, cinematic camerawork, and native audio sync.',
    '电影级动态运镜。': 'Cinematic Dynamic Camera.',
    '复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。':
      'Reproduce tracking, orbiting, and fast transitions from reference videos with smooth, crisp motion.',
    '逼真物理与动作连贯。': 'Realistic Physics & Motion Coherence.',
    '密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。':
      'Through intense fights, collisions, and bullet-time, timing, weight, and momentum stay consistent.',
    '活动推广视频。': 'Campaign-Ready Videos.',
    '一致品牌形象与强叙事性的宣传视频，无需制作团队。':
      'Promotional videos with consistent branding and strong storytelling — no production team needed.',
    '高影响力视频广告。': 'High-Impact Video Ads.',
    '由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。':
      'Generate polished ads from product photos, with dynamic demos, multiple variations, and frame-level brand consistency.',
    '音频节奏引导。': 'Audio Rhythm Guidance.',
    '以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。':
      'Use a music track as a rhythm reference — align motion and edits to the beat and mood for effortless beat-synced videos.',
    '悬浮播放声音': 'Hover to play sound',
    '快速接入': 'Quick Start',
    '两步生成你的第一条视频': 'Generate your first video in two steps',
    '同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。':
      'Compatible with both OpenAI-style and the Volcengine Ark V3 official protocol endpoint, unified as an async task API — submit and poll for results.',
    '火山 V3': 'Volcengine V3',
    '支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。':
      'Supports the Volcengine Ark V3 official protocol endpoint: just swap the Base URL to our address to migrate your existing Volcengine SDK / client seamlessly.',
    '提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。':
      'Tip: switch model to doubao-seedance-2.0-fast to use the fast version.',
    '① 提交生成任务': '① Submit a generation task',
    '② 轮询任务结果': '② Poll for the task result',
    '常见问题': 'FAQ',
    '还有什么疑问吗？': 'Got any questions left?',
    '我们整理了最常被问到的问题。': "We've answered the most frequently asked questions.",
    'Seedance 2.0 是什么？': 'What is Seedance 2.0?',
    'Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。':
      "Seedance 2.0 is ByteDance's multimodal AI video generation model. It accepts text, image, audio, and video references and produces coherent, cinematic video with multi-shot consistency and native audio — including lifelike humans.",
    '标准模式和极速模式有什么区别？': "What's the difference between Standard and Fast modes?",
    '标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。':
      'Standard mode (doubao-seedance-2.0) targets high-quality output, supporting complex motion and multi-shot generation up to 1080p — ideal for professional production. Fast mode (doubao-seedance-2.0-fast) is faster and cheaper, fixed at 720p — ideal for prompt testing, batch generation, and rapid iteration. Both share the same API; just switch the model.',
    '支持哪些输入与生成方式？': 'What inputs and generation modes are supported?',
    '支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。':
      'Text-to-video and image-to-video are supported, using text, image, audio, and video as multimodal references; resolutions of 720p / 1080p are available (Fast is fixed at 720p).',
    '可以用它创作什么？': 'What can I create with it?',
    '短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。':
      'Short films and cinematic narratives, action and VFX clips, campaign videos, high-impact video ads, beat-synced music videos, and more — covering marketing, app development, and professional production workflows.',
    '如何接入调用？': 'How do I integrate it?',
    '提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。':
      'Two methods are provided: OpenAI-style (/v1/video/generations) and the Volcengine Ark V3 official protocol endpoint (/api/v3/contents/generations/tasks), both async tasks — submit then poll for results. For Volcengine V3, just replace the Base URL to migrate your existing client.',
    '价格与渠道是怎样的？': 'What about pricing and channels?',
    'premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。':
      'The premium/doubao channel offers an official 20% discount, as low as ~$0.12/sec — the best value (human-likeness unlock on this channel is not guaranteed 100%). For commercial use needing stable human-likeness unlock, use the premium/doubao_video_max channel at 1.2× the official price.',
    '使用 premium 渠道有什么要求？': 'Are there requirements to use premium channels?',
    'premium 渠道模型需累计充值满 $20 后解锁使用。':
      'Premium channel models unlock after a cumulative top-up of $20.',
    '需要视频剪辑经验吗？': 'Do I need video editing experience?',
    '不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。':
      'No. Write a prompt or upload references and the model handles the rest; advanced users can further control camera moves, transitions, and duration for deeper creative control.',
    '即刻开始': 'Get Started',
    '用 Seedance 2.0 开始你的创作': 'Start creating with Seedance 2.0',
    'Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。':
      'Seedance 2.0 is offered via premium channels; unlock it with a cumulative top-up of $20, then generate your first AI video in minutes.',
    '立即生成视频': 'Generate Video Now',
    '查看模型与价格': 'View models & pricing',
    [SEEDANCE_SEO_TITLE]: 'Seedance 2.0 for Amux — Multimodal AI Video Generation API',
    [SEEDANCE_SEO_DESC]:
      'Call Seedance 2.0 (and Fast) multimodal video generation on Amux API: text-to-video and image-to-video, up to 1080p cinematic quality, compatible with OpenAI and Volcengine Ark V3 protocol endpoints.',
  },

  ja: {
    'Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。':
      'Seedance 2.0 は統一されたマルチモーダル音声・動画統合生成アーキテクチャを採用し、テキスト・画像・音声・動画の入力に対応、業界最先端のマルチモーダル参照・編集能力を備えています。',
    '立即体验': '今すぐ体験',
    '获取 API 文档': 'API ドキュメント',
    '在 Amux API 上体验': 'Amux API で体験',
    '精准控制，生成连贯的电影级 AI 视频': '精密な制御で、一貫した映画品質の AI 動画を生成',
    '通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。':
      '精密な制御で一貫した映画品質の AI 動画（リアルな人物を含む）を生成し、マーケティング・アプリ開発・プロ制作のワークフローに最適。Seedance 2.0 Fast なら、より速い生成・より低いコスト・大規模な高速イテレーションを実現します。',
    '效果展示': 'ショーケース',
    '为每个场景而生的专业视频': 'あらゆるユースケースのためのプロ品質動画',
    '无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。':
      '短編映画の撮影、ミュージックビデオの編集、広告コンテンツの大量生成まで、Seedance 2.0 が対応します。',
    '短片与电影叙事。': '短編・映画的ストーリーテリング。',
    '多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。':
      '一貫したキャラクターによるマルチショット物語、映画的なカメラワーク、ネイティブ音声同期。',
    '电影级动态运镜。': '映画的なダイナミックカメラ。',
    '复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。':
      '参照動画のトラッキング・周回・高速トランジションを再現し、滑らかで鮮明な動きを実現。',
    '逼真物理与动作连贯。': 'リアルな物理と動作の一貫性。',
    '密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。':
      '激しい格闘・衝突・バレットタイムでも、タイミング・重量感・運動量が一貫。',
    '活动推广视频。': 'キャンペーン向け動画。',
    '一致品牌形象与强叙事性的宣传视频，无需制作团队。':
      '一貫したブランディングと強い物語性の宣伝動画を、制作チームなしで。',
    '高影响力视频广告。': 'ハイインパクトな動画広告。',
    '由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。':
      '商品写真から洗練された広告を生成。ダイナミックなデモ、複数のバリエーション、フレーム単位のブランド一貫性。',
    '音频节奏引导。': 'オーディオリズムガイド。',
    '以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。':
      '音楽をリズムの参照に使い、動きと編集をビートとムードに合わせて、ビート同期動画を手軽に作成。',
    '悬浮播放声音': 'ホバーで音声を再生',
    '快速接入': 'クイックスタート',
    '两步生成你的第一条视频': '2 ステップで最初の動画を生成',
    '同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。':
      'OpenAI 形式と火山方舟 V3 公式プロトコルエンドポイントの両方に対応し、非同期タスク API として統一。送信してポーリングで結果を取得。',
    '火山 V3': '火山 V3',
    '支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。':
      '火山方舟 V3 公式プロトコルエンドポイントに対応：Base URL を本サイトのアドレスに変更するだけで、既存の火山 SDK / クライアントをシームレスに移行できます。',
    '提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。':
      'ヒント：model を doubao-seedance-2.0-fast に変更すると高速版を利用できます。',
    '① 提交生成任务': '① 生成タスクを送信',
    '② 轮询任务结果': '② タスク結果をポーリング',
    '常见问题': 'よくある質問',
    '还有什么疑问吗？': 'まだ疑問がありますか？',
    '我们整理了最常被问到的问题。': '最もよくある質問にお答えします。',
    'Seedance 2.0 是什么？': 'Seedance 2.0 とは？',
    'Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。':
      'Seedance 2.0 は ByteDance が提供するマルチモーダル AI 動画生成モデルです。テキスト・画像・音声・動画などのマルチモーダル参照入力に対応し、マルチショットの一貫性とネイティブ音声を備えた映画品質の一貫した動画を生成、リアルな人物にも対応します。',
    '标准模式和极速模式有什么区别？': '標準モードと高速モードの違いは？',
    '标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。':
      '標準モード（doubao-seedance-2.0）は高品質な完成映像向けで、複雑な動きとマルチショット生成に対応し最大 1080p、プロ制作に最適。高速モード（doubao-seedance-2.0-fast）はより速く低コストで 720p 固定、プロンプトテスト・バッチ生成・高速イテレーションに最適。両者は同じ API を共有し、model を切り替えるだけです。',
    '支持哪些输入与生成方式？': 'どのような入力・生成方式に対応していますか？',
    '支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。':
      'テキストから動画、画像から動画に対応し、テキスト・画像・音声・動画などのマルチモーダル素材を参照として使用できます。解像度は 720p / 1080p（高速版は 720p 固定）。',
    '可以用它创作什么？': '何を作れますか？',
    '短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。':
      '短編・映画的物語、アクションと VFX クリップ、キャンペーン動画、ハイインパクトな動画広告、ビート同期のミュージックビデオなど、マーケティング・アプリ開発・プロ制作のワークフローを幅広くカバーします。',
    '如何接入调用？': 'どうやって統合しますか？',
    '提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。':
      'OpenAI 形式（/v1/video/generations）と火山方舟 V3 公式プロトコルエンドポイント（/api/v3/contents/generations/tasks）の 2 つの方式を提供。いずれも非同期タスクで、送信後にポーリングで結果を取得します。火山 V3 は Base URL を置き換えるだけで既存クライアントを移行できます。',
    '价格与渠道是怎样的？': '価格とチャンネルは？',
    'premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。':
      'premium/doubao チャンネルは公式 20% オフで、最安約 $0.12/秒とコストパフォーマンスに優れます（このチャンネルの人物アンロックは 100% 成功するとは限りません）。安定した人物アンロックが必要な商用利用には、公式価格の 1.2 倍の premium/doubao_video_max チャンネルをご利用ください。',
    '使用 premium 渠道有什么要求？': 'premium チャンネルの利用条件は？',
    'premium 渠道模型需累计充值满 $20 后解锁使用。':
      'premium チャンネルのモデルは、累計 $20 のチャージ後に利用可能になります。',
    '需要视频剪辑经验吗？': '動画編集の経験は必要ですか？',
    '不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。':
      '不要です。プロンプトを書くか参照素材をアップロードするだけでモデルが処理します。上級者はカメラワーク・トランジション・尺などをさらに制御し、より深い創作コントロールが可能です。',
    '即刻开始': 'さあ始めよう',
    '用 Seedance 2.0 开始你的创作': 'Seedance 2.0 で創作を始めよう',
    'Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。':
      'Seedance 2.0 は premium チャンネルで提供されます。累計 $20 のチャージで利用可能になり、数分で最初の AI 動画を生成できます。',
    '立即生成视频': '今すぐ動画を生成',
    '查看模型与价格': 'モデルと価格を見る',
    [SEEDANCE_SEO_TITLE]: 'Seedance 2.0 for Amux — マルチモーダル AI 動画生成 API',
    [SEEDANCE_SEO_DESC]:
      'Amux API で Seedance 2.0（Fast 含む）マルチモーダル動画生成を利用：テキストから動画・画像から動画、最大 1080p の映画品質、OpenAI と火山方舟 V3 プロトコルエンドポイントに対応。',
  },

  ru: {
    'Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。':
      'Seedance 2.0 использует единую мультимодальную архитектуру совместной генерации аудио и видео, поддерживает ввод текста, изображений, аудио и видео и обладает передовыми в отрасли возможностями мультимодальной референции и редактирования.',
    '立即体验': 'Попробовать',
    '获取 API 文档': 'Документация API',
    '在 Amux API 上体验': 'Попробуйте в Amux API',
    '精准控制，生成连贯的电影级 AI 视频': 'Точный контроль для связного кинематографичного AI-видео',
    '通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。':
      'Создавайте связное кинематографичное AI-видео (включая реалистичных людей) с точным контролем — идеально для маркетинга, разработки приложений и профессионального производства. С Seedance 2.0 Fast вы получаете более быструю генерацию, меньшую стоимость и масштабные быстрые итерации.',
    '效果展示': 'Примеры',
    '为每个场景而生的专业视频': 'Профессиональное видео для любых задач',
    '无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。':
      'Снимаете короткометражку, монтируете музыкальное видео или масштабируете рекламный контент — Seedance 2.0 справится.',
    '短片与电影叙事。': 'Короткометражки и кинонарратив.',
    '多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。':
      'Многокадровые истории с согласованными персонажами, кинематографичной съёмкой и нативной синхронизацией звука.',
    '电影级动态运镜。': 'Кинематографичная динамичная камера.',
    '复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。':
      'Воспроизводит трекинг, облёты и быстрые переходы из референсных видео с плавным, чётким движением.',
    '逼真物理与动作连贯。': 'Реалистичная физика и связность движений.',
    '密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。':
      'В насыщенных боях, столкновениях и буллет-тайме тайминг, вес и импульс остаются согласованными.',
    '活动推广视频。': 'Видео для кампаний.',
    '一致品牌形象与强叙事性的宣传视频，无需制作团队。':
      'Рекламные видео с единым брендингом и сильным повествованием — без производственной команды.',
    '高影响力视频广告。': 'Эффектная видеореклама.',
    '由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。':
      'Создавайте отполированную рекламу из фото товара: динамичные демо, множество вариаций и покадровая согласованность бренда.',
    '音频节奏引导。': 'Ведение по ритму аудио.',
    '以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。':
      'Используйте музыкальную дорожку как ритмическую референцию — синхронизируйте движение и монтаж с битом и настроением для лёгких ритмичных видео.',
    '悬浮播放声音': 'Наведите для звука',
    '快速接入': 'Быстрый старт',
    '两步生成你的第一条视频': 'Создайте первое видео в два шага',
    '同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。':
      'Совместимо со стилем OpenAI и официальным эндпоинтом протокола Volcengine Ark V3, объединено в асинхронный API задач — отправьте и опрашивайте результат.',
    '火山 V3': 'Volcengine V3',
    '支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。':
      'Поддержка официального эндпоинта протокола Volcengine Ark V3: просто замените Base URL на адрес нашего сайта, чтобы бесшовно перенести существующий Volcengine SDK / клиент.',
    '提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。':
      'Совет: смените model на doubao-seedance-2.0-fast, чтобы использовать быструю версию.',
    '① 提交生成任务': '① Отправьте задачу генерации',
    '② 轮询任务结果': '② Опросите результат задачи',
    '常见问题': 'Частые вопросы',
    '还有什么疑问吗？': 'Остались вопросы?',
    '我们整理了最常被问到的问题。': 'Мы ответили на самые частые вопросы.',
    'Seedance 2.0 是什么？': 'Что такое Seedance 2.0?',
    'Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。':
      'Seedance 2.0 — мультимодальная модель генерации видео от ByteDance. Она принимает текст, изображения, аудио и видео в качестве референсов и создаёт связное кинематографичное видео с многокадровой согласованностью и нативным звуком, включая реалистичных людей.',
    '标准模式和极速模式有什么区别？': 'В чём разница между стандартным и быстрым режимами?',
    '标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。':
      'Стандартный режим (doubao-seedance-2.0) ориентирован на высокое качество, поддерживает сложное движение и многокадровую генерацию до 1080p, идеален для профессионального производства. Быстрый режим (doubao-seedance-2.0-fast) быстрее и дешевле, фиксированный 720p, идеален для тестирования промптов, пакетной генерации и быстрых итераций. Оба используют один API — просто смените model.',
    '支持哪些输入与生成方式？': 'Какие входные данные и способы генерации поддерживаются?',
    '支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。':
      'Поддерживаются генерация видео из текста и из изображения; в качестве мультимодальных референсов можно использовать текст, изображения, аудио и видео. Доступны разрешения 720p / 1080p (быстрая версия фиксирована на 720p).',
    '可以用它创作什么？': 'Что можно создать?',
    '短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。':
      'Короткометражки и кинонарратив, экшн и VFX-клипы, видео для кампаний, эффектная видеореклама, ритмичные музыкальные видео и многое другое — для маркетинга, разработки приложений и профессионального производства.',
    '如何接入调用？': 'Как интегрировать?',
    '提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。':
      'Доступны два способа: стиль OpenAI (/v1/video/generations) и официальный эндпоинт протокола Volcengine Ark V3 (/api/v3/contents/generations/tasks), оба — асинхронные задачи: отправьте, затем опрашивайте результат. Для Volcengine V3 достаточно заменить Base URL, чтобы перенести существующий клиент.',
    '价格与渠道是怎样的？': 'Какие цены и каналы?',
    'premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。':
      'Канал premium/doubao предлагает официальную скидку 20%, от ~$0,12/сек — лучшее соотношение цены и качества (разблокировка реалистичных людей на этом канале не гарантируется на 100%). Для коммерческого использования со стабильной разблокировкой используйте канал premium/doubao_video_max по цене 1,2× от официальной.',
    '使用 premium 渠道有什么要求？': 'Какие требования для premium-каналов?',
    'premium 渠道模型需累计充值满 $20 后解锁使用。':
      'Модели premium-каналов разблокируются после суммарного пополнения на $20.',
    '需要视频剪辑经验吗？': 'Нужен ли опыт видеомонтажа?',
    '不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。':
      'Нет. Напишите промпт или загрузите референсы — модель сделает остальное; продвинутые пользователи могут дополнительно управлять движением камеры, переходами и длительностью для более глубокого контроля.',
    '即刻开始': 'Начать',
    '用 Seedance 2.0 开始你的创作': 'Начните творить с Seedance 2.0',
    'Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。':
      'Seedance 2.0 доступна через premium-каналы; разблокируйте её суммарным пополнением на $20 и создайте первое AI-видео за минуты.',
    '立即生成视频': 'Сгенерировать видео',
    '查看模型与价格': 'Модели и цены',
    [SEEDANCE_SEO_TITLE]: 'Seedance 2.0 for Amux — API мультимодальной генерации AI-видео',
    [SEEDANCE_SEO_DESC]:
      'Используйте Seedance 2.0 (и Fast) для мультимодальной генерации видео в Amux API: текст-в-видео и изображение-в-видео, кинокачество до 1080p, совместимость с эндпоинтами OpenAI и Volcengine Ark V3.',
  },

  fr: {
    'Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。':
      "Seedance 2.0 adopte une architecture unifiée de génération conjointe audio-vidéo multimodale, prenant en charge les entrées texte, image, audio et vidéo, avec des capacités de référence et d'édition multimodales à la pointe du secteur.",
    '立即体验': 'Essayer',
    '获取 API 文档': 'Documentation API',
    '在 Amux API 上体验': 'Découvrir sur Amux API',
    '精准控制，生成连贯的电影级 AI 视频': 'Un contrôle précis pour une vidéo IA cinématographique et cohérente',
    '通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。':
      "Générez une vidéo IA cinématographique et cohérente (avec des personnages réalistes) grâce à un contrôle précis — parfait pour le marketing, le développement d'applications et la production professionnelle. Avec Seedance 2.0 Fast, profitez d'une génération plus rapide, de coûts réduits et d'itérations rapides à grande échelle.",
    '效果展示': 'Démonstrations',
    '为每个场景而生的专业视频': 'Une vidéo professionnelle pour chaque usage',
    '无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。':
      'Que vous tourniez un court-métrage, montiez un clip musical ou produisiez du contenu publicitaire à grande échelle, Seedance 2.0 est à la hauteur.',
    '短片与电影叙事。': 'Court-métrage et narration cinématographique.',
    '多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。':
      'Récits multi-plans avec des personnages cohérents, une prise de vue cinématographique et une synchronisation audio native.',
    '电影级动态运镜。': 'Mouvements de caméra cinématographiques.',
    '复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。':
      'Reproduit le suivi, les rotations et les transitions rapides des vidéos de référence avec un mouvement fluide et net.',
    '逼真物理与动作连贯。': 'Physique réaliste et cohérence des mouvements.',
    '密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。':
      "Dans les combats intenses, les collisions et le bullet time, le timing, le poids et l'élan restent cohérents.",
    '活动推广视频。': 'Vidéos prêtes pour vos campagnes.',
    '一致品牌形象与强叙事性的宣传视频，无需制作团队。':
      "Des vidéos promotionnelles avec une image de marque cohérente et une forte narration — sans équipe de production.",
    '高影响力视频广告。': 'Publicités vidéo à fort impact.',
    '由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。':
      'Générez de superbes publicités à partir de photos produit : démos dynamiques, multiples variantes et cohérence de marque image par image.',
    '音频节奏引导。': 'Guidage par le rythme audio.',
    '以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。':
      "Utilisez une piste musicale comme référence rythmique — alignez le mouvement et le montage sur le tempo et l'ambiance pour des vidéos rythmées sans effort.",
    '悬浮播放声音': 'Survolez pour le son',
    '快速接入': 'Démarrage rapide',
    '两步生成你的第一条视频': 'Générez votre première vidéo en deux étapes',
    '同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。':
      "Compatible à la fois avec le style OpenAI et l'endpoint du protocole officiel Volcengine Ark V3, unifiés en une API de tâches asynchrones — soumettez puis interrogez le résultat.",
    '火山 V3': 'Volcengine V3',
    '支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。':
      "Prend en charge l'endpoint du protocole officiel Volcengine Ark V3 : remplacez simplement la Base URL par l'adresse de notre site pour migrer votre SDK / client Volcengine existant en toute fluidité.",
    '提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。':
      'Astuce : remplacez model par doubao-seedance-2.0-fast pour utiliser la version rapide.',
    '① 提交生成任务': '① Soumettre une tâche de génération',
    '② 轮询任务结果': '② Interroger le résultat de la tâche',
    '常见问题': 'FAQ',
    '还有什么疑问吗？': 'Encore des questions ?',
    '我们整理了最常被问到的问题。': 'Nous avons répondu aux questions les plus fréquentes.',
    'Seedance 2.0 是什么？': "Qu'est-ce que Seedance 2.0 ?",
    'Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。':
      "Seedance 2.0 est le modèle de génération vidéo IA multimodal de ByteDance. Il accepte le texte, les images, l'audio et la vidéo comme références et produit une vidéo cinématographique cohérente avec une cohérence multi-plans et un audio natif, y compris des personnages réalistes.",
    '标准模式和极速模式有什么区别？': 'Quelle est la différence entre les modes Standard et Fast ?',
    '标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。':
      "Le mode Standard (doubao-seedance-2.0) vise une qualité élevée, prend en charge les mouvements complexes et la génération multi-plans jusqu'à 1080p, idéal pour la production professionnelle. Le mode Fast (doubao-seedance-2.0-fast) est plus rapide et économique, fixé à 720p, idéal pour tester des prompts, la génération par lots et l'itération rapide. Les deux partagent la même API — il suffit de changer le model.",
    '支持哪些输入与生成方式？': 'Quelles entrées et méthodes de génération sont prises en charge ?',
    '支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。':
      "La génération texte-vers-vidéo et image-vers-vidéo est prise en charge, avec du texte, des images, de l'audio et de la vidéo comme références multimodales ; résolutions 720p / 1080p disponibles (Fast fixé à 720p).",
    '可以用它创作什么？': 'Que peut-on créer ?',
    '短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。':
      "Courts-métrages et récits cinématographiques, clips d'action et VFX, vidéos de campagne, publicités vidéo à fort impact, clips musicaux rythmés, et plus — pour le marketing, le développement d'applications et la production professionnelle.",
    '如何接入调用？': 'Comment intégrer ?',
    '提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。':
      "Deux méthodes sont proposées : le style OpenAI (/v1/video/generations) et l'endpoint du protocole officiel Volcengine Ark V3 (/api/v3/contents/generations/tasks), toutes deux en tâches asynchrones : soumettez puis interrogez le résultat. Pour Volcengine V3, remplacez simplement la Base URL pour migrer votre client existant.",
    '价格与渠道是怎样的？': 'Quels sont les tarifs et les canaux ?',
    'premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。':
      "Le canal premium/doubao offre une remise officielle de 20%, dès ~0,12 $/s, le meilleur rapport qualité-prix (le déblocage des personnages réalistes sur ce canal n'est pas garanti à 100%). Pour un usage commercial nécessitant un déblocage stable, utilisez le canal premium/doubao_video_max à 1,2× le prix officiel.",
    '使用 premium 渠道有什么要求？': 'Quelles sont les conditions pour les canaux premium ?',
    'premium 渠道模型需累计充值满 $20 后解锁使用。':
      'Les modèles des canaux premium se débloquent après un rechargement cumulé de 20 $.',
    '需要视频剪辑经验吗？': "Faut-il de l'expérience en montage vidéo ?",
    '不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。':
      "Non. Écrivez un prompt ou téléversez des références, le modèle s'occupe du reste ; les utilisateurs avancés peuvent affiner les mouvements de caméra, les transitions et la durée pour un contrôle créatif plus poussé.",
    '即刻开始': 'Commencer',
    '用 Seedance 2.0 开始你的创作': 'Commencez à créer avec Seedance 2.0',
    'Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。':
      'Seedance 2.0 est proposé via les canaux premium ; débloquez-le avec un rechargement cumulé de 20 $, puis générez votre première vidéo IA en quelques minutes.',
    '立即生成视频': 'Générer une vidéo',
    '查看模型与价格': 'Voir les modèles et tarifs',
    [SEEDANCE_SEO_TITLE]: 'Seedance 2.0 for Amux — API de génération vidéo IA multimodale',
    [SEEDANCE_SEO_DESC]:
      "Utilisez Seedance 2.0 (et Fast) pour la génération vidéo multimodale sur Amux API : texte-vers-vidéo et image-vers-vidéo, qualité cinéma jusqu'à 1080p, compatible avec les endpoints OpenAI et Volcengine Ark V3.",
  },

  vi: {
    'Seedance 2.0 采用统一的多模态音视频联合生成架构，支持文本、图像、音频与视频输入，具备业界领先的多模态内容参考与编辑能力。':
      'Seedance 2.0 sử dụng kiến trúc tạo sinh âm thanh - video đa phương thức hợp nhất, hỗ trợ đầu vào văn bản, hình ảnh, âm thanh và video, với khả năng tham chiếu và chỉnh sửa đa phương thức hàng đầu ngành.',
    '立即体验': 'Trải nghiệm ngay',
    '获取 API 文档': 'Tài liệu API',
    '在 Amux API 上体验': 'Trải nghiệm trên Amux API',
    '精准控制，生成连贯的电影级 AI 视频': 'Kiểm soát chính xác, tạo video AI điện ảnh mạch lạc',
    '通过精准控制生成连贯的电影级 AI 视频（含逼真人像），完美适用于营销推广、应用开发及专业制作工作流；使用 Seedance 2.0 Fast，可实现更快的生成速度、更低的成本以及大规模快速迭代。':
      'Tạo video AI điện ảnh mạch lạc (bao gồm nhân vật chân thực) với khả năng kiểm soát chính xác — hoàn hảo cho tiếp thị, phát triển ứng dụng và quy trình sản xuất chuyên nghiệp. Với Seedance 2.0 Fast, bạn có tốc độ tạo nhanh hơn, chi phí thấp hơn và lặp nhanh ở quy mô lớn.',
    '效果展示': 'Trình diễn',
    '为每个场景而生的专业视频': 'Video chuyên nghiệp cho mọi nhu cầu',
    '无论是拍摄短片、剪辑音乐视频，还是规模化产出广告内容，Seedance 2.0 都能胜任。':
      'Dù bạn quay phim ngắn, dựng video ca nhạc hay sản xuất nội dung quảng cáo quy mô lớn — Seedance 2.0 đều đáp ứng.',
    '短片与电影叙事。': 'Phim ngắn & kể chuyện điện ảnh.',
    '多镜头叙事，人物形象一致，电影级摄影机拍摄，原生音频同步。':
      'Tự sự nhiều cảnh quay với nhân vật nhất quán, quay phim điện ảnh và đồng bộ âm thanh gốc.',
    '电影级动态运镜。': 'Chuyển động máy quay điện ảnh.',
    '复刻参考视频中的跟踪、环绕与快速转场，画面运动流畅清晰。':
      'Tái hiện bám theo, xoay quanh và chuyển cảnh nhanh từ video tham chiếu với chuyển động mượt mà, sắc nét.',
    '逼真物理与动作连贯。': 'Vật lý chân thực & chuyển động mạch lạc.',
    '密集打斗、碰撞与子弹时间下，时序、重量感与动量保持一致。':
      'Trong các pha đánh nhau dồn dập, va chạm và bullet-time, thời điểm, trọng lượng và quán tính vẫn nhất quán.',
    '活动推广视频。': 'Video cho chiến dịch.',
    '一致品牌形象与强叙事性的宣传视频，无需制作团队。':
      'Video quảng bá với nhận diện thương hiệu nhất quán và tính tự sự mạnh mẽ — không cần đội ngũ sản xuất.',
    '高影响力视频广告。': 'Quảng cáo video tác động cao.',
    '由产品照片生成精美广告，动态演示与多种变体，锁定每一帧的品牌一致性。':
      'Tạo quảng cáo tinh tế từ ảnh sản phẩm: demo động, nhiều biến thể và sự nhất quán thương hiệu trong từng khung hình.',
    '音频节奏引导。': 'Dẫn dắt theo nhịp âm thanh.',
    '以音乐为节奏参考，画面动作与剪辑对齐节拍情绪，轻松做卡点视频。':
      'Dùng một bản nhạc làm tham chiếu nhịp điệu — căn chỉnh chuyển động và dựng phim theo nhịp và cảm xúc để dễ dàng tạo video bắt nhịp.',
    '悬浮播放声音': 'Di chuột để phát âm thanh',
    '快速接入': 'Bắt đầu nhanh',
    '两步生成你的第一条视频': 'Tạo video đầu tiên trong hai bước',
    '同时兼容 OpenAI 风格与火山方舟 V3 官方协议端点，统一封装为异步任务接口，提交即走、轮询取回。':
      'Tương thích cả phong cách OpenAI lẫn endpoint giao thức chính thức Volcengine Ark V3, hợp nhất thành API tác vụ bất đồng bộ — gửi rồi truy vấn kết quả.',
    '火山 V3': 'Volcengine V3',
    '支持火山方舟 V3 官方协议端点：只需把 Base URL 换成本站地址，即可将现有火山 SDK / 客户端无缝迁移接入。':
      'Hỗ trợ endpoint giao thức chính thức Volcengine Ark V3: chỉ cần đổi Base URL sang địa chỉ của trang, bạn có thể chuyển SDK / client Volcengine hiện có một cách liền mạch.',
    '提示：将 model 换成 doubao-seedance-2.0-fast 即可使用极速版。':
      'Mẹo: đổi model thành doubao-seedance-2.0-fast để dùng bản tốc độ cao.',
    '① 提交生成任务': '① Gửi tác vụ tạo video',
    '② 轮询任务结果': '② Truy vấn kết quả tác vụ',
    '常见问题': 'Câu hỏi thường gặp',
    '还有什么疑问吗？': 'Vẫn còn thắc mắc?',
    '我们整理了最常被问到的问题。': 'Chúng tôi đã giải đáp những câu hỏi thường gặp nhất.',
    'Seedance 2.0 是什么？': 'Seedance 2.0 là gì?',
    'Seedance 2.0 是字节跳动推出的多模态 AI 视频生成模型，支持文本、图像、音频与视频等多模态参考输入，能生成具备多镜头一致性与原生音频的电影级连贯视频，并支持逼真人像。':
      'Seedance 2.0 là mô hình tạo video AI đa phương thức của ByteDance. Nó nhận văn bản, hình ảnh, âm thanh và video làm tham chiếu, tạo ra video điện ảnh mạch lạc với sự nhất quán nhiều cảnh quay và âm thanh gốc, bao gồm cả nhân vật chân thực.',
    '标准模式和极速模式有什么区别？': 'Chế độ Tiêu chuẩn và Tốc độ khác nhau thế nào?',
    '标准模式（doubao-seedance-2.0）面向高质量成片，支持复杂运动与多镜头生成，最高 1080p，适合专业制作；极速模式（doubao-seedance-2.0-fast）更快更省、固定 720p，适合提示词测试、批量生成与快速迭代。两者共用同一套接口，切换 model 即可。':
      'Chế độ Tiêu chuẩn (doubao-seedance-2.0) hướng đến chất lượng cao, hỗ trợ chuyển động phức tạp và tạo nhiều cảnh quay lên tới 1080p, lý tưởng cho sản xuất chuyên nghiệp. Chế độ Tốc độ (doubao-seedance-2.0-fast) nhanh hơn và tiết kiệm hơn, cố định 720p, lý tưởng để thử nghiệm prompt, tạo hàng loạt và lặp nhanh. Cả hai dùng chung một API — chỉ cần đổi model.',
    '支持哪些输入与生成方式？': 'Hỗ trợ những đầu vào và cách tạo nào?',
    '支持文生视频与图生视频，可用文本、图像、音频、视频等多模态素材作为参考；分辨率提供 720p / 1080p（极速版固定 720p）。':
      'Hỗ trợ tạo video từ văn bản và từ hình ảnh, có thể dùng văn bản, hình ảnh, âm thanh, video làm tham chiếu đa phương thức; độ phân giải 720p / 1080p (bản Tốc độ cố định 720p).',
    '可以用它创作什么？': 'Có thể tạo những gì?',
    '短片与电影叙事、动作与视觉特效、活动推广视频、高影响力视频广告、音乐卡点 MV 等，覆盖营销推广、应用开发与专业制作工作流。':
      'Phim ngắn và tự sự điện ảnh, clip hành động và VFX, video chiến dịch, quảng cáo video tác động cao, video ca nhạc bắt nhịp, và hơn thế — phục vụ tiếp thị, phát triển ứng dụng và sản xuất chuyên nghiệp.',
    '如何接入调用？': 'Tích hợp như thế nào?',
    '提供 OpenAI 风格（/v1/video/generations）与火山方舟 V3 官方协议端点（/api/v3/contents/generations/tasks）两种方式，均为异步任务：提交后轮询取回结果。火山 V3 只需替换 Base URL 即可迁移现有客户端。':
      'Cung cấp hai cách: phong cách OpenAI (/v1/video/generations) và endpoint giao thức chính thức Volcengine Ark V3 (/api/v3/contents/generations/tasks), đều là tác vụ bất đồng bộ: gửi rồi truy vấn kết quả. Với Volcengine V3, chỉ cần thay Base URL để chuyển client hiện có.',
    '价格与渠道是怎样的？': 'Giá và kênh như thế nào?',
    'premium/doubao 渠道享官方 8 折，低至约 $0.12 / 秒，性价比首选（该渠道真人解限不能 100% 成功）；商业应用如需稳定真人解限，可使用 premium/doubao_video_max 渠道，价格为官方的 1.2 倍。':
      'Kênh premium/doubao được giảm 20% chính thức, thấp tới ~$0,12/giây, đáng giá nhất (việc mở khóa nhân vật chân thực trên kênh này không đảm bảo 100%). Với ứng dụng thương mại cần mở khóa ổn định, hãy dùng kênh premium/doubao_video_max với giá gấp 1,2 lần giá chính thức.',
    '使用 premium 渠道有什么要求？': 'Dùng kênh premium có yêu cầu gì?',
    'premium 渠道模型需累计充值满 $20 后解锁使用。':
      'Mô hình kênh premium được mở khóa sau khi nạp tích lũy đủ $20.',
    '需要视频剪辑经验吗？': 'Có cần kinh nghiệm dựng video không?',
    '不需要。写一句提示词或上传参考素材即可生成；进阶用户还能进一步控制运镜、转场与时长等，获得更深度的创作掌控。':
      'Không. Viết một prompt hoặc tải lên tham chiếu, mô hình lo phần còn lại; người dùng nâng cao có thể kiểm soát thêm chuyển động máy quay, chuyển cảnh và thời lượng để sáng tạo sâu hơn.',
    '即刻开始': 'Bắt đầu ngay',
    '用 Seedance 2.0 开始你的创作': 'Bắt đầu sáng tạo với Seedance 2.0',
    'Seedance 2.0 由 premium 渠道提供，累计充值满 $20 即可解锁使用，随后几分钟内即可生成你的第一条 AI 视频。':
      'Seedance 2.0 được cung cấp qua kênh premium; mở khóa bằng cách nạp tích lũy đủ $20, rồi tạo video AI đầu tiên trong vài phút.',
    '立即生成视频': 'Tạo video ngay',
    '查看模型与价格': 'Xem mô hình & giá',
    [SEEDANCE_SEO_TITLE]: 'Seedance 2.0 for Amux — API tạo video AI đa phương thức',
    [SEEDANCE_SEO_DESC]:
      'Dùng Seedance 2.0 (và Fast) để tạo video đa phương thức trên Amux API: văn bản-thành-video và hình ảnh-thành-video, chất lượng điện ảnh đến 1080p, tương thích endpoint OpenAI và Volcengine Ark V3.',
  },
};
