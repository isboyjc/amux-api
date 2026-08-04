package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newLostUpdateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.RedisEnabled = false
	prevDB := DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
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
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// 回归：用户自助改设置（侧边栏/语言/通知/账单偏好）绝不能回写计费列。
//
// 历史缺陷：这些入口走 User.Update()，内部是 DB.Model(user).Updates(newUser) —— GORM
// 结构体更新会写入所有非零字段，而 newUser 是请求开始时读出的完整用户快照，于是
// quota / used_quota / request_count 被一并写回 T0 的旧值。并发调用即可把中间发生的
// 计费原子自增全部抹掉（余额复原 + 消费不计账）。
func TestUpdateUserSettingColumn_DoesNotClobberBillingColumns(t *testing.T) {
	db := newLostUpdateTestDB(t)

	u := &User{Username: "victim", Password: "x", Quota: 1000, UsedQuota: 100, RequestCount: 2}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// T0：请求开始，读出完整用户（controller 里就是 GetUserById(userId, false)）
	snapshot, err := GetUserById(u.Id, false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// T0+Δ：并发的计费结算，原子自增/自减
	if err := db.Model(&User{}).Where("id = ?", u.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 300),
		"used_quota":    gorm.Expr("used_quota + ?", 300),
		"request_count": gorm.Expr("request_count + ?", 5),
	}).Error; err != nil {
		t.Fatalf("billing: %v", err)
	}

	// T1：用户的设置更新落库
	setting := snapshot.GetSetting()
	setting.Language = "en"
	snapshot.SetSetting(setting)
	if err := UpdateUserSettingColumn(snapshot.Id, snapshot.Setting); err != nil {
		t.Fatalf("update setting: %v", err)
	}

	var after User
	if err := db.First(&after, u.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Quota != 700 {
		t.Fatalf("quota 被回滚：期望 700，实际 %d", after.Quota)
	}
	if after.UsedQuota != 400 {
		t.Fatalf("used_quota 被回滚：期望 400，实际 %d", after.UsedQuota)
	}
	if after.RequestCount != 7 {
		t.Fatalf("request_count 被回滚：期望 7，实际 %d", after.RequestCount)
	}
	if !strings.Contains(after.Setting, `"language":"en"`) {
		t.Fatalf("setting 没写进去：%q", after.Setting)
	}
}

// User.Update 是历史遗留的宽口径写入，被邮箱/OAuth 绑定等路径复用。
// 即使调用方传的是完整快照，它也不允许触碰计费列。
func TestUserUpdate_DoesNotClobberBillingColumns(t *testing.T) {
	db := newLostUpdateTestDB(t)

	u := &User{Username: "binder", Password: "x", Quota: 1000, UsedQuota: 100, RequestCount: 2}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	snapshot, err := GetUserById(u.Id, false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := db.Model(&User{}).Where("id = ?", u.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 400),
		"used_quota":    gorm.Expr("used_quota + ?", 400),
		"request_count": gorm.Expr("request_count + ?", 7),
	}).Error; err != nil {
		t.Fatalf("billing: %v", err)
	}

	snapshot.Email = "bound@example.com"
	if err := snapshot.Update(false); err != nil {
		t.Fatalf("update: %v", err)
	}

	var after User
	if err := db.First(&after, u.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Quota != 600 || after.UsedQuota != 500 || after.RequestCount != 9 {
		t.Fatalf("计费列被回滚：quota=%d used_quota=%d request_count=%d（期望 600/500/9）",
			after.Quota, after.UsedQuota, after.RequestCount)
	}
	if after.Email != "bound@example.com" {
		t.Fatalf("业务字段没写进去：%q", after.Email)
	}
}

