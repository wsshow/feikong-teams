package router

import (
	"fkteams/server/handler"
	"fkteams/server/middleware"
	"fkteams/web"
	"net/http"

	"github.com/gin-gonic/gin"
)

func Init() *gin.Engine {
	r := gin.New()
	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.Cors(),
	)

	// 使用嵌入的静态文件服务
	webFS := web.GetFS()
	r.StaticFS("/static", http.FS(webFS))

	// 首页路由 - 使用嵌入的 index.html
	r.GET("/", func(c *gin.Context) {
		data, err := webFS.Open("index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		defer data.Close()
		c.DataFromReader(http.StatusOK, -1, "text/html; charset=utf-8", data, nil)
	})

	r.GET("/chat", func(c *gin.Context) {
		data, err := webFS.Open("index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		defer data.Close()
		c.DataFromReader(http.StatusOK, -1, "text/html; charset=utf-8", data, nil)
	})

	// WebSocket 路由
	r.GET("/ws", handler.WebSocketHandler())

	apiV1 := r.Group("/api/fkteams")
	{
		apiV1.GET("/version", handler.VersionHandler())

		// 历史文件管理 API
		apiV1.GET("/history/files", handler.ListHistoryFilesHandler())
		apiV1.GET("/history/files/:filename", handler.LoadHistoryFileHandler())
		apiV1.DELETE("/history/files/:filename", handler.DeleteHistoryFileHandler())
		apiV1.POST("/history/files/rename", handler.RenameHistoryFileHandler())
	}
	return r
}
