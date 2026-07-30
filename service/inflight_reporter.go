package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
)

// 在途请求数的跨实例汇总。
//
// 为什么是「定期上报」而不是「每请求写 Redis」：见 common/limiter/inflight.go 的
// 顶部注释。要点是写路径开销必须与 QPS 无关 —— 这里每个实例每 InflightReportInterval
// 只发一个 pipeline，无论期间处理了 10 个还是 10 万个请求。
//
// 数据布局：每个实例一个自己的 hash，另有一个注册表 ZSET 记录活跃实例。
//
//	inflight:node:{instanceId}  hash  → { "{tokenId}": count, ... }
//	inflight:nodes              zset  → member=instanceId, score=上报时刻
//
// 不共用一个 hash：多实例写同一个 field 会互相覆盖（谁最后写谁赢），账就错了。
// 一实例一 key 后各写各的，读时求和。
//
// 为什么要注册表而不是读时 KEYS/SCAN 匹配 inflight:node:*：KEYS 在大 keyspace 上
// 是 O(N) 阻塞操作，SCAN 要多次往返且仍要遍历全库。ZSET 直接给出活跃实例列表，
// 一次 ZRANGEBYSCORE 就够，成本只与实例数相关。
//
// 泄漏自愈：hash 带 TTL，实例被强杀后不再续期，TTL 到点自动消失；注册表按 score
// （上报时刻）裁掉过期成员。两者都不需要墓碑或显式注销。
const (
	// InflightReportInterval 上报间隔。取得比前端轮询间隔（5s）略小，
	// 使展示的滞后不超过一个前端周期。
	InflightReportInterval = 3 * time.Second
	// inflightKeyTTL 快照存活时间。取上报间隔的 4 倍：容忍偶发的上报失败/抖动
	// 而不至于让数字闪烁归零，同时进程被强杀后最多 12 秒就自愈。
	inflightKeyTTL = 4 * InflightReportInterval

	inflightKeyPrefix   = "inflight:node:"
	inflightRegistryKey = "inflight:nodes"
)

var inflightReporterOnce sync.Once

func inflightNodeKey(instanceId string) string {
	return inflightKeyPrefix + instanceId
}

// StartInflightReporter 启动在途数上报器。
//
// 与项目里其他后台任务不同，这个必须在**每个**节点上跑（不能加 IsMasterNode
// 守卫）：每个实例只知道自己进程内的在途数，从节点不上报的话，它承载的请求在
// 管理页上就完全看不见。
//
// 无 Redis 时不启动：此时只有单进程视角，读取侧直接取本地快照。
func StartInflightReporter() {
	inflightReporterOnce.Do(func() {
		if !common.RedisEnabled {
			return
		}
		instanceId := common.InstanceId
		key := inflightNodeKey(instanceId)
		gopool.Go(func() {
			common.SysLog(fmt.Sprintf("inflight reporter started (instance %s, interval %s)",
				instanceId, InflightReportInterval))
			ticker := time.NewTicker(InflightReportInterval)
			defer ticker.Stop()
			for range ticker.C {
				reportInflightOnce(instanceId, key)
			}
		})
	})
}

// reportInflightOnce 把本进程快照覆盖写入自己的 hash，并在注册表里续期。
//
// 用 DEL + HSET 而不是逐个 field 更新：归零的令牌必须从 hash 里消失，否则它会
// 一直以旧值挂在那里（HSET 只增改不删）。DEL 与 HSET 在同一个 pipeline 里，
// 中间那个「key 不存在」的瞬间读到的是 0 而不是脏值，对观测无害。
//
// 快照为空时只删 hash 不写：一个完全空闲的实例不该在 Redis 里留下任何 hash。
// 但注册表仍要续期，否则它会被判定为已死 —— 而它其实活着，只是没有在途请求。
func reportInflightOnce(instanceId, key string) {
	snapshot := limiter.SnapshotInflight()
	ctx := context.Background()
	now := float64(time.Now().Unix())

	pipe := common.RDB.Pipeline()
	pipe.Del(ctx, key)
	if len(snapshot) > 0 {
		values := make([]any, 0, len(snapshot)*2)
		for tokenId, n := range snapshot {
			values = append(values, strconv.Itoa(tokenId), n)
		}
		pipe.HSet(ctx, key, values...)
		pipe.Expire(ctx, key, inflightKeyTTL)
	}
	pipe.ZAdd(ctx, inflightRegistryKey, &redis.Z{Score: now, Member: instanceId})
	// 裁掉早已不再上报的实例，避免注册表随历史容器数无限增长
	pipe.ZRemRangeByScore(ctx, inflightRegistryKey, "-inf",
		strconv.FormatFloat(now-inflightKeyTTL.Seconds(), 'f', -1, 64))
	pipe.Expire(ctx, inflightRegistryKey, inflightKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		// 上报失败不影响任何请求路径，记一条日志即可。前端最多看到数字滞后。
		common.SysLog("failed to report inflight snapshot: " + err.Error())
	}
}

