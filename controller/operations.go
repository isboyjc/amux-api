package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// disposableEmailDomains 一组常见一次性邮箱后缀，用于 AFF 邀请套利识别。
// 不追求穷尽，只覆盖最高频被滥用的，命中即标红供管理员复核。
var disposableEmailDomains = map[string]struct{}{
	"mailinator.com":     {},
	"tempmail.com":       {},
	"temp-mail.org":      {},
	"10minutemail.com":   {},
	"10minutemail.net":   {},
	"guerrillamail.com":  {},
	"guerrillamail.info": {},
	"guerrillamail.net":  {},
	"sharklasers.com":    {},
	"yopmail.com":        {},
	"yopmail.net":        {},
	"throwawaymail.com":  {},
	"getnada.com":        {},
	"fakeinbox.com":      {},
	"dispostable.com":    {},
	"maildrop.cc":        {},
	"trashmail.com":      {},
	"mintemail.com":      {},
	"mohmal.com":         {},
	"emailondeck.com":    {},
}

func extractEmailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

func isDisposableDomain(domain string) bool {
	_, ok := disposableEmailDomains[domain]
	return ok
}

// 运营统计面板（管理员侧）相关接口。
//
// 设计口径说明：
//   - 平台真实成本（上游账单）依赖各家阶梯价 + 兜底回退，无法在站内可靠估算，
//     因此本面板不展示"毛利"，避免误导。
//   - "余额净变化 = 充值 - 站内消耗" 是用户余额池层面的现金流口径，仅反映流水方向，
//     不等于平台利润。前端会在卡片副标题明确标注"非利润"。
//   - 时间区间统一按 Unix 秒传入；上限/下限缺省时按"近 7 天"兜底。

const (
	defaultOperationsRangeSeconds int64 = 7 * 24 * 3600
	// 最大时间跨度：50 年，给"全部时间"预设留余量。
	// 此接口仅 admin 可访问，且 quota_data / top_ups / logs 都已建索引，全表扫不会出问题。
	maxOperationsRangeSeconds int64 = 50 * 365 * 24 * 3600
	defaultTopN                     = 10
	maxTopN                         = 100
)

// parseOperationsRange 解析 start_timestamp / end_timestamp，返回闭区间。
// 缺省时按 [now-7d, now] 兜底；非法时返回错误。
func parseOperationsRange(c *gin.Context) (start int64, end int64, err error) {
	start, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	now := time.Now().Unix()
	if end <= 0 {
		end = now
	}
	if start <= 0 {
		start = end - defaultOperationsRangeSeconds
	}
	if start >= end {
		return 0, 0, errors.New("start_timestamp must be less than end_timestamp")
	}
	if end-start > maxOperationsRangeSeconds {
		return 0, 0, errors.New("time range too large, max 1 year")
	}
	return start, end, nil
}

func parseTopN(c *gin.Context) int {
	n, _ := strconv.Atoi(c.Query("limit"))
	if n <= 0 {
		return defaultTopN
	}
	if n > maxTopN {
		return maxTopN
	}
	return n
}

// pickBucket 根据时间跨度自适应选择按小时还是按天分桶。
//   - <= 2 天：按小时（quota_data 已是小时级聚合，logs 用 created_at 折算）
//   - 其它：按天
func pickBucket(start, end int64) string {
	if end-start <= 2*24*3600 {
		return "hour"
	}
	return "day"
}

// GetOperationsOverview 经营概览 KPI。
// 返回：
//   - topup_amount        充值流水（货币口径，原币种）
//   - topup_count         成功充值笔数
//   - paying_users        付费用户数（distinct user_id）
//   - arpu                客单价 = topup_amount / paying_users
//   - consumption_quota   站内消耗（quota 单位）
//   - redemption_quota    兑换码核销额度（quota 单位）
//   - aff_rebate_quota    邀请返利发放额度（quota 单位）
//   - net_balance_change  余额净变化 = topup_amount - consumption_quota / quota_per_unit
//                         （仅反映余额池流水，非利润）
//
// 同时返回上一周期的同口径数据（period-over-period）用于前端展示环比。
func GetOperationsOverview(c *gin.Context) {
	start, end, err := parseOperationsRange(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	prevEnd := start
	prevStart := start - (end - start)

	current, err := buildOverviewSnapshot(start, end)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	previous, err := buildOverviewSnapshot(prevStart, prevEnd)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"current":  current,
			"previous": previous,
			"range": gin.H{
				"start": start,
				"end":   end,
			},
		},
	})
}

