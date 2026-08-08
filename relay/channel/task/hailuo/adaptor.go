package hailuo

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// h3RequestContextKey 缓存归一化后的 H3 请求。校验、计费、构造上游请求体
// 三处都要用，解析只做一次。
const h3RequestContextKey = "minimax_h3_request"

// https://platform.minimaxi.com/docs/api-reference/video-generation-intro
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if c.GetBool(constant.CtxKeyMinimaxV2Format) {
		// v2 原生协议的 prompt 藏在 content[] 里，通用校验会误判为缺 prompt。
		// 参数校验统一推迟到 ValidateMappedRequestAndSetAction（模型映射之后
		// 执行，别名与官方模型名因此享有一致的约束）。
		info.Action = constant.TaskActionGenerate
		return nil
	}
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// ValidateMappedRequestAndSetAction 在渠道模型映射完成后、价格计算之前执行
// H3 专属校验。放在这一层而不是 ValidateRequestAndSetAction，是为了让模型
// 别名与直接使用官方模型名具有完全一致的参数约束。
//
// 这里同时预演一遍计费：算不出价的请求必须在预扣费之前就被拒掉，绝不能
// 放行一个不知道该收多少钱的任务。
func (a *TaskAdaptor) ValidateMappedRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !IsH3Model(info.UpstreamModelName) {
		return nil
	}
	h3, err := a.resolveH3Request(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if _, err := billing_setting.ComputeVideoCost(info.OriginModelName, h3.ToVideoUsage()); err != nil {
		return service.TaskErrorWrapperLocal(err, "model_price_error", http.StatusBadRequest)
	}
	return nil
}

