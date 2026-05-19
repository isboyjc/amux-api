package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// 重试退避时间表。retry_count = N 时下次重试在 retrySchedule[min(N, len-1)] 之后。
// 6 次失败后标记 dead。
var retrySchedule = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
}

const maxRetries = 6

// 启动时回收"processing 且 updated_at < now - stuckThreshold"的同 worker 残留行。
// 必须大于 handleTimeout，避免误回收正在处理的任务。
const stuckThreshold = 5 * time.Minute

// WorkerOpts worker 启动参数。
type WorkerOpts struct {
	PollInterval  time.Duration // 默认 2s
	BatchSize     int           // 默认 50
	Concurrency   int           // 默认 4
	HandleTimeout time.Duration // 单次 Handle 上下文超时，默认 30s
	WorkerId      string        // 进程启动时生成的唯一 id（如 uuid）
}

func (o *WorkerOpts) setDefaults() {
	if o.PollInterval <= 0 {
		o.PollInterval = 2 * time.Second
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 50
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	if o.HandleTimeout <= 0 {
		o.HandleTimeout = 30 * time.Second
	}
	if o.WorkerId == "" {
		o.WorkerId = "default"
	}
}

// reclaimInterval 周期性回收陈旧 processing 行的间隔。
// 不仅启动时跑，运行期也定期跑，确保某实例崩溃后其孤儿能被其他活着的实例捡走。
const reclaimInterval = 1 * time.Minute

// StartWorker 启动事件 worker。阻塞调用，通常在独立 goroutine 内执行。
// ctx 被 cancel 时优雅停机：停止拉取新任务，等待已 claim 的任务完成或上下文超时。
func StartWorker(ctx context.Context, opts WorkerOpts) {
	opts.setDefaults()

	// 启动时立即跑一次回收（也包括其他崩溃实例留下的孤儿）
	doReclaim()

	common.SysLog(fmt.Sprintf("[events] worker started: id=%s poll=%s batch=%d concurrency=%d",
		opts.WorkerId, opts.PollInterval, opts.BatchSize, opts.Concurrency))

	jobs := make(chan int64, opts.BatchSize)
	var wg sync.WaitGroup
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				processDispatch(ctx, id, opts)
			}
		}()
	}

	pollTicker := time.NewTicker(opts.PollInterval)
	defer pollTicker.Stop()
	reclaimTicker := time.NewTicker(reclaimInterval)
	defer reclaimTicker.Stop()

	pollOnce(ctx, jobs, opts)
	for {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			common.SysLog("[events] worker stopped")
			return
		case <-pollTicker.C:
			pollOnce(ctx, jobs, opts)
		case <-reclaimTicker.C:
			doReclaim()
		}
	}
}

func doReclaim() {
	staleBefore := time.Now().Add(-stuckThreshold).Unix()
	n, err := reclaimStuckDispatches(staleBefore)
	if err != nil {
		common.SysError(fmt.Sprintf("[events] reclaim stuck dispatches failed: %v", err))
		return
	}
	if n > 0 {
		common.SysLog(fmt.Sprintf("[events] reclaimed %d stuck dispatches (stale > %s)", n, stuckThreshold))
	}
}

func pollOnce(ctx context.Context, jobs chan<- int64, opts WorkerOpts) {
	ids, err := pendingDispatchIDs(time.Now().Unix(), opts.BatchSize)
	if err != nil {
		common.SysError(fmt.Sprintf("[events] poll pending failed: %v", err))
		return
	}
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		case jobs <- id:
		}
	}
}

func processDispatch(ctx context.Context, id int64, opts WorkerOpts) {
	ok, err := claimDispatch(id, opts.WorkerId)
	if err != nil {
		common.SysError(fmt.Sprintf("[events] claim dispatch id=%d failed: %v", id, err))
		return
	}
	if !ok {
		return // 被其他实例抢走
	}

	d, err := getDispatchById(id)
	if err != nil {
		common.SysError(fmt.Sprintf("[events] load dispatch id=%d failed: %v", id, err))
		return
	}

	sub, ok := LookupSubscriber(d.Subscriber)
	if !ok {
		_ = markDispatchDead(id, opts.WorkerId, d.RetryCount,
			fmt.Sprintf("subscriber %q not registered", d.Subscriber))
		common.SysError(fmt.Sprintf("[events] subscriber %q not registered, dispatch %d marked dead", d.Subscriber, id))
		return
	}

	eventType, aggregateId, payload, publishedAt, err := getEventLogPayload(d.EventId)
	if err != nil {
		_ = markDispatchDead(id, opts.WorkerId, d.RetryCount,
			fmt.Sprintf("event_log %d not found: %v", d.EventId, err))
		common.SysError(fmt.Sprintf("[events] event_log id=%d missing, dispatch %d marked dead: %v", d.EventId, id, err))
		return
	}

	handleCtx, cancel := context.WithTimeout(ctx, opts.HandleTimeout)
	defer cancel()
	handleErr := sub.Handle(handleCtx, Event{
		Id:          d.EventId,
		Type:        eventType,
		AggregateId: aggregateId,
		Payload:     []byte(payload),
		PublishedAt: publishedAt,
	})

	if handleErr == nil {
		if err := finishDispatchOK(id, opts.WorkerId); err != nil {
			common.SysError(fmt.Sprintf("[events] mark dispatch %d done failed: %v", id, err))
		}
		return
	}
	if errors.Is(handleErr, ErrPermanent) {
		if err := markDispatchDead(id, opts.WorkerId, d.RetryCount+1, handleErr.Error()); err != nil {
			common.SysError(fmt.Sprintf("[events] mark dispatch %d dead failed: %v", id, err))
		}
		common.SysError(fmt.Sprintf("[events] dispatch %d permanent failure (subscriber=%s event=%s): %v",
			id, d.Subscriber, eventType, handleErr))
		return
	}
	newRetryCount := d.RetryCount + 1
	if newRetryCount >= maxRetries {
		if err := markDispatchDead(id, opts.WorkerId, newRetryCount, handleErr.Error()); err != nil {
			common.SysError(fmt.Sprintf("[events] mark dispatch %d dead failed: %v", id, err))
		}
		common.SysError(fmt.Sprintf("[events] dispatch %d exhausted retries (subscriber=%s event=%s): %v",
			id, d.Subscriber, eventType, handleErr))
		return
	}
	delay := retrySchedule[min(newRetryCount-1, len(retrySchedule)-1)]
	nextRetry := time.Now().Add(delay).Unix()
	if err := rescheduleDispatch(id, opts.WorkerId, newRetryCount, nextRetry, handleErr.Error()); err != nil {
		common.SysError(fmt.Sprintf("[events] reschedule dispatch %d failed: %v", id, err))
	}
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("[events] dispatch %d failed (attempt %d, next retry in %s): %v",
			id, newRetryCount, delay, handleErr))
	}
}
