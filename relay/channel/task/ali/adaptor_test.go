package ali

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
)

func stubHappyHorseSourceDurationProbe(t *testing.T, duration float64) {
	t.Helper()
	original := happyHorseSourceDurationProbe
	happyHorseSourceDurationProbe = func(context.Context, string, int64) (float64, error) {
		return duration, nil
	}
	t.Cleanup(func() {
		happyHorseSourceDurationProbe = original
	})
}

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

func TestHappyHorseOfficialVideoPricing(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		want       float64
	}{
		{model: "happyhorse-1.1-t2v", resolution: "480P", want: 0.35},
		{model: "happyhorse-1.1-t2v", resolution: "720P", want: 0.70},
		{model: "happyhorse-1.1-i2v", resolution: "1080P", want: 0.90},
		{model: "happyhorse-1.0-r2v", resolution: "480P", want: 0.35},
		{model: "happyhorse-1.0-r2v", resolution: "720P", want: 0.70},
		{model: "happyhorse-1.0-video-edit", resolution: "1080P", want: 1.20},
	}
	for _, test := range tests {
		t.Run(test.model+"/"+test.resolution, func(t *testing.T) {
			got, err := billing_setting.ComputeVideoCost(test.model, billing_setting.VideoUsage{
				Resolution: test.resolution, OutputSeconds: 5,
			})
			if err != nil {
				t.Fatalf("compute video cost: %v", err)
			}
			if got.Total < test.want-1e-12 || got.Total > test.want+1e-12 {
				t.Fatalf("video cost=%v, want %v", got.Total, test.want)
			}
		})
	}
	if _, err := billing_setting.ComputeVideoCost("happyhorse-1.1-t2v", billing_setting.VideoUsage{
		Resolution: "480P", OutputSeconds: 5,
	}); err != nil {
		t.Fatalf("HappyHorse generation 480P must have a pricing tier: %v", err)
	}
	if _, err := billing_setting.ComputeVideoCost("happyhorse-1.0-video-edit", billing_setting.VideoUsage{
		Resolution: "480P", OutputSeconds: 5,
	}); err == nil {
		t.Fatal("HappyHorse Video Edit 480P must not have a pricing tier")
	}
}

func TestHappyHorseEstimateBillingUsesVideoCost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("ali_video_official_format", true)
	c.Set("ali_video_original_request", map[string]interface{}{
		"model": "happyhorse-1.1-t2v",
		"input": map[string]interface{}{"prompt": "horse running"},
	})
	info := &relaycommon.RelayInfo{OriginModelName: "happyhorse-1.1-t2v"}

	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if got, want := ratios[billing_setting.VideoCostRatioKey], 5*0.18; got < want-1e-12 || got > want+1e-12 {
		t.Fatalf("video_cost=%v, want %v", got, want)
	}
	if _, ok := ratios["seconds"]; ok {
		t.Fatalf("HappyHorse must use a single video_cost ratio: %#v", ratios)
	}
	snapshot, exists := c.Get(constant.CtxKeyVideoUsageSnapshot)
	if !exists {
		t.Fatal("missing HappyHorse video usage snapshot")
	}
	if snap, ok := snapshot.(*model.VideoUsageSnapshot); !ok || snap.Resolution != "1080P" || snap.OutputSeconds != 5 {
		t.Fatalf("unexpected usage snapshot: %#v", snapshot)
	}
}

func TestHappyHorseVideoEditPrechargeUsesTwiceSourceDuration(t *testing.T) {
	tests := []struct {
		sourceDuration float64
		want           float64
	}{
		{sourceDuration: 6.62, want: 13.24},
		{sourceDuration: 10, want: 20},
		{sourceDuration: 60, want: 120},
	}
	for _, test := range tests {
		req := &AliVideoRequest{
			Model:                     "happyhorse-1.0-video-edit",
			Parameters:                &AliVideoParameters{},
			BillingInputVideoDuration: test.sourceDuration,
		}
		if got := effectiveDuration(req); got != test.want {
			t.Fatalf("source duration=%v: precharge duration=%v, want %v", test.sourceDuration, got, test.want)
		}
	}
}

