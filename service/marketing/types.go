// Package marketing 是营销联系人同步的"平台无关层"。
//
// 角色：把业务事件（user.registered / billing.topup.succeeded / user.group.changed 等）
// 翻译成"目标平台应该处于什么状态"（Intent），再交给具体平台的 Provider 去落地。
//
// 这层不知道 Resend、MailChimp 或其他任何具体平台的存在；Provider 接口的实现者负责
// 把 Intent 翻译为它自己的 API 调用。换平台时只需新写一个 Provider 实现 + 在 main.go
// 改一行注入。
//
// 状态来源：所有 Resolve 都从 DB 当前态计算（不依赖事件 payload）—— 这意味着事件
// 到达顺序无关、重放幂等、新增触发事件无需改 Resolve 逻辑。
package marketing

import "context"

// Tier 营销会员等级。平台无关。
//
// 这是"业务对营销联系人状态的抽象"——具体平台用什么概念去表达（Resend Segment、
// MailChimp List、Sendgrid Group），由 Provider 自己映射。
type Tier string

const (
	TierNone    Tier = ""        // 不在营销系统里（免费用户、企业用户、已删除等）
	TierDefault Tier = "default" // 普通付费用户
	TierVIP     Tier = "vip"     // VIP 用户
)

// RemovalMode 决定 Tier=TierNone 时如何把 contact 从平台移除。
//
// 区分两种本质不同的"退出"语义：
//   - RemovalSoftUnsubscribe（默认）：用户退出付费组（admin 改商务关系等）。
//     contact 保留 + 用户在平台手动设置的偏好（unsubscribed 状态、单独取消的 topic）
//     全部保留。Provider 实现应 PATCH unsubscribed=true 而非整体删除。
//   - RemovalHardDelete：用户被注销账号（user.deleted 事件）。
//     符合 GDPR / 被遗忘权，连同所有偏好整体真删。
type RemovalMode int

const (
	RemovalSoftUnsubscribe RemovalMode = iota // 默认值（零值），保留偏好的软退订
	RemovalHardDelete                          // GDPR 真删
)

// Intent 描述事件应当推到外部平台的"目标状态"。
//
// Provider.Sync 应实现幂等：对同一 Intent 反复调用应得到同一终态，不报错。
// Sync 内部可能涉及多次 API 调用，平台应自己处理"加目标 segment 之前先删另一个"
// 等切换语义。
type Intent struct {
	// TargetEmail 目标联系人邮箱（必填）。
	TargetEmail string

	// DisplayName 用于设置联系人姓名字段（可选，空则不更新）。
	DisplayName string

	// Tier 目标 tier。
	//   - TierNone：从平台移除（如果存在），具体行为看 RemovalMode
	//   - 其他：确保 contact 存在并只在对应 tier 的 segment/list 里
	Tier Tier

	// RemovalMode 仅在 Tier==TierNone 时有意义。默认零值 RemovalSoftUnsubscribe。
	RemovalMode RemovalMode

	// CleanupEmail 额外需要清理的旧邮箱（邮箱迁移场景）。
	// 普通场景留空；非空时 Provider 应先 DELETE CleanupEmail，再处理 TargetEmail。
	CleanupEmail string
}

// Topic 是一个"用户可自助订阅项"的抽象。
//
// 跟 Tier/Segment 的区别：Tier 是 admin 管理的分层（用户不能改），
// Topic 是用户自己勾选的细粒度兴趣分类。各平台映射：
//   - Resend → Topic（UUID）
//   - MailChimp → Interest within Interest Category
//   - Sendgrid → Suppression Group
type Topic struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// TopicSubscription 描述某个 contact 对单个 topic 的勾选状态。
type TopicSubscription struct {
	TopicID    string `json:"topic_id"`
	Subscribed bool   `json:"subscribed"`
}

// Subscriptions 描述某 contact 的全局退订状态 + 各 topic 的勾选状态。
// 用作 Provider.GetSubscriptions / UpdateSubscriptions 的读写载体。
type Subscriptions struct {
	GlobalUnsubscribed bool                `json:"global_unsubscribed"`
	Topics             []TopicSubscription `json:"topics"`
}

// Provider 是外部营销平台的抽象。
//
// 实现要求：
//   - Sync 必须幂等：同一 Intent 反复调用得到同一终态
//   - 临时错误（5xx / 网络抖动 / 429）返回普通 error → worker 退避重试
//   - 永久错误（4xx 配置错 / 无效邮箱等）返回 events.ErrPermanent → worker 标记 dead
//   - 资源不存在（404 on DELETE / PATCH）当成功处理
type Provider interface {
	// Name 返回 provider 标识（如 "resend"），用于日志和 admin UI。
	Name() string

	// Sync 把目标状态推到平台。
	Sync(ctx context.Context, intent Intent) error

	// ListTopics 列出 admin 在 provider 后台配置的可订阅 topic 详情。
	// 入参 topicIDs 是从 amux 后台配置（如 ResendDefaultTopicIDs）拆分出来的 ID 列表；
	// Provider 应到对应平台拉取每个 topic 的 name/description 并返回。
	// 实现可在内部加短期缓存（topic 列表变化频率极低）。
	ListTopics(ctx context.Context, topicIDs []string) ([]Topic, error)

	// GetSubscriptions 查指定 contact 当前的全局退订状态 + topic 订阅状态。
	// 用于"用户进 amux 设置页时反向读 provider 当前真实状态"。
	// contact 不存在时返回零值 Subscriptions（不算错误，相当于"尚未注册"）。
	GetSubscriptions(ctx context.Context, email string) (*Subscriptions, error)

	// UpdateSubscriptions 把用户在 amux 的勾选写回 provider。
	// 应同时更新 global unsubscribed 状态和所有 topic 订阅状态。
	//
	// displayName 用于在 contact 还不存在时（用户刚成为付费用户，事件 worker 尚未
	// 处理 → 还未 upsert）兜底创建 contact，避免出现"保存成功但什么都没写入"的
	// 静默数据丢失。Provider 实现应当 ensure contact 存在再做 PATCH。
	UpdateSubscriptions(ctx context.Context, email string, displayName string, subs Subscriptions) error
}
