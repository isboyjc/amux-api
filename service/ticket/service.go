package ticket

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

// 公共错误。controller 层根据需要包装成用户文案。
var (
	ErrTicketDisabled     = errors.New("ticket system disabled")
	ErrInvalidType        = errors.New("invalid ticket type")
	ErrInvalidCategory    = errors.New("invalid ticket category for the given type")
	ErrTitleTooLong       = errors.New("title too long")
	ErrContentTooLong     = errors.New("content too long")
	ErrTooManyAttachments = errors.New("too many attachments")
	ErrRateLimited        = errors.New("rate limited")
	ErrEmailNotVerified   = errors.New("verified email required")
	ErrUserDisabled       = errors.New("user disabled")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrTicketNotFound     = errors.New("ticket not found")
	ErrInvalidPriority    = errors.New("invalid priority value")
	ErrMetadataTooLarge   = errors.New("metadata payload too large")
	ErrReopenTooLate      = errors.New("ticket closed too long ago, please create a new one")
)

// metadata 列在 DB 里是 TEXT，全局再加一道兜底上限：8KB。
// 用户填的 error_excerpt 已被 2KB 截断；其余字段都是短字符串，正常情况下
// 远远到不了 8KB。如果触发这条说明上下文异常或被恶意注入。
const maxMetadataBytes = 8 * 1024

// CreateTicket 完成建单的全部业务校验、enrichment、落库与首条消息插入。
// 返回新建的工单与首条消息（system 消息预留 v2，这里只插 user 消息）。
func CreateTicket(userId int, req *dto.CreateTicketReq) (*model.Ticket, *model.TicketMessage, error) {
	st := operation_setting.GetTicketSetting()
	if !st.Enabled {
		return nil, nil, ErrTicketDisabled
	}

	if err := validateUserCanFile(userId, st); err != nil {
		return nil, nil, err
	}
	if err := validateTypeAndCategory(req.Type, req.Category); err != nil {
		return nil, nil, err
	}
	if err := validateLengths(req.Title, req.Content, st); err != nil {
		return nil, nil, err
	}
	if err := validateAttachments(req.Attachments, st); err != nil {
		return nil, nil, err
	}
	if err := enforceCreationRateLimit(userId, st); err != nil {
		return nil, nil, err
	}

	// enrichment：按分类挑选要落 metadata 的结构化字段。
	//   - refund：必须带 RefundContext，校验失败直接拒绝（金额/订单是法律凭据，
	//     不能像 bug_context 那样软失败）。
	//   - 其它分类：沿用 bug_context 软校验（request_id 错了仍允许建单）。
	meta := &dto.TicketMetadata{}
	if req.Category == "refund" {
		enrichedRefund, err := EnrichRefundContext(userId, req.RefundContext)
		if err != nil {
			return nil, nil, err
		}
		meta.RefundContext = enrichedRefund
	} else {
		enrichedCtx := EnrichBugContext(userId, req.BugContext)
		meta.BugContext = enrichedCtx
	}

	metaStr, err := SerializeMetadata(meta)
	if err != nil {
		return nil, nil, fmt.Errorf("serialize metadata: %w", err)
	}
	if len(metaStr) > maxMetadataBytes {
		return nil, nil, ErrMetadataTooLarge
	}
	attsStr, err := SerializeAttachments(req.Attachments)
	if err != nil {
		return nil, nil, fmt.Errorf("serialize attachments: %w", err)
	}

	now := common.GetTimestamp()
	// 默认优先级 = 普通；用户传了就用，但边界要闸住（v1 0~3）。最终决定权
	// 仍在管理员手里——AdminUpdateTicket 可随时改。
	priority := 1
	if req.Priority != nil {
		v := *req.Priority
		if v < 0 || v > 3 {
			return nil, nil, ErrInvalidPriority
		}
		priority = v
	}
	t := &model.Ticket{
		UserId:        userId,
		Type:          req.Type,
		Category:      req.Category,
		Title:         strings.TrimSpace(req.Title),
		Status:        model.TicketStatusOpen,
		Priority:      priority,
		LastReplyAt:   now,
		LastReplyRole: model.TicketSenderRoleUser,
		ReplyCount:    1, // 首条消息算 1 次回复
		// 用户本人在建单，自然算"已读"。这样未读判定 (last_reply_at > user_seen_at
		// && last_reply_role=admin) 才不会把刚建的单立即标成未读。
		UserSeenAt:  now,
		Attachments: attsStr,
		Metadata:    metaStr,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	// 平铺索引列从 enriched bug_context 拷贝过来（refund 工单 BugContext 为空，
	// 三个索引列保持默认零值即可）。
	if meta.BugContext != nil {
		t.ChannelId = meta.BugContext.ChannelId
		t.ModelName = meta.BugContext.Model
		t.Group = meta.BugContext.Group
	}

	firstMsg := &model.TicketMessage{
		SenderId:    userId,
		SenderRole:  model.TicketSenderRoleUser,
		Content:     req.Content,
		Attachments: attsStr,
		CreatedAt:   now,
	}

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		firstMsg.TicketId = t.Id
		return tx.Create(firstMsg).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return t, firstMsg, nil
}

// ReplyTicket 用户/管理员追加一条回复。事务里写消息 + 同步 ticket 计数与状态。
// senderRole 决定状态机走向（见 model.UpdateTicketAfterReply）。
// 管理员调用时 isInternal 字段在 v2 启用，v1 强制 false。
func ReplyTicket(ticketId, senderId, senderRole int, req *dto.ReplyTicketReq) (*model.TicketMessage, *model.Ticket, error) {
	st := operation_setting.GetTicketSetting()
	if !st.Enabled {
		return nil, nil, ErrTicketDisabled
	}

	if strings.TrimSpace(req.Content) == "" {
		return nil, nil, errors.New("content is empty")
	}
	if utf8.RuneCountInString(req.Content) > st.MaxContentLength {
		return nil, nil, ErrContentTooLong
	}
	if err := validateAttachments(req.Attachments, st); err != nil {
		return nil, nil, err
	}

	t, err := model.GetTicketById(ticketId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrTicketNotFound
		}
		return nil, nil, err
	}

	// 用户视角的权限：只能回自己的工单，并受频次限流约束。
	if senderRole == model.TicketSenderRoleUser {
		if t.UserId != senderId {
			return nil, nil, ErrPermissionDenied
		}
		if err := enforceReplyRateLimit(senderId, st); err != nil {
			return nil, nil, err
		}
	}

	attsStr, err := SerializeAttachments(req.Attachments)
	if err != nil {
		return nil, nil, fmt.Errorf("serialize attachments: %w", err)
	}

	now := common.GetTimestamp()
	msg := &model.TicketMessage{
		TicketId:    ticketId,
		SenderId:    senderId,
		SenderRole:  senderRole,
		Content:     req.Content,
		Attachments: attsStr,
		IsInternal:  false, // v1 不开放
		CreatedAt:   now,
	}

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.CreateTicketMessage(tx, msg); err != nil {
			return err
		}
		return model.UpdateTicketAfterReply(tx, ticketId, senderRole, now)
	})
	if err != nil {
		return nil, nil, err
	}

	// 重新读取最新工单，给调用方/通知层
	t2, err := model.GetTicketById(ticketId)
	if err != nil {
		return msg, t, nil // 退化为旧快照
	}
	return msg, t2, nil
}

