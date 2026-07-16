package handler

import (
	"context"
	"fkteams/internal/runtime/log"
	"fmt"
	"path/filepath"
	"time"

	modelproviders "fkteams/internal/adapters/model/providers"
	eventlog "fkteams/internal/adapters/storage/file/history"
	appagent "fkteams/internal/app/agent"
	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	appschedule "fkteams/internal/app/schedule"
	appsession "fkteams/internal/app/session"
	appskill "fkteams/internal/app/skill"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
)

// Runtime 持有单个 HTTP server 实例的运行态依赖。
type Runtime struct {
	Streams        *taskstream.Manager
	Sessions       *eventlog.SessionHistoryManager
	HistoryDir     string
	RunnerCache    *appagent.Cache
	Connections    *WebSocketHub
	ChunkUploads   *ChunkUploadStore
	PreviewLinks   *PreviewLinkStore
	SessionShares  *SessionShareStore
	SessionService *appsession.Service
	Favicons       *FaviconProxy
	Scheduler      *appschedule.Service
	AgentRegistry  *agents.Registry
	ToolRegistry   *apptools.ToolGroupRegistry
	ToolDisplays   *toolmeta.Registry
	SkillProviders *appskill.ProviderRegistry
	ModelRegistry  *modelregistry.Registry
	Providers      *modelproviders.Registry
	Runtime        runtimeport.Runtime
	Interrupt      runtimeport.InterruptRuntime
	ResetChannels  func()
}

// RuntimeOptions 用于测试或嵌入式场景显式替换 HTTP runtime 依赖。
type RuntimeOptions struct {
	Streams        *taskstream.Manager
	Sessions       *eventlog.SessionHistoryManager
	HistoryDir     string
	RunnerCache    *appagent.Cache
	Connections    *WebSocketHub
	ChunkUploads   *ChunkUploadStore
	PreviewLinks   *PreviewLinkStore
	SessionShares  *SessionShareStore
	SessionService *appsession.Service
	Favicons       *FaviconProxy
	Scheduler      *appschedule.Service
	AgentRegistry  *agents.Registry
	ToolRegistry   *apptools.ToolGroupRegistry
	ToolDisplays   *toolmeta.Registry
	SkillProviders *appskill.ProviderRegistry
	ModelRegistry  *modelregistry.Registry
	Providers      *modelproviders.Registry
	Runtime        runtimeport.Runtime
	Interrupt      runtimeport.InterruptRuntime
	ResetChannels  func()
}

// NewRuntime 创建一个独立的 HTTP runtime 实例。
func NewRuntime(options ...RuntimeOptions) *Runtime {
	var opt RuntimeOptions
	if len(options) > 0 {
		opt = options[0]
	}
	streams := opt.Streams
	if streams == nil {
		streams = taskstream.NewManager()
	}
	rt := &Runtime{
		Streams:        streams,
		Sessions:       opt.Sessions,
		HistoryDir:     opt.HistoryDir,
		RunnerCache:    opt.RunnerCache,
		Connections:    opt.Connections,
		ChunkUploads:   opt.ChunkUploads,
		PreviewLinks:   opt.PreviewLinks,
		SessionShares:  opt.SessionShares,
		SessionService: opt.SessionService,
		Favicons:       opt.Favicons,
		Scheduler:      opt.Scheduler,
		AgentRegistry:  opt.AgentRegistry,
		ToolRegistry:   opt.ToolRegistry,
		ToolDisplays:   opt.ToolDisplays,
		SkillProviders: opt.SkillProviders,
		ModelRegistry:  opt.ModelRegistry,
		Providers:      opt.Providers,
		Runtime:        opt.Runtime,
		Interrupt:      opt.Interrupt,
		ResetChannels:  opt.ResetChannels,
	}
	if rt.Sessions == nil {
		rt.Sessions = eventlog.NewSessionHistoryManager()
	}
	if rt.HistoryDir == "" {
		rt.HistoryDir = appdata.SessionsDir()
	}
	if rt.RunnerCache == nil {
		rt.RunnerCache = appagent.NewCache()
	}
	if rt.Connections == nil {
		rt.Connections = NewWebSocketHub(streams)
	}
	if rt.ChunkUploads == nil {
		rt.ChunkUploads = NewChunkUploadStore()
	}
	if rt.PreviewLinks == nil {
		rt.PreviewLinks = NewPreviewLinkStore("")
	}
	if rt.SessionShares == nil {
		rt.SessionShares = NewSessionShareStore("")
	}
	if rt.SessionService == nil {
		rt.SessionService = appsession.NewService(eventlog.NewSessionRepository(rt.HistoryDir))
	}
	if rt.Favicons == nil {
		rt.Favicons = NewFaviconProxy(FaviconProxyOptions{})
	}
	if rt.ToolDisplays == nil {
		rt.ToolDisplays = toolmeta.NewRegistry()
	}
	if rt.SkillProviders == nil {
		rt.SkillProviders = appskill.NewProviderRegistry()
	}
	return rt
}

// Start 启动当前 HTTP runtime 的后台任务。
func (rt *Runtime) Start(ctx context.Context) error {
	if rt == nil || rt.Streams == nil {
		return nil
	}
	if err := rt.InitializationError(); err != nil {
		return err
	}
	rt.Streams.StartCleanup(ctx, time.Minute)
	return nil
}

