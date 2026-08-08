package doubao

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

const seedance25Tolerance = 1e-6

func mustParseUnified(t *testing.T, body string) *Seedance25Request {
	t.Helper()
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalJsonStr(body, &req); err != nil {
		t.Fatalf("解析站内统一协议请求失败: %v", err)
	}
	s25, err := Seedance25FromTaskSubmitReq(req)
	if err != nil {
		t.Fatalf("归一化站内统一协议请求失败: %v", err)
	}
	s25.ApplyDefaults()
	return s25
}

func mustParseArk(t *testing.T, body string) *Seedance25Request {
	t.Helper()
	var in ArkIncomingRequest
	if err := common.UnmarshalJsonStr(body, &in); err != nil {
		t.Fatalf("解析火山 v3 原生请求失败: %v", err)
	}
	s25, err := Seedance25FromArkRequest(&in)
	if err != nil {
		t.Fatalf("归一化火山 v3 原生请求失败: %v", err)
	}
	s25.ApplyDefaults()
	return s25
}

// TestSeedance25_BothEntrancesAgree 站内统一协议与火山 v3 原生协议表达同一个
// 语义请求时，必须归一化成完全相同的结构。这是整套接入不出岔子的前提：任何
// 一条路径上的约束或计价口径改动，另一条自动跟着变。
func TestSeedance25_BothEntrancesAgree(t *testing.T) {
	unified := mustParseUnified(t, `{
		"model": "doubao-seedance-2-5",
		"prompt": "小猫对着镜头打哈欠",
		"duration": 8,
		"metadata": {
			"resolution": "720p",
			"aspect_ratio": "16:9",
			"generate_audio": true,
			"output_format": "mov",
			"web_search": true,
			"content": [
				{"type": "image_url", "image_url": {"url": "https://e.com/a.jpg"}, "role": "reference_image"},
				{"type": "video_url", "video_url": {"url": "https://e.com/v.mp4"}, "role": "reference_video"},
				{"type": "audio_url", "audio_url": {"url": "https://e.com/a.wav"}, "role": "reference_audio"}
			]
		}
	}`)

	native := mustParseArk(t, `{
		"model": "doubao-seedance-2-5-260628",
		"content": [
			{"type": "text", "text": "小猫对着镜头打哈欠"},
			{"type": "image_url", "image_url": {"url": "https://e.com/a.jpg"}, "role": "reference_image"},
			{"type": "video_url", "video_url": {"url": "https://e.com/v.mp4"}, "role": "reference_video"},
			{"type": "audio_url", "audio_url": {"url": "https://e.com/a.wav"}, "role": "reference_audio"}
		],
		"resolution": "720p",
		"ratio": "16:9",
		"duration": 8,
		"generate_audio": true,
		"output_format": "mov",
		"tools": [{"type": "web_search"}]
	}`)

	if unified.Prompt != native.Prompt {
		t.Errorf("prompt 不一致: %q vs %q", unified.Prompt, native.Prompt)
	}
	if unified.Resolution != native.Resolution || unified.Ratio != native.Ratio {
		t.Errorf("分辨率/宽高比不一致: %v/%v vs %v/%v",
			unified.Resolution, unified.Ratio, native.Resolution, native.Ratio)
	}
	if unified.Duration != native.Duration {
		t.Errorf("时长不一致: %d vs %d", unified.Duration, native.Duration)
	}
	if unified.OutputFormat != native.OutputFormat {
		t.Errorf("输出格式不一致: %q vs %q", unified.OutputFormat, native.OutputFormat)
	}
	if unified.WebSearch != native.WebSearch {
		t.Errorf("联网搜索开关不一致: %v vs %v", unified.WebSearch, native.WebSearch)
	}
	if len(unified.RefImages) != 1 || len(unified.RefVideos) != 1 || len(unified.RefAudios) != 1 {
		t.Errorf("统一协议的参考素材解析错误: %+v", unified)
	}
	if len(native.RefImages) != 1 || len(native.RefVideos) != 1 || len(native.RefAudios) != 1 {
		t.Errorf("原生协议的参考素材解析错误: %+v", native)
	}

	// 两边产出的上游请求体也必须一致
	if a, b := unified.ToArkRequest(ModelSeedance25Official),
		native.ToArkRequest(ModelSeedance25Official); len(a.Content) != len(b.Content) {
		t.Errorf("上游 content 长度不一致: %d vs %d", len(a.Content), len(b.Content))
	}
}