// CloseTicket 用户或管理员主动关闭。
// 不强制非空回复 —— 但 controller 层应在必要时合并最后一条系统消息。
func CloseTicket(ticketId, actorId, actorRole int) error {
	t, err := model.GetTicketById(ticketId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTicketNotFound
		}
		return err
	}
	if actorRole == model.TicketSenderRoleUser && t.UserId != actorId {
		return ErrPermissionDenied
	}
	return model.UpdateTicketStatus(ticketId, model.TicketStatusClosed)
}

// ReopenTicket 用户主动 reopen。已 closed 7 天内可重开（手动 close 限制），
// 已 resolved 30 天内可重开。超期返回错误。
//
// 管理员侧总是允许 reopen，没有时限限制。
func ReopenTicket(ticketId, actorId, actorRole int) error {
	t, err := model.GetTicketById(ticketId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTicketNotFound
		}
		return err
	}
	if actorRole == model.TicketSenderRoleUser {
		if t.UserId != actorId {
			return ErrPermissionDenied
		}
		st := operation_setting.GetTicketSetting()
		now := common.GetTimestamp()
		switch t.Status {
		case model.TicketStatusClosed:
			if d := st.ReopenAfterClosedDays; d > 0 {
				limit := int64(d) * 86400
				if t.ClosedAt > 0 && now-t.ClosedAt > limit {
					return ErrReopenTooLate
				}
			}
		case model.TicketStatusResolved:
			if d := st.ReopenAfterResolvedDays; d > 0 {
				limit := int64(d) * 86400
				if t.UpdatedAt > 0 && now-t.UpdatedAt > limit {
					return ErrReopenTooLate
				}
			}
		case model.TicketStatusOpen, model.TicketStatusPending:
			return nil // 本就开着
		}
	}
	return model.UpdateTicketStatus(ticketId, model.TicketStatusOpen)
}

// ----- 内部校验函数 -----

func validateTypeAndCategory(t, c string) error {
	if !model.IsValidTicketType(t) {
		return ErrInvalidType
	}
	if !model.IsValidTicketCategory(t, c) {
		return ErrInvalidCategory
	}
	return nil
}

func validateLengths(title, content string, st *operation_setting.TicketSetting) error {
	if utf8.RuneCountInString(strings.TrimSpace(title)) == 0 {
		return errors.New("title is empty")
	}
	if utf8.RuneCountInString(title) > st.MaxTitleLength {
		return ErrTitleTooLong
	}
	if utf8.RuneCountInString(content) > st.MaxContentLength {
		return ErrContentTooLong
	}
	return nil
}

