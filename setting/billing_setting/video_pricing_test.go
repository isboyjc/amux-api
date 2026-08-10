package billing_setting

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

const floatTolerance = 1e-9

func assertMoney(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > floatTolerance {
		t.Errorf("%s = %.6f, want %.6f", label, got, want)
	}
}

// withOverride 临时设置管理员覆盖项，返回还原函数。
func withOverride(t *testing.T, pricing map[string]VideoPricing) {
	t.Helper()
	original := videoPricingSetting.Pricing
	videoPricingSetting.Pricing = pricing
	t.Cleanup(func() { videoPricingSetting.Pricing = original })
}

// TestComputeVideoCost_OfficialPriceTable 用 MiniMax 官方价目表手算的金额做断言。
// 这是整套视频计费的锚点：内置默认表一旦被改错，这里必须炸。
//
// 官方价（https://platform.minimax.io/docs/guides/pricing-paygo）：
//
//	输出 2K $0.13/s、768P $0.08/s
//	输入视频按输入时长计费，单价取输出分辨率档位
//	输入音频免费
//	输入图片 $0.04/张（本站不设免费额度）
func TestComputeVideoCost_OfficialPriceTable(t *testing.T) {
	cases := []struct {
		name  string
		usage VideoUsage
		want  VideoCostBreakdown
	}{
		{
			name:  "10秒2K纯文生",
			usage: VideoUsage{Resolution: "2K", OutputSeconds: 10},
			want:  VideoCostBreakdown{Resolution: "2K", Output: 1.30, Total: 1.30},
		},
		{
			name:  "10秒2K加6秒参考视频",
			usage: VideoUsage{Resolution: "2K", OutputSeconds: 10, VideoSeconds: 6},
			want:  VideoCostBreakdown{Resolution: "2K", Output: 1.30, Video: 0.78, Total: 2.08},
		},
		{
			name:  "5秒768P加3张图",
			usage: VideoUsage{Resolution: "768P", OutputSeconds: 5, ImageCount: 3},
			want:  VideoCostBreakdown{Resolution: "768P", Output: 0.40, Image: 0.12, Total: 0.52},
		},
		{
			name: "10秒2K加6秒参考视频加7张图",
			usage: VideoUsage{
				Resolution: "2K", OutputSeconds: 10, VideoSeconds: 6, ImageCount: 7,
			},
			want: VideoCostBreakdown{
				Resolution: "2K", Output: 1.30, Video: 0.78, Image: 0.28, Total: 2.36,
			},
		},
		{
			name:  "音频免费不改变总价",
			usage: VideoUsage{Resolution: "2K", OutputSeconds: 10, AudioSeconds: 15},
			want:  VideoCostBreakdown{Resolution: "2K", Output: 1.30, Audio: 0, Total: 1.30},
		},
		{
			// 参考视频单价取【输出】分辨率档位，不是输入自身的分辨率
			name:  "768P输出下参考视频按768P单价",
			usage: VideoUsage{Resolution: "768P", OutputSeconds: 5, VideoSeconds: 6},
			want:  VideoCostBreakdown{Resolution: "768P", Output: 0.40, Video: 0.48, Total: 0.88},
		},
		{
			name:  "最短4秒768P",
			usage: VideoUsage{Resolution: "768P", OutputSeconds: 4},
			want:  VideoCostBreakdown{Resolution: "768P", Output: 0.32, Total: 0.32},
		},
		{
			name:  "最长15秒2K",
			usage: VideoUsage{Resolution: "2K", OutputSeconds: 15},
			want:  VideoCostBreakdown{Resolution: "2K", Output: 1.95, Total: 1.95},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeVideoCost("MiniMax-H3", tc.usage)
			if err != nil {
				t.Fatalf("ComputeVideoCost failed: %v", err)
			}
			if got.Resolution != tc.want.Resolution {
				t.Errorf("Resolution = %q, want %q", got.Resolution, tc.want.Resolution)
			}
			assertMoney(t, "Output", got.Output, tc.want.Output)
			assertMoney(t, "Image", got.Image, tc.want.Image)
			assertMoney(t, "Audio", got.Audio, tc.want.Audio)
			assertMoney(t, "Video", got.Video, tc.want.Video)
			assertMoney(t, "Total", got.Total, tc.want.Total)
		})
	}
}

