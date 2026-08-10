package billing_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/samber/lo"
)

// 视频模型定价：与 token 计费（ratio / tiered_expr）完全独立的一套。
//
// 视频任务走 ModelPriceHelperPerCall（按次基础额度）+ OtherRatios 连乘，
// 表达式计费对它无效——billingexpr 的变量全是 token 维度。因此这里用
// "配置里直接写美元价、算出整单金额、只回一个 ratio" 的方式接进计费管道：
//
//	ModelPrice[model] = 1.0                        // 哨兵基准价 $1
//	EstimateBilling  → {"video_cost": total}       // 整单美元
//	Quota = 1.0 × QuotaPerUnit × GroupRatio × total
//
// 之所以不拆成 seconds × resolution × ... 多个倍率连乘：输入素材的费用是
// 加法（图片按张、音视频按秒各自加钱），塞进乘法管道会产生顺序依赖，稍一
// 改动就算错钱。加法归加法，只在最外层留一个乘数给分组倍率。
const (
	VideoPricingModule = "video_pricing_setting"
	VideoPricingField  = "pricing"
	// VideoPricingOptionKey 是 DB / option 接口里的完整键名。
	VideoPricingOptionKey = VideoPricingModule + "." + VideoPricingField

	VideoPricingUnitSecond = "second"
)

// VideoBasePrice 是视频模型的按次基准价（美元）。
//
// 视频任务的额度是 基准价 × QuotaPerUnit × 分组倍率 × video_cost，其中
// video_cost 是价目表算出的整单美元金额。基准价固定为 $1 作哨兵，真实报价
// 完全由价目表决定，因此**不需要**管理员在 ModelPrice 里给视频模型配价。
const VideoBasePrice = 1.0

// VideoCostRatioKey 是适配器返回整单金额时使用的 OtherRatio 键名。
// 预扣链路据此校验「配了视频价目表的模型，适配器确实产出了金额」。
const VideoCostRatioKey = "video_cost"

// VideoPricing 是单个视频模型的完整价目表。所有价格均为美元。
type VideoPricing struct {
	// Unit 目前只有 "second"，保留字段以便将来出现按帧/按次的模型。
	Unit string `json:"unit,omitempty"`
	// DefaultResolution 在请求未指定分辨率时使用，必须是 Output 的一个键。
	DefaultResolution string `json:"default_resolution,omitempty"`
	// Output 是输出分辨率 → 每秒单价。
	Output map[string]float64 `json:"output"`
	// OutputWithVideoInput 在输入含参考视频时【整单】覆盖 Output。
	//
	// 这是火山方舟 Seedance 的规则：含视频输入的任务换一档更低的 token 单价，
	// 且降档作用于输出帧和输入帧的全部 token，不是只对输入部分打折。缺省
	// （或某个分辨率缺项）时回落到 Output，因此不影响没有这套规则的模型。
	OutputWithVideoInput map[string]float64 `json:"output_with_video_input,omitempty"`
	// MaxOutputSeconds 是输出时长未知时的预扣口径（秒）。
	//
	// Seedance 2.5 的 duration 默认 -1（模型自选时长，最长 30s），提交时没有
	// 任何锚点；这里配一个折中值预扣，终态再按上游真实用量多退少补。为 0 表示
	// 该模型的时长总是已知，不需要兜底。
	MaxOutputSeconds float64 `json:"max_output_seconds,omitempty"`
	// Input 是输入素材计价，各项可缺省（缺省即不计费）。
	Input VideoInputPricing `json:"input,omitempty"`
}

type VideoInputPricing struct {
	Image *VideoImagePricing      `json:"image,omitempty"`
	Audio *VideoSecondsPricing    `json:"audio,omitempty"`
	Video *VideoInputVideoPricing `json:"video,omitempty"`
}

type VideoImagePricing struct {
	PerImage float64 `json:"per_image"`
	// FreeCount 是免费张数。官方 H3 是前 5 张免费，本站按张计费故默认 0；
	// 想精确对齐官方改成 5 即可。
	FreeCount int `json:"free_count,omitempty"`
}

type VideoSecondsPricing struct {
	PerSecond float64 `json:"per_second"`
}

// VideoInputVideoPricing 的单价按【输出】分辨率分档——这是 MiniMax H3 的
// 规则（参考视频按其输入时长计费，但单价取决于输出分辨率），也是通用能力。
type VideoInputVideoPricing struct {
	PerSecondByOutputResolution map[string]float64 `json:"per_second_by_output_resolution"`
}

