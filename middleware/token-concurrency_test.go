package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// 构造一条带并发限制的测试路由。handler 由调用方决定，用来模拟 panic / 正常返回。
func newConcurrencyRouter(tokenId, limit int, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 模拟 main.go 的 CustomRecovery：它注册在中间件链外层
	r.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	r.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "test-req-"+c.Query("rid"))
		c.Set("id", 4242) // userId
		common.SetContextKey(c, constant.ContextKeyTokenId, tokenId)
		common.SetContextKey(c, constant.ContextKeyTokenMaxConcurrency, limit)
		// 账户级不单独配置（nil），跟随全局默认
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{})
		c.Next()
	})
	r.Use(TokenConcurrencyLimit())
	r.GET("/t", handler)
	return r
}

// 无 Redis 场景（common.RedisEnabled=false）走进程内计数。
// handler panic 时槽位必须被释放，否则令牌会被永久占满。
func TestTokenConcurrency_SlotReleasedOnPanic(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	// 禁用异步落库：它的 goroutine 会读 common.RedisEnabled，与下面 defer 的恢复
	// 构成数据竞争（-race）
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	origCap := operation_setting.GetTokenSetting().DefaultUserMaxConcurrency
	operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = 10
	defer func() { operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = origCap }()

	r := newConcurrencyRouter(9001, 1, func(c *gin.Context) {
		panic("boom")
	})

	// 连续两次请求：如果第一次 panic 后没释放槽位，第二次会拿到 429 而不是 500
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t?rid=a", nil)
		r.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("第 %d 次请求拿到 429，说明 panic 后槽位没释放", i+1)
		}
	}
}

// 管理员把上限调成 0（关闭功能）后，存量令牌上的残留值不应继续拦请求
func TestTokenConcurrency_DisabledWhenAdminCapZero(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	// 禁用异步落库：它的 goroutine 会读 common.RedisEnabled，与下面 defer 的恢复
	// 构成数据竞争（-race）
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	origCap := operation_setting.GetTokenSetting().DefaultUserMaxConcurrency
	operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = 0
	defer func() { operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = origCap }()

	// 令牌上残留 limit=1，但功能已关闭 → 不应限流
	r := newConcurrencyRouter(9002, 1, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=b", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("功能关闭时不应限流，实际状态码 %d", w.Code)
		}
	}
}

// limit<=0（绝大多数令牌）直接放行
func TestTokenConcurrency_NoLimitPassesThrough(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	// 禁用异步落库：它的 goroutine 会读 common.RedisEnabled，与下面 defer 的恢复
	// 构成数据竞争（-race）
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	r := newConcurrencyRouter(9003, 0, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=c", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("未设并发的令牌应直接放行，实际 %d", w.Code)
	}
}

// 超限时返回 429 且带 Retry-After
func TestTokenConcurrency_Returns429WithRetryAfter(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	// 禁用异步落库：它的 goroutine 会读 common.RedisEnabled，与下面 defer 的恢复
	// 构成数据竞争（-race）
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	origCap := operation_setting.GetTokenSetting().DefaultUserMaxConcurrency
	operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = 10
	defer func() { operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = origCap }()

	blocked := make(chan struct{})
	released := make(chan struct{})
	r := newConcurrencyRouter(9004, 1, func(c *gin.Context) {
		close(blocked)
		<-released // 占住槽位不放
		c.Status(http.StatusOK)
	})

	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=d1", nil))
	}()
	<-blocked // 确保第一个请求已占住槽位

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=d2", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("超限应返回 429，实际 %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("应带 Retry-After: 1，实际 %q", got)
	}
	close(released)
}

// ── 在途计数（观测）─────────────────────────────────────────────────────
//
// 这套计数与限流是两条独立路径，测试要点是「限流不生效的情况下计数照样发生」，
// 因为这正是管理页要展示所有令牌在途数的前提。

// 未配任何并发上限（限流走快速退出）时，在途数仍必须被统计到。
// 这条是整个观测方案的核心前提，回归了就等于该列对大多数令牌恒显示 0。
func TestInflight_CountedEvenWhenNoLimitConfigured(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	// 账户级与令牌级都为 0 → 限流中间件走快速退出
	origCap := operation_setting.GetTokenSetting().DefaultUserMaxConcurrency
	operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = 0
	defer func() { operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = origCap }()

	const tokenId = 9101
	observed := make(chan int, 1)
	blocked := make(chan struct{})
	released := make(chan struct{})
	r := newConcurrencyRouter(tokenId, 0, func(c *gin.Context) {
		observed <- limiter.SnapshotInflight()[tokenId]
		close(blocked)
		<-released
		c.Status(http.StatusOK)
	})

	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=i1", nil))
	}()
	<-blocked

	if got := <-observed; got != 1 {
		t.Fatalf("未配上限的令牌在途数应为 1，实际 %d —— 观测计数不能跟着限流一起被跳过", got)
	}

	close(released)
}

