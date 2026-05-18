package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/ticket"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ----- 管理员侧接口（middleware.AdminAuth 保护） -----

// AdminListTickets GET /api/ticket/admin
// 过滤参数：type / category / status / channel_id / model_name / group / keyword / user_id
// / start_time / end_time / page / page_size。
func AdminListTickets(c *gin.Context) {
	f := buildAdminListFilter(c)
	list, total, err := model.ListTickets(f)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]dto.TicketListItem, 0, len(list))
	// 一次性查 username 填充，避免 N+1
	userIds := make(map[int]struct{})
	for _, t := range list {
		userIds[t.UserId] = struct{}{}
	}
	usernameMap := bulkLookupUsernames(userIds)
	for _, t := range list {
		v := ticket.BuildListView(t)
		v.Username = usernameMap[t.UserId]
		items = append(items, v)
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      f.Page,
		"page_size": f.PageSize,
	})
}

// AdminGetTicketDetail GET /api/ticket/admin/:id —— 包含 internal 消息。
func AdminGetTicketDetail(c *gin.Context) {
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
	msgs, err := model.ListTicketMessages(ticketId, 500, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := ticket.BuildDetailView(t, msgs, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 管理员页填充用户名 + 各发送者的展示名（v1 简化：只填工单作者）
	if user, err := model.GetUserById(t.UserId, false); err == nil && user != nil {
		view.Username = user.Username
	}
	common.ApiSuccess(c, view)
}

// AdminReplyTicket POST /api/ticket/admin/:id/reply
func AdminReplyTicket(c *gin.Context) {
	adminId := c.GetInt("id")
	ticketId, ok := parseIdParam(c)
	if !ok {
		return
	}
	var req dto.ReplyTicketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.IsInternal = false // v1 不开放
	_, t, err := ticket.ReplyTicket(ticketId, adminId, model.TicketSenderRoleAdmin, &req)
	if err != nil {
		emitTicketError(c, err)
		return
	}
	go ticket.NotifyTicketReplied(t, model.TicketSenderRoleAdmin)
	common.ApiSuccess(c, nil)
}

// AdminUpdateTicket PUT /api/admin/ticket/:id
func AdminUpdateTicket(c *gin.Context) {
	ticketId, ok := parseIdParam(c)
	if !ok {
		return
	}
	var req dto.AdminUpdateTicketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	// status 必须落在已知枚举内。
	if req.Status != nil {
		switch *req.Status {
		case model.TicketStatusOpen, model.TicketStatusPending,
			model.TicketStatusResolved, model.TicketStatusClosed:
		default:
			common.ApiErrorI18n(c, "ticket.invalid_status")
			return
		}
	}
	// priority 边界：[0,3]。v1 还没真实排序消费，但闸住后置字段不会被任意污染。
	if req.Priority != nil && (*req.Priority < 0 || *req.Priority > 3) {
		common.ApiErrorI18n(c, "ticket.invalid_priority")
		return
	}
	// category 修改时再做白名单校验，避免管理员把分类改成无效值。
	if req.Category != nil {
		t, err := model.GetTicketById(ticketId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if !model.IsValidTicketCategory(t.Type, *req.Category) {
			common.ApiErrorI18n(c, "ticket.invalid_category")
			return
		}
	}
	if err := model.AdminUpdateTicket(ticketId, model.AdminTicketUpdate{
		Status:   req.Status,
		Priority: req.Priority,
		Category: req.Category,
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminTicketStats GET /api/admin/ticket/stats —— 仪表盘卡片。
// pending 与各状态计数走一次 GROUP BY，避免多次 COUNT(*)。
func AdminTicketStats(c *gin.Context) {
	stats, err := model.GetTicketStatusStats()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pending, err := model.CountAdminPendingTickets()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dto.TicketStatsView{
		Pending:  pending,
		Open:     stats[model.TicketStatusOpen] + stats[model.TicketStatusPending],
		Resolved: stats[model.TicketStatusResolved],
		Closed:   stats[model.TicketStatusClosed],
	})
}

// ----- 辅助 -----

func buildAdminListFilter(c *gin.Context) model.TicketListFilter {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	userIdQ, _ := strconv.Atoi(c.Query("user_id"))
	channelIdQ, _ := strconv.Atoi(c.Query("channel_id"))
	start, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	f := model.TicketListFilter{
		UserId:    userIdQ,
		Type:      c.Query("type"),
		Category:  c.Query("category"),
		ChannelId: channelIdQ,
		ModelName: c.Query("model_name"),
		Group:     c.Query("group"),
		Keyword:   c.Query("keyword"),
		StartTime: start,
		EndTime:   end,
		Page:      page,
		PageSize:  pageSize,
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

// bulkLookupUsernames 批量查 username，避免列表里逐条 GetUserById。
// 失败/缺失静默退化为空字符串。
func bulkLookupUsernames(userIds map[int]struct{}) map[int]string {
	out := make(map[int]string, len(userIds))
	if len(userIds) == 0 {
		return out
	}
	ids := make([]int, 0, len(userIds))
	for id := range userIds {
		ids = append(ids, id)
	}
	var rows []struct {
		Id       int    `gorm:"column:id"`
		Username string `gorm:"column:username"`
	}
	if err := model.DB.Table("users").Select("id, username").Where("id IN ?", ids).Find(&rows).Error; err == nil {
		for _, r := range rows {
			out[r.Id] = r.Username
		}
	}
	return out
}
