package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// TaskCallbackPayload 任务终态回调的 payload
type TaskCallbackPayload struct {
	TaskID     string          `json:"task_id"`
	Status     string          `json:"status"`
	Progress   string          `json:"progress"`
	FailReason string          `json:"fail_reason,omitempty"`
	ResultURL  string          `json:"result_url,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	FinishTime int64           `json:"finish_time"`
	Timestamp  int64           `json:"timestamp"`
}

// NotifyTaskCallback 在任务到达终态时异步发送回调通知。
// 如果 task 没有配置 CallbackURL 则直接返回。
func NotifyTaskCallback(ctx context.Context, task *model.Task) {
	callbackURL := task.PrivateData.CallbackURL
	if callbackURL == "" {
		return
	}

	payload := TaskCallbackPayload{
		TaskID:     task.TaskID,
		Status:     string(task.Status),
		Progress:   task.Progress,
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		Data:       task.Data,
		FinishTime: task.FinishTime,
		Timestamp:  time.Now().Unix(),
	}

	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("task callback marshal failed for %s: %v", task.TaskID, err))
		return
	}

	go sendTaskCallback(ctx, task.TaskID, callbackURL, task.PrivateData.CallbackSecret, payloadBytes)
}

func sendTaskCallback(ctx context.Context, taskID, callbackURL, secret string, payloadBytes []byte) {
	if system_setting.EnableWorker() {
		workerReq := &WorkerRequest{
			URL:    callbackURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: payloadBytes,
		}
		if secret != "" {
			sig := generateSignature(secret, payloadBytes)
			workerReq.Headers["X-Webhook-Signature"] = sig
			workerReq.Headers["Authorization"] = "Bearer " + secret
		}
		resp, err := DoWorkerRequest(workerReq)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("task callback worker request failed for %s: %v", taskID, err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.LogWarn(ctx, fmt.Sprintf("task callback for %s returned status %d", taskID, resp.StatusCode))
		}
		return
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(callbackURL,
		fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode,
		fetchSetting.DomainList, fetchSetting.IpList,
		fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		logger.LogError(ctx, fmt.Sprintf("task callback SSRF rejected for %s: %v", taskID, err))
		return
	}

	req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(payloadBytes))
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("task callback create request failed for %s: %v", taskID, err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		sig := generateSignature(secret, payloadBytes)
		req.Header.Set("X-Webhook-Signature", sig)
	}

	client := GetHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("task callback request failed for %s: %v", taskID, err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.LogWarn(ctx, fmt.Sprintf("task callback for %s returned status %d", taskID, resp.StatusCode))
	}
}
