package hailuo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// MiniMax-H3 走的是 v2 协议（content[] 多模态数组），与旧模型的 v1
// （扁平字段）不兼容，端点也不同。本文件承担归一化层的职责：
//
//	站内统一协议 (TaskSubmitReq) ─┐
//	                              ├─→ H3Request ─┬─→ ToV2Request()  上游请求
//	MiniMax v2 原生协议 (content) ─┘              └─→ ToVideoUsage() 计费口径
//
// 两条入口共用同一套校验与计费，是这套接入不出岔子的关键：任何一条路径上
// 的参数约束或计价口径改动，另一条自动跟着变。
const (
	ModelMiniMaxH3 = "MiniMax-H3"

	H3Endpoint = "/v2/video_generation"
	// H3QueryEndpoint 后面直接拼 /{task_id}——v2 用路径参数，不是 v1 那种
	// ?task_id= 查询参数。
	H3QueryEndpoint = "/v2/query/video_generation"

	H3Resolution768P = "768P"
	H3Resolution2K   = "2K"

	H3MinDuration     = 4
	H3MaxDuration     = 15
	H3DefaultDuration = 6

	H3MaxPromptLength = 7000
	H3MaxRefImages    = 9
	H3MaxRefVideos    = 3
	H3MaxRefAudios    = 3

	// H3MaxRefMediaSeconds 是官方对参考视频/音频的总时长上限（各自合计 ≤15s）。
	// 请求里只有 URL 拿不到真实时长，预扣阶段按这个上限计费；上游若在查询响应
	// 里返回真实时长，再由 AdjustBillingOnComplete 退差额。
	H3MaxRefMediaSeconds = 15
)

// H3 content 的 type 与 role 取值。
const (
	h3ContentTypeText     = "text"
	h3ContentTypeImageURL = "image_url"
	h3ContentTypeVideoURL = "video_url"
	h3ContentTypeAudioURL = "audio_url"

	h3RoleFirstFrame     = "first_frame"
	h3RoleLastFrame      = "last_frame"
	h3RoleReferenceImage = "reference_image"
	h3RoleReferenceVideo = "reference_video"
	h3RoleReferenceAudio = "reference_audio"
)

// v2 查询响应的任务状态。与 v1 的 Preparing/Queueing/Processing/Success/Fail
// 完全不同，不能混用。
const (
	H3StatusQueued    = "queued"
	H3StatusRunning   = "running"
	H3StatusSucceeded = "succeeded"
	H3StatusFailed    = "failed"
	H3StatusCancelled = "cancelled"
)

var h3SupportedRatios = []string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}

// IsH3Model 判断模型是否走 v2 协议。端点分流、校验分流都依赖它。
func IsH3Model(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), ModelMiniMaxH3)
}

// ---------------------------------------------------------------------------
// 上游 v2 协议结构
// ---------------------------------------------------------------------------

type V2URLRef struct {
	URL string `json:"url"`
}

