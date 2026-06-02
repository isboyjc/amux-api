package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Ticket 工单主表。一张工单 = 一个会话主题，承载 N 条消息（见 TicketMessage）。
//
// 设计要点：
//   - status 只有 4 个：open / pending / resolved / closed。
//     "用户应回复 / 管理员应回复" 等态势靠 LastReplyRole 推导，前端文案再渲染。
//   - ChannelId / ModelName / Group 三列从 metadata 平铺出来，带索引，
//     用于"列出某渠道/某模型的全部工单"这种反查场景。同步镜像写入 Metadata
//     JSON，方便整体读取。
//   - Attachments / Metadata 用 TEXT 存 JSON，跨三库都 OK。
type Ticket struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId        int    `json:"user_id" gorm:"index:idx_ticket_user_status_reply,priority:1;not null"`
	Type          string `json:"type" gorm:"type:varchar(16);index;not null"`     // support / feedback
	Category      string `json:"category" gorm:"type:varchar(32);index;not null"` // model_invocation / channel_issue / billing / ...
	Title         string `json:"title" gorm:"type:varchar(200);not null"`
	Status        int    `json:"status" gorm:"index:idx_ticket_status_reply,priority:1;index:idx_ticket_user_status_reply,priority:2;not null;default:0"`
	Priority      int    `json:"priority" gorm:"not null;default:1"`
	AssigneeId    int    `json:"assignee_id" gorm:"index;not null;default:0"` // v2 启用
	LastReplyAt   int64  `json:"last_reply_at" gorm:"bigint;index:idx_ticket_status_reply,priority:2;index:idx_ticket_user_status_reply,priority:3"`
	LastReplyRole int    `json:"last_reply_role" gorm:"not null;default:0"` // 0 user / 1 admin / 2 system
	ReplyCount    int    `json:"reply_count" gorm:"not null;default:0"`

	// 用户最后查看时间，用于驱动"未读"红点；管理员侧暂不维护对应字段。
	UserSeenAt int64 `json:"user_seen_at" gorm:"bigint;not null;default:0"`

	// 调用上下文：从 bug_context 或 log 反查得到，平铺出来便于索引查询。
	ChannelId int    `json:"channel_id" gorm:"index;not null;default:0"`
	ModelName string `json:"model_name" gorm:"type:varchar(128);index;not null;default:''"`
	Group     string `json:"group" gorm:"type:varchar(64);index;not null;default:''"`

	Attachments string `json:"attachments" gorm:"type:text"` // JSON []TicketAttachment
	Metadata    string `json:"metadata" gorm:"type:text"`    // JSON TicketMetadata

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
	ClosedAt  int64 `json:"closed_at" gorm:"bigint;default:0"`
}

func (Ticket) TableName() string { return "tickets" }

// 工单状态常量。值固定，新增状态必须追加在末尾，不允许复用已删除的值。
const (
	TicketStatusOpen     = 0 // 新建/进行中（用户可见、管理员需处理）
	TicketStatusPending  = 1 // 等待对方回复（角色由 LastReplyRole 推导文案）
	TicketStatusResolved = 2 // 已解决（用户回复会自动 reopen）
	TicketStatusClosed   = 3 // 已关闭（用户回复会自动 reopen）
)

// 工单类型。
const (
	TicketTypeSupport  = "support"
	TicketTypeFeedback = "feedback"
)

// 工单消息的发送方角色。
const (
	TicketSenderRoleUser   = 0
	TicketSenderRoleAdmin  = 1
	TicketSenderRoleSystem = 2
)

// support 类型允许的 category 白名单。新增分类时同步前端 i18n。
var ticketSupportCategories = map[string]struct{}{
	"model_invocation": {},
	"channel_issue":    {},
	"billing":          {},
	"account":          {},
	"abuse":            {},
	"refund":           {},
	"other":            {},
}

