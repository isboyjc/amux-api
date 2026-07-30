package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
