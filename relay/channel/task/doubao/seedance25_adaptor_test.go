package doubao

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func seedance25RelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: ModelSeedance25},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: ModelSeedance25,
	}
}

func unifiedContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func nativeV3Context(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost,
		"/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("doubao_raw_format", true)
	return c
}

// TestSeedance25_UnifiedEndpointAllowsEmptyPrompt 2.5 的提示词是可选的：纯首
// 尾帧、甚至只传一段参考音频都是官方支持的组合。统一端点默认那条「prompt
// 必填」对它不成立。
func TestSeedance25_UnifiedEndpointAllowsEmptyPrompt(t *testing.T) {
	c := unifiedContext(t, `{
		"model": "doubao-seedance-2-5",
		"metadata": {
			"content": [
				{"type": "audio_url", "audio_url": {"url": "https://e.com/a.wav"}, "role": "reference_audio"}
			]
		}
	}`)
	info := seedance25RelayInfo()
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("无提示词请求被通用校验拦下了: %+v", taskErr)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("无提示词请求被 2.5 校验拦下了: %+v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Errorf("Action = %q, want %q", info.Action, constant.TaskActionGenerate)
	}
}

// TestSeedance25_RejectsEmptyRequest 四类内容全空的请求没有任何意义，必须在
// 网关这层拒掉。
func TestSeedance25_RejectsEmptyRequest(t *testing.T) {
	c := unifiedContext(t, `{"model": "doubao-seedance-2-5"}`)
	info := seedance25RelayInfo()
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("这一层不该拦: %+v", taskErr)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr == nil {
		t.Fatal("空请求应当被拒绝")
	}
}

// TestSeedance25_EstimateBilling 端到端串一遍：校验 → 计费 → 上游请求体。
func TestSeedance25_EstimateBilling(t *testing.T) {
	c := unifiedContext(t, `{
		"model": "doubao-seedance-2-5",
		"prompt": "小猫对着镜头打哈欠",
		"duration": 10,
		"metadata": {"resolution": "720p", "aspect_ratio": "16:9"}
	}`)
	info := seedance25RelayInfo()
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("校验失败: %+v", taskErr)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("2.5 校验失败: %+v", taskErr)
	}

	ratios := adaptor.EstimateBilling(c, info)
	// 720p 不含参考视频：10 秒 × $0.22399
	want := 10 * 0.22541
	if got := ratios[billing_setting.VideoCostRatioKey]; got < want-1e-6 || got > want+1e-6 {
		t.Errorf("video_cost = %v, want %v", got, want)
	}
	// 2.0 的相对倍率绝不能同时出现，两套计价叠加会算出完全错误的金额
	if _, ok := ratios["seedance_pricing"]; ok {
		t.Error("2.5 不该命中 2.0 的档位倍率")
	}

	snapshot, exists := c.Get(constant.CtxKeyVideoUsageSnapshot)
	if !exists {
		t.Fatal("缺少计费口径快照，终态结算会认不出档位")
	}
	snap, ok := snapshot.(*model.VideoUsageSnapshot)
	if !ok || snap.OutputSeconds != 10 || snap.Resolution != "720p" {
		t.Errorf("快照内容错误: %+v", snapshot)
	}
	if _, exists := c.Get(constant.CtxKeyVideoBillingDetail); !exists {
		t.Error("缺少计费明细，日志里只会显示哨兵价 $1")
	}

	body, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("构造上游请求体失败: %v", err)
	}
	raw := make([]byte, 4096)
	n, _ := body.Read(raw)
	var upstream ArkVideoRequest
	if err := common.Unmarshal(raw[:n], &upstream); err != nil {
		t.Fatalf("上游请求体不是合法 JSON: %v", err)
	}
	// 没配 model_mapping 时归一到官方 endpoint 名，否则官方不认
	if upstream.Model != ModelSeedance25Official {
		t.Errorf("上游模型名 = %q, want %q", upstream.Model, ModelSeedance25Official)
	}
	if upstream.Duration == nil || *upstream.Duration != 10 {
		t.Errorf("上游时长 = %+v, want 10", upstream.Duration)
	}
}

