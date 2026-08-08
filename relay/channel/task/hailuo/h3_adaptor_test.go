package hailuo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
)

// FetchTask 走的是共享 http client，进程里没跑过主程序初始化时它是 nil。
func TestMain(m *testing.M) {
	service.InitHttpClient()
	os.Exit(m.Run())
}

func h3RelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: ModelMiniMaxH3},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: ModelMiniMaxH3,
	}
}

func nativeV2Context(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(constant.CtxKeyMinimaxV2Format, true)
	return c
}

// TestValidateRequestAndSetAction_NativeSkipsGenericPromptCheck v2 原生协议的
// prompt 在 content[] 里，通用校验会误判缺 prompt——必须绕开。
func TestValidateRequestAndSetAction_NativeSkipsGenericPromptCheck(t *testing.T) {
	c := nativeV2Context(t, `{
		"model": "MiniMax-H3",
		"content": [{"type": "text", "text": "a boy playing basketball"}],
		"resolution": "2K",
		"duration": 10
	}`)
	info := h3RelayInfo()

	adaptor := &TaskAdaptor{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("native v2 request rejected by generic validation: %+v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Errorf("Action = %q, want %q", info.Action, constant.TaskActionGenerate)
	}
}

// TestNativeV2_BuildsUpstreamBodyAndBilling 端到端串一遍原生协议：
// 校验 → 计费 → 上游请求体。
func TestNativeV2_BuildsUpstreamBodyAndBilling(t *testing.T) {
	c := nativeV2Context(t, `{
		"model": "MiniMax-H3",
		"content": [
			{"type": "text", "text": "a boy playing basketball"},
			{"type": "image_url", "image_url": {"url": "https://example.com/first.jpg"}, "role": "first_frame"}
		],
		"resolution": "2K",
		"duration": 10,
		"ratio": "16:9",
		"aigc_watermark": false
	}`)
	info := h3RelayInfo()
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction failed: %+v", taskErr)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateMappedRequestAndSetAction failed: %+v", taskErr)
	}

	// 10 秒 2K + 1 张首帧 = 10×0.13 + 1×0.04 = 1.34
	ratios := adaptor.EstimateBilling(c, info)
	cost, ok := ratios["video_cost"]
	if !ok {
		t.Fatalf("EstimateBilling did not return video_cost, got %+v", ratios)
	}
	if diff := cost - 1.34; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("video_cost = %.6f, want 1.34", cost)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody failed: %v", err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}

	var sent V2VideoRequest
	if err := common.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("unmarshal upstream body failed: %v", err)
	}
	if sent.Model != ModelMiniMaxH3 || sent.Resolution != "2K" || sent.Duration != 10 || sent.Ratio != "16:9" {
		t.Errorf("upstream body mismatch: %+v", sent)
	}
	// 显式 false 必须原样送到上游（CLAUDE.md Rule 6）
	if sent.AigcWatermark == nil || *sent.AigcWatermark {
		t.Errorf("aigc_watermark should be an explicit false, got %v", sent.AigcWatermark)
	}
	if len(sent.Content) != 2 || sent.Content[1].Role != "first_frame" {
		t.Errorf("content not rebuilt as expected: %+v", sent.Content)
	}
}

// TestValidateMappedRequest_RejectsUnpricedRequest 算不出价的请求必须在预扣费
// 之前被拒，不能放行一个不知道该收多少钱的任务。
func TestValidateMappedRequest_RejectsUnpricedRequest(t *testing.T) {
	c := nativeV2Context(t, `{
		"model": "MiniMax-H3",
		"content": [{"type": "text", "text": "p"}],
		"resolution": "1080P",
		"duration": 10
	}`)
	info := h3RelayInfo()

	adaptor := &TaskAdaptor{}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr == nil {
		t.Fatal("expected an error for an unsupported resolution, got nil")
	}
}

// TestEstimateBilling_SkipsNonH3Models 老模型不该被视频定价影响。
func TestEstimateBilling_SkipsNonH3Models(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "MiniMax-Hailuo-2.3"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: "MiniMax-Hailuo-2.3",
	}

	adaptor := &TaskAdaptor{}
	if ratios := adaptor.EstimateBilling(c, info); ratios != nil {
		t.Errorf("EstimateBilling should be a no-op for v1 models, got %+v", ratios)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Errorf("ValidateMappedRequestAndSetAction should be a no-op for v1 models: %+v", taskErr)
	}
}

