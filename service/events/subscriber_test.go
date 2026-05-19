package events

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTopicMatches(t *testing.T) {
	cases := []struct {
		eventType, pattern string
		want               bool
	}{
		{"user.registered", "user.registered", true}, // 精确
		{"user.registered", "user.*", true},          // 前缀通配
		{"user.profile.updated", "user.*", true},     // 多级前缀
		{"billing.topup.succeeded", "billing.topup.*", true},
		{"billing.topup.succeeded", "billing.*", true},
		{"billing.topup.succeeded", "user.*", false},
		{"user.registered", "user.deleted", false},
		{"user.registered", "*", true},        // 全订阅
		{"user", "user.*", false},             // 不带点不算 user 域成员
		{"user.registered", "user", false},    // 非完整匹配
		{"userX.registered", "user.*", false}, // 防止误匹配，需要 "user." 而不是 "user"
	}
	for _, c := range cases {
		got := topicMatches(c.eventType, c.pattern)
		if got != c.want {
			t.Errorf("topicMatches(%q, %q) = %v, want %v", c.eventType, c.pattern, got, c.want)
		}
	}
}

// testSubscriber 是一个可观测的测试订阅者。
type testSubscriber struct {
	name   string
	topics []string
	calls  atomic.Int32
	last   atomic.Value // Event
	mu     sync.Mutex
	err    error
}

func (s *testSubscriber) Name() string     { return s.name }
func (s *testSubscriber) Topics() []string { return s.topics }
func (s *testSubscriber) Handle(_ context.Context, e Event) error {
	s.calls.Add(1)
	s.last.Store(e)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func TestRegisterAndLookup(t *testing.T) {
	resetRegistryForTest()
	a := &testSubscriber{name: "a", topics: []string{"user.*"}}
	b := &testSubscriber{name: "b", topics: []string{"*"}}
	Register(a)
	Register(b)

	got, ok := LookupSubscriber("a")
	if !ok || got.Name() != "a" {
		t.Fatalf("LookupSubscriber a failed")
	}
	if _, ok := LookupSubscriber("missing"); ok {
		t.Fatalf("LookupSubscriber missing should be false")
	}

	subs := SubscribersFor("user.registered")
	if len(subs) != 2 {
		t.Fatalf("user.registered should match both a and b; got %v", subs)
	}
	subs = SubscribersFor("billing.topup.succeeded")
	if len(subs) != 1 || subs[0] != "b" {
		t.Fatalf("billing should match only b; got %v", subs)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	resetRegistryForTest()
	Register(&testSubscriber{name: "dup", topics: []string{"*"}})
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate registration")
		}
	}()
	Register(&testSubscriber{name: "dup", topics: []string{"*"}})
}

func TestRegisterEmptyNamePanics(t *testing.T) {
	resetRegistryForTest()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on empty Name()")
		}
	}()
	Register(&testSubscriber{name: "", topics: []string{"*"}})
}
