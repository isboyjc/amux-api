package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 令牌分组链。
//
// 一个令牌可以按顺序绑定多个分组，请求时按链顺序寻找拥有目标模型的分组；
// 高优先级分组失败后自动回落到下一个拥有该模型的分组。
//
// 「auto 分组」不再是一个特殊分组，而只是分组链的一种来源：它解析成管理员配置的
// 那条链。这么做的关键收益是选路逻辑只有一份 —— 老的 auto 状态机
// （ContextKeyAutoGroupIndex / AutoGroupRetryIndex）被整体替换掉。
//
// 存量令牌零迁移：groups 列为空时按 Group 字段的旧语义解析，行为与改动前逐字节
// 一致。只有用户主动在新 UI 里保存过的令牌才会写入显式链。

type GroupChainSource string

const (
	// GroupChainSourceExplicit 用户显式配置的分组链（新行为）
	GroupChainSourceExplicit GroupChainSource = "explicit"
	// GroupChainSourceLegacyAuto 旧版 auto 分组，运行时展开成管理员配置的链
	GroupChainSourceLegacyAuto GroupChainSource = "legacy_auto"
	// GroupChainSourceLegacySingle 旧版单分组令牌
	GroupChainSourceLegacySingle GroupChainSource = "legacy_single"
	// GroupChainSourceUserGroup 令牌未指定分组，使用用户自身等级分组
	GroupChainSourceUserGroup GroupChainSource = "user_group"
)

type GroupChain struct {
	Groups     []string
	Source     GroupChainSource
	CrossGroup bool
}

func (c GroupChain) IsEmpty() bool {
	return len(c.Groups) == 0
}

func (c GroupChain) Head() string {
	if len(c.Groups) == 0 {
		return ""
	}
	return c.Groups[0]
}

func (c GroupChain) String() string {
	return strings.Join(c.Groups, " → ")
}

// ResolveGroupChain 把令牌解析成有序分组链。
//
// 解析优先级（前三条是存量令牌的兼容路径，行为与改动前完全一致）：
//
//  1. groups 列非空          → 显式链，按用户可用分组过滤后保序去重
//  2. Group == "auto"        → 展开成 GetUserAutoGroup(userGroup)
//  3. Group != ""            → 单元素链
//  4. Group == ""            → 用户自身等级分组
//
// 失效分组的处理在显式链和单分组之间刻意不对称：
//
//   - 显式链里某个分组被管理员回收权限 → 跳过它，继续用链上剩下的分组。多选场景
//     下让整个令牌因为一个分组失效而不可用是不可接受的。
//   - 全部失效 → 返回错误（403）。不静默回退到用户分组，否则等于悄悄换了计费分组。
//   - 单分组 / auto → 完全维持既有逻辑，校验仍在 middleware/auth.go 里做。
func ResolveGroupChain(token *model.Token, userGroup string) (GroupChain, error) {
	if token == nil {
		if userGroup == "" {
			return GroupChain{}, fmt.Errorf("无法确定请求分组")
		}
		return GroupChain{
			Groups: []string{userGroup},
			Source: GroupChainSourceUserGroup,
		}, nil
	}

	if explicit := token.GetGroups(); len(explicit) > 0 {
		usable := GetUserUsableGroups(userGroup)
		filtered := make([]string, 0, len(explicit))
		for _, group := range explicit {
			if _, ok := usable[group]; !ok {
				// 权限被回收，跳过
				continue
			}
			if !ratio_setting.ContainsGroupRatio(group) {
				// 分组已被弃用，跳过
				continue
			}
			filtered = append(filtered, group)
		}
		if len(filtered) == 0 {
			return GroupChain{}, fmt.Errorf("令牌绑定的分组均不可用，请重新配置令牌分组")
		}
		return GroupChain{
			Groups:     filtered,
			Source:     GroupChainSourceExplicit,
			CrossGroup: token.CrossGroupRetry,
		}, nil
	}

	if token.Group == "auto" {
		autoGroups := GetUserAutoGroup(userGroup)
		if len(autoGroups) == 0 {
			return GroupChain{}, fmt.Errorf("auto groups is not enabled")
		}
		return GroupChain{
			Groups:     autoGroups,
			Source:     GroupChainSourceLegacyAuto,
			CrossGroup: token.CrossGroupRetry,
		}, nil
	}

	if token.Group != "" {
		return GroupChain{
			Groups: []string{token.Group},
			Source: GroupChainSourceLegacySingle,
		}, nil
	}

	if userGroup == "" {
		return GroupChain{}, fmt.Errorf("无法确定请求分组")
	}
	return GroupChain{
		Groups: []string{userGroup},
		Source: GroupChainSourceUserGroup,
	}, nil
}

// NormalizeGroupChainInput 清洗用户提交的分组链：去空白、去空串、保序去重。
func NormalizeGroupChainInput(groups []string) []string {
	cleaned := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		cleaned = append(cleaned, group)
	}
	return cleaned
}

// ValidateGroupChain 校验用户提交的分组链。写入期校验（今天完全没有，分组错误
// 要到发请求时才以 403 暴露出来），让用户在保存令牌时就能拿到明确报错。
func ValidateGroupChain(groups []string, userGroup string) error {
	if len(groups) == 0 {
		return nil
	}
	maxLength := operation_setting.GetMaxChainLength()
	if len(groups) > maxLength {
		return fmt.Errorf("最多只能选择 %d 个分组", maxLength)
	}
	usable := GetUserUsableGroups(userGroup)
	for _, group := range groups {
		if _, ok := usable[group]; !ok {
			return fmt.Errorf("无权访问 %s 分组", group)
		}
		if !ratio_setting.ContainsGroupRatio(group) {
			return fmt.Errorf("分组 %s 已被弃用", group)
		}
	}
	return nil
}

// GetChainEnabledModels 返回分组链上所有分组可用模型的并集，保持链顺序去重
// （靠前的分组先出现）。用于 /v1/models、定价页与令牌编辑弹窗的模型预览。
func GetChainEnabledModels(groups []string) []string {
	models := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, modelName := range model.GetGroupEnabledModels(group) {
			if _, exists := seen[modelName]; exists {
				continue
			}
			seen[modelName] = struct{}{}
			models = append(models, modelName)
		}
	}
	return models
}
