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
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// 编译期断言：这两个接口是可选的（relay 靠类型断言决定调不调），签名写错了
// 不会有编译错误，只会在运行时静默不生效——Seedance 2.5 的参数校验和差额结算
// 都挂在它们上面，静默失效等于放行错误参数、按预扣多收钱。
var (
	_ channel.MappedTaskValidator = (*TaskAdaptor)(nil)
	_ interface {
		AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int
	} = (*TaskAdaptor)(nil)
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
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID     int                    `json:"id"` // Used in query response
		Type   string                 `json:"type"`
		Status string                 `json:"status"` // RUNNING, SUCCESS, FAILED, PENDING
		Param  map[string]interface{} `json:"param"`
		Output *struct {
			URL           string `json:"url"`       // 老格式：视频直链
			VideoURL      string `json:"video_url"` // 新格式：ZeroCut 升级后改用 video_url
			Error         string `json:"error"`     // Error message for failed tasks
			Ratio         string `json:"ratio"`
			Duration      int    `json:"duration"`
			Resolution    string `json:"resolution"`
			RevisedPrompt string `json:"revised_prompt"`
			Usage         struct {
				Credits          int    `json:"credits"`
				TotalTokens      int    `json:"total_tokens"`
				CompletionTokens int    `json:"completion_tokens"`
				TransactionID    string `json:"transactionId"`
			} `json:"usage"`
		} `json:"output"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	} `json:"data"`
	Timestamp string `json:"timestamp"`
}

// ZeroCut create response (for task creation)
type zeroCutCreateResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		WorkflowId int    `json:"workflowId"` // Note: different field name than query response
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
	// Seedance 2.5（官渠）的参数校验统一推迟到 ValidateMappedRequestAndSetAction：
	// 它跑在模型映射之后，别名与官方模型名因此享有完全一致的约束。
	if IsSeedance25Model(info.OriginModelName) {
		return a.acceptSeedance25Request(c, info)
	}
	// Check if this is Doubao raw format (from /api/v3 route)
	if c.GetBool("doubao_raw_format") {
		return a.validateDoubaoRawRequest(c, info)
	}
	// OpenAI format (from /v1 route) uses standard validation
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// acceptSeedance25Request 只把请求体读进 context，不做参数校验。
//
// 2.5 的提示词是可选的（纯首尾帧、甚至只传一段参考音频都是官方支持的组合），
// 通用校验那条「prompt 必填」对它不成立；原生协议那条「content 必须含 text
// 项」同理。真正的校验在 ValidateMappedRequestAndSetAction 里由
// Seedance25Request.Validate 统一完成。
//
// 注意这里认的是 OriginModelName：管理员用 channel.model_mapping 把一个完全
// 自定义的别名映射到 2.5 时，这一层认不出来，会走到默认的 prompt 必填分支。
// 这类别名想用无提示词组合，得把别名起成能被 seedanceAliasMap 认出的名字。
func (a *TaskAdaptor) acceptSeedance25Request(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c.GetBool("doubao_raw_format") {
		// 原生协议的请求体由 resolveSeedance25Request 直接解析成自己的类型，
		// 不需要在这里预存
		info.Action = constant.TaskActionGenerate
		return nil
	}
	return relaycommon.ValidateBasicTaskRequestWithOptions(c, info,
		constant.TaskActionGenerate, relaycommon.TaskValidateOptions{PromptOptional: true})
}

// ValidateMappedRequestAndSetAction 在渠道模型映射完成后、价格计算之前执行
// Seedance 2.5 的专属校验。
//
// 这里同时预演一遍计费：算不出价的请求必须在预扣费之前就被拒掉，绝不能放行
// 一个不知道该收多少钱的任务。
func (a *TaskAdaptor) ValidateMappedRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !IsSeedance25Model(info.UpstreamModelName) {
		return nil
	}
	req, err := a.resolveSeedance25Request(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	usage, err := Seedance25Usage(info.OriginModelName, req)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "model_price_error", http.StatusBadRequest)
	}
	if _, err := billing_setting.ComputeVideoCost(info.OriginModelName, usage); err != nil {
		return service.TaskErrorWrapperLocal(err, "model_price_error", http.StatusBadRequest)
	}
	return nil
}

// resolveSeedance25Request 把当前请求归一化为 Seedance25Request，带默认值与
// 校验，结果缓存在 context 上。站内统一协议与火山 v3 原生协议在这里汇合成同
// 一个结构，之后的校验、计费、上游请求构造全部只认它。
func (a *TaskAdaptor) resolveSeedance25Request(c *gin.Context) (*Seedance25Request, error) {
	if cached, exists := c.Get(seedance25RequestContextKey); exists {
		if req, ok := cached.(*Seedance25Request); ok {
			return req, nil
		}
	}

	var (
		s25 *Seedance25Request
		err error
	)
	if c.GetBool("doubao_raw_format") {
		var incoming ArkIncomingRequest
		if err = common.UnmarshalBodyReusable(c, &incoming); err != nil {
			return nil, errors.Wrap(err, "invalid volcengine v3 request body")
		}
		s25, err = Seedance25FromArkRequest(&incoming)
	} else {
		var req relaycommon.TaskSubmitReq
		if req, err = relaycommon.GetTaskRequest(c); err == nil {
			s25, err = Seedance25FromTaskSubmitReq(req)
		}
	}
	if err != nil {
		return nil, err
	}
	s25.ApplyDefaults()
	if err := s25.Validate(); err != nil {
		return nil, err
	}

	c.Set(seedance25RequestContextKey, s25)
	return s25, nil
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

// EstimateBilling 按 (model, resolution, hasVideoInput) 查 seedance 计费档位，
// 返回相对 ModelRatio 基准（720p + 不含视频）的乘数。命中后塞到 OtherRatios
// 的 "seedance_pricing" key，由 relay_task 累乘到基础额度上。
//
// 三维档位映射放在 constants.go 的 seedancePricingMap：
//   - doubao-seedance-2-0：(720p / 1080p) × (含视频 / 不含视频) 共 4 档
//   - doubao-seedance-2-0-fast：仅 720p 档（fast 不支持 1080p）
//
// 查表优先级：UpstreamModelName（考虑 channel 的 model_mapping）> 原始
// OriginModelName。这样无论管理员用"官方端点名直接上"还是"配 model_mapping
// 把别名映射过去"，都能命中；GetSeedancePricingRatio 内部还会再走一遍
// 别名归一，兜底"没有配 model_mapping 也没改名"的场景。
//
// 仅在 ratio != 1.0 时塞 OtherRatios——720p 不含视频是基准档，没必要在
// 日志里挂一个无意义的 ratio=1.0 entry，让 BillingContext 干净。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// Seedance 2.5 走 video_pricing 价目表，返回整单美元金额而不是相对倍率。
	// 两套计价互不相干：2.5 不在 seedancePricingMap 里，2.0 也没有价目表。
	if IsSeedance25Model(info.UpstreamModelName) {
		return a.estimateSeedance25Billing(c, info)
	}

	// Check if this is Doubao raw format
	if c.GetBool("doubao_raw_format") {
		originalReq, exists := c.Get("doubao_original_request")
		if !exists {
			return nil
		}
		reqMap := originalReq.(map[string]interface{})
		hasVideo := hasVideoInRawContent(reqMap)
		resolution := getResolutionFromRawContent(reqMap)
		if ratio, ok := lookupSeedancePricing(info, resolution, hasVideo); ok && ratio != 1.0 {
			return map[string]float64{"seedance_pricing": ratio}
		}
		return nil
	}

	// OpenAI format
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	hasVideo := hasVideoInMetadata(req.Metadata)
	resolution := getResolutionFromMetadata(req.Metadata)
	if ratio, ok := lookupSeedancePricing(info, resolution, hasVideo); ok && ratio != 1.0 {
		return map[string]float64{"seedance_pricing": ratio}
	}
	return nil
}

// estimateSeedance25Billing 把整单美元金额作为唯一的 OtherRatio 返回。
//
// 基础额度是哨兵价 $1（billing_setting.VideoBasePrice），乘上这个金额后就是
// 真实报价，分组倍率照常作用在最外层。
//
// 同时留下两份快照：BillingContext 里的计费口径（终态结算要靠它判断原请求
// 有没有带参考视频，那决定走哪一档单价），以及消费日志里的分项明细。
func (a *TaskAdaptor) estimateSeedance25Billing(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := a.resolveSeedance25Request(c)
	if err != nil {
		return nil
	}
	usage, err := Seedance25Usage(info.OriginModelName, req)
	if err != nil {
		return nil
	}
	cost, err := billing_setting.ComputeVideoCost(info.OriginModelName, usage)
	if err != nil {
		return nil
	}
	logger.LogJson(c, "seedance 2.5 cost breakdown", cost)

	c.Set(constant.CtxKeyVideoUsageSnapshot, videoUsageSnapshot(usage))
	c.Set(constant.CtxKeyVideoBillingDetail, map[string]any{
		"resolution":     cost.Resolution,
		"output_seconds": usage.OutputSeconds,
		"image_count":    usage.ImageCount,
		"video_seconds":  usage.VideoSeconds,
		"output":         cost.Output,
		"image":          cost.Image,
		"video":          cost.Video,
		"total":          cost.Total,
		// duration=-1 时输出秒数是价目表配的折中值，终态会按上游真实用量重算。
		// 标出来，免得对账时把预估值当成实际时长。
		"estimated": req.Duration <= 0 || usage.VideoSeconds > 0,
	})
	return map[string]float64{billing_setting.VideoCostRatioKey: cost.Total}
}

// AdjustBillingOnComplete 用上游返回的真实 token 用量重算额度，多退少补。
//
// 为什么认 token 而不是响应里的 duration：duration 只是输出时长，既不含参考
// 视频的贡献，也不含官方对「含视频输入」任务的最低 token 用量下限。token 是
// 上游真正的计费口径，这些都已经算在里面了。换算见 Seedance25BillableSeconds。
//
// 返回 0 表示保持预扣额度——这是拿不到用量、或任何一步对不上时的安全兜底。
// 注意它必须配合 settleTaskBillingOnComplete 里「有视频价目表就不按 token
// 倍率重算」的守卫：否则返回 0 会掉进那条分支，用 ModelRatio（视频模型是 0
// 或管理员从 2.0 复制来的残值）算出完全错误的金额。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	if task == nil || task.PrivateData.BillingContext == nil {
		return 0
	}
	bc := task.PrivateData.BillingContext
	pricing, ok := billing_setting.GetVideoPricing(bc.OriginModelName)
	if !ok {
		// 2.0 这类按 token 倍率计费的模型不走这条路，交给通用的 token 重算
		return 0
	}

	var stored responseTask
	if err := common.Unmarshal(task.Data, &stored); err != nil {
		return 0
	}
	tokens := stored.Usage.CompletionTokens
	if tokens <= 0 {
		tokens = stored.Usage.TotalTokens
	}

	// 没有提交时的口径快照就判断不出走哪一档单价。此时默认按「不含视频」结算
	// 会给带参考视频的任务按 1.66 倍的高单价收钱——宁可保持预扣，也不能朝多收
	// 的方向猜。正常路径下 EstimateBilling 必然写入快照，走到这里说明数据不全。
	snap := bc.VideoUsage
	if snap == nil {
		return 0
	}
	resolution := stored.Resolution
	if resolution == "" {
		resolution = snap.Resolution
	}

	seconds, ok := Seedance25BillableSeconds(tokens, resolution, stored.Ratio, stored.FramesPerSecond)
	if !ok {
		return 0
	}
	// 计费秒数不可能少于输出时长：真出现说明 token 字段选错了、或上游改了
	// token 公式。这种时候宁可保持预扣，也不能拿一个算错的数去扣钱。
	if stored.Duration > 0 && seconds < float64(stored.Duration) {
		return 0
	}

	// 原请求带没带参考视频决定走哪一档单价。上游只给一个合并的 token 用量，
	// 拆不出输出/输入各占多少，所以档位只能靠提交时的快照来还原。
	// 存量快照没有 HasVideoInput 字段，用 VideoSeconds 兜底。
	hasVideoInput := snap.HasVideoInput || snap.VideoSeconds > 0
	usage := billing_setting.VideoUsage{
		Resolution:    resolution,
		OutputSeconds: seconds,
		ImageCount:    snap.ImageCount,
		HasVideoInput: hasVideoInput,
	}
	// 价目表给输入视频单独配了价时才拆分，好让计费明细能看出输出/输入各占
	// 多少。两档单价相同（2.5 就是如此）时拆不拆总额一样；没配输入视频价时
	// 拆分会把那部分按 0 计费，所以不拆。
	if hasVideoInput && stored.Duration > 0 &&
		float64(stored.Duration) < seconds && pricing.ChargesInputVideo(resolution) {
		usage.OutputSeconds = float64(stored.Duration)
		usage.VideoSeconds = seconds - float64(stored.Duration)
	}

	cost, err := billing_setting.ComputeVideoCost(bc.OriginModelName, usage)
	if err != nil {
		return 0
	}
	// 把结算口径写回快照：差额结算那条消费日志要靠它渲染分项明细，不改的话
	// 显示的还是预扣时的秒数，和这笔金额对不上。
	bc.VideoUsage = videoUsageSnapshot(usage)

	groupRatio := bc.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}
	// 必须与预扣用同一套公式（先算基础额度再乘金额，各自截断一次），否则
	// 预扣本来就准的单子会因为浮点误差产生 1 个单位的虚假差额，凭空多出一条
	// 退款/补扣日志。
	baseQuota := int(billing_setting.VideoBasePrice * common.QuotaPerUnit * groupRatio)
	return int(float64(baseQuota) * cost.Total)
}

// videoUsageSnapshot 把计费口径转成落库快照。提交时存预扣口径，终态结算时
// 改写成实际口径，两条日志各自渲染出的明细才对得上自己那笔金额。
func videoUsageSnapshot(usage billing_setting.VideoUsage) *model.VideoUsageSnapshot {
	return &model.VideoUsageSnapshot{
		Resolution:    usage.Resolution,
		OutputSeconds: usage.OutputSeconds,
		ImageCount:    usage.ImageCount,
		AudioSeconds:  usage.AudioSeconds,
		VideoSeconds:  usage.VideoSeconds,
		HasVideoInput: usage.HasVideoInput || usage.VideoSeconds > 0,
	}
}

// seedance25UpstreamModel 决定发给上游的模型名。
//
// 管理员显式配了 channel.model_mapping 就完全听它的——上游可能是只认某个对外
// 别名的第三方；没配映射时归一到火山官方 endpoint 名，因为 2.5 的前提就是直
// 连官渠，把 "doubao-seedance-2-5" 原样发过去官方不认。
//
// 这与 2.0 那条路径「一律不自动归一」的取舍不同，原因也在这里：2.0 对接的是
// 第三方聚合器，替换成官方端点名会被直接拒掉。
func (a *TaskAdaptor) seedance25UpstreamModel(info *relaycommon.RelayInfo) string {
	if info.IsModelMapped {
		return info.UpstreamModelName
	}
	canonical, ok := CanonicalSeedanceName(strings.ToLower(strings.TrimSpace(info.UpstreamModelName)))
	if !ok {
		return info.UpstreamModelName
	}
	info.UpstreamModelName = canonical
	return canonical
}

// lookupSeedancePricing 先尝试 UpstreamModelName（含 model_mapping 结果），
// 再退回 OriginModelName。两个都经过 GetSeedancePricingRatio 的别名归一。
func lookupSeedancePricing(info *relaycommon.RelayInfo, resolution string, hasVideo bool) (float64, bool) {
	if info.UpstreamModelName != "" {
		if r, ok := GetSeedancePricingRatio(info.UpstreamModelName, resolution, hasVideo); ok {
			return r, true
		}
	}
	return GetSeedancePricingRatio(info.OriginModelName, resolution, hasVideo)
}

// getResolutionFromMetadata 从 OpenAI 格式 metadata 里读 resolution 字段。
// schema 里管理员声明的 resolution 字段会经 setting 面板/前端塞到 metadata
// 顶层。读不到就返回 ""——下游 normalizeSeedanceResolution 会兜成 720p 档。
func getResolutionFromMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["resolution"].(string); ok {
		return v
	}
	return ""
}

// getResolutionFromRawContent 从 Doubao raw 格式请求体里读 resolution。
// raw 格式下 resolution 是 body 顶层字段（与 ratio / duration / seed 同级），
// 不在 content 数组里。
func getResolutionFromRawContent(reqMap map[string]interface{}) string {
	if reqMap == nil {
		return ""
	}
	if v, ok := reqMap["resolution"].(string); ok {
		return v
	}
	return ""
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
	if IsSeedance25Model(info.UpstreamModelName) {
		s25, err := a.resolveSeedance25Request(c)
		if err != nil {
			return nil, err
		}
		arkReq := s25.ToArkRequest(a.seedance25UpstreamModel(info))
		logger.LogJson(c, "seedance 2.5 video request body", arkReq)
		data, err := common.Marshal(arkReq)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

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
	// 必须放在 model 映射之后：admin 可能用 channel.model_mapping 把
	// 自定义别名（如 "seedance-pro"）映射到官方 doubao-seedance-2-0-260128，
	// 在映射前调 strip 拿到的是原始别名，CanonicalSeedanceName 命不中，
	// duration 不会被剔除→上游照样 InvalidParameter。
	stripIncompatibleParams(body)
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

	// 只有非空 prompt 才覆盖 Content 里的 text 项；空 prompt（如 first_last
	// 模式只发首/末帧）时保留 metadata.content 自带的 text 项（如果有）；
	// 都没有的话 Content 里就完全没有 text 条目，让上游用纯帧画面推断过渡，
	// 避免向 Volcengine 推一个 `{type:"text", text:""}` 触发上游校验失败。
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
		r.Content = append(r.Content, ContentItem{
			Type: "text",
			Text: prompt,
		})
	}

	return &r, nil
}

// stripIncompatibleParams 按 model + 模式（t2v / i2v / r2v）剔除上游会拒收的
// 参数。doubao-seedance-2-0 系列在 r2v 模式（仅 reference_image，无 first_frame/
// last_frame）下不接受 duration——上游直接报 InvalidParameter；保留给用户
// 让他们看到 "the parameter duration is not valid for ... in r2v" 没意义。
//
// 仅在 OpenAI 格式（playground / 标准 task 提交）路径调用；raw 格式由上层
// 客户端按官方文档自构请求，不在这里 second-guess。
func stripIncompatibleParams(r *requestPayload) {
	canonical, _ := CanonicalSeedanceName(r.Model)
	if canonical != "doubao-seedance-2-0-260128" && canonical != "doubao-seedance-2-0-fast-260128" {
		return
	}
	hasReference := false
	hasFrame := false
	for _, item := range r.Content {
		if item.Type != "image_url" {
			continue
		}
		switch item.Role {
		case "reference_image":
			hasReference = true
		case "first_frame", "last_frame":
			hasFrame = true
		}
	}
	if hasReference && !hasFrame {
		r.Duration = nil
	}
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
				// 兼容新老字段：优先 video_url（新格式），回退 url（老格式）
				taskResult.Url = zeroCutResp.Data.Output.VideoURL
				if taskResult.Url == "" {
					taskResult.Url = zeroCutResp.Data.Output.URL
				}
				taskResult.CompletionTokens = zeroCutResp.Data.Output.Usage.CompletionTokens
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
	case arkStatusExpired:
		// 任务在 execution_expires_after 内没跑完，上游已经终止它。漏掉这个
		// 分支会掉进 default 被当成「进行中」，任务永远到不了终态，预扣的
		// 额度也就永远不释放。
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
		if taskResult.Reason == "" {
			taskResult.Reason = "task expired"
		}
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
	//   2) ZeroCut 聚合器：{code, data:{status, output:{url|video_url, error, revised_prompt}}}
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

// mapToDoubaoV3Status 将网关内部任务状态映射回火山 v3 协议的状态字符串。
func mapToDoubaoV3Status(s model.TaskStatus) string {
	switch s {
	case model.TaskStatusInProgress:
		return dto.DoubaoV3StatusRunning
	case model.TaskStatusSuccess:
		return dto.DoubaoV3StatusSucceeded
	case model.TaskStatusFailure:
		return dto.DoubaoV3StatusFailed
	default:
		// NOT_START / SUBMITTED / QUEUED / UNKNOWN 统一归为 queued
		return dto.DoubaoV3StatusQueued
	}
}

// ConvertToDoubaoV3 将网关任务转换为火山方舟 v3 协议的任务查询响应。
//
// 注意：当前上游是 ZeroCut 聚合器，task.Data 里存的是 ZeroCut 格式
// （{code, data:{status, output:{url, ratio, duration, resolution, usage}}}），
// 与火山官方结构不同。因此提取 resolution/duration/ratio 等元数据时优先按
// ZeroCut 解析，官方格式兜底；对外输出统一归一化为火山官方 v3 结构。
//
// id 使用网关 public task ID（非上游 cgt-xxx），video_url 使用脱敏/代理后的
// GetResultURL()，与 ConvertToOpenAIVideo 的口径保持一致，不暴露上游直链。
func (a *TaskAdaptor) ConvertToDoubaoV3(originTask *model.Task) ([]byte, error) {
	resp := &dto.DoubaoV3Video{
		ID:        originTask.TaskID,
		Model:     originTask.Properties.OriginModelName,
		Status:    mapToDoubaoV3Status(originTask.Status),
		CreatedAt: originTask.CreatedAt,
		UpdatedAt: originTask.UpdatedAt,
	}

	if originTask.TotalTokens > 0 || originTask.CompletionTokens > 0 {
		resp.Usage = &dto.DoubaoV3VideoUsage{
			CompletionTokens: originTask.CompletionTokens,
			TotalTokens:      originTask.TotalTokens,
		}
	}

	// 从原始上游响应提取 resolution/duration/ratio 等元数据。
	if len(originTask.Data) > 0 {
		var zResp zeroCutResponse
		if err := common.Unmarshal(originTask.Data, &zResp); err == nil && zResp.Data.Output != nil {
			// ZeroCut 聚合器格式（当前实际上游）
			resp.Resolution = zResp.Data.Output.Resolution
			resp.Duration = zResp.Data.Output.Duration
			resp.Ratio = zResp.Data.Output.Ratio
		} else {
			// 火山官方格式兜底
			var oResp responseTask
			if err := common.Unmarshal(originTask.Data, &oResp); err == nil {
				resp.Resolution = oResp.Resolution
				resp.Duration = oResp.Duration
				resp.Ratio = oResp.Ratio
				resp.Seed = oResp.Seed
				resp.FramesPerSecond = oResp.FramesPerSecond
				// 联网搜索的实际调用次数：官方在 usage.tool_usage 里给，
				// 客户端靠它判断这一单有没有真的走搜索。
				if oResp.Usage.ToolUsage.WebSearch > 0 {
					if resp.Usage == nil {
						resp.Usage = &dto.DoubaoV3VideoUsage{
							CompletionTokens: originTask.CompletionTokens,
							TotalTokens:      originTask.TotalTokens,
						}
					}
					resp.Usage.ToolUsage = &dto.DoubaoV3VideoToolUsage{
						WebSearch: oResp.Usage.ToolUsage.WebSearch,
					}
				}
			}
		}
	}

	if originTask.Status == model.TaskStatusFailure {
		resp.Error = &dto.DoubaoV3VideoError{
			Message: originTask.FailReason,
			Code:    "task_failed",
		}
		// 失败任务从上游原始体里尽量提取更精确的错误信息
		if len(originTask.Data) > 0 {
			var zResp zeroCutResponse
			if err := common.Unmarshal(originTask.Data, &zResp); err == nil && zResp.Data.Output != nil && zResp.Data.Output.Error != "" {
				resp.Error.Message = zResp.Data.Output.Error
				resp.Error.Code = "zerocut_error"
			} else {
				var oResp responseTask
				if err := common.Unmarshal(originTask.Data, &oResp); err == nil && oResp.Error.Message != "" {
					resp.Error.Message = oResp.Error.Message
					resp.Error.Code = oResp.Error.Code
				}
			}
		}
	} else if url := originTask.GetResultURL(); url != "" {
		resp.Content = &dto.DoubaoV3VideoContent{VideoURL: url}
	}

	return common.Marshal(resp)
}