// VideoUsage 是适配器解析请求后得到的计费口径。适配器只负责填这个结构，
// 不接触任何价格数字。
type VideoUsage struct {
	Resolution    string  // 输出分辨率，空则用 DefaultResolution
	OutputSeconds float64 // 输出视频时长
	ImageCount    int     // 输入图片张数（首帧/尾帧/参考图合计）
	AudioSeconds  float64 // 输入音频总时长
	VideoSeconds  float64 // 输入视频总时长
	// HasVideoInput 表示原请求带了参考视频，决定走哪一档单价。
	//
	// 之所以不直接用 VideoSeconds > 0 判断：结算时上游只给一个合并的计费用量
	// （Seedance 是 token），拆不出输出/输入各占多少，此时会把总量记在
	// OutputSeconds 上而 VideoSeconds 为 0——档位却仍然必须按「含视频输入」取，
	// 否则会按不含视频的高单价收钱。VideoSeconds > 0 时本字段可以不填。
	HasVideoInput bool
}

// VideoCostBreakdown 是计费明细，写进日志和 task 快照供对账用。
type VideoCostBreakdown struct {
	Resolution string  `json:"resolution"`
	Output     float64 `json:"output"`
	Image      float64 `json:"image"`
	Audio      float64 `json:"audio"`
	Video      float64 `json:"video"`
	Total      float64 `json:"total"`
}

type VideoPricingSetting struct {
	// Pricing 只存管理员的覆盖项。未覆盖的模型回落到内置默认表，
	// 避免管理员保存一份不含某模型的配置后该模型被静默按 $0 计费。
	Pricing map[string]VideoPricing `json:"pricing"`
}

var videoPricingSetting = VideoPricingSetting{
	Pricing: make(map[string]VideoPricing),
}

func init() {
	config.GlobalConfig.Register(VideoPricingModule, &videoPricingSetting)
}

// defaultVideoPricing 是随代码发布的内置价目表，数值照抄上游官方价目页。
// 管理员在面板上改的是覆盖项，不会动这里。
//
// 构建一次并复用：GetVideoPricing 既在请求热路径上（EstimateBilling），
// 也会被定价页对每个模型各调一次，不能每次都重新分配整张表。
// 只读使用，任何对外暴露都走 Copy 版本。
var defaultVideoPricing = buildDefaultVideoPricing()

func buildDefaultVideoPricing() map[string]VideoPricing {
	return map[string]VideoPricing{
		// Alibaba Model Studio HappyHorse 官方按输出分辨率和计费时长收费。
		// 1.1 生成模型的国际站 480P 单价为 $0.07/秒。1.0 API 支持 480P；
		// 当前价格页未单列该档，暂按 1.1 的 480P 单价 $0.07/秒计费。
		// Video Edit 仅支持 720P / 1080P，必须使用独立价目表。
		// 管理员可通过 video_pricing_setting.pricing 覆盖这里的默认单价。
		// https://modelstudio.console.alibabacloud.com/ap-southeast-1?tab=api#/api/?type=model&url=3029821
		// https://modelstudio.console.alibabacloud.com/ap-southeast-1?tab=api#/api/?type=model&url=3030778
		// https://modelstudio.console.alibabacloud.com/ap-southeast-1?tab=api#/api/?type=model&url=3030779
		"happyhorse-1.1-t2v":        happyHorse11GenerationPricing(),
		"happyhorse-1.1-i2v":        happyHorse11GenerationPricing(),
		"happyhorse-1.1-r2v":        happyHorse11GenerationPricing(),
		"happyhorse-1.0-t2v":        happyHorse10GenerationPricing(),
		"happyhorse-1.0-i2v":        happyHorse10GenerationPricing(),
		"happyhorse-1.0-r2v":        happyHorse10GenerationPricing(),
		"happyhorse-1.0-video-edit": happyHorse10VideoEditPricing(),

		// https://platform.minimax.io/docs/guides/pricing-paygo
		// 输出：2K $0.13/s、768P $0.08/s
		// 输入：音频免费；图片官方前 5 张免费之后 $0.04/张（本站不设免费额度）；
		//      视频按输入时长计费，单价取输出分辨率档位。
		"MiniMax-H3": {
			Unit:              VideoPricingUnitSecond,
			DefaultResolution: "2K",
			Output: map[string]float64{
				"768P": 0.08,
				"2K":   0.13,
			},
			Input: VideoInputPricing{
				Image: &VideoImagePricing{PerImage: 0.04, FreeCount: 0},
				Audio: &VideoSecondsPricing{PerSecond: 0},
				Video: &VideoInputVideoPricing{
					PerSecondByOutputResolution: map[string]float64{
						"768P": 0.08,
						"2K":   0.13,
					},
				},
			},
		},

		"doubao-seedance-2-5": seedance25Pricing(),
	}
}

