package minimax

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func TestGetRequestURLForImageGeneration(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.minimax.chat",
		},
	}

	got, err := GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://api.minimax.chat/v1/image_generation"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestConvertImageRequest(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "image-01",
	}
	request := dto.ImageRequest{
		Model:          "image-01",
		Prompt:         "a red fox in snowfall",
		Size:           "1536x1024",
		ResponseFormat: "url",
		N:              uintPtr(2),
	}

	got, err := adaptor.ConvertImageRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, request)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["model"] != "image-01" {
		t.Fatalf("model = %#v, want %q", payload["model"], "image-01")
	}
	if payload["prompt"] != request.Prompt {
		t.Fatalf("prompt = %#v, want %q", payload["prompt"], request.Prompt)
	}
	if payload["n"] != float64(2) {
		t.Fatalf("n = %#v, want 2", payload["n"])
	}
	if payload["aspect_ratio"] != "3:2" {
		t.Fatalf("aspect_ratio = %#v, want %q", payload["aspect_ratio"], "3:2")
	}
	if payload["response_format"] != "url" {
		t.Fatalf("response_format = %#v, want %q", payload["response_format"], "url")
	}
}

func TestGetRequestURLForImageEdit(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.minimax.chat",
		},
	}
	got, err := GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}
	want := "https://api.minimax.chat/v1/image_generation"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q (i2i should reuse t2i endpoint)", got, want)
	}
}

func TestConvertImageRequestEditMultipart(t *testing.T) {
	t.Parallel()

	// 构造一个真实的 multipart 请求：image 文件 + 表单字段
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fw, err := writer.CreateFormFile("image", "ref.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 'f', 'a', 'k', 'e'}
	if _, err := fw.Write(fakeJPEG); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	_ = writer.WriteField("model", "image-01")
	_ = writer.WriteField("prompt", "a portrait in cyberpunk style")
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: "image-01",
	}
	got, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "image-01",
		Prompt: "a portrait in cyberpunk style",
	})
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	srRaw, ok := decoded["subject_reference"].([]any)
	if !ok || len(srRaw) == 0 {
		t.Fatalf("subject_reference missing or wrong type: %#v", decoded["subject_reference"])
	}
	first, ok := srRaw[0].(map[string]any)
	if !ok {
		t.Fatalf("subject_reference[0] type = %T, want map", srRaw[0])
	}
	if first["type"] != "character" {
		t.Fatalf("subject_reference[0].type = %#v, want %q", first["type"], "character")
	}
	dataURL, ok := first["image_file"].(string)
	if !ok || !strings.HasPrefix(dataURL, "data:") || !strings.Contains(dataURL, ";base64,") {
		t.Fatalf("subject_reference[0].image_file = %#v, want string data URL (per MiniMax official spec)", first["image_file"])
	}
}

func TestConvertImageRequestReadsExtraBody(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "image-01",
	}

	// Playground / OpenAI SDK 把私有参数塞 extra_body 里，后端必须能读到。
	// image_file 用数组写法（民间博客示例的旧形式），后端要拍扁成 string 再发给上游。
	extraBody := json.RawMessage(`{
		"aspect_ratio": "9:16",
		"prompt_optimizer": false,
		"seed": 42,
		"subject_reference": [
			{"type": "character", "image_file": ["data:image/png;base64,QUJD"]}
		]
	}`)

	got, err := adaptor.ConvertImageRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info,
		dto.ImageRequest{
			Model:     "image-01",
			Prompt:    "a portrait",
			ExtraBody: extraBody,
		},
	)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["aspect_ratio"] != "9:16" {
		t.Fatalf("aspect_ratio = %#v, want %q", payload["aspect_ratio"], "9:16")
	}
	if payload["prompt_optimizer"] != false {
		t.Fatalf("prompt_optimizer = %#v, want false", payload["prompt_optimizer"])
	}
	if payload["seed"] != float64(42) {
		t.Fatalf("seed = %#v, want 42", payload["seed"])
	}
	srArr, ok := payload["subject_reference"].([]any)
	if !ok || len(srArr) == 0 {
		t.Fatalf("subject_reference missing or wrong type: %#v", payload["subject_reference"])
	}
	first, ok := srArr[0].(map[string]any)
	if !ok {
		t.Fatalf("subject_reference[0] type = %T, want map", srArr[0])
	}
	// 即便用户用数组写法透传，后端也要拍扁成 string 发给上游
	if _, isStr := first["image_file"].(string); !isStr {
		t.Fatalf("subject_reference[0].image_file = %#v, want flattened string per MiniMax spec", first["image_file"])
	}
}

func TestConvertImageRequestAssemblesStyle(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "image-01-live",
	}

	// schema 平铺写：style_type / style_weight 在 extra_body 顶层
	extraBody := json.RawMessage(`{
		"style_type": "漫画",
		"style_weight": 0.7
	}`)

	got, err := adaptor.ConvertImageRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info,
		dto.ImageRequest{
			Model:     "image-01-live",
			Prompt:    "a young hero",
			ExtraBody: extraBody,
		},
	)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	style, ok := payload["style"].(map[string]any)
	if !ok {
		t.Fatalf("style missing or wrong type: %#v", payload["style"])
	}
	if style["style_type"] != "漫画" {
		t.Fatalf("style.style_type = %#v, want %q", style["style_type"], "漫画")
	}
	if style["style_weight"] != 0.7 {
		t.Fatalf("style.style_weight = %#v, want 0.7", style["style_weight"])
	}
	// 平铺字段不应该泄漏到顶层
	if _, leaked := payload["style_type"]; leaked {
		t.Fatalf("style_type leaked to top level: %#v", payload)
	}
}

func TestConvertImageRequestEditRejectsNonImage01(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: "abab6.5-chat",
	}
	_, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "abab6.5-chat",
		Prompt: "edit me",
	})
	if err == nil {
		t.Fatalf("expected error for non-image-01 model on edit endpoint, got nil")
	}
}

func TestDoResponseForImageGeneration(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Unix(1700000000, 0),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       httptest.NewRecorder().Result().Body,
	}
	resp.Body = ioNopCloser(`{"data":{"image_urls":["https://example.com/minimax.png"]}}`)

	adaptor := &Adaptor{}
	usage, err := adaptor.DoResponse(c, resp, info)
	if err != nil {
		t.Fatalf("DoResponse returned error: %v", err)
	}
	if usage == nil {
		t.Fatalf("DoResponse returned nil usage")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"url":"https://example.com/minimax.png"`) {
		t.Fatalf("response body = %s, want OpenAI image response with image URL", body)
	}
	if strings.Contains(body, `"image_urls"`) {
		t.Fatalf("response body = %s, should not expose raw MiniMax image_urls payload", body)
	}
}

type nopReadCloser struct {
	*strings.Reader
}

func (n nopReadCloser) Close() error {
	return nil
}

func ioNopCloser(body string) nopReadCloser {
	return nopReadCloser{Reader: strings.NewReader(body)}
}

func uintPtr(v uint) *uint {
	return &v
}
