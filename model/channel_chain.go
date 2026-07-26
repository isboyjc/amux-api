package model

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 分组链（多分组令牌 / 旧版 auto 分组）使用的渠道选择原语。
//
// 与 GetRandomSatisfiedChannel 的区别：
//
//  1. 优先级层数是「真实层数」。GetRandomSatisfiedChannel 在 retry 超出层数时会
//     clamp 到最低优先级（见 channel_cache.go 中 `if retry >= len(uniquePriorities)`
//     以及 ability.go 中 getPriority 的同款逻辑），永远返回渠道而不是 nil，导致
//     调用方无法区分「还有下一层」和「这个分组已经用完了」。分组链必须能区分，
//     否则永远切不到下一个分组。
//
//  2. 支持排除已尝试过的渠道。老逻辑只按 (group, model, retry) 加权随机，同一
//     优先级层里完全可能反复抽中刚刚失败的那个渠道，重试等于空转。
//
// 两条路径（内存缓存 / 直连数据库）都要实现：MemoryCacheEnabled 在启用 Redis 时
// 会被自动置 true（见 main.go），但私有部署可能两者都没开。
//
// 加权随机沿用内存缓存路径的平滑算法（见 pickChannelByWeight），使绝大多数部署
// 的渠道命中分布与改动前完全一致。

// GetPriorityLevelCount 返回 (group, model) 下启用渠道的不同优先级层数。
// 返回 0 表示该分组下没有该模型的可用渠道，调用方应当直接跳到下一个分组，
// 且不应消耗任何重试预算 —— 这一步是纯内存查找，没有 IO，也不会请求上游。
func GetPriorityLevelCount(group string, modelName string) int {
	if group == "" || modelName == "" {
		return 0
	}
	if !common.MemoryCacheEnabled {
		priorities, err := getPriorityLevelsDB(group, modelName)
		if err != nil {
			return 0
		}
		return len(priorities)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channels := lookupGroupModelChannels(group, modelName)
	if len(channels) == 0 {
		return 0
	}
	priorities := make(map[int64]struct{}, len(channels))
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			priorities[channel.GetPriority()] = struct{}{}
		}
	}
	return len(priorities)
}

