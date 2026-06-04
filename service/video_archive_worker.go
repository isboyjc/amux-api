package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/storage"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 视频归档 worker 池：把"上游视频直链 → 下载 → 上传 R2"这个重活从单线程的
// 任务轮询循环里搬出来，避免大视频下载阻塞整条轮询。
//
// 并发模型前提：轮询只跑在主节点（main.go: IsMasterNode 守卫），所以归档也
// 只在主节点启动，进程内就是单一入队方。在途去重用进程内 sync.Map 即可，
// 不需要跨进程锁。若将来扩成多主节点，需改用 DB 中间状态 + CAS 认领。
const (
	archiveQueueSize      = 256
	defaultArchiveWorkers = 3
	maxArchiveAttempts    = 3
)

type archiveJob struct {
	taskID      string // 对外公开 task_id
	channelID   int
	platform    constant.TaskPlatform
	upstreamURL string               // 上游原始视频直链
	taskResult  relaycommon.TaskInfo // 结算所需的快照（token 用量等）
}

var (
	archiveJobChan   chan archiveJob
	archiveInflight  sync.Map // taskID(string) -> struct{}，防止同一任务重复入队
	archiveStartOnce sync.Once
)

// StartVideoArchiveWorkers 启动视频归档 worker 池。仅应在主节点调用一次
// （sync.Once 兜底，重复调用安全）。
func StartVideoArchiveWorkers(workers int) {
	archiveStartOnce.Do(func() {
		if workers <= 0 {
			workers = defaultArchiveWorkers
		}
		archiveJobChan = make(chan archiveJob, archiveQueueSize)
		for i := 0; i < workers; i++ {
			go archiveWorkerLoop()
		}
		common.SysLog(fmt.Sprintf("video archive workers started: %d", workers))
	})
}

func archiveWorkerLoop() {
	for job := range archiveJobChan {
		processArchiveJob(job)
	}
}

