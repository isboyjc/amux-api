package resend

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/service/events"

	rd "github.com/resend/resend-go/v3"
	"golang.org/x/time/rate"
)

func TestClassifyErr_Nil(t *testing.T) {
	if err := classifyErr(nil, true); err != nil {
		t.Fatalf("nil should be nil, got %v", err)
	}
}

func TestClassifyErr_RateLimitIsTransient(t *testing.T) {
	rateErr := &rd.RateLimitError{Message: "too many"}
	got := classifyErr(rateErr, false)
	if got == nil {
		t.Fatal("rate limit should not be nil")
	}
	if errors.Is(got, events.ErrPermanent) {
		t.Fatal("rate limit should not be permanent")
	}
}

func TestClassifyErr_NotFoundIgnored(t *testing.T) {
	cases := []string{
		"[ERROR]: Contact not found",
		"[ERROR]: 404 not found",
		"[ERROR]: Not Found",
	}
	for _, msg := range cases {
		if err := classifyErr(errors.New(msg), true); err != nil {
			t.Errorf("with ignoreNotFound=true, %q should be nil; got %v", msg, err)
		}
	}
}

func TestClassifyErr_NotFoundNotIgnored(t *testing.T) {
	err := errors.New("[ERROR]: Contact not found")
	got := classifyErr(err, false)
	if got == nil {
		t.Fatal("with ignoreNotFound=false, should pass through err")
	}
}

func TestClassifyErr_AuthErrorsArePermanent(t *testing.T) {
	cases := []string{
		"[ERROR]: Invalid API key",
		"[ERROR]: Unauthorized",
		"[ERROR]: 401 Unauthorized",
		"[ERROR]: 403 Forbidden",
		"[ERROR]: Forbidden",
	}
	for _, msg := range cases {
		got := classifyErr(errors.New(msg), false)
		if !errors.Is(got, events.ErrPermanent) {
			t.Errorf("%q should be ErrPermanent; got %v", msg, got)
		}
	}
}

func TestClassifyErr_UnknownErrorIsTransient(t *testing.T) {
	// 5xx / 网络错 / 其他未识别错 → 返回原 err，让 worker 重试
	err := errors.New("[ERROR]: Internal server error")
	got := classifyErr(err, false)
	if got == nil {
		t.Fatal("should not be nil")
	}
	if errors.Is(got, events.ErrPermanent) {
		t.Fatal("unknown server error should be transient (worker can retry)")
	}
}

func TestErrIsAlreadyExists(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"[ERROR]: Contact already exists", true},
		{"[ERROR]: contact_already_exists", true},
		{"[ERROR]: Email already_exists", true},
		{"[ERROR]: not found", false},
		{"", false},
	}
	for _, c := range cases {
		var err error
		if c.msg != "" {
			err = errors.New(c.msg)
		}
		if errIsAlreadyExists(err) != c.want {
			t.Errorf("errIsAlreadyExists(%q) = %v, want %v", c.msg, !c.want, c.want)
		}
	}
}

