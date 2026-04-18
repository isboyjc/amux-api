package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// endpointModalityPriority 定义按 endpoint 推断 modality 的"特化优先级"。
// 同一条 Model 记录的 endpoints 可能同时包含 chat 和 image-generation，
// 这时更"特化"的信号（图片 / 重排 / 嵌入 / 视频 / 音频）胜出；
// 没有任何特化信号时才归到 text。
var endpointModalityPriority = []struct {
	endpoint constant.EndpointType
	modality string
}{
	{constant.EndpointTypeImageGeneration, constant.ModalityImage},
	{constant.EndpointTypeOpenAIVideo, constant.ModalityVideo},
	{constant.EndpointTypeJinaRerank, constant.ModalityRerank},
	{constant.EndpointTypeEmbeddings, constant.ModalityEmbedding},
}

// chatEndpoints 是被视为"文本对话"的 endpoint 集合；这些都推断为 text。
// 多模态（视觉）无法从 endpoint 区分（都走 chat），只能靠显式 modality 字段。
var chatEndpoints = map[constant.EndpointType]bool{
	constant.EndpointTypeOpenAI:                true,
	constant.EndpointTypeOpenAIResponse:        true,
	constant.EndpointTypeOpenAIResponseCompact: true,
	constant.EndpointTypeAnthropic:             true,
	constant.EndpointTypeGemini:                true,
}

// inferFromEndpointSet 基于一组已收集的 endpoint 类型推断 modality。
// specializedOnly=true 时仅返回"特化信号"（image/video/rerank/embedding），
// 对纯 chat endpoint 返回 ""；这样上层可以把 chat-only 的情况留给后续层级
// （如 rule 记录的显式 modality）决定是 text 还是 multimodal。
func inferFromEndpointSet(keys map[constant.EndpointType]struct{}, specializedOnly bool) string {
	if len(keys) == 0 {
		return ""
	}
	for _, p := range endpointModalityPriority {
		if _, ok := keys[p.endpoint]; ok {
			return p.modality
		}
	}
	if specializedOnly {
		return ""
	}
	for k := range keys {
		if chatEndpoints[k] {
			return constant.ModalityText
		}
	}
	return ""
}

// InferModalityFromEndpoints 解析 Model.endpoints 字段，按 endpoint 信号
// 推断 modality。endpoints 可以是：
//   - JSON 对象：{"openai": {...}, "image-generation": {...}}
//   - JSON 数组：["openai", "image-generation"]
// 返回空字符串表示无法推断。
func InferModalityFromEndpoints(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	keys := make(map[constant.EndpointType]struct{})
	var asMap map[string]interface{}
	if err := common.UnmarshalJsonStr(raw, &asMap); err == nil {
		for k := range asMap {
			keys[constant.EndpointType(k)] = struct{}{}
		}
	} else {
		var asList []string
		if err := common.UnmarshalJsonStr(raw, &asList); err != nil {
			return ""
		}
		for _, k := range asList {
			keys[constant.EndpointType(k)] = struct{}{}
		}
	}
	return inferFromEndpointSet(keys, false)
}

// inferStrongModalityFromRuntime 从运行时 endpoint 缓存（由渠道/ability 汇总
// 得到）查询某个模型名的支持 endpoint，只在出现"特化信号"时返回：
// image / video / rerank / embedding。纯 chat endpoint 返回 ""，交给后续
// 层决定 text / multimodal。
func inferStrongModalityFromRuntime(modelName string) string {
	if modelName == "" {
		return ""
	}
	eps := GetModelSupportEndpointTypes(modelName)
	if len(eps) == 0 {
		return ""
	}
	set := make(map[constant.EndpointType]struct{}, len(eps))
	for _, ep := range eps {
		set[ep] = struct{}{}
	}
	return inferFromEndpointSet(set, true)
}

// ResolveModality 仅依据单条 Model 记录自身的字段做解析（显式 modality >
// endpoints 推断）。保留给已持有一条 Model 记录、不关心 name 级别分层的
// 调用方使用。
func ResolveModality(m *Model) string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.Modality) != "" {
		return m.Modality
	}
	if inferred := InferModalityFromEndpoints(m.Endpoints); inferred != "" {
		return inferred
	}
	return ""
}

// ResolveModalityForName 对一个具体的模型名按分层策略解析 modality。
// 这是 Playground 等面向 **真实被启用模型名** 场景的首选入口，它把多路
// 证据按"专一性-权威性"组合排序。
//
// 决策顺序（越上越优先）：
//  1. exact 记录显式 modality             — 该模型的管理员最终裁决
//  2. exact 记录 endpoints 推断           — 该模型自带的能力声明
//  3. 运行时 endpoints 特化信号           — image/video/rerank/embedding
//                                          （间接包含管理员补充的 image 模式）
//  4. 管理员 CustomModalityPatterns 命中  — 跨模型的通用补充规则
//                                          （不依赖 Model 记录）
//  5. rule 记录显式 modality              — 家族级（如 gemini 前缀）默认
//  6. rule 记录 endpoints 推断            — 家族级弱信号
//  7. 运行时 endpoints 弱信号 (text)      — 只有 chat 的回退
//  8. 空字符串                            — 调用方自行 text 兜底
func ResolveModalityForName(modelName string, exact *Model, rule *Model) string {
	if exact != nil {
		if v := strings.TrimSpace(exact.Modality); v != "" {
			return v
		}
		if inferred := InferModalityFromEndpoints(exact.Endpoints); inferred != "" {
			return inferred
		}
	}
	if strong := inferStrongModalityFromRuntime(modelName); strong != "" {
		return strong
	}
	if v := model_setting.MatchModalityByPatterns(modelName); v != "" {
		return v
	}
	if rule != nil {
		if v := strings.TrimSpace(rule.Modality); v != "" {
			return v
		}
		if inferred := InferModalityFromEndpoints(rule.Endpoints); inferred != "" {
			return inferred
		}
	}
	// 最后，运行时 endpoint 只要存在就作为最弱信号兜底（可能是 text）
	if modelName != "" {
		eps := GetModelSupportEndpointTypes(modelName)
		if len(eps) > 0 {
			set := make(map[constant.EndpointType]struct{}, len(eps))
			for _, ep := range eps {
				set[ep] = struct{}{}
			}
			if v := inferFromEndpointSet(set, false); v != "" {
				return v
			}
		}
	}
	return ""
}
