package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// 盲盒在用户点击"开盒"之前必须是不透明的：一旦把 BlindBoxWinner 原样吐给前端，
// 用户打开控制台就能提前看到 prize_name / prize_quota，"盲"盒就不成立了。
// 因此 pending 状态只暴露"有一个盒子、什么时候过期"，奖品内容在开盒响应里才给。

// blindBoxPendingView 待开启盲盒的对外视图（不含任何奖品信息）。
type blindBoxPendingView struct {
	DrawDate string `json:"draw_date"`
	ExpireAt int64  `json:"expire_at"`
}

// blindBoxRecentRecordCount 用户卡片默认展示的参与记录条数，更多走分页接口。
const blindBoxRecentRecordCount = 5

// 参与记录的对外状态。前三种沿用中奖记录的状态机，后两种是「没中奖」的两种成因。
const (
	blindBoxRecordLost    = "lost"    // 已开奖，未抽中
	blindBoxRecordWaiting = "waiting" // 已报名，本期尚未开奖
)

// blindBoxRecordView 一条「参与记录」= 一次报名 + 该期的结果。
// pending 记录的奖品字段被抹掉，保持盲盒不透明。
type blindBoxRecordView struct {
	DrawDate   string `json:"draw_date"`
	Status     string `json:"status"`
	PrizeName  string `json:"prize_name"`
	PrizeQuota int    `json:"prize_quota"`
	ExpireAt   int64  `json:"expire_at"`
	ClaimedAt  int64  `json:"claimed_at"`
	JoinedAt   int64  `json:"joined_at"`
}

// buildBlindBoxRecords 把报名记录与中奖结果合并成用户可见的参与历史。
func buildBlindBoxRecords(userId int, entries []model.BlindBoxEntry) ([]blindBoxRecordView, error) {
	dates := make([]string, 0, len(entries))
	for _, e := range entries {
		dates = append(dates, e.DrawDate)
	}
	winners, err := model.GetUserBlindBoxWinnersByDates(userId, dates)
	if err != nil {
		return nil, err
	}
	settled, err := model.GetSettledBlindBoxDates(dates)
	if err != nil {
		return nil, err
	}

	views := make([]blindBoxRecordView, 0, len(entries))
	for _, e := range entries {
		v := blindBoxRecordView{DrawDate: e.DrawDate, JoinedAt: e.CreatedAt}
		if w, ok := winners[e.DrawDate]; ok {
			v.Status = w.Status
			v.ExpireAt = w.ExpireAt
			v.ClaimedAt = w.ClaimedAt
			// 已开启 / 已过期才揭晓奖品；待开启的必须保持不透明
			if w.Status != model.BlindBoxStatusPending {
				v.PrizeName = w.PrizeName
				v.PrizeQuota = w.PrizeQuota
			}
		} else if _, done := settled[e.DrawDate]; done {
			v.Status = blindBoxRecordLost
		} else {
			v.Status = blindBoxRecordWaiting
		}
		views = append(views, v)
	}
	return views, nil
}

// GetBlindBoxStatus 返回用户盲盒状态：本期消耗进度、是否达标、可开启的盲盒、历史记录。
func GetBlindBoxStatus(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "盲盒功能未启用")
		return
	}
	userId := c.GetInt("id")

	hour, minute, ok := setting.ParseDrawTime()
	if !ok {
		hour, minute = 0, 0
	}
	now := time.Now()
	// 当前进行中的周期从最近一个开奖边界开始。
	boundary := model.MostRecentDrawBoundary(now, hour, minute)
	periodStart := boundary.Unix()
	nextDrawAt := model.NextDrawBoundary(boundary, hour, minute).Unix()
	currentDrawDate := model.CurrentBlindBoxDrawDate(now, hour, minute)

	consume, err := model.GetUserBlindBoxConsume(userId, periodStart, now.Unix())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	joined, _ := model.HasBlindBoxEntry(userId, currentDrawDate)
	totalWon, _ := model.SumUserBlindBoxClaimedQuota(userId)

	// 卡片默认只展示最近 5 条，更多走 /blindbox/records 分页拉取
	entries, recordsTotal, _ := model.GetUserBlindBoxEntries(userId, 0, blindBoxRecentRecordCount)
	records, _ := buildBlindBoxRecords(userId, entries)
	pending, _ := model.GetUserPendingBlindBox(userId, now.Unix())
	var pendingView *blindBoxPendingView
	if pending != nil {
		pendingView = &blindBoxPendingView{DrawDate: pending.DrawDate, ExpireAt: pending.ExpireAt}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":         setting.Enabled,
			"draw_time":       setting.DrawTime,
			"pool_mode":       setting.PoolMode,
			"threshold_quota": setting.ThresholdQuota,
			"current_consume": consume,
			"qualified":       consume >= int64(setting.ThresholdQuota),
			"period_start":    periodStart,
			"next_draw_at":    nextDrawAt,
			// 报名要求消耗已达标，所以 joined 成立即代表已入池
			"current_draw_date": currentDrawDate,
			"joined":            joined,
			"pending":           pendingView,
			"total_won_quota":   totalWon,
			"records":           records,
			"records_total":     recordsTotal,
		},
	})
}