// TestSeedance25_NativeEndpointBilling 原生协议入口同样要产出计费口径——
// 这条路径以前是裸透传，拿不到可靠的计费依据。
func TestSeedance25_NativeEndpointBilling(t *testing.T) {
	c := nativeV3Context(t, `{
		"model": "doubao-seedance-2-5-260628",
		"content": [
			{"type": "text", "text": "a cat"},
			{"type": "video_url", "video_url": {"url": "https://e.com/v.mp4"}, "role": "reference_video"}
		],
		"resolution": "720p"
	}`)
	info := seedance25RelayInfo()
	info.UpstreamModelName = ModelSeedance25Official
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("原生协议请求被拦: %+v", taskErr)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("2.5 校验失败: %+v", taskErr)
	}

	ratios := adaptor.EstimateBilling(c, info)
	// duration 未指定 → 按价目表的 15 秒预扣；带参考视频 → 按 30 秒上限
	// 且整单走降档单价：(15 + 30) × $0.13457
	want := 45 * 0.13542
	if got := ratios[billing_setting.VideoCostRatioKey]; got < want-1e-6 || got > want+1e-6 {
		t.Errorf("video_cost = %v, want %v", got, want)
	}
}

// TestSeedance25_NativeEndpointRejectsUnsupported 原生协议入口也要挡住 2.5
// 不支持的参数，而不是透传给上游。
func TestSeedance25_NativeEndpointRejectsUnsupported(t *testing.T) {
	c := nativeV3Context(t, `{
		"model": "doubao-seedance-2-5-260628",
		"content": [{"type": "text", "text": "a cat"}],
		"seed": 42
	}`)
	info := seedance25RelayInfo()
	info.UpstreamModelName = ModelSeedance25Official
	adaptor := &TaskAdaptor{}

	_ = adaptor.ValidateRequestAndSetAction(c, info)
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr == nil {
		t.Fatal("2.5 不支持 seed，应当报错")
	}
}