func TestDo_RetriesOnRateLimit(t *testing.T) {
	// 高速率限制 + 大 burst → 限流不会拖慢测试，专心验证重试逻辑
	c := &client{limiter: rate.NewLimiter(rate.Limit(1000), 100)}

	var calls atomic.Int32
	err := c.do(context.Background(), func() error {
		n := calls.Add(1)
		if n < 3 {
			return &rd.RateLimitError{
				Message:    "too many",
				RetryAfter: "0", // 立即（实际会 sleep ~ parseRetryAfter 兜底的 1s + 抖动）
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 attempts (2 fail + 1 success), got %d", got)
	}
}

func TestDo_NonRateLimitErrorReturnsImmediately(t *testing.T) {
	c := &client{limiter: rate.NewLimiter(rate.Limit(1000), 100)}

	var calls atomic.Int32
	sentinel := errors.New("boom")
	err := c.do(context.Background(), func() error {
		calls.Add(1)
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("non-429 should not retry, got %d calls", got)
	}
}

func TestDo_GivesUpAfterMaxRetries(t *testing.T) {
	c := &client{limiter: rate.NewLimiter(rate.Limit(1000), 100)}

	var calls atomic.Int32
	rateErr := &rd.RateLimitError{Message: "persistent", RetryAfter: "0"}
	err := c.do(context.Background(), func() error {
		calls.Add(1)
		return rateErr
	})

	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	var rle *rd.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	// 给出比较宽容的下界：requireRetryAttempt 在 maxTotalSleep 用满前应至少试 2 次
	// 真实上限 = min(maxRetryOn429+1, 到 maxTotalSleep 为止的次数)
	if got := calls.Load(); got < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", got)
	}
	if got := calls.Load(); got > int32(maxRetryOn429+1) {
		t.Fatalf("expected at most %d attempts, got %d", maxRetryOn429+1, got)
	}
}

// TestDo_RespectsTotalSleepCap 验证累计 sleep 超过 maxTotalSleep 时即使没用完
// maxRetryOn429 也会放弃（保护 SyncTimeout 不被打穿）。
func TestDo_RespectsTotalSleepCap(t *testing.T) {
	c := &client{limiter: rate.NewLimiter(rate.Limit(1000), 100)}

	var calls atomic.Int32
	// retry-after = 15s → 第一次重试就累计 ~15s，第二次就破 20s 上限退出
	rateErr := &rd.RateLimitError{Message: "long wait", RetryAfter: "15"}
	start := time.Now()
	err := c.do(context.Background(), func() error {
		calls.Add(1)
		return rateErr
	})
	dur := time.Since(start)

	if err == nil {
		t.Fatal("expected give-up error")
	}
	// 应该只睡过一次 ~15s，第二次发现破上限就退出，不会真睡 30s+
	if dur > 18*time.Second {
		t.Fatalf("total sleep should respect %s cap, took %s", maxTotalSleep, dur)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("expected at least 2 attempts before give up, got %d", got)
	}
}

func TestDo_RespectsCtxCancel(t *testing.T) {
	c := &client{limiter: rate.NewLimiter(rate.Limit(1000), 100)}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var calls atomic.Int32
	err := c.do(ctx, func() error {
		calls.Add(1)
		return &rd.RateLimitError{Message: "rl", RetryAfter: "5"} // 想 sleep 5s
	})

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected ctx error, got %v", err)
	}
	// 至少调用了 1 次（首次没受 ctx 影响）
	if calls.Load() < 1 {
		t.Fatal("expected at least one call before ctx cancel")
	}
}

// TestDo_DetectsWrappedRateLimitError 防御性测试：即使 SDK 未来给 *RateLimitError
// 加一层 fmt.Errorf("...: %w") 包装，errors.As 仍应能识别并触发重试。
// 如果有一天 SDK 改成 fmt.Errorf("...: %v")（非 %w），这个测试会失败 —— 提示需要
// 在 do() 里加 strings.Contains 兜底。
func TestDo_DetectsWrappedRateLimitError(t *testing.T) {
	c := &client{limiter: rate.NewLimiter(rate.Limit(1000), 100)}

	var calls atomic.Int32
	err := c.do(context.Background(), func() error {
		n := calls.Add(1)
		if n < 2 {
			// 模拟 SDK 包了一层
			return fmt.Errorf("upstream: %w", &rd.RateLimitError{
				Message: "wrapped", RetryAfter: "0",
			})
		}
		return nil
	})

	if err != nil {
		t.Fatalf("wrapped rate-limit error should be retried, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 attempts (1 retry on wrapped 429), got %d", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", time.Second},
		{"0", time.Second},
		{"-1", time.Second},
		{"abc", time.Second},
		{"1", 1 * time.Second},
		{"  3  ", 3 * time.Second},
		{"10", 10 * time.Second},
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.in); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSplitName(t *testing.T) {
	cases := []struct {
		in            string
		first, last   string
	}{
		{"", "", ""},
		{"Alice", "Alice", ""},
		{"Alice Smith", "Alice", "Smith"},
		{"Alice Van Der Berg", "Alice", "Van Der Berg"},
		{"   Bob   ", "Bob", ""},
		{"  Bob  Smith  ", "Bob", "Smith"},
	}
	for _, c := range cases {
		first, last := splitName(c.in)
		if first != c.first || last != c.last {
			t.Errorf("splitName(%q) = (%q, %q), want (%q, %q)", c.in, first, last, c.first, c.last)
		}
	}
}
