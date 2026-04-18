package constant

// Modality 表示一个模型的能力类别。用于后台分类、模型广场展示、
// Playground 自适应 UI 等场景。相同基座模型的不同挂牌（例如 qwen-max vs
// qwen-vl-max）需要分别打标。
const (
	ModalityText       = "text"       // 纯文本对话
	ModalityMultimodal = "multimodal" // 视觉语言模型（图/PDF 输入 + 文本输出）
	ModalityImage      = "image"      // 文本生图
	ModalityVideo      = "video"      // 文本生视频
	ModalityAudio      = "audio"      // TTS / STT / 语音对话
	ModalityEmbedding  = "embedding"  // 文本向量嵌入
	ModalityRerank     = "rerank"     // 重排序
)

// ValidModalities 返回所有受支持的 modality 枚举。空字符串将在写入时由数据库
// 默认值 'text' 兜底，但显式提交时必须在此列表里，避免脏数据。
var ValidModalities = map[string]bool{
	ModalityText:       true,
	ModalityMultimodal: true,
	ModalityImage:      true,
	ModalityVideo:      true,
	ModalityAudio:      true,
	ModalityEmbedding:  true,
	ModalityRerank:     true,
}

// ModalityList 保持顺序，供前端下拉渲染使用。
var ModalityList = []string{
	ModalityText,
	ModalityMultimodal,
	ModalityImage,
	ModalityVideo,
	ModalityAudio,
	ModalityEmbedding,
	ModalityRerank,
}

// IsValidModality 校验入参，空字符串视为合法（数据库层有默认值）。
func IsValidModality(m string) bool {
	if m == "" {
		return true
	}
	return ValidModalities[m]
}