// DoBlindBoxEnter 用户参与当前进行中的这一期抽奖。
//
// 两道前置条件，都必须在服务端校验（前端只是把按钮置灰，直接打接口能绕过）：
//  1. 本期消耗必须已达标 —— 参与即入池，不存在"先报名再慢慢花"的中间态；
//  2. 不能有尚未开启的盲盒 —— 必须先把上一期的盒子开掉才能参与下一期，
//     否则奖品会在手上堆积，"开盒"这个动作也就失去意义了。
func DoBlindBoxEnter(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "盲盒功能未启用")
		return
	}
	userId := c.GetInt("id")
	username := c.GetString("username")

	hour, minute, ok := setting.ParseDrawTime()
	if !ok {
		hour, minute = 0, 0
	}
	now := time.Now()
	boundary := model.MostRecentDrawBoundary(now, hour, minute)
	drawDate := model.CurrentBlindBoxDrawDate(now, hour, minute)

	// 查询失败必须拒绝而不是放行：查不出来就等于无法确认"手上没有未开的盒子"，
	// 放行会让用户在持有盲盒时混进新一期。
	pending, err := model.GetUserPendingBlindBox(userId, now.Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if pending != nil {
		common.ApiErrorMsg(c, "请先开启已有的盲盒，再参与新一期")
		return
	}

	consume, err := model.GetUserBlindBoxConsume(userId, boundary.Unix(), now.Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if consume < int64(setting.ThresholdQuota) {
		common.ApiErrorMsg(c, "本期消耗未达到参与门槛")
		return
	}

	created, err := model.EnterBlindBoxDraw(userId, username, drawDate, now.Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		common.ApiErrorMsg(c, "你已参与本期抽奖")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已参与本期抽奖，等待开奖",
		"data":    gin.H{"draw_date": drawDate},
	})
}

// GetBlindBoxRecords 分页返回用户的参与记录（中奖与未中奖都在内）。
func GetBlindBoxRecords(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "盲盒功能未启用")
		return
	}
	userId := c.GetInt("id")

	// GetPageQuery 只把上界夹到 100，负数会原样透传；GORM 的 Limit(负数) 等于「不限制」，
	// 那样一次会把用户全部参与记录取出来，后面 IN 子句的期次数量也跟着失控。
	pageInfo := common.GetPageQuery(c)
	pageSize := pageInfo.GetPageSize()
	if pageSize <= 0 {
		pageSize = blindBoxRecentRecordCount
	}
	entries, total, err := model.GetUserBlindBoxEntries(userId,
		pageInfo.GetStartIdx(), pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	records, err := buildBlindBoxRecords(userId, entries)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}

// ===================== 管理端接口 =====================
//
// 查询接口全部只读。唯一的写入口是「中奖资格屏蔽」：它不碰 blind_box_draws / winners，
// 只在抽样前收窄候选集，因此不影响结算的幂等性（幂等仍由 draw_date 唯一索引保证）。
// 手动开奖 / 补发 / 改判历史中奖记录依旧不提供 —— 那类操作会和定时任务抢同一把逻辑锁。

// blindBoxPeriodWindow 当前进行中周期的时间窗口。
type blindBoxPeriodWindow struct {
	drawDate    string // 本期将要开奖的期号
	periodStart int64  // 周期起点（最近一个已越过的开奖时点）
	nextDrawAt  int64  // 本期开奖时点
	now         int64
}

