// Package router 定义 HTTP 路由和 API 端点
package router

import (
	"fkteams/internal/app/appstate"
	"fkteams/server/handler"
	"fkteams/server/middleware"
	"fkteams/version"
	"fkteams/web"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	staticAssetRefRe = regexp.MustCompile(`(/static/(?:assets|css|js)/[^"'<>?\s)]+)(?:\?[^"'<>\s)]*)?`)
	cssImportURLRe   = regexp.MustCompile(`url\((['"]?)([^'")]+\.css)(?:\?[^'")]*)?(['"]?)\)`)
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
	registerAPIRoutesWithState(r, authEnabled, nil)
}

// registerAPIRoutesWithState 注册带应用状态的公共 API 路由。
func registerAPIRoutesWithState(r *gin.Engine, authEnabled bool, state *appstate.State) {
	r.GET("/health", handler.HealthHandler())
	r.GET("/ws", handler.WebSocketHandlerWithState(state))

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

		// 来源图标代理
		apiV1.GET("/favicon", handler.FaviconHandler())

		// 聊天 API
		apiV1.POST("/chat", handler.ChatHandlerWithState(state))

		// 流式任务 API（前端订阅模式，支持断线重连）
		stream := apiV1.Group("/stream")
		{
			stream.POST("/start", handler.StreamStartHandlerWithState(state))
			stream.POST("/steer", handler.StreamSteerHandler())
			stream.GET("/queue/:sessionID", handler.StreamQueueHandler())
			stream.PATCH("/queue/:sessionID/:queueID", handler.StreamQueueUpdateHandler())
			stream.DELETE("/queue/:sessionID/:queueID", handler.StreamQueueDeleteHandler())
			stream.POST("/queue/:sessionID/:queueID/kind", handler.StreamQueueKindHandler())
			stream.POST("/queue/:sessionID/:queueID/move", handler.StreamQueueMoveHandler())
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

		// 会话分享管理 API
		sessionShares := apiV1.Group("/session-shares")
		{
			sessionShares.POST("", handler.CreateSessionShareHandler())
			sessionShares.GET("", handler.ListSessionSharesHandler())
			sessionShares.DELETE("/:shareID", handler.DeleteSessionShareHandler())
		}

		// 公开会话分享 API
		publicSessionShares := apiV1.Group("/public/session-shares")
		{
			publicSessionShares.GET("/:shareID/info", handler.GetPublicSessionShareInfoHandler())
			publicSessionShares.POST("/:shareID/access", handler.AccessPublicSessionShareHandler())
		}

		// 会话管理 API
		sessions := apiV1.Group("/sessions")
		{
			sessions.GET("", handler.ListSessionsHandler())
			sessions.POST("", handler.CreateSessionHandler())
			sessions.GET("/:sessionID", handler.GetSessionHandler())
			sessions.DELETE("/:sessionID", handler.DeleteSessionHandler())
			sessions.POST("/rename", handler.RenameSessionHandler())
			sessions.POST("/agent", handler.UpdateSessionAgentHandler())
		}

		// 定时任务管理 API
		schedules := apiV1.Group("/schedules")
		{
			schedules.GET("", handler.GetScheduleTasksHandler())
			schedules.POST("/:id/cancel", handler.CancelScheduleTaskHandler())
			schedules.GET("/:id/result", handler.GetTaskResultHandler())
			schedules.GET("/:id/history", handler.GetTaskHistoryHandler())
			schedules.GET("/:id/history/:filename", handler.GetTaskHistoryFileHandler())
		}

		// 技能管理 API
		skills := apiV1.Group("/skills")
		{
			skills.GET("", handler.GetInstalledSkillsHandler())
			skills.GET("/search", handler.SearchSkillsHandler())
			skills.POST("/install", handler.InstallSkillHandler())
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
			configGroup.PUT("", handler.UpdateConfigHandlerWithState(state))
			configGroup.GET("/tools", handler.GetToolNamesHandler())
			configGroup.GET("/tool-catalog", handler.GetToolCatalogHandler())
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
	return InitWithState(nil)
}

// InitWithState 初始化并返回带应用状态的 Gin 路由引擎（含 Web 界面）。
func InitWithState(state *appstate.State) (*gin.Engine, error) {
	authEnabled, err := handler.AuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("check auth config: %w", err)
	}

	r := newEngine(authEnabled)

	webFS := web.GetFS()
	r.GET("/static/*filepath", serveStatic(webFS))
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
			serveHTML(c, webFS, "login.html")
		}
		r.GET("/login", serveLogin)
	}

	serveIndex := func(c *gin.Context) {
		serveHTML(c, webFS, "index.html")
	}
	r.GET("/", serveIndex)
	r.GET("/chat", serveIndex)

	// 文件分享预览页面
	servePreview := func(c *gin.Context) {
		serveHTML(c, webFS, "preview.html")
	}
	r.GET("/p/:linkId", servePreview)
	r.GET("/s/:shareID", func(c *gin.Context) {
		serveHTML(c, webFS, "session_share.html")
	})

	registerAPIRoutesWithState(r, authEnabled, state)
	return r, nil
}