func happyHorse11GenerationPricing() VideoPricing {
	return VideoPricing{
		Unit:              VideoPricingUnitSecond,
		DefaultResolution: "1080P",
		Output: map[string]float64{
			"480P":  0.07,
			"720P":  0.14,
			"1080P": 0.18,
		},
	}
}

func happyHorse10GenerationPricing() VideoPricing {
	return VideoPricing{
		Unit:              VideoPricingUnitSecond,
		DefaultResolution: "1080P",
		Output: map[string]float64{
			"480P":  0.07,
			"720P":  0.14,
			"1080P": 0.24,
		},
	}
}

func happyHorse10VideoEditPricing() VideoPricing {
	return VideoPricing{
		Unit:              VideoPricingUnitSecond,
		DefaultResolution: "1080P",
		Output: map[string]float64{
			"720P":  0.14,
			"1080P": 0.24,
		},
	}
}

// defaultVideoPricingAliases 是同一个模型的其它可用名字。
//
// GetVideoPricing 是精确查表，查表键是 OriginModelName（调用方写的那个名字）。
// 少配一个，那条路径就会掉回 ModelPrice 查表，同一个模型的两个名字算出不同的
// 钱。这几个名字与 relay/channel/task/doubao/constants.go 的 seedanceAliasMap
// 一一对应——那张表认得的名字都会被路由到 2.5，这里就都得能定价。
//
// 与 defaultVideoPricing 分开放，是因为管理端要展示内置价目表：别名混在一起
// 会让定价面板上冒出好几行一模一样的记录。DefaultVideoPricingJSON 只吐规范名。
var defaultVideoPricingAliases = map[string]VideoPricing{
	"doubao-seedance-2-5-260628": seedance25Pricing(), // 火山官方 endpoint 名
	"doubao-seedance-2.5":        seedance25Pricing(),
	"seedance-2.5":               seedance25Pricing(),
	"seedance-2.5-api":           seedance25Pricing(),
}

