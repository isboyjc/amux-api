package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/service/storage"

	"github.com/gin-gonic/gin"
)

// uploadScopeRule 描述一个允许的 scope：
//
//   - pathPrefix: 服务端硬编码的 R2 key 前缀（业务域/子类层级）。**用户绝不
//     能填路径**，即便对外开放上传 API 也只能传 scope，不允许传 path
//   - maxBytes: 该 scope 单文件大小上限（presign 时校验，并签进 URL）
//   - contentPrefixes: 接受的 MIME 前缀白名单；空数组表示不限（用于通用 file）
type uploadScopeRule struct {
	pathPrefix      string
	maxBytes        int64
	contentPrefixes []string
}

const mb = 1024 * 1024

// allowedUploadScopes 服务端硬编码的 scope 白名单。前端只能从这里选；
// 非白名单的 scope 直接 400。
//
// 命名规约（外部 API 看到的 scope）：{产品域}-{子类}-{资源类型}
// 实际落到 R2 的目录（pathPrefix）：{产品域}/{子类}/{资源类型}
//
// R2 目录结构：
//
//	playground/video/{image,video,audio}/{userID}/{YYYY/MM/DD}/{uuid}.ext
//	playground/image/reference/{userID}/{YYYY/MM/DD}/{uuid}.ext
//	user-upload/{image,video,audio,file}/{userID}/{YYYY/MM/DD}/{uuid}.ext
//
// playground/* 给操练场内部用；user-upload/* 留给"对外统一上传 API"——日后
// 暴露给 API key 用户时，复用同一份 presign 逻辑，路径自带按用户隔离。
var allowedUploadScopes = map[string]uploadScopeRule{
	"playground-video-image": {
		pathPrefix:      "playground/video/image",
		maxBytes:        30 * mb,
		contentPrefixes: []string{"image/"},
	},
	"playground-video-video": {
		// 视频普遍较大，给 200MB；上游模型一般也只接受这个量级以下
		pathPrefix:      "playground/video/video",
		maxBytes:        200 * mb,
		contentPrefixes: []string{"video/"},
	},
	"playground-video-audio": {
		pathPrefix:      "playground/video/audio",
		maxBytes:        30 * mb,
		contentPrefixes: []string{"audio/"},
	},
	"playground-image-reference": {
		pathPrefix:      "playground/image/reference",
		maxBytes:        30 * mb,
		contentPrefixes: []string{"image/"},
	},

	// ----- 对外统一上传（路由暴露后即可被 API key / 平台用户使用）-----
	"user-upload-image": {
		pathPrefix:      "user-upload/image",
		maxBytes:        30 * mb,
		contentPrefixes: []string{"image/"},
	},
	"user-upload-video": {
		pathPrefix:      "user-upload/video",
		maxBytes:        200 * mb,
		contentPrefixes: []string{"video/"},
	},
	"user-upload-audio": {
		pathPrefix:      "user-upload/audio",
		maxBytes:        30 * mb,
		contentPrefixes: []string{"audio/"},
	},
	"user-upload-file": {
		// 通用文件兜底：不限 MIME；仅限大小（避免被滥用为大文件分发）
		pathPrefix:      "user-upload/file",
		maxBytes:        50 * mb,
		contentPrefixes: nil,
	},
}

// presignUploadRequest /api/upload/presign 请求体。所有字段都来自客户端，
// 服务端必须独立做白名单校验——尤其是 size / content_type 决定了能上传什么。
type presignUploadRequest struct {
	Scope       string `json:"scope" binding:"required"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size" binding:"required"`
	ContentType string `json:"content_type"`
}

// PresignUpload 通用预签名上传入口。客户端先 POST 这个接口拿一份 PUT 预签名
// URL，然后浏览器（或 SDK）直接 PUT 到 R2，文件流不经过 amux-api。
//
// 安全要点：
//   - scope 决定 pathPrefix 和限制，由服务端写死的白名单查表；客户端没法
//     越权写到别的目录
//   - size 和 content_type 都签进 URL：客户端实际 PUT 时如果改了任意一个，
//     R2 会拒签；不需要 /complete 二次校验
//   - 文件名只用于推断扩展名，最终 key 用 uuid 重命名，避免特殊字符 / 撞名
func PresignUpload(c *gin.Context) {
	if !storage.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "对象存储未启用，请联系管理员配置 R2_*",
		})
		return
	}

	var req presignUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求体非法：" + err.Error(),
		})
		return
	}

	scope := strings.TrimSpace(req.Scope)
	rule, ok := allowedUploadScopes[scope]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "scope 非法或未注册",
		})
		return
	}

	if req.Size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "size 必须大于 0",
		})
		return
	}
	if req.Size > rule.maxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"success": false,
			"message": "文件超出该 scope 允许的最大体积",
		})
		return
	}

	contentType := strings.TrimSpace(req.ContentType)
	if len(rule.contentPrefixes) > 0 {
		if !matchesAnyPrefix(contentType, rule.contentPrefixes) {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"success": false,
				"message": "该 scope 不接受此 Content-Type：" + contentType,
			})
			return
		}
	}

	userID := c.GetInt("id")
	res, err := storage.PresignPut(
		c.Request.Context(),
		rule.pathPrefix,
		userID,
		req.Filename,
		contentType,
		req.Size,
		0, // ttl 走默认（5min）
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "签名上传 URL 失败：" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    res,
	})
}

// TestUploadConnection 管理员"测试连接"入口。不依赖总开关——开关关着也能
// 测，让管理员在打开开关前先确认配置正确。
//
// 路由：POST /api/upload/test，AdminAuth 保护。
//
// 实际逻辑全部委托给 storage.TestConnection；这里只做鉴权 + 把结果原样下发。
// 即使端到端测试失败，HTTP 仍然返回 200（business success）——因为"测试
// 失败"是业务结果而非系统错误，前端要展示每一步细节，不该被全局错误处理
// 拦截。失败/成功靠响应体里的 success 字段区分。
func TestUploadConnection(c *gin.Context) {
	if !storage.CredsReady() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "凭证不完整，先把 access_key / secret / bucket / public_base_url 填齐再测试",
			"data": gin.H{
				"success": false,
				"steps": []gin.H{
					{"name": "config", "ok": false, "message": "凭证不完整"},
				},
			},
		})
		return
	}
	userID := c.GetInt("id")
	res := storage.TestConnection(c.Request.Context(), userID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    res,
	})
}

func matchesAnyPrefix(contentType string, prefixes []string) bool {
	if contentType == "" || len(prefixes) == 0 {
		return false
	}
	ct := strings.ToLower(contentType)
	for _, p := range prefixes {
		if strings.HasPrefix(ct, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
