package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/samber/hot"
)

// 渠道健康度熔断。
//
// 目的是把「发现坏渠道」这件事从「每个请求都重新踩一遍坑」变成「一个请求踩到之后，
// 冷却窗口内其它请求直接绕开」。重试是被动的、按请求重复付费的；熔断是主动的、
// 全进程只付一次学习成本。这是让分组链真正变快的主要手段 —— 单纯提高重试次数
// 只会让失败请求更慢。
//
// 刻意使用进程本地内存而不是 Redis：熔断状态要在选路阶段对每个候选渠道查一次，
// 走 Redis 就是每候选一次网络往返，本来为了提速的功能反而会拖慢热路径。多节点
// 各自独立学习完全可以接受 —— 无非是每个节点各踩一次坑。
//
// 计数窗口是「距最后一次失败」的滑动窗口：每次失败都会刷新 TTL，所以持续失败的
// 渠道会一直处于冷却状态，而偶发失败会自然过期。
//
// 重要：熔断只能是「优先级」，不能是「硬排除」。如果整条链的候选全在冷却中，
// 选路必须忽略熔断再来一轮，否则一次全站抖动会让所有请求直接失败 —— 比不做熔断
// 更糟。这个兜底是 selectFromChain 的第二阶段。

const channelHealthDefaultCapacity = 100_000

var (
	channelHealthCache     *hot.HotCache[string, int]
	channelHealthCacheOnce sync.Once
	channelHealthMu        sync.Mutex
)

func getChannelHealthCache() *hot.HotCache[string, int] {
	channelHealthCacheOnce.Do(func() {
		setting := operation_setting.GetGroupChainSetting()
		capacity := setting.HealthMaxEntries
		if capacity <= 0 {
			capacity = channelHealthDefaultCapacity
		}
		channelHealthCache = hot.NewHotCache[string, int](hot.LRU, capacity).
			WithTTL(channelHealthWindow()).
			WithJanitor().
			Build()
	})
	return channelHealthCache
}

func channelHealthWindow() time.Duration {
	seconds := operation_setting.GetGroupChainSetting().HealthWindowSeconds
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func channelHealthThreshold() int {
	threshold := operation_setting.GetGroupChainSetting().HealthThreshold
	if threshold <= 0 {
		threshold = 3
	}
	return threshold
}

func channelHealthKey(channelID int, modelName string) string {
	return fmt.Sprintf("%d|%s", channelID, modelName)
}

// IsChannelCooling 判断 (渠道, 模型) 是否处于熔断冷却期。
// 选路热路径调用，必须保持纯内存、无 IO。
func IsChannelCooling(channelID int, modelName string) bool {
	if channelID <= 0 {
		return false
	}
	if !operation_setting.GetGroupChainSetting().HealthEnabled {
		return false
	}
	count, found, err := getChannelHealthCache().Get(channelHealthKey(channelID, modelName))
	if err != nil || !found {
		return false
	}
	return count >= channelHealthThreshold()
}

// MarkChannelFailure 记一次 (渠道, 模型) 失败。达到阈值后进入冷却。
func MarkChannelFailure(channelID int, modelName string) {
	if channelID <= 0 {
		return
	}
	if !operation_setting.GetGroupChainSetting().HealthEnabled {
		return
	}
	key := channelHealthKey(channelID, modelName)
	cache := getChannelHealthCache()

	// hot.HotCache 没有原子自增，用一把小锁串行化读改写。失败是低频事件，
	// 这把锁不在成功路径上，不会成为热点。
	channelHealthMu.Lock()
	defer channelHealthMu.Unlock()

	count, found, err := cache.Get(key)
	if err != nil || !found {
		count = 0
	}
	cache.SetWithTTL(key, count+1, channelHealthWindow())
}

// MarkChannelSuccess 清除 (渠道, 模型) 的失败计数，让恢复的渠道立即回到候选池。
func MarkChannelSuccess(channelID int, modelName string) {
	if channelID <= 0 {
		return
	}
	if !operation_setting.GetGroupChainSetting().HealthEnabled {
		return
	}
	getChannelHealthCache().Delete(channelHealthKey(channelID, modelName))
}

// ResetChannelHealth 清空全部熔断状态，供运维 / 测试使用。
func ResetChannelHealth() {
	channelHealthMu.Lock()
	defer channelHealthMu.Unlock()
	if channelHealthCache != nil {
		channelHealthCache.Purge()
	}
}
