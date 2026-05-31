package amux_stt

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
}

// setBody puts JSON bytes into the gin context so UnmarshalBodyReusable can read them.
func setBody(c *gin.Context, data []byte) {
	storage, _ := common.CreateBodyStorage(data)
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Header.Set("Content-Type", "application/json")
}

// ── ValidateRequestAndSetAction ──────────────────────────────────────

func TestValidate_AudioURL(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body, _ := common.Marshal(map[string]any{
		"audio_url": "https://example.com/audio.mp3",
		"model":     "amux-stt-v1",
	})
	setBody(c, body)

	a := &TaskAdaptor{}
	if err := a.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{}); err != nil {
		t.Fatalf("expected no error, got: %s", err.Message)
	}
	if _, exists := c.Get(contextKeyParsedRequest); !exists {
		t.Fatal("parsed request not stored in context")
	}
}

func TestValidate_Base64WithFilename(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body, _ := common.Marshal(map[string]any{
		"audio_base64":   "SGVsbG8=",
		"audio_filename": "test.mp3",
	})
	setBody(c, body)

	a := &TaskAdaptor{}
	if err := a.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{}); err != nil {
		t.Fatalf("expected no error, got: %s", err.Message)
	}
}

func TestValidate_MissingAudio(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body, _ := common.Marshal(map[string]any{"model": "amux-stt-v1"})
	setBody(c, body)

	a := &TaskAdaptor{}
	taskErr := a.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	if taskErr == nil {
		t.Fatal("expected error for missing audio, got nil")
	}
	if taskErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", taskErr.StatusCode)
	}
}

func TestValidate_Base64MissingFilename(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body, _ := common.Marshal(map[string]any{"audio_base64": "SGVsbG8="})
	setBody(c, body)

	a := &TaskAdaptor{}
	taskErr := a.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	if taskErr == nil {
		t.Fatal("expected error for missing filename, got nil")
	}
	if taskErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", taskErr.StatusCode)
	}
}

// ── EstimateBilling ──────────────────────────────────────────────────

func TestEstimateBilling(t *testing.T) {
	a := &TaskAdaptor{}
	ratios := a.EstimateBilling(nil, nil)
	if ratios["hours"] != 1 {
		t.Fatalf("expected 1 hour, got %v", ratios["hours"])
	}
}

// ── AdjustBillingOnComplete ──────────────────────────────────────────

func TestAdjustBilling_DirectDuration(t *testing.T) {
	a := &TaskAdaptor{}
	// 5 min 15 sec = 315s → ceil(315/60) = 6 min → 6/60 hour
	data, _ := common.Marshal(map[string]any{"audio_duration": 315.0})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.36, // $0.36/hour
				GroupRatio: 1.0,
			},
		},
	}
	quota := a.AdjustBillingOnComplete(task, nil)
	expected := int(0.36 * common.QuotaPerUnit * 1.0 * (6.0 / 60.0))
	if quota != expected {
		t.Fatalf("expected quota %d, got %d", expected, quota)
	}
}

func TestAdjustBilling_WrappedResult(t *testing.T) {
	a := &TaskAdaptor{}
	// 90 sec → ceil(90/60) = 2 min → 2/60 hour
	data, _ := common.Marshal(map[string]any{
		"result": map[string]any{"audio_duration": 90.0},
	})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.01,
				GroupRatio: 1.5,
			},
		},
	}
	quota := a.AdjustBillingOnComplete(task, nil)
	expected := int(0.01 * common.QuotaPerUnit * 1.5 * (2.0 / 60.0))
	if quota != expected {
		t.Fatalf("expected quota %d, got %d", expected, quota)
	}
}

func TestAdjustBilling_ShortAudio(t *testing.T) {
	a := &TaskAdaptor{}
	// 5 sec → ceil(5/60) = 1 min (minimum) → 1/60 hour
	data, _ := common.Marshal(map[string]any{"audio_duration": 5.0})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.01,
				GroupRatio: 1.0,
			},
		},
	}
	quota := a.AdjustBillingOnComplete(task, nil)
	expected := int(0.01 * common.QuotaPerUnit * 1.0 * (1.0 / 60.0))
	if quota != expected {
		t.Fatalf("expected quota %d (1 min), got %d", expected, quota)
	}
}

func TestAdjustBilling_NoBillingContext(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{PrivateData: model.TaskPrivateData{}}
	if q := a.AdjustBillingOnComplete(task, nil); q != 0 {
		t.Fatalf("expected 0 for nil billing context, got %d", q)
	}
}

