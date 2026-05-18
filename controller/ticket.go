package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/ticket"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ----- 用户侧接口 -----

// GetTicketSettingForUser 暴露用户在前端建单页需要的配置：
// 总开关、上限、分类白名单、附件限制。不暴露 telegram/email 等运维字段。
func GetTicketSettingForUser(c *gin.Context) {
	st := operation_setting.GetTicketSetting()
	common.ApiSuccess(c, gin.H{
		"enabled":                     st.Enabled,
		"max_title_length":            st.MaxTitleLength,
		"max_content_length":          st.MaxContentLength,
		"max_attachments_per_message": st.MaxAttachmentsPerMessage,
		"require_verified_email":      st.RequireVerifiedEmail,
		"categories": gin.H{
			"support":  []string{"model_invocation", "channel_issue", "billing", "account", "abuse", "refund", "other"},
			"feedback": []string{"feature", "ux", "docs", "other"},
		},
	})
}

// CreateTicket POST /api/ticket
func CreateTicket(c *gin.Context) {
	userId := c.GetInt("id")
	var req dto.CreateTicketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	t, _, err := ticket.CreateTicket(userId, &req)
	if err != nil {
		emitTicketError(c, err)
		return
	}
	go ticket.NotifyTicketCreated(t)
	common.ApiSuccess(c, gin.H{"id": t.Id})
}

// ListUserTickets GET /api/ticket
func ListUserTickets(c *gin.Context) {
	userId := c.GetInt("id")
	f := buildUserListFilter(c, userId)
	list, total, err := model.ListTickets(f)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]dto.TicketListItem, 0, len(list))
	for _, t := range list {
		items = append(items, ticket.BuildListView(t))
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      f.Page,
		"page_size": f.PageSize,
	})
}

// GetUserTicketDetail GET /api/ticket/:id
func GetUserTicketDetail(c *gin.Context) {
	userId := c.GetInt("id")
	ticketId, ok := parseIdParam(c)
	if !ok {
		return
	}
	t, err := model.GetTicketById(ticketId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, "ticket.not_found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if t.UserId != userId {
		common.ApiErrorI18n(c, "ticket.permission_denied")
		return
	}
	msgs, err := model.ListTicketMessages(ticketId, 200, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := ticket.BuildDetailView(t, msgs, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 用户读取详情即标记为"已查看"，驱动未读红点清零。异步更新，不阻塞响应。
	go model.MarkTicketSeenByUser(ticketId, userId)
	common.ApiSuccess(c, view)
}

// ReplyToTicket POST /api/ticket/:id/reply
func ReplyToTicket(c *gin.Context) {
	userId := c.GetInt("id")
	ticketId, ok := parseIdParam(c)
	if !ok {
		return
	}
	var req dto.ReplyTicketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.IsInternal = false // 用户侧绝不允许 internal
	_, t, err := ticket.ReplyTicket(ticketId, userId, model.TicketSenderRoleUser, &req)
	if err != nil {
		emitTicketError(c, err)
		return
	}
	go ticket.NotifyTicketReplied(t, model.TicketSenderRoleUser)
	common.ApiSuccess(c, nil)
}

// CloseUserTicket PUT /api/ticket/:id/close
func CloseUserTicket(c *gin.Context) {
	userId := c.GetInt("id")
	ticketId, ok := parseIdParam(c)
	if !ok {
		return
	}
	if err := ticket.CloseTicket(ticketId, userId, model.TicketSenderRoleUser); err != nil {
		emitTicketError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ReopenUserTicket PUT /api/ticket/:id/reopen
func ReopenUserTicket(c *gin.Context) {
	userId := c.GetInt("id")
	ticketId, ok := parseIdParam(c)
	if !ok {
		return
	}
	if err := ticket.ReopenTicket(ticketId, userId, model.TicketSenderRoleUser); err != nil {
		emitTicketError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// GetUserTicketUnreadCount GET /api/ticket/unread —— 用于前端小红点。
// 定义：管理员最新回复时间 > 用户最后查看时间。用户点进详情会自动清零。
func GetUserTicketUnreadCount(c *gin.Context) {
	userId := c.GetInt("id")
	n, err := model.CountUserUnseenTickets(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"count": n})
}

// ----- 辅助 -----

func parseIdParam(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, "ticket.invalid_id")
		return 0, false
	}
	return id, true
}

func buildUserListFilter(c *gin.Context, userId int) model.TicketListFilter {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	f := model.TicketListFilter{
		UserId:   userId,
		Type:     c.Query("type"),
		Category: c.Query("category"),
		Page:     page,
		PageSize: pageSize,
	}
	if s := c.Query("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			f.Status = &v
		}
	}
	if p := c.Query("priority"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 0 && v <= 3 {
			f.Priority = &v
		}
	}
	return f
}

// ticketErrorToI18nKey 把 service 层 sentinel error 映射成 i18n 资源 key。
// 配合 common.ApiErrorI18n 输出对应语言文案。无映射时返回空字符串，调用方
// 应退化到 err.Error()。
func ticketErrorToI18nKey(err error) string {
	switch {
	case errors.Is(err, ticket.ErrTicketDisabled):
		return "ticket.disabled"
	case errors.Is(err, ticket.ErrInvalidType):
		return "ticket.invalid_type"
	case errors.Is(err, ticket.ErrInvalidCategory):
		return "ticket.invalid_category"
	case errors.Is(err, ticket.ErrTitleTooLong):
		return "ticket.title_too_long"
	case errors.Is(err, ticket.ErrContentTooLong):
		return "ticket.content_too_long"
	case errors.Is(err, ticket.ErrTooManyAttachments):
		return "ticket.attachments_too_many"
	case errors.Is(err, ticket.ErrRateLimited):
		return "ticket.rate_limited"
	case errors.Is(err, ticket.ErrEmailNotVerified):
		return "ticket.email_not_verified"
	case errors.Is(err, ticket.ErrUserDisabled):
		return "ticket.user_disabled"
	case errors.Is(err, ticket.ErrPermissionDenied):
		return "ticket.permission_denied"
	case errors.Is(err, ticket.ErrTicketNotFound):
		return "ticket.not_found"
	case errors.Is(err, ticket.ErrInvalidPriority):
		return "ticket.invalid_priority"
	case errors.Is(err, ticket.ErrMetadataTooLarge):
		return "ticket.metadata_too_large"
	case errors.Is(err, ticket.ErrReopenTooLate):
		return "ticket.reopen_too_late"
	case errors.Is(err, ticket.ErrRefundContextInvalid):
		return "ticket.refund_context_invalid"
	case errors.Is(err, ticket.ErrRefundOtherReasonRequired):
		return "ticket.refund_other_reason_required"
	case errors.Is(err, ticket.ErrRefundOrderRequired):
		return "ticket.refund_order_required"
	case errors.Is(err, ticket.ErrRefundOrderTooMany):
		return "ticket.refund_order_too_many"
	case errors.Is(err, ticket.ErrRefundOrderNotFound):
		return "ticket.refund_order_not_found"
	}
	return ""
}

// emitTicketError 统一错误响应入口：有 i18n key 时走 ApiErrorI18n，
// 否则回退到原始 message。
func emitTicketError(c *gin.Context, err error) {
	if key := ticketErrorToI18nKey(err); key != "" {
		common.ApiErrorI18n(c, key)
		return
	}
	common.ApiError(c, err)
}
