// Package router 定义 HTTP 路由和 API 端点
package router

import (
	"fkteams/server/handler"
	"fkteams/server/middleware"
	"fkteams/web"
	"net/http"

	"github.com/gin-gonic/gin"
)

// newEngine 创建带公共中间件的 Gin 引擎
func newEngine() *gin.Engine {
	r := gin.New()
	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.Cors(),
		middleware.MaxBodySize(100<<20), // 100MB
	)
	if handler.AuthEnabled() {
		r.Use(middleware.Auth())
	}
	return r
}

// registerAPIRoutes 注册公共 API 路由
func registerAPIRoutes(r *gin.Engine) {
	r.GET("/health", handler.HealthHandler())
	r.GET("/ws", handler.WebSocketHandler())

	apiV1 := r.Group("/api/fkteams")
	{
		if handler.AuthEnabled() {
			apiV1.POST("/login", handler.LoginHandler())
		}
		apiV1.GET("/version", handler.VersionHandler())

		// 智能体 API
		apiV1.GET("/agents", handler.GetAgentsHandler())

		// 聊天 API
		apiV1.POST("/chat", handler.ChatHandler())

		// 文件列表 API
		apiV1.GET("/files", handler.GetFilesHandler())

		// 历史文件管理 API
		apiV1.GET("/history/files", handler.ListHistoryFilesHandler())
		apiV1.GET("/history/files/:filename", handler.LoadHistoryFileHandler())
		apiV1.DELETE("/history/files/:filename", handler.DeleteHistoryFileHandler())
		apiV1.POST("/history/files/rename", handler.RenameHistoryFileHandler())

		// 定时任务管理 API
		apiV1.GET("/schedules", handler.GetScheduleTasksHandler())
		apiV1.POST("/schedules/:id/cancel", handler.CancelScheduleTaskHandler())

		// 长期记忆管理 API
		apiV1.GET("/memory", handler.GetMemoryListHandler())
		apiV1.DELETE("/memory", handler.DeleteMemoryHandler())
		apiV1.POST("/memory/clear", handler.ClearMemoryHandler())
	}
}

// Init 初始化并返回配置好的 Gin 路由引擎（含 Web 界面）
func Init() *gin.Engine {
	r := newEngine()

	webFS := web.GetFS()
	r.StaticFS("/static", http.FS(webFS))

	// 登录页（仅启用认证时注册）
	if handler.AuthEnabled() {
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

	registerAPIRoutes(r)
	return r
}

// InitAPI 初始化纯 API 路由（无 Web 界面）
func InitAPI() *gin.Engine {
	r := newEngine()
	registerAPIRoutes(r)
	return r
}
