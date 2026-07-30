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

// 令牌连通性测试的模态 → 端点分发表。
//
// 和操练场（/pg/* + UserAuth）不同，这里一律走对外的 /v1/* + Bearer sk-xxx，
// 目的就是让请求完整经过 TokenAuth → Distribute → 计费：令牌禁用/过期/额度
// 耗尽、model_limits 白名单、allow_ips、分组链 fallback 全都真实生效。用后端
// 身份代打（渠道测试那套）验不到这些，对用户没有意义。
//
// 注意 /v1/* 不解析 body 里的 group 字段（middleware/distributor.go 里那段只
// 对 /pg/ 前缀生效），分组完全由令牌自身的链决定 —— 所以测试 UI 不提供分组
// 选择器，这是有意为之。

import { isSttModel } from '../components/playground/messageModality';

// 模态无法自动测试时的原因标记（UI 据此禁用行内测试按钮并给出说明）。
export const UNTESTABLE_STT = 'stt';

// 每个 profile：
//   path     请求端点
//   body     (model) => 请求体
//   extract  (json) => 给用户看的一句话结论
//   stream   是否支持流式开关
//   async    是否是「提交即算通过」的异步任务
export const TEST_PROFILES = {
  text: {
    path: '/v1/chat/completions',
    stream: true,
    body: (model) => ({
      model,
      messages: [{ role: 'user', content: 'hi' }],
      // 给到 64 而不是个位数：推理模型会先消耗推理 token，额度太小会在还没轮到
      // 可见输出时就被 finish_reason=length 截断，返回一个 content 为空的
      // 「成功」响应 —— 用户看到一片空白，误以为模型坏了。
      // 64 对普通模型仍是极小用量，对推理模型也未必够（见下面 extract 的说明）。
      max_tokens: 64,
    }),
    extract: (r) => {
      const choice = r?.choices?.[0];
      const content = choice?.message?.content || '';
      if (content) return content;

      // 空 content 但请求成功：连通性其实是通的，必须说清楚原因，
      // 否则「成功」配一个空摘要是最让人困惑的组合。
      const reasoning =
        r?.usage?.completion_tokens_details?.reasoning_tokens || 0;
      if (choice?.finish_reason === 'length') {
        return reasoning > 0
          ? `连通正常：${reasoning} 个 token 全部用于推理，未产生可见输出（推理模型的正常现象）`
          : '连通正常：输出被 max_tokens 截断，未返回可见内容';
      }
      return '连通正常：模型返回了空内容';
    },
  },
  image: {
    path: '/v1/images/generations',
    body: (model) => ({
      model,
      prompt: 'a red circle',
      n: 1,
    }),
    extract: (r) =>
      Array.isArray(r?.data) && r.data.length
        ? `返回 ${r.data.length} 张图`
        : '',
  },
  embedding: {
    path: '/v1/embeddings',
    body: (model) => ({ model, input: 'hi' }),
    extract: (r) => {
      const dim = r?.data?.[0]?.embedding?.length;
      return dim ? `向量维度 ${dim}` : '';
    },
  },
  rerank: {
    path: '/v1/rerank',
    body: (model) => ({
      model,
      query: 'hi',
      documents: ['hello', 'world'],
      top_n: 1,
    }),
    extract: (r) => {
      const n = r?.results?.length ?? r?.data?.length;
      return n ? `返回 ${n} 条排序结果` : '';
    },
  },
  // TTS：同步返回音频二进制，不是 JSON。useTokenTest 会按 content-type 走
  // blob 分支，extract 收到的是 { bytes }。
  audio: {
    path: '/v1/audio/speech',
    binary: true,
    body: (model) => ({ model, input: 'hi', voice: 'alloy' }),
    extract: (r) => (r?.bytes ? `返回音频 ${r.bytes} 字节` : ''),
  },
  // 视频生成是异步任务，跑完要几分钟，测试弹窗等不起。提交成功（拿到
  // task_id）就说明令牌有效、模型可用、计费链路通了，这已经是用户想知道的
  // 全部；不再轮询，结果里注明未等待完成。
  video: {
    path: '/v1/video/generations',
    async: true,
    body: (model) => ({ model, prompt: 'a red circle' }),
    extract: (r) => {
      const id = r?.task_id || r?.id;
      return id ? `任务已提交：${id}` : '任务已提交';
    },
  },
};

// multimodal 的输入形态和 text 完全一致（都是 messages），共用 profile。
TEST_PROFILES.multimodal = TEST_PROFILES.text;

/**
 * 解析一个模型该怎么测。
 *
 * audio 是唯一一个「一个 modality 对应两个不兼容端点」的模态：TTS 走
 * /v1/audio/speech + JSON，STT 走 /v1/audio/transcriptions + multipart 音频
 * 文件，而 constant/endpoint_type.go 里根本没有 audio 端点类型，后端无法区分。
 * 操练场靠用户动作分流（打字=TTS / 传文件=STT），自动化测试没有这个信号，
 * 且 STT 需要一个真实音频文件 —— 造不出来。所以这里复用操练场的启发式把 STT
 * 识别出来标成不可测，诚实禁用比发一个必然 400 的请求好。
 *
 * @returns {{profile: object|null, untestable: string|null}}
 */
export const resolveTestProfile = (modelName, modality) => {
  const key = modality || 'text';
  if (key === 'audio' && isSttModel(modelName)) {
    return { profile: null, untestable: UNTESTABLE_STT };
  }
  return {
    profile: TEST_PROFILES[key] || TEST_PROFILES.text,
    untestable: null,
  };
};

// 单引号包裹的 shell 字面量里，唯一有特殊含义的字符就是单引号本身。
// 收尾 → 插入转义的单引号 → 重开，是 POSIX sh 里的标准写法。
const shellSingleQuote = (value) => `'${String(value).replace(/'/g, `'\\''`)}'`;

/**
 * 生成与「点击测试」完全等价的 curl 命令，供用户复制到终端自行执行。
 *
 * 复用同一份 profile（path / body / stream），所以复制出来的命令和 UI 里
 * 实际发出的请求是同一个 —— 否则用户拿 curl 复现不出弹窗里看到的结果，
 * 这个按钮就是误导。
 *
 * 二进制响应（TTS）额外带 --output：不加的话音频字节会直接糊进终端。
 *
 * @param {string} model
 * @param {object} profile - resolveTestProfile 返回的 profile
 * @param {string} key - 完整 API key（含 sk- 前缀）
 * @param {string} baseUrl - 服务器地址
 * @param {boolean} isStream - 是否启用流式（仅对声明了 stream 的 profile 生效）
 * @returns {string}
 */
export const buildTestCurl = (model, profile, key, baseUrl, isStream) => {
  const body = profile.body(model);
  if (profile.stream && isStream) body.stream = true;

  const url = `${String(baseUrl).replace(/\/+$/, '')}${profile.path}`;
  const lines = [
    `curl ${shellSingleQuote(url)} \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -H ${shellSingleQuote(`Authorization: Bearer ${key}`)} \\`,
  ];
  if (profile.binary) {
    lines.push(`  --output ${shellSingleQuote(`${model}.mp3`)} \\`);
  }
  // JSON 里可能有单引号（prompt 之类），统一走同一套转义
  lines.push(`  -d ${shellSingleQuote(JSON.stringify(body))}`);
  return lines.join('\n');
};
