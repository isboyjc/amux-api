package controller

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// SeedanceTaskWebhook 接收上游（官方 Ark / ZeroCut）对 Seedance 任务的异步回调。
//
// 任务寻址：优先用 URL 路径里的 task_id（网关公开 ID，构建请求体时设给上游的
// callback_url 自带）；缺失时从回调 payload 里读 trace_id 兜底（方案 A——ZeroCut
// 会把透传的 trace_id 原样带回回调体）。
//
// 应答语义（对应上游重试策略）：
//   - 处理成功（含中间态进度、已终态后的重复推送、任务未走 webhook 模式）→ 2xx，
//     上游停止重试；
//   - 重试可能救回来的（任务尚未落库 / 解析失败 / 未知状态 / CAS 竞争）→ 5xx，
//     上游按重试策略再推；
//   - 签名不符 → 404，不给出任何任务是否存在的信息。
//
// 安全说明：本端点无 TokenAuth，但要求回调请求携带 HMAC 签名。构建回调地址时
// 网关在 query 里拼了 sig=HMAC(SeedanceWebhookSecret, taskID)，只有拿到该回调
// 地址的上游能回带正确签名；仅知道公开 task_id 的任务调用者无法伪造（伪造
// FAILURE 全额退款 / 伪造 SUCCESS 篡改用量），签名不符直接 404。签名校验用
// constant-time 比较，避免时序侧信道。若全局签名密钥被清空，则任何请求都无法
// 通过校验，宁可不接收回调也不降级为无鉴权。
func SeedanceTaskWebhook(c *gin.Context) {
	taskID := c.Param("task_id")
	respBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c, "seedance webhook: read body failed: "+err.Error())
		c.Status(http.StatusBadRequest)
		return
	}
	if taskID == "" {
		taskID = extractTaskIDFromWebhookBody(respBody)
	}
	if taskID == "" {
		logger.LogWarn(c, "seedance webhook: no task_id in url path nor trace_id in body")
		c.Status(http.StatusNotFound)
		return
	}

	// HMAC 签名校验：回调地址自带 sig，只有上游能正确回带。
	if !verifySeedanceWebhookSig(taskID, c.Query("sig")) {
		logger.LogWarn(c, fmt.Sprintf("seedance webhook: task %s has invalid or missing sig, reject", taskID))
		c.Status(http.StatusNotFound)
		return
	}

	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil || !exist || task == nil {
		// 签名已校验通过，说明这个 task_id 确实是本网关签发的——找不到行不是伪造，
		// 而是竞态：RelayTask 的 task.Insert() 发生在上游提交响应返回之后，快失败
		// 的任务其回调可能先于落库到达。必须回可重试的 503（而不是 404），否则
		// 上游按「已投递」处理，任务就再也等不到回调、只能卡到超时。
		logger.LogWarn(c, fmt.Sprintf("seedance webhook: task %s not yet visible (exist=%v err=%v), ask upstream to retry", taskID, exist, err))
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if !task.PrivateData.WebhookMode {
		// 任务存在但没走 webhook 模式：轮询会负责推进它，回调无事可做。回 2xx
		// 让上游停止重试——重试不会改变任何结果，只是白白重推。
		logger.LogWarn(c, fmt.Sprintf("seedance webhook: task %s is not in webhook mode, ignore callback", task.TaskID))
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ignored"})
		return
	}

	adaptor := service.GetTaskAdaptorFunc(task.Platform)
	if adaptor == nil {
		logger.LogError(c, fmt.Sprintf("seedance webhook: no adaptor for platform %s", task.Platform))
		c.Status(http.StatusInternalServerError)
		return
	}

	if !service.HandleTaskWebhookCallback(c, task, adaptor, respBody) {
		logger.LogWarn(c, fmt.Sprintf("seedance webhook: handler rejected callback for task %s, upstream will retry", task.TaskID))
		c.Status(http.StatusServiceUnavailable)
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// verifySeedanceWebhookSig 校验回调请求携带的 HMAC 签名。sig 由构造回调地址时用
// HMAC(SeedanceWebhookSecret, taskID) 生成，只有持有该密钥的上游能正确回带。
// 用 constant-time 比较避免时序侧信道。
func verifySeedanceWebhookSig(taskID, sig string) bool {
	if system_setting.SeedanceWebhookSecret == "" || taskID == "" || sig == "" {
		return false
	}
	expected := common.GenerateHMACWithKey([]byte(system_setting.SeedanceWebhookSecret), taskID)
	return subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1
}

// extractTaskIDFromWebhookBody 从回调 payload 里提取透传的 trace_id（方案 A）。
// ZeroCut 回调结构顶层与 data 下都可能带 trace_id，两处都读一遍兼容。
func extractTaskIDFromWebhookBody(body []byte) string {
	var raw struct {
		TraceID string `json:"trace_id"`
		Data    struct {
			TraceID string `json:"trace_id"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &raw); err != nil {
		return ""
	}
	if raw.TraceID != "" {
		return raw.TraceID
	}
	return raw.Data.TraceID
}
