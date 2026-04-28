package model_setting

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
	// CustomModalityPatterns 让管理员在不改代码的前提下补充模型 → modality
	// 的识别规则。key 是 modality（image/video/audio/embedding/rerank/
	// multimodal/text），value 是该类别的匹配模式数组。
	// 模式语法（与 common.ImageGenerationModels 保持一致）：
	//   - 普通字符串：不区分大小写 contains 匹配
	//   - "prefix:xxx"：不区分大小写前缀匹配
	// 与底层硬编码（common.ImageGenerationModels 等）取 **并集**。
	CustomModalityPatterns map[string][]string `json:"custom_modality_patterns"`
}

// DefaultCustomModalityPatterns 出厂默认的模型分类规则，覆盖主流厂商的常
// 见命名。优先级（外层由 modalityCheckOrder 控制）：
//
//	image > video > rerank > embedding > audio > multimodal > text
//
// 因此如 `gemini-2.5-flash-image` 会先被 image 层的 "flash-image" 命中，
// 不会被 multimodal 层的 "prefix:gemini-" 覆盖。
//
// 管理员在后台保存的值会 **完全替换** 本默认；如果只想增删几条，建议先
// 点 UI 上的"填入内置默认"再修改。
var DefaultCustomModalityPatterns = map[string][]string{
	// 视频（优先于图片判断，防止 prefix:jimeng- 吃掉 jimeng-video）
	"video": {
		// 国际
		"prefix:sora-",     // sora-2, sora-2-pro, sora-2-i2v
		"prefix:veo-",      // veo-2.0-*, veo-3.0-*, veo-3.1-*
		"prefix:gen4",      // gen4_turbo
		"prefix:gen-4",
		"prefix:gen3",
		"prefix:gen-3",
		"runway-gen",
		"prefix:ray-", // ray-2, ray-flash-2, ray-3（Luma）
		"prefix:ray2",
		"prefix:ray3",
		"dream-machine",
		"prefix:pika-",
		"pika-v",
		// 国产
		"prefix:seedance-", // seedance-1.0, seedance-2.0 及未来版本
		"doubao-seedance",  // doubao-seedance-2-0-pro 系
		"prefix:kling-",    // kling-v2.6 等
		"kling-v",
		"prefix:hailuo-", // hailuo-02, hailuo-2.3
		"minimax-video",
		"t2v-01",
		"i2v-01",
		"prefix:hunyuanvideo", // hunyuanvideo-1.5
		"hunyuan-video",
		"prefix:cogvideo", // cogvideox, cogvideo-pro
		"jimeng-video",
		"prefix:wan-video",
		"wanx-video",
		// wan 系视频：-t2v- / -i2v- 是"text-to-video / image-to-video"的标
		// 准后缀，一网打尽 wan2.1-t2v-*、wan2.6-i2v-* 等未来版本
		"-t2v-",
		"-i2v-",
	},

	// 图片生成
	"image": {
		// OpenAI
		"prefix:gpt-image-",
		"prefix:dall-e-",
		// Google：Gemini 图片模型 image 在名字**中间**（Nano Banana 家族），
		// 覆盖 gemini-2.5-flash-image、gemini-3.1-flash-image-preview、
		// gemini-3-pro-image-preview 等
		"flash-image",
		"pro-image",
		"prefix:imagen-", // imagen-3.0-*, imagen-4.0-*
		// Black Forest Labs
		"prefix:flux-",
		"prefix:flux.", // flux.2, flux.2-max, flux.2-klein
		// Stability AI
		"prefix:stable-diffusion-",
		"stable-image-",
		"prefix:sd3", // sd3-5-large, sd3.5-*
		// Midjourney
		"prefix:midjourney-",
		"mj-v",
		// Ideogram
		"prefix:ideogram-",
		// 国产
		"prefix:seedream-", // Seedream 3.0/4.0/4.5 及未来版本
		"doubao-seedream",
		"prefix:cogview-", // CogView-3 / 4
		"prefix:jimeng-",  // jimeng-3 等；jimeng-video 已在 video 层被先命中
		"prefix:wanx-",    // wanx-v1（万相 v1 系）
		"prefix:hunyuan-image",
		"prefix:qwen-image",
		// MiniMax：contains 匹配（case-insensitive）同时覆盖上游名 image-01 / image-01-live
		// 与对外名 MiniMax-Image-01 / MiniMax-Image-01-Live
		"image-01",
	},

	// 音频（优先于 multimodal，防止 gpt-4o-audio 被 prefix:gpt-4o 吃掉）
	"audio": {
		// OpenAI
		"prefix:whisper-",
		"prefix:tts-",
		"prefix:gpt-4o-tts",
		"gpt-4o-transcribe",
		"gpt-4o-mini-transcribe",
		"gpt-4o-audio",
		"gpt-4o-mini-audio",
		"gpt-4o-realtime",
		"gpt-4o-mini-realtime",
		// ElevenLabs
		"prefix:eleven_",
		"elevenlabs-",
		// 国产
		"qwen-audio",
		"qwen2-audio",
		"qwen2.5-omni", // omni 系含音频能力
		"qwen3-omni",
		"doubao-tts",
		"doubao-seed-tts",
		"doubao-asr",
		"prefix:cosyvoice",
		"prefix:sensevoice",
		"prefix:paraformer",
		"step-tts",
		"step-asr",
		"minimax-speech",
		"speech-01",
	},

	// 重排（优先于 embedding，防止 bge-reranker 被 prefix:bge- 吃掉）
	"rerank": {
		// 国际
		"prefix:rerank-", // rerank-v3.5, rerank-english-v3.0
		"cohere.rerank-",
		"jina-reranker-",       // contains，避免误伤 jina-embeddings-
		"prefix:mxbai-rerank-", // mxbai-rerank-xsmall/base/large
		// 国产
		"bge-reranker-", // bge-reranker-v2-m3, bge-reranker-v2-gemma
		"prefix:qwen3-reranker",
		"gte-rerank",
		"prefix:tao-8k-rerank",
	},

	// 向量嵌入
	"embedding": {
		// 国际
		"prefix:text-embedding-", // text-embedding-3-small/large, text-embedding-ada-002
		"prefix:voyage-",         // voyage-3/3.5/4 系
		"prefix:embed-v",
		"prefix:embed-english-v",
		"prefix:embed-multilingual-v",
		"cohere.embed-",
		"prefix:jina-embeddings-",
		"prefix:jina-clip-",
		"prefix:nomic-embed-",
		"mistral-embed",
		// 国产
		"prefix:bge-", // bge-m3, bge-large-zh-v1.5 等；rerank 已先被命中
		"prefix:m3e-",
		"prefix:gte-",
		"prefix:qwen3-embedding",
		"text-embedding-v", // DashScope 自家 v1/v2/v3 命名
		"prefix:conan-embedding",
		"prefix:xiaobu-embedding",
		"prefix:yinka",
		"prefix:stella-",
	},

	// 多模态对话（视觉理解）
	"multimodal": {
		// OpenAI（audio 变体已先命中）
		"prefix:gpt-4o",
		"prefix:gpt-4.1",
		"prefix:gpt-4-vision",
		"prefix:gpt-5",
		// Anthropic
		"prefix:claude-3", // claude-3-*, claude-3.5-*, claude-3.7-*
		"prefix:claude-4", // claude-4-*, claude-4.5-*, claude-4.7-*
		"prefix:claude-opus-4",
		"prefix:claude-sonnet-4",
		"prefix:claude-haiku-4",
		// Google
		"prefix:gemini-1.5",
		"prefix:gemini-2",
		"prefix:gemini-3", // image 变体已先命中
		// Meta / Mistral
		"prefix:llama-3.2",
		"llama-3.2-vision",
		"prefix:llama-4",
		"prefix:pixtral",
		// 阿里
		"qwen-vl",
		"qwen2-vl",
		"qwen2.5-vl",
		"qwen3-vl",
		// 智谱
		"prefix:glm-4v",
		"prefix:glm-4.5v",
		"prefix:glm-4.6v",
		"prefix:glm-5v",
		// DeepSeek
		"prefix:deepseek-vl",
		// 月之暗面
		"kimi-vl",
		"prefix:kimi-k2",
		"prefix:moonshot-v1",
		// 字节豆包
		"doubao-vision",
		"doubao-seed-1-6-vision",
		"doubao-1-5-vision",
		"doubao-1-5-thinking-vision",
		// 其他
		"prefix:internvl",
		"prefix:minicpm-v",
		"prefix:minicpm-o",
		"prefix:yi-vl",
		"step-1v",
		"step-1o",
		"prefix:ernie-vl",
		"ernie-4-turbo-vl",
	},
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
	CustomModalityPatterns: DefaultCustomModalityPatterns,
}

