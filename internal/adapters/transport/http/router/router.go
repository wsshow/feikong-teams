// Package router 定义 HTTP 路由和 API 端点
package router

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/adapters/transport/http/middleware"
	"fkteams/internal/app/appstate"
	"fkteams/web"

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
	registerAPIRoutesWithRuntime(r, authEnabled, nil, handler.NewRuntime())
}

// registerAPIRoutesWithState 注册带应用状态的公共 API 路由。
func registerAPIRoutesWithState(r *gin.Engine, authEnabled bool, state *appstate.State) {
	registerAPIRoutesWithRuntime(r, authEnabled, state, handler.NewRuntime())
}

func registerAPIRoutesWithRuntime(r *gin.Engine, authEnabled bool, state *appstate.State, runtime *handler.Runtime) {
	r.GET("/health", handler.HealthHandler())
	r.GET("/ws", runtime.WebSocketHandlerWithState(state))

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
		apiV1.GET("/agents", runtime.GetAgentsHandler())

		// 来源图标代理
		apiV1.GET("/favicon", runtime.FaviconHandler())

		// AI 辅助 API
		ai := apiV1.Group("/ai")
		{
			ai.POST("/agents/draft", runtime.GenerateAgentDraftsHandler())
			ai.POST("/text/rewrite", runtime.RewriteTextHandler())
		}

		// 聊天 API
		apiV1.POST("/chat", runtime.ChatHandlerWithState(state))

		// 流式任务 API（前端订阅模式，支持断线重连）
		stream := apiV1.Group("/stream")
		{
			stream.POST("/start", runtime.StreamStartHandlerWithState(state))
			stream.POST("/steer", runtime.StreamSteerHandler())
			stream.GET("/queue/:sessionID", runtime.StreamQueueHandler())
			stream.PATCH("/queue/:sessionID/:queueID", runtime.StreamQueueUpdateHandler())
			stream.DELETE("/queue/:sessionID/:queueID", runtime.StreamQueueDeleteHandler())
			stream.POST("/queue/:sessionID/:queueID/kind", runtime.StreamQueueKindHandler())
			stream.POST("/queue/:sessionID/:queueID/move", runtime.StreamQueueMoveHandler())
			stream.POST("/stop/:sessionID", runtime.StreamStopHandler())
			stream.GET("/subscribe/:sessionID", runtime.StreamSubscribeHandler())
			stream.GET("/snapshot/:sessionID", runtime.StreamSnapshotHandler())
			stream.GET("/status/:sessionID", runtime.StreamStatusHandler())
			stream.GET("/events/:sessionID", runtime.StreamEventsHandler())
			stream.POST("/approval", runtime.StreamApprovalHandler())
			stream.POST("/ask-response", runtime.StreamAskResponseHandler())
		}

		// 文件管理 API
		files := apiV1.Group("/files")
		{
			files.GET("", handler.GetFilesHandler())
			files.GET("/search", handler.SearchFilesHandler())
			files.GET("/content", handler.GetFileContentHandler())
			files.PUT("/content", handler.SaveFileContentHandler())
			files.GET("/download", handler.DownloadFileHandler())
			files.POST("/download/batch", handler.BatchDownloadHandler())
			files.POST("/upload", handler.UploadFileHandler())
			files.POST("/upload/chunk", runtime.UploadChunkHandler())
			files.DELETE("", handler.DeleteFileHandler())
			files.GET("/serve/*filepath", handler.ServeFileHandler())
		}

		// 文件预览链接 API
		preview := apiV1.Group("/preview")
		{
			preview.POST("", runtime.CreatePreviewLinkHandler())
			preview.GET("", runtime.ListPreviewLinksHandler())
			preview.GET("/:linkId", runtime.PreviewFileHandler())
			preview.GET("/:linkId/info", runtime.PreviewInfoHandler())
			preview.GET("/:linkId/render/*filepath", runtime.PreviewRenderHandler())
			preview.DELETE("/:linkId", runtime.DeletePreviewLinkHandler())
		}

		// 会话分享管理 API
		sessionShares := apiV1.Group("/session-shares")
		{
			sessionShares.POST("", runtime.CreateSessionShareHandler())
			sessionShares.GET("", runtime.ListSessionSharesHandler())
			sessionShares.DELETE("/:shareID", runtime.DeleteSessionShareHandler())
		}

		// 公开会话分享 API
		publicSessionShares := apiV1.Group("/public/session-shares")
		{
			publicSessionShares.GET("/:shareID/info", runtime.GetPublicSessionShareInfoHandler())
			publicSessionShares.POST("/:shareID/access", runtime.AccessPublicSessionShareHandler())
		}

		// 会话管理 API
		sessions := apiV1.Group("/sessions")
		{
			sessions.GET("", runtime.ListSessionsHandler())
			sessions.POST("", runtime.CreateSessionHandler())
			sessions.GET("/:sessionID", runtime.GetSessionHandler())
			sessions.DELETE("/:sessionID", runtime.DeleteSessionHandler())
			sessions.POST("/rename", runtime.RenameSessionHandler())
			sessions.POST("/favorite", runtime.FavoriteSessionHandler())
			sessions.POST("/agent", runtime.UpdateSessionAgentHandler())
		}

		// 定时任务管理 API
		schedules := apiV1.Group("/schedules")
		{
			schedules.GET("", runtime.GetScheduleTasksHandler())
			schedules.POST("", runtime.CreateScheduleTaskHandler())
			schedules.PUT("/:id", runtime.UpdateScheduleTaskHandler())
			schedules.DELETE("/:id", runtime.DeleteScheduleTaskHandler())
			schedules.POST("/:id/cancel", runtime.CancelScheduleTaskHandler())
			schedules.GET("/:id/result", runtime.GetTaskResultHandler())
			schedules.GET("/:id/history", runtime.GetTaskHistoryHandler())
			schedules.GET("/:id/history/:filename", runtime.GetTaskHistoryFileHandler())
		}

		// 技能管理 API
		skills := apiV1.Group("/skills")
		{
			skills.GET("", handler.GetInstalledSkillsHandler())
			skills.GET("/search", runtime.SearchSkillsHandler())
			skills.POST("/install", runtime.InstallSkillHandler())
			skills.DELETE("/:slug", handler.RemoveSkillHandler())
			skills.GET("/:slug/files", handler.GetSkillFilesHandler())
			skills.GET("/:slug/file", handler.GetSkillFileContentHandler())
		}

		// 长期记忆管理 API
		memory := apiV1.Group("/memory")
		{
			memory.GET("", handler.GetMemoryListHandlerWithState(state))
			memory.DELETE("", handler.DeleteMemoryHandlerWithState(state))
			memory.POST("/clear", handler.ClearMemoryHandlerWithState(state))
		}

		// 配置管理 API
		configGroup := apiV1.Group("/config")
		{
			configGroup.GET("", handler.GetConfigHandler())
			configGroup.PUT("", runtime.UpdateConfigHandlerWithState(state))
			configGroup.GET("/tools", runtime.GetToolNamesHandler())
			configGroup.GET("/tool-catalog", runtime.GetToolCatalogHandler())
			configGroup.GET("/template-vars", handler.GetTemplateVarsHandler())
		}

		// 模型提供者 API
		apiV1.GET("/providers", runtime.GetProvidersHandler())
		apiV1.POST("/providers/models", runtime.GetProviderModelsHandler())

		// 系统管理 API
		apiV1.POST("/shutdown", handler.ShutdownHandler())
		apiV1.POST("/restart", handler.RestartHandler())
	}
}

