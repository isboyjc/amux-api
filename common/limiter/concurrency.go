package limiter

import (
	"context"
	_ "embed"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/go-redis/redis/v8"
)

//go:embed lua/concurrency.lua
var concurrencyScriptSrc string

// concurrencyScript 用 redis.NewScript 而不是预加载 SHA + EvalSha：NewScript.Run
// 内部先试 EvalSha，遇到 NOSCRIPT 自动回退成 EVAL 并重新缓存脚本。
// 现有的 RedisLimiter 用的是「启动时 ScriptLoad 拿 SHA，之后一直 EvalSha」，
// Redis 重启或被 SCRIPT FLUSH 之后会永久失败，这里不要复制那个模式。
var concurrencyScript = redis.NewScript(concurrencyScriptSrc)

// AcquireOutcome 描述一次并发槽位申请的结果。
type AcquireOutcome int

const (
	AcquireOK            AcquireOutcome = 0 // 通过
	AcquireUserExceeded  AcquireOutcome = 1 // 账户级超限
	AcquireTokenExceeded AcquireOutcome = 2 // 令牌级超限
)

// UserConcurrencyKey / TokenConcurrencyKey 生成两级槽位的 Redis key。
func UserConcurrencyKey(userId int) string   { return fmt.Sprintf("concurrency:user:%d", userId) }
func TokenConcurrencyKey(tokenId int) string { return fmt.Sprintf("concurrency:token:%d", tokenId) }

// AcquireConcurrencySlots 一次往返内申请两级槽位。
//
// 传空 key 或 limit<=0 表示该级不限制、跳过。令牌级失败时账户级槽位在 Lua 内部
// 被原子回滚，调用方无需处理。
func AcquireConcurrencySlots(ctx context.Context, rdb *redis.Client,
	userKey string, userLimit int, tokenKey string, tokenLimit int,
	member string, ttlSeconds int) (AcquireOutcome, error) {
	res, err := concurrencyScript.Run(ctx, rdb,
		[]string{userKey, tokenKey},
		userLimit, tokenLimit, member, ttlSeconds).Int()
	if err != nil {
		return AcquireOK, err
	}
	return AcquireOutcome(res), nil
}

// ReleaseConcurrencySlots 释放两级槽位。
//
// 用 pipeline 把两个 ZREM 打包成一次往返。ZREM 对不存在的 member 返回 0 而不报错，
// 所以只配了一级时传空 key 跳过即可，无需区分。
func ReleaseConcurrencySlots(ctx context.Context, rdb *redis.Client, userKey, tokenKey, member string) error {
	if userKey == "" && tokenKey == "" {
		return nil
	}
	pipe := rdb.Pipeline()
	if userKey != "" {
		pipe.ZRem(ctx, userKey, member)
	}
	if tokenKey != "" {
		pipe.ZRem(ctx, tokenKey, member)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ── 无 Redis 时的进程内降级实现 ───────────────────────────────────────────
//
// 注意语义差异：计数只在当前进程内，多实例部署下每个实例各算一份，
// 实际全局上限约等于「配置值 × 实例数」。这是近似而非硬保证，前端文案需讲清楚。
//
// 用分片锁而不是单把全局锁：现有的 common.InMemoryRateLimiter 是一把 Mutex 保护
// 整个 map，所有用户的限流判断相互串行化，高并发下就是瓶颈。这里按 key 哈希分片，
// 不同用户/令牌互不阻塞。临界区本身只有几个整数操作。

const concurrencyShardCount = 32

type concurrencyShard struct {
	mu    sync.Mutex
	inUse map[string]int
}

var memConcurrencyShards = func() [concurrencyShardCount]*concurrencyShard {
	var shards [concurrencyShardCount]*concurrencyShard
	for i := range shards {
		shards[i] = &concurrencyShard{inUse: make(map[string]int)}
	}
	return shards
}()

func shardFor(key string) *concurrencyShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return memConcurrencyShards[h.Sum32()%concurrencyShardCount]
}

func acquireMemorySlot(key string, limit int) bool {
	sh := shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.inUse[key] >= limit {
		return false
	}
	sh.inUse[key]++
	return true
}

func releaseMemorySlot(key string) {
	sh := shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if n := sh.inUse[key]; n <= 1 {
		delete(sh.inUse, key) // 归零即删，避免 map 无限增长
	} else {
		sh.inUse[key] = n - 1
	}
}

// AcquireMemoryConcurrencySlots 进程内版本的两级申请，语义与 Redis 版一致
// （含令牌级失败时回滚账户级）。
func AcquireMemoryConcurrencySlots(userKey string, userLimit int, tokenKey string, tokenLimit int) AcquireOutcome {
	userTaken := false
	if userKey != "" && userLimit > 0 {
		if !acquireMemorySlot(userKey, userLimit) {
			return AcquireUserExceeded
		}
		userTaken = true
	}
	if tokenKey != "" && tokenLimit > 0 {
		if !acquireMemorySlot(tokenKey, tokenLimit) {
			if userTaken {
				releaseMemorySlot(userKey)
			}
			return AcquireTokenExceeded
		}
	}
	return AcquireOK
}

// ReleaseMemoryConcurrencySlots 释放进程内两级槽位。
func ReleaseMemoryConcurrencySlots(userKey, tokenKey string) {
	if userKey != "" {
		releaseMemorySlot(userKey)
	}
	if tokenKey != "" {
		releaseMemorySlot(tokenKey)
	}
}
