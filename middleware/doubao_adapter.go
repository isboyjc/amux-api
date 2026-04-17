package middleware

import (
	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// DoubaoRequestConvert handles Doubao official API format requests
// Marks the request as raw format without conversion
func DoubaoRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// Check if it's Doubao raw format (has content array but no prompt)
		_, hasContent := originalReq["content"]
		_, hasPrompt := originalReq["prompt"]

		if hasContent && !hasPrompt {
			// Mark as Doubao raw format, pass through without conversion
			c.Set("doubao_raw_format", true)
			c.Set("doubao_original_request", originalReq)
		}

		c.Next()
	}
}
