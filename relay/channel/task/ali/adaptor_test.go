package ali

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func TestHappyHorseConversionPreservesExplicitFalseAndZero(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-t2v",
		Prompt: "a cardboard city at night",
		Metadata: map[string]interface{}{
			"resolution": "720P",
			"ratio":      "9:16",
			"duration":   3,
			"watermark":  false,
			"seed":       0,
		},
	}

	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}
	if got.Parameters.Watermark == nil || *got.Parameters.Watermark {
		t.Fatalf("watermark=false was not preserved: %#v", got.Parameters.Watermark)
	}
	if got.Parameters.Seed == nil || *got.Parameters.Seed != 0 {
		t.Fatalf("seed=0 was not preserved: %#v", got.Parameters.Seed)
	}
	body, err := common.Marshal(got)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	for _, fragment := range []string{`"watermark":false`, `"seed":0`, `"ratio":"9:16"`} {
		if !strings.Contains(string(body), fragment) {
			t.Fatalf("marshaled request missing %s: %s", fragment, body)
		}
	}
}

func TestHappyHorseDefaults(t *testing.T) {
	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0-t2v",
		Prompt: "horse running",
	})
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}
	if got.Parameters.Resolution == nil || *got.Parameters.Resolution != "1080P" {
		t.Fatalf("unexpected resolution: %#v", got.Parameters.Resolution)
	}
	if got.Parameters.Ratio == nil || *got.Parameters.Ratio != "16:9" {
		t.Fatalf("unexpected ratio: %#v", got.Parameters.Ratio)
	}
	if got.Parameters.Duration == nil || *got.Parameters.Duration != 5 {
		t.Fatalf("unexpected duration: %#v", got.Parameters.Duration)
	}
	if got.Parameters.Watermark == nil || !*got.Parameters.Watermark {
		t.Fatalf("HappyHorse watermark default must be true")
	}
}