// PickChannelAtLevel 在 (group, model) 的第 level 个优先级层中按权重随机选一个渠道，
// level 从 0 开始，0 是最高优先级。skip 返回 true 的渠道会被跳过；skip 为 nil 表示
// 不跳过任何渠道。
//
// 用谓词而不是集合，是因为调用方需要按候选逐个判断多种条件（本次请求已尝试过、
// 处于熔断冷却期），这些条件没法预先展开成一个 ID 集合。
//
// 返回 (nil, nil) 表示该层没有可选渠道（层不存在，或该层渠道都被 skip 掉了），
// 调用方应继续尝试下一层 / 下一个分组，而不是把它当成错误。
func PickChannelAtLevel(group string, modelName string, level int, skip func(channelID int) bool) (*Channel, error) {
	if group == "" || modelName == "" || level < 0 {
		return nil, nil
	}
	if !common.MemoryCacheEnabled {
		return pickChannelAtLevelDB(group, modelName, level, skip)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channelIds := lookupGroupModelChannels(group, modelName)
	if len(channelIds) == 0 {
		return nil, nil
	}

	uniquePriorities := make(map[int64]struct{}, len(channelIds))
	for _, channelId := range channelIds {
		channel, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
		uniquePriorities[channel.GetPriority()] = struct{}{}
	}
	sortedPriorities := sortPrioritiesDesc(uniquePriorities)
	if level >= len(sortedPriorities) {
		// 与 GetRandomSatisfiedChannel 的关键差异：这里不 clamp，如实返回「没有了」。
		return nil, nil
	}
	targetPriority := sortedPriorities[level]

	candidates := make([]*Channel, 0, len(channelIds))
	for _, channelId := range channelIds {
		if skip != nil && skip(channelId) {
			continue
		}
		channel, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
		if channel.GetPriority() == targetPriority {
			candidates = append(candidates, channel)
		}
	}
	return pickChannelByWeight(candidates), nil
}

// lookupGroupModelChannels 读取内存索引，带模型名归一化回退（gpts / thinking-* 等）。
// 调用方必须已持有 channelSyncLock 读锁。
func lookupGroupModelChannels(group string, modelName string) []int {
	if group2model2channels == nil {
		return nil
	}
	channels := group2model2channels[group][modelName]
	if len(channels) == 0 {
		normalized := ratio_setting.FormatMatchingModelName(modelName)
		if normalized != "" && normalized != modelName {
			channels = group2model2channels[group][normalized]
		}
	}
	return channels
}

func sortPrioritiesDesc(set map[int64]struct{}) []int64 {
	sorted := make([]int64, 0, len(set))
	for priority := range set {
		sorted = append(sorted, priority)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	return sorted
}

// pickChannelByWeight 按权重随机挑一个渠道，算法与 GetRandomSatisfiedChannel
// 的内存路径保持一致（含权重为 0 与平均权重过小时的平滑处理），以保证改动前后
// 渠道命中分布不变。候选为空返回 nil。
func pickChannelByWeight(candidates []*Channel) *Channel {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	sumWeight := 0
	for _, channel := range candidates {
		sumWeight += channel.GetWeight()
	}

	smoothingFactor := 1
	smoothingAdjustment := 0
	if sumWeight == 0 {
		// 所有渠道权重都是 0：等权随机，每个渠道有效权重 100
		sumWeight = len(candidates) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(candidates) < 10 {
		smoothingFactor = 100
	}

	totalWeight := sumWeight * smoothingFactor
	if totalWeight <= 0 {
		return candidates[0]
	}
	randomWeight := rand.Intn(totalWeight)
	for _, channel := range candidates {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel
		}
	}
	// 浮点/取整边界兜底，正常不会走到
	return candidates[len(candidates)-1]
}

// getPriorityLevelsDB 返回 (group, model) 下启用渠道的优先级列表，降序去重。
func getPriorityLevelsDB(group string, modelName string) ([]int64, error) {
	var priorities []int64
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, modelName, true).
		Order("priority DESC").
		Pluck("priority", &priorities).Error
	if err != nil {
		return nil, err
	}
	if len(priorities) == 0 {
		// 归一化模型名回退，与内存路径保持一致
		normalized := ratio_setting.FormatMatchingModelName(modelName)
		if normalized != "" && normalized != modelName {
			err = DB.Model(&Ability{}).
				Select("DISTINCT(priority)").
				Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, normalized, true).
				Order("priority DESC").
				Pluck("priority", &priorities).Error
			if err != nil {
				return nil, err
			}
		}
	}
	return priorities, nil
}

func pickChannelAtLevelDB(group string, modelName string, level int, skip func(channelID int) bool) (*Channel, error) {
	priorities, err := getPriorityLevelsDB(group, modelName)
	if err != nil {
		return nil, err
	}
	if level >= len(priorities) {
		return nil, nil
	}
	targetPriority := priorities[level]

	var abilities []Ability
	err = DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?",
		group, modelName, true, targetPriority).Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		normalized := ratio_setting.FormatMatchingModelName(modelName)
		if normalized != "" && normalized != modelName {
			err = DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?",
				group, normalized, true, targetPriority).Find(&abilities).Error
			if err != nil {
				return nil, err
			}
		}
	}

	channelIds := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		if skip != nil && skip(ability.ChannelId) {
			continue
		}
		channelIds = append(channelIds, ability.ChannelId)
	}
	if len(channelIds) == 0 {
		return nil, nil
	}

	var channels []*Channel
	if err := DB.Where("id IN (?)", channelIds).Find(&channels).Error; err != nil {
		return nil, err
	}
	return pickChannelByWeight(channels), nil
}
