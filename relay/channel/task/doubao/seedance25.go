package doubao

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// Seedance 2.5 接的是火山方舟官渠（Base URL 不含 zerocut.cn 时走官方分支），
// 与同包里 2.0 走 ZeroCut 聚合器的裸透传路径完全分开。本文件承担归一化层：
//
//	站内统一协议 (TaskSubmitReq) ─┐
//	                              ├─→ Seedance25Request ─┬─→ ToArkRequest() 上游请求
//	火山 v3 原生协议 (content[])  ─┘                     ├─→ ToVideoUsage() 计费口径
//	                                                     └─→ Validate()     参数校验
//
// 两条入口共用同一套校验与计费，是这套接入不出岔子的关键：任何一条路径上的
// 参数约束或计价口径改动，另一条自动跟着变。
//
// 2.0 那条路径为什么不这么做：它对接的是只认自己那套参数的第三方聚合器，
// 裸透传是有意为之；2.5 直连官方，能也应该把请求解析清楚——不然算不出钱。

// seedance25RequestContextKey 缓存归一化后的请求。校验、计费、构造上游请求体
// 三处都要用，解析只做一次。
const seedance25RequestContextKey = "seedance_2_5_request"

const (
	// ModelSeedance25 是本站对外暴露的模型名，ModelSeedance25Official 是火山
	// 的 endpoint 名。两个都要在 video_pricing 里配价，见 seedance25Pricing()。
	ModelSeedance25         = "doubao-seedance-2-5"
	ModelSeedance25Official = "doubao-seedance-2-5-260628"

	Seedance25Resolution480P = "480p"
	Seedance25Resolution720P = "720p"

	// Seedance25AutoDuration 是官方的「模型自选时长」取值，也是 2.5 的默认值。
	// 视频编辑任务只接受它。提交时拿不到真实时长，预扣口径见 Seedance25Usage。
	Seedance25AutoDuration = -1
	Seedance25MinDuration  = 4
	Seedance25MaxDuration  = 30

	Seedance25MaxRefImages = 30
	Seedance25MaxRefVideos = 10
	Seedance25MaxRefAudios = 10

	// Seedance25MaxRefMediaSeconds 是官方对参考视频/音频的总时长上限（各自
	// 合计 ≤30s）。请求里只有 URL 拿不到真实时长，预扣阶段按这个上限计费；
	// 终态再用上游返回的 token 用量重算，多退少补。
	Seedance25MaxRefMediaSeconds = 30

	Seedance25MinPriority = 0
	Seedance25MaxPriority = 9

	Seedance25MinExpiresAfter = 3600
	Seedance25MaxExpiresAfter = 259200

	// Seedance25DefaultFPS 是官方的出片帧率，token 换算的分母。上游查询响应
	// 会返回 framespersecond，缺失时回落到这里。
	Seedance25DefaultFPS = 24

	Seedance25OutputFormatMP4 = "mp4"
	Seedance25OutputFormatMOV = "mov"

	Seedance25ToolWebSearch = "web_search"
)

// 火山 v3 content 的 type 与 role 取值。
const (
	arkContentTypeText      = "text"
	arkContentTypeImageURL  = "image_url"
	arkContentTypeVideoURL  = "video_url"
	arkContentTypeAudioURL  = "audio_url"
	arkContentTypeDraftTask = "draft_task"

	arkRoleFirstFrame     = "first_frame"
	arkRoleLastFrame      = "last_frame"
	arkRoleReferenceImage = "reference_image"
	arkRoleReferenceVideo = "reference_video"
	arkRoleReferenceAudio = "reference_audio"
)

// arkStatusExpired 是官方的任务超时终态。漏掉它会让任务在轮询里被当成
// 「未知状态 → 进行中」，永远到不了终态，预扣的额度也就永远不释放。
const (
	arkStatusQueued    = "queued"
	arkStatusRunning   = "running"
	arkStatusSucceeded = "succeeded"
	arkStatusFailed    = "failed"
	arkStatusExpired   = "expired"
)

// seedance25RatioAdaptive 让输出宽高比跟随输入内容，是 2.5 的默认值；
// 首帧/首尾帧与视频编辑场景官方只支持它。
const seedance25RatioAdaptive = "adaptive"

