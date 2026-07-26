package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// GroupChainSetting 控制「令牌多分组链」的选路、重试预算与渠道健康度熔断。
//
// 设计要点：
//
//   - 链长本身几乎不影响性能。链上没有目标模型的分组会被内存索引零成本跳过
//     （见 model.GetPriorityLevelCount），不发请求、不消耗重试预算，所以
//     MaxChainLength 只是 UI / 存储保护，不是性能约束。
//
//   - 尾延迟用「时间预算」而不是「换组次数」来控制。一次上游超时可能几十秒，
//     用次数根本无法反映用户实际等待时长。
//
//   - 熔断是让链路变快的主要手段：重试是被动的，每个请求都要重新踩一遍坑；
//     熔断是主动的，一个请求踩到之后，冷却窗口内其它请求直接绕开。
type GroupChainSetting struct {
	// MaxChainLength 单个令牌最多可选择的分组数量。
	MaxChainLength int `json:"max_chain_length"`

	// TotalRetryBudgetMs 单次请求从进入 relay 循环开始的重试总时间预算（毫秒）。
	// 0 表示不限制。每次准备重试前检查剩余预算，不足则直接返回最后一次的错误。
	TotalRetryBudgetMs int `json:"total_retry_budget_ms"`

	// HealthEnabled 是否启用渠道健康度熔断。
	HealthEnabled bool `json:"health_enabled"`
	// HealthWindowSeconds 失败计数窗口（秒）。
	HealthWindowSeconds int `json:"health_window_seconds"`
	// HealthThreshold 窗口内失败多少次后进入冷却，冷却期内选路阶段直接跳过该
	// (渠道, 模型) 组合。
	HealthThreshold int `json:"health_threshold"`
	// HealthMaxEntries 熔断计数器的内存缓存容量。
	HealthMaxEntries int `json:"health_max_entries"`

	// CrossGroupSkipStatusCodes 命中这些 HTTP 状态码时只在当前分组内重试，
	// 不跨分组回退。默认是「请求本身有问题」的状态码 —— 换多少个分组都是同样
	// 的失败，跨组只会浪费额度和时延。
	//
	// 注意：这一层是叠加在既有 ShouldRetryByStatusCode 之上的额外约束，只影响
	// 「是否走出当前分组」，不改变组内重试行为，因此对单分组令牌完全无感。
	CrossGroupSkipStatusCodes []int `json:"cross_group_skip_status_codes"`

	// CrossGroupSkipErrorCodes 命中这些内部错误码时不跨分组回退。
	CrossGroupSkipErrorCodes []string `json:"cross_group_skip_error_codes"`
}

var groupChainSetting = GroupChainSetting{
	MaxChainLength:     10,
	TotalRetryBudgetMs: 0,

	HealthEnabled:       true,
	HealthWindowSeconds: 60,
	HealthThreshold:     3,
	HealthMaxEntries:    100_000,

	CrossGroupSkipStatusCodes: []int{400, 413, 422},
	CrossGroupSkipErrorCodes: []string{
		"sensitive_words_detected",
		"prompt_blocked",
		"invalid_request",
		"bad_request_body",
		"convert_request_failed",
	},
}

func init() {
	config.GlobalConfig.Register("group_chain_setting", &groupChainSetting)
}

func GetGroupChainSetting() *GroupChainSetting {
	return &groupChainSetting
}

// GetMaxChainLength 返回令牌分组链长度上限，做了下界保护，避免误配成 0 后
// 所有令牌都存不下分组。
func GetMaxChainLength() int {
	if groupChainSetting.MaxChainLength <= 0 {
		return 10
	}
	return groupChainSetting.MaxChainLength
}
