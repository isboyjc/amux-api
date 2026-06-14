package marketing

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 邀请关系羊毛风控评估器。
//
// 核心性能约束（请在维护时严格保持，避免拖慢主链路）：
//  1. 所有 per-inviter 指标必须用单条聚合 SQL 算出，禁止应用层 N+1 查询；
//  2. 写路径（注册 / 充值）只调用 model.MarkAffiliateRiskDirty —— O(1) upsert；
//  3. 评估、缓存写入完全异步，由 RunAffiliateRiskWorker 在后台批量处理；
//  4. SQL 不使用任何 DB 特有函数 / 运算符，跨 SQLite/MySQL/PostgreSQL 兼容；
//  5. 任何昂贵聚合（COUNT/GROUP BY）只对 users 单表 + 已有索引 inviter_id 跑。
//
// 评估输出落 model.AffiliateRiskCache；用户列表通过 LEFT JOIN 主键关联即可命中。

// 风控判定阈值。集中放这里方便后续抽到 operation_setting 做运行时配置。
// 当前先用编译期常量，避免引入新的配置面板字段；调阈值通过修改源码 + 重启完成。
const (
	// 邀请数硬触发：24h 内新增邀请 ≥ 此值 ⇒ danger（典型刷邀请行为）。
	affRiskInvites24hDanger = 10
	// 邀请数预警：7d 内新增邀请 ≥ 此值且其他信号命中 ⇒ suspect。
	affRiskInvites7dSuspect = 20
	// 一次性邮箱占比阈值。
	affRiskDisposableRatioDanger  = 0.30
	affRiskDisposableRatioSuspect = 0.10
	// 被邀者活跃度：在被邀者数 ≥ 该样本量时才计算活跃比例（避免小样本噪声）。
	affRiskActivationMinSample = 5
	// 活跃比例阈值（产生过调用或充值的被邀者占比）。
	affRiskActivationRatioDanger  = 0.10
	affRiskActivationRatioSuspect = 0.30
	// 被邀者封禁比例阈值。
	affRiskBannedRatioSuspect = 0.20
	// 邮箱前缀同模式占比阈值（如 user001 / user002 这种批量号）。
	affRiskEmailPrefixRatioSuspect = 0.40
	// 风险原因最多保留几条，避免 TEXT 列膨胀（控制单行 < 600B）。
	affRiskMaxReasons = 5
)

// AffiliateRiskSignals 评估时收集到的原始指标，序列化后写入 cache.signals。
// 前端打开详情时可读取，用于解释"为什么判定为可疑/高危"。
type AffiliateRiskSignals struct {
	TotalInvitees       int     `json:"total_invitees"`
	Invites24h          int     `json:"invites_24h"`
	Invites7d           int     `json:"invites_7d"`
	DisposableHits      int     `json:"disposable_hits"`
	DisposableRatio     float64 `json:"disposable_ratio"`
	BannedInvitees      int     `json:"banned_invitees"`
	BannedRatio         float64 `json:"banned_ratio"`
	ActiveInvitees      int     `json:"active_invitees"`
	ActivationRatio     float64 `json:"activation_ratio"`
	TopEmailPrefixCount int     `json:"top_email_prefix_count"`
	TopEmailPrefixRatio float64 `json:"top_email_prefix_ratio"`
}