// EstimateBilling 把整单美元金额作为唯一的 OtherRatio 返回。
//
// 基础额度是哨兵价 $1（defaultModelPrice["MiniMax-H3"]），乘上这个金额后
// 就是真实报价，分组倍率照常作用在最外层。之所以不拆成
// seconds × resolution × ... 多个倍率连乘：输入素材是按张/按秒加钱的加法
// 项，塞进乘法管道会产生顺序依赖，改一处就算错钱。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if !IsH3Model(info.UpstreamModelName) {
		return nil
	}
	h3, err := a.resolveH3Request(c)
	if err != nil {
		return nil
	}
	cost, err := billing_setting.ComputeVideoCost(info.OriginModelName, h3.ToVideoUsage())
	if err != nil {
		return nil
	}
	logger.LogJson(c, "minimax h3 cost breakdown", cost)
	usage := h3.ToVideoUsage()
	c.Set(constant.CtxKeyVideoUsageSnapshot, &model.VideoUsageSnapshot{
		Resolution:    usage.Resolution,
		OutputSeconds: usage.OutputSeconds,
		ImageCount:    usage.ImageCount,
		AudioSeconds:  usage.AudioSeconds,
		VideoSeconds:  usage.VideoSeconds,
	})
	c.Set(constant.CtxKeyVideoBillingDetail, map[string]any{
		"resolution":     cost.Resolution,
		"output_seconds": usage.OutputSeconds,
		"image_count":    usage.ImageCount,
		"audio_seconds":  usage.AudioSeconds,
		"video_seconds":  usage.VideoSeconds,
		"output":         cost.Output,
		"image":          cost.Image,
		"audio":          cost.Audio,
		"video":          cost.Video,
		"total":          cost.Total,
	})
	return map[string]float64{billing_setting.VideoCostRatioKey: cost.Total}
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if IsH3Model(info.UpstreamModelName) {
		return fmt.Sprintf("%s%s", a.baseURL, H3Endpoint), nil
	}
	return fmt.Sprintf("%s%s", a.baseURL, TextToVideoEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if IsH3Model(info.UpstreamModelName) {
		h3, err := a.resolveH3Request(c)
		if err != nil {
			return nil, err
		}
		v2Req := h3.ToV2Request(info.UpstreamModelName)
		logger.LogJson(c, "minimax h3 video request body", v2Req)
		data, err := common.Marshal(v2Req)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	if IsH3Model(info.UpstreamModelName) {
		return a.doH3Response(c, responseBody, info)
	}

	var hResp VideoResponse
	if err := common.Unmarshal(responseBody, &hResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if hResp.BaseResp.StatusCode != StatusSuccess {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("hailuo api error: %s", hResp.BaseResp.StatusMsg),
			strconv.Itoa(hResp.BaseResp.StatusCode),
			http.StatusBadRequest,
		)
		return
	}

	upstreamTaskID := hResp.TaskID

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return upstreamTaskID, responseBody, nil
}

// AdjustBillingOnComplete 用上游返回的真实用量重算额度，多退少补。
//
// 提交时参考素材只有 URL、拿不到时长，只能按官方上限 15 秒预扣；上游在
// 查询响应的 usage 里给出真实秒数后，这里按真值重算。实测一单 2K 6 秒
// + 7 秒参考视频，预扣 $2.73、实收应为 $1.69，不结算就会多收 38%。
//
// 返回 0 表示保持预扣额度（拿不到用量、或算不出价时的安全兜底）。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	if task == nil || task.PrivateData.BillingContext == nil {
		return 0
	}
	bc := task.PrivateData.BillingContext
	if _, ok := billing_setting.GetVideoPricing(bc.OriginModelName); !ok {
		return 0
	}

	var stored V2QueryResponse
	if err := common.Unmarshal(task.Data, &stored); err != nil || stored.Task == nil || stored.Task.Usage == nil {
		return 0
	}
	var upstream struct {
		InputImageCount *int     `json:"input_image_count"`
		InputSeconds    *float64 `json:"input_seconds"`
		OutputSeconds   *float64 `json:"output_seconds"`
	}
	if err := common.Unmarshal(stored.Task.Usage, &upstream); err != nil {
		return 0
	}

	// 以提交时的口径为基准，只用上游真值替换能确认的维度
	usage := billing_setting.VideoUsage{Resolution: stored.Task.Resolution}
	if snap := bc.VideoUsage; snap != nil {
		usage = billing_setting.VideoUsage{
			Resolution:    snap.Resolution,
			OutputSeconds: snap.OutputSeconds,
			ImageCount:    snap.ImageCount,
			AudioSeconds:  snap.AudioSeconds,
			VideoSeconds:  snap.VideoSeconds,
		}
		if stored.Task.Resolution != "" {
			usage.Resolution = stored.Task.Resolution
		}
	}
	if upstream.OutputSeconds != nil && *upstream.OutputSeconds > 0 {
		usage.OutputSeconds = *upstream.OutputSeconds
	}
	if upstream.InputImageCount != nil {
		usage.ImageCount = *upstream.InputImageCount
	}
	// input_seconds 是上游返回的输入素材计费秒数，且不拆分视频/音频。
	// 归属由价目表 + 原请求实际带了哪些素材共同决定，见 AttributeInputSeconds。
	// 不可归属时保持提交口径。
	if upstream.InputSeconds != nil {
		if pricing, ok := billing_setting.GetVideoPricing(bc.OriginModelName); ok {
			videoSec, audioSec, attributed := pricing.AttributeInputSeconds(
				usage.Resolution, *upstream.InputSeconds,
				usage.VideoSeconds > 0, usage.AudioSeconds > 0,
			)
			if attributed {
				usage.VideoSeconds = videoSec
				usage.AudioSeconds = audioSec
			}
		}
	}

	cost, err := billing_setting.ComputeVideoCost(bc.OriginModelName, usage)
	if err != nil {
		return 0
	}
	groupRatio := bc.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}
	// 必须与预扣用同一套公式（先算基础额度再乘金额，各自截断一次），
	// 否则纯文生这种「预扣本来就准」的单子会因为浮点误差产生 1 个单位的
	// 虚假差额，凭空多出一条退款/补扣日志。
	baseQuota := int(billing_setting.VideoBasePrice * common.QuotaPerUnit * groupRatio)
	return int(float64(baseQuota) * cost.Total)
}

