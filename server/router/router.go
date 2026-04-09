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

	// OpenAI 兼容 API（独立的 API Key 认证）
	v1 := r.Group("/v1", middleware.APIKeyAuth())
	{
		v1.GET("/models", handler.OpenAIModelsHandler())
		v1.POST("/chat/completions", handler.OpenAIChatCompletionsHandler())
	}

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

		// 流式任务 API（前端订阅模式，支持断线重连）
		stream := apiV1.Group("/stream")
		{
			stream.POST("/start", handler.StreamStartHandler())
			stream.POST("/stop/:sessionID", handler.StreamStopHandler())
			stream.GET("/subscribe/:sessionID", handler.StreamSubscribeHandler())
			stream.GET("/status/:sessionID", handler.StreamStatusHandler())
			stream.GET("/events/:sessionID", handler.StreamEventsHandler())
			stream.POST("/approval", handler.StreamApprovalHandler())
			stream.POST("/ask-response", handler.StreamAskResponseHandler())
		}

		// 文件管理 API
		files := apiV1.Group("/files")
		{
			files.GET("", handler.GetFilesHandler())
			files.GET("/search", handler.SearchFilesHandler())
			files.GET("/download", handler.DownloadFileHandler())
			files.POST("/download/batch", handler.BatchDownloadHandler())
			files.POST("/upload", handler.UploadFileHandler())
			files.POST("/upload/chunk", handler.UploadChunkHandler())
			files.DELETE("", handler.DeleteFileHandler())
			files.GET("/serve/*filepath", handler.ServeFileHandler())
		}

		// 文件预览链接 API
		preview := apiV1.Group("/preview")
		{
			preview.POST("", handler.CreatePreviewLinkHandler())
			preview.GET("", handler.ListPreviewLinksHandler())
			preview.GET("/:linkId", handler.PreviewFileHandler())
			preview.GET("/:linkId/info", handler.PreviewInfoHandler())
			preview.GET("/:linkId/render/*filepath", handler.PreviewRenderHandler())
			preview.DELETE("/:linkId", handler.DeletePreviewLinkHandler())
		}

		// 会话管理 API
		sessions := apiV1.Group("/sessions")
		{
			sessions.GET("", handler.ListSessionsHandler())
			sessions.POST("", handler.CreateSessionHandler())
			sessions.GET("/:sessionID", handler.GetSessionHandler())
			sessions.DELETE("/:sessionID", handler.DeleteSessionHandler())
			sessions.POST("/rename", handler.RenameSessionHandler())
		}

		// 定时任务管理 API
		schedules := apiV1.Group("/schedules")
		{
			schedules.GET("", handler.GetScheduleTasksHandler())
			schedules.POST("/:id/cancel", handler.CancelScheduleTaskHandler())
		}

		// 长期记忆管理 API
		memory := apiV1.Group("/memory")
		{
			memory.GET("", handler.GetMemoryListHandler())
			memory.DELETE("", handler.DeleteMemoryHandler())
			memory.POST("/clear", handler.ClearMemoryHandler())
		}

		// 配置管理 API
		configGroup := apiV1.Group("/config")
		{
			configGroup.GET("", handler.GetConfigHandler())
			configGroup.PUT("", handler.UpdateConfigHandler())
			configGroup.GET("/tools", handler.GetToolNamesHandler())
			configGroup.GET("/template-vars", handler.GetTemplateVarsHandler())
		}

		// 模型提供者 API
		apiV1.GET("/providers", handler.GetProvidersHandler())
		apiV1.POST("/providers/models", handler.GetProviderModelsHandler())

		// 系统管理 API
		apiV1.POST("/shutdown", handler.ShutdownHandler())
		apiV1.POST("/restart", handler.RestartHandler())
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

	// 文件分享预览页面
	servePreview := func(c *gin.Context) {
		data, err := webFS.Open("preview.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		defer data.Close()
		c.DataFromReader(http.StatusOK, -1, "text/html; charset=utf-8", data, nil)
	}
	r.GET("/p/:linkId", servePreview)

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