// InitializationError 返回持久化状态加载期间发生的错误。
func (rt *Runtime) InitializationError() error {
	if rt == nil {
		return nil
	}
	if rt.PreviewLinks != nil && rt.PreviewLinks.LoadError() != nil {
		return fmt.Errorf("initialize preview links: %w", rt.PreviewLinks.LoadError())
	}
	if rt.SessionShares != nil && rt.SessionShares.LoadError() != nil {
		return fmt.Errorf("initialize session shares: %w", rt.SessionShares.LoadError())
	}
	return nil
}

// Close 停止后台任务并取消仍在运行的流。
func (rt *Runtime) Close() {
	if rt == nil {
		return
	}
	if rt.Streams != nil {
		rt.Streams.StopCleanup()
		rt.Streams.CancelAll()
	}
	if rt.ChunkUploads != nil {
		rt.ChunkUploads.Close()
	}
}

func (rt *Runtime) recorder(sessionID string) *eventlog.HistoryRecorder {
	recorder := rt.Sessions.GetOrCreate(sessionID, rt.HistoryDir)
	recorder.SetToolDisplayResolver(rt.ToolDisplays)
	return recorder
}

func (rt *Runtime) sessionDirPath(sessionID string) string {
	return sessionDirPath(rt.HistoryDir, sessionID)
}

func sessionDirPath(historyDir, sessionID string) string {
	return filepath.Join(historyDir, filepath.Base(sessionID))
}

func (rt *Runtime) clearRunnerCache() {
	rt.RunnerCache.Clear()
	log.Println("runner cache cleared")
}

func (rt *Runtime) resolveRunner(ctx context.Context, mode, agentName string) (runtimeport.Runner, error) {
	ctx = rt.withExecutionDependencies(ctx)
	return rt.RunnerCache.ResolveWithTeamFallback(ctx, mode, agentName)
}

// withExecutionDependencies 为一次智能体执行装配请求级依赖。
// 普通 HTTP 查询直接使用 Runtime 字段，不经过 context 服务定位。
func (rt *Runtime) withExecutionDependencies(ctx context.Context) context.Context {
	ctx = runtimeport.WithRuntime(ctx, rt.Runtime)
	ctx = runtimeport.WithInterruptRuntime(ctx, rt.Interrupt)
	ctx = modelregistry.WithRegistry(ctx, rt.ModelRegistry)
	ctx = modelproviders.WithRegistry(ctx, rt.Providers)
	ctx = apptools.WithRegistry(ctx, rt.ToolRegistry)
	ctx = toolmeta.WithRegistry(ctx, rt.ToolDisplays)
	ctx = agents.WithRegistry(ctx, rt.AgentRegistry)
	return appschedule.WithService(ctx, rt.Scheduler)
}

func (rt *Runtime) chatLifecycle() *appchat.SessionLifecycle {
	store := eventlog.NewChatSessionStore(rt.HistoryDir)
	return appchat.NewSessionLifecycle(store, store)
}

func (rt *Runtime) saveTurnHistory(recorder *eventlog.HistoryRecorder, sessionID string) {
	err := eventlog.NewChatSessionStore(rt.HistoryDir).SaveHistory(context.Background(), sessionID, recorder)
	appchat.LogLifecycleError("http", sessionID, err)
}

func (rt *Runtime) updateSessionTitleAndStatus(sessionID, userInput, status string) {
	err := rt.chatLifecycle().MarkProcessing(context.Background(), sessionID, userInput)
	appchat.LogLifecycleError("http", sessionID, err)
}

func (rt *Runtime) finishChat(recorder *eventlog.HistoryRecorder, sessionID, userInput string, manager appstate.MemoryManager) {
	err := rt.chatLifecycle().Finish(context.Background(), appchat.FinishRequest{
		SessionID:       sessionID,
		TitleSource:     userInput,
		Status:          appchat.SessionStatusCompleted,
		History:         recorder,
		FinalizeHistory: true,
		Memory:          manager,
		MemoryMessages:  eventlog.ConvertMemoryMessages(recorder),
	})
	appchat.LogLifecycleError("http", sessionID, err)
}

func (rt *Runtime) finishCancelledChat(recorder *eventlog.HistoryRecorder, sessionID, userInput string) {
	err := rt.chatLifecycle().Finish(context.Background(), appchat.FinishRequest{
		SessionID:   sessionID,
		TitleSource: userInput,
		Status:      appchat.SessionStatusCancelled,
		History:     recorder,
	})
	appchat.LogLifecycleError("http", sessionID, err)
}

func (rt *Runtime) finishErrorChat(recorder *eventlog.HistoryRecorder, sessionID, userInput string, err error) {
	lifecycleErr := rt.chatLifecycle().Finish(context.Background(), appchat.FinishRequest{
		SessionID:       sessionID,
		TitleSource:     userInput,
		Status:          appchat.SessionStatusError,
		History:         recorder,
		FinalizeHistory: true,
		Error:           err,
	})
	appchat.LogLifecycleError("http", sessionID, lifecycleErr)
}
