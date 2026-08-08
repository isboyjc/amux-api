package hailuo

import (
	"reflect"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func mustNormalize(t *testing.T, build func() (*H3Request, error)) *H3Request {
	t.Helper()
	req, err := build()
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	req.ApplyDefaults()
	if err := req.Validate(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	return req
}

// TestH3Normalization_BothEntriesAgree 是这一层最重要的保证：同一个语义请求，
// 无论从站内统一协议还是 MiniMax v2 原生协议进来，归一化结果、上游请求体、
// 计费口径都必须完全一致。任何一条路径被单独改动，这里就会炸。
func TestH3Normalization_BothEntriesAgree(t *testing.T) {
	const (
		prompt     = "一个男孩在海边打篮球"
		firstFrame = "https://example.com/first.jpg"
		lastFrame  = "https://example.com/last.jpg"
		refImage   = "https://example.com/ref.jpg"
		refVideo   = "https://example.com/motion.mp4"
		refAudio   = "https://example.com/mood.mp3"
	)

	fromUnified := mustNormalize(t, func() (*H3Request, error) {
		return H3FromTaskSubmitReq(relaycommon.TaskSubmitReq{
			Model:    ModelMiniMaxH3,
			Prompt:   prompt,
			Duration: intPtr(10),
			Size:     "2K",
			Metadata: map[string]interface{}{
				"aspect_ratio":         "16:9",
				"first_frame_image":    firstFrame,
				"last_frame_image":     lastFrame,
				"reference_image_urls": []string{refImage},
				"reference_video_urls": []string{refVideo},
				"reference_audio_urls": []string{refAudio},
				"aigc_watermark":       true,
			},
		})
	})

	fromNative := mustNormalize(t, func() (*H3Request, error) {
		return H3FromV2Request(&V2VideoRequest{
			Model:      ModelMiniMaxH3,
			Resolution: "2K",
			Duration:   10,
			Ratio:      "16:9",
			Content: []V2Content{
				{Type: "text", Text: prompt},
				{Type: "image_url", ImageURL: &V2URLRef{URL: firstFrame}, Role: "first_frame"},
				{Type: "image_url", ImageURL: &V2URLRef{URL: lastFrame}, Role: "last_frame"},
				{Type: "image_url", ImageURL: &V2URLRef{URL: refImage}, Role: "reference_image"},
				{Type: "video_url", VideoURL: &V2URLRef{URL: refVideo}, Role: "reference_video"},
				{Type: "audio_url", AudioURL: &V2URLRef{URL: refAudio}, Role: "reference_audio"},
			},
			AigcWatermark: boolPtr(true),
		})
	})

	if !reflect.DeepEqual(fromUnified, fromNative) {
		t.Fatalf("normalized requests differ:\n unified = %+v\n native  = %+v", fromUnified, fromNative)
	}

	if !reflect.DeepEqual(fromUnified.ToVideoUsage(), fromNative.ToVideoUsage()) {
		t.Errorf("video usage differs:\n unified = %+v\n native = %+v",
			fromUnified.ToVideoUsage(), fromNative.ToVideoUsage())
	}

	if !reflect.DeepEqual(fromUnified.ToV2Request(ModelMiniMaxH3), fromNative.ToV2Request(ModelMiniMaxH3)) {
		t.Error("upstream request bodies differ between the two entry points")
	}
}

// TestH3FromTaskSubmitReq_ImagesAsFrames 站内协议用 images[0]/[1] 表达首尾帧。
func TestH3FromTaskSubmitReq_ImagesAsFrames(t *testing.T) {
	req := mustNormalize(t, func() (*H3Request, error) {
		return H3FromTaskSubmitReq(relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3,
			Prompt: "walk forward",
			Images: []string{"https://example.com/a.jpg", "https://example.com/b.jpg"},
		})
	})

	if req.FirstFrame != "https://example.com/a.jpg" {
		t.Errorf("FirstFrame = %q", req.FirstFrame)
	}
	if req.LastFrame != "https://example.com/b.jpg" {
		t.Errorf("LastFrame = %q", req.LastFrame)
	}
	if req.ImageCount() != 2 {
		t.Errorf("ImageCount = %d, want 2", req.ImageCount())
	}
}

// TestH3FromTaskSubmitReq_MetadataFramesWinOverImages metadata 里的显式声明
// 优先级高于 images 数组的位置约定。
func TestH3FromTaskSubmitReq_MetadataFramesWinOverImages(t *testing.T) {
	req := mustNormalize(t, func() (*H3Request, error) {
		return H3FromTaskSubmitReq(relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3,
			Prompt: "p",
			Images: []string{"https://example.com/positional.jpg"},
			Metadata: map[string]interface{}{
				"first_frame_image": "https://example.com/explicit.jpg",
			},
		})
	})

	if req.FirstFrame != "https://example.com/explicit.jpg" {
		t.Errorf("FirstFrame = %q, want the explicit metadata value", req.FirstFrame)
	}
}

