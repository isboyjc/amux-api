package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetSTTRouter(router *gin.Engine) {
	sttV1Router := router.Group("/v1")
	sttV1Router.Use(middleware.RouteTag("relay"))
	sttV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		sttV1Router.POST("/audio/transcriptions/async", controller.RelayTask)
		sttV1Router.GET("/audio/transcriptions/:task_id", controller.RelayTaskFetch)
	}
}