// feedback 类型允许的 category 白名单。
var ticketFeedbackCategories = map[string]struct{}{
	"feature": {},
	"ux":      {},
	"docs":    {},
	"other":   {},
}

// IsValidTicketType 类型枚举校验，避免非法字符串落库后污染查询。
func IsValidTicketType(t string) bool {
	return t == TicketTypeSupport || t == TicketTypeFeedback
}

// IsValidTicketCategory category 与 type 联动校验。
func IsValidTicketCategory(t, category string) bool {
	switch t {
	case TicketTypeSupport:
		_, ok := ticketSupportCategories[category]
		return ok
	case TicketTypeFeedback:
		_, ok := ticketFeedbackCategories[category]
		return ok
	}
	return false
}

// CreateTicket 落库一张新工单。调用方负责把 metadata/attachments
// 先序列化为字符串；DAO 不做业务校验。
func CreateTicket(t *Ticket) error {
	if t.UserId <= 0 {
		return errors.New("invalid user_id")
	}
	now := common.GetTimestamp()
	if t.CreatedAt == 0 {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.LastReplyAt == 0 {
		t.LastReplyAt = now
	}
	return DB.Create(t).Error
}

// GetTicketById 取详情。注意：调用方需自行做所有权 / 管理员校验。
func GetTicketById(id int) (*Ticket, error) {
	if id <= 0 {
		return nil, errors.New("invalid ticket id")
	}
	var t Ticket
	if err := DB.First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// TicketListFilter 列表/搜索参数。所有字段都是可选过滤。
type TicketListFilter struct {
	UserId    int
	Type      string
	Category  string
	Status    *int // nil 表示不过滤
	Priority  *int // nil 表示不过滤，支持显式 0（低）
	ChannelId int
	ModelName string
	Group     string
	Keyword   string // 命中 title（LIKE）
	StartTime int64
	EndTime   int64
	Page      int
	PageSize  int
}

// ListTickets 列出工单，按 last_reply_at desc 排序。列表接口投影排除大字段
// （attachments / metadata），减少带宽。详情接口才返回完整字段。
func ListTickets(f TicketListFilter) ([]*Ticket, int64, error) {
	tx := DB.Model(&Ticket{})
	if f.UserId > 0 {
		tx = tx.Where("user_id = ?", f.UserId)
	}
	if f.Type != "" {
		tx = tx.Where("type = ?", f.Type)
	}
	if f.Category != "" {
		tx = tx.Where("category = ?", f.Category)
	}
	if f.Status != nil {
		tx = tx.Where("status = ?", *f.Status)
	}
	if f.Priority != nil {
		tx = tx.Where("priority = ?", *f.Priority)
	}
	if f.ChannelId > 0 {
		tx = tx.Where("channel_id = ?", f.ChannelId)
	}
	if f.ModelName != "" {
		tx = tx.Where("model_name = ?", f.ModelName)
	}
	if f.Group != "" {
		tx = tx.Where(commonGroupCol+" = ?", f.Group)
	}
	if f.Keyword != "" {
		// 简单 LIKE 搜索，title 已限长 200，没有性能问题
		kw := "%" + escapeLike(f.Keyword) + "%"
		tx = tx.Where("title LIKE ? ESCAPE '!'", kw)
	}
	if f.StartTime > 0 {
		tx = tx.Where("created_at >= ?", f.StartTime)
	}
	if f.EndTime > 0 {
		tx = tx.Where("created_at <= ?", f.EndTime)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	var list []*Ticket
	err := tx.Select("id, user_id, type, category, title, status, priority, assignee_id, last_reply_at, last_reply_role, reply_count, user_seen_at, channel_id, model_name, " + commonGroupCol + ", created_at, updated_at, closed_at").
		Order("last_reply_at DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&list).Error
	return list, total, err
}

// BackfillTicketUserSeenAt 一次性回填存量工单的 user_seen_at。
//
// 场景：v1 引入 user_seen_at 列时，已有工单的该字段默认为 0，导致所有
// "管理员最后回复过"的老工单立刻顶着红点。把这些工单的 user_seen_at 推到
// last_reply_at，相当于"假设用户对历史工单都已知晓"。
//
// 幂等性：CreateTicket 已会把新建工单 user_seen_at 设为 now，所以匹配
// `user_seen_at = 0 AND last_reply_at > 0` 的只剩历史数据；首次启动跑完后
// 再次启动不再命中。即使误命中也是把值改成相同的 last_reply_at，无副作用。
func BackfillTicketUserSeenAt() {
	res := DB.Model(&Ticket{}).
		Where("user_seen_at = 0 AND last_reply_at > 0").
		UpdateColumn("user_seen_at", gorm.Expr("last_reply_at"))
	if res.Error != nil {
		common.SysLog("BackfillTicketUserSeenAt error: " + res.Error.Error())
		return
	}
	if res.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("BackfillTicketUserSeenAt: %d ticket(s) baselined", res.RowsAffected))
	}
}

// MarkTicketSeenByUser 用户进入详情页时调用，把 user_seen_at 推到当前时刻。
// 只有 last_reply_role=admin 的工单需要更新（用户对自己的回复显然已读）。
// 静默忽略错误：未读红点不准不影响主流程。
func MarkTicketSeenByUser(ticketId, userId int) {
	now := common.GetTimestamp()
	_ = DB.Model(&Ticket{}).
		Where("id = ? AND user_id = ?", ticketId, userId).
		Update("user_seen_at", now).Error
}

// CountUserUnseenTickets 计算当前用户的工单中"管理员最新回复且用户尚未查看过"
// 的数量。比"是否 closed"更精确——已 closed 但用户未读过的最新回复也算未读。
func CountUserUnseenTickets(userId int) (int64, error) {
	var n int64
	err := DB.Model(&Ticket{}).
		Where("user_id = ? AND last_reply_role = ? AND last_reply_at > user_seen_at",
			userId, TicketSenderRoleAdmin).
		Count(&n).Error
	return n, err
}

// CountUserTicketsSince 计算用户在 since 之后建的工单数量，用于限流判断。
func CountUserTicketsSince(userId int, since int64) (int64, error) {
	var n int64
	err := DB.Model(&Ticket{}).
		Where("user_id = ? AND created_at >= ?", userId, since).
		Count(&n).Error
	return n, err
}

// GetTicketStatusStats 一次 GROUP BY 拿到 status → count 的映射，
// 仪表盘卡片用。比 4 次 COUNT(*) 高效，对 1k 级以内工单也微秒级返回。
func GetTicketStatusStats() (map[int]int64, error) {
	type row struct {
		Status int
		N      int64
	}
	var rows []row
	err := DB.Model(&Ticket{}).
		Select("status, COUNT(*) AS n").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int]int64, len(rows))
	for _, r := range rows {
		out[r.Status] = r.N
	}
	return out, nil
}

