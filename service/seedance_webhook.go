package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// webhookPollGraceExpired 判断一个 webhook 模式任务是否已过轮询宽限期
// （constant.TaskWebhookPollGraceMinutes，环境变量 TASK_WEBHOOK_POLL_GRACE_MINUTES）。
//
// 宽限期内信任上游回调，轮询循环不去打扰任务；超过之后仍未终态的任务重新纳入
// 轮询，作为「上游没回调 / 回调丢了 / 归档 worker 收尾失败」的自愈路径。
// 宽限期从提交时刻起算；SubmitTime 缺失（历史数据）时按已过期处理——宁可多轮询
// 一次，也不要让任务永远没人管。配成 0 则 webhook 任务照常轮询（逃生开关）。
func webhookPollGraceExpired(task *model.Task) bool {
	if task.SubmitTime <= 0 {
		return true
	}
	if constant.TaskWebhookPollGraceMinutes <= 0 {
		return true
	}
	return time.Now().Unix()-task.SubmitTime >= int64(constant.TaskWebhookPollGraceMinutes)*60
}

// HandleTaskWebhookCallback 处理上游异步回调推送的任务状态事件（Seedance webhook 模式）。
//
// 与轮询循环（updateVideoSingleTask）共用同一套终态处理——R2 归档 / 差额结算 /
// 退款 / 下游回调——保证两条路径行为一致，不会有一条路径绕过归档或计费。
//
// 返回值语义（controller 据此定应答码）：
//   - true：本次回调已被接受并落库，应回 2xx。中间态回调、已终态后的重复回调
//     也回 2xx（幂等，重复推送不产生副作用）。
//   - false：回调无法处理（解析失败 / 未知状态 / CAS 失败），应回非 2xx，让上游
//     按重试策略（ZeroCut 3次/15s，Ark 3次）再推。
func HandleTaskWebhookCallback(ctx context.Context, task *model.Task, adaptor TaskPollingAdaptor, respBody []byte) bool {
	if task == nil || adaptor == nil {
		return false
	}

	// 终态已落：上游重复推送终态（或轮询/归档 worker 抢先推进），幂等回 2xx。
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		logger.LogInfo(ctx, fmt.Sprintf("webhook: task %s already finalized (%s), ack idempotently", task.TaskID, task.Status))
		return true
	}

	taskResult, err := adaptor.ParseTaskResult(respBody)
	if err != nil || taskResult == nil {
		logger.LogError(ctx, fmt.Sprintf("webhook: parse task result failed for %s: %v", task.TaskID, err))
		return false
	}
	if taskResult.Status == "" {
		// 空状态：上游可能回了错误体（网关地址不可达时的错误页等），无法处理。
		logger.LogError(ctx, fmt.Sprintf("webhook: task %s returned empty status, response: %s", task.TaskID, string(respBody)))
		return false
	}

	// 写回原始响应（脱敏），供查询接口/审计使用。
	task.Data = redactVideoResponseBody(respBody)

	now := time.Now().Unix()
	snap := task.Snapshot()
	shouldRefund := false
	shouldSettle := false
	archiving := false

	switch taskResult.Status {
	case model.TaskStatusSubmitted, model.TaskStatusQueued:
		// 中间态：只推进度，不结算。
		task.Status = model.TaskStatus(taskResult.Status)
	case model.TaskStatusInProgress:
		task.Status = model.TaskStatusInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		// R2 归档：与轮询一致，若开启且结果是可下载的 http(s) 直链，先不落终态——
		// 交给后台 worker 下载上传 R2，成功后由 worker 翻 SUCCESS 并回调下游。
		// 必须看 TryEnqueueVideoArchive 的入队结果：入队失败（worker 未启动 /
		// 队列满 / 非 http 直链）时若仍停在 99%，任务要等到宽限期后的轮询才会被
		// 捡回来，白等一段时间；直接降级走普通 SUCCESS 终态、用脱敏后的上游 URL
		// 收尾更干净。（入队成功但 worker 收尾 CAS 失败的情况没有本地信号，
		// 由 webhookPollGraceExpired 之后恢复的轮询兜底。）
		if TryEnqueueVideoArchive(task.TaskID, task.ChannelId, task.Platform, taskResult.Url, taskResult) {
			archiving = true
			if task.StartTime == 0 {
				task.StartTime = now
			}
			break
		}
		task.Status = model.TaskStatusSuccess
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if strings.HasPrefix(taskResult.Url, "data:") {
			// data: URI 内联结果：用代理 URL 占位，视频本体留在 Data 字段。
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		} else if taskResult.Url != "" {
			// 上游直链经过管理员配置的前缀脱敏，不直接暴露到用户可见的 URL。
			task.PrivateData.ResultURL = operation_setting.ApplyTaskURLRewrite(taskResult.Url)
		} else {
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		if task.Quota != 0 {
			shouldRefund = true
		}
	default:
		// 未知状态：不落库、不回 2xx，让上游重试或交给超时兜底。ParseTaskResult
		// 已对未知状态返回错误、提前 return false，此处仅作防御。
		logger.LogWarn(ctx, fmt.Sprintf("webhook: task %s unknown status %q", task.TaskID, taskResult.Status))
		return false
	}

	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}

	if archiving {
		// 归档进行中：压回"处理中 + 99%"，避免 isDone 误判终态触发结算。
		task.Status = model.TaskStatusInProgress
		task.Progress = taskcommon.ProgressArchiving
		task.FinishTime = 0
	}

	isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if isDone && snap.Status != task.Status {
		// 终态迁移：CAS from 当前 DB 状态，保证只有一次终态转换触发结算/回调。
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("webhook: UpdateWithStatus failed for task %s: %s", task.TaskID, err.Error()))
			return false
		}
		if !won {
			// 并发回调抢先推进了 DB 状态（如先到的一条 RUNNING 已把 queued→
			// in_progress）。不能直接 ack——那会丢掉这次终态：宽限期内没有轮询
			// 自愈，SUCCESS 的差额结算就要一直等到宽限期后才可能补上。重新读取
			// DB 判断归属：
			//   - DB 已是终态：另一条路径已完成终态转换/结算/回调，本次幂等 ack；
			//   - DB 仍非终态：从最新状态重试一次终态 CAS，保证终态（含结算/回调）
			//     落库。残余竞争（重试期间又被抢先）则拒收，交给上游重试兜底。
			fresh, exist, err := model.GetByOnlyTaskId(task.TaskID)
			if err != nil || !exist || fresh == nil {
				logger.LogError(ctx, fmt.Sprintf("webhook: reload task %s after lost CAS failed: exist=%v err=%v", task.TaskID, exist, err))
				return false
			}
			if fresh.Status == model.TaskStatusSuccess || fresh.Status == model.TaskStatusFailure {
				logger.LogWarn(ctx, fmt.Sprintf("webhook: task %s already finalized by another process (%s), ack idempotently", task.TaskID, fresh.Status))
				return true
			}
			logger.LogWarn(ctx, fmt.Sprintf("webhook: task %s CAS from %s lost, retry terminal CAS from fresh status %s", task.TaskID, snap.Status, fresh.Status))
			won, err = task.UpdateWithStatus(fresh.Status)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("webhook: retry UpdateWithStatus failed for task %s: %s", task.TaskID, err.Error()))
				return false
			}
			if !won {
				logger.LogError(ctx, fmt.Sprintf("webhook: retry terminal CAS still lost for task %s, reject for upstream retry", task.TaskID))
				return false
			}
		}
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			logger.LogError(ctx, fmt.Sprintf("webhook: update task %s failed: %s", task.TaskID, err.Error()))
			return false
		}
	} else {
		logger.LogDebug(ctx, fmt.Sprintf("webhook: no change for task %s", task.TaskID))
	}

	if shouldSettle {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
	if isDone && (shouldSettle || shouldRefund) {
		NotifyTaskCallback(ctx, task)
	}

	logger.LogInfo(ctx, fmt.Sprintf("webhook: task %s updated to %s (progress %s)", task.TaskID, task.Status, task.Progress))
	return true
}