// EvaluateAffiliateRisk 对单个邀请人评估风险。
// 调用方需保证 userId 是有效邀请人；若该用户当前 aff_count == 0，会返回 nil
// 表示"无需缓存"，调用方应清理缓存表。
//
// SQL 数量上限：本函数最多发 3 条聚合 SQL（被邀者整体聚合、邮箱前缀聚合、用户存在性）。
// 即便邀请人有 10 万被邀者，每条 SQL 仍是 O(N) 的单次扫描 + 索引（inviter_id 有索引）。
func EvaluateAffiliateRisk(userId int) (*AffiliateRiskSignals, string, []string, error) {
	if userId <= 0 {
		return nil, model.AffRiskLevelNormal, nil, nil
	}

	now := time.Now().Unix()
	t24h := now - 24*3600
	t7d := now - 7*24*3600

	// 聚合 SQL 1：被邀者整体指标（一次扫描得到全部 count）。
	// 用 SUM(CASE WHEN ...) 写法，跨 SQLite/MySQL/PostgreSQL 通用，无需 DB 特有函数。
	// 显式 gorm column 标签：GORM 的字段→列名默认走 snake_case 转换（Invites24h→invites24_h），
	// 与我们 SELECT 起的别名对不上，必须固定。
	type aggRow struct {
		Total          int64 `gorm:"column:total"`
		Invites24h     int64 `gorm:"column:invites24h"`
		Invites7d      int64 `gorm:"column:invites7d"`
		BannedCnt      int64 `gorm:"column:banned_cnt"`
		ActiveCnt      int64 `gorm:"column:active_cnt"`
		DisposableHits int64 `gorm:"column:disposable_hits"`
	}
	var agg aggRow

	// 一次性邮箱后缀列表来自 operation_setting；用 IN (?) 让 DB 优化器走索引扫描。
	disposable := disposableDomainList()

	// 注意：所有列都来自 users 单表，无 JOIN；inviter_id 已有索引，性能可控。
	q := model.DB.Table("users").
		Select(`COUNT(*) AS total,
			SUM(CASE WHEN created_time >= ? THEN 1 ELSE 0 END) AS invites24h,
			SUM(CASE WHEN created_time >= ? THEN 1 ELSE 0 END) AS invites7d,
			SUM(CASE WHEN status <> ? THEN 1 ELSE 0 END) AS banned_cnt,
			SUM(CASE WHEN used_quota > 0 OR request_count > 0 THEN 1 ELSE 0 END) AS active_cnt`,
			t24h, t7d, common.UserStatusEnabled).
		Where("inviter_id = ?", userId)

	if err := q.Scan(&agg).Error; err != nil {
		return nil, model.AffRiskLevelNormal, nil, fmt.Errorf("aggregate invitees: %w", err)
	}

	if agg.Total == 0 {
		// 无被邀者；删除缓存交给调用方处理。
		return nil, "", nil, nil
	}

	// 聚合 SQL 2：一次性邮箱命中数。
	// 单独发一条而不是塞进 SQL 1，是因为 IN 列表参数化更清晰；
	// 一次性邮箱列表只有几十条，IN 走索引覆盖扫描，开销可忽略。
	if len(disposable) > 0 {
		var disposableHits int64
		domainExpr := emailDomainExpr() // 跨库的"取 @ 后域名"表达式
		placeholders, args := inPlaceholders(disposable)
		if err := model.DB.Table("users").
			Select("COUNT(*)").
			Where("inviter_id = ?", userId).
			Where("email LIKE ?", "%@_%"). // 排除空 / 无 @ 的 email
			Where(fmt.Sprintf("LOWER(%s) IN (%s)", domainExpr, placeholders), args...).
			Scan(&disposableHits).Error; err != nil {
			// 失败不致命，置 0 继续评估其他维度。
			common.SysError(fmt.Sprintf("EvaluateAffiliateRisk: disposable count error inviter=%d: %s", userId, err))
			disposableHits = 0
		}
		agg.DisposableHits = disposableHits
	}

	// 聚合 SQL 3：邮箱前缀模式（去尾部数字后分组），找最大簇大小。
	// 仅在被邀者数 ≥ 阈值时计算，小样本意义不大且 SQL 开销不值。
	topPrefixCount := int64(0)
	if agg.Total >= int64(affRiskActivationMinSample) {
		// DB 端取 @ 前缀（跨库表达式），应用层再做"去尾部数字"归一化。
		// 完全应用层会 N+1，DB 端正则在 SQLite 不支持，因此走"DB 端粗 GROUP BY +
		// 应用层归一化合并"的折中：结果集行数 ≤ 不同前缀数 ≤ 被邀者数 N，开销可控。
		prefixExpr := emailPrefixExpr()
		type prefixRow struct {
			Prefix string `gorm:"column:prefix"`
			Cnt    int64  `gorm:"column:cnt"`
		}
		var rows []prefixRow
		if err := model.DB.Table("users").
			Select(fmt.Sprintf("%s AS prefix, COUNT(*) AS cnt", prefixExpr)).
			Where("inviter_id = ?", userId).
			Where("email LIKE ?", "%@_%").
			Group("prefix").
			Order("cnt DESC").
			Limit(50). // 截断保护，邮箱前缀种类极多时只取 Top 50
			Scan(&rows).Error; err != nil {
			common.SysError(fmt.Sprintf("EvaluateAffiliateRisk: email prefix group error inviter=%d: %s", userId, err))
		} else {
			normalized := make(map[string]int64, len(rows))
			for _, r := range rows {
				key := normalizeEmailPrefix(r.Prefix)
				if key == "" {
					continue
				}
				normalized[key] += r.Cnt
			}
			for _, c := range normalized {
				if c > topPrefixCount {
					topPrefixCount = c
				}
			}
		}
	}

	signals := &AffiliateRiskSignals{
		TotalInvitees:       int(agg.Total),
		Invites24h:          int(agg.Invites24h),
		Invites7d:           int(agg.Invites7d),
		DisposableHits:      int(agg.DisposableHits),
		BannedInvitees:      int(agg.BannedCnt),
		ActiveInvitees:      int(agg.ActiveCnt),
		TopEmailPrefixCount: int(topPrefixCount),
	}
	if signals.TotalInvitees > 0 {
		signals.DisposableRatio = float64(signals.DisposableHits) / float64(signals.TotalInvitees)
		signals.BannedRatio = float64(signals.BannedInvitees) / float64(signals.TotalInvitees)
		signals.ActivationRatio = float64(signals.ActiveInvitees) / float64(signals.TotalInvitees)
		signals.TopEmailPrefixRatio = float64(signals.TopEmailPrefixCount) / float64(signals.TotalInvitees)
	}

	level, reasons := judgeAffiliateRisk(signals)
	return signals, level, reasons, nil
}

