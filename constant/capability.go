package constant

// 模型能力（输入/输出模态）。区别于 Modality 单值字段（用于
// Playground UI 切换），这里是更细粒度的多值集合，支持一个模型同时声明
// 多种输入/输出模态（典型场景：多模态 LLM 接受 文本+图片+文件）。
//
// 字段在 model.Model 上以逗号分隔字符串存储；空值表示"未显式配置"，
// 由 model.ResolveCapabilities 根据 Modality + 倍率字段自动推断。
const (
	CapabilityText      = "text"
	CapabilityImage     = "image"
	CapabilityAudio     = "audio"
	CapabilityVideo     = "video"
	CapabilityFile      = "file"
	CapabilityEmbedding = "embedding"
	CapabilityRerank    = "rerank"
)

// ValidInputCapabilities 允许作为"输入模态"的取值集合。
var ValidInputCapabilities = map[string]bool{
	CapabilityText:  true,
	CapabilityImage: true,
	CapabilityAudio: true,
	CapabilityVideo: true,
	CapabilityFile:  true,
}

// ValidOutputCapabilities 允许作为"输出模态"的取值集合。
var ValidOutputCapabilities = map[string]bool{
	CapabilityText:      true,
	CapabilityImage:     true,
	CapabilityAudio:     true,
	CapabilityVideo:     true,
	CapabilityEmbedding: true,
	CapabilityRerank:    true,
}

// InputCapabilityList 保持顺序，供前端下拉渲染使用。
var InputCapabilityList = []string{
	CapabilityText,
	CapabilityImage,
	CapabilityAudio,
	CapabilityVideo,
	CapabilityFile,
}

// OutputCapabilityList 保持顺序，供前端下拉渲染使用。
var OutputCapabilityList = []string{
	CapabilityText,
	CapabilityImage,
	CapabilityAudio,
	CapabilityVideo,
	CapabilityEmbedding,
	CapabilityRerank,
}

// IsValidInputCapability 校验单个输入模态值。
func IsValidInputCapability(v string) bool {
	return ValidInputCapabilities[v]
}

// IsValidOutputCapability 校验单个输出模态值。
func IsValidOutputCapability(v string) bool {
	return ValidOutputCapabilities[v]
}