// seedance25Pricing 是火山方舟 doubao-seedance-2.5 的价目表。
//
// 上游按 token 计费，公式为 tokens = 宽 × 高 × 帧数 / 1024（参考视频按 24fps
// 全采样，且按【输出】分辨率折算），单价分两档：
//
//	输入不含视频：$10.37 / 百万 token
//	输入含视频：  $6.23  / 百万 token（降档作用于输出+输入的全部 token）
//
// 同一分辨率下不同宽高比的像素数不一样，每秒 token 数也就不一样，而秒价一档
// 分辨率只有一个数。这里按【该分辨率下像素最多的那个宽高比】定价，保证任何
// 比例都不会成本倒挂——代价是最常用的 16:9 会比官方 16:9 报价高一点点
// （720p +0.63%、480p +4.54%）。
//
//	480p 最贵档 21:9   992×432 → 10,044.0   tok/s
//	720p 最贵档 4:3   1112×834 → 21,736.125 tok/s
//
// 折算成每秒单价（5 位小数，一律向上取整，避免舍入造成少收）：
//
//	480p → 不含视频 $0.10416/s、含视频 $0.06258/s
//	720p → 不含视频 $0.22541/s、含视频 $0.13542/s
//
// 公式与档位已用官方人民币价目表的六个锚点交叉验证过（16:9 口径误差 ≤0.2%）。
//
// 输入素材：火山对参考图片与音频不单独计价（不配 = 免费），参考视频则按其
// 自身时长实打实加钱——官方 720p 输出 5 秒是 7.56 元，加 30 秒参考视频变成
// 31.75 元，多出来的 24.19 元就是参考视频。它的单价与「含视频输入」档的输出
// 单价相同。
//
// 官方对「含视频输入」另有最低 token 用量限制（实测锚点折合 216 帧 ≈ 9 秒），
// 这里不建模：结算认上游返回的 completion_tokens，下限已经含在里面；预扣又按
// 参考视频 30s 上界估，恒高于下限，咬不到。
func seedance25Pricing() VideoPricing {
	return VideoPricing{
		Unit:              VideoPricingUnitSecond,
		DefaultResolution: "720p",
		// duration 默认 -1（模型自选，最长 30s），按 15s 折中预扣，终态多退少补
		MaxOutputSeconds: 15,
		Output: map[string]float64{
			"480p": 0.10416,
			"720p": 0.22541,
		},
		OutputWithVideoInput: map[string]float64{
			"480p": 0.06258,
			"720p": 0.13542,
		},
		Input: VideoInputPricing{
			// 参考视频按其自身时长收费，单价与「含视频输入」档的输出单价相同
			// ——上游是同一个 token 单价、同一个输出分辨率，两者本就该相等。
			// 调价时这两张表要一起改。
			Video: &VideoInputVideoPricing{
				PerSecondByOutputResolution: map[string]float64{
					"480p": 0.06258,
					"720p": 0.13542,
				},
			},
			// 参考图片与音频不配：火山对它们不单独计价，缺省即免费
		},
	}
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

// VideoPricingDefaultsOptionKey 是内置价目表在 option 接口里的只读键名。
//
// 它不挂在 VideoPricingModule 前缀下——那个前缀由 config 模块负责读写，多一个
// 结构体里不存在的字段只会带来麻烦。这里走的是和 CompletionRatioMeta 一样的
// 「派生只读项」路子：GetOptions 现算现给，UpdateOption 不接受它。
const VideoPricingDefaultsOptionKey = "VideoPricingDefaults"

// VideoPricingAliasesOptionKey 是内置别名价目表在 option 接口里的只读键名。
const VideoPricingAliasesOptionKey = "VideoPricingAliases"

// DefaultVideoPricingJSON 把内置价目表的【规范名】序列化给管理端展示。
//
// 管理端的模型定价面板只拿得到 DB 里的覆盖项。不给它这份数据，靠内置定价跑的
// 视频模型（HappyHorse、MiniMax-H3、Seedance 2.5）在面板上会显示成
// 「按量计费 + 空倍率」，管理员会以为没配价，然后去填一个根本不会生效的
// ModelRatio——面板说的和实际扣的钱对不上，是最难查的一类问题。
func DefaultVideoPricingJSON() string {
	jsonBytes, err := common.Marshal(defaultVideoPricing)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

// VideoPricingAliasesJSON 把内置别名价目表序列化给管理端。
//
// 与规范名分开给，是因为两者在面板上的用途不同：规范名决定「列表里有哪些行」，
// 别名只决定「这一行怎么渲染」。混在一起会让定价面板冒出好几行一模一样的记录；
// 完全不给，管理员把渠道模型名配成 doubao-seedance-2.5 这种别名时，面板又会
// 把它显示成没配价的按量计费模型。
func VideoPricingAliasesJSON() string {
	jsonBytes, err := common.Marshal(defaultVideoPricingAliases)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

// GetVideoPricing 返回模型的有效价目表：
// 管理员覆盖 > 内置规范名 > 内置别名。
func GetVideoPricing(model string) (VideoPricing, bool) {
	if p, ok := videoPricingSetting.Pricing[model]; ok {
		return p, true
	}
	if p, ok := defaultVideoPricing[model]; ok {
		return p, true
	}
	p, ok := defaultVideoPricingAliases[model]
	return p, ok
}

// ResolveVideoPricing 按候选顺序返回首个可用的视频价目表。
// 模型映射场景应先传用户侧模型名，再传上游规范模型名：管理员若为别名配置了
// 覆盖价则优先采用；未配置时自动回退规范模型的内置价目表。
func ResolveVideoPricing(models ...string) (string, VideoPricing, bool) {
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		if pricing, ok := GetVideoPricing(model); ok {
			return model, pricing, true
		}
	}
	return "", VideoPricing{}, false
}

// ---------------------------------------------------------------------------
// Cost computation
// ---------------------------------------------------------------------------

// ComputeVideoCost 按模型价目表算出整单美元金额。
//
// 返回 error 而不是静默按 0 计费：模型没配价、或分辨率落不到任何档位，都必须
// 让调用方在校验阶段就把请求拒掉，绝不能放行一个不知道该收多少钱的任务。
func ComputeVideoCost(model string, u VideoUsage) (VideoCostBreakdown, error) {
	pricing, ok := GetVideoPricing(model)
	if !ok {
		return VideoCostBreakdown{}, fmt.Errorf("video pricing not configured for model %s", model)
	}
	return pricing.Compute(u)
}

func (p VideoPricing) Compute(u VideoUsage) (VideoCostBreakdown, error) {
	resolution, ok := p.ResolveResolution(u.Resolution)
	if !ok {
		return VideoCostBreakdown{}, fmt.Errorf(
			"video pricing has no output tier for resolution %q (configured: %s)",
			u.Resolution, strings.Join(lo.Keys(p.Output), ", "))
	}

	b := VideoCostBreakdown{Resolution: resolution}

	if u.OutputSeconds > 0 {
		b.Output = p.OutputRate(resolution, u.HasVideoInput || u.VideoSeconds > 0) * u.OutputSeconds
	}

	if img := p.Input.Image; img != nil {
		if billable := u.ImageCount - img.FreeCount; billable > 0 {
			b.Image = float64(billable) * img.PerImage
		}
	}

	if audio := p.Input.Audio; audio != nil && u.AudioSeconds > 0 {
		b.Audio = audio.PerSecond * u.AudioSeconds
	}

	// 输入视频按【输出】分辨率取单价。未配置该项、或该分辨率缺档，都视为
	// 免费而不是报错——输出档位已经在上面校验过，这里缺项是价目表的有意留白：
	// 有的上游对参考素材完全不收费。
	if video := p.Input.Video; video != nil && u.VideoSeconds > 0 {
		if rate, ok := video.PerSecondByOutputResolution[resolution]; ok {
			b.Video = rate * u.VideoSeconds
		}
	}

	b.Total = b.Output + b.Image + b.Audio + b.Video
	return b, nil
}

// OutputRate 返回输出秒的单价。resolution 必须已经过 ResolveResolution 归一。
//
// 含视频输入时优先取 OutputWithVideoInput；该表整体缺省、或缺这一档分辨率，
// 都回落到 Output——缺项是「这个模型没有降档规则」，不是配置错误。
func (p VideoPricing) OutputRate(resolution string, hasVideoInput bool) float64 {
	if hasVideoInput {
		if rate, ok := p.OutputWithVideoInput[resolution]; ok {
			return rate
		}
	}
	return p.Output[resolution]
}

// ResolveResolution 把请求里的分辨率归一到价目表的档位键。
// 非空值只允许精确匹配或大小写不敏感匹配；只有调用方未传分辨率时，
// 才回退 DefaultResolution，避免把显式未知档位静默按默认档计费。
func (p VideoPricing) ResolveResolution(resolution string) (string, bool) {
	resolution = strings.TrimSpace(resolution)
	if resolution != "" {
		if _, ok := p.Output[resolution]; ok {
			return resolution, true
		}
		for key := range p.Output {
			if strings.EqualFold(key, resolution) {
				return key, true
			}
		}
		return "", false
	}
	if p.DefaultResolution != "" {
		if _, ok := p.Output[p.DefaultResolution]; ok {
			return p.DefaultResolution, true
		}
	}
	return "", false
}

// ChargesInputImage 价目表是否对输入图片实际计费。
// 未配置该项、或单价为 0，都视为免费。
func (p VideoPricing) ChargesInputImage() bool {
	img := p.Input.Image
	return img != nil && img.PerImage > 0
}

// ChargesInputAudio 价目表是否对输入音频实际计费。
func (p VideoPricing) ChargesInputAudio() bool {
	audio := p.Input.Audio
	return audio != nil && audio.PerSecond > 0
}

// ChargesInputVideo 价目表是否对该输出分辨率下的输入视频实际计费。
func (p VideoPricing) ChargesInputVideo(resolution string) bool {
	video := p.Input.Video
	if video == nil {
		return false
	}
	key, ok := p.ResolveResolution(resolution)
	if !ok {
		return false
	}
	return video.PerSecondByOutputResolution[key] > 0
}

// AttributeInputSeconds 把上游返回的「输入素材合并秒数」归属到具体维度。
//
// 上游只给一个合计值，不拆分视频/音频。归属规则由两件事共同决定：
//
//  1. 价目表：该维度是否真的计费（未配置或单价为 0 都算免费）
//  2. 原请求：该维度是否确实有素材（hasVideo / hasAudio）
//
// 只有同时满足两者的维度才是候选。恰好一个候选时，合计值全部归它；
// 零个或多个候选时无法从合计值里拆分，返回 false 让调用方保持提交时的
// 口径——宁可不动，也不要把免费维度算成收费维度、或把没传的素材算上。
//
// 返回 (视频秒数, 音频秒数, 是否可归属)。
func (p VideoPricing) AttributeInputSeconds(
	resolution string, inputSeconds float64, hasVideo, hasAudio bool,
) (float64, float64, bool) {
	videoCandidate := hasVideo && p.ChargesInputVideo(resolution)
	audioCandidate := hasAudio && p.ChargesInputAudio()

	switch {
	case videoCandidate && !audioCandidate:
		return inputSeconds, 0, true
	case audioCandidate && !videoCandidate:
		return 0, inputSeconds, true
	default:
		return 0, 0, false
	}
}

// ---------------------------------------------------------------------------
// Validation (called before save)
// ---------------------------------------------------------------------------

// CheckVideoPricingJSONString 校验管理员提交的整张价目表，供 option 保存前调用。
// 只做校验不落库——真正的写入仍由 config 模块的通用路径完成。
func CheckVideoPricingJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}
	var pricing map[string]VideoPricing
	if err := common.UnmarshalJsonStr(jsonStr, &pricing); err != nil {
		return fmt.Errorf("invalid video pricing JSON: %w", err)
	}
	return ValidateVideoPricingMap(pricing)
}

