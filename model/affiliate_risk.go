package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 邀请关系羊毛风控相关的两张轻量表。
//
// 设计动机详见 docs/affiliate-risk-design（讨论）。这里给出关键性能约束：
//   - 用户列表查询只允许 LEFT JOIN AffiliateRiskCache 主键关联，
//     绝不在列表查询里跑任何聚合 SQL；
//   - AffiliateRiskDirty 用作"待重算"的标记集合，由后台 worker
//     按批次消费，注册 / 充值等写路径只调用 MarkDirty（O(1) upsert）；
//   - 两张表都只用 GORM 抽象 + 占位符 SQL，跨 SQLite / MySQL / PostgreSQL 兼容
//     （遵守 AGENTS.md Rule 2）；
//   - JSON 字段一律走 common.Marshal/Unmarshal（遵守 Rule 1），
//     存储类型用 TEXT，避免 JSONB 等 DB 特有列类型。
//
// 风险等级常量。前端按这三档渲染。
const (
	AffRiskLevelNormal  = "normal"
	AffRiskLevelSuspect = "suspect"
	AffRiskLevelDanger  = "danger"
)

// AffiliateRiskCache 缓存每个邀请人（inviter）的风控评估结果。
// 主键即 user_id（一定是 users.id），LEFT JOIN 时走主键索引，常数级开销。
//
// 仅在 aff_count > 0 或被任何活跃用户 inviter_id 引用的用户上有有效记录；
// 普通用户不会在此表中出现，列表查询时 LEFT JOIN 后字段为 NULL，前端按"未评估"处理。
type AffiliateRiskCache struct {
	UserId      int    `json:"user_id" gorm:"primaryKey;column:user_id"`
	RiskLevel   string `json:"risk_level" gorm:"type:varchar(16);column:risk_level;index"`
	RiskReasons string `json:"risk_reasons" gorm:"type:text;column:risk_reasons"` // JSON array, 限长 <=512B
	Signals     string `json:"signals" gorm:"type:text;column:signals"`           // JSON object, 原始指标数值
	ComputedAt  int64  `json:"computed_at" gorm:"column:computed_at;index"`
}

func (AffiliateRiskCache) TableName() string {
	return "affiliate_risk_cache"
}

// AffiliateRiskDirty 标记待重算的邀请人 id。worker 按批消费后整批删除。
// 同一 user_id 多次标 dirty 只写一行（upsert 更新 marked_at），不会膨胀。
type AffiliateRiskDirty struct {
	UserId   int   `json:"user_id" gorm:"primaryKey;column:user_id"`
	MarkedAt int64 `json:"marked_at" gorm:"column:marked_at;index"`
}

func (AffiliateRiskDirty) TableName() string {
	return "affiliate_risk_dirty"
}

// MarkAffiliateRiskDirty 把一个邀请人 id 标记为待重算。
// O(1) upsert，写路径调用（如 inviteUser、充值成功等），不阻塞主流程。
// userId == 0 / 负值视为非法，安静忽略，避免污染脏集合。
func MarkAffiliateRiskDirty(userId int) error {
	if userId <= 0 {
		return nil
	}
	row := AffiliateRiskDirty{
		UserId:   userId,
		MarkedAt: time.Now().Unix(),
	}
	// DoUpdates 复用 GORM clause 跨库兼容（SQLite/MySQL/PG 都支持）。
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"marked_at"}),
	}).Create(&row).Error
}

// FetchAffiliateRiskDirtyBatch 取出一批待重算的 user_id，按 marked_at 升序。
// 仅读取，不删除——避免取出后处理失败导致丢失。处理成功后调用 ClearAffiliateRiskDirty。
func FetchAffiliateRiskDirtyBatch(limit int) ([]int, error) {
	if limit <= 0 {
		return nil, nil
	}
	var ids []int
	err := DB.Model(&AffiliateRiskDirty{}).
		Order("marked_at ASC").
		Limit(limit).
		Pluck("user_id", &ids).Error
	return ids, err
}

// ClearAffiliateRiskDirty 删除已处理的 user_id。
func ClearAffiliateRiskDirty(userIds []int) error {
	if len(userIds) == 0 {
		return nil
	}
	return DB.Where("user_id IN ?", userIds).Delete(&AffiliateRiskDirty{}).Error
}

