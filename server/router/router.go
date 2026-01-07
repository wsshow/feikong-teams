package router

import (
	"fkteams/server/handler"
	"fkteams/server/middleware"

	"github.com/gin-gonic/gin"
)

func Init() *gin.Engine {
	r := gin.New()
	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.Cors(),
	)
	apiV1 := r.Group("/api/fkteams")
	{
		apiV1.GET("/version", handler.VersionHandler())
		apiV1.POST("/roundtable", handler.RoundtableHandler())
		apiV1.POST("/supervisor", handler.SupervisorHandler())
	}
	return r
}
