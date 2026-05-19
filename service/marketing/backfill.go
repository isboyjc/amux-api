package marketing

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Backfill 用于"启用 Resend 之前已经存在的付费用户"的一次性手工同步。
//
// 为什么不复用事件总线？
//   - 历史数据可能上万行；如果灌成事件，会污染 event_log（增加管理 UI 的噪音、
//     占用 retention 窗口、被 logger 等订阅者重复处理）
//   - Provider.Sync 本就幂等；直接调用更短路径，重跑只会刷新到当前状态
//   - 失败的逐条记录到日志即可，admin 看 SysError 自行排查；不需要 dead 队列
//
// 互斥：同一进程同一时刻只允许一个 Backfill 在跑（防误触双重提交）。
// 多实例部署下不互斥 —— Provider.Sync 是幂等的，最坏情况两实例对同一用户调两次 Resend，结果一致。

// BackfillOpts 控制并发与批次大小。
type BackfillOpts struct {
	// Concurrency 同时调 Provider.Sync 的 goroutine 数。
	// 默认 2 —— 配合 Provider.Sync 每次 3-4 个 HTTP 调用，已能稳定塞满 Resend 免费档（2 req/s）
	// 不会触发限流。如果是付费档可以调到 4-6。
	Concurrency int

	// BatchSize 每次从 DB 拉取的用户 id 数。默认 200。
	BatchSize int

	// SyncTimeout 单次 Provider.Sync 的上下文超时。默认 30s。
	SyncTimeout time.Duration
}

func (o *BackfillOpts) setDefaults() {
	if o.Concurrency <= 0 {
		o.Concurrency = 2
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 200
	}
	if o.SyncTimeout <= 0 {
		o.SyncTimeout = 30 * time.Second
	}
}

// BackfillResult 一次回填任务的结果汇总。
type BackfillResult struct {
	Total      int    `json:"total"`       // 候选用户总数
	Synced     int    `json:"synced"`      // Provider.Sync 成功
	Skipped    int    `json:"skipped"`     // Resolve 返回 nil（如 TierNone、无邮箱）
	Failed     int    `json:"failed"`      // Provider.Sync 失败
	StartedAt  int64  `json:"started_at"`  // unix 秒
	FinishedAt int64  `json:"finished_at"` // unix 秒；0 表示还在跑
	LastError  string `json:"last_error,omitempty"`
}

var (
	backfillRunning atomic.Bool
	backfillResult  atomic.Pointer[BackfillResult] // 仅记录最近一次结果
)

// IsBackfillRunning 当前是否有回填任务在跑。
func IsBackfillRunning() bool { return backfillRunning.Load() }

// LastBackfillResult 上一次回填的结果（可能为 nil）。
func LastBackfillResult() *BackfillResult { return backfillResult.Load() }

// 哨兵错误：Provider 未注入 / 已有任务运行。caller 可用 errors.Is 区分。
var (
	ErrProviderNotConfigured = errors.New("marketing: provider not configured")
	ErrBackfillRunning       = errors.New("marketing: backfill already running")
)