func TestComputeVideoCost_HappyHorseOfficialPriceTable(t *testing.T) {
	cases := []struct {
		model      string
		resolution string
		seconds    float64
		want       float64
	}{
		{model: "happyhorse-1.1-t2v", resolution: "480P", seconds: 5, want: 0.35},
		{model: "happyhorse-1.1-t2v", resolution: "720P", seconds: 5, want: 0.70},
		{model: "happyhorse-1.1-i2v", resolution: "1080P", seconds: 5, want: 0.90},
		{model: "happyhorse-1.1-r2v", resolution: "1080P", seconds: 10, want: 1.80},
		{model: "happyhorse-1.0-t2v", resolution: "480P", seconds: 5, want: 0.35},
		{model: "happyhorse-1.0-t2v", resolution: "720P", seconds: 5, want: 0.70},
		{model: "happyhorse-1.0-i2v", resolution: "1080P", seconds: 5, want: 1.20},
		{model: "happyhorse-1.0-r2v", resolution: "1080P", seconds: 10, want: 2.40},
		{model: "happyhorse-1.0-video-edit", resolution: "1080P", seconds: 13.24, want: 3.1776},
	}
	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.resolution, func(t *testing.T) {
			got, err := ComputeVideoCost(tc.model, VideoUsage{
				Resolution: tc.resolution, OutputSeconds: tc.seconds,
			})
			if err != nil {
				t.Fatalf("ComputeVideoCost failed: %v", err)
			}
			assertMoney(t, "Total", got.Total, tc.want)
		})
	}

	if _, err := ComputeVideoCost("happyhorse-1.1-t2v", VideoUsage{
		Resolution: "480P", OutputSeconds: 5,
	}); err != nil {
		t.Fatalf("HappyHorse generation 480P must be priced: %v", err)
	}
	if _, err := ComputeVideoCost("happyhorse-1.0-video-edit", VideoUsage{
		Resolution: "480P", OutputSeconds: 5,
	}); err == nil {
		t.Fatal("HappyHorse Video Edit 480P must be rejected")
	}
}

// TestComputeVideoCost_TotalIsSumOfParts 保证 Total 永远等于各项之和——
// 明细会写进日志用于对账，两者不一致比算错更难排查。
func TestComputeVideoCost_TotalIsSumOfParts(t *testing.T) {
	usage := VideoUsage{
		Resolution: "2K", OutputSeconds: 12, ImageCount: 9,
		AudioSeconds: 8, VideoSeconds: 5,
	}
	got, err := ComputeVideoCost("MiniMax-H3", usage)
	if err != nil {
		t.Fatalf("ComputeVideoCost failed: %v", err)
	}
	assertMoney(t, "Total", got.Total, got.Output+got.Image+got.Audio+got.Video)
}

func TestComputeVideoCost_ResolutionFallback(t *testing.T) {
	t.Run("大小写不敏感", func(t *testing.T) {
		lower, err := ComputeVideoCost("MiniMax-H3", VideoUsage{Resolution: "2k", OutputSeconds: 10})
		if err != nil {
			t.Fatalf("ComputeVideoCost failed: %v", err)
		}
		if lower.Resolution != "2K" {
			t.Errorf("Resolution = %q, want 2K", lower.Resolution)
		}
		assertMoney(t, "Total", lower.Total, 1.30)
	})

	t.Run("768p小写", func(t *testing.T) {
		got, err := ComputeVideoCost("MiniMax-H3", VideoUsage{Resolution: "768p", OutputSeconds: 5})
		if err != nil {
			t.Fatalf("ComputeVideoCost failed: %v", err)
		}
		assertMoney(t, "Total", got.Total, 0.40)
	})

	t.Run("空分辨率回落默认档", func(t *testing.T) {
		got, err := ComputeVideoCost("MiniMax-H3", VideoUsage{OutputSeconds: 10})
		if err != nil {
			t.Fatalf("ComputeVideoCost failed: %v", err)
		}
		if got.Resolution != "2K" {
			t.Errorf("Resolution = %q, want 2K (default)", got.Resolution)
		}
		assertMoney(t, "Total", got.Total, 1.30)
	})

	t.Run("显式未知分辨率不回落默认档", func(t *testing.T) {
		if _, err := ComputeVideoCost("MiniMax-H3", VideoUsage{
			Resolution: "4K", OutputSeconds: 10,
		}); err == nil {
			t.Fatal("expected error for explicit unknown resolution, got nil")
		}
	})
}

// TestComputeVideoCost_NeverSilentlyFree 保证算不出价时报错而不是按 $0 放行。
func TestComputeVideoCost_NeverSilentlyFree(t *testing.T) {
	t.Run("模型未配价", func(t *testing.T) {
		if _, err := ComputeVideoCost("no-such-video-model", VideoUsage{OutputSeconds: 5}); err == nil {
			t.Fatal("expected error for unconfigured model, got nil")
		}
	})

	t.Run("分辨率落不到档位且无默认", func(t *testing.T) {
		withOverride(t, map[string]VideoPricing{
			"gapless": {
				Output: map[string]float64{"720P": 0.05},
				// 故意不设 DefaultResolution
			},
		})
		if _, err := ComputeVideoCost("gapless", VideoUsage{Resolution: "4K", OutputSeconds: 5}); err == nil {
			t.Fatal("expected error for unknown resolution without default, got nil")
		}
	})

	t.Run("默认档位本身不在价目表里", func(t *testing.T) {
		withOverride(t, map[string]VideoPricing{
			"broken": {
				DefaultResolution: "1080P",
				Output:            map[string]float64{"720P": 0.05},
			},
		})
		if _, err := ComputeVideoCost("broken", VideoUsage{OutputSeconds: 5}); err == nil {
			t.Fatal("expected error when default_resolution is absent from output, got nil")
		}
	})
}

