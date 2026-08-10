package ali

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

// AliVideoRequest 阿里通义万相视频生成请求
type AliVideoRequest struct {
	Model                     string              `json:"model"`
	Input                     AliVideoInput       `json:"input"`
	Parameters                *AliVideoParameters `json:"parameters,omitempty"`
	BillingInputVideoDuration float64             `json:"-"`
}

// AliVideoInput 视频输入参数
type AliVideoInput struct {
	Prompt         string          `json:"prompt,omitempty"`          // 文本提示词
	ImgURL         string          `json:"img_url,omitempty"`         // 首帧图像URL或Base64（旧版图生视频）
	FirstFrameURL  string          `json:"first_frame_url,omitempty"` // 首帧图片URL（旧版首尾帧生视频）
	LastFrameURL   string          `json:"last_frame_url,omitempty"`  // 尾帧图片URL（旧版首尾帧生视频）
	AudioURL       string          `json:"audio_url,omitempty"`       // 音频URL（旧版 wan2.5）
	NegativePrompt string          `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string          `json:"template,omitempty"`        // 视频特效模板
	Media          []AliVideoMedia `json:"media,omitempty"`           // 新版视频模型多模态素材
}

// AliVideoMedia 是阿里视频模型 input.media 中的单个媒体素材。
// BillingDuration 仅供网关按源视频实际时长预扣，不会透传给阿里上游。
type AliVideoMedia struct {
	Type            string  `json:"type"`
	URL             string  `json:"url"`
	BillingDuration float64 `json:"-"`
}

// UnmarshalJSON 接受网关扩展的 media.duration 计费提示。MarshalJSON 不实现
// 对应扩展，因此 BillingDuration 会由 json:"-" 自动从上游请求中移除。
func (m *AliVideoMedia) UnmarshalJSON(data []byte) error {
	var wire struct {
		Type     string         `json:"type"`
		URL      string         `json:"url"`
		Duration dto.FloatValue `json:"duration,omitempty"`
	}
	if err := common.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.Type = wire.Type
	m.URL = wire.URL
	m.BillingDuration = float64(wire.Duration)
	return nil
}

// AliVideoParameters 视频参数
type AliVideoParameters struct {
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Size         *string `json:"size,omitempty"`          // 尺寸: 如 "832*480"（旧版文生视频）
	Ratio        *string `json:"ratio,omitempty"`         // HappyHorse 输出宽高比
	Duration     *int    `json:"duration,omitempty"`      // 时长（显式 0 仍需保留给校验）
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启 prompt 智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否添加音频（旧版 wan2.5）
	AudioSetting *string `json:"audio_setting,omitempty"` // HappyHorse 视频编辑声音控制
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子（显式 0 必须发送）
}

// AliVideoResponse 阿里通义万相响应
type AliVideoResponse struct {
	Output    AliVideoOutput `json:"output"`
	RequestID string         `json:"request_id"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Usage     *AliUsage      `json:"usage,omitempty"`
}

// AliVideoOutput 输出信息
type AliVideoOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// AliUsage 使用统计
type AliUsage struct {
	Duration            dto.FloatValue `json:"duration,omitempty"`
	InputVideoDuration  dto.FloatValue `json:"input_video_duration,omitempty"`
	OutputVideoDuration dto.FloatValue `json:"output_video_duration,omitempty"`
	VideoCount          dto.IntValue   `json:"video_count,omitempty"`
	SR                  dto.IntValue   `json:"SR,omitempty"`
	Ratio               string         `json:"ratio,omitempty"`
}

