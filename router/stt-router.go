package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetSTTRouter(router *gin.Engine) {
	sttV1Router := router.Group("/v1")
	sttV1Router.Use(middleware.RouteTag("relay"))
	// 并发限制需早于 Distribute（选渠道 + 推进多 Key 轮询游标），详见 video-router.go
	sttV1Router.Use(middleware.TokenAuth())
	{
		sttV1Router.POST("/audio/transcriptions/async", middleware.TokenConcurrencyLimit(), middleware.Distribute(), controller.RelayTask)
		sttV1Router.GET("/audio/transcriptions/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
	}
}
