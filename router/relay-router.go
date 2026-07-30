package router

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func SetRelayRouter(router *gin.Engine) {
	router.Use(middleware.CORS())
	router.Use(middleware.DecompressRequestMiddleware())
	router.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	router.Use(middleware.StatsMiddleware())
	// https://platform.openai.com/docs/api-reference/introduction
	modelsRouter := router.Group("/v1/models")
	modelsRouter.Use(middleware.RouteTag("relay"))
	modelsRouter.Use(middleware.TokenAuth())
	{
		modelsRouter.GET("", func(c *gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				controller.ListModels(c, constant.ChannelTypeAnthropic)
			case c.GetHeader("x-goog-api-key") != "" || c.Query("key") != "": // 单独的适配
				controller.RetrieveModel(c, constant.ChannelTypeGemini)
			default:
				controller.ListModels(c, constant.ChannelTypeOpenAI)
			}
		})

		modelsRouter.GET("/:model", func(c *gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				controller.RetrieveModel(c, constant.ChannelTypeAnthropic)
			default:
				controller.RetrieveModel(c, constant.ChannelTypeOpenAI)
			}
		})
	}

	geminiRouter := router.Group("/v1beta/models")
	geminiRouter.Use(middleware.RouteTag("relay"))
	geminiRouter.Use(middleware.TokenAuth())
	{
		geminiRouter.GET("", func(c *gin.Context) {
			controller.ListModels(c, constant.ChannelTypeGemini)
		})
	}

	geminiCompatibleRouter := router.Group("/v1beta/openai/models")
	geminiCompatibleRouter.Use(middleware.RouteTag("relay"))
	geminiCompatibleRouter.Use(middleware.TokenAuth())
	{
		geminiCompatibleRouter.GET("", func(c *gin.Context) {
			controller.ListModels(c, constant.ChannelTypeOpenAI)
		})
	}

	playgroundRouter := router.Group("/pg")
	playgroundRouter.Use(middleware.RouteTag("relay"))
	playgroundRouter.Use(middleware.SystemPerformanceCheck())
	playgroundRouter.Use(middleware.UserAuth(), middleware.Distribute())
	{
		playgroundRouter.POST("/chat/completions", controller.Playground)
		playgroundRouter.POST("/images/generations", controller.Playground)
		playgroundRouter.POST("/images/edits", controller.Playground)
		// 语音合成（TTS）：同步返回音频二进制，和 /v1/audio/speech 走同一条
		// relay，只是换成操练场专用路径 + UserAuth。
		playgroundRouter.POST("/audio/speech", controller.Playground)
		// 语音识别（STT，同步）：multipart 上传音频 → 直接返回转写文本，和
		// /v1/audio/transcriptions 走同一条 relay。适用于 whisper 这类同步模型。
		playgroundRouter.POST("/audio/transcriptions", controller.Playground)
		// 语音识别（STT，异步任务）：amux_stt 这类模型单次耗时长，走"提交 →
		// 轮询"任务流（和视频生成一样），避免同步阻塞被网关/上游 504。
		playgroundRouter.POST("/audio/transcriptions/async", controller.PlaygroundTask)
		playgroundRouter.GET("/audio/transcriptions/:task_id", controller.PlaygroundTaskFetch)
		// 视频生成是异步任务：POST 提交 → 返回 task_id；GET 用 task_id 拉
		// 当前状态/结果。对外 /v1/video/generations 的契约一致，只是换成
		// 操练场专用的 /pg/video/generations，使用 UserAuth。
		playgroundRouter.POST("/video/generations", controller.PlaygroundTask)
		playgroundRouter.GET("/video/generations/:task_id", controller.PlaygroundTaskFetch)
	}
	relayV1Router := router.Group("/v1")
	relayV1Router.Use(middleware.RouteTag("relay"))
	relayV1Router.Use(middleware.SystemPerformanceCheck())
	relayV1Router.Use(middleware.TokenAuth())
	relayV1Router.Use(middleware.ModelRequestRateLimit())
	{
		// WebSocket 路由（统一到 Relay）
		wsRouter := relayV1Router.Group("")
		wsRouter.Use(middleware.Distribute())
		wsRouter.GET("/realtime", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIRealtime)
		})
	}
	{
		//http router
		httpRouter := relayV1Router.Group("")
		// 并发限制放在 Distribute 之前：超限时快速失败，不浪费选渠道的开销。
		// 只挂 httpRouter 而不挂 relayV1Router，是为了排除上面的 /realtime —— WebSocket
		// 长会话会持续占槽，与 TTL 兜底回收机制打架（长会话会被误判成僵尸槽位），
		// 要正确支持得加心跳刷新 score，先不做。
		httpRouter.Use(middleware.TokenConcurrencyLimit())
		httpRouter.Use(middleware.Distribute())

		// claude related routes
		httpRouter.POST("/messages", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatClaude)
		})

		// chat related routes
		httpRouter.POST("/completions", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		})
		httpRouter.POST("/chat/completions", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		})

		// response related routes
		httpRouter.POST("/responses", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponses)
		})
		httpRouter.POST("/responses/compact", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponsesCompaction)
		})

		// image related routes
		httpRouter.POST("/edits", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})
		httpRouter.POST("/images/generations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})
		httpRouter.POST("/images/edits", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})

		// embedding related routes
		httpRouter.POST("/embeddings", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatEmbedding)
		})

		// audio related routes
		httpRouter.POST("/audio/transcriptions", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio)
		})
		httpRouter.POST("/audio/translations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio)
		})
		httpRouter.POST("/audio/speech", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio)
		})

		// rerank related routes
		httpRouter.POST("/rerank", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatRerank)
		})

		// gemini relay routes
		httpRouter.POST("/engines/:model/embeddings", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatGemini)
		})
		httpRouter.POST("/models/*path", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatGemini)
		})

		// other relay routes
		httpRouter.POST("/moderations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		})

		// not implemented
		httpRouter.POST("/images/variations", controller.RelayNotImplemented)
		httpRouter.GET("/files", controller.RelayNotImplemented)
		httpRouter.POST("/files", controller.RelayNotImplemented)
		httpRouter.DELETE("/files/:id", controller.RelayNotImplemented)
		httpRouter.GET("/files/:id", controller.RelayNotImplemented)
		httpRouter.GET("/files/:id/content", controller.RelayNotImplemented)
		httpRouter.POST("/fine-tunes", controller.RelayNotImplemented)
		httpRouter.GET("/fine-tunes", controller.RelayNotImplemented)
		httpRouter.GET("/fine-tunes/:id", controller.RelayNotImplemented)
		httpRouter.POST("/fine-tunes/:id/cancel", controller.RelayNotImplemented)
		httpRouter.GET("/fine-tunes/:id/events", controller.RelayNotImplemented)
		httpRouter.DELETE("/models/:model", controller.RelayNotImplemented)
	}

	relayMjRouter := router.Group("/mj")
	relayMjRouter.Use(middleware.RouteTag("relay"))
	relayMjRouter.Use(middleware.SystemPerformanceCheck())
	registerMjRouterGroup(relayMjRouter)

	relayMjModeRouter := router.Group("/:mode/mj")
	relayMjModeRouter.Use(middleware.RouteTag("relay"))
	relayMjModeRouter.Use(middleware.SystemPerformanceCheck())
	registerMjRouterGroup(relayMjModeRouter)
	//relayMjRouter.Use()

	relaySunoRouter := router.Group("/suno")
	relaySunoRouter.Use(middleware.RouteTag("relay"))
	relaySunoRouter.Use(middleware.SystemPerformanceCheck())
	relaySunoRouter.Use(middleware.TokenAuth())
	{
		relaySunoRouter.POST("/submit/:action", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		relaySunoRouter.POST("/fetch", middleware.Distribute(), controller.RelayTaskFetch)
		relaySunoRouter.GET("/fetch/:id", middleware.Distribute(), controller.RelayTaskFetch)
	}

	relayGeminiRouter := router.Group("/v1beta")
	relayGeminiRouter.Use(middleware.RouteTag("relay"))
	relayGeminiRouter.Use(middleware.SystemPerformanceCheck())
	relayGeminiRouter.Use(middleware.TokenAuth())
	relayGeminiRouter.Use(middleware.ModelRequestRateLimit())
	relayGeminiRouter.Use(middleware.TokenConcurrencyLimit())
	relayGeminiRouter.Use(middleware.Distribute())
	{
		// Gemini API 路径格式: /v1beta/models/{model_name}:{action}
		relayGeminiRouter.POST("/models/*path", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatGemini)
		})
	}
}

func registerMjRouterGroup(relayMjRouter *gin.RouterGroup) {
	relayMjRouter.GET("/image/:id", relay.RelayMidjourneyImage)
	relayMjRouter.Use(middleware.TokenAuth())
	{
		relayMjRouter.POST("/submit/action", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/shorten", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/modal", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/imagine", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/change", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/simple-change", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/describe", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/blend", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/edits", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/video", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		//relayMjRouter.POST("/notify", controller.RelayMidjourney)
		relayMjRouter.GET("/task/:id/fetch", middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.GET("/task/:id/image-seed", middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/task/list-by-condition", middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/insight-face/swap", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
		relayMjRouter.POST("/submit/upload-discord-images", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayMidjourney)
	}
}
