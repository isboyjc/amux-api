package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/events"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

// 设计见 docs/event-system-design.md 第 9 节"数据保留策略"。
//
//   - event_dispatch.done 行：保留 30 天
//   - event_dispatch.dead 行：永久保留（量极小，需人工干预）
//   - event_log 行：默认 365 天（可配置 0 = 永不删），且仅删除
//     已不被 dispatch (pending/processing/dead) 引用的行，避免孤儿
//
// 仅在 master 节点运行；批间 sleep 避免大表锁竞争。

var (
	eventCleanupOnce     sync.Once
	eventCleanupRunning  atomic.Bool
	eventCleanupLastDate atomic.Int64 // 已执行过清理的"日期"，格式 YYYYMMDD（UTC）
)

const eventCleanupBatchSleep = 100 * time.Millisecond

// StartEventCleanupTask 启动事件清理后台任务。
// 每分钟检查一次"今天是否到了 EventCleanupHourUTC 且尚未跑过"，是则触发一次清理。
func StartEventCleanupTask() {
	eventCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			common.SysLog(fmt.Sprintf("[events] cleanup task started: daily at %02d:00 UTC",
				operation_setting.EventCleanupHourUTC))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				maybeRunEventCleanup()
			}
		})
	})
}

func maybeRunEventCleanup() {
	now := time.Now().UTC()
	if now.Hour() != operation_setting.EventCleanupHourUTC {
		return
	}
	today := int64(now.Year()*10000 + int(now.Month())*100 + now.Day())
	if eventCleanupLastDate.Load() == today {
		return
	}
	if !eventCleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer eventCleanupRunning.Store(false)

	runEventCleanupOnce(context.Background())
	eventCleanupLastDate.Store(today)
}

func runEventCleanupOnce(ctx context.Context) {
	start := time.Now()
	batch := operation_setting.EventCleanupBatchSize
	if batch <= 0 {
		batch = 1000
	}

	// 1. 清 done dispatch
	doneCutoff := time.Now().Unix() - int64(operation_setting.EventDispatchDoneRetentionDays)*86400
	doneDeleted := deleteInBatches(ctx, "event_dispatch.done", func() (int64, error) {
		return events.DeleteDoneDispatchesBatch(doneCutoff, batch)
	})

	// 2. 清旧 event_log（仅当配置了正数保留天数）
	var logDeleted int64
	if days := operation_setting.EventLogRetentionDays; days > 0 {
		logCutoff := time.Now().Unix() - int64(days)*86400
		logDeleted = deleteInBatches(ctx, "event_log", func() (int64, error) {
			return events.DeleteEventLogsBatch(logCutoff, batch)
		})
	}

	common.SysLog(fmt.Sprintf("[events] cleanup finished: dispatch_done_deleted=%d event_log_deleted=%d elapsed=%s",
		doneDeleted, logDeleted, time.Since(start).Truncate(time.Millisecond)))
}

// deleteInBatches 反复调 fn 删除一批，直到 fn 返回 0 或 ctx 取消；批间 sleep 让出锁。
func deleteInBatches(ctx context.Context, label string, fn func() (int64, error)) int64 {
	var total int64
	for {
		if ctx.Err() != nil {
			return total
		}
		n, err := fn()
		if err != nil {
			common.SysError(fmt.Sprintf("[events] cleanup %s batch failed: %v", label, err))
			return total
		}
		total += n
		if n == 0 {
			return total
		}
		select {
		case <-ctx.Done():
			return total
		case <-time.After(eventCleanupBatchSleep):
		}
	}
}