type AliMetadata struct {
	Input      *AliVideoInput      `json:"input,omitempty"`      // 兼容 metadata.input 私有协议
	Parameters *AliVideoParameters `json:"parameters,omitempty"` // 兼容 metadata.parameters 私有协议

	// Input 相关
	AudioURL       string           `json:"audio_url,omitempty"`       // 音频URL
	ImgURL         string           `json:"img_url,omitempty"`         // 图片URL（图生视频）
	FirstFrameURL  string           `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string           `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	NegativePrompt string           `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string           `json:"template,omitempty"`        // 视频特效模板
	Media          []AliVideoMedia  `json:"media,omitempty"`           // Wan 2.7 官方 media 结构
	Content        []AliContentItem `json:"content,omitempty"`         // 通用视频接口的富媒体结构

	// Parameters 相关
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Size         *string `json:"size,omitempty"`          // 尺寸: 如 "832*480"
	Ratio        *string `json:"ratio,omitempty"`         // HappyHorse 宽高比
	Duration     *int    `json:"duration,omitempty"`      // 时长
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否添加音频
	AudioSetting *string `json:"audio_setting,omitempty"` // HappyHorse 视频编辑声音控制
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子
}

// AliContentItem 是通用视频接口 metadata.content 中的媒体条目。
// role 由 Playground 的参数 Schema 生成，适配器再映射为 Wan 2.7 media.type。
type AliContentItem struct {
	Type     string         `json:"type,omitempty"`
	Role     string         `json:"role,omitempty"`
	Duration dto.FloatValue `json:"duration,omitempty"`
	ImageURL *AliMediaURL   `json:"image_url,omitempty"`
	VideoURL *AliMediaURL   `json:"video_url,omitempty"`
	AudioURL *AliMediaURL   `json:"audio_url,omitempty"`
}

type AliMediaURL struct {
	URL string `json:"url,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

var _ channel.PreValidationBillingEstimator = (*TaskAdaptor)(nil)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if c.GetBool("ali_video_official_format") {
		return a.validateOfficialRequest(c, info)
	}
	// ValidateMultipartDirect 负责解析并将原始 TaskSubmitReq 存入 context
	if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		return taskErr
	}
	if taskReq, err := getAliTaskRequest(c); err == nil {
		if action := actionForAliVideoModelKind(getAliVideoModelKind(taskReq.Model)); action != "" {
			info.Action = action
		} else if strings.Contains(strings.ToLower(taskReq.Model), "i2v") ||
			taskReq.InputReference != "" || taskReq.Image != "" || len(taskReq.Images) > 0 {
			info.Action = constant.TaskActionGenerate
		}
	}
	return nil
}

// ValidateMappedRequestAndSetAction 在模型映射完成后按最终上游模型校验。
// 这样模型别名与直接使用官方模型名具有完全一致的参数约束和任务动作。
func (a *TaskAdaptor) ValidateMappedRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	aliReq, err := a.getMappedAliRequest(c, info)
	if err != nil {
		code := "invalid_request"
		if c.GetBool("ali_video_official_format") {
			code = "InvalidParameter"
		}
		return service.TaskErrorWrapperLocal(err, code, http.StatusBadRequest)
	}
	if err := ensureVerifiedHappyHorseEditDuration(c, aliReq); err != nil {
		code := "invalid_request"
		if c.GetBool("ali_video_official_format") {
			code = "InvalidParameter"
		}
		return service.TaskErrorWrapperLocal(err, code, http.StatusBadRequest)
	}

	if action := actionForAliVideoModelKind(getAliVideoModelKind(aliReq.Model)); action != "" {
		info.Action = action
	}
	if isHappyHorseModelKind(getAliVideoModelKind(aliReq.Model)) {
		originModel := ""
		if info != nil && strings.TrimSpace(info.OriginModelName) != "" {
			originModel = info.OriginModelName
		}
		_, pricing, ok := billing_setting.ResolveVideoPricing(originModel, aliReq.Model)
		if !ok {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("video pricing not configured for model %s", originModel),
				"model_price_error", http.StatusBadRequest,
			)
		}
		if _, err := pricing.Compute(happyHorseVideoUsage(aliReq)); err != nil {
			return service.TaskErrorWrapperLocal(err, "model_price_error", http.StatusBadRequest)
		}
	}
	return nil
}

func (a *TaskAdaptor) getMappedAliRequest(c *gin.Context, info *relaycommon.RelayInfo) (*AliVideoRequest, error) {
	if c.GetBool("ali_video_official_format") {
		return a.getOfficialRequest(c, info)
	}
	taskReq, err := getAliTaskRequest(c)
	if err != nil {
		return nil, err
	}
	return a.convertToAliRequest(info, taskReq)
}

