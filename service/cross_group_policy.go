package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// 跨分组回退策略与重试时间预算。
//
// 这一层只约束「是否走出当前分组」，不改变分组内的重试判定（那仍然由
// operation_setting.ShouldRetryByStatusCode 决定），所以对单分组令牌完全无感。
//
// 之所以需要它：默认的重试状态码范围包含 4xx（401-407、409-499），对单分组令牌
// 是合理的（换个渠道可能就好了），但对分组链是灾难 —— 一个参数非法或被内容审核
// 拦下的请求，会被原样打到链上每一个分组，白烧额度和时延，而且必然全部失败。

// AllowCrossGroupFallback 判断该错误是否值得回落到下一个分组。
// 返回 false 表示只在当前分组内继续重试。
func AllowCrossGroupFallback(err *types.NewAPIError) bool {
	if err == nil {
		return true
	}
	setting := operation_setting.GetGroupChainSetting()

	errorCode := string(err.GetErrorCode())
	for _, blocked := range setting.CrossGroupSkipErrorCodes {
		if blocked == errorCode {
			return false
		}
	}

	statusCode := err.StatusCode
	for _, blocked := range setting.CrossGroupSkipStatusCodes {
		if blocked == statusCode {
			return false
		}
	}
	return true
}

// SetCrossGroupBlocked 标记本次请求后续是否禁止跨分组回退。
func SetCrossGroupBlocked(c *gin.Context, blocked bool) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyCrossGroupBlocked, blocked)
}

func isCrossGroupBlocked(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return common.GetContextKeyBool(c, constant.ContextKeyCrossGroupBlocked)
}

// StartRetryBudget 记录重试时间预算的截止时刻。
//
// 用时间而不是次数来约束尾延迟：一次上游超时可能几十秒，「最多换 2 个分组」这种
// 配置根本无法反映用户实际等待了多久。链长和延迟因此彻底解耦 —— 链想配多长配多长。
func StartRetryBudget(c *gin.Context) {
	if c == nil {
		return
	}
	budgetMs := operation_setting.GetGroupChainSetting().TotalRetryBudgetMs
	if budgetMs <= 0 {
		return
	}
	common.SetContextKey(c, constant.ContextKeyRetryDeadline, time.Now().Add(time.Duration(budgetMs)*time.Millisecond))
}

// RetryBudgetExhausted 判断是否已经用光重试时间预算。未配置预算时恒为 false。
func RetryBudgetExhausted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := common.GetContextKey(c, constant.ContextKeyRetryDeadline)
	if !ok {
		return false
	}
	deadline, ok := v.(time.Time)
	if !ok {
		return false
	}
	return time.Now().After(deadline)
}