func TestBuildRequestURL_SplitsByModel(t *testing.T) {
	adaptor := &TaskAdaptor{baseURL: "https://api.minimaxi.com"}

	h3URL, err := adaptor.BuildRequestURL(h3RelayInfo())
	if err != nil {
		t.Fatalf("BuildRequestURL failed: %v", err)
	}
	if h3URL != "https://api.minimaxi.com"+H3Endpoint {
		t.Errorf("H3 URL = %q", h3URL)
	}

	v1Info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "MiniMax-Hailuo-2.3"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	v1URL, err := adaptor.BuildRequestURL(v1Info)
	if err != nil {
		t.Fatalf("BuildRequestURL failed: %v", err)
	}
	if v1URL != "https://api.minimaxi.com"+TextToVideoEndpoint {
		t.Errorf("v1 URL = %q", v1URL)
	}
}

// TestFetchTask_SplitsQueryEndpointByModel 轮询时只能靠 body["model"] 区分
// v1/v2 查询端点，见 service/task_polling.go 传参处。
func TestFetchTask_SplitsQueryEndpointByModel(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"task_id":"t1","status":"Success","base_resp":{"status_code":0}}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}

	resp, err := adaptor.FetchTask(server.URL, "k", map[string]any{
		"task_id": "t1",
		"model":   ModelMiniMaxH3,
	}, "")
	if err != nil {
		t.Fatalf("FetchTask failed: %v", err)
	}
	_ = resp.Body.Close()
	// v2 用路径参数，v1 用 ?task_id= 查询参数——两者不能混
	if want := H3QueryEndpoint + "/t1"; gotPath != want {
		t.Errorf("H3 query path = %q, want %q", gotPath, want)
	}
	if gotQuery != "" {
		t.Errorf("v2 query should carry no query string, got %q", gotQuery)
	}

	resp, err = adaptor.FetchTask(server.URL, "k", map[string]any{
		"task_id": "t1",
		"model":   "MiniMax-Hailuo-2.3",
	}, "")
	if err != nil {
		t.Fatalf("FetchTask failed: %v", err)
	}
	_ = resp.Body.Close()
	if gotPath != QueryTaskEndpoint {
		t.Errorf("v1 query path = %q, want %q", gotPath, QueryTaskEndpoint)
	}

	// 老任务快照里没有 model 字段时必须回落到 v1，不能 panic 或走错端点
	resp, err = adaptor.FetchTask(server.URL, "k", map[string]any{"task_id": "t1"}, "")
	if err != nil {
		t.Fatalf("FetchTask failed: %v", err)
	}
	_ = resp.Body.Close()
	if gotPath != QueryTaskEndpoint {
		t.Errorf("legacy task without model should fall back to v1, got %q", gotPath)
	}
}

// TestConvertToMinimaxV2 查询响应只暴露网关公开 ID 与脱敏后的结果地址，
// 上游任务 ID 不能外泄。
func TestConvertToMinimaxV2(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_id",
		Status: model.TaskStatusSuccess,
		Data: []byte(`{
			"task": {
				"id": "upstream-secret-id",
				"status": "succeeded",
				"resolution": "2K",
				"duration": 10,
				"ratio": "16:9",
				"content": {"url": "https://upstream.example.com/raw.mp4"}
			}
		}`),
	}
	task.Properties.OriginModelName = ModelMiniMaxH3
	task.PrivateData.ResultURL = "https://cdn.example.com/archived.mp4"

	adaptor := &TaskAdaptor{}
	raw, err := adaptor.ConvertToMinimaxV2(task)
	if err != nil {
		t.Fatalf("ConvertToMinimaxV2 failed: %v", err)
	}

	body := string(raw)
	if strings.Contains(body, "upstream-secret-id") {
		t.Error("upstream task id leaked into the response")
	}
	if strings.Contains(body, "upstream.example.com") {
		t.Error("upstream result url leaked into the response")
	}

	var got V2QueryResponse
	if err := common.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Task == nil {
		t.Fatal("response has no task object")
	}
	if got.Task.ID != "task_public_id" {
		t.Errorf("task.id = %q, want the gateway public id", got.Task.ID)
	}
	if got.Task.Status != H3StatusSucceeded {
		t.Errorf("task.status = %q", got.Task.Status)
	}
	if got.Task.Content == nil || got.Task.Content.URL != "https://cdn.example.com/archived.mp4" {
		t.Errorf("task.content = %+v, want the archived result url", got.Task.Content)
	}
	// 请求参数从存量数据里带出来，方便用户核对
	if got.Task.Resolution != "2K" || got.Task.Duration != 10 || got.Task.Ratio != "16:9" {
		t.Errorf("request params dropped: %+v", got.Task)
	}
}

