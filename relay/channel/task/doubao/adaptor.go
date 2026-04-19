package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// APIType represents the type of upstream API
type APIType int

const (
	APITypeDoubaoOfficial APIType = iota // Doubao official API
	APITypeZeroCut                       // ZeroCut API
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	Resolution  string         `json:"resolution,omitempty"`
	Ratio       string         `json:"ratio,omitempty"`
	Duration    *dto.IntValue  `json:"duration,omitempty"`
	Frames      *dto.IntValue  `json:"frames,omitempty"`
	Seed        *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark   *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ZeroCut API response structure (for task query - full format)
type zeroCutResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      struct {
		ID     int    `json:"id"`  // Used in query response
		Type   string `json:"type"`
		Status string `json:"status"` // RUNNING, SUCCESS, FAILED, PENDING
		Param  map[string]interface{} `json:"param"`
		Output *struct {
			URL            string `json:"url"`
			Error          string `json:"error"` // Error message for failed tasks
			Ratio          string `json:"ratio"`
			Duration       int    `json:"duration"`
			Resolution     string `json:"resolution"`
			RevisedPrompt  string `json:"revised_prompt"`
			Usage          struct {
				Credits        int    `json:"credits"`
				TotalTokens    int    `json:"total_tokens"`
				TransactionID  string `json:"transactionId"`
			} `json:"usage"`
		} `json:"output"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	} `json:"data"`
	Timestamp string `json:"timestamp"`
}

// ZeroCut create response (for task creation)
type zeroCutCreateResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      struct {
		WorkflowId int    `json:"workflowId"`  // Note: different field name than query response
		Status     string `json:"status"`
	} `json:"data"`
	Timestamp string `json:"timestamp"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
	apiType     APIType
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey

	// Detect API type based on Base URL
	if strings.Contains(a.baseURL, "zerocut.cn") {
		a.apiType = APITypeZeroCut
	} else {
		a.apiType = APITypeDoubaoOfficial
	}
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Check if this is Doubao raw format (from /api/v3 route)
	if c.GetBool("doubao_raw_format") {
		return a.validateDoubaoRawRequest(c, info)
	}
	// OpenAI format (from /v1 route) uses standard validation
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// validateDoubaoRawRequest validates Doubao official API format
func (a *TaskAdaptor) validateDoubaoRawRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	originalReq, exists := c.Get("doubao_original_request")
	if !exists {
		return service.TaskErrorWrapperLocal(fmt.Errorf("doubao original request not found"), "invalid_request", http.StatusBadRequest)
	}

	reqMap, ok := originalReq.(map[string]interface{})
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid request format"), "invalid_request", http.StatusBadRequest)
	}

	// Validate content array
	contentRaw, ok := reqMap["content"]
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("content is required"), "invalid_request", http.StatusBadRequest)
	}

	contentArray, ok := contentRaw.([]interface{})
	if !ok || len(contentArray) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("content must be non-empty array"), "invalid_request", http.StatusBadRequest)
	}

	// Validate at least one text item exists
	hasText := false
	for _, item := range contentArray {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if typeVal, ok := itemMap["type"].(string); ok && typeVal == "text" {
				hasText = true
				break
			}
		}
	}
	if !hasText {
		return service.TaskErrorWrapperLocal(fmt.Errorf("content must contain at least one text item"), "invalid_request", http.StatusBadRequest)
	}

	info.Action = constant.TaskActionGenerate
	return nil
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	switch a.apiType {
	case APITypeZeroCut:
		return fmt.Sprintf("%s/api/video-service/seedance/create", a.baseURL), nil
	default:
		return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
	}
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 检测请求 metadata 中是否包含视频输入，返回视频折扣 OtherRatio。
//
// 折扣查表优先级：UpstreamModelName（考虑 channel 的 model_mapping）> 原始
// OriginModelName。这样无论管理员用"官方端点名直接上"还是"配 model_mapping
// 把别名映射过去"，都能命中；GetVideoInputRatio 内部还会再走一遍别名归一，
// 兜底"没有配 model_mapping 也没改名"的场景。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// Check if this is Doubao raw format
	if c.GetBool("doubao_raw_format") {
		originalReq, exists := c.Get("doubao_original_request")
		if !exists {
			return nil
		}
		reqMap := originalReq.(map[string]interface{})
		if hasVideoInRawContent(reqMap) {
			if ratio, ok := lookupVideoInputRatio(info); ok {
				return map[string]float64{"video_input": ratio}
			}
		}
		return nil
	}

	// OpenAI format
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	if hasVideoInMetadata(req.Metadata) {
		if ratio, ok := lookupVideoInputRatio(info); ok {
			return map[string]float64{"video_input": ratio}
		}
	}
	return nil
}

