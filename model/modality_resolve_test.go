package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func TestInferModalityFromEndpoints(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"invalid json", "{oops", ""},
		{"chat only (object)", `{"openai":{"path":"/v1/chat/completions"}}`, constant.ModalityText},
		{"chat only (array)", `["openai","anthropic"]`, constant.ModalityText},
		{"image takes precedence over chat",
			`{"openai":{"path":"/v1/chat/completions"},"image-generation":{"path":"/v1/images/generations"}}`,
			constant.ModalityImage},
		{"rerank", `["jina-rerank"]`, constant.ModalityRerank},
		{"embedding", `["embeddings"]`, constant.ModalityEmbedding},
		{"video", `["openai-video"]`, constant.ModalityVideo},
		{"gemini chat", `["gemini"]`, constant.ModalityText},
		{"image in array", `["openai","image-generation"]`, constant.ModalityImage},
		{"unknown only", `["something-weird"]`, ""},
	}
	for _, c := range cases {
		got := InferModalityFromEndpoints(c.raw)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveModality_Priority(t *testing.T) {
	// 显式字段胜过 endpoints 推断
	m := &Model{
		Modality:  constant.ModalityMultimodal,
		Endpoints: `{"image-generation":{"path":"/v1/images/generations"}}`,
	}
	if got := ResolveModality(m); got != constant.ModalityMultimodal {
		t.Errorf("explicit modality should win: got %q", got)
	}
}

func TestResolveModality_FallbackToEndpoints(t *testing.T) {
	// 显式字段空，走 endpoints 推断
	m := &Model{
		Endpoints: `["image-generation","openai"]`,
	}
	if got := ResolveModality(m); got != constant.ModalityImage {
		t.Errorf("should infer image from endpoints, got %q", got)
	}
}

func TestResolveModality_EmptyEverywhere(t *testing.T) {
	m := &Model{}
	if got := ResolveModality(m); got != "" {
		t.Errorf("no signal should return empty, got %q", got)
	}
}

func TestResolveModality_NilSafe(t *testing.T) {
	if got := ResolveModality(nil); got != "" {
		t.Errorf("nil model should return empty, got %q", got)
	}
}

// --- Tiered name-level resolution ---

func TestResolveModalityForName_ExactExplicitWins(t *testing.T) {
	exact := &Model{Modality: constant.ModalityMultimodal}
	rule := &Model{Modality: constant.ModalityImage}
	if got := ResolveModalityForName("x", exact, rule); got != constant.ModalityMultimodal {
		t.Errorf("exact explicit should win, got %q", got)
	}
}

func TestResolveModalityForName_ExactEndpointsBeatRuleExplicit(t *testing.T) {
	// 用户的 Gemini 场景：exact 记录有 image-generation endpoint（由 sync
	// official 填），rule 记录把 gemini 族统一标成 multimodal。image 应胜出。
	exact := &Model{Endpoints: `["image-generation","gemini"]`}
	rule := &Model{Modality: constant.ModalityMultimodal}
	if got := ResolveModalityForName("gemini-2.5-flash-image", exact, rule); got != constant.ModalityImage {
		t.Errorf("exact endpoints should beat rule explicit, got %q", got)
	}
}

func TestResolveModalityForName_RuleExplicitWhenNoExact(t *testing.T) {
	// 没有 exact 记录时，只能靠 rule 记录的显式 modality
	rule := &Model{Modality: constant.ModalityMultimodal}
	if got := ResolveModalityForName("gemini-2.5-pro", nil, rule); got != constant.ModalityMultimodal {
		t.Errorf("rule explicit should fire, got %q", got)
	}
}

func TestResolveModalityForName_AllEmpty(t *testing.T) {
	if got := ResolveModalityForName("x", nil, nil); got != "" {
		t.Errorf("no signal anywhere should return empty, got %q", got)
	}
}

// --- CustomModalityPatterns 集成测试 ---

func TestResolveModalityForName_PatternMatchBeatsRule(t *testing.T) {
	// 管理员在 GlobalSettings 里加了 "image" → ["-image"] 模式；
	// 对 foo-image 应胜过 rule 记录的 multimodal。
	settings := model_setting.GetGlobalSettings()
	originalPatterns := settings.CustomModalityPatterns
	settings.CustomModalityPatterns = map[string][]string{
		"image": {"-image"},
	}
	defer func() { settings.CustomModalityPatterns = originalPatterns }()

	rule := &Model{Modality: constant.ModalityMultimodal}
	if got := ResolveModalityForName("foo-image", nil, rule); got != constant.ModalityImage {
		t.Errorf("pattern should beat rule, got %q", got)
	}
}

func TestResolveModalityForName_PatternLosesToExact(t *testing.T) {
	// exact 记录的显式 modality 必须胜过 pattern。
	settings := model_setting.GetGlobalSettings()
	originalPatterns := settings.CustomModalityPatterns
	settings.CustomModalityPatterns = map[string][]string{
		"image": {"-image"},
	}
	defer func() { settings.CustomModalityPatterns = originalPatterns }()

	exact := &Model{Modality: constant.ModalityMultimodal}
	if got := ResolveModalityForName("foo-image", exact, nil); got != constant.ModalityMultimodal {
		t.Errorf("exact explicit should beat pattern, got %q", got)
	}
}

func TestResolveModalityForName_PatternPrefixSyntax(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalPatterns := settings.CustomModalityPatterns
	settings.CustomModalityPatterns = map[string][]string{
		"embedding": {"prefix:text-embedding-"},
	}
	defer func() { settings.CustomModalityPatterns = originalPatterns }()

	if got := ResolveModalityForName("text-embedding-3-large", nil, nil); got != constant.ModalityEmbedding {
		t.Errorf("prefix: syntax should match, got %q", got)
	}
	if got := ResolveModalityForName("random-text-embedding-x", nil, nil); got != "" {
		t.Errorf("prefix: must anchor at start, got %q", got)
	}
}

// 用出厂默认跑一遍常见模型，做兜底回归保护。
func TestResolveModalityForName_BuiltinDefaults(t *testing.T) {
	// 不 override，直接走出厂默认
	cases := []struct {
		modelName string
		want      string
	}{
		// ===== image =====
		// OpenAI
		{"gpt-image-1", constant.ModalityImage},
		{"gpt-image-1-mini", constant.ModalityImage},
		{"dall-e-3", constant.ModalityImage},
		{"dall-e-2", constant.ModalityImage},
		// Google Gemini 图片模型（image 在中间，覆盖 2.5/3/3.1 各代）
		{"gemini-2.5-flash-image", constant.ModalityImage},
		{"gemini-3-pro-image-preview", constant.ModalityImage},
		{"gemini-3.1-flash-image-preview", constant.ModalityImage},
		{"gemini-3.1-pro-image-preview", constant.ModalityImage},
		// Google Imagen
		{"imagen-3.0-generate-001", constant.ModalityImage},
		{"imagen-4.0-ultra-generate-001", constant.ModalityImage},
		// Flux
		{"flux-1.1-pro", constant.ModalityImage},
		{"flux.2-max", constant.ModalityImage},
		// Stability AI
		{"stable-diffusion-3.5-large", constant.ModalityImage},
		{"sd3.5-large", constant.ModalityImage},
		{"stable-image-ultra-v1:1", constant.ModalityImage},
		// Midjourney / Ideogram
		{"midjourney-v7", constant.ModalityImage},
		{"ideogram-v3", constant.ModalityImage},
		// 国产
		{"seedream-4.5", constant.ModalityImage},
		{"doubao-seedream-3-0-t2i-250415", constant.ModalityImage},
		{"cogview-4-250304", constant.ModalityImage},
		{"jimeng-3.0", constant.ModalityImage}, // 注意不是 jimeng-video
		{"wanx-v1", constant.ModalityImage},
		{"qwen-image", constant.ModalityImage},

		// ===== video =====
		{"sora-2", constant.ModalityVideo},
		{"sora-2-pro", constant.ModalityVideo},
		{"veo-3.1-generate-001", constant.ModalityVideo},
		{"gen4_turbo", constant.ModalityVideo},
		{"gen-4.5", constant.ModalityVideo},
		{"ray-3", constant.ModalityVideo},
		{"dream-machine-v1", constant.ModalityVideo},
		{"pika-2.5", constant.ModalityVideo},
		// 国产
		{"seedance-1.0", constant.ModalityVideo},
		{"seedance-2.0", constant.ModalityVideo},
		{"doubao-seedance-2-0-pro", constant.ModalityVideo},
		{"doubao-seedance-2-0-lite-250828", constant.ModalityVideo},
		{"kling-v2.6", constant.ModalityVideo},
		{"hailuo-2.3", constant.ModalityVideo},
		{"hunyuanvideo-1.5", constant.ModalityVideo},
		{"hunyuan-video-i2v", constant.ModalityVideo},
		{"cogvideox-5b", constant.ModalityVideo},
		{"jimeng-video-v1", constant.ModalityVideo}, // 验证不会被 jimeng- image 前缀吃掉
		{"wan2.6-i2v-a14b", constant.ModalityVideo},

		// ===== audio =====
		{"whisper-1", constant.ModalityAudio},
		{"tts-1-hd", constant.ModalityAudio},
		{"gpt-4o-audio-preview", constant.ModalityAudio}, // 防 prefix:gpt-4o 吞噬
		{"gpt-4o-mini-realtime-preview", constant.ModalityAudio},
		{"gpt-4o-transcribe", constant.ModalityAudio},
		{"eleven_turbo_v2_5", constant.ModalityAudio},
		{"qwen3-omni", constant.ModalityAudio},
		{"cosyvoice-v1", constant.ModalityAudio},
		{"speech-01-hd", constant.ModalityAudio},

		// ===== rerank =====
		{"rerank-v3.5", constant.ModalityRerank},
		{"bge-reranker-v2-m3", constant.ModalityRerank}, // 防 prefix:bge- 吞噬
		{"jina-reranker-v3", constant.ModalityRerank},
		{"mxbai-rerank-large-v2", constant.ModalityRerank},
		{"qwen3-reranker-4b", constant.ModalityRerank},

		// ===== embedding =====
		{"text-embedding-3-large", constant.ModalityEmbedding},
		{"bge-m3", constant.ModalityEmbedding},
		{"bge-large-zh-v1.5", constant.ModalityEmbedding},
		{"voyage-3-large", constant.ModalityEmbedding},
		{"embed-v4.0", constant.ModalityEmbedding},
		{"jina-embeddings-v4", constant.ModalityEmbedding},
		{"qwen3-embedding-4b", constant.ModalityEmbedding},

		// ===== multimodal =====
		{"gpt-4o", constant.ModalityMultimodal},
		{"gpt-4o-mini", constant.ModalityMultimodal},
		{"gpt-4.1", constant.ModalityMultimodal},
		{"claude-3-5-sonnet-20241022", constant.ModalityMultimodal},
		{"claude-sonnet-4-5", constant.ModalityMultimodal},
		{"claude-opus-4", constant.ModalityMultimodal},
		{"claude-haiku-4-5", constant.ModalityMultimodal},
		{"gemini-2.5-pro", constant.ModalityMultimodal},
		{"gemini-1.5-flash", constant.ModalityMultimodal},
		{"gemini-3-pro", constant.ModalityMultimodal},
		{"gemini-3.1-flash", constant.ModalityMultimodal}, // 不是 image，会落对话
		{"qwen-vl-max", constant.ModalityMultimodal},
		{"qwen2.5-vl-72b-instruct", constant.ModalityMultimodal},
		{"qwen3-vl-plus", constant.ModalityMultimodal},
		{"glm-4v-plus", constant.ModalityMultimodal},
		{"glm-4.5v", constant.ModalityMultimodal},
		{"glm-5v-turbo", constant.ModalityMultimodal},
		{"deepseek-vl2", constant.ModalityMultimodal},
		{"kimi-k2.5", constant.ModalityMultimodal},
		{"doubao-seed-1-6-vision", constant.ModalityMultimodal},
		{"doubao-1-5-thinking-vision-pro", constant.ModalityMultimodal},
		{"internvl3.5-78b", constant.ModalityMultimodal},
		{"minicpm-v-4.5", constant.ModalityMultimodal},
		{"minicpm-o-4.5", constant.ModalityMultimodal},
		{"llama-3.2-90b-vision-instruct", constant.ModalityMultimodal},
		{"pixtral-large", constant.ModalityMultimodal},
	}
	for _, c := range cases {
		got := ResolveModalityForName(c.modelName, nil, nil)
		if got != c.want {
			t.Errorf("builtin default for %s: got %q, want %q", c.modelName, got, c.want)
		}
	}
}

// 验证 suffix: 语法：既独立能用，也在默认里由 -image 后缀命中。
func TestMatchModalityByPatterns_SuffixSyntax(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.CustomModalityPatterns
	settings.CustomModalityPatterns = map[string][]string{
		"rerank": {"suffix:-rerank"},
	}
	defer func() { settings.CustomModalityPatterns = original }()

	if got := model_setting.MatchModalityByPatterns("foo-rerank"); got != constant.ModalityRerank {
		t.Errorf("suffix: should match trailing text, got %q", got)
	}
	// "foo-rerank-v2" 并不以 -rerank 结尾
	if got := model_setting.MatchModalityByPatterns("foo-rerank-v2"); got != "" {
		t.Errorf("suffix: must anchor at end, got %q", got)
	}
}

func TestMatchModalityByPatterns_Priority(t *testing.T) {
	// 管理员同时把一个模型写进 image 和 multimodal；image 应先命中。
	settings := model_setting.GetGlobalSettings()
	originalPatterns := settings.CustomModalityPatterns
	settings.CustomModalityPatterns = map[string][]string{
		"multimodal": {"vision"},
		"image":      {"vision"},
	}
	defer func() { settings.CustomModalityPatterns = originalPatterns }()

	if got := model_setting.MatchModalityByPatterns("some-vision-model"); got != constant.ModalityImage {
		t.Errorf("image priority over multimodal failed, got %q", got)
	}
}