func TestConvertToMinimaxV2_StatusMapping(t *testing.T) {
	cases := map[model.TaskStatus]string{
		model.TaskStatusNotStart:   H3StatusQueued,
		model.TaskStatusSubmitted:  H3StatusQueued,
		model.TaskStatusQueued:     H3StatusQueued,
		model.TaskStatusInProgress: H3StatusRunning,
		model.TaskStatusSuccess:    H3StatusSucceeded,
		model.TaskStatusFailure:    H3StatusFailed,
	}

	adaptor := &TaskAdaptor{}
	for status, want := range cases {
		raw, err := adaptor.ConvertToMinimaxV2(&model.Task{TaskID: "t", Status: status})
		if err != nil {
			t.Fatalf("ConvertToMinimaxV2 failed: %v", err)
		}
		var got V2QueryResponse
		if err := common.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if got.Task == nil || got.Task.Status != want {
			t.Errorf("status %v mapped to %+v, want %q", status, got.Task, want)
		}
	}

	// 失败任务用网关记录的失败原因兜底
	raw, err := adaptor.ConvertToMinimaxV2(&model.Task{
		TaskID: "t", Status: model.TaskStatusFailure, FailReason: "content moderation",
	})
	if err != nil {
		t.Fatalf("ConvertToMinimaxV2 failed: %v", err)
	}
	var got V2QueryResponse
	if err := common.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Task.Error == nil || got.Task.Error.Message != "content moderation" {
		t.Errorf("task.error = %+v, want the gateway fail reason", got.Task.Error)
	}
}

// TestParseTaskResult_V2 v2 的状态枚举与响应结构和 v1 完全不同，
// ParseTaskResult 拿不到模型名，只能按结构判别，两种都要能解析。
func TestParseTaskResult_V2(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"task": {
			"id": "t1",
			"status": "succeeded",
			"content": {"url": "https://cdn.example.com/out.mp4"}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseTaskResult failed: %v", err)
	}
	if result.Status != model.TaskStatusSuccess {
		t.Errorf("Status = %v", result.Status)
	}
	// v2 成功直接给 content.url，不该再去调 file 检索
	if result.Url != "https://cdn.example.com/out.mp4" {
		t.Errorf("Url = %q", result.Url)
	}

	states := map[string]string{
		"queued":    model.TaskStatusQueued,
		"running":   model.TaskStatusInProgress,
		"failed":    model.TaskStatusFailure,
		"cancelled": model.TaskStatusFailure,
	}
	for state, want := range states {
		r, err := adaptor.ParseTaskResult([]byte(`{"task":{"id":"t1","status":"` + state + `"}}`))
		if err != nil {
			t.Fatalf("ParseTaskResult(%s) failed: %v", state, err)
		}
		if r.Status != want {
			t.Errorf("state %q → %v, want %v", state, r.Status, want)
		}
	}

	// error 对象优先作为失败原因
	r, err := adaptor.ParseTaskResult([]byte(
		`{"task":{"id":"t1","status":"failed","error":{"code":"moderation","message":"blocked"}}}`))
	if err != nil {
		t.Fatalf("ParseTaskResult failed: %v", err)
	}
	if r.Status != model.TaskStatusFailure || r.Reason != "blocked" {
		t.Errorf("failure not surfaced: %+v", r)
	}
}