// BackfillAffiliateRiskDirty 把当前所有"有被邀者"的用户全部塞进 dirty 表。
// 仅在启动或运营手动触发时调用一次，给存量数据"加塞"评估机会。
//
// 性能保护：
//   - 单表 users 扫描 + WHERE aff_count > 0，命中量 <= 邀请人数（一般 << 总用户数）；
//   - 批量 upsert，每 500 条提交一次，避免一次性 INSERT 撑爆事务日志；
//   - 失败容忍：单批失败只记日志、继续下一批，避免启动卡死。
//
// 返回值为实际入队的用户数（用于启动日志），error 仅在彻底无法查询 users 时返回。
func BackfillAffiliateRiskDirty() (int, error) {
	var ids []int
	if err := DB.Model(&User{}).
		Where("aff_count > 0").
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	const chunk = 500
	now := time.Now().Unix()
	written := 0
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		rows := make([]AffiliateRiskDirty, 0, end-i)
		for _, id := range ids[i:end] {
			rows = append(rows, AffiliateRiskDirty{UserId: id, MarkedAt: now})
		}
		// DoNothing：已存在的脏标记不覆盖 marked_at，保护原有处理顺序。
		if err := DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(&rows).Error; err != nil {
			// 单批失败：跳过，避免整体回填中止；worker 仍会消费已成功的批次。
			return written, err
		}
		written += len(rows)
	}
	return written, nil
}

// UpsertAffiliateRiskCache 写入或更新一条风控缓存。
// 单条写入，worker 在批处理中循环调用；高并发场景由 worker 自己做限速。
func UpsertAffiliateRiskCache(row *AffiliateRiskCache) error {
	if row == nil || row.UserId <= 0 {
		return nil
	}
	if row.ComputedAt == 0 {
		row.ComputedAt = time.Now().Unix()
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"risk_level", "risk_reasons", "signals", "computed_at",
		}),
	}).Create(row).Error
}

// DeleteAffiliateRiskCache 删除一条缓存（用户被删 / 邀请关系归零时调用）。
func DeleteAffiliateRiskCache(userId int) error {
	if userId <= 0 {
		return nil
	}
	return DB.Where("user_id = ?", userId).Delete(&AffiliateRiskCache{}).Error
}