func TestHappyHorseValidation(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
	}{
		{name: "resolution", metadata: map[string]interface{}{"resolution": "1440P"}},
		{name: "ratio", metadata: map[string]interface{}{"ratio": "2:1"}},
		{name: "duration", metadata: map[string]interface{}{"duration": 2}},
		{name: "media", metadata: map[string]interface{}{"first_frame_url": "https://example.com/a.png"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
				Model:    "happyhorse-1.1-t2v",
				Prompt:   "test",
				Metadata: test.metadata,
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHappyHorseExplicitZeroDurationIsRejected(t *testing.T) {
	var req relaycommon.TaskSubmitReq
	if err := common.Unmarshal([]byte(`{"model":"happyhorse-1.1-t2v","prompt":"test","duration":0}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if req.Duration == nil || *req.Duration != 0 {
		t.Fatalf("explicit duration=0 was not preserved: %#v", req.Duration)
	}
	if _, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, req); err == nil {
		t.Fatal("expected explicit duration=0 to fail validation")
	}
}

func TestMappedModelValidationRunsBeforeBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "ali-video-alias",
		Prompt: "test",
		Metadata: map[string]interface{}{
			"duration": 2,
		},
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			IsModelMapped:     true,
			UpstreamModelName: "happyhorse-1.1-t2v",
		},
	}
	taskErr := (&TaskAdaptor{}).ValidateMappedRequestAndSetAction(c, info)
	if taskErr == nil {
		t.Fatal("expected mapped HappyHorse validation error")
	}
	if taskErr.StatusCode != http.StatusBadRequest || !taskErr.LocalError {
		t.Fatalf("unexpected mapped validation error: %#v", taskErr)
	}
}

func TestWan27MediaConversions(t *testing.T) {
	tests := []struct {
		name      string
		req       relaycommon.TaskSubmitReq
		wantTypes []string
	}{
		{
			name: "first frame",
			req: relaycommon.TaskSubmitReq{
				Model:          "wan2.7-i2v-2026-04-25",
				Prompt:         "cat running",
				InputReference: "https://example.com/first.png",
			},
			wantTypes: []string{"first_frame"},
		},
		{
			name: "first last audio",
			req: relaycommon.TaskSubmitReq{
				Model:  "wan2.7-i2v-2026-04-25",
				Prompt: "cat running",
				Metadata: map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "image_url", "role": "first_frame", "image_url": map[string]interface{}{"url": "https://example.com/first.png"}},
						map[string]interface{}{"type": "image_url", "role": "last_frame", "image_url": map[string]interface{}{"url": "https://example.com/last.png"}},
						map[string]interface{}{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]interface{}{"url": "https://example.com/audio.mp3"}},
					},
				},
			},
			wantTypes: []string{"first_frame", "last_frame", "driving_audio"},
		},
		{
			name: "first clip last frame",
			req: relaycommon.TaskSubmitReq{
				Model:  "wan2.7-i2v-2026-04-25",
				Prompt: "continue the scene",
				Metadata: map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "video_url", "role": "reference_video", "video_url": map[string]interface{}{"url": "https://example.com/clip.mp4"}},
						map[string]interface{}{"type": "image_url", "role": "last_frame", "image_url": map[string]interface{}{"url": "https://example.com/last.png"}},
					},
				},
			},
			wantTypes: []string{"first_clip", "last_frame"},
		},
		{
			name: "playground reference images become first and last frame",
			req: relaycommon.TaskSubmitReq{
				Model:  "wan2.7-i2v-2026-04-25",
				Prompt: "transition",
				Metadata: map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "image_url", "role": "reference_image", "image_url": map[string]interface{}{"url": "https://example.com/first.png"}},
						map[string]interface{}{"type": "image_url", "role": "reference_image", "image_url": map[string]interface{}{"url": "https://example.com/last.png"}},
					},
				},
			},
			wantTypes: []string{"first_frame", "last_frame"},
		},
		{
			name: "playground clip plus reference image becomes last frame",
			req: relaycommon.TaskSubmitReq{
				Model:  "wan2.7-i2v-2026-04-25",
				Prompt: "continue",
				Metadata: map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "video_url", "role": "reference_video", "video_url": map[string]interface{}{"url": "https://example.com/clip.mp4"}},
						map[string]interface{}{"type": "image_url", "role": "reference_image", "image_url": map[string]interface{}{"url": "https://example.com/last.png"}},
					},
				},
			},
			wantTypes: []string{"first_clip", "last_frame"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, test.req)
			if err != nil {
				t.Fatalf("convert request: %v", err)
			}
			if len(got.Input.Media) != len(test.wantTypes) {
				t.Fatalf("media length=%d, want=%d: %#v", len(got.Input.Media), len(test.wantTypes), got.Input.Media)
			}
			for i, want := range test.wantTypes {
				if got.Input.Media[i].Type != want {
					t.Fatalf("media[%d].type=%q, want=%q", i, got.Input.Media[i].Type, want)
				}
			}
		})
	}
}

func TestWan27OpenAIMultipartInputReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("model", "wan2.7-i2v-2026-04-25"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("prompt", "a cat looks at the sky"); err != nil {
		t.Fatal(err)
	}
	part, err := w.CreateFormFile("input_reference", "first.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("validate multipart request: %#v", taskErr)
	}
	if taskErr := adaptor.ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("validate mapped multipart request: %#v", taskErr)
	}
	if info.Action != "generate" {
		t.Fatalf("action=%q", info.Action)
	}

	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build request body: %v", err)
	}
	encoded, err := io.ReadAll(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	var got AliVideoRequest
	if err := common.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal upstream body: %v", err)
	}
	if len(got.Input.Media) != 1 || got.Input.Media[0].Type != "first_frame" ||
		!strings.HasPrefix(got.Input.Media[0].URL, "data:image/png;base64,") {
		t.Fatalf("unexpected multipart media: %#v", got.Input.Media)
	}
}

func TestWan27InvalidMediaCombinations(t *testing.T) {
	tests := []struct {
		name    string
		content []interface{}
	}{
		{
			name: "audio only",
			content: []interface{}{
				map[string]interface{}{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]interface{}{"url": "https://example.com/a.mp3"}},
			},
		},
		{
			name: "first frame and first clip",
			content: []interface{}{
				map[string]interface{}{"type": "image_url", "role": "first_frame", "image_url": map[string]interface{}{"url": "https://example.com/f.png"}},
				map[string]interface{}{"type": "video_url", "role": "first_clip", "video_url": map[string]interface{}{"url": "https://example.com/v.mp4"}},
			},
		},
		{
			name: "duplicate frame",
			content: []interface{}{
				map[string]interface{}{"type": "image_url", "role": "first_frame", "image_url": map[string]interface{}{"url": "https://example.com/a.png"}},
				map[string]interface{}{"type": "image_url", "role": "first_frame", "image_url": map[string]interface{}{"url": "https://example.com/b.png"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
				Model:  "wan2.7-i2v-2026-04-25",
				Prompt: "test",
				Metadata: map[string]interface{}{
					"content": test.content,
				},
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWan27SupportsAllDocumentedMediaCombinations(t *testing.T) {
	frame := func(mediaType string) AliVideoMedia {
		return AliVideoMedia{Type: mediaType, URL: "https://example.com/" + mediaType}
	}
	tests := []struct {
		name  string
		media []AliVideoMedia
	}{
		{name: "first frame", media: []AliVideoMedia{frame("first_frame")}},
		{name: "first frame and audio", media: []AliVideoMedia{frame("first_frame"), frame("driving_audio")}},
		{name: "first and last frame", media: []AliVideoMedia{frame("first_frame"), frame("last_frame")}},
		{name: "first last and audio", media: []AliVideoMedia{frame("first_frame"), frame("last_frame"), frame("driving_audio")}},
		{name: "first clip", media: []AliVideoMedia{frame("first_clip")}},
		{name: "first clip and last frame", media: []AliVideoMedia{frame("first_clip"), frame("last_frame")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &AliVideoRequest{
				Model: "wan2.7-i2v-2026-04-25",
				Input: AliVideoInput{Media: test.media},
			}
			if err := validateAliVideoRequest(req); err != nil {
				t.Fatalf("documented combination rejected: %v", err)
			}
		})
	}
}

func TestMetadataCannotOverrideModel(t *testing.T) {
	_, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-t2v",
		Prompt: "test",
		Metadata: map[string]interface{}{
			"model": "wan2.7-i2v-2026-04-25",
		},
	})
	if err == nil {
		t.Fatal("expected model override error")
	}
}

func TestAliBaseURLNormalization(t *testing.T) {
	tests := map[string]string{
		"https://workspace.cn-beijing.maas.aliyuncs.com":        "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/":       "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/api/v1": "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.ap-southeast-1.maas.aliyuncs.com":    "https://workspace.ap-southeast-1.maas.aliyuncs.com",
		"https://dashscope-us.aliyuncs.com/api/v1":              "https://dashscope-us.aliyuncs.com",
		"https://workspace.eu-central-1.maas.aliyuncs.com":      "https://workspace.eu-central-1.maas.aliyuncs.com",
		"https://workspace.ap-northeast-1.maas.aliyuncs.com":    "https://workspace.ap-northeast-1.maas.aliyuncs.com",
	}
	for input, want := range tests {
		got, err := normalizeAliBaseURL(input)
		if err != nil {
			t.Fatalf("normalize %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalize %q=%q, want=%q", input, got, want)
		}
	}
	if _, err := normalizeAliBaseURL("https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com"); err == nil {
		t.Fatal("expected WorkspaceId placeholder error")
	}
}

func TestBuildRequestHeaderUsesDashScopeAsyncProtocol(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
	adaptor := &TaskAdaptor{apiKey: "ali-key"}
	if err := adaptor.BuildRequestHeader(nil, req, nil); err != nil {
		t.Fatalf("BuildRequestHeader: %v", err)
	}
	if req.Header.Get("Authorization") != "Bearer ali-key" ||
		req.Header.Get("Content-Type") != "application/json" ||
		req.Header.Get("X-DashScope-Async") != "enable" {
		t.Fatalf("unexpected headers: %#v", req.Header)
	}
}

func TestFetchTaskUsesNormalizedRegionalBaseAndSubmissionKey(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/upstream-task" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ali-task-key" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"task_id":"upstream-task","task_status":"RUNNING"}}`))
	}))
	defer server.Close()

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL+"/api/v1", "ali-task-key", map[string]any{
		"task_id": "upstream-task",
	}, "")
	if err != nil {
		t.Fatalf("FetchTask: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestBuildRequestURLAcceptsAPIv1Base(t *testing.T) {
	adaptor := &TaskAdaptor{baseURL: "https://workspace.cn-beijing.maas.aliyuncs.com/api/v1/"}
	got, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
	if err != nil {
		t.Fatalf("BuildRequestURL: %v", err)
	}
	want := "https://workspace.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis"
	if got != want {
		t.Fatalf("request URL=%q, want=%q", got, want)
	}
}

func TestOfficialRequestPreservesExplicitValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("ali_video_original_request", map[string]interface{}{
		"model": "happyhorse-1.1-t2v",
		"input": map[string]interface{}{"prompt": "test"},
		"parameters": map[string]interface{}{
			"watermark": false,
			"seed":      0,
		},
	})

	got, err := (&TaskAdaptor{}).getOfficialRequest(c, nil)
	if err != nil {
		t.Fatalf("getOfficialRequest: %v", err)
	}
	if got.Parameters.Watermark == nil || *got.Parameters.Watermark {
		t.Fatalf("watermark=false was not preserved")
	}
	if got.Parameters.Seed == nil || *got.Parameters.Seed != 0 {
		t.Fatalf("seed=0 was not preserved")
	}
}