// currentBlindBoxPeriod 计算进行中周期的窗口，口径与定时任务一致。
func currentBlindBoxPeriod(setting *operation_setting.BlindBoxSetting) blindBoxPeriodWindow {
	hour, minute, ok := setting.ParseDrawTime()
	if !ok {
		hour, minute = 0, 0
	}
	now := time.Now()
	boundary := model.MostRecentDrawBoundary(now, hour, minute)
	return blindBoxPeriodWindow{
		drawDate:    model.CurrentBlindBoxDrawDate(now, hour, minute),
		periodStart: boundary.Unix(),
		nextDrawAt:  model.NextDrawBoundary(boundary, hour, minute).Unix(),
		now:         now.Unix(),
	}
}

// AdminGetBlindBoxSummary 返回当前生效规则 + 本期实时预览 + 全局统计 + 健康标志。
//
// 规则一律读内存中的 setting 实例而非 options 表：那才是定时任务真正参与结算的值，
// 两者若因写库失败而不一致，管理员看到的必须是"实际生效的"那份。
func AdminGetBlindBoxSummary(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()
	win := currentBlindBoxPeriod(setting)

	// 本期实时预览：按当前配置推演今天的奖池，方便开奖前调整参数。
	// 与「当期明细」共用同一个推演函数，预览与实际开奖口径不会漂移。
	detail, previewErr := model.GetBlindBoxPeriodDetail(setting, win.drawDate, win.periodStart, win.now)
	if detail == nil {
		detail = &model.BlindBoxPeriodDetail{}
	}

	overview, err := model.GetBlindBoxOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"setting": gin.H{
			"enabled":         setting.Enabled,
			"draw_time":       setting.DrawTime,
			"threshold_quota": setting.ThresholdQuota,
			"pool_mode":       setting.PoolMode,
			"prizes":          setting.GetPrizes(),
			"percent_rate":    setting.PercentRate,
			"percent_count":   setting.PercentCount,
		},
		"period": gin.H{
			"period_start":      win.periodStart,
			"next_draw_at":      win.nextDrawAt,
			"current_draw_date": win.drawDate,
		},
		"preview": gin.H{
			// 消费日志关闭时聚合恒为空，前端据 log_consume_enabled 提示，不要误读成"今天没人消费"
			"available": previewErr == nil,
			// entry_count 是报名总数，participant_count 是报名且已达标且未被屏蔽（真正入池）的人数
			"entry_count":       detail.EntryCount,
			"qualified_count":   detail.QualifiedCount,
			"blocked_count":     detail.BlockedCount,
			"participant_count": detail.EffectiveCount,
			"total_consume":     detail.TotalConsume,
			"blocked_consume":   detail.BlockedConsume,
			"prize_count":       detail.PrizeCount,
			"winner_count":      detail.WinnerCount,
			"total_prize_quota": detail.TotalPrizeQuota,
			"prizes":            detail.Prizes,
		},
		"overview": overview,
		"health": gin.H{
			// 达标判定读 logs 表，消费日志关闭时盲盒永远无人达标
			"log_consume_enabled": common.LogConsumeEnabled,
			// 开奖任务仅在主节点运行
			"is_master_node": common.IsMasterNode,
		},
	})
}

