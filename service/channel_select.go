package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	Retry        *int
	resetNextTry bool
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
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (no channel in group for this model),
//     moves to next group. When the current group's priorities are exhausted
//     (priorityRetry >= RetryTimes) and crossGroupRetry is enabled, also moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（当前分组无该模型的可用渠道）时，
//     切换到下一个分组。当当前分组的优先级用完（priorityRetry >= RetryTimes）且
//     启用了跨分组重试时，也切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → fall through to GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 自动落到 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
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
			// 用户在自动分组配置中没有可用的分组（被用户分组限制过滤掉）
			// The user has no accessible group in the auto-group configuration
			return nil, selectGroup, errors.New("user has no accessible auto groups")
		}

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		// startRetryIndex: the global Retry value at which the current group started,
		// used to compute priorityRetry = currentRetry - startRetryIndex.
		// 当前分组开始时的全局 Retry 值，用于计算 priorityRetry = currentRetry - startRetryIndex
		startRetryIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}
		if lastRetryIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex); exists {
			if idx, ok := lastRetryIndex.(int); ok {
				startRetryIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// priorityRetry represents the priority level within the current group.
			// When i > startGroupIndex, we are continuing within the same call after
			// having fallen through from a previous group, so priorityRetry must be 0
			// for the new group (this is the "within-call fall-through" case).
			// When i == startGroupIndex, priorityRetry = param.GetRetry() - startRetryIndex,
			// which gives the priority within the current group based on the global retry
			// counter and the saved startRetryIndex.
			// priorityRetry 表示当前分组内的优先级级别
			var priorityRetry int
			if i > startGroupIndex {
				// Within-call fall-through: the new group always starts at priority 0.
				// 同一次调用内跨分组回落：新分组始终从优先级 0 开始
				priorityRetry = 0
			} else {
				priorityRetry = param.GetRetry() - startRetryIndex
				if priorityRetry < 0 {
					priorityRetry = 0
				}
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s (index=%d, startRetryIndex=%d, currentRetry=%d, priorityRetry=%d)", autoGroup, i, startRetryIndex, param.GetRetry(), priorityRetry)

			channel, _ = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry)
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// Advance to next group. Record the current global Retry as the new
				// group's startRetryIndex so that priorityRetry on subsequent calls
				// is computed correctly (next call's priorityRetry = (Retry+1) - startRetryIndex = 1).
				// Note: do NOT call SetRetry(0) or ResetRetryNextTry() here. The outer
				// for-loop's IncreaseRetry must run normally so that:
				//   - the for-loop's total iteration count is bounded by RetryTimes
				//   - subsequent iterations of the for-loop get distinct retry values
				// Within-call fall-through to the next group (i > startGroupIndex)
				// already forces priorityRetry = 0 for the new group on the same call.
				// 切换到下一个分组。把当前全局 Retry 记录为新分组的 startRetryIndex，
				// 这样后续调用计算 priorityRetry 时 = (Retry+1) - startRetryIndex = 1。
				// 注意：不要在这里调用 SetRetry(0) 或 ResetRetryNextTry()。
				// 外层 for 循环的 IncreaseRetry 必须正常运行,以便:
				//   - for 循环的总迭代次数由 RetryTimes 限制
				//   - 后续迭代获取不同的 retry 值
				// 同一次调用内跨分组回落(i > startGroupIndex)已经强制新分组的 priorityRetry = 0。
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, param.GetRetry())
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Record the current global Retry as the new group's startRetryIndex
				// so that the next call's priorityRetry = (Retry+1) - startRetryIndex = 1.
				// 把当前全局 Retry 记录为新分组的 startRetryIndex
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, param.GetRetry())
				// Reset retry counter so outer loop's next IncreaseRetry starts fresh
				// at the new group's priority 0.
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
				// Keep startRetryIndex unchanged so priorityRetry = Retry - startRetryIndex
				// increments by 1 on each retry within the same group.
				// 保持 startRetryIndex 不变，使得同分组内重试时 priorityRetry = Retry - startRetryIndex 递增 1
			}
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
