package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// TokenSetting 令牌相关配置
type TokenSetting struct {
	MaxUserTokens int `json:"max_user_tokens"` // 每用户最大令牌数量
	// DefaultUserMaxConcurrency 全局默认的「账户级」并发上限，作用于未被管理员
	// 单独配置的用户。0 表示不限制（并发功能整体关闭）。
	//
	// 三层结构：全局默认 → 用户账户级(UserSetting.MaxConcurrency) → 令牌级。
	// 账户级统计该用户所有令牌的在途请求之和；令牌级只管单个令牌，且被静默夹紧到
	// 账户级实际值（管理员事后调低账户级时，存量令牌自动收紧而不是报错失效）。
	DefaultUserMaxConcurrency int `json:"default_user_max_concurrency"`

	// MaxTokenConcurrency 旧字段名，仅用于读取存量配置后迁移到
	// DefaultUserMaxConcurrency。不要在新代码里使用。
	//
	// 注意 json tag 不能带 ",omitempty"：配置系统（setting/config/config.go）
	// 用完整 tag 字符串做键名匹配，带 omitempty 会让这个键永远读不进来，
	// 迁移就会静默失效。
	// 保留原因：线上 option 表里可能已存有 token_setting.max_token_concurrency，
	// 直接改名会让管理员配过的值静默归零。
	MaxTokenConcurrency int `json:"max_token_concurrency"`
}

// 默认配置
var tokenSetting = TokenSetting{
	MaxUserTokens:             1000, // 默认每用户最多 1000 个令牌
	DefaultUserMaxConcurrency: 0,    // 默认 0：并发限制整体关闭
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("token_setting", &tokenSetting)
}

// GetTokenSetting 获取令牌配置
func GetTokenSetting() *TokenSetting {
	return &tokenSetting
}

// GetMaxUserTokens 获取每用户最大令牌数量
func GetMaxUserTokens() int {
	return GetTokenSetting().MaxUserTokens
}

// GetDefaultUserMaxConcurrency 获取全局默认账户级并发上限。0 表示不限制。
//
// 这个函数在每个中继请求的热路径上被调用，必须保持廉价：直接读进程内全局结构体
// 字段，不加锁、不查库、不分配。配置变更时由 config 管理器整体替换。
func GetDefaultUserMaxConcurrency() int {
	// 旧键 max_token_concurrency 由 model.migrateLegacyConcurrencyOption 在启动时
	// 落库搬到新键，这里不做读取期回落 —— 否则管理面板显示新键原始值(0)而限流按
	// 旧值在跑，显示与行为不一致。
	return GetTokenSetting().DefaultUserMaxConcurrency
}
