// Package router 定义 HTTP 路由和 API 端点
package router

import (
	"fkteams/server/handler"
	"fkteams/server/middleware"
	"fkteams/web"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// newEngine 创建带公共中间件的 Gin 引擎
func newEngine(authEnabled bool) *gin.Engine {
	r := gin.New()
	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.Cors(),
		middleware.MaxBodySize(100<<20), // 100MB
	)
	if authEnabled {
		r.Use(middleware.Auth())
	}
	return r
}

// registerAPIRoutes 注册公共 API 路由
func registerAPIRoutes(r *gin.Engine, authEnabled bool) {
	r.GET("/health", handler.HealthHandler())
	r.GET("/ws", handler.WebSocketHandler())

	apiV1 := r.Group("/api/fkteams")
	{
		if authEnabled {
			apiV1.POST("/login", handler.LoginHandler())
		}
		apiV1.GET("/version", handler.VersionHandler())

		// 智能体 API
		apiV1.GET("/agents", handler.GetAgentsHandler())

		// 聊天 API
		apiV1.POST("/chat", handler.ChatHandler())

		// 文件列表 API
		apiV1.GET("/files", handler.GetFilesHandler())

		// 文件上传 API
		apiV1.POST("/files/upload", handler.UploadFileHandler())

		// 文件分片上传 API
		apiV1.POST("/files/upload/chunk", handler.UploadChunkHandler())

		// 会话管理 API
		apiV1.GET("/sessions", handler.ListSessionsHandler())
		apiV1.POST("/sessions", handler.CreateSessionHandler())
		apiV1.GET("/sessions/:sessionID", handler.GetSessionHandler())
		apiV1.DELETE("/sessions/:sessionID", handler.DeleteSessionHandler())
		apiV1.POST("/sessions/rename", handler.RenameSessionHandler())

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
func Init() (*gin.Engine, error) {
	authEnabled, err := handler.AuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("check auth config: %w", err)
	}

	r := newEngine(authEnabled)

	webFS := web.GetFS()
	r.StaticFS("/static", http.FS(webFS))

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

	registerAPIRoutes(r, authEnabled)
	return r, nil
}

// InitAPI 初始化纯 API 路由（无 Web 界面）
func InitAPI() (*gin.Engine, error) {
	authEnabled, err := handler.AuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("check auth config: %w", err)
	}

	r := newEngine(authEnabled)
	registerAPIRoutes(r, authEnabled)
	return r, nil
}