// TestSeedance25_Defaults 官方默认值：720p、模型自选时长。
func TestSeedance25_Defaults(t *testing.T) {
	r := mustParseUnified(t, `{"model":"doubao-seedance-2-5","prompt":"a cat"}`)
	if r.Resolution != Seedance25Resolution720P {
		t.Errorf("默认分辨率 = %q, want %q", r.Resolution, Seedance25Resolution720P)
	}
	if r.Duration != Seedance25AutoDuration {
		t.Errorf("默认时长 = %d, want %d", r.Duration, Seedance25AutoDuration)
	}
	if err := r.Validate(); err != nil {
		t.Errorf("默认值组合应当合法: %v", err)
	}
}

// TestSeedance25_MetadataDuration 操练场把 schema 参数整体塞进 metadata，不会
// 提升到顶层字段。这里收不到 duration 的话，用户在面板上调的时长会被静默丢弃。
func TestSeedance25_MetadataDuration(t *testing.T) {
	r := mustParseUnified(t, `{
		"model":"doubao-seedance-2-5","prompt":"a cat",
		"metadata":{"duration":12}
	}`)
	if r.Duration != 12 {
		t.Errorf("metadata.duration = %d, want 12", r.Duration)
	}

	// -1 是合法取值（模型自选时长），沿途不能被当成"未设置"而被默认值覆盖
	auto := mustParseUnified(t, `{
		"model":"doubao-seedance-2-5","prompt":"a cat",
		"metadata":{"duration":-1}
	}`)
	if auto.Duration != Seedance25AutoDuration {
		t.Errorf("metadata.duration=-1 后时长 = %d, want %d", auto.Duration, Seedance25AutoDuration)
	}

	// 顶层 duration 优先级更高
	top := mustParseUnified(t, `{
		"model":"doubao-seedance-2-5","prompt":"a cat","duration":8,
		"metadata":{"duration":12}
	}`)
	if top.Duration != 8 {
		t.Errorf("顶层 duration 应当优先, got %d", top.Duration)
	}
}

// TestSeedance25Schema_DurationOffersAutoOption 操练场的时长控件必须能选到
// -1（模型自选）——它是 2.5 的官方默认值，也是视频编辑场景唯一可用的取值。
// schema 里 enum 缺了它，操练场就没法发起这类任务。
func TestSeedance25Schema_DurationOffersAutoOption(t *testing.T) {
	// 只把 duration 那一项解出来：同一份 schema 里其它字段的 enum 是字符串，
	// 整体用同一个结构解会类型冲突。
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := constant.GetDefaultModelParamSchema(ModelSeedance25)
	if raw == "" {
		t.Fatal("内置 schema 丢了")
	}
	if err := common.UnmarshalJsonStr(raw, &schema); err != nil {
		t.Fatalf("schema 不是合法 JSON: %v", err)
	}

	durationRaw, ok := schema.Properties["duration"]
	if !ok {
		t.Fatal("schema 里没有 duration")
	}
	var duration struct {
		Enum       []int             `json:"enum"`
		EnumLabels map[string]string `json:"enumLabels"`
		Default    *int              `json:"default"`
	}
	if err := common.Unmarshal(durationRaw, &duration); err != nil {
		t.Fatalf("duration 项解析失败: %v", err)
	}
	if duration.Default == nil || *duration.Default != Seedance25AutoDuration {
		t.Errorf("默认值 = %v, want %d（官方默认就是模型自选）",
			duration.Default, Seedance25AutoDuration)
	}
	if duration.EnumLabels[strconv.Itoa(Seedance25AutoDuration)] == "" {
		t.Error("-1 缺少友好文案，用户会看到一个莫名其妙的 -1")
	}

	seen := make(map[int]bool, len(duration.Enum))
	for _, v := range duration.Enum {
		seen[v] = true
	}
	if !seen[Seedance25AutoDuration] {
		t.Error("enum 里必须有 -1，否则操练场发不了视频编辑任务")
	}
	// 4~30 全覆盖，且不能混进 0~3 这些会被后端拒掉的值
	for v := Seedance25MinDuration; v <= Seedance25MaxDuration; v++ {
		if !seen[v] {
			t.Errorf("enum 缺少合法时长 %d", v)
		}
	}
	for _, v := range duration.Enum {
		if v != Seedance25AutoDuration &&
			(v < Seedance25MinDuration || v > Seedance25MaxDuration) {
			t.Errorf("enum 里有会被后端拒掉的非法值 %d", v)
		}
	}
}