func TestH3ApplyDefaults(t *testing.T) {
	req := &H3Request{Prompt: "p"}
	req.ApplyDefaults()

	if req.Duration != H3DefaultDuration {
		t.Errorf("Duration = %d, want %d", req.Duration, H3DefaultDuration)
	}
	if req.Resolution != H3Resolution2K {
		t.Errorf("Resolution = %q, want %q", req.Resolution, H3Resolution2K)
	}
}

func TestNormalizeH3Resolution(t *testing.T) {
	valid := map[string]string{
		"":          "",
		"2K":        H3Resolution2K,
		"2k":        H3Resolution2K,
		"2560x1440": H3Resolution2K,
		"1440p":     H3Resolution2K,
		"768P":      H3Resolution768P,
		"768p":      H3Resolution768P,
		"1366x768":  H3Resolution768P,
	}
	for input, want := range valid {
		got, err := normalizeH3Resolution(input)
		if err != nil {
			t.Errorf("normalizeH3Resolution(%q) errored: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeH3Resolution(%q) = %q, want %q", input, got, want)
		}
	}

	// 不认识的分辨率必须报错，不能静默降级到默认档位后照默认档计费
	for _, input := range []string{"1080P", "480p", "4K", "garbage"} {
		if _, err := normalizeH3Resolution(input); err == nil {
			t.Errorf("normalizeH3Resolution(%q) should have failed", input)
		}
	}
}