func TestComputeVideoCost_ImageFreeCount(t *testing.T) {
	withOverride(t, map[string]VideoPricing{
		"official-parity": {
			DefaultResolution: "2K",
			Output:            map[string]float64{"2K": 0.13},
			Input: VideoInputPricing{
				Image: &VideoImagePricing{PerImage: 0.04, FreeCount: 5},
			},
		},
	})

	cases := []struct {
		images int
		want   float64
	}{
		{images: 0, want: 0},
		{images: 5, want: 0},
		{images: 6, want: 0.04},
		{images: 9, want: 0.16},
	}
	for _, tc := range cases {
		got, err := ComputeVideoCost("official-parity", VideoUsage{OutputSeconds: 0, ImageCount: tc.images})
		if err != nil {
			t.Fatalf("ComputeVideoCost failed: %v", err)
		}
		assertMoney(t, "Image", got.Image, tc.want)
	}
}

// TestGetVideoPricing_OverrideWins 覆盖项优先，未覆盖的模型仍回落内置默认——
// 管理员保存一份只含自家模型的配置，不应让 MiniMax-H3 变成无价可查。
func TestGetVideoPricing_OverrideWins(t *testing.T) {
	withOverride(t, map[string]VideoPricing{
		"MiniMax-H3": {
			DefaultResolution: "2K",
			Output:            map[string]float64{"2K": 0.20},
		},
		"custom-model": {
			DefaultResolution: "720P",
			Output:            map[string]float64{"720P": 0.01},
		},
	})

	overridden, err := ComputeVideoCost("MiniMax-H3", VideoUsage{Resolution: "2K", OutputSeconds: 10})
	if err != nil {
		t.Fatalf("ComputeVideoCost failed: %v", err)
	}
	assertMoney(t, "overridden total", overridden.Total, 2.00)

	custom, err := ComputeVideoCost("custom-model", VideoUsage{OutputSeconds: 10})
	if err != nil {
		t.Fatalf("ComputeVideoCost failed: %v", err)
	}
	assertMoney(t, "custom total", custom.Total, 0.10)
}

func TestGetVideoPricing_FallsBackToDefaultWhenOverrideMissing(t *testing.T) {
	withOverride(t, map[string]VideoPricing{
		"unrelated": {Output: map[string]float64{"720P": 0.01}, DefaultResolution: "720P"},
	})

	if _, ok := GetVideoPricing("MiniMax-H3"); !ok {
		t.Fatal("MiniMax-H3 should still resolve to the built-in default pricing")
	}
	got, err := ComputeVideoCost("MiniMax-H3", VideoUsage{Resolution: "2K", OutputSeconds: 10})
	if err != nil {
		t.Fatalf("ComputeVideoCost failed: %v", err)
	}
	assertMoney(t, "Total", got.Total, 1.30)
}

func TestResolveVideoPricingPrefersAliasOverrideThenUpstream(t *testing.T) {
	withOverride(t, map[string]VideoPricing{
		"happyhorse-alias": {
			DefaultResolution: "1080P",
			Output:            map[string]float64{"1080P": 0.33},
		},
	})

	modelName, pricing, ok := ResolveVideoPricing("happyhorse-alias", "happyhorse-1.1-t2v")
	if !ok || modelName != "happyhorse-alias" {
		t.Fatalf("resolved model=%q, ok=%v, want alias override", modelName, ok)
	}
	assertMoney(t, "alias rate", pricing.Output["1080P"], 0.33)

	modelName, pricing, ok = ResolveVideoPricing("unpriced-alias", "happyhorse-1.1-t2v")
	if !ok || modelName != "happyhorse-1.1-t2v" {
		t.Fatalf("resolved model=%q, ok=%v, want upstream model", modelName, ok)
	}
	assertMoney(t, "upstream rate", pricing.Output["1080P"], 0.18)
}

