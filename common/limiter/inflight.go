package limiter

import (
	"hash/fnv"
	"sync"
)

// 在途请求数的「观测」计数，与并发「限制」是两套独立机制，原因如下。
//
// 限制（concurrency.go / lua/concurrency.lua）要求原子的 check-and-set、跨实例
// 强一致、超限必须能拒绝，所以只能同步走 Redis。观测只是给管理页展示一个数字，
// 允许几秒延迟、允许近似 —— 用限制那套机制去满足观测需求，代价是每个中继请求
// 多 2 次 Redis 往返（未配上限的令牌当前是零成本快速退出，那样就退化了）。
//
// 所以这里的取舍是：
//
//	写路径（每请求）  纯进程内：一次分片锁 + 一次整数自增，无 Redis、无分配
//	读路径（展示）    汇总各实例定期上报的快照（见 service/inflight_reporter.go）
//
// 上报频率与 QPS 完全解耦 —— 10 QPS 和 10000 QPS 的 Redis 开销完全相同。
//
// 与限制计数的语义差异（UI 必须讲清楚）：
//   - 这里统计所有令牌，包括未配并发上限的（限制那套会跳过它们）
//   - 数值最多滞后一个上报周期
//   - 单实例进程被强杀时它自己那份快照会残留，靠 key TTL 过期自愈
//
// 覆盖范围的已知缺口：计数挂在 TokenConcurrencyLimit 中间件上，所以**只覆盖挂了
// 该中间件的路由**。当前 /v1/realtime（WebSocket）有意没挂它（见 relay-router.go
// 的注释：长会话会与槽位 TTL 回收机制打架），因此实时会话不计入在途数。
//
// 这里刻意不为观测单独去 hook /realtime：进程内计数没有 TTL 自愈机制，
// WebSocket 连接异常断开若 defer 没跑到，计数会永久偏高且只能靠重启清零 ——
// 对一个展示用数字来说，「不显示」比「显示一个只增不减的错值」更好。

const inflightShardCount = 32

type inflightShard struct {
	mu     sync.Mutex
	tokens map[int]int
}

var inflightShards = func() [inflightShardCount]*inflightShard {
	var shards [inflightShardCount]*inflightShard
	for i := range shards {
		shards[i] = &inflightShard{tokens: make(map[int]int)}
	}
	return shards
}()

func inflightShardFor(tokenId int) *inflightShard {
	h := fnv.New32a()
	// 小端写入 4 字节，避免 strconv 产生字符串分配
	_, _ = h.Write([]byte{
		byte(tokenId), byte(tokenId >> 8), byte(tokenId >> 16), byte(tokenId >> 24),
	})
	return inflightShards[h.Sum32()%inflightShardCount]
}

// IncInflight 标记该令牌多了一个在途请求。必须与 DecInflight 配对（defer）。
func IncInflight(tokenId int) {
	if tokenId <= 0 {
		return
	}
	sh := inflightShardFor(tokenId)
	sh.mu.Lock()
	sh.tokens[tokenId]++
	sh.mu.Unlock()
}

// DecInflight 释放一个在途标记。归零即删 key，避免 map 随历史令牌数无限增长。
func DecInflight(tokenId int) {
	if tokenId <= 0 {
		return
	}
	sh := inflightShardFor(tokenId)
	sh.mu.Lock()
	if n := sh.tokens[tokenId]; n <= 1 {
		delete(sh.tokens, tokenId)
	} else {
		sh.tokens[tokenId] = n - 1
	}
	sh.mu.Unlock()
}

// SnapshotInflight 取当前进程的在途快照，供上报器使用。
//
// 逐个分片加锁而不是全局停写：分片之间的读取不在同一时刻，得到的是「近似同一
// 时刻」的快照。观测场景可以接受，换来的是不阻塞写路径。
func SnapshotInflight() map[int]int {
	out := make(map[int]int)
	for _, sh := range inflightShards {
		sh.mu.Lock()
		for tokenId, n := range sh.tokens {
			if n > 0 {
				out[tokenId] += n
			}
		}
		sh.mu.Unlock()
	}
	return out
}
