package marketing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/events"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// fullMockProvider 实现完整的 Provider interface 用于测试 Sync 的 RemovalMode 分支
// 和 Subscriptions API 的端到端调用。
type fullMockProvider struct {
	mockProvider // 继承 backfill_test.go 里的 mockProvider（提供 Sync 计数）

	// 新接口的调用记录
	listTopicsCalls   atomic.Int32
	getSubsCalls      atomic.Int32
	updateSubsCalls   atomic.Int32
	hardDeleteCalls   atomic.Int32
	softUnsubCalls    atomic.Int32

	mu             sync.Mutex
	lastUpdateSubs Subscriptions

	availableTopics []Topic
	currentSubs     *Subscriptions
}

func (m *fullMockProvider) Sync(_ context.Context, intent Intent) error {
	m.syncCount.Add(1)
	m.mu.Lock()
	m.calls = append(m.calls, intent)
	m.mu.Unlock()
	if intent.Tier == TierNone {
		if intent.RemovalMode == RemovalHardDelete {
			m.hardDeleteCalls.Add(1)
		} else {
			m.softUnsubCalls.Add(1)
		}
	}
	if e, ok := m.failFor[intent.TargetEmail]; ok {
		return e
	}
	return nil
}

func (m *fullMockProvider) ListTopics(_ context.Context, _ []string) ([]Topic, error) {
	m.listTopicsCalls.Add(1)
	return m.availableTopics, nil
}

func (m *fullMockProvider) GetSubscriptions(_ context.Context, _ string) (*Subscriptions, error) {
	m.getSubsCalls.Add(1)
	if m.currentSubs != nil {
		return m.currentSubs, nil
	}
	return &Subscriptions{}, nil
}

func (m *fullMockProvider) UpdateSubscriptions(_ context.Context, _ string, _ string, subs Subscriptions) error {
	m.updateSubsCalls.Add(1)
	m.mu.Lock()
	m.lastUpdateSubs = subs
	m.mu.Unlock()
	return nil
}

