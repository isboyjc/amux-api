package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 管理后台「分组与模型定价设置 → 分组相关设置」里的分组链配置，是按
// "group_chain_setting.<json_tag>" 这个扁平 key 读写 options 的
// （见 model/option.go 的 ExportAllConfigs 与 handleConfigUpdate）。
// 前端写死了这些 key，一旦 json tag 改名而前端没跟着改，保存会静默落到一个
// 不存在的配置项上、界面还显示成功。这里把 key 名字钉住。
func TestGroupChainSettingOptionKeys(t *testing.T) {
	flat, err := config.ConfigToMap(GetGroupChainSetting())
	require.NoError(t, err)

	for _, key := range []string{
		"max_chain_length",
		"total_retry_budget_ms",
		"health_enabled",
		"health_window_seconds",
		"health_threshold",
		"cross_group_skip_status_codes",
		"cross_group_skip_error_codes",
	} {
		_, ok := flat[key]
		assert.True(t, ok, "管理后台在写 group_chain_setting.%s，配置结构体里必须有这个 json tag", key)
	}
}

// 配置从 options 表回来时全是字符串，数字 / 布尔 / 数组都要能正确还原。
func TestGroupChainSettingRoundTrip(t *testing.T) {
	original := *GetGroupChainSetting()
	t.Cleanup(func() { *GetGroupChainSetting() = original })

	target := GetGroupChainSetting()
	require.NoError(t, config.UpdateConfigFromMap(target, map[string]string{
		"max_chain_length":              "6",
		"total_retry_budget_ms":         "60000",
		"health_enabled":                "false",
		"health_window_seconds":         "30",
		"health_threshold":              "5",
		"cross_group_skip_status_codes": "[400,422]",
		"cross_group_skip_error_codes":  `["sensitive_words_detected"]`,
	}))

	assert.Equal(t, 6, target.MaxChainLength)
	assert.Equal(t, 60000, target.TotalRetryBudgetMs)
	assert.False(t, target.HealthEnabled)
	assert.Equal(t, 30, target.HealthWindowSeconds)
	assert.Equal(t, 5, target.HealthThreshold)
	assert.Equal(t, []int{400, 422}, target.CrossGroupSkipStatusCodes)
	assert.Equal(t, []string{"sensitive_words_detected"}, target.CrossGroupSkipErrorCodes)
}

// MaxChainLength 误配成 0 时不能让所有令牌都存不下分组。
func TestGetMaxChainLengthFallback(t *testing.T) {
	original := GetGroupChainSetting().MaxChainLength
	t.Cleanup(func() { GetGroupChainSetting().MaxChainLength = original })

	GetGroupChainSetting().MaxChainLength = 0
	assert.Equal(t, 10, GetMaxChainLength())

	GetGroupChainSetting().MaxChainLength = 3
	assert.Equal(t, 3, GetMaxChainLength())
}
