package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// AliVideoRequestConvert 标记 DashScope 官方视频协议入口，并缓存原始请求体。
// 适配器会对请求做模型映射和白名单字段解析，不直接透传未校验 JSON。
func AliVideoRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set("ali_video_official_format", true)
		if c.Request.Method == http.MethodPost {
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type"))), "application/json") {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":       "InvalidParameter",
					"message":    "Content-Type must be application/json",
					"request_id": c.GetString(common.RequestIdKey),
				})
				return
			}
			if !strings.EqualFold(strings.TrimSpace(c.GetHeader("X-DashScope-Async")), "enable") {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":       "InvalidParameter",
					"message":    "X-DashScope-Async must be enable",
					"request_id": c.GetString(common.RequestIdKey),
				})
				return
			}
			var originalReq map[string]interface{}
			if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":       "InvalidParameter",
					"message":    "Invalid DashScope video request body",
					"request_id": c.GetString(common.RequestIdKey),
				})
				return
			}
			c.Set("ali_video_original_request", originalReq)
		}
		c.Next()
	}
}
