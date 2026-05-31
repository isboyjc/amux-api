package amux_stt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const contextKeyParsedRequest = "amux_stt_parsed_request"

// ── Request / Response DTOs ──────────────────────────────────────────

type CreateTaskRequest struct {
	AudioURL      string          `json:"audio_url,omitempty"`
	AudioBase64   string          `json:"audio_base64,omitempty"`
	AudioFilename string          `json:"audio_filename,omitempty"`
	Options       json.RawMessage `json:"options,omitempty"`
	BatchMode     bool            `json:"batch_mode,omitempty"`
	Priority      *int            `json:"priority,omitempty"`
}

type clientRequest struct {
	CreateTaskRequest
	Model          string `json:"model,omitempty"`
	CallbackURL    string `json:"callback_url,omitempty"`
	CallbackSecret string `json:"callback_secret,omitempty"`
}

type taskStatusResponse struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type mergedFetchResponse struct {
	taskStatusResponse
	Result json.RawMessage `json:"result,omitempty"`
}

type sttBilling struct {
	Amount float64 `json:"amount"`
}

type taskResultHeader struct {
	AudioDuration float64     `json:"audio_duration"`
	Billing       *sttBilling `json:"billing"`
}

// 上游 billing.amount 单位为 RMB，按固定汇率折算为 quota：amount / rmbToUSDRate × QuotaPerUnit。
const rmbToUSDRate = 6.9

// ── Adaptor ──────────────────────────────────────────────────────────

type TaskAdaptor struct {
	taskcommon.BaseBilling
	channelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.channelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req clientRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest}
	}
	if req.AudioURL == "" && req.AudioBase64 == "" {
		return &dto.TaskError{Code: "invalid_request", Message: "audio_url or audio_base64 is required", StatusCode: http.StatusBadRequest}
	}
	if req.AudioBase64 != "" && req.AudioFilename == "" {
		return &dto.TaskError{Code: "invalid_request", Message: "audio_filename is required when using audio_base64", StatusCode: http.StatusBadRequest}
	}
	c.Set(contextKeyParsedRequest, &req)
	return nil
}

// ── Billing ──────────────────────────────────────────────────────────

func (a *TaskAdaptor) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	// 提交时拿不到音频时长，固定预扣 1 小时，完成后按实际时长差额结算。
	return map[string]float64{"hours": 1}
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	result := parseTaskResult(task.Data)

	// 优先使用上游返回的实际金额（RMB），按固定汇率折算 quota，不再乘 GroupRatio。
	if result.Billing != nil && result.Billing.Amount > 0 {
		return int(result.Billing.Amount / rmbToUSDRate * common.QuotaPerUnit)
	}

	// 回退：上游未返回 billing 时，按音频时长（分钟向上取整，最低 1 分钟）× ModelPrice($/小时) × GroupRatio 计费。
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelPrice <= 0 || result.AudioDuration <= 0 {
		return 0
	}
	actualMinutes := math.Ceil(result.AudioDuration / 60.0)
	if actualMinutes < 1 {
		actualMinutes = 1
	}
	actualHours := actualMinutes / 60.0
	return int(bc.ModelPrice * common.QuotaPerUnit * bc.GroupRatio * actualHours)
}

// parseTaskResult 解析任务结果头，兼容两种存储形态：task.Data 直接是 result，或包在 "result" 字段下（轮询合并的 mergedFetchResponse）。
func parseTaskResult(data []byte) taskResultHeader {
	var result taskResultHeader
	if err := common.Unmarshal(data, &result); err == nil && (result.AudioDuration > 0 || result.Billing != nil) {
		return result
	}
	var wrapped struct {
		Result taskResultHeader `json:"result"`
	}
	if err := common.Unmarshal(data, &wrapped); err == nil {
		return wrapped.Result
	}
	return taskResultHeader{}
}

// SanitizeResultForClient 调整对客户端返回的结果快照中的 billing 块：
// 所有金额字段按固定汇率折算为网关单位（USD，与预扣/退款一致），移除上游账户余额
// balance，保留 billable_minutes、mode 等非金额字段。
// 不修改入参（DB 中保留上游原始数据），解析失败或无 billing 时原样返回。
func SanitizeResultForClient(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	var root map[string]any
	if err := common.Unmarshal(data, &root); err != nil {
		return data
	}
	billing := locateBilling(root)
	if billing == nil {
		return data
	}
	convertBilling(billing)
	out, err := common.Marshal(root)
	if err != nil {
		return data
	}
	return out
}

func locateBilling(root map[string]any) map[string]any {
	if result, ok := root["result"].(map[string]any); ok {
		if billing, ok := result["billing"].(map[string]any); ok {
			return billing
		}
	}
	if billing, ok := root["billing"].(map[string]any); ok {
		return billing
	}
	return nil
}

