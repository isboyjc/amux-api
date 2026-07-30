package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// tokenConcurrencyTTLSeconds 槽位最长存活时间。
//
// 取值权衡：太长则进程被强杀（SIGKILL/OOM，defer 跑不到）泄漏的槽位要很久才回收，
// 期间用户被误占；太短则正常的长请求会被误判成僵尸槽位提前回收，导致实际并发
// 超过设定值。取 STREAMING_TIMEOUT(默认 300s) 的 2 倍，正常流式请求碰不到，
// 崩溃泄漏最多 10 分钟自愈。两级共用同一 TTL。
const tokenConcurrencyTTLSeconds = 600

// TokenConcurrencyLimit 并发限制中间件（账户级 + 令牌级）。
//
// 与 ModelRequestRateLimit 的区别：那个限制「周期内最多请求 N 次」（频率），
// 这个限制「同一时刻最多 N 个请求在途」（并发）。超限直接 429，不排队 —— 排队要在
// 网关侧挂住大量 goroutine，且与上游超时相互干扰，复杂度和风险都高一个量级。
//
// 层级语义：
//   - 账户级：该用户名下所有令牌的在途请求总数
//   - 令牌级：单个令牌的在途请求数，静默夹紧到账户级实际值
//
// 只应挂在「提交类」请求上。任务查询类端点（RelayTaskFetch）绝对不能占槽位，
// 否则用户轮询任务状态会把自己的并发打满。
//
// 性能：这个中间件在每个中继请求上都会执行，所以未配置任何并发限制时必须是
// 零成本的 —— 见下面的快速退出。配置了的情况也只有 2 次 Redis 往返
// （acquire 1 次 + release 1 次），与只配一级时相同。
func TokenConcurrencyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── 快速退出：绝大多数请求走这条路径 ──────────────────────────
		// 两次 map 查找 + 一次类型断言，无 Redis、无锁、无内存分配。
		// UserSetting 在 middleware/auth.go 的 userCache.WriteContext(c) 里
		// 就已经是解析好的结构体，这里读它不产生 JSON 解析开销。
		tokenLimit := common.GetContextKeyInt(c, constant.ContextKeyTokenMaxConcurrency)
		userSetting, _ := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
		userLimit := service.ResolveUserMaxConcurrency(userSetting)
		if tokenLimit <= 0 && userLimit <= 0 {
			c.Next()
			return
		}

		// 令牌级静默夹紧到账户级：管理员事后调低账户级时，存量令牌自动收紧
		tokenLimit = service.ClampTokenMaxConcurrency(tokenLimit, userLimit)

		userId := c.GetInt("id")
		tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)

		var userKey, tokenKey string
		if userLimit > 0 && userId > 0 {
			userKey = limiter.UserConcurrencyKey(userId)
		}
		if tokenLimit > 0 && tokenId > 0 {
			tokenKey = limiter.TokenConcurrencyKey(tokenId)
		}
		if userKey == "" && tokenKey == "" {
			c.Next()
			return
		}

		member := c.GetString(common.RequestIdKey)

		if common.RedisEnabled {
			ctx := context.Background()
			outcome, err := limiter.AcquireConcurrencySlots(ctx, common.RDB,
				userKey, userLimit, tokenKey, tokenLimit, member, tokenConcurrencyTTLSeconds)
			if err != nil {
				// Redis 故障时放行而不是拒绝：限流组件挂掉不应该把整个网关打死。
				// Redis < 3.2 也会走到这里（脚本里的 TIME 被拒），表现为并发限制
				// 静默失效而非请求失败 —— 日志里能看到原因。
				common.SysLog("concurrency check failed, allowing request: " + err.Error())
				c.Next()
				return
			}
			if outcome != limiter.AcquireOK {
				abortWithConcurrencyExceeded(c, outcome, userLimit, tokenLimit)
				return
			}
			defer func() {
				if err := limiter.ReleaseConcurrencySlots(ctx, common.RDB, userKey, tokenKey, member); err != nil {
					common.SysLog("failed to release concurrency slots: " + err.Error())
				}
			}()
		} else {
			outcome := limiter.AcquireMemoryConcurrencySlots(userKey, userLimit, tokenKey, tokenLimit)
			if outcome != limiter.AcquireOK {
				abortWithConcurrencyExceeded(c, outcome, userLimit, tokenLimit)
				return
			}
			defer limiter.ReleaseMemoryConcurrencySlots(userKey, tokenKey)
		}

		c.Next()
	}
}

// abortWithConcurrencyExceeded 返回 429，按触发的层级给出不同的错误码与文案，
// 否则用户不知道该调令牌并发还是找管理员调账户并发。
//
// 不复用 abortWithOpenAiMessage：那个每次都走 logger.LogError，而被限流的客户端
// 往往会疯狂重试，每个 429 写一条错误日志会把日志 I/O 打成新的瓶颈。
func abortWithConcurrencyExceeded(c *gin.Context, outcome limiter.AcquireOutcome, userLimit, tokenLimit int) {
	var message string
	var code types.ErrorCode
	limit := tokenLimit
	if outcome == limiter.AcquireUserExceeded {
		limit = userLimit
		code = types.ErrorCodeUserConcurrencyExceeded
		message = fmt.Sprintf("当前账户并发数已达上限 %d，请降低并发或稍后重试", userLimit)
	} else {
		code = types.ErrorCodeTokenConcurrencyExceeded
		message = fmt.Sprintf("当前令牌并发数已达上限 %d，请降低并发或稍后重试", tokenLimit)
	}

	c.Header("Retry-After", "1")
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "new_api_error",
			"code":    string(code),
		},
	})
	c.Abort()
	// 落一条用户可见的使用日志，带节流（同一主体频繁超限时窗口逐级拉长），
	// 详见 token-concurrency-log.go
	logConcurrencyReject(c, outcome, limit)
}