func validateAttachments(atts []dto.TicketAttachment, st *operation_setting.TicketSetting) error {
	if st.MaxAttachmentsPerMessage > 0 && len(atts) > st.MaxAttachmentsPerMessage {
		return ErrTooManyAttachments
	}
	for _, a := range atts {
		if strings.TrimSpace(a.URL) == "" {
			return errors.New("attachment url is empty")
		}
	}
	return nil
}

func validateUserCanFile(userId int, st *operation_setting.TicketSetting) error {
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return err
	}
	// 被禁用用户永远不能建单（即便配置没要求邮箱验证）。
	if user.Status != common.UserStatusEnabled {
		return ErrUserDisabled
	}
	if st.RequireVerifiedEmail && strings.TrimSpace(user.Email) == "" {
		return ErrEmailNotVerified
	}
	return nil
}

func enforceCreationRateLimit(userId int, st *operation_setting.TicketSetting) error {
	now := common.GetTimestamp()
	if st.UserHourlyLimit > 0 {
		n, err := model.CountUserTicketsSince(userId, now-3600)
		if err == nil && int(n) >= st.UserHourlyLimit {
			return ErrRateLimited
		}
	}
	if st.UserDailyLimit > 0 {
		n, err := model.CountUserTicketsSince(userId, now-86400)
		if err == nil && int(n) >= st.UserDailyLimit {
			return ErrRateLimited
		}
	}
	return nil
}

func enforceReplyRateLimit(userId int, st *operation_setting.TicketSetting) error {
	if st.UserReplyPerMinute <= 0 {
		return nil
	}
	since := common.GetTimestamp() - 60
	n, err := model.CountUserRepliesSince(userId, since)
	if err == nil && int(n) >= st.UserReplyPerMinute {
		return ErrRateLimited
	}
	return nil
}

// RunAutoResolveOnce 后台任务入口，定时调用。
func RunAutoResolveOnce() {
	st := operation_setting.GetTicketSetting()
	if !st.Enabled || st.AutoResolveDays <= 0 {
		return
	}
	n, err := model.AutoCloseInactiveTickets(st.AutoResolveDays)
	if err != nil {
		common.SysLog("ticket auto-resolve error: " + err.Error())
		return
	}
	if n > 0 {
		common.SysLog(fmt.Sprintf("ticket auto-resolve: %d ticket(s) marked resolved", n))
	}
}

// BuildDetailView 把 model.Ticket + 消息流 + metadata 拼成对外 DTO。
// includeInternal=true 时（管理员侧）保留 is_internal=true 的消息，
// 否则过滤掉。
func BuildDetailView(t *model.Ticket, messages []*model.TicketMessage, includeInternal bool) (*dto.TicketDetailView, error) {
	meta, err := DeserializeMetadata(t.Metadata)
	if err != nil {
		return nil, err
	}
	atts, err := DeserializeAttachments(t.Attachments)
	if err != nil {
		return nil, err
	}

	out := &dto.TicketDetailView{
		Id:            t.Id,
		UserId:        t.UserId,
		Type:          t.Type,
		Category:      t.Category,
		Title:         t.Title,
		Status:        t.Status,
		Priority:      t.Priority,
		AssigneeId:    t.AssigneeId,
		LastReplyAt:   t.LastReplyAt,
		LastReplyRole: t.LastReplyRole,
		ReplyCount:    t.ReplyCount,
		ChannelId:     t.ChannelId,
		ModelName:     t.ModelName,
		Group:         t.Group,
		Attachments:   atts,
		Metadata:      meta,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
		ClosedAt:      t.ClosedAt,
	}
	if meta != nil && meta.BugContext != nil {
		out.ChannelName = meta.BugContext.ChannelName
	}

	for _, m := range messages {
		if m.IsInternal && !includeInternal {
			continue
		}
		mAtts, _ := DeserializeAttachments(m.Attachments)
		out.Messages = append(out.Messages, dto.TicketMessageView{
			Id:          m.Id,
			TicketId:    m.TicketId,
			SenderId:    m.SenderId,
			SenderRole:  m.SenderRole,
			Content:     m.Content,
			Attachments: mAtts,
			IsInternal:  m.IsInternal,
			CreatedAt:   m.CreatedAt,
		})
	}
	return out, nil
}

// BuildListView 列表项视图。
func BuildListView(t *model.Ticket) dto.TicketListItem {
	return dto.TicketListItem{
		Id:            t.Id,
		UserId:        t.UserId,
		Type:          t.Type,
		Category:      t.Category,
		Title:         t.Title,
		Status:        t.Status,
		Priority:      t.Priority,
		LastReplyAt:   t.LastReplyAt,
		LastReplyRole: t.LastReplyRole,
		ReplyCount:    t.ReplyCount,
		ChannelId:     t.ChannelId,
		ModelName:     t.ModelName,
		Group:         t.Group,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

