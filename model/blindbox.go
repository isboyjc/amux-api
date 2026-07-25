package model

import (
	crand "crypto/rand"
	"errors"
	"math"
	"math/big"
	"sort"
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
	EntryCount        int    `json:"entry_count" gorm:"default:0"`                                        // 本期报名人数
	ParticipantCount  int    `json:"participant_count" gorm:"default:0"`                                  // 报名且消耗达标、真正进入奖池的人数
	BlockedCount      int    `json:"blocked_count" gorm:"default:0"`                                      // 达标但被屏蔽中奖资格、未进入奖池的人数
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

// 屏蔽范围。
const (
	BlindBoxBlockScopePeriod    = "period"    // 只屏蔽某一期
	BlindBoxBlockScopePermanent = "permanent" // 永久屏蔽，开关语义：行在即生效
)

// BlindBoxBlock 中奖资格屏蔽记录（风控 / 违规惩罚用）。
//
// 语义边界必须清楚：它只决定「能不能中奖」，不影响用户报名、消耗统计、开盒或任何
// 其它功能。被屏蔽者照常参与、照常看到"等待开奖"，开奖后表现为未中奖，与真正没抽中
// 无法区分——这是风控手段，不是给中奖结果做手脚（已结算的期次一律不允许再屏蔽）。
//
// 永久屏蔽用 draw_date 空串占位，不用 NULL：MySQL/PostgreSQL 的唯一索引不约束
// NULL，用 NULL 会让同一个用户被重复写入多行永久屏蔽。
// 解除屏蔽直接删行（永久屏蔽是"开启才生效"的开关），操作留痕走 SysLog。
type BlindBoxBlock struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Scope        string `json:"scope" gorm:"type:varchar(16);not null;uniqueIndex:idx_bbb_scope_date_user,priority:1"`
	DrawDate     string `json:"draw_date" gorm:"type:varchar(10);default:'';uniqueIndex:idx_bbb_scope_date_user,priority:2"`
	UserId       int    `json:"user_id" gorm:"not null;uniqueIndex:idx_bbb_scope_date_user,priority:3;index:idx_bbb_user"`
	Username     string `json:"username" gorm:"type:varchar(64);default:''"`
	Reason       string `json:"reason" gorm:"type:varchar(255);default:''"`
	OperatorId   int    `json:"operator_id" gorm:"default:0"`
	OperatorName string `json:"operator_name" gorm:"type:varchar(64);default:''"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
}

func (BlindBoxBlock) TableName() string {
	return "blind_box_blocks"
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
// 必须只按 user_id 分组：logs.username 是行内冗余字段（且 default:”），用户改名或
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

// GetBlindBoxEntries 返回某一期的全部报名记录，按报名时间正序。
// 管理端「当期参与明细」的主线：明细以报名为准，消耗再从日志库补齐。
func GetBlindBoxEntries(drawDate string) ([]BlindBoxEntry, error) {
	var entries []BlindBoxEntry
	err := DB.Where("draw_date = ?", drawDate).Order("created_at ASC, id ASC").Find(&entries).Error
	return entries, err
}

// blindBoxConsumeChunkSize 单次 IN 查询携带的用户数上限。
const blindBoxConsumeChunkSize = 500

// sumBlindBoxConsumeByUsers 取指定用户在 [periodStart, periodEnd) 内的消耗合计。
//
// 刻意按 user_id IN 分块查询，而不是像结算那样对整个周期做一次全量 GROUP BY：
// 结算有 HAVING SUM(quota) >= threshold 帮着削掉绝大多数行，明细页要的是「每个报名者
// 的真实消耗（含门槛调高后已不达标的）」，没有 HAVING 可用，全量聚合会把当天所有消费
// 用户都捞回内存。logs.user_id 有索引，按报名名单分块反查的代价只与报名人数相关。
func sumBlindBoxConsumeByUsers(periodStart, periodEnd int64, userIds []int) (map[int]int64, error) {
	res := make(map[int]int64, len(userIds))
	for start := 0; start < len(userIds); start += blindBoxConsumeChunkSize {
		end := start + blindBoxConsumeChunkSize
		if end > len(userIds) {
			end = len(userIds)
		}
		var rows []struct {
			UserId int   `gorm:"column:user_id"`
			Quota  int64 `gorm:"column:quota"`
		}
		err := LOG_DB.Table("logs").
			Select("user_id, SUM(quota) as quota").
			Where("type = ? AND created_at >= ? AND created_at < ? AND user_id IN ?",
				LogTypeConsume, periodStart, periodEnd, userIds[start:end]).
			Group("user_id").
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			res[r.UserId] = r.Quota
		}
	}
	return res, nil
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

// ===================== 中奖资格屏蔽 =====================

// CreateBlindBoxBlock 新增一条屏蔽记录。已存在（唯一索引冲突）返回 created=false。
func CreateBlindBoxBlock(block *BlindBoxBlock) (created bool, err error) {
	if err := DB.Create(block).Error; err != nil {
		// 与报名同理：不同驱动的冲突错误码不统一，回查一次更可靠。
		if exists, checkErr := blindBoxBlockExists(block.Scope, block.DrawDate, block.UserId); checkErr == nil && exists {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func blindBoxBlockExists(scope, drawDate string, userId int) (bool, error) {
	var count int64
	err := DB.Model(&BlindBoxBlock{}).
		Where("scope = ? AND draw_date = ? AND user_id = ?", scope, drawDate, userId).
		Count(&count).Error
	return count > 0, err
}

// GetBlindBoxBlockById 按主键取一条屏蔽记录，不存在返回 nil。
func GetBlindBoxBlockById(id int) (*BlindBoxBlock, error) {
	var b BlindBoxBlock
	err := DB.Where("id = ?", id).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// DeleteBlindBoxBlock 解除屏蔽（删行）。返回是否真的删掉了一行。
func DeleteBlindBoxBlock(id int) (bool, error) {
	res := DB.Where("id = ?", id).Delete(&BlindBoxBlock{})
	return res.RowsAffected > 0, res.Error
}

// GetBlindBoxBlockedUserIds 返回 drawDate 这一期不具备中奖资格的用户集合
// = 永久屏蔽 ∪ 本期屏蔽。结算与管理端预览共用，保证两边口径一致。
func GetBlindBoxBlockedUserIds(drawDate string) (map[int]struct{}, error) {
	var ids []int
	err := DB.Model(&BlindBoxBlock{}).
		Where("scope = ? OR (scope = ? AND draw_date = ?)",
			BlindBoxBlockScopePermanent, BlindBoxBlockScopePeriod, drawDate).
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

// getBlindBoxPeriodBlocks 取某一期生效的屏蔽记录，按 scope 分别建索引，
// 供明细页展示屏蔽原因并直接拿到可解除的记录 id。
func getBlindBoxPeriodBlocks(drawDate string) (period, permanent map[int]BlindBoxBlock, err error) {
	var rows []BlindBoxBlock
	err = DB.Where("scope = ? OR (scope = ? AND draw_date = ?)",
		BlindBoxBlockScopePermanent, BlindBoxBlockScopePeriod, drawDate).Find(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	period = make(map[int]BlindBoxBlock)
	permanent = make(map[int]BlindBoxBlock)
	for _, r := range rows {
		if r.Scope == BlindBoxBlockScopePermanent {
			permanent[r.UserId] = r
		} else {
			period[r.UserId] = r
		}
	}
	return period, permanent, nil
}

// SearchBlindBoxBlocks 分页返回屏蔽名单，支持按范围 / 用户（ID 或用户名）筛选。
func SearchBlindBoxBlocks(scope, keyword string, startIdx, num int) ([]BlindBoxBlock, int64, error) {
	tx := DB.Model(&BlindBoxBlock{})
	if scope != "" {
		tx = tx.Where("scope = ?", scope)
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
	var rows []BlindBoxBlock
	err := tx.Order("id DESC").Limit(num).Offset(startIdx).Find(&rows).Error
	return rows, total, err
}

// ===================== 当期（未开奖）实时明细 =====================

// BlindBoxPeriodParticipant 当期单个报名者的实时状态。
type BlindBoxPeriodParticipant struct {
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	JoinedAt     int64  `json:"joined_at"`
	ConsumeQuota int64  `json:"consume_quota"`
	Qualified    bool   `json:"qualified"` // 按当前门槛是否达标
	Weight       int64  `json:"weight"`    // 加权抽样的权重，被屏蔽者为 0

	// FirstProb 首抽中奖概率 = weight / 有效总权重，与 BlindBoxWinner.WinProbability 同口径。
	FirstProb float64 `json:"first_prob"`
	// AnyProb 至少中一次的估算值。无放回加权抽样没有闭式解，用 1-(1-p)^k 近似，仅供参考。
	AnyProb float64 `json:"any_prob"`

	BlockedPeriod    bool   `json:"blocked_period"`
	BlockedPermanent bool   `json:"blocked_permanent"`
	PeriodBlockId    int    `json:"period_block_id"`
	PermanentBlockId int    `json:"permanent_block_id"`
	BlockReason      string `json:"block_reason"`
}

// BlindBoxPeriodDetail 当期实时推演结果。口径与 SettleBlindBoxDraw 完全一致：
// 入池 = 报名 ∩ 达标 ∩ 未被屏蔽，被屏蔽者连同其消耗一并移出奖池计算。
type BlindBoxPeriodDetail struct {
	Participants    []BlindBoxPeriodParticipant       `json:"participants"`
	EntryCount      int                               `json:"entry_count"`       // 报名总人数
	QualifiedCount  int                               `json:"qualified_count"`   // 报名且达标
	BlockedCount    int                               `json:"blocked_count"`     // 其中被屏蔽的人数
	EffectiveCount  int                               `json:"effective_count"`   // 真正进入奖池的人数
	TotalConsume    int64                             `json:"total_consume"`     // 入池者消耗合计（奖池计算口径）
	BlockedConsume  int64                             `json:"blocked_consume"`   // 被屏蔽者消耗合计（不计入奖池）
	PrizeCount      int                               `json:"prize_count"`       // 奖池奖项数
	WinnerCount     int                               `json:"winner_count"`      // 实际可开出的中奖名额
	TotalPrizeQuota int64                             `json:"total_prize_quota"` // 奖池总额
	Prizes          []operation_setting.BlindBoxPrize `json:"prizes"`
}

// GetBlindBoxPeriodDetail 推演 [periodStart, periodEnd) 这一期的实时入池与中奖概率。
//
// 管理端「当期明细」和「规则统计」的预览共用这一个函数：两处若各算一遍，
// 一旦屏蔽/门槛口径改动就会出现"预览奖池"和"实际开奖"对不上的情况。
func GetBlindBoxPeriodDetail(setting *operation_setting.BlindBoxSetting, drawDate string, periodStart, periodEnd int64) (*BlindBoxPeriodDetail, error) {
	detail := &BlindBoxPeriodDetail{Participants: []BlindBoxPeriodParticipant{}}

	entries, err := GetBlindBoxEntries(drawDate)
	if err != nil {
		return nil, err
	}
	detail.EntryCount = len(entries)
	if len(entries) == 0 {
		return detail, nil
	}

	userIds := make([]int, 0, len(entries))
	for _, e := range entries {
		userIds = append(userIds, e.UserId)
	}
	// 日志库/屏蔽表查不动时仍把已知的 entry_count 带回去：调用方据 err 标记数据不可用，
	// 但"本期有多少人报名"是主库里的确定事实，不该被一次日志库故障一起清零。
	consume, err := sumBlindBoxConsumeByUsers(periodStart, periodEnd, userIds)
	if err != nil {
		return detail, err
	}
	periodBlocks, permanentBlocks, err := getBlindBoxPeriodBlocks(drawDate)
	if err != nil {
		return detail, err
	}

	threshold := int64(setting.ThresholdQuota)
	participants := make([]BlindBoxPeriodParticipant, 0, len(entries))
	var totalWeight int64
	for _, e := range entries {
		p := BlindBoxPeriodParticipant{
			UserId:       e.UserId,
			Username:     e.Username,
			JoinedAt:     e.CreatedAt,
			ConsumeQuota: consume[e.UserId],
		}
		p.Qualified = p.ConsumeQuota >= threshold
		if b, ok := periodBlocks[e.UserId]; ok {
			p.BlockedPeriod, p.PeriodBlockId, p.BlockReason = true, b.Id, b.Reason
		}
		if b, ok := permanentBlocks[e.UserId]; ok {
			p.BlockedPermanent, p.PermanentBlockId = true, b.Id
			if p.BlockReason == "" {
				p.BlockReason = b.Reason
			}
		}
		blocked := p.BlockedPeriod || p.BlockedPermanent
		if p.Qualified {
			detail.QualifiedCount++
			if blocked {
				detail.BlockedCount++
				detail.BlockedConsume += p.ConsumeQuota
			} else {
				detail.EffectiveCount++
				detail.TotalConsume += p.ConsumeQuota
				p.Weight = blindBoxWeight(p.ConsumeQuota)
				totalWeight += p.Weight
			}
		}
		// 未达标者即便被屏蔽也不计入 BlockedCount：该字段统计的是"本来能中、被拦下"的人数
		participants = append(participants, p)
	}

	// 奖池只按有效入池者的消耗构建，与结算一致
	detail.Prizes = buildBlindBoxPrizePool(setting, detail.TotalConsume)
	detail.PrizeCount = len(detail.Prizes)
	for _, pz := range detail.Prizes {
		detail.TotalPrizeQuota += int64(pz.Quota)
	}
	detail.WinnerCount = detail.PrizeCount
	if detail.WinnerCount > detail.EffectiveCount {
		detail.WinnerCount = detail.EffectiveCount
	}

	// 概率必须在剔除屏蔽者之后再算：屏蔽的直接效果就是其余人的概率同步上浮。
	for i := range participants {
		if participants[i].Weight <= 0 || totalWeight <= 0 || detail.WinnerCount <= 0 {
			continue
		}
		participants[i].FirstProb = float64(participants[i].Weight) / float64(totalWeight)
		participants[i].AnyProb = 1 - math.Pow(1-participants[i].FirstProb, float64(detail.WinnerCount))
	}

	// 按消耗降序，运营最关心的大额用户排在最前
	sort.SliceStable(participants, func(i, j int) bool {
		return participants[i].ConsumeQuota > participants[j].ConsumeQuota
	})
	detail.Participants = participants
	return detail, nil
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

	// 中奖资格屏蔽（风控 / 违规惩罚）：本期屏蔽 ∪ 永久屏蔽的用户整个移出奖池。
	// 连同其消耗一并剔除，而不是只把人拿掉：percent 模式下奖池 = 总消耗 × 比例，
	// 保留被屏蔽者的消耗等于用一笔不可能中奖的消耗凭空放大其他人分的奖池。
	// 屏蔽只影响中奖资格，报名与消耗统计一律不受影响。
	blockedCount := 0
	if len(participants) > 0 {
		blocked, bErr := GetBlindBoxBlockedUserIds(drawDate)
		if bErr != nil {
			return bErr
		}
		if len(blocked) > 0 {
			kept := make([]blindBoxParticipant, 0, len(participants))
			for _, p := range participants {
				if _, ok := blocked[p.UserId]; ok {
					blockedCount++
					continue
				}
				kept = append(kept, p)
			}
			participants = kept
		}
	}

	// 无人报名或报名者全部未达标 → 记一条空期（entry_count 仍要留档，便于运营判断是
	// 「没人报名」还是「报了名但没人花够钱」）。
	if len(participants) == 0 {
		empty := &BlindBoxDraw{
			DrawDate:     drawDate,
			PeriodStart:  periodStart,
			PeriodEnd:    periodEnd,
			EntryCount:   entryCount,
			BlockedCount: blockedCount,
			PoolMode:     setting.PoolMode,
			Status:       BlindBoxDrawStatusEmpty,
			CreatedAt:    now,
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
			BlockedCount:      blockedCount,
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
		BlockedCount:      blockedCount,
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