// TestSeedance25_AdjustBillingOnComplete 用上游返回的 token 用量重算额度。
//
// 认 token 而不是 duration：duration 只是输出时长，既不含参考视频的贡献，也
// 不含官方对「含视频输入」任务的最低 token 用量下限。
func TestSeedance25_AdjustBillingOnComplete(t *testing.T) {
	newTask := func(modelName, data string, snap *model.VideoUsageSnapshot) *model.Task {
		task := &model.Task{Data: []byte(data)}
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			OriginModelName: modelName,
			GroupRatio:      1,
			VideoUsage:      snap,
		}
		return task
	}
	// quota = video_cost × QuotaPerUnit × groupRatio。容差 1 个额度单位，
	// 浮点截断不该让断言变脆。
	assertQuota := func(t *testing.T, got int, wantUSD float64) {
		t.Helper()
		want := int(wantUSD * common.QuotaPerUnit)
		if diff := got - want; diff > 1 || diff < -1 {
			t.Errorf("quota = %d, want ≈ %d（$%.5f）", got, want, wantUSD)
		}
	}

	adaptor := &TaskAdaptor{}

	t.Run("纯文生按真实时长退差额", func(t *testing.T) {
		// 预扣按 15 秒（duration=-1），实际出片 5 秒
		// 1280×720 × 120 帧 / 1024 = 108,000 tokens
		task := newTask(ModelSeedance25, `{
			"id":"cgt-1","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":108000,"total_tokens":108000}
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15})

		assertQuota(t, adaptor.AdjustBillingOnComplete(task, nil), 5*0.22541)
	})

	t.Run("含参考视频走降档单价", func(t *testing.T) {
		// 输出 5 秒 + 参考视频 30 秒 = 840 帧 → 756,000 tokens
		task := newTask(ModelSeedance25, `{
			"id":"cgt-2","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":756000,"total_tokens":756000}
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15, VideoSeconds: 30})

		assertQuota(t, adaptor.AdjustBillingOnComplete(task, nil), 35*0.13542)
	})

	t.Run("参考视频实际很短时退回大部分预扣", func(t *testing.T) {
		// 输出 5 秒 + 参考视频 4 秒 = 216 帧 → 194,400 tokens
		// 这也正是官方「含视频输入」的最低 token 用量档
		task := newTask(ModelSeedance25, `{
			"id":"cgt-3","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":194400,"total_tokens":194400}
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15, VideoSeconds: 30})

		assertQuota(t, adaptor.AdjustBillingOnComplete(task, nil), 9*0.13542)
	})

	t.Run("480p按自己的档位算", func(t *testing.T) {
		// 854×480 × 240 帧 / 1024 = 96,075 tokens
		task := newTask(ModelSeedance25, `{
			"id":"cgt-4","status":"succeeded","resolution":"480p","ratio":"16:9",
			"duration":10,"framespersecond":24,
			"usage":{"completion_tokens":96075,"total_tokens":96075}
		}`, &model.VideoUsageSnapshot{Resolution: "480p", OutputSeconds: 15})

		assertQuota(t, adaptor.AdjustBillingOnComplete(task, nil), 10*0.10416)
	})

	t.Run("优先用completion_tokens", func(t *testing.T) {
		task := newTask(ModelSeedance25, `{
			"id":"cgt-5","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":108000,"total_tokens":999999}
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15})

		assertQuota(t, adaptor.AdjustBillingOnComplete(task, nil), 5*0.22541)
	})

	t.Run("拿不到token时保持预扣", func(t *testing.T) {
		task := newTask(ModelSeedance25, `{
			"id":"cgt-6","status":"succeeded","resolution":"720p","duration":5
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15})

		if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
			t.Errorf("quota = %d, want 0（保持预扣）", got)
		}
	})

	t.Run("换算结果少于输出时长时保持预扣", func(t *testing.T) {
		// 21,600 tokens 只有 1 秒，却报了 5 秒的输出——token 字段选错了或
		// 上游改了公式，这时候宁可不动，也不能拿算错的数去扣钱
		task := newTask(ModelSeedance25, `{
			"id":"cgt-7","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":21600,"total_tokens":21600}
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15})

		if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
			t.Errorf("quota = %d, want 0（保持预扣）", got)
		}
	})

	t.Run("认不出分辨率时保持预扣", func(t *testing.T) {
		task := newTask(ModelSeedance25, `{
			"id":"cgt-8","status":"succeeded","resolution":"1080p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":108000,"total_tokens":108000}
		}`, nil)

		if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
			t.Errorf("quota = %d, want 0（保持预扣）", got)
		}
	})

	t.Run("非视频计价模型不走这条路", func(t *testing.T) {
		// 2.0 按 token 倍率计费，交给通用的 token 重算
		task := newTask("doubao-seedance-2-0-260128", `{
			"id":"cgt-9","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":108000,"total_tokens":108000}
		}`, nil)

		if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
			t.Errorf("quota = %d, want 0（交给 token 重算）", got)
		}
	})

	t.Run("分组倍率照常作用", func(t *testing.T) {
		task := newTask(ModelSeedance25, `{
			"id":"cgt-10","status":"succeeded","resolution":"720p","ratio":"16:9",
			"duration":5,"framespersecond":24,
			"usage":{"completion_tokens":108000,"total_tokens":108000}
		}`, &model.VideoUsageSnapshot{Resolution: "720p", OutputSeconds: 15})
		task.PrivateData.BillingContext.GroupRatio = 0.5

		assertQuota(t, adaptor.AdjustBillingOnComplete(task, nil), 5*0.22541*0.5)
	})
}

