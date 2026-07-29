package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	// 并发限制必须跑在 Distribute 之前：Distribute 会选渠道，并推进多 Key 渠道的
	// 轮询游标（GetNextEnabledKey 里的 MultiKeyPollingIndex），被 429 拒掉的请求
	// 不应该产生这个副作用。gin 的执行顺序是 group 中间件 → route 中间件，所以
	// Distribute 不能留在 Use() 里，得跟在每条路由的并发限制之后。
	videoV1Router.Use(middleware.TokenAuth())
	{
		videoV1Router.POST("/video/generations", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth())
	{
		klingV1Router.POST("/videos/text2video", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		klingV1Router.POST("/videos/image2video", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
	}

	// Doubao official API routes - direct mapping to official API format
	doubaoV3Router := router.Group("/api/v3/contents/generations")
	doubaoV3Router.Use(middleware.RouteTag("relay"))
	doubaoV3Router.Use(middleware.DoubaoRequestConvert(), middleware.TokenAuth())
	{
		doubaoV3Router.POST("/tasks", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		doubaoV3Router.GET("/tasks/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
	}
}
