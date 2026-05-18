package dto

// TicketAttachment 工单/消息的附件，前端先用 /api/upload/presign 直传 R2，
// 拿到结果后把 URL + 元数据塞这里。后端不持有文件流，只存元数据。
type TicketAttachment struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// TicketBugContext 模型调用 / 渠道问题工单的结构化补充字段。
// 所有字段都可选；request_id 是首选 —— 一旦后端校验通过其它字段会被覆盖。
//
// 字段使用指针的目的（参考 Rule 6）：让 "用户没填" 与 "用户显式填了 0 / 空"
// 在 JSON 层面可区分；不过本场景目前都是字符串和整型上下文，使用指针
// 主要是为了让 marshal 时的零值不被无意义地落到详情卡片上。
type TicketBugContext struct {
	RequestId         string `json:"request_id,omitempty"`
	RequestIdVerified bool   `json:"request_id_verified,omitempty"`

	ChannelId   int    `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty"`
	Group       string `json:"group,omitempty"`
	Model       string `json:"model,omitempty"`
	TokenName   string `json:"token_name,omitempty"`

	OccurredAt   int64  `json:"occurred_at,omitempty"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	ErrorExcerpt string `json:"error_excerpt,omitempty"`

	LogId int `json:"log_id,omitempty"`
}

// TicketRefundTopupRef 退款工单引用的一笔充值订单。前端只需要给出
// trade_no（充值订单号），其它字段由后端 enrichment 从 top_ups 表回填，
// 用户传入的值不被信任（防伪造金额 / 时间）。
type TicketRefundTopupRef struct {
	TradeNo       string  `json:"trade_no"`
	Money         float64 `json:"money,omitempty"`
	Amount        int64   `json:"amount,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`
	CompletedAt   int64   `json:"completed_at,omitempty"`
}

// TicketRefundContext 退款工单的结构化补充字段。
//   - Method=platform：从平台在线充值的订单，TopUps 必填、所有 trade_no 必须是
//     本人且 status=success；
//   - Method=offline：线下充值（运营手工对账），不附订单；
//   - Reason=other 时必须填 ReasonOther（≤512 字）。
type TicketRefundContext struct {
	Method      string                 `json:"method"`
	Reason      string                 `json:"reason"`
	ReasonOther string                 `json:"reason_other,omitempty"`
	TopUps      []TicketRefundTopupRef `json:"topups,omitempty"`
}

// TicketMetadata 是 tickets.metadata JSON 列的强类型表示。新增字段按需补充，
// 不要把它做成 map[string]any，避免前后端契约漂移。
type TicketMetadata struct {
	BugContext    *TicketBugContext    `json:"bug_context,omitempty"`
	RefundContext *TicketRefundContext `json:"refund_context,omitempty"`
	ClientUA      string               `json:"client_ua,omitempty"`
	ClientIP      string               `json:"client_ip,omitempty"`
}

// ----- 请求 DTO -----

// CreateTicketReq 用户建单入参。
type CreateTicketReq struct {
	Type        string             `json:"type" binding:"required"`
	Category    string             `json:"category" binding:"required"`
	Title       string             `json:"title" binding:"required"`
	Content     string             `json:"content" binding:"required"`
	// Priority 是用户建议优先级；最终是否生效由管理员判断。可空，省略时取默认 1（普通）。
	// 用指针：Rule 6 — 区分"没传"和"显式传 0"。
	Priority    *int               `json:"priority,omitempty"`
	Attachments   []TicketAttachment   `json:"attachments,omitempty"`
	BugContext    *TicketBugContext    `json:"bug_context,omitempty"`
	RefundContext *TicketRefundContext `json:"refund_context,omitempty"`
}

// ReplyTicketReq 工单追加回复入参。管理员侧也复用，IsInternal 仅管理员可设
// （v1 暂时忽略，预留 v2）。
type ReplyTicketReq struct {
	Content     string             `json:"content" binding:"required"`
	Attachments []TicketAttachment `json:"attachments,omitempty"`
	IsInternal  bool               `json:"is_internal,omitempty"`
}

// AdminUpdateTicketReq 管理员修改工单可变字段。
// 全部使用指针类型，遵循 Rule 6：nil 表示不动这个字段。
type AdminUpdateTicketReq struct {
	Status   *int    `json:"status,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	Category *string `json:"category,omitempty"`
}

// ----- 响应 DTO -----

// TicketListItem 列表页一行的精简视图，对应 model.ListTickets 的投影列。
type TicketListItem struct {
	Id            int    `json:"id"`
	UserId        int    `json:"user_id"`
	Username      string `json:"username,omitempty"` // 仅管理员侧填充
	Type          string `json:"type"`
	Category      string `json:"category"`
	Title         string `json:"title"`
	Status        int    `json:"status"`
	Priority      int    `json:"priority"`
	LastReplyAt   int64  `json:"last_reply_at"`
	LastReplyRole int    `json:"last_reply_role"`
	ReplyCount    int    `json:"reply_count"`
	ChannelId     int    `json:"channel_id,omitempty"`
	ModelName     string `json:"model_name,omitempty"`
	Group         string `json:"group,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// TicketMessageView 单条消息的对外结构。
type TicketMessageView struct {
	Id          int                `json:"id"`
	TicketId    int                `json:"ticket_id"`
	SenderId    int                `json:"sender_id"`
	SenderRole  int                `json:"sender_role"`
	SenderName  string             `json:"sender_name,omitempty"`
	Content     string             `json:"content"`
	Attachments []TicketAttachment `json:"attachments,omitempty"`
	IsInternal  bool               `json:"is_internal,omitempty"`
	CreatedAt   int64              `json:"created_at"`
}

// TicketDetailView 工单详情：主信息 + 完整 metadata + 消息流。
type TicketDetailView struct {
	Id            int                 `json:"id"`
	UserId        int                 `json:"user_id"`
	Username      string              `json:"username,omitempty"`
	Type          string              `json:"type"`
	Category      string              `json:"category"`
	Title         string              `json:"title"`
	Status        int                 `json:"status"`
	Priority      int                 `json:"priority"`
	AssigneeId    int                 `json:"assignee_id,omitempty"`
	LastReplyAt   int64               `json:"last_reply_at"`
	LastReplyRole int                 `json:"last_reply_role"`
	ReplyCount    int                 `json:"reply_count"`
	ChannelId     int                 `json:"channel_id,omitempty"`
	ChannelName   string              `json:"channel_name,omitempty"`
	ModelName     string              `json:"model_name,omitempty"`
	Group         string              `json:"group,omitempty"`
	Attachments   []TicketAttachment  `json:"attachments,omitempty"`
	Metadata      *TicketMetadata     `json:"metadata,omitempty"`
	Messages      []TicketMessageView `json:"messages"`
	HasMore       bool                `json:"has_more,omitempty"`
	CreatedAt     int64               `json:"created_at"`
	UpdatedAt     int64               `json:"updated_at"`
	ClosedAt      int64               `json:"closed_at,omitempty"`
}

// TicketStatsView 管理员仪表盘卡片。
type TicketStatsView struct {
	Pending  int64 `json:"pending"`  // 待处理（含 open + pending 且最后回复非管理员）
	Open     int64 `json:"open"`
	Resolved int64 `json:"resolved"`
	Closed   int64 `json:"closed"`
}
