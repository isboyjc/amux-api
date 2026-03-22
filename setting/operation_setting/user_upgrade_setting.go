package operation_setting

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// UserUpgradeRule 用户升级规则
type UserUpgradeRule struct {
	FromGroup string  `json:"from_group"` // 源分组
	ToGroup   string  `json:"to_group"`   // 目标分组
	Threshold float64 `json:"threshold"`  // 充值金额阈值（美元）
}

type UserUpgradeSetting struct {
	AutoUpgradeEnabled bool              `json:"auto_upgrade_enabled"` // 是否启用自动升级
	UpgradeRules       []UserUpgradeRule `json:"upgrade_rules"`        // 升级规则列表
}

// 默认配置
var userUpgradeSetting = UserUpgradeSetting{
	AutoUpgradeEnabled: false,
	UpgradeRules:       []UserUpgradeRule{},
}

func init() {
	config.GlobalConfig.Register("user_upgrade_setting", &userUpgradeSetting)
}

func GetUserUpgradeSetting() *UserUpgradeSetting {
	return &userUpgradeSetting
}

// IsAutoUpgradeEnabled 是否启用自动升级
func IsAutoUpgradeEnabled() bool {
	return userUpgradeSetting.AutoUpgradeEnabled
}

// GetUpgradeRules 获取升级规则列表
func GetUpgradeRules() []UserUpgradeRule {
	return userUpgradeSetting.UpgradeRules
}

// UpgradeRules2JSONString 将升级规则转换为JSON字符串
func UpgradeRules2JSONString() string {
	jsonBytes, err := json.Marshal(userUpgradeSetting.UpgradeRules)
	if err != nil {
		common.SysLog("error marshalling upgrade rules: " + err.Error())
		return "[]"
	}
	return string(jsonBytes)
}

// UpdateUpgradeRulesByJSONString 从JSON字符串更新升级规则
func UpdateUpgradeRulesByJSONString(jsonStr string) error {
	var rules []UserUpgradeRule
	err := json.Unmarshal([]byte(jsonStr), &rules)
	if err != nil {
		return err
	}
	userUpgradeSetting.UpgradeRules = rules
	return nil
}