func TestOfficialWan27AllowsPromptOmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("ali_video_original_request", map[string]interface{}{
		"model": "wan2.7-i2v-2026-04-25",
		"input": map[string]interface{}{
			"media": []interface{}{
				map[string]interface{}{"type": "first_frame", "url": "https://example.com/first.png"},
			},
		},
	})
	if _, err := (&TaskAdaptor{}).getOfficialRequest(c, nil); err != nil {
		t.Fatalf("official wan2.7 prompt is optional: %v", err)
	}
}

func TestOfficialSubmitReturnsPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("ali_video_official_format", true)
	c.Set("model", "happyhorse-1.1-t2v")
	response := `{"output":{"task_status":"PENDING","task_id":"upstream-id"},"request_id":"request-id"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(response)),
	}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

	taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse error: %v", taskErr)
	}
	if taskID != "upstream-id" {
		t.Fatalf("upstream task id=%q", taskID)
	}
	if !strings.Contains(string(taskData), `"task_id":"upstream-id"`) {
		t.Fatalf("stored task data must preserve upstream id: %s", taskData)
	}
	if !strings.Contains(recorder.Body.String(), `"task_id":"task_public"`) {
		t.Fatalf("client response must use public id: %s", recorder.Body.String())
	}
}

func TestConvertToAliVideoUsesGatewayResultURL(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Data:   []byte(`{"output":{"task_status":"SUCCEEDED","task_id":"upstream-id","video_url":"https://upstream.example/video.mp4"}}`),
	}
	task.PrivateData.ResultURL = "https://gateway.example/video.mp4"

	body, err := (&TaskAdaptor{}).ConvertToAliVideo(task)
	if err != nil {
		t.Fatalf("ConvertToAliVideo: %v", err)
	}
	var got AliVideoResponse
	if err := common.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Output.TaskID != "task_public" || got.Output.VideoURL != "https://gateway.example/video.mp4" {
		t.Fatalf("unexpected official response: %#v", got.Output)
	}
}

func TestConvertToAliVideoHidesUpstreamURLWhileArchiving(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusInProgress,
		Data:   []byte(`{"output":{"task_status":"SUCCEEDED","task_id":"upstream-id","video_url":"https://upstream.example/video.mp4"}}`),
	}

	body, err := (&TaskAdaptor{}).ConvertToAliVideo(task)
	if err != nil {
		t.Fatalf("ConvertToAliVideo: %v", err)
	}
	var got AliVideoResponse
	if err := common.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Output.TaskStatus != "RUNNING" || got.Output.VideoURL != "" {
		t.Fatalf("archiving response exposed upstream URL: %#v", got.Output)
	}
}

func TestConvertToAliVideoPreservesUnknownAndCanceledStatuses(t *testing.T) {
	for _, status := range []string{"UNKNOWN", "CANCELED"} {
		t.Run(status, func(t *testing.T) {
			task := &model.Task{
				TaskID:     "task_public",
				Status:     model.TaskStatusFailure,
				FailReason: "task failed",
				Data:       []byte(`{"output":{"task_status":"` + status + `","task_id":"upstream-id"}}`),
			}
			body, err := (&TaskAdaptor{}).ConvertToAliVideo(task)
			if err != nil {
				t.Fatal(err)
			}
			var got AliVideoResponse
			if err := common.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if got.Output.TaskStatus != status || got.Output.Code != "" || got.Output.Message != "" {
				t.Fatalf("unexpected response: %#v", got.Output)
			}
		})
	}
}

func TestConvertToOpenAIVideoHidesUpstreamURLWhileArchiving(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusInProgress,
		Data:   []byte(`{"output":{"task_status":"SUCCEEDED","task_id":"upstream-id","video_url":"https://upstream.example/video.mp4"}}`),
	}

	body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("ConvertToOpenAIVideo: %v", err)
	}
	var got dto.OpenAIVideo
	if err := common.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Status != dto.VideoStatusInProgress {
		t.Fatalf("status=%q", got.Status)
	}
	if url, _ := got.Metadata["url"].(string); url != "" {
		t.Fatalf("archiving response exposed upstream URL: %q", url)
	}
}

func TestLegacyInputMetadataDoesNotDropInputReference(t *testing.T) {
	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:          "wan2.5-i2v-preview",
		Prompt:         "test",
		InputReference: "https://example.com/first.png",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"negative_prompt": "blur",
			},
		},
	})
	if err != nil {
		t.Fatalf("convert legacy request: %v", err)
	}
	if got.Input.ImgURL != "https://example.com/first.png" || got.Input.NegativePrompt != "blur" {
		t.Fatalf("legacy input merge regressed: %#v", got.Input)
	}
}