func TestH3Validate(t *testing.T) {
	base := func() *H3Request {
		return &H3Request{
			Prompt:     "p",
			Resolution: H3Resolution2K,
			Duration:   6,
		}
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("baseline request rejected: %v", err)
	}

	cases := map[string]func(*H3Request){
		"缺 prompt":  func(r *H3Request) { r.Prompt = "" },
		"prompt 超长": func(r *H3Request) { r.Prompt = string(make([]rune, H3MaxPromptLength+1)) },
		"时长过短":      func(r *H3Request) { r.Duration = 3 },
		"时长过长":      func(r *H3Request) { r.Duration = 16 },
		"分辨率非法":     func(r *H3Request) { r.Resolution = "1080P" },
		"比例非法":      func(r *H3Request) { r.Ratio = "5:4" },
		"参考图超上限":    func(r *H3Request) { r.RefImages = make([]string, H3MaxRefImages+1) },
		"参考视频超上限":   func(r *H3Request) { r.RefVideos = make([]string, H3MaxRefVideos+1) },
		"参考音频超上限":   func(r *H3Request) { r.RefAudios = make([]string, H3MaxRefAudios+1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := base()
			mutate(req)
			if err := req.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}

	t.Run("adaptive 是合法比例", func(t *testing.T) {
		req := base()
		req.Ratio = "adaptive"
		if err := req.Validate(); err != nil {
			t.Errorf("adaptive should be accepted: %v", err)
		}
	})
}

func TestH3FromV2Request_Errors(t *testing.T) {
	cases := map[string]*V2VideoRequest{
		"image_url 缺 url": {
			Content: []V2Content{{Type: "image_url", Role: "first_frame"}},
		},
		"未知 content 类型": {
			Content: []V2Content{{Type: "pdf_url"}},
		},
		"image_url 上的未知 role": {
			Content: []V2Content{{Type: "image_url", ImageURL: &V2URLRef{URL: "u"}, Role: "reference_video"}},
		},
		"分辨率非法": {
			Resolution: "1080P",
			Content:    []V2Content{{Type: "text", Text: "p"}},
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := H3FromV2Request(body); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}

	t.Run("空请求体", func(t *testing.T) {
		if _, err := H3FromV2Request(nil); err == nil {
			t.Error("expected error for nil request")
		}
	})
}

// TestH3FromV2Request_MultipleTextParts 多个 text 项按顺序拼接，不丢内容。
func TestH3FromV2Request_MultipleTextParts(t *testing.T) {
	req, err := H3FromV2Request(&V2VideoRequest{
		Resolution: "2K",
		Duration:   6,
		Content: []V2Content{
			{Type: "text", Text: "第一段"},
			{Type: "text", Text: "第二段"},
		},
	})
	if err != nil {
		t.Fatalf("H3FromV2Request failed: %v", err)
	}
	if req.Prompt != "第一段\n第二段" {
		t.Errorf("Prompt = %q", req.Prompt)
	}
}

// TestH3ToV2Request_ContentOrderIsStable content 顺序固定，便于比对与排查。
func TestH3ToV2Request_ContentOrderIsStable(t *testing.T) {
	req := &H3Request{
		Prompt:     "p",
		Resolution: H3Resolution2K,
		Duration:   6,
		FirstFrame: "f",
		LastFrame:  "l",
		RefImages:  []string{"i1", "i2"},
		RefVideos:  []string{"v1"},
		RefAudios:  []string{"a1"},
	}

	want := []struct{ typ, role string }{
		{"text", ""},
		{"image_url", "first_frame"},
		{"image_url", "last_frame"},
		{"image_url", "reference_image"},
		{"image_url", "reference_image"},
		{"video_url", "reference_video"},
		{"audio_url", "reference_audio"},
	}

	got := req.ToV2Request(ModelMiniMaxH3).Content
	if len(got) != len(want) {
		t.Fatalf("content length = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Type != w.typ || got[i].Role != w.role {
			t.Errorf("content[%d] = (%s, %s), want (%s, %s)", i, got[i].Type, got[i].Role, w.typ, w.role)
		}
	}
}

// TestH3ToV2Request_PreservesExplicitFalse 显式 false 必须原样送到上游，
// 不能被 omitempty 吞掉（CLAUDE.md Rule 6）。
func TestH3ToV2Request_PreservesExplicitFalse(t *testing.T) {
	req := &H3Request{Prompt: "p", Resolution: H3Resolution2K, Duration: 6, AigcWatermark: boolPtr(false)}
	v2 := req.ToV2Request(ModelMiniMaxH3)

	if v2.AigcWatermark == nil {
		t.Fatal("AigcWatermark was dropped")
	}
	if *v2.AigcWatermark {
		t.Error("AigcWatermark should be false")
	}

	// 未设置时保持 nil，让上游用它自己的默认值
	unset := &H3Request{Prompt: "p", Resolution: H3Resolution2K, Duration: 6}
	if unset.ToV2Request(ModelMiniMaxH3).AigcWatermark != nil {
		t.Error("unset AigcWatermark should stay nil")
	}
}

// TestH3ToV2Request_DropsCallbackURL 用户的回调地址不透传给上游，
// 否则上游会绕过网关直接回调用户，网关拿不到终态也就无法结算。
func TestH3ToV2Request_DropsCallbackURL(t *testing.T) {
	req := &H3Request{
		Prompt: "p", Resolution: H3Resolution2K, Duration: 6,
		CallbackURL: "https://user.example.com/hook",
	}
	if url := req.ToV2Request(ModelMiniMaxH3).CallbackURL; url != "" {
		t.Errorf("CallbackURL should not be forwarded upstream, got %q", url)
	}
}

// TestH3ToVideoUsage_FeedsPricing 归一化结果直接喂给定价引擎，
// 端到端验证「请求 → 计费口径 → 金额」这条链路。
func TestH3ToVideoUsage_FeedsPricing(t *testing.T) {
	cases := []struct {
		name  string
		req   *H3Request
		usage billing_setting.VideoUsage
		total float64
	}{
		{
			name: "10秒2K纯文生",
			req:  &H3Request{Prompt: "p", Resolution: H3Resolution2K, Duration: 10},
			usage: billing_setting.VideoUsage{
				Resolution: H3Resolution2K, OutputSeconds: 10,
			},
			total: 1.30,
		},
		{
			name: "5秒768P带首帧",
			req: &H3Request{
				Prompt: "p", Resolution: H3Resolution768P, Duration: 5,
				FirstFrame: "https://example.com/a.jpg",
			},
			usage: billing_setting.VideoUsage{
				Resolution: H3Resolution768P, OutputSeconds: 5, ImageCount: 1,
			},
			total: 0.44, // 5×0.08 + 1×0.04
		},
		{
			// 参考视频拿不到真实时长，按官方总时长上限 15s 预扣
			name: "带参考视频按上限预扣",
			req: &H3Request{
				Prompt: "p", Resolution: H3Resolution2K, Duration: 10,
				RefVideos: []string{"https://example.com/v.mp4"},
			},
			usage: billing_setting.VideoUsage{
				Resolution: H3Resolution2K, OutputSeconds: 10, VideoSeconds: 15,
			},
			total: 3.25, // 10×0.13 + 15×0.13
		},
		{
			name: "参考音频免费",
			req: &H3Request{
				Prompt: "p", Resolution: H3Resolution2K, Duration: 10,
				RefAudios: []string{"https://example.com/a.mp3"},
			},
			usage: billing_setting.VideoUsage{
				Resolution: H3Resolution2K, OutputSeconds: 10, AudioSeconds: 15,
			},
			total: 1.30,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usage := tc.req.ToVideoUsage()
			if !reflect.DeepEqual(usage, tc.usage) {
				t.Fatalf("usage = %+v, want %+v", usage, tc.usage)
			}
			cost, err := billing_setting.ComputeVideoCost(ModelMiniMaxH3, usage)
			if err != nil {
				t.Fatalf("ComputeVideoCost failed: %v", err)
			}
			if diff := cost.Total - tc.total; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("total = %.6f, want %.6f", cost.Total, tc.total)
			}
		})
	}
}

func TestIsH3Model(t *testing.T) {
	for _, name := range []string{"MiniMax-H3", "minimax-h3", " MiniMax-H3 "} {
		if !IsH3Model(name) {
			t.Errorf("IsH3Model(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"MiniMax-Hailuo-02", "MiniMax-H3-Regeneration", ""} {
		if IsH3Model(name) {
			t.Errorf("IsH3Model(%q) = true, want false", name)
		}
	}
}

// TestH3FromTaskSubmitReq_PlaygroundContent 操练场按 schema 的 x-content-role
// 把上传素材拍平成 metadata.content（结构同 v2 原生协议）。不解析它的话，
// 用户在操练场传的首尾帧和参考素材会被静默丢弃——请求照发，钱照扣，
// 但生成的是纯文生视频。
func TestH3FromTaskSubmitReq_PlaygroundContent(t *testing.T) {
	req := mustNormalize(t, func() (*H3Request, error) {
		return H3FromTaskSubmitReq(relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3,
			Prompt: "a boy playing basketball",
			Metadata: map[string]interface{}{
				"resolution": "2K",
				"duration":   10,
				"content": []interface{}{
					map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]interface{}{"url": "https://example.com/first.jpg"},
						"role":      "first_frame",
					},
					map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]interface{}{"url": "https://example.com/ref.jpg"},
						"role":      "reference_image",
					},
					map[string]interface{}{
						"type":      "video_url",
						"video_url": map[string]interface{}{"url": "https://example.com/motion.mp4"},
						"role":      "reference_video",
					},
					map[string]interface{}{
						"type":      "audio_url",
						"audio_url": map[string]interface{}{"url": "https://example.com/mood.mp3"},
						"role":      "reference_audio",
					},
				},
			},
		})
	})

	if req.FirstFrame != "https://example.com/first.jpg" {
		t.Errorf("FirstFrame = %q", req.FirstFrame)
	}
	if len(req.RefImages) != 1 || req.RefImages[0] != "https://example.com/ref.jpg" {
		t.Errorf("RefImages = %v", req.RefImages)
	}
	if len(req.RefVideos) != 1 || len(req.RefAudios) != 1 {
		t.Errorf("RefVideos = %v, RefAudios = %v", req.RefVideos, req.RefAudios)
	}

	// 素材必须进入计费口径：2 张图 + 参考视频按上限计
	usage := req.ToVideoUsage()
	if usage.ImageCount != 2 {
		t.Errorf("ImageCount = %d, want 2", usage.ImageCount)
	}
	if usage.VideoSeconds != H3MaxRefMediaSeconds {
		t.Errorf("VideoSeconds = %v, want %v", usage.VideoSeconds, H3MaxRefMediaSeconds)
	}
}