func TestAdjustBilling_UpstreamAmount(t *testing.T) {
	a := &TaskAdaptor{}
	// 上游返回实际金额 0.20 RMB，应按 amount/6.9*QuotaPerUnit*GroupRatio 折算。
	data, _ := common.Marshal(map[string]any{
		"result": map[string]any{
			"audio_duration": 315.0,
			"billing":        map[string]any{"amount": 0.20},
		},
	})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.36,
				GroupRatio: 2.0, // 验证参与上游金额折算
			},
		},
	}
	quota := a.AdjustBillingOnComplete(task, nil)
	expected := int(0.20 / 6.9 * common.QuotaPerUnit * 2.0)
	if quota != expected {
		t.Fatalf("expected quota %d (amount/6.9*QuotaPerUnit*GroupRatio), got %d", expected, quota)
	}
}

func TestAdjustBilling_NoUpstreamAmountFallsBack(t *testing.T) {
	a := &TaskAdaptor{}
	// 无 billing 字段时回退到 audio_duration 逻辑：90s → ceil = 2 min → 2/60 hour。
	data, _ := common.Marshal(map[string]any{
		"result": map[string]any{"audio_duration": 90.0},
	})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.01,
				GroupRatio: 1.5,
			},
		},
	}
	quota := a.AdjustBillingOnComplete(task, nil)
	expected := int(0.01 * common.QuotaPerUnit * 1.5 * (2.0 / 60.0))
	if quota != expected {
		t.Fatalf("expected fallback quota %d, got %d", expected, quota)
	}
}

func TestAdjustBilling_ZeroUpstreamAmountFallsBack(t *testing.T) {
	a := &TaskAdaptor{}
	// billing.amount == 0 时同样回退到 audio_duration 逻辑：90s → ceil = 2 min。
	data, _ := common.Marshal(map[string]any{
		"result": map[string]any{
			"audio_duration": 90.0,
			"billing":        map[string]any{"amount": 0.0},
		},
	})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.01,
				GroupRatio: 1.5,
			},
		},
	}
	quota := a.AdjustBillingOnComplete(task, nil)
	expected := int(0.01 * common.QuotaPerUnit * 1.5 * (2.0 / 60.0))
	if quota != expected {
		t.Fatalf("expected fallback quota %d, got %d", expected, quota)
	}
}

// ── SanitizeResultForClient ──────────────────────────────────────────

func TestSanitizeResultForClient_ConvertsAndStrips(t *testing.T) {
	// 真实上游样例：amount/detail 为 RMB，含 balance（上游账户余额）。
	data, _ := common.Marshal(map[string]any{
		"id":     "t1",
		"status": "done",
		"result": map[string]any{
			"audio_duration": 273.975,
			"billing": map[string]any{
				"amount":           0.0558,
				"balance":          4.7042,
				"billable_minutes": 5,
				"mode":             "zh_lite",
				"detail": map[string]any{
					"base_amount":          0.0225,
					"base_price_per_hour":  0.27,
					"diarize_amount":       0.0333,
					"diarize_price_per_hour": 0.4,
					"subtotal":             0.0558,
					"total":                0.0558,
					"mode":                 "zh_lite",
				},
			},
		},
	})

	const groupRatio = 1.5
	out := SanitizeResultForClient(data, groupRatio)

	var root map[string]any
	if err := common.Unmarshal(out, &root); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	billing := root["result"].(map[string]any)["billing"].(map[string]any)

	if _, ok := billing["balance"]; ok {
		t.Error("balance should be removed from client billing")
	}
	if got := billing["billable_minutes"].(float64); got != 5 {
		t.Errorf("billable_minutes should stay 5, got %v", got)
	}
	if got := billing["mode"].(string); got != "zh_lite" {
		t.Errorf("mode should stay zh_lite, got %v", got)
	}
	assertClose(t, "amount", billing["amount"].(float64), 0.0558/rmbToUSDRate*groupRatio)

	detail := billing["detail"].(map[string]any)
	assertClose(t, "detail.base_amount", detail["base_amount"].(float64), 0.0225/rmbToUSDRate*groupRatio)
	assertClose(t, "detail.base_price_per_hour", detail["base_price_per_hour"].(float64), 0.27/rmbToUSDRate*groupRatio)
	assertClose(t, "detail.diarize_amount", detail["diarize_amount"].(float64), 0.0333/rmbToUSDRate*groupRatio)
	assertClose(t, "detail.total", detail["total"].(float64), 0.0558/rmbToUSDRate*groupRatio)
	if got := detail["mode"].(string); got != "zh_lite" {
		t.Errorf("detail.mode should stay zh_lite, got %v", got)
	}
}

func TestSanitizeResultForClient_NoBilling(t *testing.T) {
	data, _ := common.Marshal(map[string]any{
		"id":     "t1",
		"status": "done",
		"result": map[string]any{"audio_duration": 90.0},
	})
	out := SanitizeResultForClient(data, 1.0)
	if string(out) != string(data) {
		t.Errorf("expected unchanged data when no billing present")
	}
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s: got %v, want %v", name, got, want)
	}
}