func TestValidateVideoPricing(t *testing.T) {
	valid := VideoPricing{
		Unit:              VideoPricingUnitSecond,
		DefaultResolution: "2K",
		Output:            map[string]float64{"768P": 0.08, "2K": 0.13},
		Input: VideoInputPricing{
			Image: &VideoImagePricing{PerImage: 0.04},
			Audio: &VideoSecondsPricing{PerSecond: 0},
			Video: &VideoInputVideoPricing{
				PerSecondByOutputResolution: map[string]float64{"768P": 0.08, "2K": 0.13},
			},
		},
	}
	if err := ValidateVideoPricing("MiniMax-H3", valid); err != nil {
		t.Fatalf("valid pricing rejected: %v", err)
	}

	invalid := map[string]VideoPricing{
		"输出档位为空": {
			Output: map[string]float64{},
		},
		"输出单价为负": {
			Output: map[string]float64{"2K": -0.1},
		},
		"默认档位不在价目表": {
			DefaultResolution: "1080P",
			Output:            map[string]float64{"2K": 0.13},
		},
		"图片单价为负": {
			Output: map[string]float64{"2K": 0.13},
			Input:  VideoInputPricing{Image: &VideoImagePricing{PerImage: -0.01}},
		},
		"免费张数为负": {
			Output: map[string]float64{"2K": 0.13},
			Input:  VideoInputPricing{Image: &VideoImagePricing{PerImage: 0.04, FreeCount: -1}},
		},
		"音频单价为负": {
			Output: map[string]float64{"2K": 0.13},
			Input:  VideoInputPricing{Audio: &VideoSecondsPricing{PerSecond: -1}},
		},
		"输入视频引用了未知输出档位": {
			Output: map[string]float64{"2K": 0.13},
			Input: VideoInputPricing{
				Video: &VideoInputVideoPricing{
					PerSecondByOutputResolution: map[string]float64{"768P": 0.08},
				},
			},
		},
	}
	for name, pricing := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateVideoPricing("m", pricing); err == nil {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}

// TestDefaultVideoPricingIsValid 保证随代码发布的内置表本身能通过校验。
func TestDefaultVideoPricingIsValid(t *testing.T) {
	if err := ValidateVideoPricingMap(defaultVideoPricing); err != nil {
		t.Fatalf("built-in default video pricing is invalid: %v", err)
	}
	if err := ValidateVideoPricingMap(defaultVideoPricingAliases); err != nil {
		t.Fatalf("built-in video pricing aliases are invalid: %v", err)
	}
}

// TestDefaultVideoPricingJSONExcludesAliases 管理端展示的是规范名。别名混进去
// 会让定价面板上冒出好几行一模一样的记录，但它们必须仍然能查到价。
func TestDefaultVideoPricingJSONExcludesAliases(t *testing.T) {
	var shown map[string]VideoPricing
	if err := common.UnmarshalJsonStr(DefaultVideoPricingJSON(), &shown); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	for alias := range defaultVideoPricingAliases {
		if _, ok := shown[alias]; ok {
			t.Errorf("别名 %q 不该出现在管理端展示的内置价目表里", alias)
		}
		if _, ok := GetVideoPricing(alias); !ok {
			t.Errorf("别名 %q 必须仍然查得到价", alias)
		}
	}
	if _, ok := shown["doubao-seedance-2-5"]; !ok {
		t.Error("规范名 doubao-seedance-2-5 必须展示给管理端")
	}
}

// TestCheckVideoPricingJSONString 保存前的守门：配错的价目表不能落库，
// 否则该模型的每个请求都会在预扣费前被拒。
func TestCheckVideoPricingJSONString(t *testing.T) {
	valid := `{"m":{"default_resolution":"2K","output":{"2K":0.13},
		"input":{"image":{"per_image":0.04},"video":{"per_second_by_output_resolution":{"2K":0.13}}}}}`
	if err := CheckVideoPricingJSONString(valid); err != nil {
		t.Fatalf("valid pricing rejected: %v", err)
	}

	// 空值放行：管理员清空配置等于回落到内置默认表
	if err := CheckVideoPricingJSONString("   "); err != nil {
		t.Errorf("empty config should be accepted: %v", err)
	}

	invalid := map[string]string{
		"非法 JSON":    `{"m":`,
		"输出档位为空":     `{"m":{"output":{}}}`,
		"默认档位不在价目表":  `{"m":{"default_resolution":"1080P","output":{"2K":0.13}}}`,
		"单价为负":       `{"m":{"output":{"2K":-1}}}`,
		"输入视频引用未知档位": `{"m":{"output":{"2K":0.13},"input":{"video":{"per_second_by_output_resolution":{"768P":0.08}}}}}`,
	}
	for name, jsonStr := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := CheckVideoPricingJSONString(jsonStr); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

// TestGetBillingMode_VideoPricingImpliesVideoMode 有价目表就等于视频计费。
// 不成立的话，内置视频模型会在管理端显示成"未设置价格"，并被当成倍率为 0
// 的按量计费模型。
func TestGetBillingMode_VideoPricingImpliesVideoMode(t *testing.T) {
	if got := GetBillingMode("MiniMax-H3"); got != BillingModeVideo {
		t.Errorf("GetBillingMode(MiniMax-H3) = %q, want %q", got, BillingModeVideo)
	}
	if got := GetBillingMode("gpt-4o"); got != BillingModeRatio {
		t.Errorf("GetBillingMode(gpt-4o) = %q, want %q", got, BillingModeRatio)
	}

	// 管理员显式指定的模式优先于价目表推断
	original := billingSetting.BillingMode
	billingSetting.BillingMode = map[string]string{"MiniMax-H3": BillingModeRatio}
	t.Cleanup(func() { billingSetting.BillingMode = original })
	if got := GetBillingMode("MiniMax-H3"); got != BillingModeRatio {
		t.Errorf("explicit billing mode should win, got %q", got)
	}
}

// TestVideoCostRatioKey_MatchesGuard 预扣链路（relay_task.go）用这个键名校验
// 「配了视频价目表的模型，适配器确实产出了金额」。适配器返回的键名与它不一致
// 时，请求会被当成不支持视频计费而拒绝——两边必须引用同一个常量。
func TestVideoCostRatioKey_MatchesGuard(t *testing.T) {
	if VideoCostRatioKey != "video_cost" {
		t.Errorf("VideoCostRatioKey = %q；改名会同时影响日志展示与预扣校验，"+
			"确认前端 other.video_billing 与 relay_task 的守卫都已跟随", VideoCostRatioKey)
	}
}

// TestAttributeInputSeconds 上游只给一个合并的输入素材秒数，归属到哪个维度
// 必须由价目表 + 原请求共同决定，不能在代码里写死「算作视频」。
func TestAttributeInputSeconds(t *testing.T) {
	// 只对视频计费（H3 的默认配置：音频免费）
	videoOnly := VideoPricing{
		DefaultResolution: "2K",
		Output:            map[string]float64{"2K": 0.13},
		Input: VideoInputPricing{
			Audio: &VideoSecondsPricing{PerSecond: 0},
			Video: &VideoInputVideoPricing{
				PerSecondByOutputResolution: map[string]float64{"2K": 0.13},
			},
		},
	}
	// 只对音频计费
	audioOnly := VideoPricing{
		DefaultResolution: "2K",
		Output:            map[string]float64{"2K": 0.13},
		Input:             VideoInputPricing{Audio: &VideoSecondsPricing{PerSecond: 0.02}},
	}
	// 两者都计费
	both := VideoPricing{
		DefaultResolution: "2K",
		Output:            map[string]float64{"2K": 0.13},
		Input: VideoInputPricing{
			Audio: &VideoSecondsPricing{PerSecond: 0.02},
			Video: &VideoInputVideoPricing{
				PerSecondByOutputResolution: map[string]float64{"2K": 0.13},
			},
		},
	}

	cases := []struct {
		name                 string
		pricing              VideoPricing
		hasVideo, hasAudio   bool
		wantVideo, wantAudio float64
		wantOK               bool
	}{
		{"只有视频计费且请求带了视频", videoOnly, true, false, 9, 0, true},
		{"视频计费 + 请求同时带音频，音频免费不参与归属", videoOnly, true, true, 9, 0, true},
		{"请求只带音频而音频免费，不能算成视频", videoOnly, false, true, 0, 0, false},
		{"只有音频计费且请求带了音频", audioOnly, false, true, 0, 9, true},
		{"音频计费但请求带的是视频，视频未配价", audioOnly, true, false, 0, 0, false},
		{"两者都计费且都带了，拆不出来", both, true, true, 0, 0, false},
		{"两者都计费但只带了视频", both, true, false, 9, 0, true},
		{"两者都计费但只带了音频", both, false, true, 0, 9, true},
		{"什么都没带", videoOnly, false, false, 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotVideo, gotAudio, gotOK := tc.pricing.AttributeInputSeconds("2K", 9, tc.hasVideo, tc.hasAudio)
			if gotOK != tc.wantOK {
				t.Fatalf("attributed = %v, want %v", gotOK, tc.wantOK)
			}
			assertMoney(t, "video seconds", gotVideo, tc.wantVideo)
			assertMoney(t, "audio seconds", gotAudio, tc.wantAudio)
		})
	}
}

// TestChargesInput_ZeroPriceIsFree 单价为 0 或未配置都必须视为免费，
// 否则免费维度会被当成收费维度参与归属。
func TestChargesInput_ZeroPriceIsFree(t *testing.T) {
	zero := VideoPricing{
		DefaultResolution: "2K",
		Output:            map[string]float64{"2K": 0.13},
		Input: VideoInputPricing{
			Image: &VideoImagePricing{PerImage: 0},
			Audio: &VideoSecondsPricing{PerSecond: 0},
			Video: &VideoInputVideoPricing{
				PerSecondByOutputResolution: map[string]float64{"2K": 0},
			},
		},
	}
	if zero.ChargesInputImage() || zero.ChargesInputAudio() || zero.ChargesInputVideo("2K") {
		t.Error("单价为 0 的维度应视为免费")
	}

	unset := VideoPricing{DefaultResolution: "2K", Output: map[string]float64{"2K": 0.13}}
	if unset.ChargesInputImage() || unset.ChargesInputAudio() || unset.ChargesInputVideo("2K") {
		t.Error("未配置的维度应视为免费")
	}

	// 分辨率落不到档位时不能误判为计费
	priced := VideoPricing{
		DefaultResolution: "2K",
		Output:            map[string]float64{"2K": 0.13},
		Input: VideoInputPricing{Video: &VideoInputVideoPricing{
			PerSecondByOutputResolution: map[string]float64{"768P": 0.08},
		}},
	}
	if priced.ChargesInputVideo("2K") {
		t.Error("2K 档未配置输入视频价，不应算作计费")
	}
}

// ---------------------------------------------------------------------------
// Seedance 2.5：含视频输入的整单降档
// ---------------------------------------------------------------------------

// TestComputeVideoCost_Seedance25 用火山方舟官方价目表的锚点做断言。
//
// 上游按 token 计费：tokens = 宽 × 高 × 帧数 / 1024，参考视频按 24fps 全采样
// 并折算到输出分辨率；单价不含视频 $10.37/M、含视频 $6.23/M（降档作用于整单）。
// 下面的期望值是「计费秒数 × 该档每秒价」，与内置价目表必须逐分对上。
func TestComputeVideoCost_Seedance25(t *testing.T) {
	const model = "doubao-seedance-2-5"

	cases := []struct {
		name  string
		usage VideoUsage
		want  VideoCostBreakdown
	}{
		{
			name:  "720p5秒纯文生",
			usage: VideoUsage{Resolution: "720p", OutputSeconds: 5},
			want:  VideoCostBreakdown{Resolution: "720p", Output: 1.12705, Total: 1.12705},
		},
		{
			name:  "480p5秒纯文生",
			usage: VideoUsage{Resolution: "480p", OutputSeconds: 5},
			want:  VideoCostBreakdown{Resolution: "480p", Output: 0.52080, Total: 0.52080},
		},
		{
			// 输出秒也要降档：整单换单价，不是只对输入部分打折。参考视频那段
			// 走的也是这个降档单价（VideoInputBilledAsOutput），单独成项只是
			// 为了让计费明细看得出各占多少。
			name:  "720p5秒加4秒参考视频",
			usage: VideoUsage{Resolution: "720p", OutputSeconds: 5, VideoSeconds: 4},
			want: VideoCostBreakdown{
				Resolution: "720p", Output: 0.67710, Video: 0.54168, Total: 1.21878,
			},
		},
		{
			name:  "480p5秒加30秒参考视频",
			usage: VideoUsage{Resolution: "480p", OutputSeconds: 5, VideoSeconds: 30},
			want: VideoCostBreakdown{
				Resolution: "480p", Output: 0.31290, Video: 1.87740, Total: 2.19030,
			},
		},
		{
			// 结算口径：上游只给合并的 token 用量，拆不出输出/输入，全部记在
			// OutputSeconds 上，靠 HasVideoInput 保住降档
			name: "结算合并秒数走降档",
			usage: VideoUsage{
				Resolution: "720p", OutputSeconds: 21, HasVideoInput: true,
			},
			want: VideoCostBreakdown{Resolution: "720p", Output: 2.84382, Total: 2.84382},
		},
		{
			// 预扣口径：duration=-1 按 15 秒，参考视频时长未知按 30 秒上界
			name:  "预扣720p未知时长加参考视频",
			usage: VideoUsage{Resolution: "720p", OutputSeconds: 15, VideoSeconds: 30},
			want: VideoCostBreakdown{
				Resolution: "720p", Output: 2.03130, Video: 4.06260, Total: 6.09390,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeVideoCost(model, tc.usage)
			if err != nil {
				t.Fatalf("ComputeVideoCost 出错: %v", err)
			}
			if got.Resolution != tc.want.Resolution {
				t.Errorf("resolution = %q, want %q", got.Resolution, tc.want.Resolution)
			}
			assertMoney(t, "output", got.Output, tc.want.Output)
			assertMoney(t, "video", got.Video, tc.want.Video)
			assertMoney(t, "total", got.Total, tc.want.Total)
		})
	}
}

// TestSeedance25_NeverUndercutsOfficialPrice 交叉验证：把美元报价按两张官方
// 价目表的隐含汇率（6.75）折回人民币，与火山公开的六个锚点对照。
//
// 秒价按【每档分辨率里像素最多的宽高比】定，官方那张表报的是 16:9，所以我们
// 一定只高不低——低了就是成本倒挂。上界 5% 是 480p 21:9 与 16:9 的像素差
// （4.54%）加一点舍入余量：超出说明价目表被改错了，而不是宽高比造成的。
//
// 这是整套推导的锚：token 单价、token 公式、24fps 全采样、降档规则、最贵比例
// 定价，任何一处被改错这里都会炸。
func TestSeedance25_NeverUndercutsOfficialPrice(t *testing.T) {
	const impliedRate = 6.75

	cases := []struct {
		name        string
		usage       VideoUsage
		officialCNY float64
	}{
		{"480p输出5秒", VideoUsage{Resolution: "480p", OutputSeconds: 5}, 3.36},
		{"720p输出5秒", VideoUsage{Resolution: "720p", OutputSeconds: 5}, 7.56},
		{"480p输出5秒输入4秒", VideoUsage{Resolution: "480p", OutputSeconds: 5, VideoSeconds: 4}, 3.63},
		{"480p输出5秒输入30秒", VideoUsage{Resolution: "480p", OutputSeconds: 5, VideoSeconds: 30}, 14.12},
		{"720p输出5秒输入4秒", VideoUsage{Resolution: "720p", OutputSeconds: 5, VideoSeconds: 4}, 8.16},
		{"720p输出5秒输入30秒", VideoUsage{Resolution: "720p", OutputSeconds: 5, VideoSeconds: 30}, 31.75},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeVideoCost("doubao-seedance-2-5", tc.usage)
			if err != nil {
				t.Fatalf("ComputeVideoCost 出错: %v", err)
			}
			cny := got.Total * impliedRate
			delta := (cny - tc.officialCNY) / tc.officialCNY
			if delta < 0 {
				t.Errorf("折算 %.4f 元 < 官方 16:9 报价 %.2f 元，成本倒挂 %.2f%%",
					cny, tc.officialCNY, -delta*100)
			}
			if delta > 0.05 {
				t.Errorf("折算 %.4f 元，比官方 %.2f 元高 %.2f%%，超出宽高比差异能解释的范围",
					cny, tc.officialCNY, delta*100)
			}
		})
	}
}