func TestHappyHorseVideoEditEstimateBillingUsesTwiceSourceDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubHappyHorseSourceDurationProbe(t, 6.62)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", nil)
	c.Set("ali_video_official_format", true)
	c.Set("ali_video_original_request", map[string]interface{}{
		"model": "happyhorse-1.0-video-edit",
		"input": map[string]interface{}{
			"prompt": "replace the background",
			"media": []interface{}{
				map[string]interface{}{"type": "video", "url": "https://example.com/source.mp4", "duration": 3},
			},
		},
		"parameters": map[string]interface{}{"resolution": "720P"},
	})
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: "happyhorse-1.0-video-edit",
	}

	if taskErr := (&TaskAdaptor{}).ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("validate video edit request: %#v", taskErr)
	}
	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if got, want := ratios[billing_setting.VideoCostRatioKey], 13.24*0.14; got < want-1e-12 || got > want+1e-12 {
		t.Fatalf("video edit precharge cost=%v, want %v", got, want)
	}
	snapshot, exists := c.Get(constant.CtxKeyVideoUsageSnapshot)
	if !exists {
		t.Fatal("missing video edit usage snapshot")
	}
	if snap, ok := snapshot.(*model.VideoUsageSnapshot); !ok || snap.OutputSeconds != 13.24 || !snap.HasVideoInput {
		t.Fatalf("unexpected video edit usage snapshot: %#v", snapshot)
	}
}

func TestHappyHorseVideoEditPreValidationBillingUsesMinimumGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("ali_video_official_format", true)
	c.Set("ali_video_original_request", map[string]interface{}{
		"model": "happyhorse-1.0-video-edit",
		"input": map[string]interface{}{
			"prompt": "replace the background",
			"media": []interface{}{
				map[string]interface{}{
					"type": "video", "url": "https://example.com/source.mp4", "duration": 60,
				},
			},
		},
		"parameters": map[string]interface{}{"resolution": "720P"},
	})
	info := &relaycommon.RelayInfo{OriginModelName: "happyhorse-1.0-video-edit"}

	ratios, taskErr := (&TaskAdaptor{}).EstimatePreValidationBilling(c, info)
	if taskErr != nil {
		t.Fatalf("estimate pre-validation billing: %#v", taskErr)
	}
	// 最短合法源视频 3 秒，输入与输出等长，因此探测前只建立 6 秒门槛。
	if got, want := ratios[billing_setting.VideoCostRatioKey], 6*0.14; got < want-1e-12 || got > want+1e-12 {
		t.Fatalf("pre-validation video_cost=%v, want %v", got, want)
	}
}

func TestHappyHorseAdjustBillingOnCompleteUsesOfficialTotalDuration(t *testing.T) {
	task := &model.Task{
		Data:       []byte(`{"output":{"task_status":"SUCCEEDED"},"usage":{"duration":13.24,"input_video_duration":6.62,"output_video_duration":6.62,"video_count":1,"SR":1080}}`),
		Properties: model.Properties{UpstreamModelName: "happyhorse-1.0-video-edit"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginModelName: "happyhorse-1.0-video-edit",
			GroupRatio:      1.5,
			OtherRatios: map[string]float64{
				billing_setting.VideoCostRatioKey: 12 * 0.24,
			},
			VideoUsage: &model.VideoUsageSnapshot{
				Resolution: "1080P", OutputSeconds: 12, HasVideoInput: true,
			},
		}},
	}

	got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil)
	want := int(float64(int(billing_setting.VideoBasePrice*common.QuotaPerUnit*1.5)) * 13.24 * 0.24)
	if got != want {
		t.Fatalf("actual quota=%d, want %d", got, want)
	}
	if snap := task.PrivateData.BillingContext.VideoUsage; snap == nil || snap.OutputSeconds != 13.24 || snap.Resolution != "1080P" {
		t.Fatalf("billing context did not record actual usage: %#v", snap)
	}
}

