package marketing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

// mockProvider 用来计数 Sync 调用 + 可选地按邮箱注入错误。
type mockProvider struct {
	syncCount atomic.Int32
	mu        sync.Mutex
	calls     []Intent
	failFor   map[string]error // email → 注入的错误
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Sync(_ context.Context, intent Intent) error {
	m.syncCount.Add(1)
	m.mu.Lock()
	m.calls = append(m.calls, intent)
	m.mu.Unlock()
	if e, ok := m.failFor[intent.TargetEmail]; ok {
		return e
	}
	return nil
}

func TestBackfill_RefusesWhenProviderNil(t *testing.T) {
	SetProvider(nil)
	defer SetProvider(nil)

	_, err := Backfill(context.Background(), BackfillOpts{})
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
	}
}

func TestBackfill_RefusesWhenAlreadyRunning(t *testing.T) {
	defer setupTestDB(t)()
	mp := &mockProvider{}
	SetProvider(mp)
	defer SetProvider(nil)

	// 人为占住运行标志
	if !backfillRunning.CompareAndSwap(false, true) {
		t.Fatal("backfillRunning already true at test start")
	}
	defer backfillRunning.Store(false)

	_, err := Backfill(context.Background(), BackfillOpts{})
	if !errors.Is(err, ErrBackfillRunning) {
		t.Fatalf("expected ErrBackfillRunning, got %v", err)
	}
}

func TestBackfill_SyncsVIPAndPaidDefault_SkipsFreeAndEnterprise(t *testing.T) {
	defer setupTestDB(t)()
	mp := &mockProvider{}
	SetProvider(mp)
	defer SetProvider(nil)

	// 4 个用户：
	//   VIP（不查充值，应同步）
	//   default + 充过钱（应同步）
	//   default + 没充过钱（应跳过，TierNone）
	//   enterprise 组（应跳过，TierNone）
	vip := &model.User{Username: "vip", Email: "vip@example.com", Group: "vip", DisplayName: "VIP"}
	paid := &model.User{Username: "paid", Email: "paid@example.com", Group: "default", DisplayName: "Paid"}
	free := &model.User{Username: "free", Email: "free@example.com", Group: "default", DisplayName: "Free"}
	ent := &model.User{Username: "ent", Email: "ent@example.com", Group: "enterprise_a"}
	for _, u := range []*model.User{vip, paid, free, ent} {
		createUser(t, u)
	}
	recordSuccessTopup(t, paid.Id, 100)
	// 注意：ent 也充过钱，但因为不是 default/vip 组 → 应跳过
	recordSuccessTopup(t, ent.Id, 999)

	res, err := Backfill(context.Background(), BackfillOpts{Concurrency: 2, BatchSize: 10})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// 候选 = vip + paid（GetPaidUserIDsBatch 的查询条件）
	// ent 因为不是 default/vip 不会被查出来；free 也不会
	if res.Total != 2 {
		t.Fatalf("Total=2 expected, got %d", res.Total)
	}
	if res.Synced != 2 {
		t.Fatalf("Synced=2 expected, got %d (failed=%d skipped=%d)", res.Synced, res.Failed, res.Skipped)
	}
	if mp.syncCount.Load() != 2 {
		t.Fatalf("Sync called %d times, expected 2", mp.syncCount.Load())
	}

	// 验证两次 Sync 都带了正确 Tier
	tierCount := map[Tier]int{}
	for _, c := range mp.calls {
		tierCount[c.Tier]++
	}
	if tierCount[TierVIP] != 1 || tierCount[TierDefault] != 1 {
		t.Fatalf("expected 1 VIP + 1 Default sync, got %v", tierCount)
	}
}

func TestBackfill_CountsFailures(t *testing.T) {
	defer setupTestDB(t)()
	mp := &mockProvider{
		failFor: map[string]error{
			"bad@example.com": errors.New("simulated resend 500"),
		},
	}
	SetProvider(mp)
	defer SetProvider(nil)

	good := &model.User{Username: "good", Email: "good@example.com", Group: "vip"}
	bad := &model.User{Username: "bad", Email: "bad@example.com", Group: "vip"}
	createUser(t, good)
	createUser(t, bad)

	res, err := Backfill(context.Background(), BackfillOpts{Concurrency: 1})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.Total != 2 || res.Synced != 1 || res.Failed != 1 {
		t.Fatalf("expected total=2 synced=1 failed=1, got %+v", res)
	}
	if res.LastError == "" {
		t.Fatal("expected LastError to be set")
	}
}

func TestBackfill_FinishedResultStored(t *testing.T) {
	defer setupTestDB(t)()
	mp := &mockProvider{}
	SetProvider(mp)
	defer SetProvider(nil)

	createUser(t, &model.User{Username: "u", Email: "u@example.com", Group: "vip"})

	_, err := Backfill(context.Background(), BackfillOpts{})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	last := LastBackfillResult()
	if last == nil {
		t.Fatal("LastBackfillResult is nil after Backfill")
	}
	if last.FinishedAt == 0 {
		t.Fatal("FinishedAt should be set on completion")
	}
	if last.Synced != 1 {
		t.Fatalf("LastBackfillResult.Synced=1 expected, got %d", last.Synced)
	}
}

func TestBackfill_SkipsUsersWithoutEmail(t *testing.T) {
	defer setupTestDB(t)()
	mp := &mockProvider{}
	SetProvider(mp)
	defer SetProvider(nil)

	// 没邮箱的 VIP 用户应该被 GetPaidUserIDsBatch 排除（WHERE email <> ''）
	createUser(t, &model.User{Username: "noemail", Email: "", Group: "vip"})

	res, err := Backfill(context.Background(), BackfillOpts{})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.Total != 0 {
		t.Fatalf("expected 0 candidates (no email user excluded by query), got %d", res.Total)
	}
	if mp.syncCount.Load() != 0 {
		t.Fatalf("Sync should not be called, got %d times", mp.syncCount.Load())
	}
}