// TestSeedance25_InputPricing 2.5 的参考素材是「部分收费」：图片和音频官方不
// 单独计价（不配 = 免费），参考视频则按其自身时长实打实加钱。
func TestSeedance25_InputPricing(t *testing.T) {
	p, ok := GetVideoPricing("doubao-seedance-2-5")
	if !ok {
		t.Fatal("doubao-seedance-2-5 应有内置价目表")
	}
	if p.Input.Image != nil || p.Input.Audio != nil {
		t.Error("火山对参考图片与音频不单独计价，不该配这两项")
	}
	if p.Input.Video == nil {
		t.Fatal("参考视频是收费的，必须配单价——官方 720p 加 30 秒参考视频要多收 24.19 元")
	}

	// 参考视频单价与「含视频输入」档的输出单价相同：上游是同一个 token 单价、
	// 同一个输出分辨率，两者本就该相等。只改一张表会让它们悄悄错位。
	for resolution, outputRate := range p.OutputWithVideoInput {
		assertMoney(t, resolution+" 参考视频单价",
			p.Input.Video.PerSecondByOutputResolution[resolution], outputRate)
	}

	// 图片再多也不加钱
	withImages, err := ComputeVideoCost("doubao-seedance-2-5",
		VideoUsage{Resolution: "720p", OutputSeconds: 5, ImageCount: 30})
	if err != nil {
		t.Fatalf("ComputeVideoCost 出错: %v", err)
	}
	assertMoney(t, "30 张参考图不加钱", withImages.Image, 0)
	assertMoney(t, "音频不加钱", withImages.Audio, 0)
}