var seedance25SupportedRatios = []string{
	seedance25RatioAdaptive, "21:9", "16:9", "4:3", "1:1", "3:4", "9:16",
}

// seedance25PixelTable 是官方给出的「分辨率 × 宽高比 → 宽高像素值」。
//
// 结算要用它把上游返回的 token 数换算回计费秒数：
// tokens = 宽 × 高 × 帧数 / 1024，所以 计费秒数 = tokens × 1024 / 像素 / 帧率。
var seedance25PixelTable = map[string]map[string]int{
	Seedance25Resolution480P: {
		"16:9": 854 * 480,
		"4:3":  752 * 560,
		"1:1":  640 * 640,
		"3:4":  560 * 752,
		"9:16": 480 * 854,
		"21:9": 992 * 432,
	},
	Seedance25Resolution720P: {
		"16:9": 1280 * 720,
		"4:3":  1112 * 834,
		"1:1":  960 * 960,
		"3:4":  834 * 1112,
		"9:16": 720 * 1280,
		"21:9": 1470 * 630,
	},
}

// IsSeedance25Model 判断模型是否走本文件这套官渠 2.5 逻辑。所有分流点都依赖它。
func IsSeedance25Model(modelName string) bool {
	canonical, _ := CanonicalSeedanceName(strings.ToLower(strings.TrimSpace(modelName)))
	return strings.EqualFold(canonical, ModelSeedance25Official)
}

// ---------------------------------------------------------------------------
// 上游协议结构
// ---------------------------------------------------------------------------

type ArkTool struct {
	Type string `json:"type"`
}

// ArkVideoRequest 是发往上游的创建任务请求体，字段范围严格取 2.5 支持的参数。
// 结构里没有的参数（seed / camera_fixed / frames / draft / service_tier）不是
// 遗漏，是 2.5 不支持——它们在 Validate 阶段就被明确拒掉了。
//
// 可选标量一律用指针 + omitempty：显式传 false / 0 必须原样送到上游，不能被
// omitempty 静默吞掉（CLAUDE.md Rule 6）。
type ArkVideoRequest struct {
	Model                 string        `json:"model"`
	Content               []ContentItem `json:"content"`
	Resolution            string        `json:"resolution,omitempty"`
	Ratio                 string        `json:"ratio,omitempty"`
	Duration              *int          `json:"duration,omitempty"`
	GenerateAudio         *bool         `json:"generate_audio,omitempty"`
	Watermark             *bool         `json:"watermark,omitempty"`
	ReturnLastFrame       *bool         `json:"return_last_frame,omitempty"`
	OutputFormat          string        `json:"output_format,omitempty"`
	Priority              *int          `json:"priority,omitempty"`
	ExecutionExpiresAfter *int          `json:"execution_expires_after,omitempty"`
	SafetyIdentifier      string        `json:"safety_identifier,omitempty"`
	Tools                 []ArkTool     `json:"tools,omitempty"`
}

// ArkIncomingRequest 解析客户端按火山 v3 原生协议发来的请求体。
//
// 数值字段用 dto.IntValue / dto.BoolValue 而不是 int / bool：官方 SDK 之外的
// 客户端常把 "duration": "5" 写成字符串，这两个类型两种写法都收。
type ArkIncomingRequest struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content"`
	Resolution            string         `json:"resolution"`
	Ratio                 string         `json:"ratio"`
	Duration              *dto.IntValue  `json:"duration"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio"`
	Watermark             *dto.BoolValue `json:"watermark"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame"`
	OutputFormat          string         `json:"output_format"`
	Priority              *dto.IntValue  `json:"priority"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after"`
	SafetyIdentifier      string         `json:"safety_identifier"`
	Tools                 []ArkTool      `json:"tools"`
	CallbackURL           string         `json:"callback_url"`

	// 以下是 2.5 不支持的参数，解析它们只为能明确报错。静默剔除更糟：用户
	// 以为 seed 生效了、拿到的却是随机结果，排查成本远高于一条报错。
	Seed        *dto.IntValue  `json:"seed"`
	CameraFixed *dto.BoolValue `json:"camera_fixed"`
	Frames      *dto.IntValue  `json:"frames"`
	Draft       *dto.BoolValue `json:"draft"`
	ServiceTier string         `json:"service_tier"`
}

