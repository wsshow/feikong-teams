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

	authEnabled := handler.AuthEnabled()
	if authEnabled {
		r.Use(middleware.Auth())
	}

	webFS := web.GetFS()
	r.StaticFS("/static", http.FS(webFS))

	// 登录页（仅启用认证时注册）
	if authEnabled {
		serveLogin := func(c *gin.Context) {
			data, err := webFS.Open("login.html")
			if err != nil {
				c.String(http.StatusNotFound, "Page not found")
				return
			}
			defer data.Close()
			c.DataFromReader(http.StatusOK, -1, "text/html; charset=utf-8", data, nil)
		}
		r.GET("/login", serveLogin)
	}

	// 首页
	serveIndex := func(c *gin.Context) {
		data, err := webFS.Open("index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		defer data.Close()
		c.DataFromReader(http.StatusOK, -1, "text/html; charset=utf-8", data, nil)
	}
	r.GET("/", serveIndex)
	r.GET("/chat", serveIndex)

	r.GET("/ws", handler.WebSocketHandler())

	apiV1 := r.Group("/api/fkteams")
	{
		if authEnabled {
			apiV1.POST("/login", handler.LoginHandler())
		}
		apiV1.GET("/version", handler.VersionHandler())

		// 智能体 API
		apiV1.GET("/agents", handler.GetAgentsHandler())

		// 文件列表 API
		apiV1.GET("/files", handler.GetFilesHandler())

		// 历史文件管理 API
		apiV1.GET("/history/files", handler.ListHistoryFilesHandler())
		apiV1.GET("/history/files/:filename", handler.LoadHistoryFileHandler())
		apiV1.DELETE("/history/files/:filename", handler.DeleteHistoryFileHandler())
		apiV1.POST("/history/files/rename", handler.RenameHistoryFileHandler())
	}
	return r
}
