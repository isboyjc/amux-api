package common

import "strings"

var (
	// OpenAIResponseOnlyModels is a list of models that are only available for OpenAI responses.
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}
	ImageGenerationModels = []string{
		"dall-e-3",
		"dall-e-2",
		"gpt-image-1",
		"prefix:imagen-",
		"flux-",
		"flux.1-",
	}
	OpenAITextModels = []string{
		"gpt-",
		"o1",
		"o3",
		"o4",
		"chatgpt",
	}
)

func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

// imageModelOverrideFn 在运行时由 model_setting 包注入，用来把"管理员自定义
// 的 image 模式"合入匹配。这里用注入而不是 import，是因为 setting 包会反过来
// 依赖 common，直接 import 会形成循环。
var imageModelOverrideFn func(string) bool

// RegisterImageModelOverride 允许上层 setting 包注入自定义 image 模式匹配。
func RegisterImageModelOverride(fn func(string) bool) {
	imageModelOverrideFn = fn
}

func IsImageGenerationModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	for _, m := range ImageGenerationModels {
		if strings.Contains(lower, m) {
			return true
		}
		if strings.HasPrefix(m, "prefix:") && strings.HasPrefix(lower, strings.TrimPrefix(m, "prefix:")) {
			return true
		}
	}
	if imageModelOverrideFn != nil && imageModelOverrideFn(modelName) {
		return true
	}
	return false
}

func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}