// CountAdminPendingTickets 管理员仪表盘小卡片用：当前待处理工单数。
// 定义为 status in (open, pending) 且 last_reply_role != admin。
func CountAdminPendingTickets() (int64, error) {
	var n int64
	err := DB.Model(&Ticket{}).
		Where("status IN ? AND last_reply_role <> ?",
			[]int{TicketStatusOpen, TicketStatusPending},
			TicketSenderRoleAdmin).
		Count(&n).Error
	return n, err
}

// UpdateTicketAfterReply 在回复事务内更新工单状态字段。
// senderRole 决定 LastReplyRole；若用户在 resolved/closed 工单回复，
// 自动 reopen 为 open。
func UpdateTicketAfterReply(tx *gorm.DB, ticketId int, senderRole int, replyAt int64) error {
	if tx == nil {
		tx = DB
	}
	updates := map[string]interface{}{
		"last_reply_at":   replyAt,
		"last_reply_role": senderRole,
		"updated_at":      replyAt,
		"reply_count":     gorm.Expr("reply_count + 1"),
	}
	// 用户回复时：resolved/closed → 自动 reopen 为 open；其它情况 → pending
	if senderRole == TicketSenderRoleUser {
		// 状态值内联为整型字面量，不走 bind 参数：PostgreSQL 无法从 CASE 分支
		// 内的占位符推断类型，会默认成 text，再赋给 bigint 的 status 列即报
		// "column status is of type bigint but expression is of type text"。
		// 这些值均为内部 int 常量，直接拼接安全且跨三库兼容。
		updates["status"] = gorm.Expr(fmt.Sprintf(
			"CASE WHEN status IN (%d, %d) THEN %d ELSE %d END",
			TicketStatusResolved, TicketStatusClosed,
			TicketStatusOpen, TicketStatusPending,
		))
		updates["closed_at"] = 0
	} else if senderRole == TicketSenderRoleAdmin {
		// 管理员回复 → pending（等用户回应）；不主动 close
		updates["status"] = TicketStatusPending
	}
	return tx.Model(&Ticket{}).Where("id = ?", ticketId).Updates(updates).Error
}