// 请求结束后在途数必须归零（正常返回路径）。
func TestInflight_ReleasedAfterRequest(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	const tokenId = 9102
	r := newConcurrencyRouter(tokenId, 0, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=i2", nil))

	if got := limiter.SnapshotInflight()[tokenId]; got != 0 {
		t.Fatalf("请求结束后在途数应归零，实际 %d", got)
	}
}

// handler panic 时在途数也必须归零 —— 否则一次 panic 就让该令牌的展示值永久偏高。
func TestInflight_ReleasedOnPanic(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	const tokenId = 9103
	r := newConcurrencyRouter(tokenId, 0, func(c *gin.Context) {
		panic("boom")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=i3", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic 应被 recovery 兜成 500，实际 %d", w.Code)
	}

	if got := limiter.SnapshotInflight()[tokenId]; got != 0 {
		t.Fatalf("panic 后在途数应归零，实际 %d", got)
	}
}

// 被限流拒绝（429）的请求不应留下在途计数：它没有真的在跑。
func TestInflight_ReleasedOnRejectedRequest(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	origCap := operation_setting.GetTokenSetting().DefaultUserMaxConcurrency
	operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = 10
	defer func() { operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = origCap }()

	const tokenId = 9104
	blocked := make(chan struct{})
	released := make(chan struct{})
	r := newConcurrencyRouter(tokenId, 1, func(c *gin.Context) {
		close(blocked)
		<-released
		c.Status(http.StatusOK)
	})

	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=i4a", nil))
	}()
	<-blocked

	// 第二个请求会被 429 拒掉
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=i4b", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("应被限流拒绝，实际 %d", w.Code)
	}
	// 此刻只有第一个请求真的在跑
	if got := limiter.SnapshotInflight()[tokenId]; got != 1 {
		t.Fatalf("被拒的请求不应计入在途，期望 1，实际 %d", got)
	}

	close(released)
}

// 被限流拒绝的请求，在**处理过程中**（而不只是结束后）也绝不能计入在途数。
//
// 这是 TestInflight_ReleasedOnRejectedRequest 漏掉的场景：那个用例只在请求
// 返回后检查最终态，而早期实现是「进中间件就 Inc、走到拒绝分支再靠 defer Dec」，
// 最终态同样是干净的，所以它验不出中途的虚高。
//
// 采样点必须在被拒请求**仍在中间件内部**时：放在 c.Next() 之后是无效的（那时
// 中间件已返回，defer Dec 早已执行，旧实现看起来也是干净的）。这里挂在
// inflightRejectProbe 上 —— 它在 abortWithConcurrencyExceeded 内部、紧挨着
// 429 写出的位置被调用。
func TestInflight_RejectedRequestNeverCountedDuringHandling(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	concurrencyLogAsyncDisabled = true
	defer func() {
		common.RedisEnabled = origRedis
		concurrencyLogAsyncDisabled = false
	}()

	origCap := operation_setting.GetTokenSetting().DefaultUserMaxConcurrency
	operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = 10
	defer func() { operation_setting.GetTokenSetting().DefaultUserMaxConcurrency = origCap }()

	const tokenId = 9105
	sampled := make(chan int, 4)
	inflightRejectProbe = func() {
		sampled <- limiter.SnapshotInflight()[tokenId]
	}
	defer func() { inflightRejectProbe = nil }()

	blocked := make(chan struct{})
	released := make(chan struct{})
	r := newConcurrencyRouter(tokenId, 1, func(c *gin.Context) {
		close(blocked)
		<-released
		c.Status(http.StatusOK)
	})

	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=j1", nil))
	}()
	<-blocked // 第一个请求已占住唯一槽位

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t?rid=j2", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("应被限流拒绝，实际 %d", w.Code)
	}

	select {
	case during := <-sampled:
		if during != 1 {
			t.Fatalf("被拒请求处理过程中在途数应仍为 1（只有真正在跑的那个），实际 %d "+
				"—— 被 429 拒掉的请求不该计入在途", during)
		}
	default:
		t.Fatal("拒绝路径未触发采样钩子")
	}

	close(released)
}
