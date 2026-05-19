// Package events 提供统一的事件总线 + Outbox 异步分发能力。
//
// 设计文档：docs/event-system-design.md
//
// 核心概念：
//   - Publish / PublishNoTx：业务代码调用，写 event_log + 扇出到 event_dispatch
//   - Subscriber：实现接口并 init 时 Register，按 topic 订阅事件
//   - Worker：后台轮询拉 dispatch，调对应 Subscriber.Handle
//
// 事件命名约定：<domain>.<action> 或 <domain>.<subdomain>.<action>，
// 订阅者 Topics() 支持前缀通配（如 "user.*"）和全订阅（"*"）。
package events

// 事件类型常量。新增事件必须在此集中定义，禁止业务代码使用裸字符串。
const (
	// user 域 —— 用户生命周期与属性变化
	UserRegistered     = "user.registered"
	UserDeleted        = "user.deleted"
	UserProfileUpdated = "user.profile.updated"
	UserEmailBound     = "user.email.bound"
	UserGroupChanged   = "user.group.changed"

	// billing 域 —— 钱流相关
	BillingTopupSucceeded = "billing.topup.succeeded"
	BillingRedemptionUsed = "billing.redemption.used"
)

// Event 是订阅者 Handle 收到的事件结构体。
// Payload 是原始 JSON 字节，订阅者按需 Unmarshal 到具体 Payload struct。
type Event struct {
	Id          int64
	Type        string
	AggregateId int
	Payload     []byte
	PublishedAt int64
}

// 各事件的 Payload struct。Publish 时传入对应类型的指针。

type UserRegisteredPayload struct {
	UserId         int    `json:"user_id"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Group          string `json:"group"`
	RegisterSource string `json:"register_source"` // email|github|discord|oidc|linuxdo|telegram|wechat
	InviterId      int    `json:"inviter_id,omitempty"`
	CreatedAt      int64  `json:"created_at"`
}

type UserDeletedPayload struct {
	UserId     int    `json:"user_id"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	DeleteType string `json:"delete_type"` // admin_hard|self_soft
	DeletedAt  int64  `json:"deleted_at"`
}

type UserProfileUpdatedPayload struct {
	UserId        int      `json:"user_id"`
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	DisplayName   string   `json:"display_name"`
	ChangedFields []string `json:"changed_fields"`
	UpdatedAt     int64    `json:"updated_at"`
}

type UserEmailBoundPayload struct {
	UserId   int    `json:"user_id"`
	OldEmail string `json:"old_email,omitempty"`
	NewEmail string `json:"new_email"`
	BoundAt  int64  `json:"bound_at"`
}

type UserGroupChangedPayload struct {
	UserId    int    `json:"user_id"`
	Email     string `json:"email"`
	FromGroup string `json:"from_group"`
	ToGroup   string `json:"to_group"`
	Trigger   string `json:"trigger"` // topup|subscription|admin|manual
	ChangedAt int64  `json:"changed_at"`
}

type BillingTopupSucceededPayload struct {
	UserId           int    `json:"user_id"`
	Email            string `json:"email"`
	TopupId          int    `json:"topup_id"`
	AmountQuota      int    `json:"amount_quota"`
	AmountMoneyCents int64  `json:"amount_money_cents"`
	Currency         string `json:"currency"`
	PaymentMethod    string `json:"payment_method"` // stripe|epay|waffo|creem
	TradeNo          string `json:"trade_no"`
	CompletedAt      int64  `json:"completed_at"`
}

type BillingRedemptionUsedPayload struct {
	UserId       int    `json:"user_id"`
	Email        string `json:"email"`
	RedemptionId int    `json:"redemption_id"`
	AmountQuota  int    `json:"amount_quota"`
	UsedAt       int64  `json:"used_at"`
}