func TestHappyHorseAdjustBillingOnCompleteSkipsLegacyBillingContext(t *testing.T) {
	task := &model.Task{
		Data:       []byte(`{"usage":{"duration":10,"SR":1080}}`),
		Properties: model.Properties{UpstreamModelName: "happyhorse-1.1-t2v"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginModelName: "happyhorse-1.1-t2v",
			GroupRatio:      1,
			OtherRatios:     map[string]float64{"seconds": 5},
		}},
	}
	if got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil); got != 0 {
		t.Fatalf("legacy task quota=%d, want 0 to preserve original billing", got)
	}
}

func TestHappyHorseAdjustBillingOnCompleteRequiresOfficialUsage(t *testing.T) {
	task := &model.Task{
		Data:       []byte(`{"output":{"task_status":"SUCCEEDED"}}`),
		Properties: model.Properties{UpstreamModelName: "happyhorse-1.1-t2v"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginModelName: "happyhorse-1.1-t2v",
			GroupRatio:      1,
		}},
	}
	if got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil); got != 0 {
		t.Fatalf("quota=%d, want 0 without official usage", got)
	}
}

func TestHappyHorseI2VConversion(t *testing.T) {
	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:          "happyhorse-1.1-i2v",
		InputReference: "https://example.com/first.png",
		Metadata: map[string]interface{}{
			"resolution": "720P",
			"watermark":  false,
			"seed":       0,
		},
	})
	if err != nil {
		t.Fatalf("convert i2v request: %v", err)
	}
	if len(got.Input.Media) != 1 || got.Input.Media[0].Type != "first_frame" {
		t.Fatalf("unexpected i2v media: %#v", got.Input.Media)
	}
	if got.Parameters.Ratio != nil {
		t.Fatalf("i2v must not send ratio: %#v", got.Parameters.Ratio)
	}
	if got.Parameters.Duration == nil || *got.Parameters.Duration != 5 {
		t.Fatalf("unexpected i2v duration: %#v", got.Parameters.Duration)
	}
	if got.Parameters.Watermark == nil || *got.Parameters.Watermark {
		t.Fatalf("i2v watermark=false was not preserved: %#v", got.Parameters.Watermark)
	}
	if got.Parameters.Seed == nil || *got.Parameters.Seed != 0 {
		t.Fatalf("i2v seed=0 was not preserved: %#v", got.Parameters.Seed)
	}
}

func TestHappyHorseR2VConversion(t *testing.T) {
	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0-r2v",
		Prompt: "use [Image 1] and [Image 2]",
		Images: []string{"https://example.com/1.png", "https://example.com/2.png"},
		Metadata: map[string]interface{}{
			"ratio": "9:16",
		},
	})
	if err != nil {
		t.Fatalf("convert r2v request: %v", err)
	}
	if len(got.Input.Media) != 2 {
		t.Fatalf("unexpected r2v media count: %#v", got.Input.Media)
	}
	for _, media := range got.Input.Media {
		if media.Type != "reference_image" {
			t.Fatalf("unexpected r2v media: %#v", got.Input.Media)
		}
	}
	if got.Parameters.Ratio == nil || *got.Parameters.Ratio != "9:16" {
		t.Fatalf("unexpected r2v ratio: %#v", got.Parameters.Ratio)
	}
}

func TestHappyHorseVideoEditConversion(t *testing.T) {
	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0-video-edit",
		Prompt: "replace the clothes",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"type": "image_url", "role": "reference_image", "image_url": map[string]interface{}{"url": "https://example.com/clothes.png"}},
				map[string]interface{}{"type": "video_url", "role": "reference_video", "duration": 6.62, "video_url": map[string]interface{}{"url": "https://example.com/source.mp4"}},
			},
			"audio_setting": "origin",
			"watermark":     false,
			"seed":          0,
		},
	})
	if err != nil {
		t.Fatalf("convert video edit request: %v", err)
	}
	if len(got.Input.Media) != 2 || got.Input.Media[0].Type != "video" || got.Input.Media[1].Type != "reference_image" {
		t.Fatalf("unexpected video edit media: %#v", got.Input.Media)
	}
	if got.Parameters.Duration != nil || got.Parameters.Ratio != nil {
		t.Fatalf("video edit sent unsupported duration/ratio: %#v", got.Parameters)
	}
	if got.BillingInputVideoDuration != 6.62 || effectiveDuration(got) != 13.24 {
		t.Fatalf("unexpected video edit billing duration: %#v", got)
	}
	if got.Parameters.AudioSetting == nil || *got.Parameters.AudioSetting != "origin" {
		t.Fatalf("unexpected audio_setting: %#v", got.Parameters.AudioSetting)
	}
	if got.Parameters.Watermark == nil || *got.Parameters.Watermark {
		t.Fatalf("video edit watermark=false was not preserved: %#v", got.Parameters.Watermark)
	}
	body, err := common.Marshal(got)
	if err != nil {
		t.Fatalf("marshal video edit request: %v", err)
	}
	if strings.Contains(string(body), `"duration"`) {
		t.Fatalf("billing duration must not be sent upstream: %s", body)
	}
}