// TestParseTaskResult_V1StillWorks v1 模型仍走扁平结构 + base_resp，
// 加了 v2 分支后不能把它们弄坏。
func TestParseTaskResult_V1StillWorks(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"task_id": "t1",
		"status": "Success",
		"video_url": "https://cdn.example.com/out.mp4",
		"file_id": "should-not-be-used",
		"base_resp": {"status_code": 0}
	}`))
	if err != nil {
		t.Fatalf("ParseTaskResult failed: %v", err)
	}
	if result.Url != "https://cdn.example.com/out.mp4" {
		t.Errorf("Url = %q, want the direct video_url", result.Url)
	}
	if result.Status != model.TaskStatusSuccess {
		t.Errorf("Status = %v", result.Status)
	}
}

// TestAdjustBillingOnComplete 用上游真实用量重算额度。
// 参考素材提交时只能按 15 秒上限预扣，不结算会显著多收——实测一单
// 2K 6 秒 + 7 秒参考视频，预扣 $2.73 而实收应为 $1.69。
func TestAdjustBillingOnComplete(t *testing.T) {
	newTask := func(data string, snap *model.VideoUsageSnapshot) *model.Task {
		task := &model.Task{Data: []byte(data)}
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			OriginModelName: ModelMiniMaxH3,
			GroupRatio:      1,
			VideoUsage:      snap,
		}
		return task
	}
	// quota = video_cost × QuotaPerUnit × groupRatio
	quotaOf := func(usd float64) int { return int(usd * common.QuotaPerUnit) }

	adaptor := &TaskAdaptor{}

	t.Run("参考视频按真实时长退差额", func(t *testing.T) {
		task := newTask(`{"task":{"id":"t","status":"succeeded","resolution":"2K",
			"usage":{"input_image_count":0,"input_seconds":7,"output_seconds":6,"total_seconds":13}}}`,
			&model.VideoUsageSnapshot{Resolution: "2K", OutputSeconds: 6, VideoSeconds: 15})

		// 6×0.13 + 7×0.13 = 1.69
		if got, want := adaptor.AdjustBillingOnComplete(task, nil), quotaOf(1.69); got != want {
			t.Errorf("quota = %d, want %d", got, want)
		}
	})

	t.Run("纯文生预扣本来就准", func(t *testing.T) {
		task := newTask(`{"task":{"id":"t","status":"succeeded","resolution":"768P",
			"usage":{"input_image_count":0,"input_seconds":0,"output_seconds":6,"total_seconds":6}}}`,
			&model.VideoUsageSnapshot{Resolution: "768P", OutputSeconds: 6})

		if got, want := adaptor.AdjustBillingOnComplete(task, nil), quotaOf(0.48); got != want {
			t.Errorf("quota = %d, want %d", got, want)
		}
	})

	t.Run("图片张数以上游为准", func(t *testing.T) {
		task := newTask(`{"task":{"id":"t","status":"succeeded","resolution":"2K",
			"usage":{"input_image_count":2,"input_seconds":0,"output_seconds":6}}}`,
			&model.VideoUsageSnapshot{Resolution: "2K", OutputSeconds: 6, ImageCount: 2})

		// 6×0.13 + 2×0.04 = 0.86
		if got, want := adaptor.AdjustBillingOnComplete(task, nil), quotaOf(0.86); got != want {
			t.Errorf("quota = %d, want %d", got, want)
		}
	})

	t.Run("带音频时同样按真值结算", func(t *testing.T) {
		task := newTask(`{"task":{"id":"t","status":"succeeded","resolution":"2K",
			"usage":{"input_image_count":0,"input_seconds":10,"output_seconds":6}}}`,
			&model.VideoUsageSnapshot{Resolution: "2K", OutputSeconds: 6, VideoSeconds: 15, AudioSeconds: 15})

		// 6×0.13 + 10×0.13 = 2.08（预扣 2.73，退 0.65）
		if got, want := adaptor.AdjustBillingOnComplete(task, nil), quotaOf(2.08); got != want {
			t.Errorf("quota = %d, want %d", got, want)
		}
	})

	// 结算是双向的：真实用量超过预扣口径时要补扣，不做单边封顶
	t.Run("真实用量超过预扣时补扣", func(t *testing.T) {
		task := newTask(`{"task":{"id":"t","status":"succeeded","resolution":"2K",
			"usage":{"input_image_count":3,"input_seconds":12,"output_seconds":6}}}`,
			&model.VideoUsageSnapshot{Resolution: "2K", OutputSeconds: 6, ImageCount: 1, VideoSeconds: 8})

		// 6×0.13 + 12×0.13 + 3×0.04 = 0.78 + 1.56 + 0.12 = 2.46
		// 预扣口径只有 8 秒视频 + 1 张图，结算高于预扣
		if got, want := adaptor.AdjustBillingOnComplete(task, nil), quotaOf(2.46); got != want {
			t.Errorf("quota = %d, want %d", got, want)
		}
	})

	t.Run("原请求无参考视频时不按视频单价收", func(t *testing.T) {
		// 纯参考音频场景：VideoSeconds 为 0，input_seconds 不应被算作视频
		task := newTask(`{"task":{"id":"t","status":"succeeded","resolution":"2K",
			"usage":{"input_image_count":1,"input_seconds":9,"output_seconds":6}}}`,
			&model.VideoUsageSnapshot{Resolution: "2K", OutputSeconds: 6, ImageCount: 1, AudioSeconds: 15})

		// 6×0.13 + 1×0.04 = 0.82，音频免费且不转成视频秒数
		if got, want := adaptor.AdjustBillingOnComplete(task, nil), quotaOf(0.82); got != want {
			t.Errorf("quota = %d, want %d", got, want)
		}
	})

	t.Run("拿不到用量时保持预扣", func(t *testing.T) {
		for _, data := range []string{
			`{"task":{"id":"t","status":"succeeded"}}`,
			`{}`,
			`not json`,
		} {
			task := newTask(data, &model.VideoUsageSnapshot{Resolution: "2K", OutputSeconds: 6})
			if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
				t.Errorf("data=%s → %d, want 0 (保持预扣)", data, got)
			}
		}
	})

	// 预扣与结算必须用同一套公式（先算基础额度、再乘金额，各自截断一次）。
	// 公式漂移会让「预扣本来就准」的单子产生 1 个单位的虚假差额，
	// 凭空多出一条退款/补扣日志。
	t.Run("预扣准确时结算额度完全一致", func(t *testing.T) {
		for _, gr := range []float64{1, 1.5, 2, 0.7} {
			usage := billing_setting.VideoUsage{Resolution: "768P", OutputSeconds: 6}
			cost, err := billing_setting.ComputeVideoCost(ModelMiniMaxH3, usage)
			if err != nil {
				t.Fatalf("ComputeVideoCost failed: %v", err)
			}
			// 复刻预扣链路：ModelPriceHelperPerCall → relay_task.go 的 ratio 连乘
			preCharge := int(float64(int(billing_setting.VideoBasePrice*common.QuotaPerUnit*gr)) * cost.Total)

			task := &model.Task{Data: []byte(`{"task":{"id":"t","status":"succeeded","resolution":"768P",
				"usage":{"input_image_count":0,"input_seconds":0,"output_seconds":6}}}`)}
			task.PrivateData.BillingContext = &model.TaskBillingContext{
				OriginModelName: ModelMiniMaxH3,
				GroupRatio:      gr,
				VideoUsage:      &model.VideoUsageSnapshot{Resolution: "768P", OutputSeconds: 6},
			}
			if got := adaptor.AdjustBillingOnComplete(task, nil); got != preCharge {
				t.Errorf("groupRatio=%v: 结算 %d != 预扣 %d，会产生虚假差额", gr, got, preCharge)
			}
		}
	})

	t.Run("非视频模型不参与", func(t *testing.T) {
		task := &model.Task{Data: []byte(`{"task":{"usage":{"output_seconds":6}}}`)}
		task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "MiniMax-Hailuo-2.3"}
		if got := adaptor.AdjustBillingOnComplete(task, nil); got != 0 {
			t.Errorf("v1 model → %d, want 0", got)
		}
	})
}