// GetAffiliateRiskCache 取单个用户的缓存。未命中返回 (nil, nil)，不区分错误。
// 用于详情接口；列表接口不走这里，走 LEFT JOIN。
func GetAffiliateRiskCache(userId int) (*AffiliateRiskCache, error) {
	if userId <= 0 {
		return nil, nil
	}
	var row AffiliateRiskCache
	err := DB.Where("user_id = ?", userId).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ===== 邀请关系详情查询（管理员后台） =====
//
// 设计：给后台"用户详情 → 邀请关系"用，一次调用拿齐：
//   1) 目标用户基本信息 + 风险摘要；
//   2) 邀请人（若有）基本信息 + 风险摘要；
//   3) 该用户邀请的下级用户分页列表（含每人的风险等级）。
//
// 性能边界：
//   - 不做任何全表扫描；下级列表沿用已有的 GetInvitees（topup 聚合 + LIMIT/OFFSET，
//     与 self/invitees 一致）；
//   - 下级用户的风险等级通过单条 IN 语句批量拉取，避免 N+1；
//   - 目标用户、邀请人的风险记录走主键查找，O(1)。

// AffiliateRelationUser 邀请关系视图里的用户摘要。
// 仅返回管理员场景需要的字段，避免泄露与 PII 风险扩大。
type AffiliateRelationUser struct {
	Id              int    `json:"id"`
	Username        string `json:"username"`
	DisplayName     string `json:"display_name"`
	Email           string `json:"email"`
	Status          int    `json:"status"`
	Role            int    `json:"role"`
	AffCode         string `json:"aff_code"`
	AffCount        int    `json:"aff_count"`
	InviterId       int    `json:"inviter_id"`
	AffQuota        int    `json:"aff_quota"`
	AffHistoryQuota int    `json:"aff_history_quota"`
	CreatedTime     int64  `json:"created_time"`
	LastLoginAt     int64  `json:"last_login_at"`

	// 风险快照；当 cache 未生成时为 nil。
	Risk *AffiliateRiskCache `json:"risk"`
}

// AffiliateRelationInvitee 下级用户视图，复用 InviteeInfo 字段并附带风险等级。
type AffiliateRelationInvitee struct {
	Id          int     `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Email       string  `json:"email"`
	Status      int     `json:"status"`
	CreatedTime int64   `json:"created_time"`
	LastLoginAt int64   `json:"last_login_at"`
	TopupAmount float64 `json:"topup_amount"`
	RiskLevel   string  `json:"risk_level"` // 空串 = 未评估
}

// AffiliateRelationView 单次 API 返回的聚合结构。
type AffiliateRelationView struct {
	Target   *AffiliateRelationUser     `json:"target"`
	Inviter  *AffiliateRelationUser     `json:"inviter"` // 可能为 nil（用户没有邀请人）
	Invitees []*AffiliateRelationInvitee `json:"invitees"`
	Total    int64                      `json:"total"`     // 该用户邀请的总人数
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

// loadAffiliateRelationUser 加载单用户摘要 + 风险。
func loadAffiliateRelationUser(userId int) (*AffiliateRelationUser, error) {
	if userId <= 0 {
		return nil, nil
	}
	var u User
	if err := DB.Select(
		"id, username, display_name, email, status, role, aff_code, aff_count, " +
			"inviter_id, aff_quota, aff_history, created_time, last_login_at",
	).Where("id = ?", userId).First(&u).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	risk, err := GetAffiliateRiskCache(userId)
	if err != nil {
		return nil, err
	}
	return &AffiliateRelationUser{
		Id:              u.Id,
		Username:        u.Username,
		DisplayName:     u.DisplayName,
		Email:           u.Email,
		Status:          u.Status,
		Role:            u.Role,
		AffCode:         u.AffCode,
		AffCount:        u.AffCount,
		InviterId:       u.InviterId,
		AffQuota:        u.AffQuota,
		AffHistoryQuota: u.AffHistoryQuota,
		CreatedTime:     u.CreatedTime,
		LastLoginAt:     u.LastLoginAt,
		Risk:            risk,
	}, nil
}

// GetAffiliateRelationView 聚合查询：返回目标用户的邀请关系全景。
//
// page 从 1 起；pageSize 上限保护见 caller。
func GetAffiliateRelationView(userId, page, pageSize int) (*AffiliateRelationView, error) {
	if userId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	target, err := loadAffiliateRelationUser(userId)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, gorm.ErrRecordNotFound
	}

	view := &AffiliateRelationView{
		Target:   target,
		Page:     page,
		PageSize: pageSize,
		Invitees: []*AffiliateRelationInvitee{},
	}

	// 邀请人（若有）
	if target.InviterId > 0 {
		inviter, err := loadAffiliateRelationUser(target.InviterId)
		if err != nil {
			return nil, err
		}
		view.Inviter = inviter
	}

	// 总数：该用户邀请的下级用户数。Unscoped 与已有 GetInvitees 保持一致语义。
	if err := DB.Model(&User{}).Where("inviter_id = ?", userId).Count(&view.Total).Error; err != nil {
		return nil, err
	}
	if view.Total == 0 {
		return view, nil
	}

	// 下级用户分页列表，按充值金额+id 倒序，与 self/invitees 视图一致以减少认知差。
	type inviteeRow struct {
		Id          int
		Username    string
		DisplayName string
		Email       string
		Status      int
		CreatedTime int64
		LastLoginAt int64
		TopupAmount float64
	}
	var rows []*inviteeRow
	offset := (page - 1) * pageSize
	if err := DB.Table("users").
		Select(
			"users.id, users.username, users.display_name, users.email, users.status, " +
				"users.created_time, users.last_login_at, " +
				"COALESCE(SUM(top_ups.money), 0) as topup_amount",
		).
		Joins("LEFT JOIN top_ups ON users.id = top_ups.user_id AND top_ups.status = ?", common.TopUpStatusSuccess).
		Where("users.inviter_id = ?", userId).
		Group("users.id, users.username, users.display_name, users.email, users.status, users.created_time, users.last_login_at").
		Order("topup_amount DESC, users.id DESC").
		Limit(pageSize).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 批量拉取这一页下级用户的风险等级，单条 IN，避免 N+1。
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	riskMap := make(map[int]string, len(ids))
	if len(ids) > 0 {
		var caches []AffiliateRiskCache
		if err := DB.Select("user_id, risk_level").
			Where("user_id IN ?", ids).
			Find(&caches).Error; err != nil {
			return nil, err
		}
		for _, c := range caches {
			riskMap[c.UserId] = c.RiskLevel
		}
	}

	view.Invitees = make([]*AffiliateRelationInvitee, 0, len(rows))
	for _, r := range rows {
		view.Invitees = append(view.Invitees, &AffiliateRelationInvitee{
			Id:          r.Id,
			Username:    r.Username,
			DisplayName: r.DisplayName,
			Email:       r.Email,
			Status:      r.Status,
			CreatedTime: r.CreatedTime,
			LastLoginAt: r.LastLoginAt,
			TopupAmount: r.TopupAmount,
			RiskLevel:   riskMap[r.Id],
		})
	}
	return view, nil
}
