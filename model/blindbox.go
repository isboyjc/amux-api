package model

import (
	crand "crypto/rand"
	"errors"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

// 盲盒抽奖数据模型与核心逻辑。
//
// 两张表都在主库（DB）：
//   - BlindBoxDraw   每个开奖周期一行，draw_date 唯一索引保证「每期只结算一次」（幂等）。
//   - BlindBoxWinner 中奖明细，pending→claimed→expired 状态机，靠条件更新保证并发安全。
//
// 达标判定读 LOG_DB.logs（可能是独立日志库），只做只读聚合，不与主库事务耦合。

// 中奖记录状态。
const (
	BlindBoxStatusPending = "pending" // 已中奖，待用户开盒
	BlindBoxStatusClaimed = "claimed" // 已开盒到账
	BlindBoxStatusExpired = "expired" // 超时未开，已作废
)

// 开奖记录状态。
const (
	BlindBoxDrawStatusDone  = "done"  // 正常开奖（有中奖者）
	BlindBoxDrawStatusEmpty = "empty" // 无人达标，空期
)

// BlindBoxDraw 每个开奖周期的结算记录。
type BlindBoxDraw struct {
	Id                int    `json:"id" gorm:"primaryKey;autoIncrement"`
	DrawDate          string `json:"draw_date" gorm:"type:varchar(10);not null;uniqueIndex:idx_bbd_date"` // 开奖边界日期 YYYY-MM-DD
	PeriodStart       int64  `json:"period_start" gorm:"bigint"`                                          // 周期起始（Unix 秒）
	PeriodEnd         int64  `json:"period_end" gorm:"bigint"`                                            // 周期结束/开奖时点（Unix 秒）
	EntryCount        int    `json:"entry_count" gorm:"default:0"`       // 本期报名人数
	ParticipantCount  int    `json:"participant_count" gorm:"default:0"` // 报名且消耗达标、真正进入奖池的人数
	WinnerCount       int    `json:"winner_count" gorm:"default:0"`
	TotalConsumeQuota int64  `json:"total_consume_quota" gorm:"default:0"`
	TotalPrizeQuota   int64  `json:"total_prize_quota" gorm:"default:0"`
	PoolMode          string `json:"pool_mode" gorm:"type:varchar(16)"`
	Status            string `json:"status" gorm:"type:varchar(16)"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint"`
}

func (BlindBoxDraw) TableName() string {
	return "blind_box_draws"
}

// BlindBoxWinner 单条中奖明细。(draw_date, user_id) 唯一，保证每人每期最多一个奖。
type BlindBoxWinner struct {
	Id             int     `json:"id" gorm:"primaryKey;autoIncrement"`
	DrawDate       string  `json:"draw_date" gorm:"type:varchar(10);not null;uniqueIndex:idx_bbw_date_user,priority:1"`
	UserId         int     `json:"user_id" gorm:"not null;uniqueIndex:idx_bbw_date_user,priority:2;index:idx_bbw_user"`
	Username       string  `json:"username" gorm:"type:varchar(64);default:''"`
	PrizeName      string  `json:"prize_name" gorm:"type:varchar(64);default:''"`
	PrizeQuota     int     `json:"prize_quota" gorm:"default:0"`
	ConsumeQuota   int64   `json:"consume_quota" gorm:"default:0"`   // 该用户当期消耗（存档）
	WinProbability float64 `json:"win_probability" gorm:"default:0"` // 中奖概率（首抽口径，存档展示用）
	Status         string  `json:"status" gorm:"type:varchar(16);index:idx_bbw_status"`
	ExpireAt       int64   `json:"expire_at" gorm:"bigint"` // 领取截止（下次开奖时点）
	ClaimedAt      int64   `json:"claimed_at" gorm:"bigint;default:0"`
	CreatedAt      int64   `json:"created_at" gorm:"bigint"`
}

func (BlindBoxWinner) TableName() string {
	return "blind_box_winners"
}

// BlindBoxEntry 用户对某一期抽奖的主动报名。
//
// 存在的意义是公平性：只按消耗自动入池的话，从不关心活动的用户会白白占用中奖名额，
// 挤低主动参与者的概率。报名 + 达标 + 主动开盒，三步都要用户自己做。
// (draw_date, user_id) 唯一，重复报名靠唯一索引兜底。
type BlindBoxEntry struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	DrawDate  string `json:"draw_date" gorm:"type:varchar(10);not null;uniqueIndex:idx_bbe_date_user,priority:1"`
	UserId    int    `json:"user_id" gorm:"not null;uniqueIndex:idx_bbe_date_user,priority:2;index:idx_bbe_user"`
	Username  string `json:"username" gorm:"type:varchar(64);default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
}

func (BlindBoxEntry) TableName() string {
	return "blind_box_entries"
}

// blindBoxParticipant 达标参与者聚合结果。
type blindBoxParticipant struct {
	UserId   int    `gorm:"column:user_id"`
	Username string `gorm:"column:username"`
	Quota    int64  `gorm:"column:quota"`
}

// drawBoundaryOn 返回「ref 所在日期偏移 dayOffset 天」的 hour:minute 时刻。
//
// 一律走 time.Date 的日历运算，不用 Add(±24h)：在有夏令时的时区，切换日的 24 小时
// 绝对偏移会落到 23:00 或 01:00 而不是配置的开奖时刻，那一期就变成 23/25 小时，
// 消耗要么被相邻两期重复计入、要么整段漏掉。日历运算保持挂钟时刻不变，
// 周期严格等于「上一个开奖时点 → 下一个开奖时点」。
// time.Date 会自动归一化越界的日（如 3 月 32 日 → 4 月 1 日）。
func drawBoundaryOn(ref time.Time, dayOffset, hour, minute int) time.Time {
	return time.Date(ref.Year(), ref.Month(), ref.Day()+dayOffset, hour, minute, 0, 0, ref.Location())
}

// MostRecentDrawBoundary 返回 ref 之前（含）最近的一个开奖边界时刻。
// 例：drawTime=20:00，ref=今天 21:00 → 今天 20:00；ref=今天 19:00 → 昨天 20:00。
func MostRecentDrawBoundary(ref time.Time, hour, minute int) time.Time {
	b := drawBoundaryOn(ref, 0, hour, minute)
	if ref.Before(b) {
		b = drawBoundaryOn(ref, -1, hour, minute)
	}
	return b
}

// PrevDrawBoundary 返回 boundary 的上一个开奖时点（周期起点）。
func PrevDrawBoundary(boundary time.Time, hour, minute int) time.Time {
	return drawBoundaryOn(boundary, -1, hour, minute)
}

// NextDrawBoundary 返回 boundary 的下一个开奖时点（领取截止 / 周期终点）。
func NextDrawBoundary(boundary time.Time, hour, minute int) time.Time {
	return drawBoundaryOn(boundary, 1, hour, minute)
}

// CurrentBlindBoxDrawDate 返回「进行中周期」将要开奖的期号（YYYY-MM-DD）。
//
// 每期以其开奖时点所在日期命名：定时任务在边界 B 结算的是 [B-1天, B)，期号取 B 的日期。
// 用户此刻正在累计的是 [B, B+1天)，将在 B+1天 开奖，所以报名要落到下一个边界的日期上。
func CurrentBlindBoxDrawDate(now time.Time, hour, minute int) string {
	boundary := MostRecentDrawBoundary(now, hour, minute)
	return NextDrawBoundary(boundary, hour, minute).Format("2006-01-02")
}

// EnterBlindBoxDraw 用户报名参加 drawDate 这一期。重复报名返回 false。
func EnterBlindBoxDraw(userId int, username, drawDate string, now int64) (created bool, err error) {
	entry := &BlindBoxEntry{
		DrawDate:  drawDate,
		UserId:    userId,
		Username:  username,
		CreatedAt: now,
	}
	if err := DB.Create(entry).Error; err != nil {
		// 唯一索引冲突 = 已报名过。GORM 不区分驱动的冲突错误码，回查一次更可靠。
		if exists, checkErr := HasBlindBoxEntry(userId, drawDate); checkErr == nil && exists {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// HasBlindBoxEntry 判断用户是否已报名某一期。
func HasBlindBoxEntry(userId int, drawDate string) (bool, error) {
	var count int64
	err := DB.Model(&BlindBoxEntry{}).
		Where("draw_date = ? AND user_id = ?", drawDate, userId).Count(&count).Error
	return count > 0, err
}

// GetBlindBoxEntryUserIds 返回某一期已报名的用户 ID 集合。
//
// 结算时用它与「消耗达标名单」在内存里取交集，而不是把 user_id 拼进 SQL 的 IN：
// 达标名单来自 LOG_DB（可能是独立日志库），跟主库的报名表根本没法 JOIN。
func GetBlindBoxEntryUserIds(drawDate string) (map[int]struct{}, error) {
	var ids []int
	err := DB.Model(&BlindBoxEntry{}).Where("draw_date = ?", drawDate).
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, err
	}
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

// getBlindBoxParticipants 聚合 [periodStart, periodEnd) 内消耗达标的用户。
// 读 LOG_DB.logs，type=消费，按 user_id 分组，HAVING SUM(quota) >= threshold。
//
// 必须只按 user_id 分组：logs.username 是行内冗余字段（且 default:''），用户改名或
// 历史行缺名会让同一 user_id 裂成多组 —— 轻则消耗被拆分导致达标用户漏掉，重则同一人
// 被抽中两次，撞上 (draw_date, user_id) 唯一索引让整期结算回滚。
// MAX(username) 在 SQLite/MySQL/PostgreSQL 上语义一致。
func getBlindBoxParticipants(periodStart, periodEnd, threshold int64) ([]blindBoxParticipant, error) {
	var rows []blindBoxParticipant
	err := LOG_DB.Table("logs").
		Select("user_id, MAX(username) as username, SUM(quota) as quota").
		Where("type = ? AND created_at >= ? AND created_at < ?", LogTypeConsume, periodStart, periodEnd).
		Group("user_id").
		Having("SUM(quota) >= ?", threshold).
		Find(&rows).Error
	return rows, err
}

// GetUserBlindBoxConsume 返回某用户在 [start, end) 内的消费额度合计（用于实时进度展示）。
func GetUserBlindBoxConsume(userId int, start, end int64) (int64, error) {
	var total int64
	err := LOG_DB.Table("logs").
		Select("COALESCE(SUM(quota),0)").
		Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ?", userId, LogTypeConsume, start, end).
		Scan(&total).Error
	return total, err
}

// GetUserPendingBlindBox 返回用户当前可开启（pending 且未过期）的盲盒，无则返回 nil。
func GetUserPendingBlindBox(userId int, now int64) (*BlindBoxWinner, error) {
	var w BlindBoxWinner
	err := DB.Where("user_id = ? AND status = ? AND expire_at > ?", userId, BlindBoxStatusPending, now).
		Order("draw_date DESC").First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// GetUserBlindBoxEntries 分页返回用户的报名记录，按期次倒序。
//
// 用户的「参与历史」以报名记录为主线而不是中奖记录：只查中奖表的话，
// 报了名但没抽中的期次会整个消失，用户看到的就只剩一串中奖，无从判断自己参与过几次。
func GetUserBlindBoxEntries(userId int, startIdx, num int) ([]BlindBoxEntry, int64, error) {
	var total int64
	if err := DB.Model(&BlindBoxEntry{}).Where("user_id = ?", userId).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []BlindBoxEntry
	err := DB.Where("user_id = ?", userId).Order("draw_date DESC").
		Limit(num).Offset(startIdx).Find(&entries).Error
	return entries, total, err
}

// GetUserBlindBoxWinnersByDates 取用户在指定期次中的中奖记录，按期次建索引。
// 与报名记录在内存里合并，避免 LEFT JOIN 在三种数据库上的写法差异。
func GetUserBlindBoxWinnersByDates(userId int, dates []string) (map[string]BlindBoxWinner, error) {
	res := make(map[string]BlindBoxWinner, len(dates))
	if len(dates) == 0 {
		return res, nil
	}
	var rows []BlindBoxWinner
	if err := DB.Where("user_id = ? AND draw_date IN ?", userId, dates).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		res[r.DrawDate] = r
	}
	return res, nil
}

// GetSettledBlindBoxDates 返回指定期次里已完成结算的那些。
// 用于区分「已开奖但没抽中」和「本期还没到开奖时间」。
func GetSettledBlindBoxDates(dates []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(dates))
	if len(dates) == 0 {
		return set, nil
	}
	var got []string
	if err := DB.Model(&BlindBoxDraw{}).Where("draw_date IN ?", dates).
		Pluck("draw_date", &got).Error; err != nil {
		return nil, err
	}
	for _, d := range got {
		set[d] = struct{}{}
	}
	return set, nil
}

// SumUserBlindBoxClaimedQuota 用户累计开出并已到账的奖励额度。
//
// 只统计 claimed：pending 的奖品对用户必须保持不透明（算进去等于泄露未开奖品的金额），
// expired 的从未到账，计入会让用户误以为余额里有这笔钱。
func SumUserBlindBoxClaimedQuota(userId int) (int64, error) {
	var total int64
	err := DB.Model(&BlindBoxWinner{}).
		Select("COALESCE(SUM(prize_quota),0)").
		Where("user_id = ? AND status = ?", userId, BlindBoxStatusClaimed).
		Scan(&total).Error
	return total, err
}

// blindBoxDrawExists 判断某开奖边界是否已结算。
func blindBoxDrawExists(drawDate string) (bool, error) {
	var count int64
	err := DB.Model(&BlindBoxDraw{}).Where("draw_date = ?", drawDate).Count(&count).Error
	return count > 0, err
}

// GetLastBlindBoxDraw 返回最近一期已结算记录，无则返回 nil。
func GetLastBlindBoxDraw() (*BlindBoxDraw, error) {
	var d BlindBoxDraw
	err := DB.Order("period_end DESC").First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ExpireBlindBoxWinners 把所有已过领取窗口的 pending 记录置为 expired，返回受影响行数。
//
// 刻意与结算解耦、由定时任务每 tick 独立执行：过期的判定标准是 expire_at，而不是
// "下一期开奖有没有跑成功"。否则功能被关掉、主节点停机或某期结算失败时，记录会永远
// 停在 pending —— 前端显示"待开启"，但 ClaimBlindBox 因 expire_at 过滤又领不到。
func ExpireBlindBoxWinners(now int64) (int64, error) {
	res := DB.Model(&BlindBoxWinner{}).
		Where("status = ? AND expire_at <= ?", BlindBoxStatusPending, now).
		Update("status", BlindBoxStatusExpired)
	return res.RowsAffected, res.Error
}

// ===================== 管理端只读查询 =====================
//
// 管理页全部只读：结算的幂等性建立在 draw_date 唯一索引上，任何"手动补发/重开奖"
// 入口都会和定时任务抢同一把逻辑锁，收益远小于风险，因此不提供写接口。

// BlindBoxClaimStat 单期按状态拆分的领取情况。
type BlindBoxClaimStat struct {
	ClaimedCount int   `json:"claimed_count"`
	PendingCount int   `json:"pending_count"`
	ExpiredCount int   `json:"expired_count"`
	ClaimedQuota int64 `json:"claimed_quota"`
	PendingQuota int64 `json:"pending_quota"`
	ExpiredQuota int64 `json:"expired_quota"`
}

// BlindBoxOverview 盲盒全局统计。
type BlindBoxOverview struct {
	DrawCount         int64 `json:"draw_count"`
	TotalParticipant  int64 `json:"total_participant"`
	TotalWinner       int64 `json:"total_winner"`
	TotalConsumeQuota int64 `json:"total_consume_quota"`
	TotalPrizeQuota   int64 `json:"total_prize_quota"`
	ClaimedCount      int64 `json:"claimed_count"`
	ClaimedQuota      int64 `json:"claimed_quota"`
	PendingCount      int64 `json:"pending_count"`
	PendingQuota      int64 `json:"pending_quota"`
	ExpiredCount      int64 `json:"expired_count"`
	ExpiredQuota      int64 `json:"expired_quota"`
}

// GetBlindBoxDraws 分页返回开奖周期记录（按开奖时点倒序）。
func GetBlindBoxDraws(startIdx, num int) ([]BlindBoxDraw, int64, error) {
	var total int64
	if err := DB.Model(&BlindBoxDraw{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var draws []BlindBoxDraw
	err := DB.Order("period_end DESC").Limit(num).Offset(startIdx).Find(&draws).Error
	return draws, total, err
}

// GetBlindBoxClaimStats 批量聚合多期的领取情况。
// 用一条 GROUP BY 查询一次性取回，避免按行 N+1 查询。
func GetBlindBoxClaimStats(drawDates []string) (map[string]*BlindBoxClaimStat, error) {
	stats := make(map[string]*BlindBoxClaimStat)
	if len(drawDates) == 0 {
		return stats, nil
	}
	var rows []struct {
		DrawDate string `gorm:"column:draw_date"`
		Status   string `gorm:"column:status"`
		Cnt      int    `gorm:"column:cnt"`
		Quota    int64  `gorm:"column:quota"`
	}
	err := DB.Model(&BlindBoxWinner{}).
		Select("draw_date, status, COUNT(*) as cnt, COALESCE(SUM(prize_quota),0) as quota").
		Where("draw_date IN ?", drawDates).
		Group("draw_date, status").
		Find(&rows).Error
	if err != nil {
		return stats, err
	}
	for _, r := range rows {
		s, ok := stats[r.DrawDate]
		if !ok {
			s = &BlindBoxClaimStat{}
			stats[r.DrawDate] = s
		}
		switch r.Status {
		case BlindBoxStatusClaimed:
			s.ClaimedCount, s.ClaimedQuota = r.Cnt, r.Quota
		case BlindBoxStatusPending:
			s.PendingCount, s.PendingQuota = r.Cnt, r.Quota
		case BlindBoxStatusExpired:
			s.ExpiredCount, s.ExpiredQuota = r.Cnt, r.Quota
		}
	}
	return stats, nil
}

// SearchBlindBoxWinners 分页返回中奖明细，支持按期次 / 状态 / 用户（ID 或用户名）筛选。
func SearchBlindBoxWinners(drawDate, status, keyword string, startIdx, num int) ([]BlindBoxWinner, int64, error) {
	tx := DB.Model(&BlindBoxWinner{})
	if drawDate != "" {
		tx = tx.Where("draw_date = ?", drawDate)
	}
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if keyword != "" {
		if uid, err := strconv.Atoi(keyword); err == nil {
			tx = tx.Where("user_id = ?", uid)
		} else {
			tx = tx.Where("username LIKE ?", "%"+keyword+"%")
		}
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var winners []BlindBoxWinner
	err := tx.Order("id DESC").Limit(num).Offset(startIdx).Find(&winners).Error
	return winners, total, err
}

// GetBlindBoxOverview 汇总全部期次与中奖记录。空表时 SUM 返回 NULL，统一用 COALESCE 兜底。
func GetBlindBoxOverview() (*BlindBoxOverview, error) {
	ov := &BlindBoxOverview{}

	var drawAgg struct {
		DrawCount         int64 `gorm:"column:draw_count"`
		TotalParticipant  int64 `gorm:"column:total_participant"`
		TotalWinner       int64 `gorm:"column:total_winner"`
		TotalConsumeQuota int64 `gorm:"column:total_consume_quota"`
		TotalPrizeQuota   int64 `gorm:"column:total_prize_quota"`
	}
	err := DB.Model(&BlindBoxDraw{}).
		Select("COUNT(*) as draw_count, " +
			"COALESCE(SUM(participant_count),0) as total_participant, " +
			"COALESCE(SUM(winner_count),0) as total_winner, " +
			"COALESCE(SUM(total_consume_quota),0) as total_consume_quota, " +
			"COALESCE(SUM(total_prize_quota),0) as total_prize_quota").
		Scan(&drawAgg).Error
	if err != nil {
		return nil, err
	}
	ov.DrawCount = drawAgg.DrawCount
	ov.TotalParticipant = drawAgg.TotalParticipant
	ov.TotalWinner = drawAgg.TotalWinner
	ov.TotalConsumeQuota = drawAgg.TotalConsumeQuota
	ov.TotalPrizeQuota = drawAgg.TotalPrizeQuota

	var statusRows []struct {
		Status string `gorm:"column:status"`
		Cnt    int64  `gorm:"column:cnt"`
		Quota  int64  `gorm:"column:quota"`
	}
	err = DB.Model(&BlindBoxWinner{}).
		Select("status, COUNT(*) as cnt, COALESCE(SUM(prize_quota),0) as quota").
		Group("status").Find(&statusRows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range statusRows {
		switch r.Status {
		case BlindBoxStatusClaimed:
			ov.ClaimedCount, ov.ClaimedQuota = r.Cnt, r.Quota
		case BlindBoxStatusPending:
			ov.PendingCount, ov.PendingQuota = r.Cnt, r.Quota
		case BlindBoxStatusExpired:
			ov.ExpiredCount, ov.ExpiredQuota = r.Cnt, r.Quota
		}
	}
	return ov, nil
}

// GetBlindBoxPeriodPreview 统计进行中周期 [periodStart, now) 的入池情况，
// 供管理员在开奖前预判今日奖池规模。口径与结算完全一致：报名 ∩ 达标。
// entryCount 为本期报名总人数（含尚未达标者），便于区分"没人报名"和"报了名没花够"。
func GetBlindBoxPeriodPreview(drawDate string, periodStart, now, threshold int64) (entryCount, count int, totalConsume int64, err error) {
	entryIds, err := GetBlindBoxEntryUserIds(drawDate)
	if err != nil {
		return 0, 0, 0, err
	}
	entryCount = len(entryIds)
	if entryCount == 0 {
		return 0, 0, 0, nil
	}
	qualified, err := getBlindBoxParticipants(periodStart, now, threshold)
	if err != nil {
		return entryCount, 0, 0, err
	}
	for _, p := range qualified {
		if _, ok := entryIds[p.UserId]; ok {
			count++
			totalConsume += p.Quota
		}
	}
	return entryCount, count, totalConsume, nil
}

// BuildBlindBoxPrizePool 导出给管理端预览奖池构成（与结算共用同一实现，保证一致）。
func BuildBlindBoxPrizePool(setting *operation_setting.BlindBoxSetting, totalConsume int64) []operation_setting.BlindBoxPrize {
	return buildBlindBoxPrizePool(setting, totalConsume)
}

// blindBoxWeight 由消耗额度换算权重 = max(1, round(美元))，保证达标者权重恒 ≥ 1。
func blindBoxWeight(quota int64) int64 {
	usd := float64(quota) / common.QuotaPerUnit
	w := int64(math.Round(usd))
	if w < 1 {
		w = 1
	}
	return w
}

// cryptoRandInt63n 返回 [0, n) 的加密安全随机数。n<=0 返回 0。
// crypto/rand 失败时退回基于纳秒的弱随机（极端兜底，不影响可用性）。
func cryptoRandInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	bi, err := crand.Int(crand.Reader, big.NewInt(n))
	if err != nil {
		return time.Now().UnixNano() % n
	}
	return bi.Int64()
}

// sampleWinnerIndices 加权无放回抽样：按 weights 抽出 k 个下标（抽中顺序）。
func sampleWinnerIndices(weights []int64, k int) []int {
	type item struct {
		idx int
		w   int64
	}
	pool := make([]item, len(weights))
	for i, w := range weights {
		pool[i] = item{idx: i, w: w}
	}
	winners := make([]int, 0, k)
	for len(winners) < k && len(pool) > 0 {
		var total int64
		for _, it := range pool {
			total += it.w
		}
		if total <= 0 {
			break
		}
		r := cryptoRandInt63n(total)
		var acc int64
		sel := len(pool) - 1
		for j, it := range pool {
			acc += it.w
			if r < acc {
				sel = j
				break
			}
		}
		winners = append(winners, pool[sel].idx)
		pool = append(pool[:sel], pool[sel+1:]...)
	}
	return winners
}

// buildBlindBoxPrizePool 根据配置与当期总消耗构建奖池（有序，优先大奖）。
func buildBlindBoxPrizePool(setting *operation_setting.BlindBoxSetting, totalConsume int64) []operation_setting.BlindBoxPrize {
	if setting.PoolMode == operation_setting.BlindBoxPoolModePercent {
		count := setting.PercentCount
		if count <= 0 {
			return nil
		}
		if count > operation_setting.MaxBlindBoxPrizes {
			count = operation_setting.MaxBlindBoxPrizes
		}
		pool := int64(float64(totalConsume) * setting.PercentRate)
		each := int(pool / int64(count))
		if each <= 0 {
			return nil
		}
		prizes := make([]operation_setting.BlindBoxPrize, count)
		for i := 0; i < count; i++ {
			prizes[i] = operation_setting.BlindBoxPrize{Name: "盲盒奖励", Quota: each}
		}
		return prizes
	}

	// 固定模式：过滤非法额度并按金额降序（大奖优先分配给先抽中者）。
	src := setting.GetPrizes()
	prizes := make([]operation_setting.BlindBoxPrize, 0, len(src))
	for _, p := range src {
		if p.Quota > 0 {
			prizes = append(prizes, p)
		}
	}
	for i := 0; i < len(prizes); i++ {
		for j := i + 1; j < len(prizes); j++ {
			if prizes[j].Quota > prizes[i].Quota {
				prizes[i], prizes[j] = prizes[j], prizes[i]
			}
		}
	}
	if len(prizes) > operation_setting.MaxBlindBoxPrizes {
		prizes = prizes[:operation_setting.MaxBlindBoxPrizes]
	}
	return prizes
}

// settleErrIfNotConflict 区分「唯一索引冲突（其它实例已结算，属正常竞态）」与真实故障。
// 前者返回 nil 静默放弃，后者必须把错误抛给调用方，否则一次真实故障会被当成"已结算"
// 吞掉：draw 行没写成、下一分钟又重试、每次都失败，运维却看不到任何告警。
func settleErrIfNotConflict(drawDate string, cause error) error {
	if exists, checkErr := blindBoxDrawExists(drawDate); checkErr == nil && exists {
		return nil
	}
	return cause
}

// SettleBlindBoxDraw 结算一个已闭合的开奖周期 [periodStart, periodEnd)。
// expireAt 为领取截止时点（下一次开奖时点），由调用方按日历运算给出。
// 幂等：draw_date 已存在则直接返回。仅生成 pending 中奖记录，不发放额度
// （发放在用户开盒时完成）。过期作废由 ExpireBlindBoxWinners 独立负责，不在此处耦合。
func SettleBlindBoxDraw(drawDate string, periodStart, periodEnd, expireAt int64) error {
	exists, err := blindBoxDrawExists(drawDate)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	setting := operation_setting.GetBlindBoxSetting()

	// 管理员改动开奖时间会让「上期开奖时点 → 本期开奖时点」不等于 24h，而 periodStart
	// 是按 boundary-24h 反推的，可能盖住上一期已结算过的区间，导致同一笔消耗被两期重复计入。
	// 这里只允许向后收缩、绝不向前扩展：向前扩展会在功能停用很久后把长期消耗并成一期。
	if last, lastErr := GetLastBlindBoxDraw(); lastErr == nil && last != nil {
		if last.PeriodEnd > periodStart && last.PeriodEnd < periodEnd {
			periodStart = last.PeriodEnd
		}
	}

	now := common.GetTimestamp()

	// 入池资格 = 本期已主动报名 AND 本期消耗达标，两者缺一不可。
	// 先取报名名单：没人报名就直接记空期，省掉一次对日志库的重聚合。
	entryIds, err := GetBlindBoxEntryUserIds(drawDate)
	if err != nil {
		return err
	}
	entryCount := len(entryIds)

	var participants []blindBoxParticipant
	if entryCount > 0 {
		qualified, qErr := getBlindBoxParticipants(periodStart, periodEnd, int64(setting.ThresholdQuota))
		if qErr != nil {
			return qErr
		}
		// 达标名单来自 LOG_DB，报名名单来自主库，两者可能是不同的库，只能在内存里取交集。
		participants = make([]blindBoxParticipant, 0, len(qualified))
		for _, p := range qualified {
			if _, ok := entryIds[p.UserId]; ok {
				participants = append(participants, p)
			}
		}
	}

	// 无人报名或报名者全部未达标 → 记一条空期（entry_count 仍要留档，便于运营判断是
	// 「没人报名」还是「报了名但没人花够钱」）。
	if len(participants) == 0 {
		empty := &BlindBoxDraw{
			DrawDate:    drawDate,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			EntryCount:  entryCount,
			PoolMode:    setting.PoolMode,
			Status:      BlindBoxDrawStatusEmpty,
			CreatedAt:   now,
		}
		if err := DB.Create(empty).Error; err != nil {
			return settleErrIfNotConflict(drawDate, err)
		}
		return nil
	}

	var totalConsume int64
	weights := make([]int64, len(participants))
	var totalWeight int64
	for i, p := range participants {
		totalConsume += p.Quota
		weights[i] = blindBoxWeight(p.Quota)
		totalWeight += weights[i]
	}

	pool := buildBlindBoxPrizePool(setting, totalConsume)
	k := len(pool)
	if k > len(participants) {
		k = len(participants)
	}

	// 奖池为空（如百分比算下来每份为 0，或固定奖池未配置）→ 记空期，不发奖。
	if k == 0 {
		empty := &BlindBoxDraw{
			DrawDate:          drawDate,
			PeriodStart:       periodStart,
			PeriodEnd:         periodEnd,
			EntryCount:        entryCount,
			ParticipantCount:  len(participants),
			TotalConsumeQuota: totalConsume,
			PoolMode:          setting.PoolMode,
			Status:            BlindBoxDrawStatusEmpty,
			CreatedAt:         now,
		}
		if err := DB.Create(empty).Error; err != nil {
			return settleErrIfNotConflict(drawDate, err)
		}
		return nil
	}

	winnerIdx := sampleWinnerIndices(weights, k)
	var totalPrize int64
	winners := make([]*BlindBoxWinner, 0, len(winnerIdx))
	for i, idx := range winnerIdx {
		p := participants[idx]
		prize := pool[i]
		prob := 0.0
		if totalWeight > 0 {
			prob = float64(weights[idx]) / float64(totalWeight)
		}
		totalPrize += int64(prize.Quota)
		winners = append(winners, &BlindBoxWinner{
			DrawDate:       drawDate,
			UserId:         p.UserId,
			Username:       p.Username,
			PrizeName:      prize.Name,
			PrizeQuota:     prize.Quota,
			ConsumeQuota:   p.Quota,
			WinProbability: prob,
			Status:         BlindBoxStatusPending,
			ExpireAt:       expireAt,
			CreatedAt:      now,
		})
	}

	draw := &BlindBoxDraw{
		DrawDate:          drawDate,
		PeriodStart:       periodStart,
		PeriodEnd:         periodEnd,
		EntryCount:        entryCount,
		ParticipantCount:  len(participants),
		WinnerCount:       len(winners),
		TotalConsumeQuota: totalConsume,
		TotalPrizeQuota:   totalPrize,
		PoolMode:          setting.PoolMode,
		Status:            BlindBoxDrawStatusDone,
		CreatedAt:         now,
	}

	// 结算只写主库两张表，不碰余额，单库事务即可。
	txErr := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(draw).Error; err != nil {
			return err // 唯一索引冲突 → 其它实例已结算，回滚放弃
		}
		for _, w := range winners {
			if err := tx.Create(w).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		if err := settleErrIfNotConflict(drawDate, txErr); err != nil {
			return err
		}
		// draw_date 已存在 = 其它实例已结算，属正常竞态，安静放弃。
		common.SysLog("blindbox: settle draw skipped, already settled: " + drawDate)
		return nil
	}
	common.SysLog("blindbox: settled draw " + drawDate + ", winners=" + strconv.Itoa(len(winners)))
	return nil
}

// ClaimBlindBox 用户主动开盒：把当前可领取的 pending 记录发放到余额并置为 claimed。
// 靠 WHERE status='pending' 条件更新保证并发安全（不重复发放、领取与过期不冲突）。
func ClaimBlindBox(userId int) (*BlindBoxWinner, error) {
	setting := operation_setting.GetBlindBoxSetting()
	if !setting.Enabled {
		return nil, errors.New("盲盒功能未启用")
	}
	now := common.GetTimestamp()

	var winner BlindBoxWinner
	err := DB.Where("user_id = ? AND status = ? AND expire_at > ?", userId, BlindBoxStatusPending, now).
		Order("draw_date DESC").First(&winner).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("暂无可开启的盲盒")
	}
	if err != nil {
		return nil, err
	}

	if common.UsingSQLite {
		return claimBlindBoxWithoutTransaction(&winner, userId, now)
	}
	return claimBlindBoxWithTransaction(&winner, userId, now)
}

// claimBlindBoxWithTransaction 事务开盒（MySQL / PostgreSQL）。
func claimBlindBoxWithTransaction(winner *BlindBoxWinner, userId int, now int64) (*BlindBoxWinner, error) {
	err := DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&BlindBoxWinner{}).
			Where("id = ? AND status = ?", winner.Id, BlindBoxStatusPending).
			Updates(map[string]interface{}{"status": BlindBoxStatusClaimed, "claimed_at": now})
		if res.Error != nil {
			return errors.New("开盒失败，请稍后重试")
		}
		if res.RowsAffected == 0 {
			return errors.New("该盲盒已被开启或已过期")
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota + ?", winner.PrizeQuota)).Error; err != nil {
			return errors.New("开盒失败：发放额度出错")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	winner.Status = BlindBoxStatusClaimed
	winner.ClaimedAt = now
	go func() {
		_ = cacheIncrUserQuota(userId, int64(winner.PrizeQuota))
	}()
	return winner, nil
}

// claimBlindBoxWithoutTransaction 无事务开盒（SQLite）。
func claimBlindBoxWithoutTransaction(winner *BlindBoxWinner, userId int, now int64) (*BlindBoxWinner, error) {
	res := DB.Model(&BlindBoxWinner{}).
		Where("id = ? AND status = ?", winner.Id, BlindBoxStatusPending).
		Updates(map[string]interface{}{"status": BlindBoxStatusClaimed, "claimed_at": now})
	if res.Error != nil {
		return nil, errors.New("开盒失败，请稍后重试")
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("该盲盒已被开启或已过期")
	}
	if err := IncreaseUserQuota(userId, winner.PrizeQuota, true); err != nil {
		// 发放失败，回滚状态
		DB.Model(&BlindBoxWinner{}).Where("id = ?", winner.Id).
			Updates(map[string]interface{}{"status": BlindBoxStatusPending, "claimed_at": 0})
		return nil, errors.New("开盒失败：发放额度出错")
	}
	winner.Status = BlindBoxStatusClaimed
	winner.ClaimedAt = now
	return winner, nil
}
