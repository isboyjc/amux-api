package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func intPtr(v int) *int { return &v }

func withGlobalDefault(t *testing.T, v int) {
	t.Helper()
	ts := operation_setting.GetTokenSetting()
	orig := ts.DefaultUserMaxConcurrency
	origOld := ts.MaxTokenConcurrency
	ts.DefaultUserMaxConcurrency = v
	ts.MaxTokenConcurrency = 0
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = orig
		ts.MaxTokenConcurrency = origOld
	})
}

// 三态优先级：未配置跟随全局、显式 0 覆盖全局、正值直接生效
func TestResolveUserMaxConcurrency_ThreeStates(t *testing.T) {
	withGlobalDefault(t, 10)

	if got := ResolveUserMaxConcurrency(dto.UserSetting{}); got != 10 {
		t.Fatalf("未单独配置应跟随全局默认 10，实际 %d", got)
	}
	// 显式 0 必须能覆盖全局默认（这正是用指针而非 int 的理由）
	if got := ResolveUserMaxConcurrency(dto.UserSetting{MaxConcurrency: intPtr(0)}); got != 0 {
		t.Fatalf("显式 0 应表示不限制，实际 %d", got)
	}
	if got := ResolveUserMaxConcurrency(dto.UserSetting{MaxConcurrency: intPtr(50)}); got != 50 {
		t.Fatalf("单独配置应直接生效，实际 %d", got)
	}
}

// 全局默认为 0 且用户未配置 → 不限制
func TestResolveUserMaxConcurrency_GlobalZeroMeansUnlimited(t *testing.T) {
	withGlobalDefault(t, 0)
	if got := ResolveUserMaxConcurrency(dto.UserSetting{}); got != 0 {
		t.Fatalf("全局默认 0 且未配置应为不限制，实际 %d", got)
	}
	// 但用户单独配了就应生效，不受全局为 0 影响
	if got := ResolveUserMaxConcurrency(dto.UserSetting{MaxConcurrency: intPtr(3)}); got != 3 {
		t.Fatalf("用户单独配置应生效，实际 %d", got)
	}
}

// 负值按不限制处理，防御脏数据
func TestResolveUserMaxConcurrency_NegativeTreatedAsUnlimited(t *testing.T) {
	withGlobalDefault(t, 10)
	if got := ResolveUserMaxConcurrency(dto.UserSetting{MaxConcurrency: intPtr(-5)}); got != 0 {
		t.Fatalf("负值应按不限制处理，实际 %d", got)
	}
}

// 读取端不做旧键回落：迁移由 model.migrateLegacyConcurrencyOption 在启动时落库完成。
// 若这里做回落，管理面板会显示新键原始值(0)而限流按旧值在跑，显示与行为不一致。
func TestGetDefaultUserMaxConcurrency_NoReadTimeFallback(t *testing.T) {
	ts := operation_setting.GetTokenSetting()
	origNew, origOld := ts.DefaultUserMaxConcurrency, ts.MaxTokenConcurrency
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = origNew
		ts.MaxTokenConcurrency = origOld
	})

	ts.DefaultUserMaxConcurrency = 0
	ts.MaxTokenConcurrency = 77
	if got := operation_setting.GetDefaultUserMaxConcurrency(); got != 0 {
		t.Fatalf("不应在读取期回落到旧键，期望 0，实际 %d", got)
	}
	ts.DefaultUserMaxConcurrency = 20
	if got := operation_setting.GetDefaultUserMaxConcurrency(); got != 20 {
		t.Fatalf("新键应生效，实际 %d", got)
	}
}

// 令牌级夹紧：超过账户级时收紧，未超则原样保留
func TestClampTokenMaxConcurrency(t *testing.T) {
	cases := []struct{ token, user, want int }{
		{100, 10, 10}, // 超过账户级 → 夹紧
		{5, 10, 5},    // 未超 → 保留
		{10, 10, 10},  // 相等 → 保留
		{50, 0, 50},   // 账户级不限制 → 令牌级原样保留
		{0, 10, 0},    // 令牌级不限制 → 保持不限制
		{-3, 10, 0},   // 负值 → 归零
	}
	for _, c := range cases {
		if got := ClampTokenMaxConcurrency(c.token, c.user); got != c.want {
			t.Errorf("ClampTokenMaxConcurrency(%d, %d) = %d，期望 %d", c.token, c.user, got, c.want)
		}
	}
}
