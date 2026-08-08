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
	}
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

// GetVideoPricing 返回模型的有效价目表：管理员覆盖优先，其次内置默认。
func GetVideoPricing(model string) (VideoPricing, bool) {
	if p, ok := videoPricingSetting.Pricing[model]; ok {
		return p, true
	}
	p, ok := defaultVideoPricing[model]
	return p, ok
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
		b.Output = p.Output[resolution] * u.OutputSeconds
	}

	if img := p.Input.Image; img != nil {
		if billable := u.ImageCount - img.FreeCount; billable > 0 {
			b.Image = float64(billable) * img.PerImage
		}
	}

	if audio := p.Input.Audio; audio != nil && u.AudioSeconds > 0 {
		b.Audio = audio.PerSecond * u.AudioSeconds
	}

	// 输入视频按【输出】分辨率取单价。该分辨率未配置输入视频价时视为不计费，
	// 而不是报错——输出档位已经在上面校验过，这里缺项是价目表的有意留白。
	if video := p.Input.Video; video != nil && u.VideoSeconds > 0 {
		if rate, ok := video.PerSecondByOutputResolution[resolution]; ok {
			b.Video = rate * u.VideoSeconds
		}
	}

	b.Total = b.Output + b.Image + b.Audio + b.Video
	return b, nil
}

// ResolveResolution 把请求里的分辨率归一到价目表的档位键。
// 精确匹配 → 大小写不敏感匹配 → DefaultResolution。
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