// ── BuildRequestURL ──────────────────────────────────────────────────

func TestBuildRequestURL(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"https://stt.amux.ai", "https://stt.amux.ai/tasks"},
		{"https://stt.amux.ai/", "https://stt.amux.ai/tasks"},
		{"https://stt.amux.ai/v1/", "https://stt.amux.ai/v1/tasks"},
	}
	for _, tc := range cases {
		a := &TaskAdaptor{baseURL: tc.base}
		url, err := a.BuildRequestURL(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url != tc.want {
			t.Errorf("base=%q: got %q, want %q", tc.base, url, tc.want)
		}
	}
}

// ── BuildRequestHeader ───────────────────────────────────────────────

func TestBuildRequestHeader(t *testing.T) {
	a := &TaskAdaptor{apiKey: "asr_test-123"}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if err := a.BuildRequestHeader(nil, req, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := req.Header.Get("X-API-Key"); got != "asr_test-123" {
		t.Errorf("api key header = %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
}

// ── BuildRequestBody ─────────────────────────────────────────────────

func TestBuildRequestBody_StripsClientFields(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body, _ := common.Marshal(map[string]any{
		"audio_url":       "https://example.com/a.mp3",
		"model":           "amux-stt-v1",
		"callback_url":    "https://hooks.example.com/done",
		"callback_secret": "s3cret",
		"options":         map[string]any{"language": "zh"},
	})
	setBody(c, body)

	a := &TaskAdaptor{}
	a.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})

	reader, err := a.BuildRequestBody(c, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, _ := io.ReadAll(reader)

	var result map[string]any
	_ = common.Unmarshal(out, &result)

	if _, ok := result["model"]; ok {
		t.Error("upstream body should not contain 'model'")
	}
	if _, ok := result["callback_url"]; ok {
		t.Error("upstream body should not contain 'callback_url'")
	}
	if _, ok := result["callback_secret"]; ok {
		t.Error("upstream body should not contain 'callback_secret'")
	}
	if result["audio_url"] != "https://example.com/a.mp3" {
		t.Errorf("audio_url missing or wrong: %v", result["audio_url"])
	}
}

// ── DoResponse ───────────────────────────────────────────────────────

func TestDoResponse_Success(t *testing.T) {
	body, _ := common.Marshal(map[string]any{
		"id":     "upstream-abc-123",
		"status": "processing",
	})
	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(io.Reader(io.NopCloser(jsonReader(body)))),
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_1"}}

	a := &TaskAdaptor{}
	id, data, taskErr := a.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("unexpected error: %s", taskErr.Message)
	}
	// 返回值是上游真实 ID（供轮询用）
	if id != "upstream-abc-123" {
		t.Errorf("expected upstream id=upstream-abc-123, got %s", id)
	}
	if len(data) == 0 {
		t.Error("expected non-empty taskData")
	}
	// 回写给客户端的是我们自己的公开 ID
	var out map[string]any
	if err := common.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("client response not valid json: %v", err)
	}
	if out["id"] != "task_public_1" {
		t.Errorf("expected client id=task_public_1, got %v", out["id"])
	}
}

func TestDoResponse_EmptyID(t *testing.T) {
	body, _ := common.Marshal(map[string]any{"status": "processing"})
	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(jsonReader(body)),
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_1"}}

	a := &TaskAdaptor{}
	_, _, taskErr := a.DoResponse(c, resp, info)
	if taskErr == nil {
		t.Fatal("expected error for empty ID")
	}
	if taskErr.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", taskErr.StatusCode)
	}
}

// ── ParseTaskResult ──────────────────────────────────────────────────

func TestParseTaskResult_Processing(t *testing.T) {
	body, _ := common.Marshal(map[string]any{
		"id":     "t1",
		"status": "processing",
	})
	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != model.TaskStatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", info.Status)
	}
	if info.TaskID != "t1" {
		t.Errorf("expected t1, got %s", info.TaskID)
	}
}

func TestParseTaskResult_Done(t *testing.T) {
	body, _ := common.Marshal(map[string]any{
		"id":     "t2",
		"status": "done",
		"result": map[string]any{
			"audio_duration": 315.5,
			"segments":       []any{},
		},
	})
	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Errorf("expected SUCCESS, got %s", info.Status)
	}
}

func TestParseTaskResult_Failed(t *testing.T) {
	body, _ := common.Marshal(map[string]any{
		"id":            "t3",
		"status":        "failed",
		"error_message": "audio format not supported",
	})
	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Errorf("expected FAILURE, got %s", info.Status)
	}
	if info.Reason != "audio format not supported" {
		t.Errorf("expected reason, got %q", info.Reason)
	}
}