// UpdateTicketStatus 显式修改状态。close 时刷 closed_at。
func UpdateTicketStatus(ticketId int, status int) error {
	now := common.GetTimestamp()
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": now,
	}
	if status == TicketStatusClosed {
		updates["closed_at"] = now
	} else if status == TicketStatusOpen || status == TicketStatusPending {
		updates["closed_at"] = 0
	}
	return DB.Model(&Ticket{}).Where("id = ?", ticketId).Updates(updates).Error
}

// AdminUpdateTicket 管理员修改 priority / category 等可变字段。
// 注意：type / user_id 永远不可变。
type AdminTicketUpdate struct {
	Status   *int
	Priority *int
	Category *string
}

func AdminUpdateTicket(ticketId int, u AdminTicketUpdate) error {
	updates := map[string]interface{}{"updated_at": common.GetTimestamp()}
	if u.Status != nil {
		updates["status"] = *u.Status
		if *u.Status == TicketStatusClosed {
			updates["closed_at"] = common.GetTimestamp()
		} else if *u.Status == TicketStatusOpen || *u.Status == TicketStatusPending {
			updates["closed_at"] = 0
		}
	}
	if u.Priority != nil {
		updates["priority"] = *u.Priority
	}
	if u.Category != nil {
		updates["category"] = *u.Category
	}
	if len(updates) == 1 {
		// 只有 updated_at 等于没改任何业务字段
		return nil
	}
	return DB.Model(&Ticket{}).Where("id = ?", ticketId).Updates(updates).Error
}

// AutoCloseInactiveTickets 后台任务调用：把超过 inactiveDays 天没人回复且
// 非 closed 的工单转为 resolved。返回处理数量。
func AutoCloseInactiveTickets(inactiveDays int) (int64, error) {
	if inactiveDays <= 0 {
		return 0, nil
	}
	threshold := time.Now().Unix() - int64(inactiveDays)*86400
	res := DB.Model(&Ticket{}).
		Where("status IN ? AND last_reply_at < ?",
			[]int{TicketStatusOpen, TicketStatusPending},
			threshold).
		Updates(map[string]interface{}{
			"status":     TicketStatusResolved,
			"updated_at": common.GetTimestamp(),
		})
	return res.RowsAffected, res.Error
}

// escapeLike 把 LIKE 的特殊字符（% _ !）转义掉，配合 ESCAPE '!' 使用。
// 与现有 sanitizeLikePattern 同思路，但更简单——只对 keyword 用。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "!", "!!")
	s = strings.ReplaceAll(s, "%", "!%")
	s = strings.ReplaceAll(s, "_", "!_")
	return s
}
