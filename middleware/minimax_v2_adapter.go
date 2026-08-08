package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

// MinimaxV2RequestConvert 标记 MiniMax v2 官方视频协议入口。
//
// 与 Ali/Doubao 的同类中间件一样，这里只做入口标记和最基本的形态校验，
// 不缓存请求体——common.UnmarshalBodyReusable 可重复调用，适配器直接解析成
// 自己的类型即可，省掉一层 map[string]any 的来回转换。
func MinimaxV2RequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set(constant.CtxKeyMinimaxV2Format, true)

		// 查询走 /v2/query/video_generation/:task_id 路径参数，
		// videoFetchByIDRespBodyBuilder 的 c.Param("task_id") 直接就能读到，
		// 这里不需要额外处理。
		if c.Request.Method == http.MethodPost {
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type"))), "application/json") {
				abortMinimaxV2(c, http.StatusBadRequest, "Content-Type must be application/json")
				return
			}
		}

		c.Next()
	}
}

// abortMinimaxV2 按 v2 的错误结构返回，保证原生协议客户端的错误处理分支
// 不用为网关单开一套。v2 用 HTTP 状态码 + error 对象，没有 v1 的 base_resp。
func abortMinimaxV2(c *gin.Context, statusCode int, message string) {
	c.AbortWithStatusJSON(statusCode, gin.H{
		"error": gin.H{
			"code":    "bad_request_error",
			"message": message,
		},
	})
}
