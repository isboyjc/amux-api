package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestVideoRouterRegistersGenericAndDashScopeRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		http.MethodPost + " /v1/videos",
		http.MethodPost + " /v1/video/generations",
		http.MethodGet + " /v1/videos/:task_id",
		http.MethodGet + " /v1/video/generations/:task_id",
		http.MethodPost + " /api/v1/services/aigc/video-generation/video-synthesis",
		http.MethodGet + " /api/v1/tasks/:task_id",
		// MiniMax v2 原生协议：查询走路径参数，与官方一致
		http.MethodPost + " /v2/video_generation",
		http.MethodGet + " /v2/query/video_generation/:task_id",
	} {
		if !routes[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
}