func TestHappyHorseVideoEditDerivesDurationWithoutClientHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubHappyHorseSourceDurationProbe(t, 8.5)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/video/generations", nil)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0-video-edit",
		Prompt: "replace the background",
		Metadata: map[string]interface{}{
			"media": []interface{}{
				map[string]interface{}{"type": "video", "url": "https://example.com/source.mp4"},
			},
		},
	})
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: "happyhorse-1.0-video-edit",
	}
	if taskErr := (&TaskAdaptor{}).ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("validate video edit request: %#v", taskErr)
	}
	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if got, want := ratios[billing_setting.VideoCostRatioKey], 17*0.24; got < want-1e-12 || got > want+1e-12 {
		t.Fatalf("video edit precharge cost=%v, want %v", got, want)
	}
}

func TestHappyHorseVideoEditDoesNotTrustClientDurationForRangeValidation(t *testing.T) {
	for _, duration := range []float64{2.99, 60.01} {
		got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
			Model:  "happyhorse-1.0-video-edit",
			Prompt: "replace the background",
			Metadata: map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{
						"type": "video", "url": "https://example.com/source.mp4", "duration": duration,
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("duration hint %v must not reject before probe: %v", duration, err)
		}
		if got.BillingInputVideoDuration != duration {
			t.Fatalf("duration hint=%v, captured=%v", duration, got.BillingInputVideoDuration)
		}
	}
}

func TestHappyHorseVideoEditRejectsOutOfRangeProbedSourceDuration(t *testing.T) {
	original := happyHorseSourceDurationProbe
	t.Cleanup(func() { happyHorseSourceDurationProbe = original })

	for _, duration := range []float64{2.99, 60.01} {
		probeDuration := duration
		happyHorseSourceDurationProbe = func(context.Context, string, int64) (float64, error) {
			return probeDuration, nil
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", nil)
		req := &AliVideoRequest{
			Model: "happyhorse-1.0-video-edit",
			Input: AliVideoInput{Media: []AliVideoMedia{
				{Type: "video", URL: "https://example.com/source.mp4"},
			}},
			Parameters: &AliVideoParameters{},
		}
		err := ensureVerifiedHappyHorseEditDuration(c, req)
		if err == nil || !strings.Contains(err.Error(), "must be between 3 and 60 seconds") {
			t.Fatalf("probed duration=%v error=%v", duration, err)
		}
	}
}

func TestHappyHorseVideoEditAcceptsDurationHintWithoutForwardingIt(t *testing.T) {
	got, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
		Model:    "happyhorse-1.0-video-edit",
		Prompt:   "replace the background",
		Duration: ptr(7),
		Metadata: map[string]interface{}{
			"media": []interface{}{
				map[string]interface{}{"type": "video", "url": "https://example.com/source.mp4"},
			},
		},
	})
	if err != nil {
		t.Fatalf("convert video edit request: %v", err)
	}
	if got.Parameters.Duration != nil || got.BillingInputVideoDuration != 7 || effectiveDuration(got) != 14 {
		t.Fatalf("unexpected duration hint conversion: %#v", got)
	}
}