// ShouldArchiveVideo 判断一个上游结果 URL 是否应走 R2 归档：
//   - 必须是 http(s) 直链（data:/asset:// 之类内联结果不归档）；
//   - storage 总开关 + 凭证齐（storage.IsEnabled()）；
//   - 视频归档开关打开（StorageSettings.VideoArchiveEnabled）。
func ShouldArchiveVideo(url string) bool {
	if url == "" {
		return false
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	if !storage.IsEnabled() {
		return false
	}
	if !system_setting.GetStorageSettings().VideoArchiveEnabled {
		return false
	}
	return true
}

// EnqueueVideoArchive 尝试把一个视频归档任务入队。
//
// 返回值语义（调用方据此决定是否落终态）：
//   - true：已交给归档流程（已入队或已在途）。调用方应把任务保持为"处理中"，
//     不要落 SUCCESS、不要结算——最终由 worker 在 R2 就绪后翻 SUCCESS。
//   - false：入队失败（worker 未启动 / 队列已满）。调用方应按原逻辑落 SUCCESS
//     （用脱敏后的上游 URL），不阻塞；下一轮轮询不会再进来（已是终态）。
func EnqueueVideoArchive(job archiveJob) bool {
	if archiveJobChan == nil {
		return false // workers 未启动（非主节点 / 未初始化）
	}
	// 在途去重：同一 taskID 同时只允许一个归档任务。已在途则视为"已交给归档"，
	// 让调用方继续保持处理中，由现存任务负责收尾。
	if _, loaded := archiveInflight.LoadOrStore(job.taskID, struct{}{}); loaded {
		return true
	}
	select {
	case archiveJobChan <- job:
		return true
	default:
		// 队列满：撤销在途占位，让调用方走降级落终态，避免任务卡死
		archiveInflight.Delete(job.taskID)
		return false
	}
}

// TryEnqueueVideoArchive 是归档入队的统一入口：先判断该结果 URL 是否该归档
// （ShouldArchiveVideo），再尝试入队。
//
// 返回 true 表示"归档已接管"——调用方应把任务保持为处理中、不要落 SUCCESS、
// 不要结算，最终由 worker 翻 SUCCESS。返回 false 表示不归档（未开启 / 非 http
// 直链 / worker 未启动 / 队列满），调用方按原逻辑落终态。
//
// 后台轮询(updateVideoSingleTask)与实时查询(relay.tryRealtimeFetch)两条终态
// 路径共用此入口，保证行为一致，不会有一条绕过归档直接暴露上游 URL。
func TryEnqueueVideoArchive(taskID string, channelID int, platform constant.TaskPlatform, upstreamURL string, taskResult *relaycommon.TaskInfo) bool {
	if !ShouldArchiveVideo(upstreamURL) {
		return false
	}
	var tr relaycommon.TaskInfo
	if taskResult != nil {
		tr = *taskResult
	}
	return EnqueueVideoArchive(archiveJob{
		taskID:      taskID,
		channelID:   channelID,
		platform:    platform,
		upstreamURL: upstreamURL,
		taskResult:  tr,
	})
}

func processArchiveJob(job archiveJob) {
	defer archiveInflight.Delete(job.taskID)
	ctx := context.Background()

	task, exist, err := model.GetByOnlyTaskId(job.taskID)
	if err != nil || !exist || task == nil {
		logger.LogError(ctx, fmt.Sprintf("archive: load task %s failed: exist=%v err=%v", job.taskID, exist, err))
		return
	}
	// 已被其它流程（如超时清理）落终态：放弃归档，避免覆盖
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		logger.LogInfo(ctx, fmt.Sprintf("archive: task %s already finalized (%s), skip", job.taskID, task.Status))
		return
	}

	// 归档 key 需要 userID + 提交时间分目录，从已 load 的 task 取（此处 task
	// 还是顶部那次 load，尚未被下面的 finalize 前 reload 覆盖）。
	userID := task.UserId
	submitTime := task.SubmitTime

	// 下载 + 上传 R2，带重试
	var r2url string
	var archErr error
	for attempt := 1; attempt <= maxArchiveAttempts; attempt++ {
		r2url, archErr = storage.ArchiveVideoToR2(ctx, job.taskID, userID, submitTime, job.upstreamURL)
		if archErr == nil {
			break
		}
		logger.LogError(ctx, fmt.Sprintf("archive: task %s attempt %d/%d failed: %v", job.taskID, attempt, maxArchiveAttempts, archErr))
		if attempt < maxArchiveAttempts {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}

	finalURL := r2url
	if archErr != nil {
		// 降级：回退到脱敏后的上游 URL，保证用户仍能拿到视频（只是没存永久副本）
		finalURL = operation_setting.ApplyTaskURLRewrite(job.upstreamURL)
		logger.LogWarn(ctx, fmt.Sprintf("archive: task %s archiving failed after %d attempts, fallback to proxied upstream url", job.taskID, maxArchiveAttempts))
	}

	// 归档耗时数秒，期间轮询循环可能刷新过 task.Data/StartTime 等列。落终态前
	// 重新 load 一份最新的，只覆盖我们关心的 4 个字段，避免用陈旧副本回写（如把
	// StartTime 写回 0）。结算也用这份最新的 task（BillingContext/Quota 更准）。
	task, exist, err = model.GetByOnlyTaskId(job.taskID)
	if err != nil || !exist || task == nil {
		logger.LogError(ctx, fmt.Sprintf("archive: reload task %s before finalize failed: exist=%v err=%v", job.taskID, exist, err))
		return
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		logger.LogInfo(ctx, fmt.Sprintf("archive: task %s finalized by another process before archive done (%s), skip", job.taskID, task.Status))
		return
	}

	// 翻成 SUCCESS：CAS from IN_PROGRESS，保证只有一次终态转换触发结算/回调。
	// 若此刻 DB 还未被轮询提交成 IN_PROGRESS（极少见的时序竞争），CAS 命中 0 行、
	// won=false，本次跳过；任务仍 != 100% 会被下一轮轮询重新入队，自愈续传。
	now := time.Now().Unix()
	task.Status = model.TaskStatusSuccess
	task.Progress = taskcommon.ProgressComplete
	if task.FinishTime == 0 {
		task.FinishTime = now
	}
	task.PrivateData.ResultURL = finalURL

	won, err := task.UpdateWithStatus(model.TaskStatusInProgress)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("archive: finalize task %s CAS update error: %v", job.taskID, err))
		return
	}
	if !won {
		logger.LogWarn(ctx, fmt.Sprintf("archive: task %s already transitioned by another process, skip settle", job.taskID))
		return
	}

	// 结算 + 回调，镜像轮询循环的终态处理
	if adaptor := GetTaskAdaptorFunc(job.platform); adaptor != nil {
		if ch, cerr := model.CacheGetChannel(job.channelID); cerr == nil {
			info := &relaycommon.RelayInfo{}
			info.ChannelMeta = &relaycommon.ChannelMeta{ChannelBaseUrl: ch.GetBaseURL()}
			info.ApiKey = ch.Key
			adaptor.Init(info)
		}
		tr := job.taskResult
		settleTaskBillingOnComplete(ctx, adaptor, task, &tr)
	}
	NotifyTaskCallback(ctx, task)

	logger.LogInfo(ctx, fmt.Sprintf("archive: task %s finalized, result_url=%s archived=%t", job.taskID, finalURL, archErr == nil))
}