// GetInflightByTokens 返回给定令牌的当前在途请求数（跨实例求和）。
//
// 只查询传入的令牌（当页那几个），不做全量扫描 —— 单次成本是一个 pipeline，
// 每个活跃实例一次 HMGET。
//
// 无 Redis 时退化成本进程快照，由 InflightIsClusterWide 告知前端该数字只代表
// 当前实例，UI 需给出说明。
func GetInflightByTokens(tokenIds []int) (map[int]int, error) {
	result := make(map[int]int, len(tokenIds))
	if len(tokenIds) == 0 {
		return result, nil
	}

	if !common.RedisEnabled {
		local := limiter.SnapshotInflight()
		for _, id := range tokenIds {
			if n := local[id]; n > 0 {
				result[id] = n
			}
		}
		return result, nil
	}

	ctx := context.Background()
	// 只取仍在上报窗口内的实例。过期成员由上报侧裁剪，这里再按 score 过滤一次，
	// 避免「所有实例都挂了、没人来裁剪」时读到一堆陈旧 id。
	cutoff := strconv.FormatFloat(
		float64(time.Now().Unix())-inflightKeyTTL.Seconds(), 'f', -1, 64)
	instanceIds, err := common.RDB.ZRangeByScore(ctx, inflightRegistryKey, &redis.ZRangeBy{
		Min: cutoff,
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}
	if len(instanceIds) == 0 {
		return result, nil
	}

	fields := make([]string, 0, len(tokenIds))
	for _, id := range tokenIds {
		fields = append(fields, strconv.Itoa(id))
	}

	pipe := common.RDB.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(instanceIds))
	for _, instanceId := range instanceIds {
		cmds = append(cmds, pipe.HMGet(ctx, inflightNodeKey(instanceId), fields...))
	}
	// 不因单个命令出错就整体失败：go-redis 的 Pipeline.Exec 返回的是「第一个出错
	// 命令的 error」（redis.go: generalProcessPipeline → cmdsFirstErr），而某个
	// 实例的 key 在 TTL 边界上消失是完全正常的。这里忽略 Exec 的返回值，改为逐
	// 命令判断 —— 个别实例读失败时，其余实例的数据仍然可用，展示值只是偏小而不
	// 是整列报错。
	//
	// 连接级故障（Redis 挂了）会让所有命令都带上同一个 error，下面的循环全部
	// continue，最终返回空 map —— 对纯展示数据而言，「显示 0」比「整个列表报错」
	// 更合适。
	_, _ = pipe.Exec(ctx)

	for _, cmd := range cmds {
		vals, cmdErr := cmd.Result()
		if cmdErr != nil {
			continue // 该实例的 key 恰好在 TTL 边界消失，或连接故障，跳过
		}
		for i, v := range vals {
			if i >= len(tokenIds) || v == nil {
				continue
			}
			s, ok := v.(string)
			if !ok {
				continue
			}
			n, convErr := strconv.Atoi(s)
			if convErr != nil || n <= 0 {
				continue
			}
			result[tokenIds[i]] += n
		}
	}
	return result, nil
}

// InflightIsClusterWide 报告在途数是否为全集群口径。
//
// 无 Redis 时并发限制本身就是「每实例各算一份」的近似（见
// common/limiter/concurrency.go 的注释），在途数同理。前端据此提示用户，
// 避免把单实例数字当成全局真相。
func InflightIsClusterWide() bool {
	return common.RedisEnabled
}