// TestSeedance25_ParseTaskResultExpired 官方的 expired 终态漏掉的话，任务会被
// 当成「进行中」永远轮询下去，预扣的额度也就永远不释放。
func TestSeedance25_ParseTaskResultExpired(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"cgt-x","status":"expired","model":"doubao-seedance-2-5-260628"
	}`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Status != model.TaskStatusFailure {
		t.Errorf("Status = %v, want %v", result.Status, model.TaskStatusFailure)
	}
	if result.Progress != "100%" {
		t.Errorf("Progress = %q, want 100%%", result.Progress)
	}
	if result.Reason == "" {
		t.Error("失败原因不能为空，否则用户看不出任务为什么没了")
	}
}

// TestSeedance25_AdjustBillingWithoutSnapshot 没有提交时的口径快照就判断不出
// 走哪一档单价。默认按「不含视频」结算会给带参考视频的任务按 1.66 倍收钱，
// 必须保持预扣而不是朝多收的方向猜。
func TestSeedance25_AdjustBillingWithoutSnapshot(t *testing.T) {
	task := &model.Task{Data: []byte(`{
		"id":"cgt-nosnap","status":"succeeded","resolution":"720p","ratio":"16:9",
		"duration":5,"framespersecond":24,
		"usage":{"completion_tokens":756000,"total_tokens":756000}
	}`)}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: ModelSeedance25,
		GroupRatio:      1,
	}

	if got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil); got != 0 {
		t.Errorf("quota = %d, want 0（保持预扣）", got)
	}
}

// TestSeedance20_UnaffectedBy25 2.0 / 2.0-fast 线上在跑，2.5 的接入不能碰它们。
//
// 两者共用同一个 adaptor，2.5 的逻辑全部挂在 IsSeedance25Model 分支后面；这条
// 测试把每个分流点都走一遍，确认 2.0 走的还是原来那条路。
func TestSeedance20_UnaffectedBy25(t *testing.T) {
	names := []string{
		"doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-fast-260128",
		"seedance-2.0",
		"seedance-2.0-api",
		"seedance-2.0-fast",
		"doubao-seedance-1-5-pro-251215",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if IsSeedance25Model(name) {
				t.Fatal("被误判成 2.5，会走进全新的解析与计费路径")
			}
			if _, ok := billing_setting.GetVideoPricing(name); ok {
				t.Error("不该有视频价目表——有了就会走 video_cost 而不是 ModelRatio")
			}
			if constant.GetDefaultModelParamSchema(name) != "" {
				t.Error("不该被塞进 2.5 的内置 schema")
			}
		})
	}

	// 2.0 的档位倍率表必须原样可用，四个档位一个不能少
	for _, tc := range []struct {
		model      string
		resolution string
		hasVideo   bool
	}{
		{"doubao-seedance-2-0-260128", "720p", false},
		{"doubao-seedance-2-0-260128", "720p", true},
		{"doubao-seedance-2-0-260128", "1080p", false},
		{"doubao-seedance-2-0-260128", "1080p", true},
		{"doubao-seedance-2-0-fast-260128", "720p", false},
		{"doubao-seedance-2-0-fast-260128", "720p", true},
		{"seedance-2.0", "720p", true},
	} {
		if _, ok := GetSeedancePricingRatio(tc.model, tc.resolution, tc.hasVideo); !ok {
			t.Errorf("2.0 档位倍率表命不中: %s / %s / hasVideo=%v",
				tc.model, tc.resolution, tc.hasVideo)
		}
	}

	adaptor := &TaskAdaptor{}

	// 新增的 ValidateMappedRequestAndSetAction 对 2.0 必须是空操作：这个方法
	// 以前不存在（adaptor 没实现 MappedTaskValidator），现在每个 doubao 任务
	// 都会调到它。
	c := nativeV3Context(t, `{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"a cat"}],
		"seed":42,"camera_fixed":true,"resolution":"1080p"
	}`)
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seedance-2-0-260128"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: "doubao-seedance-2-0-260128",
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Errorf("2.0 被 2.5 的校验拦下了（seed/1080p 对 2.0 是合法的）: %+v", taskErr)
	}
	// 计费仍走相对倍率，不能冒出 video_cost
	ratios := adaptor.EstimateBilling(c, info)
	if _, ok := ratios[billing_setting.VideoCostRatioKey]; ok {
		t.Error("2.0 不该产出 video_cost")
	}

	// 新增的 AdjustBillingOnComplete 对 2.0 必须返回 0，把结算让回给通用的
	// 按 token 重算——那是 2.0 一直在用的路径。
	task := &model.Task{Data: []byte(`{"id":"x","status":"succeeded","resolution":"720p",
		"usage":{"completion_tokens":108000,"total_tokens":108000}}`)}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "doubao-seedance-2-0-260128",
		GroupRatio:      1,
	}
	if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
		t.Errorf("quota = %d, want 0（交给按 token 重算）", got)
	}

	// ZeroCut 聚合器的响应解析原样可用
	result, err := adaptor.ParseTaskResult([]byte(`{
		"code":200,"data":{"id":123,"status":"SUCCESS",
		"output":{"video_url":"https://cdn.example.com/o.mp4",
		"usage":{"total_tokens":1000,"completion_tokens":900}}}}`))
	if err != nil {
		t.Fatalf("ZeroCut 响应解析失败: %v", err)
	}
	if result.Url != "https://cdn.example.com/o.mp4" || result.TotalTokens != 1000 {
		t.Errorf("ZeroCut 解析结果不对: %+v", result)
	}
}

// TestSeedance25_ParseTaskResultCancelled 上游主动取消的任务是终态，必须落
// FAILURE 并给失败原因，否则掉进 default 被当成「进行中」，任务永远到不了终态、
// 预扣额度不释放。
func TestSeedance25_ParseTaskResultCancelled(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"cgt-x","status":"cancelled","model":"doubao-seedance-2-5-260628",
		"error":{"code":"cancelled","message":"user cancelled"}
	}`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Status != model.TaskStatusFailure {
		t.Errorf("Status = %v, want %v", result.Status, model.TaskStatusFailure)
	}
	if result.Progress != "100%" {
		t.Errorf("Progress = %q, want 100%%", result.Progress)
	}
	if result.Reason != "user cancelled" {
		t.Errorf("Reason = %q, want 上游错误信息", result.Reason)
	}
}