type V2Content struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *V2URLRef `json:"image_url,omitempty"`
	VideoURL *V2URLRef `json:"video_url,omitempty"`
	AudioURL *V2URLRef `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

// V2VideoRequest 是 POST /v2/video_generation 的请求体。
// 可选标量一律用指针 + omitempty：显式传 false / 0 必须原样送到上游，
// 不能被 omitempty 静默吞掉（CLAUDE.md Rule 6）。
type V2VideoRequest struct {
	Model         string      `json:"model"`
	Content       []V2Content `json:"content"`
	Resolution    string      `json:"resolution"`
	Duration      int         `json:"duration"`
	Ratio         string      `json:"ratio,omitempty"`
	CallbackURL   string      `json:"callback_url,omitempty"`
	AigcWatermark *bool       `json:"aigc_watermark,omitempty"`
}

// V2CreateResponse 是 POST /v2/video_generation 的响应。
// v2 没有 v1 那个 base_resp——错误通过 HTTP 状态码 + error 对象表达。
type V2CreateResponse struct {
	TaskID string   `json:"task_id"`
	Error  *V2Error `json:"error,omitempty"`
}

// V2QueryResponse 是 GET /v2/query/video_generation/{task_id} 的响应，
// 任务信息包在 task 字段里，与 v1 的扁平结构完全不同。
type V2QueryResponse struct {
	Task  *V2Task  `json:"task,omitempty"`
	Error *V2Error `json:"error,omitempty"`
}

type V2Task struct {
	ID         string          `json:"id"`
	Model      string          `json:"model,omitempty"`
	Status     string          `json:"status"`
	CreatedAt  int64           `json:"created_at,omitempty"`
	UpdatedAt  int64           `json:"updated_at,omitempty"`
	Content    *V2TaskContent  `json:"content,omitempty"`
	Resolution string          `json:"resolution,omitempty"`
	Duration   int             `json:"duration,omitempty"`
	Ratio      string          `json:"ratio,omitempty"`
	TaskType   string          `json:"task_type,omitempty"`
	Modality   string          `json:"modality,omitempty"`
	Usage      json.RawMessage `json:"usage,omitempty"`
	Error      *V2Error        `json:"error,omitempty"`
}

// V2TaskContent 成功时直接给结果地址，不需要像 v1 那样再用 file_id 换一次。
type V2TaskContent struct {
	URL string `json:"url,omitempty"`
}

type V2Error struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ---------------------------------------------------------------------------
// 归一化结构
// ---------------------------------------------------------------------------

type H3Request struct {
	Prompt        string
	Resolution    string
	Duration      int
	Ratio         string
	FirstFrame    string
	LastFrame     string
	RefImages     []string
	RefVideos     []string
	RefAudios     []string
	AigcWatermark *bool
	CallbackURL   string
}

// h3Metadata 是站内统一协议下 H3 专有能力的载体。这些字段是 H3 特有的，
// 不值得为它们扩 TaskSubmitReq 的顶层字段——metadata 就是干这个用的。
type h3Metadata struct {
	Resolution  string `json:"resolution,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Ratio       string `json:"ratio,omitempty"`
	// Duration 是 metadata 里的时长。操练场把 schema 渲染出的参数整体塞进
	// metadata（不会提升到顶层字段），所以这里必须收一份，否则用户在面板上
	// 调的时长会被静默丢弃、一律按默认 6 秒出片。顶层 duration 优先级更高。
	Duration           *int     `json:"duration,omitempty"`
	FirstFrameImage    string   `json:"first_frame_image,omitempty"`
	LastFrameImage     string   `json:"last_frame_image,omitempty"`
	ReferenceImageURLs []string `json:"reference_image_urls,omitempty"`
	ReferenceVideoURLs []string `json:"reference_video_urls,omitempty"`
	ReferenceAudioURLs []string `json:"reference_audio_urls,omitempty"`
	AigcWatermark      *bool    `json:"aigc_watermark,omitempty"`
	CallbackURL        string   `json:"callback_url,omitempty"`
	// Content 承载操练场/客户端上传的富媒体。结构与 v2 原生协议的 content[]
	// 完全一致（{type, image_url:{url}, role}），因此这里直接复用同一套解析。
	// 缺了它，操练场上传的首尾帧和参考素材会被静默丢弃。
	Content []V2Content `json:"content,omitempty"`
}

// H3FromTaskSubmitReq 把站内统一视频协议的请求归一化为 H3Request。
func H3FromTaskSubmitReq(req relaycommon.TaskSubmitReq) (*H3Request, error) {
	var meta h3Metadata
	if err := req.UnmarshalMetadata(&meta); err != nil {
		return nil, fmt.Errorf("parse metadata failed: %w", err)
	}

	h3 := &H3Request{
		Prompt:        strings.TrimSpace(req.Prompt),
		Duration:      req.GetDuration(),
		Ratio:         firstNonEmpty(meta.AspectRatio, meta.Ratio),
		FirstFrame:    strings.TrimSpace(meta.FirstFrameImage),
		LastFrame:     strings.TrimSpace(meta.LastFrameImage),
		RefImages:     trimAll(meta.ReferenceImageURLs),
		RefVideos:     trimAll(meta.ReferenceVideoURLs),
		RefAudios:     trimAll(meta.ReferenceAudioURLs),
		AigcWatermark: meta.AigcWatermark,
		CallbackURL:   strings.TrimSpace(meta.CallbackURL),
	}

	// 时长的取值优先级：顶层 duration > metadata.duration > OpenAI 风格的
	// seconds。三者都缺时由 ApplyDefaults 兜底。
	if h3.Duration == 0 && meta.Duration != nil {
		h3.Duration = *meta.Duration
	}
	if h3.Duration == 0 {
		if seconds, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil {
			h3.Duration = seconds
		}
	}

	// 分辨率：metadata.resolution 优先（表达力更强），其次从 size 推断。
	resolution, err := normalizeH3Resolution(firstNonEmpty(meta.Resolution, req.Size))
	if err != nil {
		return nil, err
	}
	h3.Resolution = resolution

	// images[0] / images[1] 是站内协议表达首尾帧的方式；metadata 里的显式
	// first/last_frame_image 优先级更高。
	images := trimAll(req.Images)
	if h3.FirstFrame == "" && len(images) > 0 {
		h3.FirstFrame = images[0]
	}
	if h3.LastFrame == "" && len(images) > 1 {
		h3.LastFrame = images[1]
	}

	// 操练场按 schema 的 x-content-role 把上传的素材拍平成 metadata.content，
	// 结构与 v2 原生协议一致，直接复用同一套解析。
	if err := h3.applyContent(meta.Content); err != nil {
		return nil, err
	}

	return h3, nil
}

