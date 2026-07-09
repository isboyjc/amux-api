package poyo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestPickUpstreamModel(t *testing.T) {
	cases := []struct {
		base     string
		hasImage bool
		want     string
	}{
		{"seedream-4.5", false, "seedream-4.5"},     // 文生图 → 基础名
		{"seedream-4.5", true, "seedream-4.5-edit"}, // 图生图 → -edit
		{"seedream-5.0-lite", true, "seedream-5.0-lite-edit"},
		{"seedream-4.5-edit", true, "seedream-4.5-edit"}, // 已是 -edit,不重复拼
	}
	for _, c := range cases {
		if got := pickUpstreamModel(c.base, c.hasImage); got != c.want {
			t.Errorf("pickUpstreamModel(%q, %v) = %q, want %q", c.base, c.hasImage, got, c.want)
		}
	}
}

func TestCollectImageURLsFromRequest(t *testing.T) {
	// image 单个 URL、images 数组;只取 http(s),base64/data-url 被忽略(不托管)。
	req := &dto.ImageRequest{
		Image:  json.RawMessage(`"https://a.com/1.png"`),
		Images: json.RawMessage(`["https://a.com/2.png","data:image/png;base64,AAAA","not-a-url"]`),
	}
	got := collectImageURLsFromRequest(req)
	want := []string{"https://a.com/1.png", "https://a.com/2.png"}
	if len(got) != len(want) {
		t.Fatalf("got %d urls %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("url[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// 无图片字段 → 空(文生图不受影响)。
	if s := collectImageURLsFromRequest(&dto.ImageRequest{}); len(s) != 0 {
		t.Errorf("empty request should yield 0 urls, got %v", s)
	}
}

func TestSubmitResponseParsing(t *testing.T) {
	body := []byte(`{"code":200,"data":{"task_id":"task-abc","status":"not_started","created_time":"t"}}`)
	var r poyoSubmitResponse
	if err := common.Unmarshal(body, &r); err != nil {
		t.Fatalf("unmarshal submit: %v", err)
	}
	if r.Code != 200 || r.Data.TaskID != "task-abc" {
		t.Errorf("parsed submit = %+v", r)
	}
}

func TestStatusResponseParsing(t *testing.T) {
	// 真实 poyo 响应:progress / credits_amount 是浮点(100.0 / 5.0)。
	// 回归:若把 Progress 声明成 int,这里会反序列化失败 → 完成态永远读不到 → 超时。
	body := []byte(`{"code":200,"data":{"task_id":"C72IEII8KUGTWYQJ","status":"finished",` +
		`"files":[{"file_url":"https://cdn.doculator.org/images/x/x.png","file_type":"image"}],` +
		`"created_time":"2026-07-09T08:58:27","progress":100.0,"credits_amount":5.0}}`)
	var r poyoStatusResponse
	if err := common.Unmarshal(body, &r); err != nil {
		t.Fatalf("unmarshal status (float progress): %v", err)
	}
	if r.Data.Status != statusFinished {
		t.Errorf("status = %q, want %q", r.Data.Status, statusFinished)
	}
	if got := firstFileURL(r.Data.Files); got != "https://cdn.doculator.org/images/x/x.png" {
		t.Errorf("firstFileURL = %q, want the cdn url", got)
	}
}

// 用 Anthropic 官方文档「Resolution and token cost」一节的对照表当基准,
// 守住 patch 公式与 patchSize 常数。文档:
// https://platform.claude.com/docs/en/build-with-claude/vision
func TestEstimateImageTokens(t *testing.T) {
	cases := []struct{ w, h, want int }{
		{200, 200, 64},
		{1000, 1000, 1296},
		{1092, 1092, 1521},
		{0, 100, 0},
		{100, -1, 0},
	}
	for _, c := range cases {
		if got := estimateImageTokens(c.w, c.h); got != c.want {
			t.Errorf("estimateImageTokens(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestParseImageSize(t *testing.T) {
	cases := []struct {
		in   string
		w, h int
	}{
		{"1728x3072", 1728, 3072},
		{"2048*1152", 2048, 1152},
		{"2K-16:9", 2048, 1152},
		{"4K-1:1", 4096, 4096},
		{"3K-9:16", 1728, 3072},
		{"2K", 2048, 2048},
		{"16:9", 2048, 1152},
		{"", 2048, 2048},
		{"garbage", 2048, 2048},
	}
	for _, c := range cases {
		w, h := parseImageSize(c.in)
		if w != c.w || h != c.h {
			t.Errorf("parseImageSize(%q) = %dx%d, want %dx%d", c.in, w, h, c.w, c.h)
		}
	}
}

func TestCapsFor(t *testing.T) {
	cases := []struct {
		model     string
		wantKnown bool
		wantMaxN  int
	}{
		{"seedream-4.5", true, 15},
		{"seedream-4.5-edit", true, 15}, // -edit 变体沿用基础模型约束
		{"seedream-5.0-lite", true, 15},
		{"seedream-5.0-lite-edit", true, 15},
		{"some-future-model", false, 0}, // 未知模型放行
	}
	for _, c := range cases {
		got, ok := capsFor(c.model)
		if ok != c.wantKnown {
			t.Errorf("capsFor(%q) known = %v, want %v", c.model, ok, c.wantKnown)
			continue
		}
		if got.maxN != c.wantMaxN {
			t.Errorf("capsFor(%q).maxN = %d, want %d", c.model, got.maxN, c.wantMaxN)
		}
	}
}

// n 超上限必须在本地拦下,不白跑一次上游提交;带图切 -edit 变体。
func TestConvertImageRequestCaps(t *testing.T) {
	uintPtr := func(v uint) *uint { return &v }
	info := &relaycommon.RelayInfo{}
	a := &Adaptor{}

	if _, err := a.ConvertImageRequest(nil, info, dto.ImageRequest{
		Model: "seedream-4.5", Prompt: "x", N: uintPtr(16),
	}); err == nil {
		t.Error("seedream-4.5 n=16 should be rejected")
	}

	got, err := a.ConvertImageRequest(nil, info, dto.ImageRequest{
		Model: "seedream-4.5", Prompt: "x", N: uintPtr(15), Size: "2048x1152",
	})
	if err != nil {
		t.Fatalf("seedream-4.5 n=15: %v", err)
	}
	if req := got.(poyoSubmitRequest); req.Model != "seedream-4.5" || req.Input.N != 15 {
		t.Errorf("got %+v", req)
	}

	got, err = a.ConvertImageRequest(nil, info, dto.ImageRequest{
		Model: "seedream-5.0-lite", Prompt: "x",
		Images: json.RawMessage(`["https://a.com/1.png"]`),
	})
	if err != nil {
		t.Fatalf("lite edit: %v", err)
	}
	if req := got.(poyoSubmitRequest); req.Model != "seedream-5.0-lite-edit" {
		t.Errorf("model = %q, want seedream-5.0-lite-edit", req.Model)
	}
}

// 轮询超时随 n 线性缩放:60s + 30s×n,并以管理员配置为下限。
// 实测基准:n=2 全程约 51s、n=4 约 78s(含提交/下载/base64),预算须留足余量。
func TestPollTimeoutFor(t *testing.T) {
	cases := []struct {
		n    int
		want time.Duration
	}{
		{0, 90 * time.Second},  // n<1 视为 1
		{1, 90 * time.Second},  // 60 + 30
		{2, 120 * time.Second}, // 实测 51s，余量充足
		{4, 180 * time.Second}, // 实测 78s
		{15, 510 * time.Second},
		{10000, maxPollTimeout}, // 绕过 n 校验的路径(透传/未知模型)不得让超时无界
		{-5, 90 * time.Second},  // 负数(uint→int 溢出)视为 1
	}
	for _, c := range cases {
		if got := pollTimeoutFor("seedream-4.5", c.n); got != c.want {
			t.Errorf("pollTimeoutFor(n=%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestRequestedImageCount(t *testing.T) {
	uintPtr := func(v uint) *uint { return &v }
	cases := []struct {
		req  *dto.ImageRequest
		want int
	}{
		{&dto.ImageRequest{}, 1},              // 未指定 n
		{&dto.ImageRequest{N: uintPtr(0)}, 1}, // n=0 兜底成 1
		{&dto.ImageRequest{N: uintPtr(4)}, 4},
	}
	for _, c := range cases {
		info := &relaycommon.RelayInfo{Request: c.req}
		if got := requestedImageCount(info); got != c.want {
			t.Errorf("requestedImageCount(%+v) = %d, want %d", c.req, got, c.want)
		}
	}
	// info.Request 不是 ImageRequest 时不 panic
	if got := requestedImageCount(&relaycommon.RelayInfo{}); got != 1 {
		t.Errorf("nil request should default to 1, got %d", got)
	}
}

// n 是 *uint,直接 int() 转换会让超大值溢出成负数,从而绕过 maxN 校验
// 并把负数 n 发给上游。回归守卫。
func TestConvertImageRequestRejectsOverflowN(t *testing.T) {
	huge := uint(1) << 63
	_, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{}, dto.ImageRequest{
		Model: "seedream-4.5", Prompt: "x", N: &huge,
	})
	if err == nil {
		t.Fatal("overflowing n should be rejected, not silently sent upstream")
	}
}

// token 估算只在按次计价时给出。倍率计价 / 阶梯计费会拿 token 当计费基数,
// 展示用的估算值(一张 2K 图 5476 token)会把账单放大几千倍。
func TestEstimateUsageOnlyWhenUsePrice(t *testing.T) {
	req := &dto.ImageRequest{Prompt: "a red cube", Size: "2048x2048"}
	resp := dto.ImageResponse{Data: []dto.ImageData{{B64Json: "x"}}}

	// 按次计价:给出真实估算
	priced := &relaycommon.RelayInfo{Request: req}
	priced.PriceData.UsePrice = true
	u := estimateUsage(priced, resp)
	if u.CompletionTokens != 5476 { // 2048x2048 → ceil(2048/28)^2 = 74^2
		t.Errorf("UsePrice: CompletionTokens = %d, want 5476", u.CompletionTokens)
	}
	if u.PromptTokens == 0 {
		t.Error("UsePrice: PromptTokens should count the prompt")
	}

	// 倍率计价:退回 1/0,绝不让展示值变成计费基数
	ratio := &relaycommon.RelayInfo{Request: req}
	ratio.PriceData.UsePrice = false
	u = estimateUsage(ratio, resp)
	if u.PromptTokens != 1 || u.CompletionTokens != 0 || u.TotalTokens != 1 {
		t.Errorf("!UsePrice: got %+v, want 1/0/1", u)
	}
}

// 漏配 model_mapping 时 request.Model 仍是对外 id,poyo 不认识,
// 且提交阶段错误不加 SkipRetry 会跨 key/渠道重试。必须提前拦下。
func TestUnmappedModelRejected(t *testing.T) {
	for _, name := range ModelList {
		if !isUnmappedModel(name) {
			t.Errorf("isUnmappedModel(%q) = false, want true", name)
		}
		_, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{}, dto.ImageRequest{
			Model: name, Prompt: "x",
		})
		if err == nil {
			t.Errorf("unmapped model %q should be rejected", name)
		}
	}
	// 映射后的上游名必须放行
	for _, name := range []string{"seedream-4.5", "seedream-5.0-lite", "seedream-4.5-edit"} {
		if isUnmappedModel(name) {
			t.Errorf("isUnmappedModel(%q) = true, want false", name)
		}
	}
}
