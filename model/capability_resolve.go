package model

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// ResolveCapabilities 返回某个模型最终生效的"输入模态"与"输出模态"集合。
//
// 解析优先级：
//  1. meta.InputModalities / meta.OutputModalities 显式非空 -> 直接使用
//     （只保留白名单内的合法值并去重）。
//  2. 否则根据 meta.Modality 给出基础集合，再用倍率字段（image_ratio /
//     audio_ratio / audio_completion_ratio）和 endpoints 信号补充。
//
// 返回值始终是按官方枚举顺序排列、去重后的字符串切片，调用方可直接 strings.Join
// 后写入 API 响应。
func ResolveCapabilities(meta *Model) (input []string, output []string) {
	if meta == nil {
		return []string{}, []string{}
	}

	explicitIn := parseExplicitCapabilities(meta.InputModalities, constant.ValidInputCapabilities)
	explicitOut := parseExplicitCapabilities(meta.OutputModalities, constant.ValidOutputCapabilities)

	if len(explicitIn) == 0 {
		explicitIn = inferInputCapabilities(meta)
	}
	if len(explicitOut) == 0 {
		explicitOut = inferOutputCapabilities(meta)
	}

	return orderCapabilities(explicitIn, constant.InputCapabilityList),
		orderCapabilities(explicitOut, constant.OutputCapabilityList)
}

// parseExplicitCapabilities 把逗号分隔字符串解析为合法值集合。
func parseExplicitCapabilities(raw string, allowed map[string]bool) map[string]bool {
	set := make(map[string]bool)
	if strings.TrimSpace(raw) == "" {
		return set
	}
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v != "" && allowed[v] {
			set[v] = true
		}
	}
	return set
}

// orderCapabilities 按照官方顺序输出，确保返回结果稳定。
func orderCapabilities(set map[string]bool, order []string) []string {
	out := make([]string, 0, len(set))
	for _, k := range order {
		if set[k] {
			out = append(out, k)
		}
	}
	return out
}

// effectiveModality 把 ResolveModality 与内置模型名 pattern 串起来，用作能力推断的输入。
// 决策顺序：
//  1. meta.Modality 显式非空 -> 直接用
//  2. 从 meta.Endpoints 推断（image/video/embedding/rerank 这种端点能推出来）
//  3. model_setting 的内置模型名正则（gpt-4o / claude-3-* / gemini-*-vision 等会被识别成 multimodal）
//  4. 兜底 text
func effectiveModality(meta *Model) string {
	if v := strings.TrimSpace(meta.Modality); v != "" {
		return v
	}
	if inferred := InferModalityFromEndpoints(meta.Endpoints); inferred != "" {
		return inferred
	}
	if v := model_setting.MatchModalityByPatterns(meta.ModelName); v != "" {
		return v
	}
	return constant.ModalityText
}

// inferInputCapabilities 推断输入模态。
func inferInputCapabilities(meta *Model) map[string]bool {
	set := map[string]bool{
		// 任何模型都至少接受 text 输入（embedding / rerank 也接受 text 作为输入）。
		constant.CapabilityText: true,
	}

	// Modality 信号（使用 effectiveModality 以支持 pattern 推断）
	switch effectiveModality(meta) {
	case constant.ModalityMultimodal:
		// 多模态默认包含图片；具体音频再由倍率字段进一步增强。
		set[constant.CapabilityImage] = true
	case constant.ModalityImage:
		// 图片生成模型普遍支持图生图 / 编辑（dall-e edit / flux 控制图 /
		// gpt-image 参考图等），默认带上 image 输入。
		set[constant.CapabilityImage] = true
	case constant.ModalityVideo:
		// 视频生成模型常见输入：text-to-video / image-to-video / video-to-video
		// 以及部分音频驱动场景（talking head / lip-sync），默认全开。
		set[constant.CapabilityImage] = true
		set[constant.CapabilityVideo] = true
		set[constant.CapabilityAudio] = true
	case constant.ModalityAudio:
		// TTS / STT / 语音对话三种都涉及 audio I/O，默认双向标 audio，
		// 管理员可在编辑表单中精确指定。
		set[constant.CapabilityAudio] = true
	}

	// 倍率信号：单独配置过 image_ratio / audio_ratio 表明模型计费上明确支持该输入。
	name := meta.ModelName
	if _, ok := ratio_setting.GetImageRatio(name); ok {
		set[constant.CapabilityImage] = true
	}
	if ratio_setting.ContainsAudioRatio(name) {
		set[constant.CapabilityAudio] = true
	}

	return set
}

// inferOutputCapabilities 推断输出模态。
func inferOutputCapabilities(meta *Model) map[string]bool {
	set := make(map[string]bool)

	// Modality 信号（使用 effectiveModality 以支持 pattern 推断）
	switch effectiveModality(meta) {
	case constant.ModalityText, constant.ModalityMultimodal:
		set[constant.CapabilityText] = true
	case constant.ModalityImage:
		set[constant.CapabilityImage] = true
	case constant.ModalityVideo:
		set[constant.CapabilityVideo] = true
	case constant.ModalityAudio:
		// 双向标 audio，再加 text（语音对话场景常见）。
		set[constant.CapabilityAudio] = true
		set[constant.CapabilityText] = true
	case constant.ModalityEmbedding:
		set[constant.CapabilityEmbedding] = true
	case constant.ModalityRerank:
		set[constant.CapabilityRerank] = true
	}

	// 倍率信号：audio_completion_ratio 明确表示模型有 audio 输出计费，
	// 用作 modality 之外的补强（如 audio 模型的纯 TTS 也保留 audio 输出）。
	if ratio_setting.ContainsAudioCompletionRatio(meta.ModelName) {
		set[constant.CapabilityAudio] = true
	}

	// 兜底：完全无信号时，至少有 text 输出。
	// 注意：effectiveModality 已经做过端点推断（image-generation/openai-video 等），
	// 这里不再二次扫端点，避免视频/图片/向量模型被附带的 chat 端点误加 text。
	if len(set) == 0 {
		set[constant.CapabilityText] = true
	}

	return set
}