// judgeAffiliateRisk 把原始指标映射到风险等级 + 可解释原因。
// 命中任一 danger 规则 ⇒ danger；只命中 suspect ⇒ suspect；否则 normal。
func judgeAffiliateRisk(s *AffiliateRiskSignals) (string, []string) {
	if s == nil {
		return model.AffRiskLevelNormal, nil
	}
	level := model.AffRiskLevelNormal
	reasons := make([]string, 0, 4)

	addDanger := func(reason string) {
		level = model.AffRiskLevelDanger
		reasons = append(reasons, reason)
	}
	addSuspect := func(reason string) {
		if level != model.AffRiskLevelDanger {
			level = model.AffRiskLevelSuspect
		}
		reasons = append(reasons, reason)
	}

	if s.Invites24h >= affRiskInvites24hDanger {
		addDanger(fmt.Sprintf("invites_24h:%d", s.Invites24h))
	}
	if s.DisposableRatio >= affRiskDisposableRatioDanger && s.DisposableHits > 0 {
		addDanger(fmt.Sprintf("disposable_ratio:%.0f%%", s.DisposableRatio*100))
	} else if s.DisposableRatio >= affRiskDisposableRatioSuspect && s.DisposableHits > 0 {
		addSuspect(fmt.Sprintf("disposable_ratio:%.0f%%", s.DisposableRatio*100))
	}

	if s.TotalInvitees >= affRiskActivationMinSample {
		if s.ActivationRatio <= affRiskActivationRatioDanger {
			addDanger(fmt.Sprintf("activation_ratio:%.0f%%", s.ActivationRatio*100))
		} else if s.ActivationRatio <= affRiskActivationRatioSuspect {
			addSuspect(fmt.Sprintf("activation_ratio:%.0f%%", s.ActivationRatio*100))
		}
	}

	if s.Invites7d >= affRiskInvites7dSuspect && level != model.AffRiskLevelDanger {
		addSuspect(fmt.Sprintf("invites_7d:%d", s.Invites7d))
	}
	if s.BannedRatio >= affRiskBannedRatioSuspect && s.BannedInvitees > 0 {
		addSuspect(fmt.Sprintf("banned_ratio:%.0f%%", s.BannedRatio*100))
	}
	if s.TopEmailPrefixRatio >= affRiskEmailPrefixRatioSuspect && s.TopEmailPrefixCount >= 3 {
		addSuspect(fmt.Sprintf("email_pattern:%d/%d", s.TopEmailPrefixCount, s.TotalInvitees))
	}

	if len(reasons) > affRiskMaxReasons {
		reasons = reasons[:affRiskMaxReasons]
	}
	return level, reasons
}