// Backfill 同步当前所有付费用户到 Provider。
//
// 调用前：必须已注入 Provider（MarketingEnabled=true 且配置完整）。
// 阻塞调用：在 caller 选择的 goroutine 内跑完整个流程。caller 一般在 background goroutine
// 里调；HTTP handler 应立即返回 "已开始" 给用户。
func Backfill(ctx context.Context, opts BackfillOpts) (*BackfillResult, error) {
	provider := CurrentProvider()
	if provider == nil {
		return nil, ErrProviderNotConfigured
	}
	if !backfillRunning.CompareAndSwap(false, true) {
		return nil, ErrBackfillRunning
	}
	defer backfillRunning.Store(false)

	opts.setDefaults()
	result := &BackfillResult{StartedAt: time.Now().Unix()}
	backfillResult.Store(result) // 中间状态也能查（FinishedAt=0 表示运行中）

	common.SysLog("[marketing] backfill started")

	// 并发控制：bounded worker pool
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	var (
		syncedAtomic  atomic.Int32
		skippedAtomic atomic.Int32
		failedAtomic  atomic.Int32
		totalAtomic   atomic.Int32
		lastErrMu     sync.Mutex
		lastErr       string
	)

	processOne := func(userId int) {
		defer func() {
			<-sem
			wg.Done()
		}()
		if ctx.Err() != nil {
			return
		}
		// 用 ctx + 超时构造每个 Sync 的 context
		syncCtx, cancel := context.WithTimeout(ctx, opts.SyncTimeout)
		defer cancel()

		intent, err := resolveForUser(syncCtx, userId)
		if err != nil {
			failedAtomic.Add(1)
			lastErrMu.Lock()
			lastErr = err.Error()
			lastErrMu.Unlock()
			common.SysError("[marketing] backfill resolve failed userId=" + strconv.Itoa(userId) + ": " + err.Error())
			return
		}
		if intent == nil {
			skippedAtomic.Add(1)
			return
		}
		if err := provider.Sync(syncCtx, *intent); err != nil {
			failedAtomic.Add(1)
			lastErrMu.Lock()
			lastErr = err.Error()
			lastErrMu.Unlock()
			common.SysError("[marketing] backfill sync failed userId=" + strconv.Itoa(userId) + " email=" + intent.TargetEmail + ": " + err.Error())
			return
		}
		syncedAtomic.Add(1)
	}

	// 游标分页 + 喂入 worker pool
	afterID := 0
	for {
		if ctx.Err() != nil {
			break
		}
		ids, err := model.GetPaidUserIDsBatch(afterID, opts.BatchSize)
		if err != nil {
			result.LastError = "query users: " + err.Error()
			common.SysError("[marketing] backfill query failed: " + err.Error())
			break
		}
		if len(ids) == 0 {
			break
		}
		totalAtomic.Add(int32(len(ids)))
		for _, id := range ids {
			wg.Add(1)
			sem <- struct{}{}
			uid := id
			go processOne(uid)
			if id > afterID {
				afterID = id
			}
		}
		// 实时回写中间进度，方便 status API 观察
		result.Total = int(totalAtomic.Load())
		result.Synced = int(syncedAtomic.Load())
		result.Skipped = int(skippedAtomic.Load())
		result.Failed = int(failedAtomic.Load())
		backfillResult.Store(result)
	}
	wg.Wait()

	finalResult := &BackfillResult{
		Total:      int(totalAtomic.Load()),
		Synced:     int(syncedAtomic.Load()),
		Skipped:    int(skippedAtomic.Load()),
		Failed:     int(failedAtomic.Load()),
		StartedAt:  result.StartedAt,
		FinishedAt: time.Now().Unix(),
	}
	lastErrMu.Lock()
	finalResult.LastError = lastErr
	if result.LastError != "" {
		finalResult.LastError = result.LastError
	}
	lastErrMu.Unlock()
	backfillResult.Store(finalResult)

	common.SysLog("[marketing] backfill finished: " +
		"total=" + strconv.Itoa(finalResult.Total) +
		" synced=" + strconv.Itoa(finalResult.Synced) +
		" skipped=" + strconv.Itoa(finalResult.Skipped) +
		" failed=" + strconv.Itoa(finalResult.Failed))

	return finalResult, nil
}

// resolveForUser 跟 Resolve 共用业务规则，但跳过事件相关分支（user.deleted / email.bound）。
// 直接按"当前 DB 状态"产出 Intent。
func resolveForUser(ctx context.Context, userId int) (*Intent, error) {
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Email == "" {
		return nil, nil
	}
	tier, err := tierForUser(user)
	if err != nil {
		return nil, err
	}
	if tier == TierNone {
		return nil, nil
	}
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	return &Intent{
		TargetEmail: user.Email,
		DisplayName: displayName,
		Tier:        tier,
	}, nil
}