// H3FromV2Request 把 MiniMax v2 原生请求归一化为 H3Request。
func H3FromV2Request(v2 *V2VideoRequest) (*H3Request, error) {
	if v2 == nil {
		return nil, fmt.Errorf("request body is empty")
	}

	resolution, err := normalizeH3Resolution(v2.Resolution)
	if err != nil {
		return nil, err
	}

	h3 := &H3Request{
		Resolution:    resolution,
		Duration:      v2.Duration,
		Ratio:         strings.TrimSpace(v2.Ratio),
		AigcWatermark: v2.AigcWatermark,
		CallbackURL:   strings.TrimSpace(v2.CallbackURL),
	}

	if err := h3.applyContent(v2.Content); err != nil {
		return nil, err
	}

	return h3, nil
}

// applyContent 解析 v2 协议的 content 数组。站内统一协议下操练场也用同一套
// 结构（metadata.content），因此两条入口共用这段解析。
func (r *H3Request) applyContent(content []V2Content) error {
	for i, item := range content {
		switch item.Type {
		case h3ContentTypeText:
			// 多个 text 项拼接，保持用户书写顺序
			text := strings.TrimSpace(item.Text)
			if text == "" {
				continue
			}
			if r.Prompt == "" {
				r.Prompt = text
			} else {
				r.Prompt += "\n" + text
			}
		case h3ContentTypeImageURL:
			url, err := contentURL(i, item.ImageURL, "image_url")
			if err != nil {
				return err
			}
			switch item.Role {
			case h3RoleFirstFrame:
				r.FirstFrame = url
			case h3RoleLastFrame:
				r.LastFrame = url
			case h3RoleReferenceImage, "":
				r.RefImages = append(r.RefImages, url)
			default:
				return fmt.Errorf("content[%d]: unsupported role %q for image_url", i, item.Role)
			}
		case h3ContentTypeVideoURL:
			url, err := contentURL(i, item.VideoURL, "video_url")
			if err != nil {
				return err
			}
			if item.Role != h3RoleReferenceVideo && item.Role != "" {
				return fmt.Errorf("content[%d]: unsupported role %q for video_url", i, item.Role)
			}
			r.RefVideos = append(r.RefVideos, url)
		case h3ContentTypeAudioURL:
			url, err := contentURL(i, item.AudioURL, "audio_url")
			if err != nil {
				return err
			}
			if item.Role != h3RoleReferenceAudio && item.Role != "" {
				return fmt.Errorf("content[%d]: unsupported role %q for audio_url", i, item.Role)
			}
			r.RefAudios = append(r.RefAudios, url)
		default:
			return fmt.Errorf("content[%d]: unsupported type %q", i, item.Type)
		}
	}

	return nil
}

func contentURL(index int, ref *V2URLRef, field string) (string, error) {
	if ref == nil || strings.TrimSpace(ref.URL) == "" {
		return "", fmt.Errorf("content[%d]: %s.url is required", index, field)
	}
	return strings.TrimSpace(ref.URL), nil
}

// ---------------------------------------------------------------------------
// 校验
// ---------------------------------------------------------------------------

// ApplyDefaults 填充缺省值。必须在 Validate 之前调用。
func (r *H3Request) ApplyDefaults() {
	if r.Duration == 0 {
		r.Duration = H3DefaultDuration
	}
	if r.Resolution == "" {
		r.Resolution = H3Resolution2K
	}
}