// 全局实例
var globalSettings = defaultOpenaiSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
	// 把自定义 image 模式注入 common.IsImageGenerationModel；这样运行时 endpoint
	// 缓存（updatePricing）在判定"是否图片模型"时会同时用硬编码 + 管理员补充。
	common.RegisterImageModelOverride(MatchesCustomImagePatterns)
}

func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}

// matchPatterns 使用单条 pattern 列表尝试匹配模型名。支持三种语法：
//   - 普通字符串：不区分大小写 contains
//   - "prefix:xxx"：不区分大小写前缀
//   - "suffix:xxx"：不区分大小写后缀
func matchPatterns(modelName string, patterns []string) bool {
	if modelName == "" {
		return false
	}
	name := strings.ToLower(modelName)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pl := strings.ToLower(p)
		if strings.HasPrefix(pl, "prefix:") {
			if strings.HasPrefix(name, strings.TrimPrefix(pl, "prefix:")) {
				return true
			}
			continue
		}
		if strings.HasPrefix(pl, "suffix:") {
			if strings.HasSuffix(name, strings.TrimPrefix(pl, "suffix:")) {
				return true
			}
			continue
		}
		if strings.Contains(name, pl) {
			return true
		}
	}
	return false
}

// modalityCheckOrder 定义 pattern 匹配时的 modality 优先级（特化先于通用）。
// 决策依据：
//   - video 先于 image：防止 prefix:jimeng- 这类品牌家族前缀把 jimeng-video
//     误判为图片（jimeng-video 必须先被 video 命中）
//   - audio 先于 multimodal：gpt-4o-audio-preview 要识别为 audio 而不是把
//     prefix:gpt-4o 的多模态规则命中
//   - rerank 先于 embedding：bge-reranker-v2 要先走 rerank，不能被 prefix:bge-
//     的嵌入规则吃掉
var modalityCheckOrder = []string{
	"video",
	"image",
	"audio",
	"rerank",
	"embedding",
	"multimodal",
	"text",
}

// MatchModalityByPatterns 从 GlobalSettings.CustomModalityPatterns 匹配出
// 该模型对应的 modality。未命中返回空串。
func MatchModalityByPatterns(modelName string) string {
	if modelName == "" {
		return ""
	}
	patternsMap := globalSettings.CustomModalityPatterns
	if len(patternsMap) == 0 {
		return ""
	}
	for _, modality := range modalityCheckOrder {
		if matchPatterns(modelName, patternsMap[modality]) {
			return modality
		}
	}
	return ""
}

// MatchesCustomImagePatterns 供 common.IsImageGenerationModel 合并使用。
// 单独暴露 image 维度的匹配，避免跨包形成循环依赖（common → setting）。
func MatchesCustomImagePatterns(modelName string) bool {
	patternsMap := globalSettings.CustomModalityPatterns
	if len(patternsMap) == 0 {
		return false
	}
	return matchPatterns(modelName, patternsMap["image"])
}