// EstimatePreValidationBilling 为 HappyHorse Video Edit 的远程视频探测建立
// 最低额度门槛。源视频最短 3 秒，而官方计费时长是输入+输出（等长），所以
// 探测前仅预扣 6 秒；探测成功后 RelayTaskSubmit 会再补到真实源时长×2。
func (a *TaskAdaptor) EstimatePreValidationBilling(c *gin.Context, info *relaycommon.RelayInfo) (map[string]float64, *dto.TaskError) {
	aliReq, err := a.getMappedAliRequest(c, info)
	if err != nil {
		code := "invalid_request"
		if c.GetBool("ali_video_official_format") {
			code = "InvalidParameter"
		}
		return nil, service.TaskErrorWrapperLocal(err, code, http.StatusBadRequest)
	}
	if getAliVideoModelKind(aliReq.Model) != aliVideoModelHappyHorseEdit {
		return nil, nil
	}
	originModel := ""
	if info != nil {
		originModel = strings.TrimSpace(info.OriginModelName)
	}
	_, pricing, ok := billing_setting.ResolveVideoPricing(originModel, aliReq.Model)
	if !ok {
		return nil, service.TaskErrorWrapperLocal(
			fmt.Errorf("video pricing not configured for model %s", originModel),
			"model_price_error", http.StatusBadRequest,
		)
	}
	usage := happyHorseVideoUsage(aliReq)
	usage.OutputSeconds = float64(happyHorseVideoEditMinSourceSeconds * 2)
	cost, err := pricing.Compute(usage)
	if err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "model_price_error", http.StatusBadRequest)
	}
	return map[string]float64{billing_setting.VideoCostRatioKey: cost.Total}, nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL, err := normalizeAliBaseURL(a.baseURL)
	if err != nil {
		return "", err
	}
	return baseURL + "/api/v1/services/aigc/video-generation/video-synthesis", nil
}

// BuildRequestHeader sets required headers for Ali API
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 阿里异步任务必须设置
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if c.GetBool("ali_video_official_format") {
		aliReq, err := a.getOfficialRequest(c, info)
		if err != nil {
			return nil, errors.Wrap(err, "get_official_ali_request_failed")
		}
		logger.LogJson(c, "ali official video request body", aliReq)
		bodyBytes, err := common.Marshal(aliReq)
		if err != nil {
			return nil, errors.Wrap(err, "marshal_ali_request_failed")
		}
		return bytes.NewReader(bodyBytes), nil
	}

	taskReq, err := getAliTaskRequest(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_task_request_failed")
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_ali_request_failed")
	}
	logger.LogJson(c, "ali video request body", aliReq)

	bodyBytes, err := common.Marshal(aliReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ali_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

// EstimateBilling 根据用户请求参数计算 OtherRatios（时长、分辨率等）。
// 在 ValidateRequestAndSetAction 之后、价格计算之前调用。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	var aliReq *AliVideoRequest
	var err error
	if c.GetBool("ali_video_official_format") {
		aliReq, err = a.getOfficialRequest(c, info)
	} else {
		var taskReq relaycommon.TaskSubmitReq
		taskReq, err = getAliTaskRequest(c)
		if err == nil {
			aliReq, err = a.convertToAliRequest(info, taskReq)
		}
	}
	if err != nil || aliReq == nil {
		return nil
	}
	if err := ensureVerifiedHappyHorseEditDuration(c, aliReq); err != nil {
		return nil
	}
	if isHappyHorseModelKind(getAliVideoModelKind(aliReq.Model)) {
		originModel := ""
		if info != nil && strings.TrimSpace(info.OriginModelName) != "" {
			originModel = info.OriginModelName
		}
		_, pricing, ok := billing_setting.ResolveVideoPricing(originModel, aliReq.Model)
		if !ok {
			return nil
		}
		usage := happyHorseVideoUsage(aliReq)
		cost, err := pricing.Compute(usage)
		if err != nil {
			return nil
		}
		// 官方协议允许省略 resolution。把价目表解析出的默认档位写回快照，
		// 避免终态响应缺少 usage.SR 时只能靠空值再次猜测。
		usage.Resolution = cost.Resolution
		logger.LogJson(c, "happyhorse cost breakdown", cost)
		c.Set(constant.CtxKeyVideoUsageSnapshot, aliVideoUsageSnapshot(usage))
		c.Set(constant.CtxKeyVideoBillingDetail, map[string]any{
			"resolution":     cost.Resolution,
			"output_seconds": usage.OutputSeconds,
			"image_count":    usage.ImageCount,
			"video_seconds":  usage.VideoSeconds,
			"output":         cost.Output,
			"image":          cost.Image,
			"video":          cost.Video,
			"total":          cost.Total,
			"estimated":      getAliVideoModelKind(aliReq.Model) == aliVideoModelHappyHorseEdit,
		})
		return map[string]float64{billing_setting.VideoCostRatioKey: cost.Total}
	}

	otherRatios := map[string]float64{
		"seconds": float64(effectiveDuration(aliReq)),
	}
	ratios, err := ProcessAliOtherRatios(aliReq)
	if err != nil {
		return otherRatios
	}
	for k, v := range ratios {
		otherRatios[k] = v
	}
	return otherRatios
}

