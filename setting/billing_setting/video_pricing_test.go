package billing_setting

import (
	"math"
	"testing"
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