// convertBilling 原地转换 billing 块：删除 balance，把金额字段按固定汇率折算到网关单位。
// billable_minutes（计数）与字符串字段（mode 等）保持不变；detail 内全部为金额，递归折算。
func convertBilling(billing map[string]any) {
	delete(billing, "balance")
	for k, v := range billing {
		switch k {
		case "billable_minutes":
			continue
		case "detail":
			if detail, ok := v.(map[string]any); ok {
				convertMoneyMap(detail)
			}
		default:
			if f, ok := v.(float64); ok {
				billing[k] = f / rmbToUSDRate
			}
		}
	}
}

func convertMoneyMap(m map[string]any) {
	for k, v := range m {
		if f, ok := v.(float64); ok {
			m[k] = f / rmbToUSDRate
		}
	}
}

// ── Request Building ─────────────────────────────────────────────────

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return strings.TrimRight(a.baseURL, "/") + "/tasks", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, _ *relaycommon.RelayInfo) (io.Reader, error) {
	reqVal, exists := c.Get(contextKeyParsedRequest)
	if !exists {
		return nil, fmt.Errorf("parsed request not found in context")
	}
	parsed := reqVal.(*clientRequest)

	upstreamReq := parsed.CreateTaskRequest
	body, err := common.Marshal(upstreamReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// ── Response Handling ────────────────────────────────────────────────

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, &dto.TaskError{Code: "read_response_failed", Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	defer resp.Body.Close()

	var statusResp taskStatusResponse
	if err := common.Unmarshal(body, &statusResp); err != nil {
		return "", nil, &dto.TaskError{Code: "parse_response_failed", Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	upstreamTaskID := statusResp.ID
	if upstreamTaskID == "" {
		return "", nil, &dto.TaskError{Code: "invalid_upstream_response", Message: "upstream returned empty task id", StatusCode: http.StatusBadGateway}
	}

	// 回写给客户端的是网关自己的公开 task ID（task_xxxx），客户端用它轮询；
	// 上游真实 ID 作为返回值存为 UpstreamTaskID，仅供后台轮询访问上游使用。
	statusResp.ID = info.PublicTaskID
	c.JSON(http.StatusOK, statusResp)

	return upstreamTaskID, body, nil
}

// ── Polling ──────────────────────────────────────────────────────────

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	base := strings.TrimRight(baseURL, "/")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("create http client failed: %w", err)
	}

	// 1. Fetch status
	statusURL := fmt.Sprintf("%s/tasks/%s", base, taskID)
	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", key)

	statusResp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch task status failed: %w", err)
	}
	statusBody, err := io.ReadAll(statusResp.Body)
	statusResp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read status body failed: %w", err)
	}

	var status taskStatusResponse
	if err := common.Unmarshal(statusBody, &status); err != nil {
		return buildResponseFromBody(statusBody, statusResp.StatusCode), nil
	}

	// 2. If done, also fetch result and merge
	if status.Status == "done" {
		resultURL := fmt.Sprintf("%s/tasks/%s/result", base, taskID)
		resultReq, err := http.NewRequest(http.MethodGet, resultURL, nil)
		if err == nil {
			resultReq.Header.Set("X-API-Key", key)
			resultResp, err := client.Do(resultReq)
			if err == nil {
				resultBody, _ := io.ReadAll(resultResp.Body)
				resultResp.Body.Close()
				if len(resultBody) > 0 {
					merged := mergedFetchResponse{
						taskStatusResponse: status,
						Result:             json.RawMessage(resultBody),
					}
					mergedBody, _ := common.Marshal(merged)
					return buildResponseFromBody(mergedBody, http.StatusOK), nil
				}
			}
		}
	}

	return buildResponseFromBody(statusBody, statusResp.StatusCode), nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var merged mergedFetchResponse
	if err := common.Unmarshal(respBody, &merged); err != nil {
		return nil, fmt.Errorf("parse task result failed: %w", err)
	}

	info := &relaycommon.TaskInfo{
		TaskID: merged.ID,
	}

	switch merged.Status {
	case "processing":
		info.Status = model.TaskStatusInProgress
	case "done":
		info.Status = model.TaskStatusSuccess
	case "failed", "abandoned":
		info.Status = model.TaskStatusFailure
		info.Reason = merged.ErrorMessage
	default:
		info.Status = model.TaskStatusInProgress
	}

	return info, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return nil
}

func (a *TaskAdaptor) GetChannelName() string {
	return "Amux STT"
}

// ── Helpers ──────────────────────────────────────────────────────────

func buildResponseFromBody(body []byte, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": {"application/json"}},
	}
}
