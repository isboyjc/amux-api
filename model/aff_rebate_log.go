package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	AffRebateTypeRegister = 1 // 注册返现
	AffRebateTypeTopup    = 2 // 充值返现
)

// AffRebateLog 返现流水记录
type AffRebateLog struct {
	Id          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId      int       `json:"user_id" gorm:"index;not null"`
	FromUserId  int       `json:"from_user_id" gorm:"index;not null"`
	Type        int       `json:"type" gorm:"type:int;not null;index"`
	Quota       int       `json:"quota" gorm:"type:int;not null"`
	TopupAmount float64   `json:"topup_amount" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (AffRebateLog) TableName() string {
	return "aff_rebate_logs"
}

// CreateAffRebateLog 创建返现流水记录
func CreateAffRebateLog(tx *gorm.DB, log *AffRebateLog) error {
	if tx == nil {
		tx = DB
	}
	return tx.Create(log).Error
}

// AffRebateStats 返现统计结果
type AffRebateStats struct {
	RegPendingQuota   int `json:"reg_pending_quota"`   // 注册返现待使用额度
	TopupPendingQuota int `json:"topup_pending_quota"` // 充值返现待使用额度
	RegHistoryQuota   int `json:"reg_history_quota"`   // 注册返现历史总额度
	TopupHistoryQuota int `json:"topup_history_quota"` // 充值返现历史总额度
}

// GetAffRebateStats 获取用户返现统计（按类型聚合）
func GetAffRebateStats(userId int) (*AffRebateStats, error) {
	stats := &AffRebateStats{}

	// 按类型聚合总额（即历史总额度）
	type TypeSum struct {
		Type     int
		TotalSum int
	}
	var results []TypeSum
	err := DB.Model(&AffRebateLog{}).
		Select("type, COALESCE(SUM(quota), 0) as total_sum").
		Where("user_id = ?", userId).
		Group("type").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	for _, r := range results {
		switch r.Type {
		case AffRebateTypeRegister:
			stats.RegHistoryQuota = r.TotalSum
		case AffRebateTypeTopup:
			stats.TopupHistoryQuota = r.TotalSum
		}
	}

	// 获取用户当前的待使用总额度和历史总额度
	var user User
	err = DB.Select("aff_quota, aff_history").Where("id = ?", userId).First(&user).Error
	if err != nil {
		return nil, err
	}

	// 待使用额度拆分：
	// 流水表记录了每笔返现的类型和金额
	// 如果流水总额 <= 当前待使用额度，说明所有有记录的返现都还在，直接展示
	// 如果流水总额 > 当前待使用额度，说明部分已划转，按 充值 → 注册 顺序消耗
	totalFromLogs := stats.RegHistoryQuota + stats.TopupHistoryQuota
	if totalFromLogs > 0 && user.AffQuota > 0 {
		if totalFromLogs <= user.AffQuota {
			// 所有有记录的返现都还在待使用中
			stats.RegPendingQuota = stats.RegHistoryQuota
			stats.TopupPendingQuota = stats.TopupHistoryQuota
		} else {
			// 部分已划转，按 充值 → 注册 顺序消耗
			consumed := totalFromLogs - user.AffQuota
			topupConsumed := consumed
			if topupConsumed > stats.TopupHistoryQuota {
				topupConsumed = stats.TopupHistoryQuota
			}
			consumed -= topupConsumed
			regConsumed := consumed
			if regConsumed > stats.RegHistoryQuota {
				regConsumed = stats.RegHistoryQuota
			}
			stats.TopupPendingQuota = stats.TopupHistoryQuota - topupConsumed
			stats.RegPendingQuota = stats.RegHistoryQuota - regConsumed
		}
	}

	return stats, nil
}