// ---------------------------------------------------------------------------
// 归一化结构
// ---------------------------------------------------------------------------

type Seedance25Request struct {
	Prompt     string
	Resolution string
	Ratio      string
	// Duration 为 Seedance25AutoDuration(-1) 表示由模型自选时长。
	Duration              int
	FirstFrame            string
	LastFrame             string
	RefImages             []string
	RefVideos             []string
	RefAudios             []string
	GenerateAudio         *bool
	Watermark             *bool
	ReturnLastFrame       *bool
	OutputFormat          string
	Priority              *int
	ExecutionExpiresAfter *int
	SafetyIdentifier      string
	WebSearch             bool

	// unroledImages 暂存 role 不填的图片，等整个请求解析完再归位。
	unroledImages []string
}

// seedance25Metadata 是站内统一协议下 2.5 专有能力的载体。这些字段是 2.5
// 特有的，不值得为它们扩 TaskSubmitReq 的顶层字段——metadata 就是干这个用的。
type seedance25Metadata struct {
	Resolution  string `json:"resolution,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Ratio       string `json:"ratio,omitempty"`
	// Duration 是 metadata 里的时长。操练场把 schema 渲染出的参数整体塞进
	// metadata（不会提升到顶层字段），所以这里必须收一份，否则用户在面板上
	// 调的时长会被静默丢弃、按默认值出片。顶层 duration 优先级更高。
	Duration              *int     `json:"duration,omitempty"`
	FirstFrameImage       string   `json:"first_frame_image,omitempty"`
	LastFrameImage        string   `json:"last_frame_image,omitempty"`
	ReferenceImageURLs    []string `json:"reference_image_urls,omitempty"`
	ReferenceVideoURLs    []string `json:"reference_video_urls,omitempty"`
	ReferenceAudioURLs    []string `json:"reference_audio_urls,omitempty"`
	GenerateAudio         *bool    `json:"generate_audio,omitempty"`
	Watermark             *bool    `json:"watermark,omitempty"`
	ReturnLastFrame       *bool    `json:"return_last_frame,omitempty"`
	OutputFormat          string   `json:"output_format,omitempty"`
	Priority              *int     `json:"priority,omitempty"`
	ExecutionExpiresAfter *int     `json:"execution_expires_after,omitempty"`
	SafetyIdentifier      string   `json:"safety_identifier,omitempty"`
	// WebSearch 是 tools:[{type:"web_search"}] 的开关式写法，给操练场的
	// boolean 控件用；Tools 则让客户端直接按官方结构传。
	WebSearch *bool     `json:"web_search,omitempty"`
	Tools     []ArkTool `json:"tools,omitempty"`
	// Content 承载操练场/客户端上传的富媒体。结构与 v3 原生协议的 content[]
	// 完全一致（{type, image_url:{url}, role}），因此这里直接复用同一套解析。
	// 缺了它，操练场上传的首尾帧和参考素材会被静默丢弃。
	Content []ContentItem `json:"content,omitempty"`
}

// Seedance25FromTaskSubmitReq 把站内统一视频协议的请求归一化。
func Seedance25FromTaskSubmitReq(req relaycommon.TaskSubmitReq) (*Seedance25Request, error) {
	var meta seedance25Metadata
	if err := req.UnmarshalMetadata(&meta); err != nil {
		return nil, fmt.Errorf("parse metadata failed: %w", err)
	}

	r := &Seedance25Request{
		Prompt:                strings.TrimSpace(req.Prompt),
		Duration:              req.GetDuration(),
		Ratio:                 seedance25FirstNonEmpty(meta.AspectRatio, meta.Ratio),
		FirstFrame:            strings.TrimSpace(meta.FirstFrameImage),
		LastFrame:             strings.TrimSpace(meta.LastFrameImage),
		RefImages:             seedance25TrimAll(meta.ReferenceImageURLs),
		RefVideos:             seedance25TrimAll(meta.ReferenceVideoURLs),
		RefAudios:             seedance25TrimAll(meta.ReferenceAudioURLs),
		GenerateAudio:         meta.GenerateAudio,
		Watermark:             meta.Watermark,
		ReturnLastFrame:       meta.ReturnLastFrame,
		OutputFormat:          strings.ToLower(strings.TrimSpace(meta.OutputFormat)),
		Priority:              meta.Priority,
		ExecutionExpiresAfter: meta.ExecutionExpiresAfter,
		SafetyIdentifier:      strings.TrimSpace(meta.SafetyIdentifier),
	}

	// 时长的取值优先级：顶层 duration > metadata.duration > OpenAI 风格的
	// seconds。-1 是合法取值（模型自选时长），沿途不要把它当成"未设置"。
	if r.Duration == 0 && meta.Duration != nil {
		r.Duration = *meta.Duration
	}
	if r.Duration == 0 {
		if seconds := strings.TrimSpace(req.Seconds); seconds != "" {
			var parsed int
			if _, err := fmt.Sscanf(seconds, "%d", &parsed); err == nil {
				r.Duration = parsed
			}
		}
	}

	resolution, err := normalizeSeedance25Resolution(seedance25FirstNonEmpty(meta.Resolution, req.Size))
	if err != nil {
		return nil, err
	}
	r.Resolution = resolution

	if err := r.applyTools(meta.Tools); err != nil {
		return nil, err
	}
	if meta.WebSearch != nil && *meta.WebSearch {
		r.WebSearch = true
	}

	// images[0] / images[1] 是站内协议表达首尾帧的方式；metadata 里的显式
	// first/last_frame_image 优先级更高。
	images := seedance25TrimAll(req.Images)
	if r.FirstFrame == "" && len(images) > 0 {
		r.FirstFrame = images[0]
	}
	if r.LastFrame == "" && len(images) > 1 {
		r.LastFrame = images[1]
	}

	// 操练场按 schema 的 x-content-role 把上传的素材拍平成 metadata.content，
	// 结构与 v3 原生协议一致，直接复用同一套解析。
	if err := r.applyContent(meta.Content); err != nil {
		return nil, err
	}

	return r, nil
}

// Seedance25FromArkRequest 把火山 v3 原生请求归一化。
func Seedance25FromArkRequest(in *ArkIncomingRequest) (*Seedance25Request, error) {
	if in == nil {
		return nil, fmt.Errorf("request body is empty")
	}
	if err := seedance25RejectUnsupported(in); err != nil {
		return nil, err
	}

	resolution, err := normalizeSeedance25Resolution(in.Resolution)
	if err != nil {
		return nil, err
	}

	r := &Seedance25Request{
		Resolution:            resolution,
		Ratio:                 strings.TrimSpace(in.Ratio),
		OutputFormat:          strings.ToLower(strings.TrimSpace(in.OutputFormat)),
		SafetyIdentifier:      strings.TrimSpace(in.SafetyIdentifier),
		GenerateAudio:         seedance25Bool(in.GenerateAudio),
		Watermark:             seedance25Bool(in.Watermark),
		ReturnLastFrame:       seedance25Bool(in.ReturnLastFrame),
		Priority:              seedance25Int(in.Priority),
		ExecutionExpiresAfter: seedance25Int(in.ExecutionExpiresAfter),
	}
	if in.Duration != nil {
		r.Duration = int(*in.Duration)
	}

	if err := r.applyTools(in.Tools); err != nil {
		return nil, err
	}
	if err := r.applyContent(in.Content); err != nil {
		return nil, err
	}

	return r, nil
}

// seedance25RejectUnsupported 把 2.5 不支持的参数挡在网关这一层。
//
// 不静默剔除：这些参数在 1.x 上是有效的，用户从 1.5 pro 迁过来时传了 seed
// 却拿到随机结果，比一条明确报错难查得多。
func seedance25RejectUnsupported(in *ArkIncomingRequest) error {
	unsupported := make([]string, 0, 5)
	if in.Seed != nil {
		unsupported = append(unsupported, "seed")
	}
	if in.CameraFixed != nil {
		unsupported = append(unsupported, "camera_fixed")
	}
	if in.Frames != nil {
		unsupported = append(unsupported, "frames")
	}
	if in.Draft != nil {
		unsupported = append(unsupported, "draft")
	}
	// service_tier 只有 flex（离线推理）是 2.5 不支持的，default 等价于不填
	if tier := strings.ToLower(strings.TrimSpace(in.ServiceTier)); tier != "" && tier != "default" {
		unsupported = append(unsupported, "service_tier="+tier)
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("%s does not support: %s",
			ModelSeedance25, strings.Join(unsupported, ", "))
	}
	return nil
}

func (r *Seedance25Request) applyTools(tools []ArkTool) error {
	for i, tool := range tools {
		switch strings.TrimSpace(tool.Type) {
		case Seedance25ToolWebSearch:
			r.WebSearch = true
		case "":
			return fmt.Errorf("tools[%d]: type is required", i)
		default:
			return fmt.Errorf("tools[%d]: unsupported tool type %q", i, tool.Type)
		}
	}
	return nil
}

// applyContent 解析 v3 协议的 content 数组。站内统一协议下操练场也用同一套
// 结构（metadata.content），因此两条入口共用这段解析。
func (r *Seedance25Request) applyContent(content []ContentItem) error {
	for i, item := range content {
		switch item.Type {
		case arkContentTypeText:
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
		case arkContentTypeImageURL:
			url, err := seedance25ContentURL(i, item.ImageURL, "image_url")
			if err != nil {
				return err
			}
			switch item.Role {
			case arkRoleFirstFrame:
				r.FirstFrame = url
			case arkRoleReferenceImage:
				r.RefImages = append(r.RefImages, url)
			case arkRoleLastFrame:
				r.LastFrame = url
			case "":
				// role 不填的含义要看整个请求，解析完再归位，见 resolveUnroledImages
				r.unroledImages = append(r.unroledImages, url)
			default:
				return fmt.Errorf("content[%d]: unsupported role %q for image_url", i, item.Role)
			}
		case arkContentTypeVideoURL:
			url, err := seedance25ContentURL(i, item.VideoURL, "video_url")
			if err != nil {
				return err
			}
			if item.Role != arkRoleReferenceVideo && item.Role != "" {
				return fmt.Errorf("content[%d]: unsupported role %q for video_url", i, item.Role)
			}
			r.RefVideos = append(r.RefVideos, url)
		case arkContentTypeAudioURL:
			url, err := seedance25ContentURL(i, item.AudioURL, "audio_url")
			if err != nil {
				return err
			}
			if item.Role != arkRoleReferenceAudio && item.Role != "" {
				return fmt.Errorf("content[%d]: unsupported role %q for audio_url", i, item.Role)
			}
			r.RefAudios = append(r.RefAudios, url)
		case arkContentTypeDraftTask:
			// 样片模式是 Seedance 1.5 pro 的能力
			return fmt.Errorf("content[%d]: %s does not support draft_task", i, ModelSeedance25)
		default:
			return fmt.Errorf("content[%d]: unsupported type %q", i, item.Type)
		}
	}
	return nil
}

func seedance25ContentURL(index int, ref *MediaURL, field string) (string, error) {
	if ref == nil || strings.TrimSpace(ref.URL) == "" {
		return "", fmt.Errorf("content[%d]: %s.url is required", index, field)
	}
	return strings.TrimSpace(ref.URL), nil
}

// ---------------------------------------------------------------------------
// 默认值与校验
// ---------------------------------------------------------------------------

// ApplyDefaults 填充官方默认值。必须在 Validate 之前调用。
func (r *Seedance25Request) ApplyDefaults() {
	r.resolveUnroledImages()
	if r.Duration == 0 {
		r.Duration = Seedance25AutoDuration
	}
	if r.Resolution == "" {
		r.Resolution = Seedance25Resolution720P
	}
}

// resolveUnroledImages 决定 role 不填的图片算什么。
//
// 官方对「图生视频-首帧」的定义是「传入 1 个 image_url 对象，role 为
// first_frame 或不填」——所以孤零零一张无角色图片是**首帧**，不是参考图。
// 这两者是互斥的两种场景，认错了出片效果完全不同（首帧是从这张图动起来，
// 参考图是照着它的人物/风格重新生成）。
//
// 只有「整个请求就这一张图、且没有任何其它参考素材」时才认定为首帧：混着别的
// 素材时官方没有定义 role 缺省的含义，一律按参考图处理，交给互斥校验和上游。
func (r *Seedance25Request) resolveUnroledImages() {
	if len(r.unroledImages) == 0 {
		return
	}
	onlyImage := len(r.unroledImages) == 1 &&
		r.FirstFrame == "" && r.LastFrame == "" &&
		len(r.RefImages) == 0 && len(r.RefVideos) == 0 && len(r.RefAudios) == 0
	if onlyImage {
		r.FirstFrame = r.unroledImages[0]
	} else {
		r.RefImages = append(r.RefImages, r.unroledImages...)
	}
	r.unroledImages = nil
}

// Validate 只校验官方文档明确写了的约束。没写的（如首尾帧与参考素材混用后
// 上游具体怎么判定视频编辑任务）交给上游——网关不该发明上游没有的限制。
func (r *Seedance25Request) Validate() error {
	// 2.5 的提示词可选：纯首尾帧、甚至只传音频都是官方支持的组合。但总得有
	// 点东西，四类内容全空的请求在网关这层就该拒掉。
	if !r.HasAnyContent() {
		return fmt.Errorf("at least one of prompt, image, video or audio is required")
	}

	if r.Duration != Seedance25AutoDuration &&
		(r.Duration < Seedance25MinDuration || r.Duration > Seedance25MaxDuration) {
		return fmt.Errorf("duration must be %d (model decides) or between %d and %d seconds",
			Seedance25AutoDuration, Seedance25MinDuration, Seedance25MaxDuration)
	}

	if r.Resolution != Seedance25Resolution480P && r.Resolution != Seedance25Resolution720P {
		return fmt.Errorf("resolution must be one of %s, %s",
			Seedance25Resolution480P, Seedance25Resolution720P)
	}

	if r.Ratio != "" && !seedance25ContainsFold(seedance25SupportedRatios, r.Ratio) {
		return fmt.Errorf("ratio must be one of %s", strings.Join(seedance25SupportedRatios, ", "))
	}

	// 首帧 / 首尾帧 / 多模态参考是三种互斥场景，混用上游直接报错
	hasFrame := r.FirstFrame != "" || r.LastFrame != ""
	hasReference := len(r.RefImages) > 0 || len(r.RefVideos) > 0 || len(r.RefAudios) > 0
	if hasFrame && hasReference {
		return fmt.Errorf("first_frame/last_frame and reference materials are mutually exclusive scenarios")
	}
	if r.LastFrame != "" && r.FirstFrame == "" {
		return fmt.Errorf("last_frame requires first_frame")
	}
	// 首帧/首尾帧场景官方「默认且仅支持 adaptive」——输出宽高比强制跟随首帧
	// 图片。提前拒掉，比让用户拿一个上游的报错回来清楚。
	if hasFrame && r.Ratio != "" && !strings.EqualFold(r.Ratio, seedance25RatioAdaptive) {
		return fmt.Errorf(
			"first_frame/last_frame scenarios only support ratio %q (output follows the first frame)",
			seedance25RatioAdaptive)
	}

	if len(r.RefImages) > Seedance25MaxRefImages {
		return fmt.Errorf("at most %d reference images are allowed", Seedance25MaxRefImages)
	}
	if len(r.RefVideos) > Seedance25MaxRefVideos {
		return fmt.Errorf("at most %d reference videos are allowed", Seedance25MaxRefVideos)
	}
	if len(r.RefAudios) > Seedance25MaxRefAudios {
		return fmt.Errorf("at most %d reference audios are allowed", Seedance25MaxRefAudios)
	}

	switch r.OutputFormat {
	case "", Seedance25OutputFormatMP4, Seedance25OutputFormatMOV:
	default:
		return fmt.Errorf("output_format must be %s or %s",
			Seedance25OutputFormatMP4, Seedance25OutputFormatMOV)
	}

	if r.Priority != nil && (*r.Priority < Seedance25MinPriority || *r.Priority > Seedance25MaxPriority) {
		return fmt.Errorf("priority must be between %d and %d",
			Seedance25MinPriority, Seedance25MaxPriority)
	}

	if r.ExecutionExpiresAfter != nil &&
		(*r.ExecutionExpiresAfter < Seedance25MinExpiresAfter ||
			*r.ExecutionExpiresAfter > Seedance25MaxExpiresAfter) {
		return fmt.Errorf("execution_expires_after must be between %d and %d seconds",
			Seedance25MinExpiresAfter, Seedance25MaxExpiresAfter)
	}

	if len(r.SafetyIdentifier) > 64 {
		return fmt.Errorf("safety_identifier must not exceed 64 characters")
	}

	return nil
}

// HasAnyContent 判断请求是否带了任何一类内容。
func (r *Seedance25Request) HasAnyContent() bool {
	return r.Prompt != "" || r.FirstFrame != "" || r.LastFrame != "" ||
		len(r.RefImages) > 0 || len(r.RefVideos) > 0 || len(r.RefAudios) > 0
}

// ---------------------------------------------------------------------------
// 出口
// ---------------------------------------------------------------------------

// ToArkRequest 构造上游请求体。content 顺序固定（文本 → 首尾帧 → 参考素材），
// 保证同一个语义请求每次序列化结果一致，便于比对和排查。
func (r *Seedance25Request) ToArkRequest(upstreamModel string) *ArkVideoRequest {
	content := make([]ContentItem, 0, 1+len(r.RefImages)+len(r.RefVideos)+len(r.RefAudios)+2)
	if r.Prompt != "" {
		content = append(content, ContentItem{Type: arkContentTypeText, Text: r.Prompt})
	}
	if r.FirstFrame != "" {
		content = append(content, seedance25ImageContent(r.FirstFrame, arkRoleFirstFrame))
	}
	if r.LastFrame != "" {
		content = append(content, seedance25ImageContent(r.LastFrame, arkRoleLastFrame))
	}
	for _, url := range r.RefImages {
		content = append(content, seedance25ImageContent(url, arkRoleReferenceImage))
	}
	for _, url := range r.RefVideos {
		content = append(content, ContentItem{
			Type: arkContentTypeVideoURL, VideoURL: &MediaURL{URL: url}, Role: arkRoleReferenceVideo,
		})
	}
	for _, url := range r.RefAudios {
		content = append(content, ContentItem{
			Type: arkContentTypeAudioURL, AudioURL: &MediaURL{URL: url}, Role: arkRoleReferenceAudio,
		})
	}

	req := &ArkVideoRequest{
		Model:                 upstreamModel,
		Content:               content,
		Resolution:            r.Resolution,
		Ratio:                 r.Ratio,
		Duration:              &r.Duration,
		GenerateAudio:         r.GenerateAudio,
		Watermark:             r.Watermark,
		ReturnLastFrame:       r.ReturnLastFrame,
		OutputFormat:          r.OutputFormat,
		Priority:              r.Priority,
		ExecutionExpiresAfter: r.ExecutionExpiresAfter,
		SafetyIdentifier:      r.SafetyIdentifier,
		// CallbackURL 不透传：任务状态由网关轮询，用户的回调地址由网关自己的
		// 回调机制处理，直接转给上游会让上游绕过网关回调用户。
	}
	if r.WebSearch {
		req.Tools = []ArkTool{{Type: Seedance25ToolWebSearch}}
	}
	return req
}

func seedance25ImageContent(url, role string) ContentItem {
	return ContentItem{Type: arkContentTypeImageURL, ImageURL: &MediaURL{URL: url}, Role: role}
}

// ImageCount 是所有输入图片的张数（首帧/尾帧/参考图合计）。
func (r *Seedance25Request) ImageCount() int {
	count := len(r.RefImages)
	if r.FirstFrame != "" {
		count++
	}
	if r.LastFrame != "" {
		count++
	}
	return count
}

// ToVideoUsage 产出预扣口径。
//
// fallbackOutputSeconds 用于 duration=-1（模型自选时长）：提交时没有任何锚点，
// 只能按价目表配的折中值预扣，终态再用上游返回的 token 用量多退少补。
//
// 参考视频只有 URL、拿不到真实时长，按官方总时长上限（30s）计费——宁可多扣
// 后退，不可少扣兜不住成本。参考音频与图片不计入金额（火山不单独收费），
// 但张数照记，好让日志里的计费明细能还原当时的请求形态。
func (r *Seedance25Request) ToVideoUsage(fallbackOutputSeconds float64) billing_setting.VideoUsage {
	usage := billing_setting.VideoUsage{
		Resolution: r.Resolution,
		ImageCount: r.ImageCount(),
	}
	if r.Duration > 0 {
		usage.OutputSeconds = float64(r.Duration)
	} else {
		usage.OutputSeconds = fallbackOutputSeconds
	}
	if len(r.RefVideos) > 0 {
		usage.VideoSeconds = Seedance25MaxRefMediaSeconds
		usage.HasVideoInput = true
	}
	return usage
}

// Seedance25Usage 产出预扣口径，并在无法定价时报错。
//
// duration 未知（-1）而价目表又没配 MaxOutputSeconds 时必须报错：按 0 秒预扣
// 等于放行一个不知道该收多少钱的任务，终态即使能结算，中间这段时间用户的额度
// 也没被冻住。
func Seedance25Usage(model string, r *Seedance25Request) (billing_setting.VideoUsage, error) {
	pricing, ok := billing_setting.GetVideoPricing(model)
	if !ok {
		return billing_setting.VideoUsage{},
			fmt.Errorf("video pricing not configured for model %s", model)
	}
	if r.Duration <= 0 && pricing.MaxOutputSeconds <= 0 {
		return billing_setting.VideoUsage{}, fmt.Errorf(
			"model %s uses model-decided duration but max_output_seconds is not configured", model)
	}
	return r.ToVideoUsage(pricing.MaxOutputSeconds), nil
}

// ---------------------------------------------------------------------------
// 结算：token → 计费秒数
// ---------------------------------------------------------------------------

// Seedance25BillableSeconds 把上游返回的计费 token 数换算回计费秒数。
//
// 上游公式是 tokens = 宽 × 高 × 帧数 / 1024（参考视频按 24fps 全采样并折算到
// 输出分辨率），所以 计费秒数 = tokens × 1024 / 像素 / 帧率。换算出来的值等于
// 「输出秒数 + 参考视频秒数」，也就是我们价目表的计费口径。
//
// 用 token 而不是响应里的 duration 字段做结算真值，是因为 duration 只是输出
// 时长：它既不含参考视频的贡献，也不含官方对含视频输入任务的最低 token 用量
// 下限。token 是上游真正的计费口径，这些都已经算在里面了。
func Seedance25BillableSeconds(tokens int, resolution, ratio string, fps int) (float64, bool) {
	if tokens <= 0 {
		return 0, false
	}
	pixels, ok := seedance25Pixels(resolution, ratio)
	if !ok {
		return 0, false
	}
	if fps <= 0 {
		fps = Seedance25DefaultFPS
	}
	return float64(tokens) * 1024 / float64(pixels) / float64(fps), true
}

// seedance25Pixels 查「分辨率 × 宽高比 → 像素数」。
//
// ratio 为空或 adaptive（上游没回具体比例）时按 16:9 算——那是价目表的定价
// 基准。720p 各比例像素差 <1%，480p 最多差 4.5%，落在这个量级的误差可接受；
// 分辨率认不出来则返回 false，由调用方放弃结算而不是猜一个数字去扣钱。
func seedance25Pixels(resolution, ratio string) (int, bool) {
	byRatio, ok := seedance25PixelTable[strings.ToLower(strings.TrimSpace(resolution))]
	if !ok {
		return 0, false
	}
	if pixels, ok := byRatio[strings.TrimSpace(ratio)]; ok {
		return pixels, true
	}
	return byRatio["16:9"], true
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// normalizeSeedance25Resolution 把用户传的分辨率/尺寸归一到 2.5 的两个档位。
// 空值返回空（由 ApplyDefaults 兜底）；1080p / 4k 明确报错而不是静默降级——
// 用户要 1080p 却拿到 720p，比直接报错更糟。
func normalizeSeedance25Resolution(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	lower := strings.ToLower(value)
	switch {
	case lower == "480p", strings.Contains(lower, "480"):
		return Seedance25Resolution480P, nil
	case lower == "720p", strings.Contains(lower, "720"):
		return Seedance25Resolution720P, nil
	}
	return "", fmt.Errorf("unsupported resolution %q, %s expects %s or %s",
		value, ModelSeedance25, Seedance25Resolution480P, Seedance25Resolution720P)
}

func seedance25Bool(v *dto.BoolValue) *bool {
	if v == nil {
		return nil
	}
	b := bool(*v)
	return &b
}

func seedance25Int(v *dto.IntValue) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func seedance25FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func seedance25TrimAll(values []string) []string {
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

func seedance25ContainsFold(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}
