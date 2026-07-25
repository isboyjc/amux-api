package controller

import (
	"fmt"
	"net/http"
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

// ===================== 管理端接口（只读） =====================

// AdminGetBlindBoxSummary 返回当前生效规则 + 本期实时预览 + 全局统计 + 健康标志。
//
// 规则一律读内存中的 setting 实例而非 options 表：那才是定时任务真正参与结算的值，
// 两者若因写库失败而不一致，管理员看到的必须是"实际生效的"那份。
func AdminGetBlindBoxSummary(c *gin.Context) {
	setting := operation_setting.GetBlindBoxSetting()

	hour, minute, ok := setting.ParseDrawTime()
	if !ok {
		hour, minute = 0, 0
	}
	now := time.Now()
	boundary := model.MostRecentDrawBoundary(now, hour, minute)
	periodStart := boundary.Unix()
	nextDrawAt := model.NextDrawBoundary(boundary, hour, minute).Unix()
	currentDrawDate := model.CurrentBlindBoxDrawDate(now, hour, minute)

	// 本期实时预览：按当前配置推演今天的奖池，方便开奖前调整参数。
	previewEntry, previewCount, previewConsume, previewErr := model.GetBlindBoxPeriodPreview(
		currentDrawDate, periodStart, now.Unix(), int64(setting.ThresholdQuota))
	previewPool := model.BuildBlindBoxPrizePool(setting, previewConsume)
	var previewPoolQuota int64
	for _, p := range previewPool {
		previewPoolQuota += int64(p.Quota)
	}
	// 实际中奖名额受达标人数限制，多余奖项作废。
	previewWinner := len(previewPool)
	if previewWinner > previewCount {
		previewWinner = previewCount
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
			"period_start":      periodStart,
			"next_draw_at":      nextDrawAt,
			"current_draw_date": currentDrawDate,
		},
		"preview": gin.H{
			// 消费日志关闭时聚合恒为空，前端据 log_consume_enabled 提示，不要误读成"今天没人消费"
			"available": previewErr == nil,
			// entry_count 是报名总数，participant_count 是报名且已达标（真正入池）的人数
			"entry_count":       previewEntry,
			"participant_count": previewCount,
			"total_consume":     previewConsume,
			"prize_count":       len(previewPool),
			"winner_count":      previewWinner,
			"total_prize_quota": previewPoolQuota,
			"prizes":            previewPool,
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