// Validate 只校验官方文档明确写了的约束。没写的（如首尾帧与参考素材是否
// 互斥）交给上游判断——网关不该发明上游没有的限制。
func (r *H3Request) Validate() error {
	if r.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if len([]rune(r.Prompt)) > H3MaxPromptLength {
		return fmt.Errorf("prompt exceeds %d characters", H3MaxPromptLength)
	}
	if r.Duration < H3MinDuration || r.Duration > H3MaxDuration {
		return fmt.Errorf("duration must be between %d and %d seconds", H3MinDuration, H3MaxDuration)
	}
	if r.Resolution != H3Resolution768P && r.Resolution != H3Resolution2K {
		return fmt.Errorf("resolution must be one of %s, %s", H3Resolution768P, H3Resolution2K)
	}
	if r.Ratio != "" && !containsFold(h3SupportedRatios, r.Ratio) {
		return fmt.Errorf("ratio must be one of %s", strings.Join(h3SupportedRatios, ", "))
	}
	if len(r.RefImages) > H3MaxRefImages {
		return fmt.Errorf("at most %d reference images are allowed", H3MaxRefImages)
	}
	if len(r.RefVideos) > H3MaxRefVideos {
		return fmt.Errorf("at most %d reference videos are allowed", H3MaxRefVideos)
	}
	if len(r.RefAudios) > H3MaxRefAudios {
		return fmt.Errorf("at most %d reference audios are allowed", H3MaxRefAudios)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 出口
// ---------------------------------------------------------------------------

// ToV2Request 构造上游请求体。content 顺序固定（文本 → 首尾帧 → 参考素材），
// 保证同一个语义请求每次序列化结果一致，便于比对和排查。
func (r *H3Request) ToV2Request(upstreamModel string) *V2VideoRequest {
	content := make([]V2Content, 0, 1+len(r.RefImages)+len(r.RefVideos)+len(r.RefAudios)+2)
	content = append(content, V2Content{Type: h3ContentTypeText, Text: r.Prompt})

	if r.FirstFrame != "" {
		content = append(content, imageContent(r.FirstFrame, h3RoleFirstFrame))
	}
	if r.LastFrame != "" {
		content = append(content, imageContent(r.LastFrame, h3RoleLastFrame))
	}
	for _, url := range r.RefImages {
		content = append(content, imageContent(url, h3RoleReferenceImage))
	}
	for _, url := range r.RefVideos {
		content = append(content, V2Content{
			Type: h3ContentTypeVideoURL, VideoURL: &V2URLRef{URL: url}, Role: h3RoleReferenceVideo,
		})
	}
	for _, url := range r.RefAudios {
		content = append(content, V2Content{
			Type: h3ContentTypeAudioURL, AudioURL: &V2URLRef{URL: url}, Role: h3RoleReferenceAudio,
		})
	}

	return &V2VideoRequest{
		Model:         upstreamModel,
		Content:       content,
		Resolution:    r.Resolution,
		Duration:      r.Duration,
		Ratio:         r.Ratio,
		AigcWatermark: r.AigcWatermark,
		// CallbackURL 不透传：任务状态由网关轮询，用户的回调地址由网关
		// 自己的回调机制处理，直接转给上游会让上游绕过网关回调用户。
	}
}

func imageContent(url, role string) V2Content {
	return V2Content{Type: h3ContentTypeImageURL, ImageURL: &V2URLRef{URL: url}, Role: role}
}

// ToVideoUsage 产出计费口径。
//
// 参考视频/音频只有 URL、拿不到真实时长，这里按官方的总时长上限（各 15s）
// 计费。这是预扣口径，宁可多扣后退，不可少扣兜不住成本。
func (r *H3Request) ToVideoUsage() billing_setting.VideoUsage {
	usage := billing_setting.VideoUsage{
		Resolution:    r.Resolution,
		OutputSeconds: float64(r.Duration),
		ImageCount:    r.ImageCount(),
	}
	if len(r.RefVideos) > 0 {
		usage.VideoSeconds = H3MaxRefMediaSeconds
	}
	if len(r.RefAudios) > 0 {
		usage.AudioSeconds = H3MaxRefMediaSeconds
	}
	return usage
}

// ImageCount 是所有输入图片的张数。官方按张计费时不区分首尾帧与参考图，
// 三者一并计入。
func (r *H3Request) ImageCount() int {
	count := len(r.RefImages)
	if r.FirstFrame != "" {
		count++
	}
	if r.LastFrame != "" {
		count++
	}
	return count
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// normalizeH3Resolution 把用户传的分辨率/尺寸归一到 H3 的两个档位。
// 空值返回空（由 ApplyDefaults 兜底）；无法识别的值报错而不是静默降级——
// 用户要 1080P 却拿到 768P 且照 768P 计费，比直接报错更糟。
func normalizeH3Resolution(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	lower := strings.ToLower(value)
	switch {
	case lower == "2k", strings.Contains(lower, "2560"), strings.Contains(lower, "1440"):
		return H3Resolution2K, nil
	case lower == "768p", strings.Contains(lower, "768"):
		return H3Resolution768P, nil
	}
	return "", fmt.Errorf("unsupported resolution %q, %s expects %s or %s",
		value, ModelMiniMaxH3, H3Resolution768P, H3Resolution2K)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func trimAll(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func containsFold(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}