func TestParseTaskResult_Abandoned(t *testing.T) {
	body, _ := common.Marshal(map[string]any{
		"id":     "t4",
		"status": "abandoned",
	})
	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Errorf("expected FAILURE for abandoned, got %s", info.Status)
	}
}

// ── FetchTask + merge via httptest ───────────────────────────────────

func TestFetchTask_Processing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tasks/t1" {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, map[string]any{"id": "t1", "status": "processing"})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	a := &TaskAdaptor{}
	resp, err := a.FetchTask(ts.URL, "key", map[string]any{"task_id": "t1"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if info.Status != model.TaskStatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", info.Status)
	}
}

func TestFetchTask_DoneMergesResult(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/t2":
			writeJSON(w, map[string]any{"id": "t2", "status": "done"})
		case "/tasks/t2/result":
			writeJSON(w, map[string]any{
				"task_id":        "t2",
				"audio_duration": 315.5,
				"segments":       []any{"seg1"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := &TaskAdaptor{}
	resp, err := a.FetchTask(ts.URL, "key", map[string]any{"task_id": "t2"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify merged structure
	var merged map[string]any
	_ = common.Unmarshal(body, &merged)
	if merged["status"] != "done" {
		t.Errorf("expected status=done, got %v", merged["status"])
	}
	resultRaw, ok := merged["result"]
	if !ok {
		t.Fatal("expected 'result' field in merged response")
	}
	resultBytes, _ := common.Marshal(resultRaw)
	var result map[string]any
	_ = common.Unmarshal(resultBytes, &result)
	if result["audio_duration"] != 315.5 {
		t.Errorf("expected audio_duration=315.5, got %v", result["audio_duration"])
	}

	// Verify ParseTaskResult works on merged body
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Errorf("expected SUCCESS, got %s", info.Status)
	}
}

func TestFetchTask_AuthHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]any{"id": "t1", "status": "processing"})
	}))
	defer ts.Close()

	a := &TaskAdaptor{}
	resp, err := a.FetchTask(ts.URL, "asr_my-secret-key", map[string]any{"task_id": "t1"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if gotAuth != "asr_my-secret-key" {
		t.Errorf("expected X-API-Key auth, got %q", gotAuth)
	}
}

func TestFetchTask_InvalidTaskID(t *testing.T) {
	a := &TaskAdaptor{}
	_, err := a.FetchTask("http://localhost", "key", map[string]any{}, "")
	if err == nil {
		t.Fatal("expected error for missing task_id")
	}
}

// ── End-to-end billing: pre-charge vs actual ─────────────────────────

func TestBillingFlow_PreChargeVsActual(t *testing.T) {
	a := &TaskAdaptor{}
	modelPrice := 0.36 // $0.36/hour
	groupRatio := 1.0

	// Pre-charge: 1 hour
	ratios := a.EstimateBilling(nil, nil)
	preChargeHours := ratios["hours"]
	preChargeQuota := int(modelPrice * common.QuotaPerUnit * groupRatio * preChargeHours)

	// Actual: 5 min 15 sec = 315s → ceil = 6 min → 6/60 hour
	actualDuration := 315.0
	actualMinutes := math.Ceil(actualDuration / 60.0)
	actualHours := actualMinutes / 60.0
	data, _ := common.Marshal(map[string]any{"audio_duration": actualDuration})
	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: modelPrice,
				GroupRatio: groupRatio,
			},
		},
	}
	actualQuota := a.AdjustBillingOnComplete(task, nil)
	expectedActual := int(modelPrice * common.QuotaPerUnit * groupRatio * actualHours)

	if actualQuota != expectedActual {
		t.Errorf("actual quota: expected %d, got %d", expectedActual, actualQuota)
	}
	if actualQuota >= preChargeQuota {
		t.Errorf("actual (%d) should be less than pre-charge (%d) for 315s audio", actualQuota, preChargeQuota)
	}

	refund := preChargeQuota - actualQuota
	if refund <= 0 {
		t.Error("expected positive refund")
	}
	t.Logf("pre-charge=%d, actual=%d, refund=%d (%.2f h pre / %.2f h actual)",
		preChargeQuota, actualQuota, refund, preChargeHours, actualHours)
}

// ── helpers ──────────────────────────────────────────────────────────

func jsonReader(data []byte) io.Reader {
	return io.NopCloser(io.Reader(nopReader(data)))
}

type nopReader []byte

func (n nopReader) Read(p []byte) (int, error) {
	return copy(p, n), io.EOF
}

func writeJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	w.Write(data)
}
