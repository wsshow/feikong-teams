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
	"fkteams/internal/app/config"
	"fkteams/internal/runtime/log"
	"fkteams/web"

	"github.com/gin-gonic/gin"
)

const (
	controlBodyLimit  int64 = 16 << 10
	smallJSONLimit    int64 = 256 << 10
	standardJSONLimit int64 = 4 << 20
	contentJSONLimit  int64 = 16 << 20
	chatBodyLimit     int64 = 32 << 20
	chunkUploadLimit  int64 = 65 << 20
	fileUploadLimit   int64 = 101 << 20
	absoluteBodyLimit int64 = 128 << 20
)

// newEngine 创建带公共中间件的 Gin 引擎
func newEngine(_ bool) *gin.Engine {
	r := gin.New()
	if err := r.SetTrustedProxies(config.Get().Server.TrustedProxies); err != nil {
		log.Printf("invalid trusted proxy configuration, ignoring proxy headers: %v", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.Cors(),
		middleware.MaxBodySize(absoluteBodyLimit),
	)
	r.Use(middleware.Auth())
	return r
}

func registerAPIRoutesWithRuntime(r *gin.Engine, _ bool, state *appstate.State, runtime *handler.Runtime) {
	controlBody := middleware.MaxBodySize(controlBodyLimit)
	smallJSONBody := middleware.MaxBodySize(smallJSONLimit)
	standardJSONBody := middleware.MaxBodySize(standardJSONLimit)
	contentJSONBody := middleware.MaxBodySize(contentJSONLimit)
	chatBody := middleware.MaxBodySize(chatBodyLimit)
	chunkUploadBody := middleware.MaxBodySize(chunkUploadLimit)
	fileUploadBody := middleware.MaxBodySize(fileUploadLimit)

	r.GET("/health", handler.HealthHandler())
	r.GET("/live", handler.HealthHandler())
	r.GET("/ready", runtime.ReadinessHandler())
	r.GET("/ws", runtime.WebSocketHandlerWithState(state))

	// OpenAI 兼容 API（独立的 API Key 认证）
	v1 := r.Group("/v1", middleware.APIKeyAuth())
	{
		v1.GET("/models", handler.OpenAIModelsHandler())
		v1.POST("/chat/completions", chatBody, handler.OpenAIChatCompletionsHandler())
	}

	apiV1 := r.Group("/api/fkteams")
	{
		apiV1.POST("/login", controlBody, handler.LoginHandler())
		apiV1.POST("/logout", controlBody, handler.LogoutHandler())
		apiV1.GET("/version", handler.VersionHandler())

		// 智能体 API
		apiV1.GET("/agents", runtime.GetAgentsHandler())

		// 来源图标代理
		apiV1.GET("/favicon", runtime.FaviconHandler())

		// AI 辅助 API
		ai := apiV1.Group("/ai")
		{
			ai.POST("/agents/draft", standardJSONBody, runtime.GenerateAgentDraftsHandler())
			ai.POST("/skills/draft", standardJSONBody, runtime.GenerateSkillDraftHandler())
			ai.POST("/text/rewrite", standardJSONBody, runtime.RewriteTextHandler())
		}

		// 聊天 API
		apiV1.POST("/chat", chatBody, runtime.ChatHandlerWithState(state))

		// 流式任务 API（前端订阅模式，支持断线重连）
		stream := apiV1.Group("/stream")
		{
			stream.POST("/start", chatBody, runtime.StreamStartHandlerWithState(state))
			stream.POST("/steer", chatBody, runtime.StreamSteerHandler())
			stream.GET("/queue/:sessionID", runtime.StreamQueueHandler())
			stream.PATCH("/queue/:sessionID/:queueID", chatBody, runtime.StreamQueueUpdateHandler())
			stream.DELETE("/queue/:sessionID/:queueID", controlBody, runtime.StreamQueueDeleteHandler())
			stream.POST("/queue/:sessionID/:queueID/kind", controlBody, runtime.StreamQueueKindHandler())
			stream.POST("/queue/:sessionID/:queueID/move", controlBody, runtime.StreamQueueMoveHandler())
			stream.POST("/stop/:sessionID", controlBody, runtime.StreamStopHandler())
			stream.GET("/subscribe/:sessionID", runtime.StreamSubscribeHandler())
			stream.GET("/snapshot/:sessionID", runtime.StreamSnapshotHandler())
			stream.GET("/status/:sessionID", runtime.StreamStatusHandler())
			stream.GET("/events/:sessionID", runtime.StreamEventsHandler())
			stream.POST("/approval", smallJSONBody, runtime.StreamApprovalHandler())
			stream.POST("/ask-response", standardJSONBody, runtime.StreamAskResponseHandler())
		}

		// 文件管理 API
		files := apiV1.Group("/files")
		{
			files.GET("", handler.GetFilesHandler())
			files.GET("/search", handler.SearchFilesHandler())
			files.GET("/content", handler.GetFileContentHandler())
			files.PUT("/content", contentJSONBody, handler.SaveFileContentHandler())
			files.GET("/download", handler.DownloadFileHandler())
			files.POST("/download/batch", smallJSONBody, handler.BatchDownloadHandler())
			files.POST("/upload", fileUploadBody, handler.UploadFileHandler())
			files.POST("/upload/chunk", chunkUploadBody, runtime.UploadChunkHandler())
			files.DELETE("", smallJSONBody, handler.DeleteFileHandler())
			files.GET("/serve/*filepath", handler.ServeFileHandler())
		}

		// 文件预览链接 API
		preview := apiV1.Group("/preview")
		{
			preview.POST("", smallJSONBody, runtime.CreatePreviewLinkHandler())
			preview.GET("", runtime.ListPreviewLinksHandler())
			preview.GET("/:linkId", runtime.PreviewFileHandler())
			preview.GET("/:linkId/info", runtime.PreviewInfoHandler())
			preview.POST("/:linkId/auth", controlBody, runtime.PreviewAuthHandler())
			preview.GET("/:linkId/render/*filepath", runtime.PreviewRenderHandler())
			preview.DELETE("/:linkId", controlBody, runtime.DeletePreviewLinkHandler())
		}

		// 会话分享管理 API
		sessionShares := apiV1.Group("/session-shares")
		{
			sessionShares.POST("", smallJSONBody, runtime.CreateSessionShareHandler())
			sessionShares.GET("", runtime.ListSessionSharesHandler())
			sessionShares.DELETE("/:shareID", controlBody, runtime.DeleteSessionShareHandler())
		}

		// 公开会话分享 API
		publicSessionShares := apiV1.Group("/public/session-shares")
		{
			publicSessionShares.GET("/:shareID/info", runtime.GetPublicSessionShareInfoHandler())
			publicSessionShares.POST("/:shareID/access", controlBody, runtime.AccessPublicSessionShareHandler())
		}

		// 会话管理 API
		sessions := apiV1.Group("/sessions")
		{
			sessions.GET("", runtime.ListSessionsHandler())
			sessions.POST("", smallJSONBody, runtime.CreateSessionHandler())
			sessions.GET("/:sessionID", runtime.GetSessionHandler())
			sessions.PATCH("/:sessionID", smallJSONBody, runtime.UpdateSessionHandler())
			sessions.DELETE("/:sessionID", controlBody, runtime.DeleteSessionHandler())
			sessions.POST("/rename", smallJSONBody, runtime.RenameSessionHandler())
			sessions.POST("/favorite", controlBody, runtime.FavoriteSessionHandler())
			sessions.POST("/agent", smallJSONBody, runtime.UpdateSessionAgentHandler())
		}

		// 定时任务管理 API
		schedules := apiV1.Group("/schedules")
		{
			schedules.GET("", runtime.GetScheduleTasksHandler())
			schedules.POST("", standardJSONBody, runtime.CreateScheduleTaskHandler())
			schedules.PUT("/:id", standardJSONBody, runtime.UpdateScheduleTaskHandler())
			schedules.DELETE("/:id", controlBody, runtime.DeleteScheduleTaskHandler())
			schedules.POST("/:id/cancel", controlBody, runtime.CancelScheduleTaskHandler())
			schedules.GET("/:id/result", runtime.GetTaskResultHandler())
			schedules.GET("/:id/history", runtime.GetTaskHistoryHandler())
			schedules.GET("/:id/history/:filename", runtime.GetTaskHistoryFileHandler())
		}

		// 技能管理 API
		skills := apiV1.Group("/skills")
		{
			skills.GET("", handler.GetInstalledSkillsHandler())
			skills.POST("", standardJSONBody, handler.CreateSkillHandler())
			skills.GET("/search", runtime.SearchSkillsHandler())
			skills.POST("/install", smallJSONBody, runtime.InstallSkillHandler())
			skills.DELETE("/:slug", controlBody, handler.RemoveSkillHandler())
			skills.GET("/:slug/files", handler.GetSkillFilesHandler())
			skills.POST("/:slug/files", standardJSONBody, handler.CreateSkillFileHandler())
			skills.GET("/:slug/file", handler.GetSkillFileContentHandler())
			skills.PUT("/:slug/file", standardJSONBody, handler.SaveSkillFileContentHandler())
			skills.DELETE("/:slug/file", controlBody, handler.DeleteSkillFileHandler())
		}

		// 长期记忆管理 API
		memory := apiV1.Group("/memory")
		{
			memory.GET("", handler.GetMemoryListHandlerWithState(state))
			memory.DELETE("", smallJSONBody, handler.DeleteMemoryHandlerWithState(state))
			memory.POST("/clear", controlBody, handler.ClearMemoryHandlerWithState(state))
		}

		// 配置管理 API
		configGroup := apiV1.Group("/config")
		{
			configGroup.GET("", handler.GetConfigHandler())
			configGroup.PUT("", standardJSONBody, runtime.UpdateConfigHandlerWithState(state))
			configGroup.GET("/tools", runtime.GetToolNamesHandler())
			configGroup.GET("/tool-catalog", runtime.GetToolCatalogHandler())
			configGroup.GET("/template-vars", handler.GetTemplateVarsHandler())
		}

		// 模型提供者 API
		apiV1.GET("/providers", runtime.GetProvidersHandler())
		apiV1.POST("/providers/models", smallJSONBody, runtime.GetProviderModelsHandler())

		// 系统管理 API
		apiV1.POST("/shutdown", controlBody, handler.ShutdownHandler())
		apiV1.POST("/restart", controlBody, handler.RestartHandler())
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
	if err := runtime.InitializationError(); err != nil {
		return nil, fmt.Errorf("initialize HTTP runtime: %w", err)
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

	serveLogin := func(c *gin.Context) {
		serveHTML(c, webFS)
	}
	r.GET("/login", serveLogin)

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
	if err := runtime.InitializationError(); err != nil {
		return nil, fmt.Errorf("initialize HTTP runtime: %w", err)
	}

	r := newEngine(authEnabled)
	registerAPIRoutesWithRuntime(r, authEnabled, state, runtime)
	return r, nil
}