// TestSeedance25_RejectsUnsupportedParams 2.5 不支持的参数必须明确报错。
// 静默剔除更糟：用户以为 seed 生效了、拿到的却是随机结果。
func TestSeedance25_RejectsUnsupportedParams(t *testing.T) {
	cases := map[string]string{
		"seed":         `{"model":"m","content":[{"type":"text","text":"a"}],"seed":42}`,
		"camera_fixed": `{"model":"m","content":[{"type":"text","text":"a"}],"camera_fixed":true}`,
		"frames":       `{"model":"m","content":[{"type":"text","text":"a"}],"frames":121}`,
		"draft":        `{"model":"m","content":[{"type":"text","text":"a"}],"draft":true}`,
		"service_tier": `{"model":"m","content":[{"type":"text","text":"a"}],"service_tier":"flex"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var in ArkIncomingRequest
			if err := common.UnmarshalJsonStr(body, &in); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			_, err := Seedance25FromArkRequest(&in)
			if err == nil {
				t.Fatalf("传了 %s 应当报错", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("错误信息里应当点名 %s，实际: %v", name, err)
			}
		})
	}

	// service_tier=default 等价于不填，不该被拦
	var in ArkIncomingRequest
	_ = common.UnmarshalJsonStr(
		`{"model":"m","content":[{"type":"text","text":"a"}],"service_tier":"default"}`, &in)
	if _, err := Seedance25FromArkRequest(&in); err != nil {
		t.Errorf("service_tier=default 不应报错: %v", err)
	}
}

// TestSeedance25_Validate 只校验官方文档写死的约束。
func TestSeedance25_Validate(t *testing.T) {
	base := func() *Seedance25Request {
		return &Seedance25Request{
			Prompt:     "a cat",
			Resolution: Seedance25Resolution720P,
			Duration:   Seedance25AutoDuration,
		}
	}
	ptrInt := func(n int) *int { return &n }

	cases := []struct {
		name    string
		mutate  func(*Seedance25Request)
		wantErr string
	}{
		{"合法请求", func(*Seedance25Request) {}, ""},
		{"四类内容全空", func(r *Seedance25Request) { r.Prompt = "" }, "at least one"},
		{"仅音频合法", func(r *Seedance25Request) {
			r.Prompt = ""
			r.RefAudios = []string{"https://e.com/a.wav"}
		}, ""},
		{"时长过短", func(r *Seedance25Request) { r.Duration = 3 }, "duration"},
		{"时长过长", func(r *Seedance25Request) { r.Duration = 31 }, "duration"},
		{"时长合法", func(r *Seedance25Request) { r.Duration = 30 }, ""},
		{"分辨率非法", func(r *Seedance25Request) { r.Resolution = "1080p" }, "resolution"},
		{"宽高比非法", func(r *Seedance25Request) { r.Ratio = "2:1" }, "ratio"},
		{"首帧与参考素材互斥", func(r *Seedance25Request) {
			r.FirstFrame = "https://e.com/f.jpg"
			r.RefImages = []string{"https://e.com/r.jpg"}
		}, "mutually exclusive"},
		{"尾帧缺首帧", func(r *Seedance25Request) { r.LastFrame = "https://e.com/l.jpg" }, "last_frame requires"},
		{"首尾帧合法", func(r *Seedance25Request) {
			r.FirstFrame = "https://e.com/f.jpg"
			r.LastFrame = "https://e.com/l.jpg"
		}, ""},
		{"参考图超限", func(r *Seedance25Request) {
			r.RefImages = make([]string, Seedance25MaxRefImages+1)
		}, "reference images"},
		{"参考视频超限", func(r *Seedance25Request) {
			r.RefVideos = make([]string, Seedance25MaxRefVideos+1)
		}, "reference videos"},
		{"参考音频超限", func(r *Seedance25Request) {
			r.RefAudios = make([]string, Seedance25MaxRefAudios+1)
		}, "reference audios"},
		{"输出格式非法", func(r *Seedance25Request) { r.OutputFormat = "webm" }, "output_format"},
		{"优先级越界", func(r *Seedance25Request) { r.Priority = ptrInt(10) }, "priority"},
		{"超时阈值越界", func(r *Seedance25Request) { r.ExecutionExpiresAfter = ptrInt(60) }, "execution_expires_after"},
		{"用户标识过长", func(r *Seedance25Request) {
			r.SafetyIdentifier = strings.Repeat("x", 65)
		}, "safety_identifier"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base()
			tc.mutate(r)
			err := r.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("应当通过校验，实际报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("应当报错 %q，实际通过", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("错误信息 = %q, 应当包含 %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestSeedance25_ResolutionRejectsUnsupported 1080p / 4k 明确报错而不是静默
// 降级——用户要 1080p 却拿到 720p，比直接报错更糟。
func TestSeedance25_ResolutionRejectsUnsupported(t *testing.T) {
	for _, value := range []string{"1080p", "4k", "1920x1080"} {
		if _, err := normalizeSeedance25Resolution(value); err == nil {
			t.Errorf("%q 应当被拒绝", value)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"720P", Seedance25Resolution720P},
		{"1280x720", Seedance25Resolution720P},
		{"480p", Seedance25Resolution480P},
		{"854x480", Seedance25Resolution480P},
	} {
		got, err := normalizeSeedance25Resolution(tc.in)
		if err != nil {
			t.Errorf("%q 归一化出错: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("%q 归一化 = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestSeedance25_ToArkRequest content 顺序固定，且回调地址不透传。
func TestSeedance25_ToArkRequest(t *testing.T) {
	r := &Seedance25Request{
		Prompt:     "a cat",
		Resolution: Seedance25Resolution720P,
		Duration:   Seedance25AutoDuration,
		FirstFrame: "https://e.com/f.jpg",
		LastFrame:  "https://e.com/l.jpg",
		WebSearch:  true,
	}
	req := r.ToArkRequest(ModelSeedance25Official)

	if req.Model != ModelSeedance25Official {
		t.Errorf("上游模型名 = %q", req.Model)
	}
	wantTypes := []string{arkContentTypeText, arkContentTypeImageURL, arkContentTypeImageURL}
	if len(req.Content) != len(wantTypes) {
		t.Fatalf("content 条数 = %d, want %d", len(req.Content), len(wantTypes))
	}
	for i, want := range wantTypes {
		if req.Content[i].Type != want {
			t.Errorf("content[%d].type = %q, want %q", i, req.Content[i].Type, want)
		}
	}
	if req.Content[1].Role != arkRoleFirstFrame || req.Content[2].Role != arkRoleLastFrame {
		t.Errorf("首尾帧 role 顺序错误: %+v", req.Content)
	}
	if req.Duration == nil || *req.Duration != Seedance25AutoDuration {
		t.Errorf("duration 应当原样带上 -1: %+v", req.Duration)
	}
	if len(req.Tools) != 1 || req.Tools[0].Type != Seedance25ToolWebSearch {
		t.Errorf("联网搜索工具没带上: %+v", req.Tools)
	}
}

// TestSeedance25_ToArkRequestOmitsEmptyPrompt 无提示词场景不能给上游推一个
// 空 text 项，官方会因此校验失败。
func TestSeedance25_ToArkRequestOmitsEmptyPrompt(t *testing.T) {
	r := &Seedance25Request{
		Resolution: Seedance25Resolution720P,
		Duration:   Seedance25AutoDuration,
		RefAudios:  []string{"https://e.com/a.wav"},
	}
	req := r.ToArkRequest(ModelSeedance25Official)
	for _, item := range req.Content {
		if item.Type == arkContentTypeText {
			t.Fatalf("空 prompt 不该产生 text 条目: %+v", req.Content)
		}
	}
}

// TestSeedance25_ToVideoUsage 预扣口径：时长未知按价目表折中值，参考视频按
// 官方总时长上限，并标记走降档单价。
func TestSeedance25_ToVideoUsage(t *testing.T) {
	auto := (&Seedance25Request{
		Resolution: Seedance25Resolution720P,
		Duration:   Seedance25AutoDuration,
	}).ToVideoUsage(15)
	if auto.OutputSeconds != 15 {
		t.Errorf("时长未知时应按兜底值 15 秒预扣, got %v", auto.OutputSeconds)
	}
	if auto.HasVideoInput {
		t.Error("没有参考视频不该走降档档位")
	}

	withVideo := (&Seedance25Request{
		Resolution: Seedance25Resolution720P,
		Duration:   8,
		FirstFrame: "https://e.com/f.jpg",
		RefVideos:  []string{"https://e.com/v.mp4"},
	}).ToVideoUsage(15)
	if withVideo.OutputSeconds != 8 {
		t.Errorf("显式时长应当优先, got %v", withVideo.OutputSeconds)
	}
	if withVideo.VideoSeconds != Seedance25MaxRefMediaSeconds {
		t.Errorf("参考视频应按上限 %d 秒预扣, got %v",
			Seedance25MaxRefMediaSeconds, withVideo.VideoSeconds)
	}
	if !withVideo.HasVideoInput {
		t.Error("带参考视频必须标记降档")
	}
	if withVideo.ImageCount != 1 {
		t.Errorf("图片张数 = %d, want 1", withVideo.ImageCount)
	}
}

// TestSeedance25Usage_RejectsUnpriceableRequest 时长未知而价目表没配兜底秒数
// 时必须报错：按 0 秒预扣等于放行一个不知道该收多少钱的任务。
func TestSeedance25Usage_RejectsUnpriceableRequest(t *testing.T) {
	r := &Seedance25Request{Resolution: Seedance25Resolution720P, Duration: Seedance25AutoDuration}
	if _, err := Seedance25Usage("不存在的模型", r); err == nil {
		t.Error("模型没配价目表应当报错")
	}
	if _, err := Seedance25Usage(ModelSeedance25, r); err != nil {
		t.Errorf("内置价目表配了 max_output_seconds，不该报错: %v", err)
	}
}

// TestSeedance25BillableSeconds 结算换算的锚点。
//
// 上游公式 tokens = 宽 × 高 × 帧数 / 1024，这里是它的逆运算。数值全部来自
// 官方价目表反推并交叉验证过的组合，改错任何一处这里都会炸。
func TestSeedance25BillableSeconds(t *testing.T) {
	cases := []struct {
		name       string
		tokens     int
		resolution string
		ratio      string
		fps        int
		want       float64
	}{
		// 1280×720 × 120 帧 / 1024 = 108,000
		{"720p输出5秒", 108000, "720p", "16:9", 24, 5},
		// 854×480 × 240 帧 / 1024 = 96,075
		{"480p输出10秒", 96075, "480p", "16:9", 24, 10},
		// 输出 5 秒 + 参考视频 30 秒 = 840 帧 → 756,000
		{"720p输出5秒加30秒参考视频", 756000, "720p", "16:9", 24, 35},
		// 992×432 × 120 帧 / 1024 = 50,220
		{"480p21:9输出5秒", 50220, "480p", "21:9", 24, 5},
		// 帧率缺失时按官方出片帧率 24 兜底
		{"帧率缺失", 108000, "720p", "16:9", 0, 5},
		// 上游没回具体宽高比时按定价基准 16:9 算
		{"宽高比缺失", 108000, "720p", "", 24, 5},
		{"宽高比为adaptive", 108000, "720p", "adaptive", 24, 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Seedance25BillableSeconds(tc.tokens, tc.resolution, tc.ratio, tc.fps)
			if !ok {
				t.Fatal("换算失败")
			}
			if math.Abs(got-tc.want) > seedance25Tolerance {
				t.Errorf("计费秒数 = %v, want %v", got, tc.want)
			}
		})
	}

	if _, ok := Seedance25BillableSeconds(0, "720p", "16:9", 24); ok {
		t.Error("token 为 0 时不能给出结算口径")
	}
	if _, ok := Seedance25BillableSeconds(108000, "1080p", "16:9", 24); ok {
		t.Error("认不出的分辨率必须放弃结算，而不是猜一个数字去扣钱")
	}
}

// TestIsSeedance25Model 别名归一：对外名、官方 endpoint 名、常见写法都要认。
func TestIsSeedance25Model(t *testing.T) {
	for _, name := range []string{
		"doubao-seedance-2-5",
		"doubao-seedance-2-5-260628",
		"doubao-seedance-2.5",
		"seedance-2.5",
		"seedance-2.5-api",
		"Doubao-Seedance-2-5",
		"  doubao-seedance-2-5  ",
	} {
		if !IsSeedance25Model(name) {
			t.Errorf("%q 应当识别为 Seedance 2.5", name)
		}
	}
	for _, name := range []string{
		"doubao-seedance-2-0-260128",
		"seedance-2.0",
		"doubao-seedance-1-5-pro-251215",
		"MiniMax-H3",
		"",
	} {
		if IsSeedance25Model(name) {
			t.Errorf("%q 不该被识别为 Seedance 2.5", name)
		}
	}
}

// TestSeedance25_NotInLegacyPricingMap 2.5 走价目表计费，绝不能命中 2.0 那张
// 相对倍率表——两套计价同时作用会算出完全错误的金额。
func TestSeedance25_NotInLegacyPricingMap(t *testing.T) {
	for _, name := range []string{"doubao-seedance-2-5", ModelSeedance25Official} {
		if _, ok := GetSeedancePricingRatio(name, "720p", false); ok {
			t.Errorf("%q 不该命中 2.0 的档位倍率表", name)
		}
	}
	// 2.0 的档位表本身不受影响
	if _, ok := GetSeedancePricingRatio("doubao-seedance-2-0-260128", "720p", true); !ok {
		t.Error("2.0 的档位倍率表被破坏了")
	}
}

// TestSeedance25_ContentErrors content 数组里的非法条目要报错而不是静默丢弃。
func TestSeedance25_ContentErrors(t *testing.T) {
	cases := map[string]string{
		"缺 url":   `{"model":"m","content":[{"type":"image_url","role":"first_frame"}]}`,
		"未知 type": `{"model":"m","content":[{"type":"file_url"}]}`,
		"未知 role": `{"model":"m","content":[{"type":"image_url","image_url":{"url":"u"},"role":"middle"}]}`,
		"样片任务":    `{"model":"m","content":[{"type":"draft_task"}]}`,
		"未知 tool": `{"model":"m","content":[{"type":"text","text":"a"}],"tools":[{"type":"code"}]}`,
		"视频角色不匹配": `{"model":"m","content":[{"type":"video_url","video_url":{"url":"u"},"role":"reference_image"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var in ArkIncomingRequest
			if err := common.UnmarshalJsonStr(body, &in); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if _, err := Seedance25FromArkRequest(&in); err == nil {
				t.Error("应当报错")
			}
		})
	}
}

// TestSeedance25_AllAliasesArePriced seedanceAliasMap 认得的名字都会被路由到
// 2.5，那么每一个都必须能定价——GetVideoPricing 是精确查表，漏一个那条路径就
// 会掉回 ModelPrice，同一个模型的两个名字算出不同的钱。
func TestSeedance25_AllAliasesArePriced(t *testing.T) {
	for alias := range seedanceAliasMap {
		if !IsSeedance25Model(alias) {
			continue
		}
		r := &Seedance25Request{
			Prompt:     "a cat",
			Resolution: Seedance25Resolution720P,
			Duration:   Seedance25AutoDuration,
		}
		if _, err := Seedance25Usage(alias, r); err != nil {
			t.Errorf("别名 %q 路由到 2.5 却没配价目表: %v", alias, err)
		}
	}
}

// TestSeedance25_PricingCoversEveryAspectRatio 秒价一档分辨率只有一个数，而同
// 一分辨率下不同宽高比的像素数不一样、每秒 token 数也就不一样。价目表按该档
// 里【像素最多】的宽高比定价，所以任何比例都不该出现成本倒挂。
//
// 这条断言的是定价策略本身：谁把秒价改回按 16:9 定，21:9 和 4:3 就会亏钱。
func TestSeedance25_PricingCoversEveryAspectRatio(t *testing.T) {
	// 推导秒价时用的 token 单价（美元/百万 token，不含视频输入档）
	const (
		usdPerMillionTokens = 10.37
		outputSeconds       = 5.0
	)

	if _, ok := billing_setting.GetVideoPricing(ModelSeedance25); !ok {
		t.Fatal("doubao-seedance-2-5 应有内置价目表")
	}

	for resolution, byRatio := range seedance25PixelTable {
		for ratio, pixels := range byRatio {
			t.Run(resolution+"_"+ratio, func(t *testing.T) {
				// 上游按 token 收：像素 × 帧数 / 1024。两边必须用同一个整数
				// token 数，否则截断误差会被误读成成本倒挂。
				tokens := int(math.Round(
					float64(pixels) * outputSeconds * Seedance25DefaultFPS / 1024))
				upstreamUSD := float64(tokens) * usdPerMillionTokens / 1e6

				// 我们按秒收：结算换算用的是实际宽高比的像素，等效秒即真实秒
				seconds, ok := Seedance25BillableSeconds(
					tokens, resolution, ratio, Seedance25DefaultFPS)
				if !ok {
					t.Fatal("换算失败")
				}
				cost, err := billing_setting.ComputeVideoCost(ModelSeedance25,
					billing_setting.VideoUsage{Resolution: resolution, OutputSeconds: seconds})
				if err != nil {
					t.Fatalf("ComputeVideoCost 出错: %v", err)
				}

				if cost.Total < upstreamUSD {
					t.Errorf("成本倒挂：收 $%.6f < 上游 $%.6f（差 %.2f%%）",
						cost.Total, upstreamUSD, (upstreamUSD/cost.Total-1)*100)
				}
				// 最贵的比例应当基本持平，不该出现远超成本的档位
				if cost.Total > upstreamUSD*1.06 {
					t.Errorf("多收过头：收 $%.6f，上游 $%.6f（高 %.2f%%）",
						cost.Total, upstreamUSD, (cost.Total/upstreamUSD-1)*100)
				}
			})
		}
	}
}

// TestSeedance25_EveryAliasIsFullyWired 一个模型名要能用，三样东西都得跟上：
// 路由（seedanceAliasMap）、定价（video_pricing）、操练场参数面板
// （DefaultModelParamSchemas）。三张表都是精确匹配模型名，任何一张漏一个名字，
// 那条路径就会以一种很难联想到的方式坏掉。
func TestSeedance25_EveryAliasIsFullyWired(t *testing.T) {
	for alias := range seedanceAliasMap {
		if !IsSeedance25Model(alias) {
			continue
		}
		t.Run(alias, func(t *testing.T) {
			if _, ok := billing_setting.GetVideoPricing(alias); !ok {
				t.Error("能路由到 2.5 却查不到价")
			}
			if constant.GetDefaultModelParamSchema(alias) == "" {
				t.Error("缺内置参数 schema，操练场右栏会是空的")
			}
			if !common.IsVideoGenerationModel(alias) {
				t.Error("识别不出是视频模型，端点与操练场工作区都会走错")
			}
		})
	}
}

// TestSeedance25_UnroledImageIsFirstFrame 官方对「图生视频-首帧」的定义是
// 「传入 1 个 image_url 对象，role 为 first_frame 或不填」——孤零零一张无角色
// 图片是首帧，不是参考图。这两者是互斥的两种场景，认错了出片效果完全不同。
func TestSeedance25_UnroledImageIsFirstFrame(t *testing.T) {
	single := mustParseArk(t, `{
		"model":"doubao-seedance-2-5",
		"content":[
			{"type":"text","text":"镜头缓慢推近"},
			{"type":"image_url","image_url":{"url":"https://e.com/a.jpg"}}
		]
	}`)
	if single.FirstFrame != "https://e.com/a.jpg" {
		t.Errorf("单张无角色图片应当作首帧, FirstFrame=%q RefImages=%v",
			single.FirstFrame, single.RefImages)
	}
	if len(single.RefImages) != 0 {
		t.Errorf("不该同时落进参考图: %v", single.RefImages)
	}

	// 多张无角色图片：官方没定义这种写法，按参考图处理
	multi := mustParseArk(t, `{
		"model":"doubao-seedance-2-5",
		"content":[
			{"type":"image_url","image_url":{"url":"https://e.com/a.jpg"}},
			{"type":"image_url","image_url":{"url":"https://e.com/b.jpg"}}
		]
	}`)
	if multi.FirstFrame != "" || len(multi.RefImages) != 2 {
		t.Errorf("多张无角色图片应当按参考图处理, FirstFrame=%q RefImages=%v",
			multi.FirstFrame, multi.RefImages)
	}

	// 与其它素材混用时同样按参考图，交给互斥校验和上游
	mixed := mustParseArk(t, `{
		"model":"doubao-seedance-2-5",
		"content":[
			{"type":"image_url","image_url":{"url":"https://e.com/a.jpg"}},
			{"type":"video_url","video_url":{"url":"https://e.com/v.mp4"},"role":"reference_video"}
		]
	}`)
	if mixed.FirstFrame != "" || len(mixed.RefImages) != 1 {
		t.Errorf("混用素材时无角色图片应当按参考图处理: %+v", mixed)
	}

	// 显式 first_frame 仍然优先
	explicit := mustParseArk(t, `{
		"model":"doubao-seedance-2-5",
		"content":[
			{"type":"image_url","image_url":{"url":"https://e.com/f.jpg"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://e.com/x.jpg"}}
		]
	}`)
	if explicit.FirstFrame != "https://e.com/f.jpg" || len(explicit.RefImages) != 1 {
		t.Errorf("显式 first_frame 应当保持: %+v", explicit)
	}
}

// TestSeedance25_FrameScenarioRatioMustBeAdaptive 首帧/首尾帧场景官方
// 「默认且仅支持 adaptive」——输出宽高比强制跟随首帧图片。
func TestSeedance25_FrameScenarioRatioMustBeAdaptive(t *testing.T) {
	base := func(ratio string) *Seedance25Request {
		return &Seedance25Request{
			Prompt:     "a cat",
			Resolution: Seedance25Resolution720P,
			Duration:   Seedance25AutoDuration,
			FirstFrame: "https://e.com/f.jpg",
			Ratio:      ratio,
		}
	}

	for _, ratio := range []string{"", "adaptive", "ADAPTIVE"} {
		if err := base(ratio).Validate(); err != nil {
			t.Errorf("ratio=%q 应当放行: %v", ratio, err)
		}
	}
	for _, ratio := range []string{"16:9", "9:16", "21:9"} {
		if err := base(ratio).Validate(); err == nil {
			t.Errorf("首帧场景 ratio=%q 应当被拒", ratio)
		}
	}

	// 参考生视频不受这条限制
	ref := &Seedance25Request{
		Prompt:     "a cat",
		Resolution: Seedance25Resolution720P,
		Duration:   Seedance25AutoDuration,
		RefImages:  []string{"https://e.com/r.jpg"},
		Ratio:      "16:9",
	}
	if err := ref.Validate(); err != nil {
		t.Errorf("参考生视频可以指定宽高比: %v", err)
	}
}
