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

// 上线安全性质：全新部署、不改任何配置时，并发中间件必须对所有请求完全放行。
// 这是本次改动「不影响存量用户」的核心保证，任何人改坏它这个测试都会失败。
func TestUpgradeSafety_DefaultConfigNeverLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 全新部署的默认状态：全局默认 0、用户无单独配置、令牌无配置
	ts := operation_setting.GetTokenSetting()
	origDefault, origLegacy := ts.DefaultUserMaxConcurrency, ts.MaxTokenConcurrency
	ts.DefaultUserMaxConcurrency = 0
	ts.MaxTokenConcurrency = 0
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = origDefault
		ts.MaxTokenConcurrency = origLegacy
	})

	// 不触碰 common.RedisEnabled：其它用例触发的 logConcurrencyReject 异步 goroutine
	// 会在 gopool 里读这个全局变量，测试里改它会被 -race 判定为数据竞争。
	// 本用例的性质（默认配置零限流）由快速退出保证，那条路径在读 RedisEnabled
	// 之前就 return 了，所以两种缓存模式的行为必然相同，无需分别验证。
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "safety")
		c.Set("id", 1)
		common.SetContextKey(c, constant.ContextKeyTokenId, 99)
		// 存量令牌：max_concurrency 列刚加，值为 0
		common.SetContextKey(c, constant.ContextKeyTokenMaxConcurrency, 0)
		// 存量用户：setting 里没有 max_concurrency
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{})
		c.Next()
	})
	r.Use(TokenConcurrencyLimit())
	r.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 连打 50 个，一个都不该被拦
	for i := 0; i < 50; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 个请求被拦截（状态码 %d）——默认配置绝不该限流", i+1, w.Code)
		}
	}
}