// Init 初始化并返回配置好的 Gin 路由引擎（含 Web 界面）
func Init() (*gin.Engine, error) {
	return InitWithState(nil)
}

// InitWithState 初始化并返回带应用状态的 Gin 路由引擎（含 Web 界面）。
func InitWithState(state *appstate.State) (*gin.Engine, error) {
	return InitWithRuntime(state, handler.NewRuntime())
}

// InitWithRuntime 初始化并返回带显式 HTTP runtime 的 Gin 路由引擎（含 Web 界面）。
func InitWithRuntime(state *appstate.State, runtime *handler.Runtime) (*gin.Engine, error) {
	authEnabled, err := handler.AuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("check auth config: %w", err)
	}
	if runtime == nil {
		runtime = handler.NewRuntime()
	}

	r := newEngine(authEnabled)

	webFS := web.GetFS()
	r.GET("/assets/*filepath", serveAssets(webFS))
	r.GET("/favicon.ico", func(c *gin.Context) {
		data, err := webFS.Open("assets/favicon.ico")
		if err != nil {
			c.String(http.StatusNotFound, "favicon not found")
			return
		}
		defer data.Close()
		c.Header("Cache-Control", "public, max-age=86400")
		c.DataFromReader(http.StatusOK, -1, "image/x-icon", data, nil)
	})

	if authEnabled {
		serveLogin := func(c *gin.Context) {
			serveHTML(c, webFS)
		}
		r.GET("/login", serveLogin)
	}

	serveIndex := func(c *gin.Context) {
		serveHTML(c, webFS)
	}
	r.GET("/", serveIndex)
	r.GET("/chat", serveIndex)
	r.GET("/chat/:sessionID", serveIndex)
	r.GET("/config", serveIndex)
	r.GET("/files", serveIndex)
	r.GET("/schedules", serveIndex)
	r.GET("/skills", serveIndex)

	// 文件分享预览页面
	servePreview := func(c *gin.Context) {
		serveHTML(c, webFS)
	}
	r.GET("/p/:linkId", servePreview)
	r.GET("/s/:shareID", func(c *gin.Context) {
		serveHTML(c, webFS)
	})

	registerAPIRoutesWithRuntime(r, authEnabled, state, runtime)
	r.NoRoute(spaFallback(webFS))
	return r, nil
}

func serveAssets(webFS fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(webFS))
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func serveHTML(c *gin.Context, webFS fs.FS) {
	data, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		c.String(http.StatusNotFound, "Page not found")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func spaFallback(webFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldServeSPA(c.Request) {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		serveHTML(c, webFS)
	}
}

func shouldServeSPA(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}

	requestPath := request.URL.Path
	if requestPath == "/ws" ||
		strings.HasPrefix(requestPath, "/api/") ||
		strings.HasPrefix(requestPath, "/v1/") ||
		strings.HasPrefix(requestPath, "/assets/") {
		return false
	}

	return path.Ext(path.Base(requestPath)) == ""
}

// InitAPI 初始化纯 API 路由（无 Web 界面）
func InitAPI() (*gin.Engine, error) {
	return InitAPIWithState(nil)
}

// InitAPIWithState 初始化带应用状态的纯 API 路由。
func InitAPIWithState(state *appstate.State) (*gin.Engine, error) {
	return InitAPIWithRuntime(state, handler.NewRuntime())
}

// InitAPIWithRuntime 初始化带显式 HTTP runtime 的纯 API 路由。
func InitAPIWithRuntime(state *appstate.State, runtime *handler.Runtime) (*gin.Engine, error) {
	authEnabled, err := handler.AuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("check auth config: %w", err)
	}
	if runtime == nil {
		runtime = handler.NewRuntime()
	}

	r := newEngine(authEnabled)
	registerAPIRoutesWithRuntime(r, authEnabled, state, runtime)
	return r, nil
}
