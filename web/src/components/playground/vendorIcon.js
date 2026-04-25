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

// 模型名 → 厂商规则。每条 [模式, 厂商展示名, lobehub 图标 key]。
// 和后端 model/pricing_default.go 的 defaultVendorRules / defaultVendorIcons
// 大致对齐，前端无需额外接口就能给模型挂厂商分组 + 图标。
// 匹配按子串包含 + 顺序优先：长前缀放前面避免误命中（dall-e 必须先于 e）。
const VENDOR_RULES = [
  ['dall-e', 'OpenAI', 'OpenAI'],
  ['whisper', 'OpenAI', 'OpenAI'],
  ['gpt', 'OpenAI', 'OpenAI'],
  ['o1', 'OpenAI', 'OpenAI'],
  ['o3', 'OpenAI', 'OpenAI'],
  ['o4', 'OpenAI', 'OpenAI'],
  ['sora', 'OpenAI', 'OpenAI'],
  ['claude', 'Anthropic', 'Claude.Color'],
  ['gemini', 'Google', 'Gemini.Color'],
  ['moonshot', 'Moonshot', 'Moonshot'],
  ['kimi', 'Moonshot', 'Moonshot'],
  ['chatglm', '智谱', 'Zhipu.Color'],
  ['glm-', '智谱', 'Zhipu.Color'],
  ['qwen', '阿里巴巴', 'Qwen.Color'],
  ['qwq', '阿里巴巴', 'Qwen.Color'],
  ['deepseek', 'DeepSeek', 'DeepSeek.Color'],
  ['abab', 'MiniMax', 'Minimax.Color'],
  ['minimax', 'MiniMax', 'Minimax.Color'],
  ['ernie', '百度', 'Wenxin.Color'],
  ['spark', '讯飞', 'Spark.Color'],
  ['hunyuan', '腾讯', 'Hunyuan.Color'],
  ['command', 'Cohere', 'Cohere.Color'],
  ['@cf/', 'Cloudflare', 'Cloudflare.Color'],
  ['360', '360', 'Ai360.Color'],
  ['yi-', '零一万物', 'Yi.Color'],
  ['jina', 'Jina', 'Jina'],
  ['mistral', 'Mistral', 'Mistral.Color'],
  ['mixtral', 'Mistral', 'Mistral.Color'],
  ['ministral', 'Mistral', 'Mistral.Color'],
  ['grok', 'xAI', 'XAI'],
  ['llama', 'Meta', 'Ollama'],
  ['doubao', '字节跳动', 'Doubao.Color'],
  ['kling', '快手', 'Kling.Color'],
  ['jimeng', '即梦', 'Jimeng.Color'],
  ['vidu', 'Vidu', 'Vidu'],
  ['flux', 'Flux', 'Flux'],
  ['stable-diffusion', 'StabilityAI', 'StabilityAI'],
  ['sd-', 'StabilityAI', 'StabilityAI'],
  ['midjourney', 'Midjourney', 'Midjourney'],
  ['mj-', 'Midjourney', 'Midjourney'],
  ['runway', 'Runway', 'Runway'],
  ['suno', 'Suno', 'Suno'],
];

const FALLBACK = { name: 'Other', iconKey: 'Layers' };

export const inferVendorMeta = (modelName) => {
  if (!modelName) return FALLBACK;
  const lower = String(modelName).toLowerCase();
  for (const [pattern, name, iconKey] of VENDOR_RULES) {
    if (lower.includes(pattern)) return { name, iconKey };
  }
  return FALLBACK;
};

// 兼容旧调用：只要图标 key
export const inferVendorIconKey = (modelName) =>
  inferVendorMeta(modelName).iconKey;