func TestResolve_UserDeletedSetsHardDelete(t *testing.T) {
	defer setupTestDB(t)()

	payload, _ := common.Marshal(&events.UserDeletedPayload{
		UserId:   42,
		Email:    "gone@example.com",
		Username: "gone",
	})
	got, err := Resolve(context.Background(), events.Event{
		Type:        events.UserDeleted,
		AggregateId: 42,
		Payload:     payload,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil intent for user.deleted")
	}
	if got.Tier != TierNone {
		t.Fatalf("expected TierNone, got %v", got.Tier)
	}
	if got.RemovalMode != RemovalHardDelete {
		t.Fatalf("user.deleted should set RemovalHardDelete, got %v", got.RemovalMode)
	}
}

func TestResolve_NonDeletedKeepsDefaultSoftRemoval(t *testing.T) {
	defer setupTestDB(t)()
	// 企业组用户 → TierNone，但应该是软退订（默认零值）
	u := &model.User{Username: "ent", Email: "ent@example.com", Group: "enterprise_a"}
	createUser(t, u)

	got, err := Resolve(context.Background(), events.Event{
		Type:        events.UserGroupChanged,
		AggregateId: u.Id,
		Payload:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Tier != TierNone {
		t.Fatalf("expected TierNone for enterprise user, got %v", got.Tier)
	}
	if got.RemovalMode != RemovalSoftUnsubscribe {
		t.Fatalf("non-delete TierNone should default to soft unsubscribe (0), got %v", got.RemovalMode)
	}
}

func TestIsEligible(t *testing.T) {
	defer setupTestDB(t)()

	vip := &model.User{Username: "vip", Email: "vip@example.com", Group: "vip"}
	paid := &model.User{Username: "paid", Email: "paid@example.com", Group: "default"}
	free := &model.User{Username: "free", Email: "free@example.com", Group: "default"}
	ent := &model.User{Username: "ent", Email: "ent@example.com", Group: "enterprise_a"}
	noEmail := &model.User{Username: "noemail", Email: "", Group: "vip"}
	for _, u := range []*model.User{vip, paid, free, ent, noEmail} {
		createUser(t, u)
	}
	recordSuccessTopup(t, paid.Id, 10)
	recordSuccessTopup(t, ent.Id, 9999)

	cases := []struct {
		name string
		u    *model.User
		want bool
	}{
		{"vip eligible", vip, true},
		{"paid default eligible", paid, true},
		{"free default not eligible", free, false},
		{"enterprise not eligible despite topup", ent, false},
		{"no email not eligible", noEmail, false},
		{"nil not eligible", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsEligible(tc.u)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestIsEligible_ExtraGroups 验证 MarketingExtraEligibleGroups 配置生效。
func TestIsEligible_ExtraGroups(t *testing.T) {
	defer setupTestDB(t)()

	entA := &model.User{Username: "entA", Email: "a@example.com", Group: "enterprise_a"}
	entB := &model.User{Username: "entB", Email: "b@example.com", Group: "enterprise_b"}
	entC := &model.User{Username: "entC", Email: "c@example.com", Group: "enterprise_c"}
	free := &model.User{Username: "free", Email: "free@example.com", Group: "default"}
	for _, u := range []*model.User{entA, entB, entC, free} {
		createUser(t, u)
	}

	// 保存原值，测试后恢复，避免污染其他测试
	orig := operation_setting.MarketingExtraEligibleGroups
	defer func() { operation_setting.MarketingExtraEligibleGroups = orig }()

	// 配置 enterprise_a 和 enterprise_b 为额外允许的组（含前后空白以测 Trim）
	operation_setting.MarketingExtraEligibleGroups = " enterprise_a , enterprise_b "

	cases := []struct {
		name string
		u    *model.User
		want bool
	}{
		{"enterprise_a (in list)", entA, true},
		{"enterprise_b (in list)", entB, true},
		{"enterprise_c (not in list)", entC, false},
		{"free default still not eligible", free, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsEligible(tc.u)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}

	// 空配置 → 不影响
	operation_setting.MarketingExtraEligibleGroups = ""
	got, _ := IsEligible(entA)
	if got {
		t.Fatal("empty config should not grant eligibility")
	}
}

// TestProvider_Sync_HardDeleteVsSoftUnsubscribe 验证 Sync 按 RemovalMode 分流。
// 这里直接调 fullMockProvider 的 Sync（不走 Resend client），重点是 Sync 决策路径。
func TestProvider_RemovalMode_Counter(t *testing.T) {
	p := &fullMockProvider{}
	ctx := context.Background()

	// 默认 RemovalMode 零值 → 软退订
	_ = p.Sync(ctx, Intent{TargetEmail: "x@example.com", Tier: TierNone})
	if p.softUnsubCalls.Load() != 1 || p.hardDeleteCalls.Load() != 0 {
		t.Fatalf("expected soft=1 hard=0, got soft=%d hard=%d",
			p.softUnsubCalls.Load(), p.hardDeleteCalls.Load())
	}

	// 显式 RemovalHardDelete
	_ = p.Sync(ctx, Intent{TargetEmail: "y@example.com", Tier: TierNone, RemovalMode: RemovalHardDelete})
	if p.softUnsubCalls.Load() != 1 || p.hardDeleteCalls.Load() != 1 {
		t.Fatalf("expected soft=1 hard=1, got soft=%d hard=%d",
			p.softUnsubCalls.Load(), p.hardDeleteCalls.Load())
	}

	// TierVIP 不进入移除分支
	_ = p.Sync(ctx, Intent{TargetEmail: "z@example.com", Tier: TierVIP})
	if p.softUnsubCalls.Load() != 1 || p.hardDeleteCalls.Load() != 1 {
		t.Fatalf("VIP sync should not increment removal counters")
	}
}

func TestProvider_SubscriptionsRoundtrip(t *testing.T) {
	p := &fullMockProvider{
		availableTopics: []Topic{
			{ID: "t1", Name: "Updates"},
			{ID: "t2", Name: "Promotions"},
		},
		currentSubs: &Subscriptions{
			GlobalUnsubscribed: false,
			Topics: []TopicSubscription{
				{TopicID: "t1", Subscribed: true},
				{TopicID: "t2", Subscribed: false},
			},
		},
	}
	ctx := context.Background()

	topics, err := p.ListTopics(ctx, []string{"t1", "t2"})
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	if len(topics) != 2 || topics[0].ID != "t1" {
		t.Fatalf("unexpected topics: %+v", topics)
	}

	subs, err := p.GetSubscriptions(ctx, "u@example.com")
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	if subs.GlobalUnsubscribed {
		t.Fatal("expected globally subscribed")
	}
	if len(subs.Topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(subs.Topics))
	}

	newSubs := Subscriptions{
		GlobalUnsubscribed: true,
		Topics: []TopicSubscription{
			{TopicID: "t1", Subscribed: false},
			{TopicID: "t2", Subscribed: true},
		},
	}
	if err := p.UpdateSubscriptions(ctx, "u@example.com", "Display Name", newSubs); err != nil {
		t.Fatalf("UpdateSubscriptions: %v", err)
	}
	if p.updateSubsCalls.Load() != 1 {
		t.Fatal("expected update call recorded")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.lastUpdateSubs.GlobalUnsubscribed {
		t.Fatal("UpdateSubscriptions did not see GlobalUnsubscribed=true")
	}
}

// 确保 backfill_test.go 里的简化 mockProvider 仍能满足新 interface。
// 如果接口扩展破坏了 backfill_test.go，下面这个 var 声明会编译失败。
var _ Provider = (*fullMockProvider)(nil)

// 让 backfill_test.go 的 mockProvider 也通过编译期检查（它没实现新方法 → 编译会失败）。
// 修法：给 mockProvider 加 stub 方法
func (m *mockProvider) ListTopics(_ context.Context, _ []string) ([]Topic, error) {
	return nil, nil
}
func (m *mockProvider) GetSubscriptions(_ context.Context, _ string) (*Subscriptions, error) {
	return &Subscriptions{}, nil
}
func (m *mockProvider) UpdateSubscriptions(_ context.Context, _ string, _ string, _ Subscriptions) error {
	return nil
}

var _ Provider = (*mockProvider)(nil)

// 引用 errors 防止"imported and not used"
var _ = errors.New