type overviewSnapshot struct {
	TopupAmount       float64 `json:"topup_amount"`
	TopupCount        int64   `json:"topup_count"`
	PayingUsers       int64   `json:"paying_users"`
	Arpu              float64 `json:"arpu"`
	ConsumptionQuota  int64   `json:"consumption_quota"`
	RedemptionQuota   int64   `json:"redemption_quota"`
	AffRebateQuota    int64   `json:"aff_rebate_quota"`
	NetBalanceChange  float64 `json:"net_balance_change"`
	NewUsers          int64   `json:"new_users"`
	ActiveUsers       int64   `json:"active_users"`
	QuotaPerUnit      float64 `json:"quota_per_unit"`
}

func buildOverviewSnapshot(start, end int64) (*overviewSnapshot, error) {
	snap := &overviewSnapshot{
		QuotaPerUnit: common.QuotaPerUnit,
	}

	// 充值（仅 status='success' 且 complete_time 在区间内）
	var topupAgg struct {
		Sum   float64
		Count int64
		Users int64
	}
	if err := model.DB.Table("top_ups").
		Select("COALESCE(SUM(money), 0) AS sum, COUNT(*) AS count, COUNT(DISTINCT user_id) AS users").
		Where("status = ? AND complete_time >= ? AND complete_time <= ?", "success", start, end).
		Scan(&topupAgg).Error; err != nil {
		return nil, fmt.Errorf("topup agg: %w", err)
	}
	snap.TopupAmount = topupAgg.Sum
	snap.TopupCount = topupAgg.Count
	snap.PayingUsers = topupAgg.Users
	if snap.PayingUsers > 0 {
		snap.Arpu = snap.TopupAmount / float64(snap.PayingUsers)
	}

	// 站内消耗（quota_data 已是小时级聚合）
	var consumption int64
	if err := model.DB.Table("quota_data").
		Select("COALESCE(SUM(quota), 0)").
		Where("created_at >= ? AND created_at <= ?", start, end).
		Scan(&consumption).Error; err != nil {
		return nil, fmt.Errorf("consumption agg: %w", err)
	}
	snap.ConsumptionQuota = consumption

	// 兑换码核销
	var redemption int64
	if err := model.DB.Table("redemptions").
		Select("COALESCE(SUM(quota), 0)").
		Where("status = ? AND redeemed_time >= ? AND redeemed_time <= ?",
			common.RedemptionCodeStatusUsed, start, end).
		Scan(&redemption).Error; err != nil {
		return nil, fmt.Errorf("redemption agg: %w", err)
	}
	snap.RedemptionQuota = redemption

	// 邀请返利（aff_rebate_logs.created_at 是 time.Time，需转换）
	var rebate int64
	startTime := time.Unix(start, 0)
	endTime := time.Unix(end, 0)
	if err := model.DB.Table("aff_rebate_logs").
		Select("COALESCE(SUM(quota), 0)").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Scan(&rebate).Error; err != nil {
		// 表可能尚未创建（旧版本升级），降级为 0 而不是失败
		common.SysLog("operations: aff_rebate_logs aggregate failed: " + err.Error())
		rebate = 0
	}
	snap.AffRebateQuota = rebate

	// 新增用户（按 created_time 计）
	var newUsers int64
	if err := model.DB.Table("users").
		Where("created_time >= ? AND created_time <= ?", start, end).
		Count(&newUsers).Error; err != nil {
		return nil, fmt.Errorf("new users count: %w", err)
	}
	snap.NewUsers = newUsers

	// 活跃用户（区间内有过消耗的去重用户数）
	var activeUsers int64
	if err := model.DB.Table("quota_data").
		Select("COUNT(DISTINCT user_id)").
		Where("created_at >= ? AND created_at <= ?", start, end).
		Scan(&activeUsers).Error; err != nil {
		return nil, fmt.Errorf("active users count: %w", err)
	}
	snap.ActiveUsers = activeUsers

	// 余额净变化（同币种）
	if common.QuotaPerUnit > 0 {
		snap.NetBalanceChange = snap.TopupAmount - float64(snap.ConsumptionQuota)/common.QuotaPerUnit
	}

	return snap, nil
}

