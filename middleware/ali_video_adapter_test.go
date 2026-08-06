package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAliVideoRequestConvertRequiresOfficialHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		contentType string
		asyncHeader string
		wantMessage string
	}{
		{name: "content type", contentType: "text/plain", asyncHeader: "enable", wantMessage: "Content-Type must be application/json"},
		{name: "async header", contentType: "application/json", wantMessage: "X-DashScope-Async must be enable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{"model":"happyhorse-1.1-t2v"}`))
			c.Request.Header.Set("Content-Type", test.contentType)
			if test.asyncHeader != "" {
				c.Request.Header.Set("X-DashScope-Async", test.asyncHeader)
			}

			AliVideoRequestConvert()(c)
			if !c.IsAborted() || recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), test.wantMessage) {
				t.Fatalf("unexpected response: status=%d aborted=%v body=%s", recorder.Code, c.IsAborted(), recorder.Body.String())
			}
		})
	}
}

func TestAliVideoRequestConvertCachesValidOfficialBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{"model":"happyhorse-1.1-t2v","input":{"prompt":"test"}}`))
	c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
	c.Request.Header.Set("X-DashScope-Async", "enable")

	AliVideoRequestConvert()(c)
	if c.IsAborted() {
		t.Fatalf("valid request was aborted: %s", recorder.Body.String())
	}
	original, exists := c.Get("ali_video_original_request")
	if !exists || original == nil {
		t.Fatal("official request body was not cached")
	}
}