func serveStatic(webFS fs.FS) gin.HandlerFunc {
	fileServer := http.StripPrefix("/static", http.FileServer(http.FS(webFS)))
	return func(c *gin.Context) {
		setStaticCacheHeader(c)
		path := strings.TrimPrefix(c.Param("filepath"), "/")
		if path == "css/style.css" {
			data, err := fs.ReadFile(webFS, path)
			if err != nil {
				c.String(http.StatusNotFound, "static file not found")
				return
			}
			c.Data(http.StatusOK, "text/css; charset=utf-8", []byte(versionCSSImports(string(data))))
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func serveHTML(c *gin.Context, webFS fs.FS, filename string) {
	data, err := fs.ReadFile(webFS, filename)
	if err != nil {
		c.String(http.StatusNotFound, "Page not found")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(versionStaticAssetRefs(string(data))))
}

func setStaticCacheHeader(c *gin.Context) {
	if c.Query("v") != "" {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	c.Header("Cache-Control", "no-cache")
}

func versionStaticAssetRefs(content string) string {
	assetVersion := staticAssetVersion()
	return staticAssetRefRe.ReplaceAllStringFunc(content, func(match string) string {
		return appendAssetVersion(match, assetVersion)
	})
}

func versionCSSImports(content string) string {
	assetVersion := staticAssetVersion()
	return cssImportURLRe.ReplaceAllStringFunc(content, func(match string) string {
		submatches := cssImportURLRe.FindStringSubmatch(match)
		if len(submatches) != 4 {
			return match
		}

		ref := submatches[2]
		if strings.Contains(ref, "://") || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "data:") {
			return match
		}

		quote := submatches[1]
		if quote == "" {
			quote = submatches[3]
		}
		return "url(" + quote + appendAssetVersion(ref, assetVersion) + quote + ")"
	})
}

func appendAssetVersion(ref string, assetVersion string) string {
	if strings.Contains(ref, "v=") {
		return ref
	}
	separator := "?"
	if strings.Contains(ref, "?") {
		separator = "&"
	}
	return ref + separator + "v=" + assetVersion
}

func staticAssetVersion() string {
	info := version.Get()
	versionToken := sanitizeAssetVersionPart(info.Version)
	buildToken := compactBuildTime(info.BuildTime)
	if versionToken != "" && buildToken != "" {
		return versionToken + "-" + buildToken
	}
	if versionToken != "" {
		return versionToken
	}
	if buildToken != "" {
		return buildToken
	}
	return "dev"
}

func compactBuildTime(buildTime string) string {
	var digits strings.Builder
	for _, r := range buildTime {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	token := digits.String()
	if len(token) >= 14 {
		return token[:14]
	}
	return token
}

func sanitizeAssetVersionPart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// InitAPI 初始化纯 API 路由（无 Web 界面）
func InitAPI() (*gin.Engine, error) {
	return InitAPIWithState(nil)
}

// InitAPIWithState 初始化带应用状态的纯 API 路由。
func InitAPIWithState(state *appstate.State) (*gin.Engine, error) {
	authEnabled, err := handler.AuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("check auth config: %w", err)
	}

	r := newEngine(authEnabled)
	registerAPIRoutesWithState(r, authEnabled, state)
	return r, nil
}