// TestSeedance_SetupWebhookGating Seedance webhook 模式的门控条件：全局开关 +
// 客户端 callback_url + 公网 ServerAddress + 公开 task_id 缺一不可。缺任何一条
// 都应回退轮询，否则上游回调不到网关、任务会卡死。
func TestSeedance_SetupWebhookGating(t *testing.T) {
	prevEnabled := system_setting.SeedanceWebhookEnabled
	prevAddr := system_setting.ServerAddress
	prevSecret := system_setting.SeedanceWebhookSecret
	t.Cleanup(func() {
		system_setting.SeedanceWebhookEnabled = prevEnabled
		system_setting.ServerAddress = prevAddr
		system_setting.SeedanceWebhookSecret = prevSecret
	})

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_abc123"},
	}

	// 1. 全局开关关闭 → 走轮询
	system_setting.SeedanceWebhookEnabled = false
	system_setting.SeedanceWebhookSecret = "test-secret"
	system_setting.ServerAddress = "https://amux.example.com"
	if _, ok := SetupSeedanceWebhook(info, "https://client.example.com/cb", APITypeZeroCut); ok {
		t.Error("全局开关关闭时不应进入 webhook 模式")
	}

	// 2. 开关开启但客户端没带 callback_url → 走轮询
	system_setting.SeedanceWebhookEnabled = true
	if _, ok := SetupSeedanceWebhook(info, "", APITypeZeroCut); ok {
		t.Error("客户端未带 callback_url 时不应进入 webhook 模式")
	}

	// 3. 签名密钥为空 → 走轮询（回调地址带不上可校验的签名，伪造风险未消除）
	system_setting.SeedanceWebhookSecret = ""
	if _, ok := SetupSeedanceWebhook(info, "https://client.example.com/cb", APITypeZeroCut); ok {
		t.Error("SeedanceWebhookSecret 为空时不应进入 webhook 模式")
	}
	system_setting.SeedanceWebhookSecret = "test-secret"

	// 4. ServerAddress 是私网地址 → 走轮询（上游回调不到）
	system_setting.ServerAddress = "http://localhost:3000"
	if _, ok := SetupSeedanceWebhook(info, "https://client.example.com/cb", APITypeZeroCut); ok {
		t.Error("ServerAddress 为 localhost 时不应进入 webhook 模式")
	}

	// 5. 全部满足 → webhook 模式，回调地址拼上公开 task_id + HMAC 签名
	system_setting.ServerAddress = "https://amux.example.com"
	setup, ok := SetupSeedanceWebhook(info, "https://client.example.com/cb", APITypeZeroCut)
	if !ok {
		t.Fatal("满足全部条件时应进入 webhook 模式")
	}
	want := "https://amux.example.com/api/v1/webhook/seedance/task_abc123?sig=" + common.GenerateHMACWithKey([]byte("test-secret"), "task_abc123")
	if setup.CallbackURL != want {
		t.Errorf("CallbackURL = %q, want %q", setup.CallbackURL, want)
	}
	if setup.TraceID != "task_abc123" {
		t.Errorf("TraceID = %q, want task_abc123", setup.TraceID)
	}

	// 6. 缺公开 task_id → 走轮询
	noTask := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	if _, ok := SetupSeedanceWebhook(noTask, "https://client.example.com/cb", APITypeZeroCut); ok {
		t.Error("缺少 PublicTaskID 时不应进入 webhook 模式")
	}
}

