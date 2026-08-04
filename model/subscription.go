package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/service/events"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/hot"
	"gorm.io/gorm"
)

// Subscription duration units
const (
	SubscriptionDurationYear   = "year"
	SubscriptionDurationMonth  = "month"
	SubscriptionDurationDay    = "day"
	SubscriptionDurationHour   = "hour"
	SubscriptionDurationCustom = "custom"
)

// Subscription quota reset period
const (
	SubscriptionResetNever   = "never"
	SubscriptionResetDaily   = "daily"
	SubscriptionResetWeekly  = "weekly"
	SubscriptionResetMonthly = "monthly"
	SubscriptionResetCustom  = "custom"
)

var (
	ErrSubscriptionOrderNotFound      = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid = errors.New("subscription order status invalid")
)

const (
	subscriptionPlanCacheNamespace     = "new-api:subscription_plan:v1"
	subscriptionPlanInfoCacheNamespace = "new-api:subscription_plan_info:v1"
)

var (
	subscriptionPlanCacheOnce     sync.Once
	subscriptionPlanInfoCacheOnce sync.Once

	subscriptionPlanCache     *cachex.HybridCache[SubscriptionPlan]
	subscriptionPlanInfoCache *cachex.HybridCache[SubscriptionPlanInfo]
)

func subscriptionPlanCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_TTL", 300)
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanInfoCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_TTL", 120)
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_CAP", 5000)
	if capacity <= 0 {
		capacity = 5000
	}
	return capacity
}

func subscriptionPlanInfoCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getSubscriptionPlanCache() *cachex.HybridCache[SubscriptionPlan] {
	subscriptionPlanCacheOnce.Do(func() {
		ttl := subscriptionPlanCacheTTL()
		subscriptionPlanCache = cachex.NewHybridCache[SubscriptionPlan](cachex.HybridCacheConfig[SubscriptionPlan]{
			Namespace: cachex.Namespace(subscriptionPlanCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlan]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlan] {
				return hot.NewHotCache[string, SubscriptionPlan](hot.LRU, subscriptionPlanCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanCache
}

func getSubscriptionPlanInfoCache() *cachex.HybridCache[SubscriptionPlanInfo] {
	subscriptionPlanInfoCacheOnce.Do(func() {
		ttl := subscriptionPlanInfoCacheTTL()
		subscriptionPlanInfoCache = cachex.NewHybridCache[SubscriptionPlanInfo](cachex.HybridCacheConfig[SubscriptionPlanInfo]{
			Namespace: cachex.Namespace(subscriptionPlanInfoCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlanInfo]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlanInfo] {
				return hot.NewHotCache[string, SubscriptionPlanInfo](hot.LRU, subscriptionPlanInfoCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanInfoCache
}

func subscriptionPlanCacheKey(id int) string {
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(id)
}

func InvalidateSubscriptionPlanCache(planId int) {
	if planId <= 0 {
		return
	}
	cache := getSubscriptionPlanCache()
	_, _ = cache.DeleteMany([]string{subscriptionPlanCacheKey(planId)})
	infoCache := getSubscriptionPlanInfoCache()
	_ = infoCache.Purge()
}

// Subscription plan
type SubscriptionPlan struct {
	Id int `json:"id"`

	Title    string `json:"title" gorm:"type:varchar(128);not null"`
	Subtitle string `json:"subtitle" gorm:"type:varchar(255);default:''"`

	// Display money amount (follow existing code style: float64 for money)
	PriceAmount float64 `json:"price_amount" gorm:"type:decimal(10,6);not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`

	DurationUnit  string `json:"duration_unit" gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue int    `json:"duration_value" gorm:"type:int;not null;default:1"`
	CustomSeconds int64  `json:"custom_seconds" gorm:"type:bigint;not null;default:0"`

	Enabled   bool `json:"enabled" gorm:"default:true"`
	SortOrder int  `json:"sort_order" gorm:"type:int;default:0"`

	StripePriceId  string `json:"stripe_price_id" gorm:"type:varchar(128);default:''"`
	CreemProductId string `json:"creem_product_id" gorm:"type:varchar(128);default:''"`

	// Max purchases per user (0 = unlimited)
	MaxPurchasePerUser int `json:"max_purchase_per_user" gorm:"type:int;default:0"`

	// Upgrade user group after purchase (empty = no change)
	UpgradeGroup string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`

	// Total quota (amount in quota units, 0 = unlimited)
	TotalAmount int64 `json:"total_amount" gorm:"type:bigint;not null;default:0"`

	// Quota reset period for plan
	QuotaResetPeriod        string `json:"quota_reset_period" gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds" gorm:"type:bigint;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (p *SubscriptionPlan) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (p *SubscriptionPlan) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = common.GetTimestamp()
	return nil
}

// Subscription order (payment -> webhook -> create UserSubscription)
type SubscriptionOrder struct {
	Id     int     `json:"id"`
	UserId int     `json:"user_id" gorm:"index"`
	PlanId int     `json:"plan_id" gorm:"index"`
	Money  float64 `json:"money"`

	TradeNo       string `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod string `json:"payment_method" gorm:"type:varchar(50)"`
	// PaymentProvider 标识下单时使用的支付网关，用于回调时校验防止跨网关攻击。
	// 为空时回退到 PaymentMethod 推断（兼容历史订单）。
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	Status          string `json:"status"`
	CreateTime      int64  `json:"create_time"`
	CompleteTime    int64  `json:"complete_time"`

	ProviderPayload string `json:"provider_payload" gorm:"type:text"`
}

// MatchesPaymentProvider 判断订单是否由指定网关创建。
// 优先用显式 PaymentProvider 字段；为空时回退到 PaymentMethod 推断（兼容历史订单）。
func (o *SubscriptionOrder) MatchesPaymentProvider(expected string) bool {
	if o.PaymentProvider != "" {
		return o.PaymentProvider == expected
	}
	return derivePaymentProvider(o.PaymentMethod) == expected
}

func (o *SubscriptionOrder) Insert() error {
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	return DB.Create(o).Error
}

func (o *SubscriptionOrder) Update() error {
	return DB.Save(o).Error
}

func GetSubscriptionOrderByTradeNo(tradeNo string) *SubscriptionOrder {
	if tradeNo == "" {
		return nil
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil
	}
	return &order
}

// User subscription instance
type UserSubscription struct {
	Id     int `json:"id"`
	UserId int `json:"user_id" gorm:"index;index:idx_user_sub_active,priority:1"`
	PlanId int `json:"plan_id" gorm:"index"`

	AmountTotal int64 `json:"amount_total" gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `json:"amount_used" gorm:"type:bigint;not null;default:0"`

	StartTime int64  `json:"start_time" gorm:"bigint"`
	EndTime   int64  `json:"end_time" gorm:"bigint;index;index:idx_user_sub_active,priority:3"`
	Status    string `json:"status" gorm:"type:varchar(32);index;index:idx_user_sub_active,priority:2"` // active/expired/cancelled

	Source string `json:"source" gorm:"type:varchar(32);default:'order'"` // order/admin

	LastResetTime int64 `json:"last_reset_time" gorm:"type:bigint;default:0"`
	NextResetTime int64 `json:"next_reset_time" gorm:"type:bigint;default:0;index"`

	UpgradeGroup  string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`
	PrevUserGroup string `json:"prev_user_group" gorm:"type:varchar(64);default:''"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (s *UserSubscription) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (s *UserSubscription) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

type SubscriptionSummary struct {
	Subscription *UserSubscription `json:"subscription"`
}

func calcPlanEndTime(start time.Time, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("plan is nil")
	}
	if plan.DurationValue <= 0 && plan.DurationUnit != SubscriptionDurationCustom {
		return 0, errors.New("duration_value must be > 0")
	}
	switch plan.DurationUnit {
	case SubscriptionDurationYear:
		return start.AddDate(plan.DurationValue, 0, 0).Unix(), nil
	case SubscriptionDurationMonth:
		return start.AddDate(0, plan.DurationValue, 0).Unix(), nil
	case SubscriptionDurationDay:
		return start.Add(time.Duration(plan.DurationValue) * 24 * time.Hour).Unix(), nil
	case SubscriptionDurationHour:
		return start.Add(time.Duration(plan.DurationValue) * time.Hour).Unix(), nil
	case SubscriptionDurationCustom:
		if plan.CustomSeconds <= 0 {
			return 0, errors.New("custom_seconds must be > 0")
		}
		return start.Add(time.Duration(plan.CustomSeconds) * time.Second).Unix(), nil
	default:
		return 0, fmt.Errorf("invalid duration_unit: %s", plan.DurationUnit)
	}
}

func NormalizeResetPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly, SubscriptionResetCustom:
		return strings.TrimSpace(period)
	default:
		return SubscriptionResetNever
	}
}

func calcNextResetTime(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	if plan == nil {
		return 0
	}
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if period == SubscriptionResetNever {
		return 0
	}
	var next time.Time
	switch period {
	case SubscriptionResetDaily:
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, 1)
	case SubscriptionResetWeekly:
		// Align to next Monday 00:00
		weekday := int(base.Weekday()) // Sunday=0
		// Convert to Monday=1..Sunday=7
		if weekday == 0 {
			weekday = 7
		}
		daysUntil := 8 - weekday
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, daysUntil)
	case SubscriptionResetMonthly:
		// Align to first day of next month 00:00
		next = time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location()).
			AddDate(0, 1, 0)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds <= 0 {
			return 0
		}
		next = base.Add(time.Duration(plan.QuotaResetCustomSeconds) * time.Second)
	default:
		return 0
	}
	if endUnix > 0 && next.Unix() > endUnix {
		return 0
	}
	return next.Unix()
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	return getSubscriptionPlanByIdTx(nil, id)
}

func getSubscriptionPlanByIdTx(tx *gorm.DB, id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("invalid plan id")
	}
	key := subscriptionPlanCacheKey(id)
	if key != "" {
		if cached, found, err := getSubscriptionPlanCache().Get(key); err == nil && found {
			return &cached, nil
		}
	}
	var plan SubscriptionPlan
	query := DB
	if tx != nil {
		query = tx
	}
	if err := query.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	_ = getSubscriptionPlanCache().SetWithTTL(key, plan, subscriptionPlanCacheTTL())
	return &plan, nil
}

func CountUserSubscriptionsByPlan(userId int, planId int) (int64, error) {
	if userId <= 0 || planId <= 0 {
		return 0, errors.New("invalid userId or planId")
	}
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func getUserGroupByIdTx(tx *gorm.DB, userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid userId")
	}
	if tx == nil {
		tx = DB
	}
	var group string
	if err := tx.Model(&User{}).Where("id = ?", userId).Select(commonGroupCol).Find(&group).Error; err != nil {
		return "", err
	}
	return group, nil
}

func downgradeUserGroupForSubscriptionTx(tx *gorm.DB, sub *UserSubscription, now int64) (string, error) {
	if tx == nil || sub == nil {
		return "", errors.New("invalid downgrade args")
	}
	upgradeGroup := strings.TrimSpace(sub.UpgradeGroup)
	if upgradeGroup == "" {
		return "", nil
	}
	currentGroup, err := getUserGroupByIdTx(tx, sub.UserId)
	if err != nil {
		return "", err
	}
	if currentGroup != upgradeGroup {
		return "", nil
	}
	var activeSub UserSubscription
	activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND id <> ? AND upgrade_group <> ''",
		sub.UserId, "active", now, sub.Id).
		Order("end_time desc, id desc").
		Limit(1).
		Find(&activeSub)
	if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
		return "", nil
	}
	prevGroup := strings.TrimSpace(sub.PrevUserGroup)
	if prevGroup == "" || prevGroup == currentGroup {
		return "", nil
	}
	if err := tx.Model(&User{}).Where("id = ?", sub.UserId).
		Update("group", prevGroup).Error; err != nil {
		return "", err
	}
	events.PublishBestEffortInTx(tx, events.UserGroupChanged, sub.UserId, &events.UserGroupChangedPayload{
		UserId:    sub.UserId,
		Email:     fetchUserEmailTx(tx, sub.UserId),
		FromGroup: currentGroup,
		ToGroup:   prevGroup,
		Trigger:   "subscription",
		ChangedAt: common.GetTimestamp(),
	})
	return prevGroup, nil
}

func CreateUserSubscriptionFromPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if plan == nil || plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if plan.MaxPurchasePerUser > 0 {
		var count int64
		if err := tx.Model(&UserSubscription{}).
			Where("user_id = ? AND plan_id = ?", userId, plan.Id).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			return nil, errors.New("已达到该套餐购买上限")
		}
	}
	nowUnix := GetDBTimestamp()
	now := time.Unix(nowUnix, 0)
	endUnix, err := calcPlanEndTime(now, plan)
	if err != nil {
		return nil, err
	}
	resetBase := now
	nextReset := calcNextResetTime(resetBase, plan, endUnix)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = now.Unix()
	}
	upgradeGroup := strings.TrimSpace(plan.UpgradeGroup)
	prevGroup := ""
	if upgradeGroup != "" {
		currentGroup, err := getUserGroupByIdTx(tx, userId)
		if err != nil {
			return nil, err
		}
		if currentGroup != upgradeGroup {
			prevGroup = currentGroup
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", upgradeGroup).Error; err != nil {
				return nil, err
			}
			events.PublishBestEffortInTx(tx, events.UserGroupChanged, userId, &events.UserGroupChangedPayload{
				UserId:    userId,
				Email:     fetchUserEmailTx(tx, userId),
				FromGroup: currentGroup,
				ToGroup:   upgradeGroup,
				Trigger:   "subscription",
				ChangedAt: common.GetTimestamp(),
			})
		}
	}
	sub := &UserSubscription{
		UserId:        userId,
		PlanId:        plan.Id,
		AmountTotal:   plan.TotalAmount,
		AmountUsed:    0,
		StartTime:     now.Unix(),
		EndTime:       endUnix,
		Status:        "active",
		Source:        source,
		LastResetTime: lastReset,
		NextResetTime: nextReset,
		UpgradeGroup:  upgradeGroup,
		PrevUserGroup: prevGroup,
		CreatedAt:     common.GetTimestamp(),
		UpdatedAt:     common.GetTimestamp(),
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

// Complete a subscription order (idempotent). Creates a UserSubscription snapshot from the plan.
// CompleteSubscriptionOrder 完成订阅订单。
// expectedPaymentProvider 用于防跨网关回调攻击：传入预期网关，
// 与订单创建时的网关不一致直接拒绝；传 "" 表示跳过校验（仅在调用方已自行校验时使用）。
func CompleteSubscriptionOrder(tradeNo string, providerPayload string, expectedPaymentProvider string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	var logUserId int
	var logPlanTitle string
	var logMoney float64
	var logPaymentMethod string
	var upgradeGroup string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := tx.Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && !order.MatchesPaymentProvider(expectedPaymentProvider) {
			return ErrPaymentMethodMismatch
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return ErrSubscriptionOrderStatusInvalid
		}

		// 先原子认领订单，再发放订阅权益。
		//
		// 原来依赖 tx.Set("gorm:query_option", "FOR UPDATE")，那是 GORM v1 的 API，
		// v2 下是空操作，没有任何行锁。两个并发回调会双双通过上面的 Pending 判断，
		// 各自跑一遍 CreateUserSubscriptionFromPlanTx——用户白拿两份订阅额度。
		// 认领放在权益发放之前，输的一方直接按幂等返回，不会产生任何副作用。
		completeTime := common.GetTimestamp()
		claim := tx.Model(&SubscriptionOrder{}).
			Where("id = ? AND status = ?", order.Id, common.TopUpStatusPending).
			Updates(map[string]interface{}{
				"status":        common.TopUpStatusSuccess,
				"complete_time": completeTime,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			// 被并发回调抢先完成（或已被置为其它状态），与上面的幂等分支保持一致
			return nil
		}
		order.Status = common.TopUpStatusSuccess
		order.CompleteTime = completeTime

		plan, err := GetSubscriptionPlanById(order.PlanId)
		if err != nil {
			return err
		}
		if !plan.Enabled {
			// still allow completion for already purchased orders
		}
		upgradeGroup = strings.TrimSpace(plan.UpgradeGroup)
		_, err = CreateUserSubscriptionFromPlanTx(tx, order.UserId, plan, "order")
		if err != nil {
			return err
		}
		if err := upsertSubscriptionTopUpTx(tx, &order); err != nil {
			return err
		}
		// status / complete_time 已由上面的原子认领写入，这里只补 provider payload。
		// 不能再用 tx.Save(&order) 整行写回：那会把 order 结构体上的全部字段按快照
		// 重写一遍，也失去了认领语义。
		if providerPayload != "" {
			order.ProviderPayload = providerPayload
			if err := tx.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).
				Update("provider_payload", providerPayload).Error; err != nil {
				return err
			}
		}
		logUserId = order.UserId
		logPlanTitle = plan.Title
		logMoney = order.Money
		logPaymentMethod = order.PaymentMethod
		return nil
	})
	if err != nil {
		return err
	}
	if upgradeGroup != "" && logUserId > 0 {
		_ = UpdateUserGroupCache(logUserId, upgradeGroup)
	}
	if logUserId > 0 {
		msg := fmt.Sprintf("订阅购买成功，套餐: %s，支付金额: %.2f，支付方式: %s", logPlanTitle, logMoney, logPaymentMethod)
		RecordLog(logUserId, LogTypeTopup, msg)

		// 检查并自动升级用户分组
		gopool.Go(func() {
			if err := CheckAndUpgradeUserGroup(logUserId); err != nil {
				common.SysLog(fmt.Sprintf("自动升级用户分组失败 userId=%d: %v", logUserId, err))
			}
		})
	}
	return nil
}

func upsertSubscriptionTopUpTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid subscription order")
	}
	now := common.GetTimestamp()
	var topup TopUp
	if err := tx.Where("trade_no = ?", order.TradeNo).First(&topup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			topup = TopUp{
				UserId:        order.UserId,
				Amount:        0,
				Money:         order.Money,
				TradeNo:       order.TradeNo,
				PaymentMethod: order.PaymentMethod,
				CreateTime:    order.CreateTime,
				CompleteTime:  now,
				Status:        common.TopUpStatusSuccess,
			}
			return tx.Create(&topup).Error
		}
		return err
	}
	topup.Money = order.Money
	if topup.PaymentMethod == "" {
		topup.PaymentMethod = order.PaymentMethod
	}
	if topup.CreateTime == 0 {
		topup.CreateTime = order.CreateTime
	}
	topup.CompleteTime = now
	topup.Status = common.TopUpStatusSuccess
	return tx.Save(&topup).Error
}

// ExpireSubscriptionOrder 标记订阅订单为过期。
// expectedPaymentProvider 用于防跨网关回调攻击；传 "" 跳过校验。
func ExpireSubscriptionOrder(tradeNo string, expectedPaymentProvider string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := tx.Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && !order.MatchesPaymentProvider(expectedPaymentProvider) {
			return ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusPending {
			return nil
		}
		// 条件更新而不是「判断 + tx.Save 整行写回」：置过期与支付成功回调可能同时到达，
		// 没有约束的话两者会互相覆盖——最坏情况是订单已发放权益却被标成 expired，
		// 或已过期的订单被回调重新标成 success。这里只允许 Pending -> Expired。
		return tx.Model(&SubscriptionOrder{}).
			Where("id = ? AND status = ?", order.Id, common.TopUpStatusPending).
			Updates(map[string]interface{}{
				"status":        common.TopUpStatusExpired,
				"complete_time": common.GetTimestamp(),
			}).Error
	})
}

// Admin bind (no payment). Creates a UserSubscription from a plan.
func AdminBindSubscription(userId int, planId int, sourceNote string) (string, error) {
	if userId <= 0 || planId <= 0 {
		return "", errors.New("invalid userId or planId")
	}
	plan, err := GetSubscriptionPlanById(planId)
	if err != nil {
		return "", err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "admin")
		return err
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(plan.UpgradeGroup) != "" {
		_ = UpdateUserGroupCache(userId, plan.UpgradeGroup)
		return fmt.Sprintf("用户分组将升级到 %s", plan.UpgradeGroup), nil
	}
	return "", nil
}

// GetAllActiveUserSubscriptions returns all active subscriptions for a user.
func GetAllActiveUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	err := DB.Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

// HasActiveUserSubscription returns whether the user has any active subscription.
// This is a lightweight existence check to avoid heavy pre-consume transactions.
func HasActiveUserSubscription(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetAllUserSubscriptions returns all subscriptions (active and expired) for a user.
func GetAllUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var subs []UserSubscription
	err := DB.Where("user_id = ?", userId).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

func buildSubscriptionSummaries(subs []UserSubscription) []SubscriptionSummary {
	if len(subs) == 0 {
		return []SubscriptionSummary{}
	}
	result := make([]SubscriptionSummary, 0, len(subs))
	for _, sub := range subs {
		subCopy := sub
		result = append(result, SubscriptionSummary{
			Subscription: &subCopy,
		})
	}
	return result
}

// AdminInvalidateUserSubscription marks a user subscription as cancelled and ends it immediately.
func AdminInvalidateUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		// 条件更新：只有仍未取消的订阅才执行作废 + 降级。管理员重复点击或与定时
		// 过期任务撞车时，输的一方 RowsAffected 为 0，直接返回，不会重复降级分组。
		res := tx.Model(&UserSubscription{}).
			Where("id = ? AND status <> ?", userSubscriptionId, "cancelled").
			Updates(map[string]interface{}{
				"status":     "cancelled",
				"end_time":   now,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" && userId > 0 {
		_ = UpdateUserGroupCache(userId, cacheGroup)
	}
	if downgradeGroup != "" {
		return fmt.Sprintf("用户分组将回退到 %s", downgradeGroup), nil
	}
	return "", nil
}

// AdminDeleteUserSubscription hard-deletes a user subscription.
func AdminDeleteUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		// 先删除并用 RowsAffected 认领，再做降级：并发重复删除时只有一个能删掉，
		// 输的一方直接返回，避免把分组降级执行两遍。
		del := tx.Where("id = ?", userSubscriptionId).Delete(&UserSubscription{})
		if del.Error != nil {
			return del.Error
		}
		if del.RowsAffected == 0 {
			return nil
		}
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" && userId > 0 {
		_ = UpdateUserGroupCache(userId, cacheGroup)
	}
	if downgradeGroup != "" {
		return fmt.Sprintf("用户分组将回退到 %s", downgradeGroup), nil
	}
	return "", nil
}

type SubscriptionPreConsumeResult struct {
	UserSubscriptionId int
	PreConsumed        int64
	AmountTotal        int64
	AmountUsedBefore   int64
	AmountUsedAfter    int64
}

// ExpireDueSubscriptions marks expired subscriptions and handles group downgrade.
func ExpireDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("status = ? AND end_time > 0 AND end_time <= ?", "active", now).
		Order("end_time asc, id asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	expiredCount := 0
	userIds := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if sub.UserId > 0 {
			userIds[sub.UserId] = struct{}{}
		}
	}
	for userId := range userIds {
		cacheGroup := ""
		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&UserSubscription{}).
				Where("user_id = ? AND status = ? AND end_time > 0 AND end_time <= ?", userId, "active", now).
				Updates(map[string]interface{}{
					"status":     "expired",
					"updated_at": common.GetTimestamp(),
				})
			if res.Error != nil {
				return res.Error
			}
			expiredCount += int(res.RowsAffected)

			// If there's an active upgraded subscription, keep current group.
			var activeSub UserSubscription
			activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND upgrade_group <> ''",
				userId, "active", now).
				Order("end_time desc, id desc").
				Limit(1).
				Find(&activeSub)
			if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
				return nil
			}

			// No active upgraded subscription, downgrade to previous group if needed.
			var lastExpired UserSubscription
			expiredQuery := tx.Where("user_id = ? AND status = ? AND upgrade_group <> ''",
				userId, "expired").
				Order("end_time desc, id desc").
				Limit(1).
				Find(&lastExpired)
			if expiredQuery.Error != nil || expiredQuery.RowsAffected == 0 {
				return nil
			}
			upgradeGroup := strings.TrimSpace(lastExpired.UpgradeGroup)
			prevGroup := strings.TrimSpace(lastExpired.PrevUserGroup)
			if upgradeGroup == "" || prevGroup == "" {
				return nil
			}
			currentGroup, err := getUserGroupByIdTx(tx, userId)
			if err != nil {
				return err
			}
			if currentGroup != upgradeGroup || currentGroup == prevGroup {
				return nil
			}
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", prevGroup).Error; err != nil {
				return err
			}
			events.PublishBestEffortInTx(tx, events.UserGroupChanged, userId, &events.UserGroupChangedPayload{
				UserId:    userId,
				Email:     fetchUserEmailTx(tx, userId),
				FromGroup: currentGroup,
				ToGroup:   prevGroup,
				Trigger:   "subscription",
				ChangedAt: common.GetTimestamp(),
			})
			cacheGroup = prevGroup
			return nil
		})
		if err != nil {
			return expiredCount, err
		}
		if cacheGroup != "" {
			_ = UpdateUserGroupCache(userId, cacheGroup)
		}
	}
	return expiredCount, nil
}

// SubscriptionPreConsumeRecord stores idempotent pre-consume operations per request.
type SubscriptionPreConsumeRecord struct {
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `json:"user_id" gorm:"index"`
	UserSubscriptionId int    `json:"user_subscription_id" gorm:"index"`
	PreConsumed        int64  `json:"pre_consumed" gorm:"type:bigint;not null;default:0"`
	Status             string `json:"status" gorm:"type:varchar(32);index"` // consumed/refunded
	CreatedAt          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
}

func (r *SubscriptionPreConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	r.CreatedAt = now
	r.UpdatedAt = now
	return nil
}

func (r *SubscriptionPreConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

// 返回值 reset 表示本次是否真的执行了「清空已用额度」的重置（抢到 CAS 才算）。
func maybeResetUserSubscriptionWithPlanTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64) (reset bool, err error) {
	if tx == nil || sub == nil || plan == nil {
		return false, errors.New("invalid reset args")
	}
	if sub.NextResetTime > 0 && sub.NextResetTime > now {
		return false, nil
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return false, nil
	}
	baseUnix := sub.LastResetTime
	if baseUnix <= 0 {
		baseUnix = sub.StartTime
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, sub.EndTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, sub.EndTime)
	}

	// 所有写入都用「CAS 掉 next_reset_time 旧值」来认领，而不是 tx.Save 整行写回。
	//
	// 两个原因：
	//  1. 调用方（PreConsumeUserSubscription / ResetDueSubscriptions）原先靠
	//     tx.Set("gorm:query_option", "FOR UPDATE") 串行化，而那在 GORM v2 下是空操作。
	//     两个实例的定时任务、或定时任务与用户请求撞在一起时会重置两次——AmountUsed
	//     被清零两轮，等于白送一个周期的订阅额度。
	//  2. tx.Save 会把 status / end_time / amount_total 等整行字段按快照重写。
	prevNextResetTime := sub.NextResetTime
	// claim 只有真正抢到时才回填内存中的 sub；输掉时必须把库里的最新值读回来。
	//
	// 否则会出现「内存说 AmountUsed=0、库里其实是赢家写的值」这种不一致，调用方
	// PreConsumeUserSubscription 紧接着就用 sub.AmountUsed 做额度预判，会误判成额度
	// 充足（真正的扣减带 WHERE 条件，不会超扣，但会从"换下一个订阅"退化成直接报错），
	// ResetDueSubscriptions 的计数也会虚高。
	claim := func(updates map[string]interface{}) (bool, error) {
		res := tx.Model(&UserSubscription{}).
			Where("id = ? AND next_reset_time = ?", sub.Id, prevNextResetTime).
			Updates(updates)
		if res.Error != nil {
			return false, res.Error
		}
		if res.RowsAffected == 0 {
			// 已被并发的重置抢先处理：回读库里的真值，保持内存与库一致
			var fresh UserSubscription
			if err := tx.Where("id = ?", sub.Id).First(&fresh).Error; err != nil {
				return false, err
			}
			*sub = fresh
			return false, nil
		}
		return true, nil
	}

	if !advanced {
		if sub.NextResetTime == 0 && next > 0 {
			won, err := claim(map[string]interface{}{
				"next_reset_time": next,
				"last_reset_time": base.Unix(),
				"updated_at":      now,
			})
			if err != nil {
				return false, err
			}
			if won {
				sub.NextResetTime = next
				sub.LastResetTime = base.Unix()
			}
			// 这一支只是补齐 next_reset_time，没有清空额度，不算一次「重置」
			return false, nil
		}
		return false, nil
	}
	won, err := claim(map[string]interface{}{
		"amount_used":     0,
		"next_reset_time": next,
		"last_reset_time": base.Unix(),
		"updated_at":      now,
	})
	if err != nil {
		return false, err
	}
	if won {
		sub.AmountUsed = 0
		sub.LastResetTime = base.Unix()
		sub.NextResetTime = next
	}
	return won, nil
}

// PreConsumeUserSubscription pre-consumes from any active subscription total quota.
func PreConsumeUserSubscription(requestId string, userId int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if strings.TrimSpace(requestId) == "" {
		return nil, errors.New("requestId is empty")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be > 0")
	}
	now := GetDBTimestamp()

	returnValue := &SubscriptionPreConsumeResult{}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing SubscriptionPreConsumeRecord
		query := tx.Where("request_id = ?", requestId).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.Status == "refunded" {
				return errors.New("subscription pre-consume already refunded")
			}
			var sub UserSubscription
			if err := tx.Where("id = ?", existing.UserSubscriptionId).First(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = existing.PreConsumed
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = sub.AmountUsed
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}

		// 这里不再假装加锁（GORM v2 下 query_option 是空操作）。真正的并发保护下沉到
		// 了写入处：额度自增带「不超总额」条件，重置对 next_reset_time 做 CAS。
		var subs []UserSubscription
		if err := tx.
			Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
			Order("end_time asc, id asc").
			Find(&subs).Error; err != nil {
			return errors.New("no active subscription")
		}
		if len(subs) == 0 {
			return errors.New("no active subscription")
		}
		for _, candidate := range subs {
			sub := candidate
			plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
			if err != nil {
				return err
			}
			if _, err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
				return err
			}
			usedBefore := sub.AmountUsed
			if sub.AmountTotal > 0 {
				remain := sub.AmountTotal - usedBefore
				if remain < amount {
					continue
				}
			}
			record := &SubscriptionPreConsumeRecord{
				RequestId:          requestId,
				UserId:             userId,
				UserSubscriptionId: sub.Id,
				PreConsumed:        amount,
				Status:             "consumed",
			}
			if err := tx.Create(record).Error; err != nil {
				var dup SubscriptionPreConsumeRecord
				if err2 := tx.Where("request_id = ?", requestId).First(&dup).Error; err2 == nil {
					if dup.Status == "refunded" {
						return errors.New("subscription pre-consume already refunded")
					}
					returnValue.UserSubscriptionId = sub.Id
					returnValue.PreConsumed = dup.PreConsumed
					returnValue.AmountTotal = sub.AmountTotal
					returnValue.AmountUsedBefore = sub.AmountUsed
					returnValue.AmountUsedAfter = sub.AmountUsed
					return nil
				}
				return err
			}
			// 原来是 sub.AmountUsed += amount 再 tx.Save(&sub)：整行写回 + 内存里累加，
			// 上面那个 FOR UPDATE 又是空操作，两个并发预扣会各自读到同一个 AmountUsed
			// 覆盖对方，用户白嫖一笔订阅额度。requestId 唯一索引只能保证同一个请求幂等，
			// 挡不住不同请求。这里换成带「不超总额」条件的原子自增；真撞上超额时整个
			// 事务回滚（含刚创建的幂等记录），失败方向是拒绝而不是放行。
			if err := postConsumeUserSubscriptionDeltaTx(tx, sub.Id, amount); err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = amount
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = usedBefore
			returnValue.AmountUsedAfter = usedBefore + amount
			return nil
		}
		return fmt.Errorf("subscription quota insufficient, need=%d", amount)
	})
	if err != nil {
		return nil, err
	}
	return returnValue, nil
}

// RefundSubscriptionPreConsume is idempotent and refunds pre-consumed subscription quota by requestId.
func RefundSubscriptionPreConsume(requestId string) error {
	if strings.TrimSpace(requestId) == "" {
		return errors.New("requestId is empty")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := tx.Where("request_id = ?", requestId).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" {
			return nil
		}

		// 先原子认领「consumed -> refunded」，再退额度。
		//
		// 原来是「判断 status != refunded -> 退额度 -> tx.Save 标 refunded」，依赖的
		// FOR UPDATE 在 GORM v2 下是空操作。两个并发退款会双双通过判断，把同一笔预扣
		// 退两次，订阅额度凭空多出来。认领放在退款之前，输的一方直接返回，不退第二次。
		claim := tx.Model(&SubscriptionPreConsumeRecord{}).
			Where("id = ? AND status = ?", record.Id, "consumed").
			Updates(map[string]interface{}{
				"status":     "refunded",
				"updated_at": common.GetTimestamp(),
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			// 已被并发退款处理，幂等返回
			return nil
		}
		if record.PreConsumed <= 0 {
			return nil
		}
		// 用 tx 版本：退额度与上面的状态认领必须同生共死，否则「已退款但记录仍是
		// consumed」的中间态会在重试时被再退一次。
		return postConsumeUserSubscriptionDeltaTx(tx, record.UserSubscriptionId, -record.PreConsumed)
	})
}

// ResetDueSubscriptions resets subscriptions whose next_reset_time has passed.
func ResetDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("next_reset_time > 0 AND next_reset_time <= ? AND status = ?", now, "active").
		Order("next_reset_time asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	resetCount := 0
	for _, sub := range subs {
		subCopy := sub
		plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
		if err != nil || plan == nil {
			continue
		}
		err = DB.Transaction(func(tx *gorm.DB) error {
			// 读取不加锁（原来的 FOR UPDATE 是空操作）；重置的互斥由
			// maybeResetUserSubscriptionWithPlanTx 内部对 next_reset_time 的 CAS 保证，
			// 多实例同时跑也只会有一个真正重置成功。
			var locked UserSubscription
			if err := tx.
				Where("id = ? AND next_reset_time > 0 AND next_reset_time <= ?", subCopy.Id, now).
				First(&locked).Error; err != nil {
				return nil
			}
			// 只统计真正抢到 CAS、确实清空了额度的那一次；输给并发实例的不算，
			// 否则多实例同时跑时报出来的重置数会成倍虚高。
			didReset, err := maybeResetUserSubscriptionWithPlanTx(tx, &locked, plan, now)
			if err != nil {
				return err
			}
			if didReset {
				resetCount++
			}
			return nil
		})
		if err != nil {
			return resetCount, err
		}
	}
	return resetCount, nil
}

// CleanupSubscriptionPreConsumeRecords removes old idempotency records to keep table small.
func CleanupSubscriptionPreConsumeRecords(olderThanSeconds int64) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 7 * 24 * 3600
	}
	cutoff := GetDBTimestamp() - olderThanSeconds
	res := DB.Where("updated_at < ?", cutoff).Delete(&SubscriptionPreConsumeRecord{})
	return res.RowsAffected, res.Error
}

type SubscriptionPlanInfo struct {
	PlanId    int
	PlanTitle string
}

func GetSubscriptionPlanInfoByUserSubscriptionId(userSubscriptionId int) (*SubscriptionPlanInfo, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	cacheKey := fmt.Sprintf("sub:%d", userSubscriptionId)
	if cached, found, err := getSubscriptionPlanInfoCache().Get(cacheKey); err == nil && found {
		return &cached, nil
	}
	var sub UserSubscription
	if err := DB.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
	if err != nil {
		return nil, err
	}
	info := &SubscriptionPlanInfo{
		PlanId:    sub.PlanId,
		PlanTitle: plan.Title,
	}
	_ = getSubscriptionPlanInfoCache().SetWithTTL(cacheKey, *info, subscriptionPlanInfoCacheTTL())
	return info, nil
}

// Update subscription used amount by delta (positive consume more, negative refund).
func PostConsumeUserSubscriptionDelta(userSubscriptionId int, delta int64) error {
	return postConsumeUserSubscriptionDeltaTx(DB, userSubscriptionId, delta)
}

// postConsumeUserSubscriptionDeltaTx 是上面的事务内版本。
// 退款路径需要「标记 refunded」和「退还额度」落在同一个事务里，否则退款已生效但外层
// 提交失败时，重试会把同一笔预扣退两次。
func postConsumeUserSubscriptionDeltaTx(db *gorm.DB, userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}
	// 这是订阅额度的结算热路径，每个请求都会走，并发度最高。
	//
	// 原来的写法是「读整行 -> newUsed = sub.AmountUsed + delta -> tx.Save(&sub)」，
	// 而它依赖的 tx.Set("gorm:query_option", "FOR UPDATE") 是 GORM v1 的 API，v2 下是
	// 空操作（生成的 SQL 里没有 FOR UPDATE）。两个并发结算各自读到同一个 AmountUsed，
	// 后写的覆盖先写的，消费就丢了——和资料整行写回回滚 used_quota 是同一类丢失更新。
	// tx.Save 还是整行写回，会把 AmountTotal 等字段一并写成快照值。
	//
	// 改成单列原子表达式：扣减把「不超总额」写进 WHERE，用 RowsAffected 判定；
	// 退款用 CASE WHEN 在 SQL 里夹到 0，三种库都支持。
	if delta > 0 {
		res := db.Model(&UserSubscription{}).
			Where("id = ? AND (amount_total <= 0 OR amount_used + ? <= amount_total)", userSubscriptionId, delta).
			Update("amount_used", gorm.Expr("amount_used + ?", delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 要么订阅不存在，要么这一笔会超总额。回读一次给出可诊断的错误。
			var sub UserSubscription
			if err := db.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
				return err
			}
			return fmt.Errorf("subscription used exceeds total, used=%d delta=%d total=%d",
				sub.AmountUsed, delta, sub.AmountTotal)
		}
		return nil
	}

	res := db.Model(&UserSubscription{}).
		Where("id = ?", userSubscriptionId).
		Update("amount_used", gorm.Expr("CASE WHEN amount_used + ? < 0 THEN 0 ELSE amount_used + ? END", delta, delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 退款路径不能直接把 RowsAffected==0 当成「订阅不存在」：MySQL 默认不开
		// CLIENT_FOUND_ROWS，当 amount_used 已经是 0、CASE WHEN 又夹回 0 时，新旧值
		// 相同，受影响行数同样是 0。必须回查一次才能区分两种情况。
		var cnt int64
		if err := db.Model(&UserSubscription{}).Where("id = ?", userSubscriptionId).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return fmt.Errorf("subscription %d not found", userSubscriptionId)
		}
	}
	return nil
}
