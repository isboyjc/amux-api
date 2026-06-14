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

	// MarketingExtraEligibleGroups 额外允许在「个人设置 → 邮件订阅」里自助管理
	// 订阅状态的用户组，逗号分隔。
	//
	// 用途：企业 / 自定义分组用户由 admin 手动维护到 Resend，不走 amux 的自动同步
	// （tierForUser 对这些组返回 TierNone），但 UI 上应该允许这些用户管理自己的
	// 全局退订状态和 topic 偏好。
	//
	// 例：" enterprise_a, enterprise_b" → ["enterprise_a", "enterprise_b"]
	//
	// 注意：本配置只影响 UI 资格判定（IsEligible），不会触发 amux 自动把这些组
	// 同步到 Resend。Admin 仍需手动建好 contact；用户首次保存时 ensureContactExists
	// 会兜底建/复用。
	MarketingExtraEligibleGroups = ""

	// 邀请关系羊毛风控相关配置。详见 service/marketing/affiliate_risk.go 顶部说明。
	//
	// 关闭开关后：写路径仍会调 MarkAffiliateRiskDirty（O(1) upsert，无副作用），
	// worker 进入长 sleep 不再消费，用户列表 LEFT JOIN 到的字段为 NULL 不影响显示。
	AffiliateRiskCacheEnabled = true

	// 每个 worker tick 一次性消费的脏数据上限。
	// 过大：单 tick 占用 DB 时间长，可能与备份/统计任务争抢。
	// 过小：积压时收敛慢；默认 200 在 10w 邀请人量级也能在 10 分钟内完成全量重算。
	AffiliateRiskDirtyBatchSize = 200

	// 队列空时 worker 的轮询间隔（秒）。设置为 0 走默认 60。
	AffiliateRiskDirtyIntervalSec = 60
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