// TestSeedance_SetupWebhookTraceIDByAPIType trace_id 是 ZeroCut 专有字段：
// ZeroCut 必须同时拿到 callback_url + trace_id 才会触发回调；官方 Ark 不认识
// 这个字段，多传会被判成 InvalidParameter 直接把提交打挂。所以填哪些字段只能
// 按 adaptor 探测到的上游类型决定，不能按走了哪条模型分支决定。
func TestSeedance_SetupWebhookTraceIDByAPIType(t *testing.T) {
	prevEnabled := system_setting.SeedanceWebhookEnabled
	prevAddr := system_setting.ServerAddress
	prevSecret := system_setting.SeedanceWebhookSecret
	t.Cleanup(func() {
		system_setting.SeedanceWebhookEnabled = prevEnabled
		system_setting.ServerAddress = prevAddr
		system_setting.SeedanceWebhookSecret = prevSecret
	})
	system_setting.SeedanceWebhookEnabled = true
	system_setting.SeedanceWebhookSecret = "test-secret"
	system_setting.ServerAddress = "https://amux.example.com"

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_abc123"},
	}

	zc, ok := SetupSeedanceWebhook(info, "https://client.example.com/cb", APITypeZeroCut)
	if !ok {
		t.Fatal("ZeroCut 应进入 webhook 模式")
	}
	if zc.TraceID != "task_abc123" {
		t.Errorf("ZeroCut TraceID = %q, want task_abc123", zc.TraceID)
	}

	ark, ok := SetupSeedanceWebhook(info, "https://client.example.com/cb", APITypeDoubaoOfficial)
	if !ok {
		t.Fatal("官方 Ark 也应进入 webhook 模式（URL 自带 task_id 即可寻址）")
	}
	if ark.TraceID != "" {
		t.Errorf("官方 Ark TraceID = %q, want 空（多传会被 Ark 判成 InvalidParameter）", ark.TraceID)
	}
	if ark.CallbackURL != zc.CallbackURL {
		t.Errorf("两种上游的回调地址应一致: ark=%q zerocut=%q", ark.CallbackURL, zc.CallbackURL)
	}
}

// TestSeedance25_ArkRequestOmitsTraceIDForOfficial 官方 Ark 请求体里不能出现
// trace_id 字段（哪怕是空串）——omitempty 必须真的把它丢掉。
func TestSeedance25_ArkRequestOmitsTraceIDForOfficial(t *testing.T) {
	official := (&Seedance25Request{
		Prompt:      "test",
		Duration:    Seedance25AutoDuration,
		CallbackURL: "https://amux.example.com/api/v1/webhook/seedance/task_abc123",
	}).ToArkRequest("doubao-seedance-2-5-260628")
	data, err := common.Marshal(official)
	if err != nil {
		t.Fatalf("marshal 失败: %v", err)
	}
	if strings.Contains(string(data), "trace_id") {
		t.Errorf("官方 Ark 请求体不应包含 trace_id: %s", string(data))
	}

	zerocut := (&Seedance25Request{
		Prompt:      "test",
		Duration:    Seedance25AutoDuration,
		CallbackURL: "https://amux.example.com/api/v1/webhook/seedance/task_abc123",
		TraceID:     "task_abc123",
	}).ToArkRequest("doubao-seedance-2-5-260628")
	if zerocut.TraceID != "task_abc123" {
		t.Errorf("ZeroCut TraceID = %q, want task_abc123", zerocut.TraceID)
	}
}

// TestSeedance25_ToArkRequestCarriesCallbackURL webhook 模式下把回调地址设给
// Seedance25Request 后，构造的上游请求体必须原样带上 callback_url。
func TestSeedance25_ToArkRequestCarriesCallbackURL(t *testing.T) {
	r := &Seedance25Request{
		Prompt:      "test",
		Duration:    Seedance25AutoDuration,
		CallbackURL: "https://amux.example.com/api/v1/webhook/seedance/task_abc123",
	}
	arkReq := r.ToArkRequest("doubao-seedance-2-5-260628")
	if arkReq.CallbackURL != r.CallbackURL {
		t.Errorf("CallbackURL = %q, want %q", arkReq.CallbackURL, r.CallbackURL)
	}
}