func TestHappyHorseSubtypeValidation(t *testing.T) {
	tests := []struct {
		name string
		req  relaycommon.TaskSubmitReq
	}{
		{
			name: "i2v missing first frame",
			req:  relaycommon.TaskSubmitReq{Model: "happyhorse-1.1-i2v"},
		},
		{
			name: "i2v ratio unsupported",
			req: relaycommon.TaskSubmitReq{
				Model:          "happyhorse-1.1-i2v",
				InputReference: "https://example.com/first.png",
				Metadata:       map[string]interface{}{"ratio": "16:9"},
			},
		},
		{
			name: "r2v too many images",
			req: relaycommon.TaskSubmitReq{
				Model:  "happyhorse-1.1-r2v",
				Prompt: "test",
				Images: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
			},
		},
		{
			name: "edit missing video",
			req: relaycommon.TaskSubmitReq{
				Model:  "happyhorse-1.0-video-edit",
				Prompt: "test",
				Images: []string{"https://example.com/ref.png"},
			},
		},
		{
			name: "edit invalid audio setting",
			req: relaycommon.TaskSubmitReq{
				Model:  "happyhorse-1.0-video-edit",
				Prompt: "test",
				Metadata: map[string]interface{}{
					"media":         []interface{}{map[string]interface{}{"type": "video", "url": "https://example.com/source.mp4"}},
					"audio_setting": "mute",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, test.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHappyHorseVideoEditFloatUsageResponse(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"output":{"task_id":"task","task_status":"SUCCEEDED","video_url":"https://example.com/result.mp4"},
		"usage":{"duration":13.24,"input_video_duration":6.62,"output_video_duration":6.62,"video_count":1,"SR":720}
	}`))
	if err != nil {
		t.Fatalf("parse float usage response: %v", err)
	}
	if result.Status != model.TaskStatusSuccess || result.Url == "" {
		t.Fatalf("unexpected task result: %#v", result)
	}
}

func TestAliUsageAcceptsNumberAndStringValues(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "numbers",
			data: `{"usage":{"duration":13.24,"input_video_duration":6.62,"output_video_duration":6.62,"video_count":1,"SR":720}}`,
		},
		{
			name: "numeric strings",
			data: `{"usage":{"duration":"13.24","input_video_duration":"6.62","output_video_duration":"6.62","video_count":"1","SR":"720"}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage, ok := parseAliTaskUsage([]byte(test.data))
			if !ok {
				t.Fatal("expected usage to parse")
			}
			if float64(usage.Duration) != 13.24 ||
				float64(usage.InputVideoDuration) != 6.62 ||
				float64(usage.OutputVideoDuration) != 6.62 ||
				int(usage.VideoCount) != 1 || int(usage.SR) != 720 {
				t.Fatalf("unexpected usage: %#v", usage)
			}
		})
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

func TestHappyHorseResolutionValidationBySubtype(t *testing.T) {
	for _, modelName := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.0-t2v",
	} {
		t.Run(modelName+" accepts 480P", func(t *testing.T) {
			_, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
				Model:  modelName,
				Prompt: "test",
				Metadata: map[string]interface{}{
					"resolution": "480P",
				},
			})
			if err != nil {
				t.Fatalf("480P must be accepted: %v", err)
			}
		})
	}

	t.Run("video edit rejects 480P", func(t *testing.T) {
		_, err := (&TaskAdaptor{}).convertToAliRequest(&relaycommon.RelayInfo{}, relaycommon.TaskSubmitReq{
			Model:  "happyhorse-1.0-video-edit",
			Prompt: "test",
			Metadata: map[string]interface{}{
				"resolution": "480P",
				"media": []interface{}{
					map[string]interface{}{"type": "video", "url": "https://example.com/source.mp4"},
				},
			},
		})
		if err == nil {
			t.Fatal("Video Edit 480P must be rejected")
		}
	})
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

func TestMappedHappyHorsePricingFallsBackToUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "happyhorse-alias",
		Prompt: "test",
	})
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: "happyhorse-alias",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			IsModelMapped:     true,
			UpstreamModelName: "happyhorse-1.1-t2v",
		},
	}

	if taskErr := (&TaskAdaptor{}).ValidateMappedRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("mapped HappyHorse validation failed: %#v", taskErr)
	}
	priceData, err := relayhelper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		t.Fatalf("mapped HappyHorse base pricing failed: %v", err)
	}
	if priceData.ModelPrice != billing_setting.VideoBasePrice || !priceData.UsePrice {
		t.Fatalf("mapped HappyHorse base price=%#v", priceData)
	}
	info.PriceData = priceData
	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if got, want := ratios[billing_setting.VideoCostRatioKey], 5*0.18; got < want-1e-12 || got > want+1e-12 {
		t.Fatalf("mapped HappyHorse video_cost=%v, want %v", got, want)
	}
	for key, ratio := range ratios {
		info.PriceData.AddOtherRatio(key, ratio)
	}
	for _, ratio := range info.PriceData.OtherRatios {
		info.PriceData.Quota = int(float64(info.PriceData.Quota) * ratio)
	}
	wantQuota := int(float64(int(
		billing_setting.VideoBasePrice*common.QuotaPerUnit*priceData.GroupRatioInfo.GroupRatio,
	)) * ratios[billing_setting.VideoCostRatioKey])
	if info.PriceData.Quota != wantQuota {
		t.Fatalf("mapped HappyHorse precharge quota=%d, want %d", info.PriceData.Quota, wantQuota)
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
		"https://workspace.cn-beijing.maas.aliyuncs.com":          "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/":         "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/api":      "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/api/":     "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/api/v1":   "https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.ap-southeast-1.maas.aliyuncs.com":      "https://workspace.ap-southeast-1.maas.aliyuncs.com",
		"https://ws-example.ap-southeast-1.maas.aliyuncs.com/api": "https://ws-example.ap-southeast-1.maas.aliyuncs.com",
		"https://dashscope-us.aliyuncs.com/api/v1":                "https://dashscope-us.aliyuncs.com",
		"https://workspace.eu-central-1.maas.aliyuncs.com":        "https://workspace.eu-central-1.maas.aliyuncs.com",
		"https://workspace.ap-northeast-1.maas.aliyuncs.com":      "https://workspace.ap-northeast-1.maas.aliyuncs.com",
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

	for _, suffix := range []string{"/api", "/api/v1"} {
		resp, err := (&TaskAdaptor{}).FetchTask(server.URL+suffix, "ali-task-key", map[string]any{
			"task_id": "upstream-task",
		}, "")
		if err != nil {
			t.Fatalf("FetchTask with %s: %v", suffix, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status with %s=%d", suffix, resp.StatusCode)
		}
	}
}

func TestBuildRequestURLAcceptsWorkspaceAPIBase(t *testing.T) {
	want := "https://workspace.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis"
	for _, baseURL := range []string{
		"https://workspace.cn-beijing.maas.aliyuncs.com",
		"https://workspace.cn-beijing.maas.aliyuncs.com/api",
		"https://workspace.cn-beijing.maas.aliyuncs.com/api/v1/",
	} {
		adaptor := &TaskAdaptor{baseURL: baseURL}
		got, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
		if err != nil {
			t.Fatalf("BuildRequestURL with %q: %v", baseURL, err)
		}
		if got != want {
			t.Fatalf("request URL with %q=%q, want=%q", baseURL, got, want)
		}
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

func TestOfficialHappyHorseVideoEditUsesDurationAsBillingHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("ali_video_original_request", map[string]interface{}{
		"model": "happyhorse-1.0-video-edit",
		"input": map[string]interface{}{
			"prompt": "replace the background",
			"media": []interface{}{
				map[string]interface{}{"type": "video", "url": "https://example.com/source.mp4"},
			},
		},
		"parameters": map[string]interface{}{
			"resolution": "720P",
			"duration":   7,
		},
	})

	got, err := (&TaskAdaptor{}).getOfficialRequest(c, nil)
	if err != nil {
		t.Fatalf("getOfficialRequest: %v", err)
	}
	if got.BillingInputVideoDuration != 7 || effectiveDuration(got) != 14 {
		t.Fatalf("unexpected official billing duration: %#v", got)
	}
	if got.Parameters.Duration != nil {
		t.Fatalf("official billing hint must be removed before forwarding: %#v", got.Parameters)
	}
	body, err := common.Marshal(got)
	if err != nil {
		t.Fatalf("marshal official request: %v", err)
	}
	if strings.Contains(string(body), `"duration"`) {
		t.Fatalf("official billing hint leaked upstream: %s", body)
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
