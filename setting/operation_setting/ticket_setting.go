package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// TicketSetting 工单系统配置。通过 GlobalConfig 自动序列化进 options 表，
// 字段命名采用 snake_case JSON tag —— GlobalConfig 用 reflect 走 json tag
// 作为子键，root 在前端 option 设置页里能直接调。
type TicketSetting struct {
	Enabled bool `json:"enabled"`

	// 用户每日 / 每小时建单上限。0 表示不限制。
	UserDailyLimit  int `json:"user_daily_limit"`
	UserHourlyLimit int `json:"user_hourly_limit"`
	// 用户回复 60s 窗口内最多次数（防自动化刷屏）。0 表示不限。
	UserReplyPerMinute int `json:"user_reply_per_minute"`

	// 内容长度上限（字符数）。
	MaxTitleLength   int `json:"max_title_length"`
	MaxContentLength int `json:"max_content_length"`

	// 每条工单/回复允许的附件数量上限（0 表示不限）。
	MaxAttachmentsPerMessage int `json:"max_attachments_per_message"`

	// 仅持有已验证邮箱的用户能建单。
	RequireVerifiedEmail bool `json:"require_verified_email"`

	// 自动 resolve：超过 N 天没人回复就转为 resolved。0 表示不启用。
	AutoResolveDays int `json:"auto_resolve_days"`

	// Reopen 时限（天）：用户在已关闭/已解决的工单上回复时，
	// 超过对应窗口仍不允许 reopen，需新建工单。0 表示不限。
	ReopenAfterClosedDays   int `json:"reopen_after_closed_days"`   // 手动 close
	ReopenAfterResolvedDays int `json:"reopen_after_resolved_days"` // 自动/标记 resolved

	// 通知开关。每个开关独立，灰度时方便单独关闭某一个通道。
	NotifyEmailToAdmin    bool `json:"notify_email_to_admin"`     // 新工单/用户回复 → 邮件管理员
	NotifyEmailToUser     bool `json:"notify_email_to_user"`      // 管理员回复 → 邮件用户
	NotifyTelegramToAdmin bool `json:"notify_telegram_to_admin"`  // 新工单/用户回复 → Telegram
	NotifyInAppEnabled    bool `json:"notify_in_app_enabled"`     // 用户站内未读小红点

	// Telegram 推送地址：bot token + chat id，仅 admin 推送用。
	// 建议放专用群组而非个人会话。空字符串则跳过 Telegram。
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatId   string `json:"telegram_chat_id"`

	// 接收新工单邮件的管理员邮箱列表（英文逗号分隔）。空则用 root 邮箱兜底。
	AdminEmails string `json:"admin_emails"`
}

// 默认值。所有功能默认关闭，避免升级老实例时出现意外行为。
var ticketSetting = TicketSetting{
	Enabled:                  false,
	UserDailyLimit:           30,
	UserHourlyLimit:          10,
	UserReplyPerMinute:       6,
	MaxTitleLength:           200,
	MaxContentLength:         32 * 1024,
	MaxAttachmentsPerMessage: 6,
	RequireVerifiedEmail:     true,
	AutoResolveDays:          14,
	ReopenAfterClosedDays:    7,
	ReopenAfterResolvedDays:  30,
	NotifyEmailToAdmin:       true,
	NotifyEmailToUser:        true,
	NotifyTelegramToAdmin:    false,
	NotifyInAppEnabled:       true,
	TelegramBotToken:         "",
	TelegramChatId:           "",
	AdminEmails:              "",
}

func init() {
	config.GlobalConfig.Register("ticket_setting", &ticketSetting)
}

// GetTicketSetting 返回当前工单配置。返回指针避免值拷贝，但调用方不应修改字段
// （修改后不会回写 DB）。
func GetTicketSetting() *TicketSetting {
	return &ticketSetting
}

// IsTicketEnabled 顶层开关快捷查询。
func IsTicketEnabled() bool {
	return ticketSetting.Enabled
}