// AdminGetBlindBoxDraws 分页返回开奖周期列表，并附带每期的领取情况聚合。
func AdminGetBlindBoxDraws(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	draws, total, err := model.GetBlindBoxDraws(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	dates := make([]string, 0, len(draws))
	for _, d := range draws {
		dates = append(dates, d.DrawDate)
	}
	stats, err := model.GetBlindBoxClaimStats(dates)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]gin.H, 0, len(draws))
	for _, d := range draws {
		s := stats[d.DrawDate]
		if s == nil {
			s = &model.BlindBoxClaimStat{}
		}
		items = append(items, gin.H{
			"id":                  d.Id,
			"draw_date":           d.DrawDate,
			"period_start":        d.PeriodStart,
			"period_end":          d.PeriodEnd,
			"entry_count":         d.EntryCount,
			"participant_count":   d.ParticipantCount,
			"blocked_count":       d.BlockedCount,
			"winner_count":        d.WinnerCount,
			"total_consume_quota": d.TotalConsumeQuota,
			"total_prize_quota":   d.TotalPrizeQuota,
			"pool_mode":           d.PoolMode,
			"status":              d.Status,
			"created_at":          d.CreatedAt,
			"claim_stat":          s,
		})
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// AdminGetBlindBoxWinners 分页返回中奖明细，支持按期次 / 状态 / 用户筛选。
func AdminGetBlindBoxWinners(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	winners, total, err := model.SearchBlindBoxWinners(
		strings.TrimSpace(c.Query("draw_date")),
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("keyword")),
		pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(winners)
	common.ApiSuccess(c, pageInfo)
}

// AdminGetBlindBoxCurrentPeriod 返回「当期（尚未开奖）」的实时推演：周期窗口、入池统计、
// 奖池预览，以及分页后的参与用户明细（本日消耗 + 中奖概率 + 屏蔽状态）。
//
// 概率、奖池、入池人数全部由 model.GetBlindBoxPeriodDetail 一次算出，与结算同源；
// keyword 只在分页前对列表做过滤，不参与概率计算——否则搜一个人就会看到被放大的概率。
func AdminGetBlindBoxCurrentPeriod(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()
	win := currentBlindBoxPeriod(setting)

	detail, err := model.GetBlindBoxPeriodDetail(setting, win.drawDate, win.periodStart, win.now)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := detail.Participants
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		filtered := make([]model.BlindBoxPeriodParticipant, 0, len(items))
		for _, p := range items {
			if strconv.Itoa(p.UserId) == keyword ||
				strings.Contains(strings.ToLower(p.Username), strings.ToLower(keyword)) {
				filtered = append(filtered, p)
			}
		}
		items = filtered
	}

	// 明细是两个库拼出来的，只能内存分页。GetPageQuery 只夹上界，page_size 传负数会
	// 原样透传，切片下标必须自己兜底，否则一个构造的请求就能把接口打 panic。
	page, size := blindBoxPageWindow(c)
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}

	common.ApiSuccess(c, gin.H{
		"period": gin.H{
			"draw_date":    win.drawDate,
			"period_start": win.periodStart,
			"next_draw_at": win.nextDrawAt,
			"now":          win.now,
		},
		"stat": gin.H{
			"entry_count":       detail.EntryCount,
			"qualified_count":   detail.QualifiedCount,
			"blocked_count":     detail.BlockedCount,
			"participant_count": detail.EffectiveCount,
			"total_consume":     detail.TotalConsume,
			"blocked_consume":   detail.BlockedConsume,
			"prize_count":       detail.PrizeCount,
			"winner_count":      detail.WinnerCount,
			"total_prize_quota": detail.TotalPrizeQuota,
			"prizes":            detail.Prizes,
		},
		"participants": gin.H{
			"page":      page,
			"page_size": size,
			"total":     len(items),
			"items":     items[start:end],
		},
		"health": gin.H{
			"log_consume_enabled": common.LogConsumeEnabled,
			"is_master_node":      common.IsMasterNode,
		},
	})
}

// blindBoxPageWindow 返回夹紧后的页码与页大小。GetPageQuery 不夹下界，内存分页必须自己兜底。
func blindBoxPageWindow(c *gin.Context) (page, size int) {
	pageInfo := common.GetPageQuery(c)
	page, size = pageInfo.GetPage(), pageInfo.GetPageSize()
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = common.ItemsPerPage
	}
	return page, size
}

// blindBoxBlockRequest 新增屏蔽的请求体。
type blindBoxBlockRequest struct {
	UserId   int    `json:"user_id"`
	Scope    string `json:"scope"`     // period | permanent
	DrawDate string `json:"draw_date"` // scope=period 时可选，缺省为当期
	Reason   string `json:"reason"`
}