// lookupVideoInputRatio 先尝试 UpstreamModelName（含 model_mapping 结果），
// 再退回 OriginModelName。两个都经过 GetVideoInputRatio 的别名归一。
func lookupVideoInputRatio(info *relaycommon.RelayInfo) (float64, bool) {
	if info.UpstreamModelName != "" {
		if r, ok := GetVideoInputRatio(info.UpstreamModelName); ok {
			return r, true
		}
	}
	return GetVideoInputRatio(info.OriginModelName)
}

// hasVideoInRawContent checks if raw Doubao request contains video_url
func hasVideoInRawContent(reqMap map[string]interface{}) bool {
	contentRaw, ok := reqMap["content"]
	if !ok {
		return false
	}
	contentArray, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// hasVideoInMetadata 直接检查 metadata 的 content 数组是否包含 video_url 条目，
// 避免构建完整的上游 requestPayload。
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	// Check if this is Doubao raw format (from /api/v3 route)
	if c.GetBool("doubao_raw_format") {
		// Use original request directly without conversion
		originalReq, exists := c.Get("doubao_original_request")
		if !exists {
			return nil, fmt.Errorf("doubao original request not found")
		}

		reqMap := originalReq.(map[string]interface{})

		// Both Doubao and ZeroCut accept the same content array format
		body := &requestPayload{}
		bodyBytes, _ := common.Marshal(reqMap)
		if err := common.Unmarshal(bodyBytes, body); err != nil {
			return nil, errors.Wrap(err, "unmarshal doubao raw request failed")
		}

		// Handle model mapping
		// 只按 channel 的 model_mapping 决定最终上游模型名；不做任何
		// 厂商别名的"自动归一"。原因：上游可能是官方 Volcengine Ark，也可能
		// 是只接受 seedance-2.0-api 这种对外别名的第三方聚合器——替换成
		// 官方的 doubao-seedance-2-0-260128 会被后者直接拒掉。需要改写名字
		// 的场景（例如对接官方 Ark + 对外暴露友好别名），管理员通过后台
		// model_mapping 显式配置即可。
		if info.IsModelMapped {
			body.Model = info.UpstreamModelName
		} else {
			info.UpstreamModelName = body.Model
		}

		data, err := common.Marshal(body)
		if err != nil {
			return nil, err
		}

		return bytes.NewReader(data), nil
	}

	// OpenAI format: use standard conversion logic
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Check for error response first (common format for both APIs)
	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.Unmarshal(responseBody, &errorResp); err == nil && errorResp.Error.Code != "" {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("%s: %s", errorResp.Error.Code, errorResp.Error.Message),
			"upstream_api_error",
			resp.StatusCode,
		)
		return
	}

	// Try to parse as ZeroCut create response first (for task creation)
	var zeroCutCreateResp zeroCutCreateResponse
	var dResp responsePayload

	if err := common.Unmarshal(responseBody, &zeroCutCreateResp); err == nil && zeroCutCreateResp.Code > 0 && zeroCutCreateResp.Data.WorkflowId > 0 {
		// ZeroCut create format detected
		
		// Check for error (non-200 code)
		if zeroCutCreateResp.Code != 200 {
			taskErr = service.TaskErrorWrapper(
				fmt.Errorf("ZeroCut error (code %d): %s", zeroCutCreateResp.Code, zeroCutCreateResp.Message),
				"zerocut_api_error",
				resp.StatusCode,
			)
			return
		}
		
		// Convert ZeroCut response to standard format
		dResp.ID = strconv.Itoa(zeroCutCreateResp.Data.WorkflowId)
	} else {
		// Try ZeroCut query format (for task query)
		var zeroCutResp zeroCutResponse
		if err := common.Unmarshal(responseBody, &zeroCutResp); err == nil && zeroCutResp.Code > 0 && zeroCutResp.Data.ID > 0 {
			// ZeroCut query format detected
			
			// Check for error (non-200 code)
			if zeroCutResp.Code != 200 {
				taskErr = service.TaskErrorWrapper(
					fmt.Errorf("ZeroCut error (code %d): %s", zeroCutResp.Code, zeroCutResp.Message),
					"zerocut_api_error",
					resp.StatusCode,
				)
				return
			}
			
			// Convert ZeroCut response to standard format
			dResp.ID = strconv.Itoa(zeroCutResp.Data.ID)
		} else {
			// Parse as Doubao response
			if err := common.Unmarshal(responseBody, &dResp); err != nil {
				taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
				return
			}
			
			if dResp.ID == "" {
				taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
				return
			}
		}
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var uri string
	// Detect API type based on Base URL
	if strings.Contains(baseUrl, "zerocut.cn") {
		// ZeroCut API: /api/video-service/omni/:id
		uri = fmt.Sprintf("%s/api/video-service/omni/%s", baseUrl, taskID)
	} else {
		// Doubao official API
		uri = fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// Add images if present
	if req.HasImage() {
		for _, imgURL := range req.Images {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: imgURL,
				},
			})
		}
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	}

	r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
	r.Content = append(r.Content, ContentItem{
		Type: "text",
		Text: req.Prompt,
	})

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Try to parse as ZeroCut format first
	var zeroCutResp zeroCutResponse
	if err := common.Unmarshal(respBody, &zeroCutResp); err == nil && zeroCutResp.Code > 0 && zeroCutResp.Data.ID > 0 {
		// ZeroCut format detected
		
		// Check for error response
		if zeroCutResp.Code != 200 {
			taskResult.Status = model.TaskStatusFailure
			taskResult.Progress = "100%"
			taskResult.Reason = fmt.Sprintf("code %d: %s", zeroCutResp.Code, zeroCutResp.Message)
			return &taskResult, nil
		}
		
		// Parse status
		switch zeroCutResp.Data.Status {
		case "PENDING":
			taskResult.Status = model.TaskStatusQueued
			taskResult.Progress = "10%"
		case "RUNNING":
			taskResult.Status = model.TaskStatusInProgress
			taskResult.Progress = "50%"
		case "SUCCESS":
			taskResult.Status = model.TaskStatusSuccess
			taskResult.Progress = "100%"
			// Extract output data
			if zeroCutResp.Data.Output != nil {
				taskResult.Url = zeroCutResp.Data.Output.URL
				// Map credits to tokens
				taskResult.CompletionTokens = zeroCutResp.Data.Output.Usage.Credits
				taskResult.TotalTokens = zeroCutResp.Data.Output.Usage.TotalTokens
			}
		case "FAILED":
			taskResult.Status = model.TaskStatusFailure
			taskResult.Progress = "100%"
			// Extract error message from output.error first, fallback to top-level message
			if zeroCutResp.Data.Output != nil && zeroCutResp.Data.Output.Error != "" {
				taskResult.Reason = zeroCutResp.Data.Output.Error
			} else {
				taskResult.Reason = zeroCutResp.Message
			}
			// Ensure Url is empty for failed tasks
			taskResult.Url = ""
		default:
			taskResult.Status = model.TaskStatusInProgress
			taskResult.Progress = "50%"
		}
		return &taskResult, nil
	}

	// Fallback to Doubao official format
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	// 视频 URL 以 task.PrivateData.ResultURL 为权威来源——ParseTaskResult
	// 在官方 Doubao 格式和 ZeroCut 聚合器格式下都已经正确提取 url 到这里，
	// 比在这里再重新反序列化 task.Data 更可靠。Data 仅用来取额外元数据
	// （如错误详情、revised_prompt 等）。
	openAIVideo.SetMetadata("url", originTask.GetResultURL())

	// 尝试解析 task.Data 获取错误详情 / revised_prompt 等元数据。
	// 两种上游格式都宽容处理：
	//   1) 官方 Doubao：{status, content:{video_url}, error:{code,message}}
	//   2) ZeroCut 聚合器：{code, data:{status, output:{url, error, revised_prompt}}}
	if len(originTask.Data) > 0 {
		// 官方 Doubao 格式
		var dResp responseTask
		if err := common.Unmarshal(originTask.Data, &dResp); err == nil {
			if dResp.Status == "failed" && dResp.Error.Message != "" {
				openAIVideo.Error = &dto.OpenAIVideoError{
					Message: dResp.Error.Message,
					Code:    dResp.Error.Code,
				}
			}
		}
		// ZeroCut 聚合器格式（兜底）
		if openAIVideo.Error == nil && originTask.Status == model.TaskStatusFailure {
			var zResp zeroCutResponse
			if err := common.Unmarshal(originTask.Data, &zResp); err == nil && zResp.Data.Output != nil {
				if zResp.Data.Output.Error != "" {
					openAIVideo.Error = &dto.OpenAIVideoError{
						Message: zResp.Data.Output.Error,
						Code:    "zerocut_error",
					}
				}
			}
		}
		// 任何格式都没解析出具体错误信息时，回退到 task.FailReason
		if openAIVideo.Error == nil && originTask.Status == model.TaskStatusFailure {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: originTask.FailReason,
				Code:    "task_failed",
			}
		}
	}

	return common.Marshal(openAIVideo)
}