// ValidateVideoPricing 在管理员保存配置前做健全性检查。
func ValidateVideoPricing(model string, p VideoPricing) error {
	if len(p.Output) == 0 {
		return fmt.Errorf("model %s: output pricing is required", model)
	}
	for resolution, price := range p.Output {
		if strings.TrimSpace(resolution) == "" {
			return fmt.Errorf("model %s: output resolution key must not be empty", model)
		}
		if price < 0 {
			return fmt.Errorf("model %s: output price for %s must not be negative", model, resolution)
		}
	}
	// 降档表只允许覆盖已存在的档位：多出来的键说明管理员写错了分辨率名，
	// 而它会静默失效（OutputRate 回落到 Output），比报错更难查。
	for resolution, price := range p.OutputWithVideoInput {
		if price < 0 {
			return fmt.Errorf("model %s: output_with_video_input price for %s must not be negative",
				model, resolution)
		}
		if _, ok := p.Output[resolution]; !ok {
			return fmt.Errorf("model %s: output_with_video_input references unknown output resolution %q",
				model, resolution)
		}
	}
	if p.MaxOutputSeconds < 0 {
		return fmt.Errorf("model %s: max_output_seconds must not be negative", model)
	}
	if p.DefaultResolution != "" {
		if _, ok := p.Output[p.DefaultResolution]; !ok {
			return fmt.Errorf("model %s: default_resolution %q is not present in output pricing",
				model, p.DefaultResolution)
		}
	}
	if img := p.Input.Image; img != nil {
		if img.PerImage < 0 {
			return fmt.Errorf("model %s: input image price must not be negative", model)
		}
		if img.FreeCount < 0 {
			return fmt.Errorf("model %s: input image free_count must not be negative", model)
		}
	}
	if audio := p.Input.Audio; audio != nil && audio.PerSecond < 0 {
		return fmt.Errorf("model %s: input audio price must not be negative", model)
	}
	if video := p.Input.Video; video != nil {
		for resolution, price := range video.PerSecondByOutputResolution {
			if price < 0 {
				return fmt.Errorf("model %s: input video price for %s must not be negative", model, resolution)
			}
			if _, ok := p.Output[resolution]; !ok {
				return fmt.Errorf("model %s: input video pricing references unknown output resolution %q",
					model, resolution)
			}
		}
	}
	return nil
}

// ValidateVideoPricingMap 校验整张表，供保存前调用。
func ValidateVideoPricingMap(pricing map[string]VideoPricing) error {
	for model, p := range pricing {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("video pricing model name must not be empty")
		}
		if err := ValidateVideoPricing(model, p); err != nil {
			return err
		}
	}
	return nil
}
