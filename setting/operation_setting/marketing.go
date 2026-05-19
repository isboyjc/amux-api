package operation_setting

// 邮件营销 / Resend 集成配置。
//
// 这些变量通过 model.InitOptionMap + updateOptionMap 持久化到 option 表，
// admin 在后台修改后会被全实例 SyncOptions 同步。
//
// 任何 marketing 相关配置变更都应该调用 TriggerMarketingReload —— 让 main.go 注册
// 的钩子重新构造 marketing.Provider 并 SetProvider。这就是"开关一开/令牌一改就生效"
// 的实现机制。
var (
	// 总开关。关闭时 Provider 设为 nil，订阅者变成 no-op；现有事件队列照常 drain。
	MarketingEnabled = false

	// 当前使用的 provider。未来可填 "mailchimp" / "sendgrid" 等；目前只支持 "resend"。
	MarketingProvider = "resend"

	// Resend API Key。明文存储（与 EpayKey / StripeApiSecret 处理一致）。
	// 后台 UI 应做 mask 显示，不回显原文。
	ResendAPIKey = ""

	// Resend Segment ID（UUID）。
	ResendDefaultSegmentID = ""
	ResendVIPSegmentID     = ""

	// 创建新 contact 时默认 opt_in 的 topic IDs，逗号分隔字符串。
	// 例：" abc-123, def-456" → ["abc-123", "def-456"]
	ResendDefaultTopicIDs = ""
)

// OnMarketingConfigChanged 是配置变更钩子。由 main.go 在启动时设置为"重新构造
// Provider 并 SetProvider"的回调。
//
// 使用钩子而非直接调 marketing.SetProvider 是为了打破 model → service/marketing 的
// 导入循环（model/option.go 不能 import service/marketing，因为 marketing 包反过来
// import 了 model）。
var OnMarketingConfigChanged func()

// TriggerMarketingReload 在任意 marketing 配置变更后调用。
// 钩子未注册时安静返回，方便测试和早期初始化。
func TriggerMarketingReload() {
	if OnMarketingConfigChanged != nil {
		OnMarketingConfigChanged()
	}
}
