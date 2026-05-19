package marketing

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/events"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 把 model.DB 指向一个临时 SQLite，迁移 User + TopUp 表，
// 返回清理函数。规则测试需要真 DB 因为 Resolve 内部调 model.GetUserById 等。
func setupTestDB(t *testing.T) func() {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "marketing_test.db")
	d, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := d.AutoMigrate(&model.User{}, &model.TopUp{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	prevDB := model.DB
	model.DB = d
	// 测试绕过 InitDB() 直接给 model.DB 赋值，需要手工把跨库列名变量初始化
	// （GetPaidUserIDsBatch 等会用到 commonGroupCol）
	model.InitCommonColumnsForTest()
	return func() {
		sqlDB, _ := d.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		model.DB = prevDB
	}
}

func createUser(t *testing.T, u *model.User) {
	t.Helper()
	if u.CreatedTime == 0 {
		u.CreatedTime = time.Now().Unix()
	}
	if u.AffCode == "" {
		u.AffCode = common.GetRandomString(4)
	}
	if err := model.DB.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func recordSuccessTopup(t *testing.T, userId int, money float64) {
	t.Helper()
	t1 := &model.TopUp{
		UserId:     userId,
		Money:      money,
		Amount:     int64(money),
		TradeNo:    common.GetRandomString(16),
		Status:     common.TopUpStatusSuccess,
		CreateTime: time.Now().Unix(),
	}
	if err := model.DB.Create(t1).Error; err != nil {
		t.Fatalf("create topup: %v", err)
	}
}

func eventForUser(eventType string, userId int, payload []byte) events.Event {
	return events.Event{
		Type:        eventType,
		AggregateId: userId,
		Payload:     payload,
		PublishedAt: time.Now().Unix(),
	}
}

func TestResolve_DefaultUserWithoutTopup_NotInPlatform(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "free", Email: "free@example.com", Group: "default", DisplayName: "Free"}
	createUser(t, u)

	got, err := Resolve(context.Background(), eventForUser(events.UserProfileUpdated, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Tier != TierNone {
		t.Fatalf("expected TierNone for free default user; got %+v", got)
	}
}

func TestResolve_DefaultUserWithTopup_Default(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "paid", Email: "paid@example.com", Group: "default", DisplayName: "Paid"}
	createUser(t, u)
	recordSuccessTopup(t, u.Id, 10)

	got, err := Resolve(context.Background(), eventForUser(events.BillingTopupSucceeded, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Tier != TierDefault {
		t.Fatalf("expected TierDefault; got %+v", got)
	}
	if got.TargetEmail != "paid@example.com" {
		t.Fatalf("wrong email: %s", got.TargetEmail)
	}
	if got.DisplayName != "Paid" {
		t.Fatalf("wrong name: %s", got.DisplayName)
	}
}

func TestResolve_VIPUser_AlwaysVIP_NoTopupCheck(t *testing.T) {
	defer setupTestDB(t)()
	// VIP 用户哪怕一笔充值都没有（线下/admin 调额度场景）也应进 TierVIP
	u := &model.User{Username: "vip", Email: "vip@example.com", Group: "vip", DisplayName: "VIP"}
	createUser(t, u)

	got, err := Resolve(context.Background(), eventForUser(events.UserGroupChanged, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Tier != TierVIP {
		t.Fatalf("expected TierVIP without topup check; got %+v", got)
	}
}

func TestResolve_EnterpriseGroup_None(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "ent", Email: "ent@example.com", Group: "enterprise_a"}
	createUser(t, u)
	recordSuccessTopup(t, u.Id, 9999) // 充了大钱也不进，因为不是 default/vip

	got, err := Resolve(context.Background(), eventForUser(events.UserGroupChanged, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Tier != TierNone {
		t.Fatalf("expected TierNone for non-default/non-vip group; got %+v", got)
	}
}

func TestResolve_UserDeleted_UsesPayloadEmail(t *testing.T) {
	defer setupTestDB(t)()
	// 不创建 user 行 —— 模拟硬删后 DB 查不到
	payload, _ := common.Marshal(&events.UserDeletedPayload{
		UserId:    999,
		Email:     "gone@example.com",
		Username:  "gone",
		DeletedAt: time.Now().Unix(),
	})

	got, err := Resolve(context.Background(), eventForUser(events.UserDeleted, 999, payload))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Tier != TierNone || got.TargetEmail != "gone@example.com" {
		t.Fatalf("expected TierNone with payload email; got %+v", got)
	}
}

func TestResolve_EmailBound_AddsCleanupEmail(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "u", Email: "new@example.com", Group: "default"}
	createUser(t, u)
	recordSuccessTopup(t, u.Id, 10)

	payload, _ := common.Marshal(&events.UserEmailBoundPayload{
		UserId:   u.Id,
		OldEmail: "old@example.com",
		NewEmail: "new@example.com",
	})
	got, err := Resolve(context.Background(), eventForUser(events.UserEmailBound, u.Id, payload))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil {
		t.Fatal("expected intent, got nil")
	}
	if got.TargetEmail != "new@example.com" {
		t.Fatalf("target email wrong: %s", got.TargetEmail)
	}
	if got.CleanupEmail != "old@example.com" {
		t.Fatalf("cleanup email wrong: %s", got.CleanupEmail)
	}
	if got.Tier != TierDefault {
		t.Fatalf("expected TierDefault; got %v", got.Tier)
	}
}

func TestResolve_EmailBound_FirstTimeBind_NoCleanup(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "u", Email: "first@example.com", Group: "default"}
	createUser(t, u)
	recordSuccessTopup(t, u.Id, 10)

	payload, _ := common.Marshal(&events.UserEmailBoundPayload{
		UserId:   u.Id,
		OldEmail: "", // 首次绑定
		NewEmail: "first@example.com",
	})
	got, err := Resolve(context.Background(), eventForUser(events.UserEmailBound, u.Id, payload))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.CleanupEmail != "" {
		t.Fatalf("expected no cleanup; got %s", got.CleanupEmail)
	}
}

func TestResolve_UserNotFoundButCleanupNeeded_StillCleansOld(t *testing.T) {
	defer setupTestDB(t)()
	// 用户已被删但仍收到 email.bound 事件（边缘场景）
	payload, _ := common.Marshal(&events.UserEmailBoundPayload{
		UserId:   12345,
		OldEmail: "old@example.com",
		NewEmail: "new@example.com",
	})
	got, err := Resolve(context.Background(), eventForUser(events.UserEmailBound, 12345, payload))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.TargetEmail != "old@example.com" || got.Tier != TierNone {
		t.Fatalf("expected fallback cleanup of old email; got %+v", got)
	}
}

func TestResolve_UserNoEmail_ReturnsNil(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "noemail", Email: "", Group: "vip"}
	createUser(t, u)

	got, err := Resolve(context.Background(), eventForUser(events.UserGroupChanged, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil intent for user without email; got %+v", got)
	}
}

func TestResolve_DisplayNameFallsBackToUsername(t *testing.T) {
	defer setupTestDB(t)()
	u := &model.User{Username: "user42", Email: "u@example.com", Group: "vip", DisplayName: ""}
	createUser(t, u)

	got, err := Resolve(context.Background(), eventForUser(events.UserGroupChanged, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.DisplayName != "user42" {
		t.Fatalf("expected fallback to username; got %s", got.DisplayName)
	}
}

func TestResolve_TopupEventForVIP_AlwaysVIPRegardlessOfPayload(t *testing.T) {
	defer setupTestDB(t)()
	// 验证"状态从 DB 当前态算"：用户已经升 VIP，topup 事件后期才到，应该 resolve 为 TierVIP
	u := &model.User{Username: "u", Email: "u@example.com", Group: "vip"}
	createUser(t, u)
	recordSuccessTopup(t, u.Id, 1000)

	got, err := Resolve(context.Background(), eventForUser(events.BillingTopupSucceeded, u.Id, []byte(`{}`)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Tier != TierVIP {
		t.Fatalf("expected TierVIP (顺序无关); got %v", got.Tier)
	}
}
