package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TopUp struct {
	Id               int     `json:"id"`
	UserId           int     `json:"user_id" gorm:"index:idx_user_status,priority:1;index"`
	Amount           int64   `json:"amount"`
	Money            float64 `json:"money"`
	TradeNo          string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod    string  `json:"payment_method" gorm:"type:varchar(50)"`
	CreateTime       int64   `json:"create_time"`
	CompleteTime     int64   `json:"complete_time"`
	Status           string  `json:"status" gorm:"index:idx_user_status,priority:2"`
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func Recharge(referenceId string, customerId string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota float64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = float64(topUp.Amount) * common.QuotaPerUnit
		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)}).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%.2f", logger.FormatQuota(int(quota)), topUp.Money))

	// 处理邀请返现（基于实付金额）
	ProcessAffiliateRebate(topUp.UserId, topUp.Money)

	// 检查并自动升级用户分组
	gopool.Go(func() {
		if err := CheckAndUpgradeUserGroup(topUp.UserId); err != nil {
			common.SysLog(fmt.Sprintf("自动升级用户分组失败 userId=%d: %v", topUp.UserId, err))
		}
	})

	return nil
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ?", userId).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ?", userId).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ?", userId)
	if keyword != "" {
		like := "%%" + keyword + "%%"
		query = query.Where("trade_no LIKE ?", like)
	}

	if err = query.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		like := "%%" + keyword + "%%"
		query = query.Where("trade_no LIKE ?", like)
	}

	if err = query.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：Amount 为充值数量，Money 为实付金额
		// 所有支付方式都应该使用 Amount 计算获得的积分
		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney))
	
	// 处理邀请返现（基于实付金额）
	ProcessAffiliateRebate(userId, payMoney)

	// 检查并自动升级用户分组
	gopool.Go(func() {
		if err := CheckAndUpgradeUserGroup(userId); err != nil {
			common.SysLog(fmt.Sprintf("自动升级用户分组失败 userId=%d: %v", userId, err))
		}
	})
	
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money))

	// 处理邀请返现（基于实付金额）
	ProcessAffiliateRebate(topUp.UserId, topUp.Money)

	// 检查并自动升级用户分组
	gopool.Go(func() {
		if err := CheckAndUpgradeUserGroup(topUp.UserId); err != nil {
			common.SysLog(fmt.Sprintf("自动升级用户分组失败 userId=%d: %v", topUp.UserId, err))
		}
	})

	return nil
}

func RechargeWaffo(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
		
		// 处理邀请返现（基于实付金额）
		ProcessAffiliateRebate(topUp.UserId, topUp.Money)

		// 检查并自动升级用户分组
		gopool.Go(func() {
			if err := CheckAndUpgradeUserGroup(topUp.UserId); err != nil {
				common.SysLog(fmt.Sprintf("自动升级用户分组失败 userId=%d: %v", topUp.UserId, err))
			}
		})
	}

	return nil
}

// GetUserTotalTopupAmount 获取用户累计充值金额（仅统计成功的订单）
func GetUserTotalTopupAmount(userId int) (float64, error) {
	var totalAmount float64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(money), 0)").
		Scan(&totalAmount).Error
	return totalAmount, err
}

// CheckAndUpgradeUserGroup 检查用户充值金额并自动升级分组
// 根据配置的升级规则，当用户累计充值达到阈值时自动升级用户分组
func CheckAndUpgradeUserGroup(userId int) error {
	// 检查是否启用自动升级
	if !operation_setting.IsAutoUpgradeEnabled() {
		return nil
	}

	// 获取升级规则
	rules := operation_setting.GetUpgradeRules()
	if len(rules) == 0 {
		return nil
	}

	// 获取用户累计充值金额
	totalAmount, err := GetUserTotalTopupAmount(userId)
	if err != nil {
		return err
	}

	// 获取当前用户
	user, err := GetUserById(userId, true)
	if err != nil {
		return err
	}

	// 遍历升级规则，找到适用的规则
	for _, rule := range rules {
		// 检查是否满足升级条件：
		// 1. 当前分组匹配源分组
		// 2. 累计充值金额达到或超过阈值
		if user.Group == rule.FromGroup && totalAmount >= rule.Threshold {
			// 执行升级
			user.Group = rule.ToGroup
			err = user.Update(false)
			if err != nil {
				return err
			}

			// 记录升级日志
			RecordLog(userId, LogTypeSystem, fmt.Sprintf("充值累计达到 %.2f 元，自动升级到 %s 分组", totalAmount, rule.ToGroup))
			common.SysLog(fmt.Sprintf("用户 %d (%s) 充值累计 %.2f 元，自动从 %s 升级到 %s", userId, user.Username, totalAmount, rule.FromGroup, rule.ToGroup))

			// 升级后继续检查是否还能继续升级（支持 default -> vip -> svip 这样的链式升级）
			return CheckAndUpgradeUserGroup(userId)
		}
	}

	return nil
}