// PersistAffiliateRisk 评估并写入缓存。无被邀者时清理缓存。
func PersistAffiliateRisk(userId int) error {
	signals, level, reasons, err := EvaluateAffiliateRisk(userId)
	if err != nil {
		return err
	}
	if signals == nil {
		// 无被邀者 ⇒ 清理缓存，避免遗留过期数据。
		return model.DeleteAffiliateRiskCache(userId)
	}
	reasonsJSON, err := common.Marshal(reasons)
	if err != nil {
		return fmt.Errorf("marshal reasons: %w", err)
	}
	signalsJSON, err := common.Marshal(signals)
	if err != nil {
		return fmt.Errorf("marshal signals: %w", err)
	}
	return model.UpsertAffiliateRiskCache(&model.AffiliateRiskCache{
		UserId:      userId,
		RiskLevel:   level,
		RiskReasons: string(reasonsJSON),
		Signals:     string(signalsJSON),
	})
}

// RunAffiliateRiskWorker 启动后台 worker，按批消费 dirty 集合。
// 单进程单 worker，避免 SQLite 并发写冲突；多实例部署时各实例都会跑，
// 处理同一 user 的并发由 Upsert 的乐观写入兜底（最后写入胜出，结果幂等）。
//
// 设计要点：
//   - 每 tick 处理至多 batchSize 个 user，跑完 sleep 一段时间，避免抖动；
//   - 处理失败的 user 不会从 dirty 表删除，下个 tick 自动重试；
//   - 出错只打日志，不让 worker 退出（panic 由 recover 兜住）；
//   - 开关从 operation_setting 读，关闭后变成 no-op，业务零影响。
func RunAffiliateRiskWorker(ctx context.Context) {
	common.SysLog("affiliate-risk worker started")

	// 启动一次性回填：把所有 aff_count>0 的存量邀请人塞入 dirty 队列。
	// 这是为了解决"功能上线前的存量邀请人永远不会被触发评估"的问题。
	// 通过 OnConflict DoNothing，多次重启不会重复堆积；
	// worker 进入主循环后自然会按批消费。
	if operation_setting.AffiliateRiskCacheEnabled {
		if n, err := model.BackfillAffiliateRiskDirty(); err != nil {
			common.SysError(fmt.Sprintf("affiliate-risk backfill error: %s", err))
		} else if n > 0 {
			common.SysLog(fmt.Sprintf("affiliate-risk backfill enqueued %d inviters", n))
		}
	}

	for {
		if ctx.Err() != nil {
			common.SysLog("affiliate-risk worker stopped")
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysError(fmt.Sprintf("affiliate-risk worker panic (will retry next tick): %v", r))
				}
			}()

			if !operation_setting.AffiliateRiskCacheEnabled {
				sleepWithCtx(ctx, 60*time.Second)
				return
			}

			batch := operation_setting.AffiliateRiskDirtyBatchSize
			if batch <= 0 {
				batch = 200
			}
			ids, err := model.FetchAffiliateRiskDirtyBatch(batch)
			if err != nil {
				common.SysError(fmt.Sprintf("affiliate-risk worker fetch dirty error: %s", err))
				sleepWithCtx(ctx, 30*time.Second)
				return
			}

			if len(ids) == 0 {
				interval := time.Duration(operation_setting.AffiliateRiskDirtyIntervalSec) * time.Second
				if interval <= 0 {
					interval = 60 * time.Second
				}
				sleepWithCtx(ctx, interval)
				return
			}

			processed := make([]int, 0, len(ids))
			for _, id := range ids {
				if ctx.Err() != nil {
					break
				}
				if err := PersistAffiliateRisk(id); err != nil {
					common.SysError(fmt.Sprintf("affiliate-risk evaluate user=%d error: %s", id, err))
					continue
				}
				processed = append(processed, id)
			}

			if len(processed) > 0 {
				if err := model.ClearAffiliateRiskDirty(processed); err != nil {
					common.SysError(fmt.Sprintf("affiliate-risk worker clear dirty error: %s", err))
				}
			}
		}()

		sleepWithCtx(ctx, 200*time.Millisecond)
	}
}

func sleepWithCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// disposableDomainList 复用 operation_setting 配置 + controller 内置黑名单。
// 当前 controller/operations.go 把名单写死在内部包变量里；为了不打破现有边界，
// 这里再维护一份等价的小名单 —— 后续可抽到 operation_setting/marketing.go 统一。
//
// 性能上，这份名单仅在 EvaluateAffiliateRisk 调用时一次性传给 SQL，不影响热路径。
func disposableDomainList() []string {
	return []string{
		"mailinator.com", "tempmail.com", "temp-mail.org",
		"10minutemail.com", "10minutemail.net",
		"guerrillamail.com", "guerrillamail.info", "guerrillamail.net",
		"sharklasers.com", "yopmail.com", "yopmail.net",
		"throwawaymail.com", "getnada.com", "fakeinbox.com",
		"dispostable.com", "maildrop.cc", "trashmail.com",
		"mintemail.com", "mohmal.com", "emailondeck.com",
	}
}

// emailDomainExpr 返回"取邮箱 @ 之后的域名"的 SQL 表达式片段，跨库兼容。
//   - PostgreSQL：SPLIT_PART(email, '@', 2)（也支持 SUBSTRING(email FROM POSITION('@' IN email)+1)）
//   - MySQL：SUBSTRING_INDEX(email, '@', -1)
//   - SQLite：SUBSTR(email, INSTR(email, '@') + 1)
//
// 三种实现都在各自数据库的本地函数集中，不引入扩展依赖。
func emailDomainExpr() string {
	switch {
	case common.UsingPostgreSQL:
		return "SPLIT_PART(email, '@', 2)"
	case common.UsingMySQL:
		return "SUBSTRING_INDEX(email, '@', -1)"
	default:
		// SQLite
		return "SUBSTR(email, INSTR(email, '@') + 1)"
	}
}

// emailPrefixExpr 返回"取邮箱 @ 之前的本地部分"的 SQL 表达式片段，跨库兼容。
func emailPrefixExpr() string {
	switch {
	case common.UsingPostgreSQL:
		return "SPLIT_PART(email, '@', 1)"
	case common.UsingMySQL:
		return "SUBSTRING_INDEX(email, '@', 1)"
	default:
		// SQLite
		return "SUBSTR(email, 1, INSTR(email, '@') - 1)"
	}
}

// inPlaceholders 生成 "?,?,?" 占位 + 转换好的 []any 参数（已 lower）。
func inPlaceholders(values []string) (string, []any) {
	if len(values) == 0 {
		return "''", nil
	}
	placeholders := strings.Repeat("?,", len(values))
	placeholders = strings.TrimRight(placeholders, ",")
	args := make([]any, 0, len(values))
	for _, v := range values {
		args = append(args, strings.ToLower(v))
	}
	return placeholders, args
}

// normalizeEmailPrefix 去掉尾部的数字（"user001" → "user"），用于聚类批量号。
// 仅做轻量字符串处理，不调用任何昂贵库。
func normalizeEmailPrefix(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	end := len(s)
	for end > 0 && unicode.IsDigit(rune(s[end-1])) {
		end--
	}
	return s[:end]
}