// TestComputeVideoCost_InputMaterialsFreeWhenUnset 参考素材完全免费的模型只要
// 不配 Input 就行，不需要额外的开关。
func TestComputeVideoCost_InputMaterialsFreeWhenUnset(t *testing.T) {
	withOverride(t, map[string]VideoPricing{
		"free-inputs": {
			DefaultResolution: "720p",
			Output:            map[string]float64{"720p": 0.2},
		},
	})

	got, err := ComputeVideoCost("free-inputs", VideoUsage{
		Resolution: "720p", OutputSeconds: 10,
		ImageCount: 20, AudioSeconds: 30, VideoSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ComputeVideoCost 出错: %v", err)
	}
	assertMoney(t, "图片", got.Image, 0)
	assertMoney(t, "音频", got.Audio, 0)
	assertMoney(t, "视频", got.Video, 0)
	assertMoney(t, "总额只有输出", got.Total, 2.0)
}

// TestComputeVideoCost_H3StillUsesSeparateInputPricing MiniMax H3 是真的对输入
// 素材另收钱（图片按张、输入视频按其自身时长），加了新开关不能把它弄坏。
func TestComputeVideoCost_H3StillUsesSeparateInputPricing(t *testing.T) {
	p, ok := GetVideoPricing("MiniMax-H3")
	if !ok {
		t.Fatal("MiniMax-H3 应有内置价目表")
	}
	if p.Input.Video == nil || p.Input.Image == nil {
		t.Error("H3 的输入素材定价被删掉了")
	}

	got, err := ComputeVideoCost("MiniMax-H3",
		VideoUsage{Resolution: "2K", OutputSeconds: 10, VideoSeconds: 6, ImageCount: 3})
	if err != nil {
		t.Fatalf("ComputeVideoCost 出错: %v", err)
	}
	assertMoney(t, "输出", got.Output, 1.30)
	assertMoney(t, "输入视频", got.Video, 0.78)
	assertMoney(t, "输入图片", got.Image, 0.12)
}

// TestSeedance25_BothModelNamesPriceIdentically 官方 endpoint 名与对外别名必须
// 同价：少配一个会让那条路径掉回 ModelPrice 查表，两个名字算出不同的钱。
func TestSeedance25_BothModelNamesPriceIdentically(t *testing.T) {
	usage := VideoUsage{Resolution: "720p", OutputSeconds: 8, VideoSeconds: 5}

	alias, err := ComputeVideoCost("doubao-seedance-2-5", usage)
	if err != nil {
		t.Fatalf("别名计价出错: %v", err)
	}
	official, err := ComputeVideoCost("doubao-seedance-2-5-260628", usage)
	if err != nil {
		t.Fatalf("官方名计价出错: %v", err)
	}
	assertMoney(t, "两个模型名的报价", official.Total, alias.Total)
}

// TestOutputRate_FallsBackWhenNoVideoTier 没有降档表的模型（如 MiniMax-H3）
// 不受这套规则影响：含视频输入时仍走 Output。
func TestOutputRate_FallsBackWhenNoVideoTier(t *testing.T) {
	p, ok := GetVideoPricing("MiniMax-H3")
	if !ok {
		t.Fatal("MiniMax-H3 应有内置价目表")
	}
	assertMoney(t, "含视频输入时的 2K 单价", p.OutputRate("2K", true), p.Output["2K"])

	// 降档表缺这一档分辨率时也回落，而不是按 0 计费
	partial := VideoPricing{
		Output:               map[string]float64{"480p": 0.1, "720p": 0.2},
		OutputWithVideoInput: map[string]float64{"720p": 0.12},
	}
	assertMoney(t, "缺档回落", partial.OutputRate("480p", true), 0.1)
	assertMoney(t, "命中降档", partial.OutputRate("720p", true), 0.12)
	assertMoney(t, "不含视频走原档", partial.OutputRate("720p", false), 0.2)
}

// TestValidateVideoPricing_OutputWithVideoInput 降档表引用不存在的分辨率必须
// 报错：它会静默失效（回落到 Output），比报错难查得多。
func TestValidateVideoPricing_OutputWithVideoInput(t *testing.T) {
	base := func() VideoPricing {
		return VideoPricing{
			DefaultResolution: "720p",
			Output:            map[string]float64{"720p": 0.2},
		}
	}

	valid := base()
	valid.OutputWithVideoInput = map[string]float64{"720p": 0.12}
	if err := ValidateVideoPricing("m", valid); err != nil {
		t.Errorf("合法配置不应报错: %v", err)
	}

	unknown := base()
	unknown.OutputWithVideoInput = map[string]float64{"1080p": 0.12}
	if err := ValidateVideoPricing("m", unknown); err == nil {
		t.Error("降档表引用未知分辨率应报错")
	}

	negative := base()
	negative.OutputWithVideoInput = map[string]float64{"720p": -1}
	if err := ValidateVideoPricing("m", negative); err == nil {
		t.Error("负单价应报错")
	}

	negSeconds := base()
	negSeconds.MaxOutputSeconds = -1
	if err := ValidateVideoPricing("m", negSeconds); err == nil {
		t.Error("负的 max_output_seconds 应报错")
	}
}

// TestDefaultVideoPricingJSON 管理端靠这份数据把内置定价的视频模型显示成
// 「视频计费」。序列化坏掉的话面板会把它们显示成「按量计费 + 空倍率」，
// 管理员就会去填一个根本不生效的 ModelRatio。
func TestDefaultVideoPricingJSON(t *testing.T) {
	var parsed map[string]VideoPricing
	if err := common.UnmarshalJsonStr(DefaultVideoPricingJSON(), &parsed); err != nil {
		t.Fatalf("内置价目表序列化后解析不回来: %v", err)
	}

	for _, name := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.1-i2v",
		"happyhorse-1.1-r2v",
		"happyhorse-1.0-t2v",
		"happyhorse-1.0-i2v",
		"happyhorse-1.0-r2v",
		"happyhorse-1.0-video-edit",
		"MiniMax-H3",
		"doubao-seedance-2-5",
	} {
		spec, ok := parsed[name]
		if !ok {
			t.Fatalf("内置价目表里缺 %s", name)
		}
		if len(spec.Output) == 0 {
			t.Errorf("%s 的输出定价丢了", name)
		}
	}

	// 降档表必须一起序列化出去，否则管理端存回来时会把它清掉，
	// 含参考视频的任务就会按不含视频的高单价收钱
	if len(parsed["doubao-seedance-2-5"].OutputWithVideoInput) == 0 {
		t.Error("doubao-seedance-2-5 的降档表没有序列化出去")
	}
	if parsed["doubao-seedance-2-5"].MaxOutputSeconds != 15 {
		t.Errorf("预扣锚点 = %v, want 15", parsed["doubao-seedance-2-5"].MaxOutputSeconds)
	}
}
