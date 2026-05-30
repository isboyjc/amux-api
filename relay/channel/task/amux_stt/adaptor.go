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

type taskResultHeader struct {
	AudioDuration float64 `json:"audio_duration"`
}

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
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelPrice <= 0 {
		return 0
	}

	var result taskResultHeader
	if err := common.Unmarshal(task.Data, &result); err != nil || result.AudioDuration <= 0 {
		// result may be wrapped in a "result" field from our merged fetch response
		var wrapped struct {
			Result taskResultHeader `json:"result"`
		}
		if err2 := common.Unmarshal(task.Data, &wrapped); err2 != nil || wrapped.Result.AudioDuration <= 0 {
			return 0
		}
		result = wrapped.Result
	}

	// ModelPrice 是 $/小时；音频时长按分钟向上取整（最低 1 分钟）后换算成小时计费。
	actualMinutes := math.Ceil(result.AudioDuration / 60.0)
	if actualMinutes < 1 {
		actualMinutes = 1
	}
	actualHours := actualMinutes / 60.0
	return int(bc.ModelPrice * common.QuotaPerUnit * bc.GroupRatio * actualHours)
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
