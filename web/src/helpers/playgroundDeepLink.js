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

// 操练场深链解析。链接形如：
//   /console/playground?model=<MODEL>&group=<GROUP>&prompt=<URL_ENCODED?>
//                       &<param1>=<v1>&<param2>=<v2>...
//
// 设计要点：
//   - model + group 都是必填；缺任一项视作"未启用深链"，按普通进入处理，
//     不报错也不抹 query。这样 ?model=xxx 单独一条参数不会引发副作用
//   - prompt 可选；URL 编码后拼接，长度由 MAX_PROMPT_LEN 兜底
//   - **其余所有非保留键** 都视作模型参数候选，类型 coerce 由调用方
//     基于模型 schema（image/video）或 TEXT_PARAM_SCHEMA（text 模型固定字段）
//     在外面做。helper 这里只抓字符串、不做语义判断
//   - 不在前端找"最低折扣分组"——参数已经显式带了 group，前端不做猜测
//   - 校验只做格式检查：非空字符串 + 长度上限。模型/分组是否真的存在 / 用户
//     是否能用，由后续 handleNewChat 的 createSession 接口反馈，不前置拦截
//
// 调用约定：
//   const link = parsePlaygroundDeepLink(searchParams);
//   if (link) { applyAndClean(link); }
//   else      { /* 普通进入 */ }

const MAX_MODEL_LEN = 200;
const MAX_GROUP_LEN = 100;
const MAX_PROMPT_LEN = 4000;
// 单条 model 参数值的硬上限——防止 URL 里塞超长字符串撑爆 inputs state
const MAX_PARAM_VALUE_LEN = 2000;
// 一次最多接受多少 model 参数键；超过抛掉多余的（防 URL 滥用）
const MAX_PARAM_COUNT = 32;

// 保留键：这些不计入 model params。新增前要确认与可能的模型 schema 字段
// 不冲突——目前 model/group 是控制态，prompt 是 UnifiedInputBar 文本区，
// 都不会作为 schema knob 出现
const RESERVED_KEYS = ['model', 'group', 'prompt'];

/**
 * 文本模型的"已知参数 + 类型" 表。文本模型在本项目里没有 JSON schema
 * （schema 仅给 image/video 用），所以深链针对文本模型时按这张表 coerce。
 *
 * 这是与 DEFAULT_CONFIG.inputs（playground.constants.js）约定的字段一致；
 * 新增文本参数时同步这张表
 */
export const TEXT_PARAM_SCHEMA = {
  temperature: 'number',
  top_p: 'number',
  max_tokens: 'integer',
  frequency_penalty: 'number',
  presence_penalty: 'number',
  seed: 'integer',
  stream: 'boolean',
};

/**
 * 把 URL 字符串值按指定 type 转成 JS 类型；coerce 失败返回 undefined。
 *
 * 调用方拿到 undefined 应该 **跳过该项**——而不是把 NaN/null 写进 state，
 * 否则 Form.InputNumber 会报 console 错。
 *
 * 接受的 type：
 *   - 字符串："number" / "integer" / "boolean" / "string" / "array"
 *   - 对象：JSON Schema 节点，自动读 .type，array 时递归 .items
 *
 * @param {string} value 来自 URL 的原始字符串值
 * @param {string|object} typeOrDef 类型字符串或 schema 节点
 * @returns {*|undefined}
 */
export function coerceParam(value, typeOrDef) {
  if (value == null) return undefined;
  const def = typeof typeOrDef === 'object' ? typeOrDef : null;
  const type = typeof typeOrDef === 'string' ? typeOrDef : typeOrDef?.type;

  if (type === 'number') {
    const n = parseFloat(value);
    return Number.isFinite(n) ? n : undefined;
  }
  if (type === 'integer') {
    const n = parseInt(value, 10);
    return Number.isFinite(n) ? n : undefined;
  }
  if (type === 'boolean') {
    if (value === 'true' || value === '1') return true;
    if (value === 'false' || value === '0') return false;
    return undefined;
  }
  if (type === 'array') {
    // 数组以 "," 分隔（足够应付 enum string 列表 / 数字列表）
    const itemDef = def?.items;
    const arr = String(value)
      .split(',')
      .map((v) => coerceParam(v.trim(), itemDef))
      .filter((v) => v !== undefined);
    return arr.length > 0 ? arr : undefined;
  }
  // string / enum / 未声明 type：原样返回（调用方做白名单 / enum 校验）
  // enum 约束：如果 schema 节点声明了 enum，这里同步过滤一次，避免把
  // ?quality=garbage 写进 state 让 Form.Select 报错
  if (def?.enum && Array.isArray(def.enum)) {
    return def.enum.includes(value) ? value : undefined;
  }
  return value;
}

/**
 * 从 URLSearchParams 解析深链参数；不合法时返回 null。
 *
 * @param {URLSearchParams} searchParams
 * @returns {{model: string, group: string, prompt: string, params: Record<string,string>} | null}
 */
export function parsePlaygroundDeepLink(searchParams) {
  if (!searchParams || typeof searchParams.get !== 'function') return null;

  const model = (searchParams.get('model') || '').trim();
  const group = (searchParams.get('group') || '').trim();
  // prompt 不 trim：用户可能有意保留首尾换行（少见但不要剥夺）
  const prompt = searchParams.get('prompt') || '';

  if (!model || !group) return null;

  // 防御：超长可能是恶意构造，截断而不是直接拒绝以保证 UX 不被打断
  if (model.length > MAX_MODEL_LEN || group.length > MAX_GROUP_LEN) {
    return null;
  }

  // 抓 model 参数：所有非保留键，截断超长值，限制总条目数
  const params = {};
  let count = 0;
  searchParams.forEach((value, key) => {
    if (RESERVED_KEYS.includes(key)) return;
    if (count >= MAX_PARAM_COUNT) return;
    if (typeof value !== 'string') return;
    params[key] = value.slice(0, MAX_PARAM_VALUE_LEN);
    count += 1;
  });

  return {
    model,
    group,
    prompt: prompt.slice(0, MAX_PROMPT_LEN),
    params,
  };
}

/**
 * 反向构造深链 URL，给后台 / 营销页面拼链接用。
 *
 * 除了固定的 model/group/prompt，还接受 params（任意键值），自动 URL 编码
 * 并 ?key=value 拼接。值可以是 string/number/boolean/array——array 用 ","
 * join；boolean / number 自动 toString。
 *
 * @param {{model:string, group:string, prompt?:string, params?:object, base?:string}} opts
 * @returns {string}
 */
export function buildPlaygroundDeepLink({
  model,
  group,
  prompt,
  params,
  base = '/console/playground',
}) {
  const search = new URLSearchParams();
  search.set('model', model);
  search.set('group', group);
  if (prompt) search.set('prompt', prompt);
  if (params && typeof params === 'object') {
    Object.entries(params).forEach(([k, v]) => {
      if (RESERVED_KEYS.includes(k)) return; // 不让 params 覆盖保留键
      if (v == null) return;
      const str = Array.isArray(v) ? v.join(',') : String(v);
      search.set(k, str);
    });
  }
  return `${base}?${search.toString()}`;
}