// AdjustBillingOnComplete 按 HappyHorse 官方 usage 进行最终结算。
// usage.duration 已是计费总时长；Video Edit 中它包含输入与输出视频时长之和，
// 因此不能再乘 video_count，也不能只使用 output_video_duration。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	if task == nil || task.PrivateData.BillingContext == nil {
		return 0
	}
	bc := task.PrivateData.BillingContext
	// 旧任务没有 video_cost + VideoUsage 快照，提交时走的是 ModelPrice/seconds
	// 计费。部署后不能拿新价目表重新结算，否则会在发布窗口内突然补扣或退款。
	if bc.VideoUsage == nil || bc.OtherRatios == nil {
		return 0
	}
	if _, ok := bc.OtherRatios[billing_setting.VideoCostRatioKey]; !ok {
		return 0
	}

	modelName := strings.TrimSpace(task.Properties.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(bc.OriginModelName)
	}
	if !isHappyHorseModelKind(getAliVideoModelKind(modelName)) {
		return 0
	}
	_, pricing, ok := billing_setting.ResolveVideoPricing(bc.OriginModelName, modelName)
	if !ok {
		return 0
	}

	usage, ok := parseAliTaskUsage(task.Data)
	if !ok || usage.Duration <= 0 {
		return 0
	}

	usageForBilling := billing_setting.VideoUsage{}
	if snap := bc.VideoUsage; snap != nil {
		usageForBilling = billing_setting.VideoUsage{
			Resolution:    snap.Resolution,
			OutputSeconds: snap.OutputSeconds,
			ImageCount:    snap.ImageCount,
			AudioSeconds:  snap.AudioSeconds,
			VideoSeconds:  snap.VideoSeconds,
			HasVideoInput: snap.HasVideoInput || snap.VideoSeconds > 0,
		}
	}
	if usage.SR > 0 {
		usageForBilling.Resolution = fmt.Sprintf("%dP", int(usage.SR))
	}
	// HappyHorse 的 usage.duration 已是官方最终计费总时长。Video Edit 中它
	// 包含输入和输出视频时长之和，因此统一记入 OutputSeconds，不能再把输入
	// 视频拆出来重复计价。
	usageForBilling.OutputSeconds = float64(usage.Duration)
	usageForBilling.VideoSeconds = 0
	cost, err := pricing.Compute(usageForBilling)
	if err != nil {
		return 0
	}
	bc.VideoUsage = aliVideoUsageSnapshot(usageForBilling)

	groupRatio := bc.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}
	baseQuota := int(billing_setting.VideoBasePrice * common.QuotaPerUnit * groupRatio)
	return int(float64(baseQuota) * cost.Total)
}

func parseAliTaskUsage(data []byte) (*AliUsage, bool) {
	if len(data) == 0 {
		return nil, false
	}
	var response AliVideoResponse
	if err := common.Unmarshal(data, &response); err == nil && response.Usage != nil && response.Usage.Duration > 0 {
		return response.Usage, true
	}

	// 兼容上游本身也是 new-api 时的 TaskResponse[Task] 包装结构。
	var wrapped struct {
		Data model.Task `json:"data"`
	}
	if err := common.Unmarshal(data, &wrapped); err == nil && len(wrapped.Data.Data) > 0 && !bytes.Equal(wrapped.Data.Data, data) {
		return parseAliTaskUsage(wrapped.Data.Data)
	}
	return nil, false
}

