package middleware

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// 并发超限日志的节流策略：递增退避窗口。
//
// 目标是「跑飞的客户端不能把日志表打爆」，同时「偶发超限仍要看得见」。
// 做法是同一令牌连续触发时把窗口逐级拉长：
//
//	第 1 条 → 之后静默 30s
//	仍在触发 → 静默 2min → 8min → 30min（上限）
//
// 每条日志都带上「本窗口内被拒 N 次」，所以拉长窗口不丢信息量，只是聚合得更粗。
// 一旦令牌安静下来（超过当前窗口没有再触发），级别重置回最短窗口，
// 这样下次偶发超限还能立刻看到。
var tokenConcurrencyLogWindows = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	8 * time.Minute,
	30 * time.Minute,
}

type concurrencyLogState struct {
	nextLogAt  time.Time // 下次允许写库的时间
	level      int       // 当前退避级别，索引 tokenConcurrencyLogWindows
	suppressed int       // 自上次写库以来被抑制（未记录）的次数
	lastHitAt  time.Time // 最近一次触发时间，用于判断是否该重置级别
}

// concurrencyLogKey 区分节流主体：账户级按 userId、令牌级按 tokenId。
// 不区分的话两级会共用一个节流状态、互相抑制，导致其中一级的日志看不见。
type concurrencyLogKey struct {
	isUser bool
	id     int
}

var (
	concurrencyLogMu    sync.Mutex
	concurrencyLogStats = make(map[concurrencyLogKey]*concurrencyLogState)
	// 上次清理时间。map 只在「设了并发限制且真的超限」的令牌上增长，
	// 规模天然很小，用惰性清理即可，不额外起 goroutine。
	concurrencyLogLastGC time.Time
)

// shouldLogConcurrencyReject 判断本次超限是否需要落库。
// 返回 (是否写库, 本窗口内被抑制的次数)。
func shouldLogConcurrencyReject(k concurrencyLogKey, now time.Time) (bool, int) {
	concurrencyLogMu.Lock()
	defer concurrencyLogMu.Unlock()

	gcConcurrencyLogStatsLocked(now)

	st, ok := concurrencyLogStats[k]
	if !ok {
		st = &concurrencyLogState{}
		concurrencyLogStats[k] = st
	}

	// 距上次触发已超过「当前级别窗口 × 2」→ 认为客户端已恢复正常，重置退避级别，
	// 否则一个偶发超限过很久之后再次发生时会继承上次的长窗口而被静默。
	if !st.lastHitAt.IsZero() {
		idle := tokenConcurrencyLogWindows[st.level] * 2
		if now.Sub(st.lastHitAt) > idle {
			st.level = 0
			st.suppressed = 0
		}
	}
	st.lastHitAt = now

	if now.Before(st.nextLogAt) {
		st.suppressed++
		return false, st.suppressed
	}

	suppressed := st.suppressed
	st.suppressed = 0
	st.nextLogAt = now.Add(tokenConcurrencyLogWindows[st.level])
	// 逐级拉长，直到上限
	if st.level < len(tokenConcurrencyLogWindows)-1 {
		st.level++
	}
	return true, suppressed
}

// gcConcurrencyLogStatsLocked 清理长期无活动的条目。调用方须持有锁。
func gcConcurrencyLogStatsLocked(now time.Time) {
	if now.Sub(concurrencyLogLastGC) < 10*time.Minute {
		return
	}
	concurrencyLogLastGC = now
	maxIdle := tokenConcurrencyLogWindows[len(tokenConcurrencyLogWindows)-1] * 2
	for id, st := range concurrencyLogStats {
		if now.Sub(st.lastHitAt) > maxIdle {
			delete(concurrencyLogStats, id)
		}
	}
}

// logConcurrencyReject 按节流策略把超限事件写进使用日志（LogTypeError）。
//
// 写库放在 gopool 里异步做：RecordErrorLog 内部会查 GetUserSetting（可能打库），
// 不能让它阻塞一个正在被拒绝的请求 —— 429 的价值就在于快速返回。
// concurrencyLogAsyncDisabled 仅测试用：置为 true 时跳过异步落库。
//
// 落库走 gopool goroutine，里面会读 common.RedisEnabled 等全局变量。测试用例
// 结束时若恢复这些全局变量，就会和仍在运行的 goroutine 构成数据竞争（-race 报错）。
// 生产环境这些变量只在启动时写一次，不存在该问题。
var concurrencyLogAsyncDisabled bool

func logConcurrencyReject(c *gin.Context, outcome limiter.AcquireOutcome, limit int) {
	userId := c.GetInt("id")
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)

	isUser := outcome == limiter.AcquireUserExceeded
	k := concurrencyLogKey{isUser: isUser, id: tokenId}
	scope, reason := "令牌", "token_concurrency_exceeded"
	if isUser {
		k.id = userId
		scope, reason = "账户", "user_concurrency_exceeded"
	}

	should, suppressed := shouldLogConcurrencyReject(k, time.Now())
	if !should {
		return
	}

	tokenName := c.GetString("token_name")
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)

	content := fmt.Sprintf("%s并发数已达上限 %d，请求被拒绝（HTTP 429）", scope, limit)
	if suppressed > 0 {
		content += fmt.Sprintf("；同一节流窗口内另有 %d 次被拒未单独记录", suppressed)
	}

	// gin.Context 在 handler 返回后不可再用（值会被回收/复用），所以先把需要的
	// 字段取出来，goroutine 里只用这些副本 + 一个脱离请求的 context。
	if concurrencyLogAsyncDisabled {
		return
	}
	ctx := c.Copy()
	gopool.Go(func() {
		model.RecordErrorLog(ctx, userId, 0, modelName, tokenName, content, tokenId, 0, false, group,
			map[string]interface{}{
				"reject_reason":    reason,
				"concurrency_max":  limit,
				"suppressed_count": suppressed,
			})
	})
}
