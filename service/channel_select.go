package service

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx        *gin.Context
	TokenGroup string
	ModelName  string
	Retry      *int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For the "auto" tokenGroup this function is a PURE selector:
// 对于 "auto" tokenGroup，本函数只做"纯选择"：
//
//   - It reads the current group index from ContextKeyAutoGroupIndex (default 0) and
//     the in-group priority retry from param.Retry.
//     从 ContextKeyAutoGroupIndex（默认 0）读取当前分组索引，从 param.Retry 读取组内优先级重试号。
//
//   - Starting from that index it skips any group that has no channel for the model,
//     then returns a channel from the first group that does, recording the chosen group
//     in ContextKeyAutoGroup (for billing) and the chosen index in ContextKeyAutoGroupIndex.
//     从该索引开始跳过没有该模型渠道的分组，返回第一个有渠道的分组里的渠道，
//     并把命中的分组写入 ContextKeyAutoGroup（计费用）、命中的索引写入 ContextKeyAutoGroupIndex。
//
//   - It does NOT decide whether to switch groups on an upstream error. That decision is
//     made by the relay retry loop (controller/relay.go) which knows the actual error, and
//     advances the group index via AdvanceToNextAutoGroup before calling this again.
//     它不再根据上游错误决定是否切组——切组由 relay 重试循环（controller/relay.go）根据真实
//     错误，通过 AdvanceToNextAutoGroup 推进分组索引后再次调用本函数完成。
//
//   - When the requested index (and every group after it) has no channel for the model it
//     returns a nil channel, signalling the caller that all auto groups are exhausted.
//     当请求的索引（及其之后所有分组）都没有该模型渠道时返回 nil channel，告知调用方所有自动分组已用尽。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("no usable auto group for current user")
		}

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// priorityRetry is the in-group priority retry for the requested group.
			// 当跳过空组落到后续分组时，新分组从优先级 0 开始。
			priorityRetry := param.GetRetry()
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, _ = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry)
			if channel == nil {
				// Current group has no available channel for this model, try next group.
				// 当前分组没有该模型的可用渠道，尝试下一个分组。
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s, trying next group", autoGroup, param.ModelName)
				// Keep the loop's in-group counter aligned with the freshly entered group.
				// 让外层循环的组内计数器与新进入的分组对齐。
				param.SetRetry(0)
				continue
			}

			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)
			break
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry())
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

// AdvanceToNextAutoGroup advances the auto group index to the next group so that the next
// call to CacheGetRandomSatisfiedChannel starts searching from there. It returns true when
// there is at least one more group to try, false when the auto group list is exhausted.
//
// AdvanceToNextAutoGroup 把自动分组索引推进到下一个分组，使下次 CacheGetRandomSatisfiedChannel
// 从该位置开始搜索。还有可尝试的分组时返回 true，自动分组列表已用尽时返回 false。
//
// Empty groups (no channel for the model) are skipped inside CacheGetRandomSatisfiedChannel,
// so this only needs the raw list length to know whether any group remains.
// 空分组（没有该模型渠道）由 CacheGetRandomSatisfiedChannel 内部跳过，这里只需用列表长度判断是否还有剩余分组。
func AdvanceToNextAutoGroup(c *gin.Context) bool {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	autoGroups := GetUserAutoGroup(userGroup)

	currentIndex := 0
	if v, exists := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := v.(int); ok {
			currentIndex = idx
		}
	}

	nextIndex := currentIndex + 1
	if nextIndex >= len(autoGroups) {
		return false
	}
	common.SetContextKey(c, constant.ContextKeyAutoGroupIndex, nextIndex)
	logger.LogDebug(c, "Auto group cross-group fallback: advancing group index %d -> %d", currentIndex, nextIndex)
	return true
}

// CrossGroupShouldFallback decides whether an upstream failure should trigger an auto
// cross-group fallback (try the next group). It is intentionally MORE permissive than the
// in-group retry gate (shouldRetry): the whole point of auto cross-group is to route around a
// group whose channel/model is unavailable, so it falls through for channel-side errors
// (429/5xx/timeouts/upstream 404/...) even when they would not normally be retried in-group.
//
// CrossGroupShouldFallback 判断一个上游失败是否应触发自动跨分组回退（尝试下一个分组）。
// 它比组内重试闸门（shouldRetry）更宽松：跨分组回退的目的就是绕开某个渠道/模型不可用的分组，
// 因此对渠道侧错误（429/5xx/超时/上游 404 等）即便组内不会重试也会回退。
//
// It does NOT fall through for failures that would fail identically on every group:
// 对于在每个分组都会同样失败的错误，它不会回退：
//   - the token is bound to a specific channel (令牌绑定了固定渠道)
//   - the channel-affinity layer requested a hard stop (亲和性层要求停止)
//   - skip-retry errors such as oversized body / sensitive words (413/敏感词等预检错误)
//   - client validation errors (400/422) — a bad request fails everywhere (客户端参数错误)
func CrossGroupShouldFallback(c *gin.Context, err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if isClientValidationStatusCode(err.StatusCode) {
		return false
	}
	return true
}

// isClientValidationStatusCode reports whether the status code denotes a client-side
// validation failure that will fail identically across all groups. Keep this list narrow:
// to also fall through on 400 (some upstreams use it for "model unavailable"), remove it here.
//
// isClientValidationStatusCode 判断状态码是否表示"在所有分组都会同样失败"的客户端参数错误。
// 这个列表要保持窄：若想对 400 也回退（部分上游用 400 表达"模型不可用"），把它从这里移除即可。
func isClientValidationStatusCode(code int) bool {
	switch code {
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		return true
	}
	return false
}