func happyHorseVideoUsage(req *AliVideoRequest) billing_setting.VideoUsage {
	usage := billing_setting.VideoUsage{OutputSeconds: float64(effectiveDuration(req))}
	if req == nil {
		return usage
	}
	if req.Parameters != nil && req.Parameters.Resolution != nil {
		usage.Resolution = *req.Parameters.Resolution
	}
	for _, media := range req.Input.Media {
		switch strings.ToLower(strings.TrimSpace(media.Type)) {
		case "first_frame", "reference_image":
			usage.ImageCount++
		case "video", "reference_video":
			usage.HasVideoInput = true
		}
	}
	return usage
}

func aliVideoUsageSnapshot(usage billing_setting.VideoUsage) *model.VideoUsageSnapshot {
	return &model.VideoUsageSnapshot{
		Resolution:    usage.Resolution,
		OutputSeconds: usage.OutputSeconds,
		ImageCount:    usage.ImageCount,
		AudioSeconds:  usage.AudioSeconds,
		VideoSeconds:  usage.VideoSeconds,
		HasVideoInput: usage.HasVideoInput || usage.VideoSeconds > 0,
	}
}

// DoRequest delegates to common helper
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 解析阿里响应
	var aliResp AliVideoResponse
	if err := common.Unmarshal(responseBody, &aliResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if aliResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", aliResp.Code, aliResp.Message), "ali_api_error", resp.StatusCode)
		if c.GetBool("ali_video_official_format") {
			taskErr.Code = aliResp.Code
			taskErr.Message = aliResp.Message
		}
		return
	}

	if aliResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	upstreamTaskID := aliResp.Output.TaskID
	if c.GetBool("ali_video_official_format") {
		aliResp.Output.TaskID = info.PublicTaskID
		c.JSON(http.StatusOK, aliResp)
		return upstreamTaskID, responseBody, nil
	}

	// 转换为 OpenAI 格式响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = info.PublicTaskID
	openAIResp.TaskID = info.PublicTaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	// 返回 OpenAI 格式
	c.JSON(http.StatusOK, openAIResp)

	return upstreamTaskID, responseBody, nil
}

// FetchTask 查询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	normalizedBaseURL, err := normalizeAliBaseURL(baseUrl)
	if err != nil {
		return nil, err
	}
	uri := fmt.Sprintf("%s/api/v1/tasks/%s", normalizedBaseURL, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

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

// ParseTaskResult 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(respBody, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}
	if aliResp.Code != "" {
		taskResult.Status = model.TaskStatusFailure
		taskResult.Reason = fmt.Sprintf("%s: %s", aliResp.Code, aliResp.Message)
		return &taskResult, nil
	}

	// 状态映射
	switch aliResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里直接返回视频URL，不需要额外的代理端点
		taskResult.Url = aliResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if aliResp.Message != "" {
			taskResult.Reason = aliResp.Message
		} else if aliResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", aliResp.Output.Code, aliResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ali response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	// 上游可能已经 SUCCEEDED，但网关仍在执行归档/URL 重写；客户端状态必须
	// 以网关内部任务为准，避免归档完成前提前显示完成态。
	openAIResp.Status = convertAliTaskStatus(task.Status)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 只有网关任务已成功时才返回视频地址。归档中的任务即使 task.Data 已经
	// 包含上游 SUCCEEDED + video_url，也必须隐藏临时直链。
	videoURL := ""
	if task.Status == model.TaskStatusSuccess {
		videoURL = task.GetResultURL()
		if videoURL == "" {
			videoURL = aliResp.Output.VideoURL
		}
	}
	openAIResp.SetMetadata("url", videoURL)

	// 错误处理
	if aliResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Code,
			Message: aliResp.Message,
		}
	} else if aliResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Output.Code,
			Message: aliResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

func convertAliStatus(aliStatus string) string {
	switch aliStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}

func convertAliTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued:
		return dto.VideoStatusQueued
	case model.TaskStatusInProgress:
		return dto.VideoStatusInProgress
	case model.TaskStatusSuccess:
		return dto.VideoStatusCompleted
	case model.TaskStatusFailure:
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