// GetOperationsRevenueTrend 营收趋势：按时间桶返回每桶的充值金额、笔数、消耗额度。
// 同时按支付方式分组返回当期占比，便于在卡片下方展示扇形图。
func GetOperationsRevenueTrend(c *gin.Context) {
	start, end, err := parseOperationsRange(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 自适应起点：当用户传的窗口很大（如"全部时间"50 年），把起点收敛到
	// 第一条真实数据出现的位置，避免曲线前半段全是 0。
	// 取 top_ups.complete_time 与 quota_data.created_at 二者最早值的较小者。
	var earliestTopup, earliestConsume int64
	model.DB.Table("top_ups").
		Select("COALESCE(MIN(complete_time), 0)").
		Where("status = ? AND complete_time >= ? AND complete_time <= ?", "success", start, end).
		Scan(&earliestTopup)
	model.DB.Table("quota_data").
		Select("COALESCE(MIN(created_at), 0)").
		Where("created_at >= ? AND created_at <= ?", start, end).
		Scan(&earliestConsume)
	earliest := earliestTopup
	if earliestConsume > 0 && (earliest == 0 || earliestConsume < earliest) {
		earliest = earliestConsume
	}
	if earliest > 0 && earliest > start {
		start = earliest
	}

	bucket := pickBucket(start, end)
	bucketSize := int64(86400)
	if bucket == "hour" {
		bucketSize = 3600
	}

	type trendPoint struct {
		Bucket     int64   `json:"bucket"`
		TopupSum   float64 `json:"topup_sum"`
		TopupCount int64   `json:"topup_count"`
		Consume    int64   `json:"consume"`
	}

	// 充值桶聚合：按 (complete_time / bucketSize) * bucketSize 分桶，三库通用
	rows := []trendPoint{}
	topupSelect := fmt.Sprintf("(complete_time/%d)*%d AS bucket, SUM(money) AS topup_sum, COUNT(*) AS topup_count",
		bucketSize, bucketSize)
	if err := model.DB.Table("top_ups").
		Select(topupSelect).
		Where("status = ? AND complete_time >= ? AND complete_time <= ?", "success", start, end).
		Group("bucket").
		Order("bucket ASC").
		Scan(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// 消耗桶聚合：quota_data 是小时级，按桶汇总后并入
	consumeRows := []trendPoint{}
	consumeSelect := fmt.Sprintf("(created_at/%d)*%d AS bucket, SUM(quota) AS consume",
		bucketSize, bucketSize)
	if err := model.DB.Table("quota_data").
		Select(consumeSelect).
		Where("created_at >= ? AND created_at <= ?", start, end).
		Group("bucket").
		Order("bucket ASC").
		Scan(&consumeRows).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// 合并两个序列到同一时间桶 map
	merged := map[int64]*trendPoint{}
	for _, r := range rows {
		copy := r
		merged[r.Bucket] = &copy
	}
	for _, r := range consumeRows {
		if existing, ok := merged[r.Bucket]; ok {
			existing.Consume = r.Consume
		} else {
			copy := r
			merged[r.Bucket] = &copy
		}
	}

	// 输出有序序列，缺失桶补零
	out := make([]trendPoint, 0, (end-start)/bucketSize+1)
	for b := (start / bucketSize) * bucketSize; b <= end; b += bucketSize {
		if p, ok := merged[b]; ok {
			out = append(out, *p)
		} else {
			out = append(out, trendPoint{Bucket: b})
		}
	}

	// 新老付费用户占比（当期）。
	// 判定：区间内有过成功充值的用户，若其历史首笔成功充值 >= start 视作"新付费用户"，否则视作"复购用户"。
	// 这里用两次查询在 Go 内分组，避免依赖各 DB 不一致的派生表 / HAVING 写法。
	type payerSplitItem struct {
		Segment string  `json:"segment"`
		Users   int64   `json:"users"`
		Amount  float64 `json:"amount"`
	}

	type periodPayer struct {
		UserId int     `json:"user_id"`
		Money  float64 `json:"money"`
	}
	periodPayers := []periodPayer{}
	if err := model.DB.Table("top_ups").
		Select("user_id, SUM(money) AS money").
		Where("status = ? AND complete_time >= ? AND complete_time <= ?",
			"success", start, end).
		Group("user_id").
		Scan(&periodPayers).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	payerSplit := []payerSplitItem{
		{Segment: "new"},
		{Segment: "returning"},
	}
	if len(periodPayers) > 0 {
		userIds := make([]int, 0, len(periodPayers))
		for _, p := range periodPayers {
			userIds = append(userIds, p.UserId)
		}
		type firstTime struct {
			UserId int   `json:"user_id"`
			First  int64 `json:"first"`
		}
		firsts := []firstTime{}
		if err := model.DB.Table("top_ups").
			Select("user_id, MIN(complete_time) AS first").
			Where("status = ? AND user_id IN ?", "success", userIds).
			Group("user_id").
			Scan(&firsts).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		firstMap := map[int]int64{}
		for _, f := range firsts {
			firstMap[f.UserId] = f.First
		}
		for _, p := range periodPayers {
			isNew := firstMap[p.UserId] >= start
			idx := 1
			if isNew {
				idx = 0
			}
			payerSplit[idx].Users++
			payerSplit[idx].Amount += p.Money
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"bucket":       bucket,
			"bucket_size":  bucketSize,
			"series":       out,
			"payer_split":  payerSplit,
		},
	})
}

// GetOperationsChannelHealth 渠道健康度：
//   - 区间内每个渠道的调用总数 / 失败数 / 成功率 / 平均耗时（毫秒）
//   - 仅对管理员可见
//
// 数据源：logs 表（type=2 消耗 / type=5 错误）。logs 在 LOG_DB（可能与主库分离）。
func GetOperationsChannelHealth(c *gin.Context) {
	start, end, err := parseOperationsRange(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	limit := parseTopN(c)

	type channelRow struct {
		ChannelId   int     `json:"channel_id"`
		Total       int64   `json:"total"`
		Errors      int64   `json:"errors"`
		AvgUseTime  float64 `json:"avg_use_time"`
		ChannelName string  `json:"channel_name"`
	}

	rows := []channelRow{}
	// logs 表中渠道列名为 channel_id（GORM 默认 snake_case），不是 json tag 里的 "channel"。
	// 三库通用：CASE WHEN ... THEN 1 ELSE 0 END
	selectExpr := "channel_id, " +
		"COUNT(*) AS total, " +
		"SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) AS errors, " +
		"COALESCE(AVG(CASE WHEN type = ? THEN use_time ELSE NULL END), 0) AS avg_use_time"
	if err := model.LOG_DB.Table("logs").
		Select(selectExpr, model.LogTypeError, model.LogTypeConsume).
		Where("type IN (?, ?) AND channel_id > 0 AND created_at >= ? AND created_at <= ?",
			model.LogTypeConsume, model.LogTypeError, start, end).
		Group("channel_id").
		Order("total DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// 渠道名映射（如果 logs 与渠道不在同一库，单独查一次主库）
	if len(rows) > 0 {
		ids := make([]int, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ChannelId)
		}
		type cn struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		}
		var names []cn
		if err := model.DB.Table("channels").
			Select("id, name").
			Where("id IN ?", ids).
			Scan(&names).Error; err == nil {
			nameMap := map[int]string{}
			for _, n := range names {
				nameMap[n.Id] = n.Name
			}
			for i := range rows {
				rows[i].ChannelName = nameMap[rows[i].ChannelId]
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

// GetOperationsTopConsumers Top N 消耗用户（基于 quota_data，已小时级预聚合，查询轻量）。
func GetOperationsTopConsumers(c *gin.Context) {
	start, end, err := parseOperationsRange(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	limit := parseTopN(c)

	type row struct {
		UserId    int    `json:"user_id"`
		Username  string `json:"username"`
		Quota     int64  `json:"quota"`
		Count     int64  `json:"count"`
		TokenUsed int64  `json:"token_used"`
	}
	rows := []row{}
	if err := model.DB.Table("quota_data").
		Select("user_id, username, SUM(quota) AS quota, SUM(count) AS count, SUM(token_used) AS token_used").
		Where("created_at >= ? AND created_at <= ?", start, end).
		Group("user_id, username").
		Order("quota DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

// GetOperationsRecentTopups 最近成功充值（用于运营快速发现大额）。
func GetOperationsRecentTopups(c *gin.Context) {
	limit := parseTopN(c)

	type recentTopup struct {
		Id              int     `json:"id"`
		UserId          int     `json:"user_id"`
		Username        string  `json:"username"`
		Money           float64 `json:"money"`
		Amount          int64   `json:"amount"`
		PaymentMethod   string  `json:"payment_method"`
		PaymentProvider string  `json:"payment_provider"`
		CompleteTime    int64   `json:"complete_time"`
	}
	rows := []recentTopup{}
	if err := model.DB.Table("top_ups").
		Select("top_ups.id, top_ups.user_id, top_ups.money, top_ups.amount, "+
			"top_ups.payment_method, top_ups.payment_provider, top_ups.complete_time, "+
			"u.username AS username").
		Joins("LEFT JOIN users u ON u.id = top_ups.user_id").
		Where("top_ups.status = ?", "success").
		Order("top_ups.complete_time DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

// GetOperationsQuotaIssuance 区间内"额度发放"按来源拆分。
//
// 数据口径说明：
//   - real_topup_quota   真实充值带入的额度（top_ups.amount，仅 success），与现金严格对应
//   - signup_gift_quota  注册赠送（含新用户、邀请码、邀请人三类，从 LogTypeSignupGift 聚合）。
//                        改造前的历史日志没有结构化 quota，会被遗漏，是已知局限。
//   - inviter_rebate     邀请人返利（aff_rebate_logs.type=1）
//   - topup_rebate       充值返利（aff_rebate_logs.type=2）
//   - redemption_quota   兑换码核销（redemptions，按 redeemed_time）
//   - admin_topup_quota  管理员调整中正向 delta 之和（线下充值，type=3 且 quota>0）
//   - admin_adjust_count 管理员手动操作总次数（含增减覆盖三种）
func GetOperationsQuotaIssuance(c *gin.Context) {
	start, end, err := parseOperationsRange(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	out := struct {
		RealTopupQuota   int64 `json:"real_topup_quota"`
		SignupGiftQuota  int64 `json:"signup_gift_quota"`
		InviterRebate    int64 `json:"inviter_rebate"`
		TopupRebate      int64 `json:"topup_rebate"`
		RedemptionQuota  int64 `json:"redemption_quota"`
		AdminTopupQuota  int64 `json:"admin_topup_quota"`  // 管理员调整中正向 delta 之和（线下充值）
		AdminDeductQuota int64 `json:"admin_deduct_quota"` // 管理员调整中负向 delta 绝对值之和（扣减）
		AdminAdjustCount int64 `json:"admin_adjust_count"`
		QuotaForNewUser  int   `json:"quota_for_new_user"`
		QuotaForInvitee  int   `json:"quota_for_invitee"`
		NewUsers         int64 `json:"new_users"`
		NewUsersInvited  int64 `json:"new_users_invited"`
	}{
		QuotaForNewUser: common.QuotaForNewUser,
		QuotaForInvitee: common.QuotaForInvitee,
	}

	if err := model.DB.Table("top_ups").
		Select("COALESCE(SUM(amount), 0)").
		Where("status = ? AND complete_time >= ? AND complete_time <= ?", "success", start, end).
		Scan(&out.RealTopupQuota).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Table("users").
		Where("created_time >= ? AND created_time <= ?", start, end).
		Count(&out.NewUsers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Table("users").
		Where("inviter_id != 0 AND created_time >= ? AND created_time <= ?", start, end).
		Count(&out.NewUsersInvited).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// 注册赠送：直接聚合 LogTypeSignupGift（结构化 quota），不再按配置 × 人数估算
	if err := model.LOG_DB.Table("logs").
		Select("COALESCE(SUM(quota), 0)").
		Where("type = ? AND created_at >= ? AND created_at <= ?",
			model.LogTypeSignupGift, start, end).
		Scan(&out.SignupGiftQuota).Error; err != nil {
		common.SysLog("operations: sum signup gift failed: " + err.Error())
	}

	startTime := time.Unix(start, 0)
	endTime := time.Unix(end, 0)
	type rebateRow struct {
		Type int   `json:"type"`
		Sum  int64 `json:"sum"`
	}
	rebates := []rebateRow{}
	if err := model.DB.Table("aff_rebate_logs").
		Select("type, COALESCE(SUM(quota), 0) AS sum").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("type").
		Scan(&rebates).Error; err != nil {
		common.SysLog("operations: aff_rebate_logs aggregate failed: " + err.Error())
		rebates = nil
	}
	for _, r := range rebates {
		switch r.Type {
		case model.AffRebateTypeRegister:
			out.InviterRebate = r.Sum
		case model.AffRebateTypeTopup:
			out.TopupRebate = r.Sum
		}
	}

	if err := model.DB.Table("redemptions").
		Select("COALESCE(SUM(quota), 0)").
		Where("status = ? AND redeemed_time >= ? AND redeemed_time <= ?",
			common.RedemptionCodeStatusUsed, start, end).
		Scan(&out.RedemptionQuota).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	if err := model.LOG_DB.Table("logs").
		Where("type = ? AND created_at >= ? AND created_at <= ?",
			model.LogTypeManage, start, end).
		Count(&out.AdminAdjustCount).Error; err != nil {
		common.SysLog("operations: count admin logs failed: " + err.Error())
	}

	// 管理员手动调额（结构化金额聚合）。
	// 注：仅 RecordQuotaLog 改造之后写入的 type=3 日志会有 quota>0/<0；
	// 历史日志 quota=0 不会被计入，是已知局限。
	if err := model.LOG_DB.Table("logs").
		Select("COALESCE(SUM(quota), 0)").
		Where("type = ? AND quota > 0 AND created_at >= ? AND created_at <= ?",
			model.LogTypeManage, start, end).
		Scan(&out.AdminTopupQuota).Error; err != nil {
		common.SysLog("operations: sum admin topup failed: " + err.Error())
	}
	var adminDeductRaw int64
	if err := model.LOG_DB.Table("logs").
		Select("COALESCE(SUM(quota), 0)").
		Where("type = ? AND quota < 0 AND created_at >= ? AND created_at <= ?",
			model.LogTypeManage, start, end).
		Scan(&adminDeductRaw).Error; err != nil {
		common.SysLog("operations: sum admin deduct failed: " + err.Error())
	}
	out.AdminDeductQuota = -adminDeductRaw // 绝对值便于前端展示

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    out,
	})
}

// GetOperationsBalanceSnapshot 系统当前用户余额快照（不带时间区间）。
//
// 由于 logs 中 type=1/3/4 不写 quota 字段，无法把"当前余额"按历史来源还原；
// 这里只能给出当前快照总额，加上 aff 池子细分。如需"剩余额度按来源拆分"，
// 需要新增结构化记账（不在本期范围内）。
func GetOperationsBalanceSnapshot(c *gin.Context) {
	out := struct {
		TotalQuota      int64 `json:"total_quota"`
		TotalAffQuota   int64 `json:"total_aff_quota"`
		TotalAffHistory int64 `json:"total_aff_history"`
		TotalUsers      int64 `json:"total_users"`
		EnabledUsers    int64 `json:"enabled_users"`
	}{}

	// 用户余额总和排除管理员/超管（role >= RoleAdminUser），只统计普通用户的余额负债。
	if err := model.DB.Table("users").
		Where("role < ?", common.RoleAdminUser).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&out.TotalQuota).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Table("users").
		Select("COALESCE(SUM(aff_quota), 0)").
		Scan(&out.TotalAffQuota).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Table("users").
		Select("COALESCE(SUM(aff_history), 0)").
		Scan(&out.TotalAffHistory).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Table("users").
		Count(&out.TotalUsers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Table("users").
		Where("status = ?", common.UserStatusEnabled).
		Count(&out.EnabledUsers).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    out,
	})
}

// GetOperationsAffiliate 邀请分析：区间内 AFF 邀请的注册情况，含套利风险标记。
//
// 风险判定（粗筛，命中任一即标 risk=true，便于管理员复核）：
//   1. 区间内邀请数 >= 5
//   2. 一次性邮箱占比 >= 30% 且至少命中 1 个
func GetOperationsAffiliate(c *gin.Context) {
	start, end, err := parseOperationsRange(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	limit := parseTopN(c)

	type invitee struct {
		Id          int    `json:"id"`
		Email       string `json:"email"`
		InviterId   int    `json:"inviter_id"`
		CreatedTime int64  `json:"created_time"`
	}
	invitees := []invitee{}
	if err := model.DB.Table("users").
		Select("id, email, inviter_id, created_time").
		Where("inviter_id != 0 AND created_time >= ? AND created_time <= ?", start, end).
		Order("created_time ASC").
		Find(&invitees).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	type inviterAgg struct {
		InvitesInRange int
		DisposableHits int
		SampleEmails   []string
	}
	aggMap := map[int]*inviterAgg{}
	domainCount := map[string]int{}

	for _, iv := range invitees {
		domain := extractEmailDomain(iv.Email)
		if domain != "" {
			domainCount[domain]++
		}
		a, ok := aggMap[iv.InviterId]
		if !ok {
			a = &inviterAgg{}
			aggMap[iv.InviterId] = a
		}
		a.InvitesInRange++
		if isDisposableDomain(domain) {
			a.DisposableHits++
		}
		if len(a.SampleEmails) < 5 && iv.Email != "" {
			a.SampleEmails = append(a.SampleEmails, iv.Email)
		}
	}

	inviterIds := make([]int, 0, len(aggMap))
	for id := range aggMap {
		inviterIds = append(inviterIds, id)
	}
	type inviterMeta struct {
		Id          int    `json:"id"`
		Username    string `json:"username"`
		Email       string `json:"email"`
		AffCount    int    `json:"aff_count"`
		AffHistory  int64  `json:"aff_history"`
		AffQuota    int64  `json:"aff_quota"`
		CreatedTime int64  `json:"created_time"`
	}
	metas := []inviterMeta{}
	if len(inviterIds) > 0 {
		if err := model.DB.Table("users").
			Select("id, username, email, aff_count, aff_history, aff_quota, created_time").
			Where("id IN ?", inviterIds).
			Scan(&metas).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	}
	metaMap := map[int]inviterMeta{}
	for _, m := range metas {
		metaMap[m.Id] = m
	}

	type topInviter struct {
		InviterId          int      `json:"inviter_id"`
		Username           string   `json:"username"`
		Email              string   `json:"email"`
		LifetimeAffCount   int      `json:"lifetime_aff_count"`
		LifetimeAffHistory int64    `json:"lifetime_aff_history"`
		PendingAffQuota    int64    `json:"pending_aff_quota"`
		InvitesInRange     int      `json:"invites_in_range"`
		DisposableHits     int      `json:"disposable_hits"`
		DisposableRatio    float64  `json:"disposable_ratio"`
		SampleInvitees     []string `json:"sample_invitees"`
		Risk               bool     `json:"risk"`
		RiskReasons        []string `json:"risk_reasons"`
	}
	tops := make([]topInviter, 0, len(aggMap))
	for id, a := range aggMap {
		m := metaMap[id]
		ratio := 0.0
		if a.InvitesInRange > 0 {
			ratio = float64(a.DisposableHits) / float64(a.InvitesInRange)
		}
		row := topInviter{
			InviterId:          id,
			Username:           m.Username,
			Email:              m.Email,
			LifetimeAffCount:   m.AffCount,
			LifetimeAffHistory: m.AffHistory,
			PendingAffQuota:    m.AffQuota,
			InvitesInRange:     a.InvitesInRange,
			DisposableHits:     a.DisposableHits,
			DisposableRatio:    ratio,
			SampleInvitees:     a.SampleEmails,
		}
		var reasons []string
		if row.InvitesInRange >= 5 {
			reasons = append(reasons, "high_volume")
		}
		if row.DisposableRatio >= 0.3 && row.DisposableHits > 0 {
			reasons = append(reasons, "disposable_email")
		}
		if len(reasons) > 0 {
			row.Risk = true
			row.RiskReasons = reasons
		}
		tops = append(tops, row)
	}

	// 排序：风险优先，再按邀请数
	for i := 0; i < len(tops); i++ {
		for j := i + 1; j < len(tops); j++ {
			score := func(r topInviter) int {
				s := r.InvitesInRange
				if r.Risk {
					s += 100000
				}
				return s
			}
			if score(tops[j]) > score(tops[i]) {
				tops[i], tops[j] = tops[j], tops[i]
			}
		}
	}
	if len(tops) > limit {
		tops = tops[:limit]
	}

	type domainRow struct {
		Domain     string `json:"domain"`
		Count      int    `json:"count"`
		Disposable bool   `json:"disposable"`
	}
	domains := make([]domainRow, 0, len(domainCount))
	for d, c := range domainCount {
		domains = append(domains, domainRow{Domain: d, Count: c, Disposable: isDisposableDomain(d)})
	}
	for i := 0; i < len(domains); i++ {
		for j := i + 1; j < len(domains); j++ {
			if domains[j].Count > domains[i].Count {
				domains[i], domains[j] = domains[j], domains[i]
			}
		}
	}
	if len(domains) > 20 {
		domains = domains[:20]
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"new_invited_users": len(invitees),
			"active_inviters":   len(aggMap),
			"top_inviters":      tops,
			"email_domains":     domains,
		},
	})
}