// ProcessAffiliateRebate 处理充值返现（异步）
// 当用户充值成功后，如果该用户有邀请者且系统启用了返现功能，
// 则按照配置的返现比例给邀请者增加AffQuota
// 参数：
//   userId: 充值用户ID
//   topupMoney: 实付金额（不是到账额度，而是用户实际支付的金额）
func ProcessAffiliateRebate(userId int, topupMoney float64) {
	// 检查是否启用返现
	if common.AffRebateRatio <= 0 {
		return
	}
	
	// 边界检查
	if topupMoney <= 0 {
		return
	}

	gopool.Go(func() {
		// 查询用户的邀请者ID
		var user User
		err := DB.Select("inviter_id, username").Where("id = ?", userId).First(&user).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				common.SysError(fmt.Sprintf("充值返现查询用户失败 userId=%d: %v", userId, err))
			}
			return
		}
		if user.InviterId == 0 {
			// 没有邀请者，正常情况，不记录日志
			return
		}

		// 计算返现额度：实付金额 * 返现比例 / 100，然后转换为 quota
		// 例如：实付 $95，返现比例 10%，则返现 $9.5 等值的 quota
		dTopupMoney := decimal.NewFromFloat(topupMoney)
		dRebateRatio := decimal.NewFromFloat(common.AffRebateRatio)
		dHundred := decimal.NewFromInt(100)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		
		// 返现金额（美元）= 实付金额 * 返现比例 / 100
		rebateMoney := dTopupMoney.Mul(dRebateRatio).Div(dHundred)
		// 返现额度（quota）= 返现金额 * QuotaPerUnit
		rebateQuota := int(rebateMoney.Mul(dQuotaPerUnit).IntPart())

		if rebateQuota <= 0 {
			// 返现额度为0，不处理（可能是因为充值额度很小或返现比例太低）
			return
		}
		
		// 可选：设置返现最小阈值，避免产生过小的返现（如1 token）
		// const minRebateQuota = 100 // 最小返现100 tokens
		// if rebateQuota < minRebateQuota {
		//     return
		// }

		// 使用行锁增加邀请者的AffQuota和AffHistoryQuota（防止并发问题）
		tx := DB.Begin()
		if tx.Error != nil {
			common.SysError("充值返现事务失败: " + tx.Error.Error())
			return
		}

		// 使用FOR UPDATE行锁
		var inviter User
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, aff_quota, aff_history").
			Where("id = ?", user.InviterId).
			First(&inviter).Error
		if err != nil {
			tx.Rollback()
			common.SysError("充值返现查询邀请者失败: " + err.Error())
			return
		}

		// 更新邀请者的额度（注意：数据库字段是 aff_history，不是 aff_history_quota）
		err = tx.Model(&User{}).Where("id = ?", user.InviterId).Updates(map[string]interface{}{
			"aff_quota":   gorm.Expr("aff_quota + ?", rebateQuota),
			"aff_history": gorm.Expr("aff_history + ?", rebateQuota),
		}).Error

		if err != nil {
			tx.Rollback()
			common.SysError("充值返现失败: " + err.Error())
			return
		}

		if err = tx.Commit().Error; err != nil {
			common.SysError("充值返现提交失败: " + err.Error())
			return
		}

		// 记录返现日志
		RecordLog(user.InviterId, LogTypeTopup, fmt.Sprintf("邀请用户充值返现 %s (被邀用户: %s, 返现比例: %.1f%%)", logger.LogQuota(rebateQuota), user.Username, common.AffRebateRatio))
		common.SysLog(fmt.Sprintf("充值返现成功: 用户 %d (%s) 充值，邀请者 %d 获得 %s (比例: %.1f%%)", userId, user.Username, user.InviterId, logger.LogQuota(rebateQuota), common.AffRebateRatio))
	})
}