// AdminCreateBlindBoxBlock 屏蔽某个用户的中奖资格。
//
// 只影响"能不能中奖"，不影响报名、消耗统计与其它任何功能，因此这里不做任何
// 用户状态改动，只写一条屏蔽记录。已结算的期次一律拒绝：那等于事后改判中奖结果。
func AdminCreateBlindBoxBlock(c *gin.Context) {
	var req blindBoxBlockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "请求参数解析失败")
		return
	}
	if req.UserId <= 0 {
		common.ApiErrorMsg(c, "用户 ID 不合法")
		return
	}

	setting := operation_setting.GetBlindBoxSetting()
	block := &model.BlindBoxBlock{
		UserId:    req.UserId,
		Reason:    strings.TrimSpace(req.Reason),
		CreatedAt: common.GetTimestamp(),
	}

	switch strings.TrimSpace(req.Scope) {
	case model.BlindBoxBlockScopePermanent:
		block.Scope = model.BlindBoxBlockScopePermanent
		block.DrawDate = "" // 永久屏蔽用空串占位，见 model.BlindBoxBlock 注释
	case model.BlindBoxBlockScopePeriod, "":
		block.Scope = model.BlindBoxBlockScopePeriod
		drawDate := strings.TrimSpace(req.DrawDate)
		if drawDate == "" {
			drawDate = currentBlindBoxPeriod(setting).drawDate
		}
		// 期号必须是合法日期：格式错的期号永远匹配不上任何一期，屏蔽会静默失效，
		// 管理员却在名单里看到一条"已屏蔽"，是最难排查的那类问题。
		if _, err := time.Parse("2006-01-02", drawDate); err != nil {
			common.ApiErrorMsg(c, "期号格式必须为 YYYY-MM-DD")
			return
		}
		settled, err := model.GetSettledBlindBoxDates([]string{drawDate})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if _, done := settled[drawDate]; done {
			common.ApiErrorMsg(c, "该期已开奖，不能再屏蔽")
			return
		}
		block.DrawDate = drawDate
	default:
		common.ApiErrorMsg(c, "屏蔽范围仅支持 period 或 permanent")
		return
	}

	user, err := model.GetUserById(req.UserId, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	block.Username = user.Username
	block.OperatorId = c.GetInt("id")
	block.OperatorName = c.GetString("username")

	created, err := model.CreateBlindBoxBlock(block)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		common.ApiErrorMsg(c, "该用户已在屏蔽名单中")
		return
	}
	common.SysLog(fmt.Sprintf("blindbox: block added scope=%s draw_date=%s user=%d(%s) by %d(%s) reason=%s",
		block.Scope, block.DrawDate, block.UserId, block.Username,
		block.OperatorId, block.OperatorName, block.Reason))
	common.ApiSuccess(c, block)
}

// AdminDeleteBlindBoxBlock 解除屏蔽（删行）。永久屏蔽是"行在即生效"的开关，删掉即恢复。
func AdminDeleteBlindBoxBlock(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "屏蔽记录 ID 不合法")
		return
	}
	block, err := model.GetBlindBoxBlockById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if block == nil {
		common.ApiErrorMsg(c, "屏蔽记录不存在")
		return
	}
	deleted, err := model.DeleteBlindBoxBlock(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !deleted {
		common.ApiErrorMsg(c, "屏蔽记录不存在")
		return
	}
	common.SysLog(fmt.Sprintf("blindbox: block removed scope=%s draw_date=%s user=%d(%s) by %d(%s)",
		block.Scope, block.DrawDate, block.UserId, block.Username,
		c.GetInt("id"), c.GetString("username")))
	common.ApiSuccess(c, nil)
}

// AdminGetBlindBoxBlocks 分页返回屏蔽名单，支持按范围 / 用户筛选。
func AdminGetBlindBoxBlocks(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	blocks, total, err := model.SearchBlindBoxBlocks(
		strings.TrimSpace(c.Query("scope")),
		strings.TrimSpace(c.Query("keyword")),
		pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(blocks)
	common.ApiSuccess(c, pageInfo)
}

// DoBlindBoxClaim 用户开盒领取奖金。
func DoBlindBoxClaim(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "盲盒功能未启用")
		return
	}
	userId := c.GetInt("id")

	winner, err := model.ClaimBlindBox(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	model.RecordQuotaLog(userId, model.LogTypeBlindBox, winner.PrizeQuota,
		fmt.Sprintf("盲盒开奖，获得额度 %s（%s）", logger.LogQuota(winner.PrizeQuota), winner.PrizeName))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "开盒成功",
		"data": gin.H{
			"prize_name":  winner.PrizeName,
			"prize_quota": winner.PrizeQuota,
			"draw_date":   winner.DrawDate,
		},
	})
}
