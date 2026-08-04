package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 这一组测试针对「FOR UPDATE 实为空操作」暴露出来的资金路径竞态。
//
// 背景：仓库里十余处 tx.Set("gorm:query_option", "FOR UPDATE") 沿用的是 GORM v1 的 API，
// v2 已移除，生成的 SQL 里根本没有 FOR UPDATE。所有依赖它串行化的「读 -> 判断 -> 写」
// 都是敞开的：兑换码重复到账、支付回调重复到账、订阅额度丢失更新。
//
// 修复统一采用「条件更新 + RowsAffected 判定」而不是补 clause.Locking——SQLite 不支持
// 行锁（CLAUDE.md Rule 2 要求三库同时兼容），条件更新一条语句原子完成，也不依赖隔离级别。
func newMoneyPathTestDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.RedisEnabled = false
	prevDB := DB
	// 用文件而非 :memory:，并发连接才能看到同一份数据
	dsn := fmt.Sprintf("file:%s/%s.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		t.TempDir(), strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	t.Cleanup(func() {
		DB = prevDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// 同一张兑换码被多个账号并发提交，只能有一个到账。
//
// 这是原漏洞里最容易变现的一条：把一张码同时投给 N 个自己的账号即可复制额度。
// 上层 TopUp 的 trylock 是 per-user 的进程内锁，既挡不住不同账号，也跨不了节点。
func TestRedeem_ConcurrentSameCodeCreditsOnce(t *testing.T) {
	db := newMoneyPathTestDB(t, &User{}, &Redemption{}, &Log{})

	const users = 8
	userIds := make([]int, 0, users)
	for i := 0; i < users; i++ {
		// aff_code 上有唯一索引，留空会在第二个用户上撞唯一约束
		u := &User{Username: fmt.Sprintf("u%d", i), Password: "x", Quota: 0,
			AffCode: fmt.Sprintf("aff%d", i)}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		userIds = append(userIds, u.Id)
	}

	code := &Redemption{Key: "TESTKEY0000000000000000000000001", Quota: 5000,
		Status: common.RedemptionCodeStatusEnabled, Name: "t"}
	if err := db.Create(code).Error; err != nil {
		t.Fatalf("create redemption: %v", err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	okCount := 0
	for _, uid := range userIds {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			if _, err := Redeem(code.Key, uid); err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}(uid)
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("同一张码被兑换成功 %d 次，应当只有 1 次", okCount)
	}

	var totalQuota int64
	if err := db.Model(&User{}).Select("COALESCE(SUM(quota),0)").Scan(&totalQuota).Error; err != nil {
		t.Fatalf("sum quota: %v", err)
	}
	if totalQuota != 5000 {
		t.Fatalf("发放额度合计 %d，应当等于面额 5000（多出来即为重复到账）", totalQuota)
	}

	var after Redemption
	if err := db.First(&after, code.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Status != common.RedemptionCodeStatusUsed || after.UsedUserId == 0 {
		t.Fatalf("兑换码状态不对：status=%d used_user_id=%d", after.Status, after.UsedUserId)
	}
}

// 支付网关重试导致的并发重复回调，只能到账一次。
func TestClaimPendingTopUp_ConcurrentCallbackCreditsOnce(t *testing.T) {
	db := newMoneyPathTestDB(t, &User{}, &TopUp{})

	u := &User{Username: "payer", Password: "x", Quota: 0}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	order := &TopUp{UserId: u.Id, Amount: 100, Money: 10, TradeNo: "T-1",
		Status: common.TopUpStatusPending}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create topup: %v", err)
	}

	const callbacks = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimedCount := 0
	for i := 0; i < callbacks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = DB.Transaction(func(tx *gorm.DB) error {
				claimed, _, err := claimPendingTopUpTx(tx, order.Id, 12345)
				if err != nil {
					return err
				}
				if !claimed {
					return nil
				}
				// 只有认领成功的那一个才允许加额度
				if err := tx.Model(&User{}).Where("id = ?", u.Id).
					Update("quota", gorm.Expr("quota + ?", 100)).Error; err != nil {
					return err
				}
				mu.Lock()
				claimedCount++
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()

	if claimedCount != 1 {
		t.Fatalf("订单被认领 %d 次，应当只有 1 次", claimedCount)
	}
	var after User
	if err := db.First(&after, u.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Quota != 100 {
		t.Fatalf("到账 %d，应当等于 100（多出来即为重复到账）", after.Quota)
	}
}

// 订阅额度结算是热路径，并发扣减一分都不能丢。
//
// 原实现是「读 sub.AmountUsed -> 内存加 delta -> tx.Save 整行写回」，
// 并发下后写覆盖先写，消费直接丢账，与本次事故的 used_quota 回滚同类。
func TestPostConsumeUserSubscriptionDelta_ConcurrentNoLostUpdate(t *testing.T) {
	db := newMoneyPathTestDB(t, &UserSubscription{})

	sub := &UserSubscription{UserId: 1, PlanId: 1, AmountTotal: 100000, AmountUsed: 0, Status: "active"}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}

	const goroutines, perGoroutine, delta = 8, 20, 5
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := PostConsumeUserSubscriptionDelta(sub.Id, delta); err != nil {
					t.Errorf("consume: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	var after UserSubscription
	if err := db.First(&after, sub.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	want := int64(goroutines * perGoroutine * delta)
	if after.AmountUsed != want {
		t.Fatalf("订阅消费丢账：期望 %d，实际 %d", want, after.AmountUsed)
	}
}

// 扣减不得突破总额；退款不得把 amount_used 打成负数。
func TestPostConsumeUserSubscriptionDelta_BoundsAndRefund(t *testing.T) {
	db := newMoneyPathTestDB(t, &UserSubscription{})

	sub := &UserSubscription{UserId: 1, PlanId: 1, AmountTotal: 100, AmountUsed: 90, Status: "active"}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}

	// 超总额必须拒绝，且不留下部分写入
	if err := PostConsumeUserSubscriptionDelta(sub.Id, 20); err == nil {
		t.Fatal("超出订阅总额时应当报错")
	}
	var after UserSubscription
	db.First(&after, sub.Id)
	if after.AmountUsed != 90 {
		t.Fatalf("被拒绝的扣减不应写库：%d", after.AmountUsed)
	}

	// 正好用满是允许的
	if err := PostConsumeUserSubscriptionDelta(sub.Id, 10); err != nil {
		t.Fatalf("用满总额应当允许: %v", err)
	}
	db.First(&after, sub.Id)
	if after.AmountUsed != 100 {
		t.Fatalf("期望 100，实际 %d", after.AmountUsed)
	}

	// 退款超过已用部分，要夹到 0 而不是负数
	if err := PostConsumeUserSubscriptionDelta(sub.Id, -500); err != nil {
		t.Fatalf("退款不应报错: %v", err)
	}
	db.First(&after, sub.Id)
	if after.AmountUsed != 0 {
		t.Fatalf("退款应夹到 0，实际 %d", after.AmountUsed)
	}

	// 已经是 0 再退一次：MySQL 下新旧值相同会返回 RowsAffected=0，
	// 不能因此被误判成「订阅不存在」而报错
	if err := PostConsumeUserSubscriptionDelta(sub.Id, -10); err != nil {
		t.Fatalf("amount_used 已为 0 时再退款不应报错: %v", err)
	}

	// 真正不存在的订阅才应该报错
	if err := PostConsumeUserSubscriptionDelta(999999, -10); err == nil {
		t.Fatal("订阅不存在时应当报错")
	}
}

// 并发退同一笔预扣，只能退一次。
func TestRefundSubscriptionPreConsume_ConcurrentRefundsOnce(t *testing.T) {
	db := newMoneyPathTestDB(t, &UserSubscription{}, &SubscriptionPreConsumeRecord{})

	sub := &UserSubscription{UserId: 1, PlanId: 1, AmountTotal: 1000, AmountUsed: 500, Status: "active"}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}
	rec := &SubscriptionPreConsumeRecord{RequestId: "req-1", UserId: 1,
		UserSubscriptionId: sub.Id, PreConsumed: 200, Status: "consumed"}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create record: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = RefundSubscriptionPreConsume("req-1")
		}()
	}
	wg.Wait()

	var after UserSubscription
	if err := db.First(&after, sub.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.AmountUsed != 300 {
		t.Fatalf("重复退款：期望 amount_used 为 300（500-200），实际 %d", after.AmountUsed)
	}
	var afterRec SubscriptionPreConsumeRecord
	db.First(&afterRec, rec.Id)
	if afterRec.Status != "refunded" {
		t.Fatalf("记录状态应为 refunded，实际 %q", afterRec.Status)
	}
}

// 上面那个并发兑换测试在 SQLite 上有个局限：SQLite 会把写事务完全串行化，第二个事务
// 总能读到第一个已提交的 used 状态，所以**即使认领语句丢掉 status 条件它也会通过**。
// 真正的竞态只在 MySQL / PostgreSQL 上出现——两个事务并发执行，双双读到 enabled。
//
// 这里手工构造那个交错，把「认领必须带 status 条件」这一点单独钉死：
// 事务 A 已经读到 enabled（相当于拿到了旧快照），期间 B 完整兑换成功，A 随后才执行
// 认领——此时认领必须 0 行受影响，A 不能到账。
func TestRedeem_ClaimIsGuardedByStatus(t *testing.T) {
	db := newMoneyPathTestDB(t, &User{}, &Redemption{}, &Log{})

	userA := &User{Username: "A", Password: "x", AffCode: "affA"}
	userB := &User{Username: "B", Password: "x", AffCode: "affB"}
	if err := db.Create(userA).Error; err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := db.Create(userB).Error; err != nil {
		t.Fatalf("create B: %v", err)
	}
	code := &Redemption{Key: "GUARD000000000000000000000000001", Quota: 5000,
		Status: common.RedemptionCodeStatusEnabled, Name: "t"}
	if err := db.Create(code).Error; err != nil {
		t.Fatalf("create code: %v", err)
	}

	// A：读到 enabled，通过了状态判断（此刻还没认领）
	var snapshotByA Redemption
	if err := db.Where("id = ?", code.Id).First(&snapshotByA).Error; err != nil {
		t.Fatalf("A read: %v", err)
	}
	if snapshotByA.Status != common.RedemptionCodeStatusEnabled {
		t.Fatal("前置条件不成立：A 应当读到 enabled")
	}

	// B：完整兑换成功
	if _, err := Redeem(code.Key, userB.Id); err != nil {
		t.Fatalf("B 兑换应当成功: %v", err)
	}

	// A：现在才执行认领。带 status 条件时必须认领失败。
	claim := db.Model(&Redemption{}).
		Where("id = ? AND status = ?", snapshotByA.Id, common.RedemptionCodeStatusEnabled).
		Updates(map[string]interface{}{
			"status":       common.RedemptionCodeStatusUsed,
			"used_user_id": userA.Id,
		})
	if claim.Error != nil {
		t.Fatalf("claim: %v", claim.Error)
	}
	if claim.RowsAffected != 0 {
		t.Fatal("A 不应认领成功——认领语句必须带 status = enabled 条件，否则同一张码会到账两次")
	}

	var a, b User
	db.First(&a, userA.Id)
	db.First(&b, userB.Id)
	if a.Quota != 0 {
		t.Fatalf("A 不应到账，实际 %d", a.Quota)
	}
	if b.Quota != 5000 {
		t.Fatalf("B 应当到账 5000，实际 %d", b.Quota)
	}
}

// 订阅额度的周期性重置必须只生效一次，且输掉 CAS 的一方必须把内存中的 sub
// 与库里对齐。
//
// 后半句是重构时踩过的坑：如果在 CAS 之前就把 sub.AmountUsed 改成 0，输掉之后内存里
// 是 0、库里却是赢家写的值。调用方 PreConsumeUserSubscription 紧接着就拿 sub.AmountUsed
// 做额度预判，会误判成额度充足；ResetDueSubscriptions 的计数也会翻倍虚高。
func TestMaybeResetUserSubscription_ClaimOnceAndKeepsStructInSync(t *testing.T) {
	db := newMoneyPathTestDB(t, &UserSubscription{}, &SubscriptionPlan{})

	now := common.GetTimestamp()
	plan := &SubscriptionPlan{Title: "p", QuotaResetPeriod: SubscriptionResetDaily,
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 1000}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	sub := &UserSubscription{
		UserId: 1, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 700, Status: "active",
		StartTime: now - 86400*3, EndTime: now + 86400*30,
		LastResetTime: now - 86400*2, NextResetTime: now - 10, // 已到重置时间
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}

	// 两个「实例」拿着同一份快照同时来重置
	a := *sub
	b := *sub

	wonA, err := maybeResetUserSubscriptionWithPlanTx(db, &a, plan, now)
	if err != nil {
		t.Fatalf("reset A: %v", err)
	}

	// 关键：A 重置完之后又发生了真实消费。这样库里的 amount_used 就不再等于
	// B 自己算出来的 0——只有让「赢家写入的值」和「输家算出的值」不同，才能真正
	// 检验输家有没有把库里的真值读回来。否则两边算出同一个值，bug 会被掩盖。
	if err := PostConsumeUserSubscriptionDelta(sub.Id, 400); err != nil {
		t.Fatalf("consume after reset: %v", err)
	}

	wonB, err := maybeResetUserSubscriptionWithPlanTx(db, &b, plan, now)
	if err != nil {
		t.Fatalf("reset B: %v", err)
	}

	if !wonA || wonB {
		t.Fatalf("应当只有先到的那次重置成功：wonA=%v wonB=%v", wonA, wonB)
	}

	var after UserSubscription
	if err := db.First(&after, sub.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.AmountUsed != 400 {
		t.Fatalf("期望重置清零后再消费 400，实际 %d", after.AmountUsed)
	}
	if after.NextResetTime <= now {
		t.Fatalf("next_reset_time 应当被推到将来，实际 %d（now=%d）", after.NextResetTime, now)
	}

	// 输掉的一方必须已经把库里的真值读回来，而不是停留在自己算出的 0
	if b.NextResetTime != after.NextResetTime || b.AmountUsed != after.AmountUsed {
		t.Fatalf("输掉 CAS 的一方内存与库不一致：内存 used=%d next=%d，库 used=%d next=%d",
			b.AmountUsed, b.NextResetTime, after.AmountUsed, after.NextResetTime)
	}
}