// 回归：转移邀请额度必须是原子的条件更新，且不得回写计费列。
//
// 历史缺陷：该方法用 tx.Set("gorm:query_option", "FOR UPDATE") 加锁 —— 那是 GORM v1
// 的 API，v2 已移除，生成的 SQL 里根本没有 FOR UPDATE；随后的 tx.Save(user) 又是整行
// 写回，会把 T0 快照的 used_quota / request_count 一起落库。
func TestTransferAffQuotaToQuota_AtomicAndDoesNotClobberBilling(t *testing.T) {
	db := newLostUpdateTestDB(t)

	prevUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = prevUnit })

	u := &User{Username: "affuser", Password: "x", Quota: 1000, UsedQuota: 100, RequestCount: 2, AffQuota: 500}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	snapshot, err := GetUserById(u.Id, true)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// 并发计费结算
	if err := db.Model(&User{}).Where("id = ?", u.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 300),
		"used_quota":    gorm.Expr("used_quota + ?", 300),
		"request_count": gorm.Expr("request_count + ?", 5),
	}).Error; err != nil {
		t.Fatalf("billing: %v", err)
	}

	if err := snapshot.TransferAffQuotaToQuota(200); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	var after User
	if err := db.First(&after, u.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	// 700（计费后）+ 200（转入）
	if after.Quota != 900 {
		t.Fatalf("quota 错：期望 900，实际 %d", after.Quota)
	}
	if after.AffQuota != 300 {
		t.Fatalf("aff_quota 错：期望 300，实际 %d", after.AffQuota)
	}
	if after.UsedQuota != 400 || after.RequestCount != 7 {
		t.Fatalf("计费列被回滚：used_quota=%d request_count=%d（期望 400/7）", after.UsedQuota, after.RequestCount)
	}

	// 余额不足时必须整体拒绝，且不产生任何写入
	if err := snapshot.TransferAffQuotaToQuota(9999); err == nil {
		t.Fatal("邀请额度不足时应当报错")
	}
	if err := db.First(&after, u.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Quota != 900 || after.AffQuota != 300 {
		t.Fatalf("失败的转移不应写库：quota=%d aff_quota=%d", after.Quota, after.AffQuota)
	}
}

// 回归：充值后自动升组只写 group 单列，不能把刚到账的额度写回旧值。
// 同时固定住列名引号处理 —— group 是三种库的保留字，写错会直接报语法错误。
func TestUpdateUserGroupColumn_DoesNotClobberBilling(t *testing.T) {
	db := newLostUpdateTestDB(t)

	u := &User{Username: "upgrader", Password: "x", Quota: 1000, UsedQuota: 100, Group: "default"}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 充值到账
	if err := db.Model(&User{}).Where("id = ?", u.Id).
		Update("quota", gorm.Expr("quota + ?", 5000)).Error; err != nil {
		t.Fatalf("topup: %v", err)
	}

	if err := UpdateUserGroupColumn(u.Id, "vip"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	var after User
	if err := db.First(&after, u.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Group != "vip" {
		t.Fatalf("分组没升上去：%q", after.Group)
	}
	if after.Quota != 6000 {
		t.Fatalf("充值额度被抹掉：期望 6000，实际 %d", after.Quota)
	}
}

// 回归：邀请奖励用原子自增写 aff_* 列，不碰计费列；邀请人不存在时必须报错。
func TestInviteUser_AtomicAndDoesNotClobberBilling(t *testing.T) {
	db := newLostUpdateTestDB(t)

	prevInviter := common.QuotaForInviter
	common.QuotaForInviter = 1000
	t.Cleanup(func() { common.QuotaForInviter = prevInviter })

	if err := db.AutoMigrate(&AffRebateLog{}); err != nil {
		t.Fatalf("migrate log: %v", err)
	}

	inviter := &User{Username: "inviter", Password: "x", Quota: 800, UsedQuota: 100, AffCount: 1, AffQuota: 50, AffHistoryQuota: 50}
	if err := db.Create(inviter).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 邀请人自己正在跑请求，计费同时在结算
	if err := db.Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]interface{}{
		"quota":      gorm.Expr("quota - ?", 300),
		"used_quota": gorm.Expr("used_quota + ?", 300),
	}).Error; err != nil {
		t.Fatalf("billing: %v", err)
	}

	if err := inviteUser(inviter.Id, 9999); err != nil {
		t.Fatalf("invite: %v", err)
	}

	var after User
	if err := db.First(&after, inviter.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.AffCount != 2 || after.AffQuota != 1050 || after.AffHistoryQuota != 1050 {
		t.Fatalf("邀请奖励没记对：aff_count=%d aff_quota=%d aff_history=%d（期望 2/1050/1050）",
			after.AffCount, after.AffQuota, after.AffHistoryQuota)
	}
	if after.Quota != 500 || after.UsedQuota != 400 {
		t.Fatalf("计费列被回滚：quota=%d used_quota=%d（期望 500/400）", after.Quota, after.UsedQuota)
	}

	// 邀请人不存在时不能静默成功（否则会记出一条挂空账号的返现流水）
	if err := inviteUser(123456, 9999); err == nil {
		t.Fatal("邀请人不存在时应当报错")
	}
}
