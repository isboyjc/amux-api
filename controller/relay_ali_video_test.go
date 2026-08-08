package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
)

func TestRespondTaskErrorPreservesDashScopeError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("ali_video_official_format", true)

	respondTaskError(c, &dto.TaskError{
		Code:       "fail_to_fetch_task",
		Message:    "wrapped error",
		StatusCode: http.StatusUnauthorized,
		Error:      errors.New(`{"code":"InvalidApiKey","message":"No API-key provided.","request_id":"upstream-request"}`),
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, fragment := range []string{`"code":"InvalidApiKey"`, `"message":"No API-key provided."`, `"request_id":"upstream-request"`} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("response missing %s: %s", fragment, body)
		}
	}
}