// doH3Response 处理 v2 创建响应。v2 没有 base_resp——成功只返回 task_id，
// 失败靠 HTTP 状态码 + error 对象。
func (a *TaskAdaptor) doH3Response(c *gin.Context, responseBody []byte, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	var resp V2CreateResponse
	if err := common.Unmarshal(responseBody, &resp); err != nil {
		return "", nil, service.TaskErrorWrapper(
			errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	if resp.Error != nil && resp.Error.Message != "" {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("minimax api error: %s", resp.Error.Message), resp.Error.Code, http.StatusBadRequest)
	}
	if resp.TaskID == "" {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("task_id is empty, body: %s", responseBody), "invalid_response", http.StatusInternalServerError)
	}

	if c.GetBool(constant.CtxKeyMinimaxV2Format) {
		// 原样回官方结构，只把上游 task_id 换成网关公开 ID
		c.JSON(http.StatusOK, V2CreateResponse{TaskID: info.PublicTaskID})
		return resp.TaskID, responseBody, nil
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return resp.TaskID, responseBody, nil
}

// ConvertToMinimaxV2 把网关任务转换为 MiniMax v2 的任务查询结构。
//
// 只输出网关公开 ID 与脱敏后的结果地址：上游任务 ID 是内部标识，
// 用户拿到也没用，反而泄露上游身份。
func (a *TaskAdaptor) ConvertToMinimaxV2(originTask *model.Task) ([]byte, error) {
	var stored V2QueryResponse
	if len(originTask.Data) > 0 {
		// 存量任务的 Data 可能是别的结构，解析失败不影响状态映射
		_ = common.Unmarshal(originTask.Data, &stored)
	}

	task := &V2Task{
		ID:       originTask.TaskID,
		Model:    originTask.Properties.OriginModelName,
		Status:   mapTaskStatusToMinimaxV2(originTask.Status),
		Modality: "video",
		TaskType: "generation",
	}
	if originTask.CreatedAt > 0 {
		task.CreatedAt = originTask.CreatedAt
	}
	if originTask.UpdatedAt > 0 {
		task.UpdatedAt = originTask.UpdatedAt
	}
	if src := stored.Task; src != nil {
		task.Resolution = src.Resolution
		task.Duration = src.Duration
		task.Ratio = src.Ratio
		task.Usage = src.Usage
		if src.TaskType != "" {
			task.TaskType = src.TaskType
		}
	}

	if originTask.Status == model.TaskStatusSuccess {
		if resultURL := originTask.GetResultURL(); resultURL != "" {
			task.Content = &V2TaskContent{URL: resultURL}
		}
	}
	if originTask.Status == model.TaskStatusFailure {
		task.Error = &V2Error{Code: "task_failed", Message: originTask.FailReason}
		if src := stored.Task; src != nil && src.Error != nil {
			if src.Error.Code != "" {
				task.Error.Code = src.Error.Code
			}
			if src.Error.Message != "" {
				task.Error.Message = src.Error.Message
			}
		}
	}

	return common.Marshal(V2QueryResponse{Task: task})
}

func mapTaskStatusToMinimaxV2(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued:
		return H3StatusQueued
	case model.TaskStatusInProgress:
		return H3StatusRunning
	case model.TaskStatusSuccess:
		return H3StatusSucceeded
	case model.TaskStatusFailure:
		return H3StatusFailed
	default:
		return H3StatusQueued
	}
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	// H3 走 v2 查询端点，且用【路径参数】；v1 是 ?task_id= 查询参数。
	// 轮询循环通过 body["model"] 传上游模型名，见 service/task_polling.go。
	var uri string
	if modelName, _ := body["model"].(string); IsH3Model(modelName) {
		uri = fmt.Sprintf("%s%s/%s", baseUrl, H3QueryEndpoint, url.PathEscape(taskID))
	} else {
		uri = fmt.Sprintf("%s%s?task_id=%s", baseUrl, QueryTaskEndpoint, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
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

// resolveH3Request 把当前请求归一化为 H3Request，带默认值与校验，结果缓存
// 在 context 上。站内统一协议与 v2 原生协议在这里汇合成同一个结构，之后的
// 校验、计费、上游请求构造全部只认它。
func (a *TaskAdaptor) resolveH3Request(c *gin.Context) (*H3Request, error) {
	if cached, exists := c.Get(h3RequestContextKey); exists {
		if h3, ok := cached.(*H3Request); ok {
			return h3, nil
		}
	}

	var (
		h3  *H3Request
		err error
	)
	if c.GetBool(constant.CtxKeyMinimaxV2Format) {
		var v2Req V2VideoRequest
		if err = common.UnmarshalBodyReusable(c, &v2Req); err != nil {
			return nil, errors.Wrap(err, "invalid minimax v2 request body")
		}
		h3, err = H3FromV2Request(&v2Req)
	} else {
		var req relaycommon.TaskSubmitReq
		if req, err = relaycommon.GetTaskRequest(c); err == nil {
			h3, err = H3FromTaskSubmitReq(req)
		}
	}
	if err != nil {
		return nil, err
	}
	h3.ApplyDefaults()
	if err := h3.Validate(); err != nil {
		return nil, err
	}

	c.Set(h3RequestContextKey, h3)
	return h3, nil
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*VideoRequest, error) {
	modelConfig := GetModelConfig(info.UpstreamModelName)
	duration := DefaultDuration
	if req.Duration != nil && *req.Duration > 0 {
		duration = *req.Duration
	}
	resolution := modelConfig.DefaultResolution
	if req.Size != "" {
		resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}

	videoRequest := &VideoRequest{
		Model:      info.UpstreamModelName,
		Prompt:     req.Prompt,
		Duration:   &duration,
		Resolution: resolution,
	}
	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to video request failed")
	}

	return videoRequest, nil
}

func (a *TaskAdaptor) parseResolutionFromSize(size string, modelConfig ModelConfig) string {
	switch {
	case strings.Contains(size, "1080"):
		return Resolution1080P
	case strings.Contains(size, "768"):
		return Resolution768P
	case strings.Contains(size, "720"):
		return Resolution720P
	case strings.Contains(size, "512"):
		return Resolution512P
	default:
		return modelConfig.DefaultResolution
	}
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// v2 的响应把任务包在 task 字段里，状态枚举也和 v1 完全不同。
	// ParseTaskResult 拿不到模型名，只能按结构判别。
	var v2 V2QueryResponse
	if err := common.Unmarshal(respBody, &v2); err == nil && v2.Task != nil && v2.Task.Status != "" {
		return parseH3TaskResult(&v2), nil
	}

	resTask := QueryTaskResponse{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{}

	if resTask.BaseResp.StatusCode == StatusSuccess {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.BaseResp.StatusCode
		taskResult.Reason = resTask.BaseResp.StatusMsg
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}

	switch resTask.Status {
	case TaskStatusPreparing, TaskStatusQueueing, TaskStatusProcessing:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
		if resTask.Status == TaskStatusProcessing {
			taskResult.Progress = "50%"
		}
	case TaskStatusSuccess:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		// v2 可能直接给结果地址；v1 只给 file_id，需要再调一次文件检索。
		if resTask.VideoURL != "" {
			taskResult.Url = resTask.VideoURL
		} else {
			taskResult.Url = a.buildVideoURL(resTask.TaskID, resTask.FileID)
		}
	case TaskStatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

// parseH3TaskResult 把 v2 查询响应映射成网关任务状态。
// v2 成功时直接给 content.url，不需要像 v1 那样再用 file_id 换一次地址。
func parseH3TaskResult(resp *V2QueryResponse) *relaycommon.TaskInfo {
	task := resp.Task
	result := relaycommon.TaskInfo{}

	switch task.Status {
	case H3StatusQueued:
		result.Status = model.TaskStatusQueued
		result.Progress = "10%"
	case H3StatusRunning:
		result.Status = model.TaskStatusInProgress
		result.Progress = "50%"
	case H3StatusSucceeded:
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		if task.Content != nil {
			result.Url = task.Content.URL
		}
	case H3StatusFailed, H3StatusCancelled:
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = "task " + task.Status
	default:
		result.Status = model.TaskStatusInProgress
		result.Progress = "30%"
	}

	// 错误详情优先取 task.error，其次顶层 error
	for _, e := range []*V2Error{task.Error, resp.Error} {
		if e != nil && e.Message != "" {
			result.Reason = e.Message
			result.Status = model.TaskStatusFailure
			result.Progress = "100%"
			break
		}
	}

	return &result
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var hailuoResp QueryTaskResponse
	if err := common.Unmarshal(originTask.Data, &hailuoResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal hailuo task data failed")
	}

	openAIVideo := originTask.ToOpenAIVideo()
	if hailuoResp.BaseResp.StatusCode != StatusSuccess {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: hailuoResp.BaseResp.StatusMsg,
			Code:    strconv.Itoa(hailuoResp.BaseResp.StatusCode),
		}
	}

	jsonData, err := common.Marshal(openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openai video failed")
	}

	return jsonData, nil
}

func (a *TaskAdaptor) buildVideoURL(_, fileID string) string {
	if a.apiKey == "" || a.baseURL == "" {
		return ""
	}

	url := fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", a.baseURL, fileID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var retrieveResp RetrieveFileResponse
	if err := common.Unmarshal(responseBody, &retrieveResp); err != nil {
		return ""
	}

	if retrieveResp.BaseResp.StatusCode != StatusSuccess {
		return ""
	}

	return retrieveResp.File.DownloadURL
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
