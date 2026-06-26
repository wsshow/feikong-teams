package handler

import (
	"context"
	"log"
	"path/filepath"
	"time"

	modelproviders "fkteams/internal/adapters/model/providers"
	eventlog "fkteams/internal/adapters/storage/file/history"
	appagent "fkteams/internal/app/agent"
	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	appschedule "fkteams/internal/app/schedule"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
)

// Runtime 持有单个 HTTP server 实例的运行态依赖。
type Runtime struct {
	Streams       *taskstream.Manager
	Sessions      *eventlog.SessionHistoryManager
	HistoryDir    string
	RunnerCache   *appagent.Cache
	Connections   *WebSocketHub
	ChunkUploads  *ChunkUploadStore
	PreviewLinks  *PreviewLinkStore
	SessionShares *SessionShareStore
	Favicons      *FaviconProxy
	Scheduler     *appschedule.Service
	AgentRegistry *agents.Registry
	ToolRegistry  *apptools.ToolGroupRegistry
	ModelRegistry *modelregistry.Registry
	Providers     *modelproviders.Registry
	Engine        runtimeport.Engine
	Interrupt     runtimeport.InterruptRuntime
	ResetChannels func()
}

// RuntimeOptions 用于测试或嵌入式场景显式替换 HTTP runtime 依赖。
type RuntimeOptions struct {
	Streams       *taskstream.Manager
	Sessions      *eventlog.SessionHistoryManager
	HistoryDir    string
	RunnerCache   *appagent.Cache
	Connections   *WebSocketHub
	ChunkUploads  *ChunkUploadStore
	PreviewLinks  *PreviewLinkStore
	SessionShares *SessionShareStore
	Favicons      *FaviconProxy
	Scheduler     *appschedule.Service
	AgentRegistry *agents.Registry
	ToolRegistry  *apptools.ToolGroupRegistry
	ModelRegistry *modelregistry.Registry
	Providers     *modelproviders.Registry
	Engine        runtimeport.Engine
	Interrupt     runtimeport.InterruptRuntime
	ResetChannels func()
}

// NewRuntime 创建一个独立的 HTTP runtime 实例。
func NewRuntime(options ...RuntimeOptions) *Runtime {
	var opt RuntimeOptions
	if len(options) > 0 {
		opt = options[0]
	}
	streams := opt.Streams
	if streams == nil {
		streams = newStreamManager()
	}
	rt := &Runtime{
		Streams:       streams,
		Sessions:      opt.Sessions,
		HistoryDir:    opt.HistoryDir,
		RunnerCache:   opt.RunnerCache,
		Connections:   opt.Connections,
		ChunkUploads:  opt.ChunkUploads,
		PreviewLinks:  opt.PreviewLinks,
		SessionShares: opt.SessionShares,
		Favicons:      opt.Favicons,
		Scheduler:     opt.Scheduler,
		AgentRegistry: opt.AgentRegistry,
		ToolRegistry:  opt.ToolRegistry,
		ModelRegistry: opt.ModelRegistry,
		Providers:     opt.Providers,
		Engine:        opt.Engine,
		Interrupt:     opt.Interrupt,
		ResetChannels: opt.ResetChannels,
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
	if rt.Favicons == nil {
		rt.Favicons = NewFaviconProxy(FaviconProxyOptions{})
	}
	return rt
}

func newStreamManager() *taskstream.Manager {
	m := taskstream.NewManager()
	m.StartCleanup(1 * time.Minute)
	return m
}

func (rt *Runtime) recorder(sessionID string) *eventlog.HistoryRecorder {
	return rt.Sessions.GetOrCreate(sessionID, rt.HistoryDir)
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
	ctx = rt.withRuntimeContext(ctx)
	return rt.RunnerCache.ResolveWithTeamFallback(ctx, mode, agentName)
}

func (rt *Runtime) withRuntimeContext(ctx context.Context) context.Context {
	ctx = runtimeport.WithEngine(ctx, rt.Engine)
	ctx = runtimeport.WithInterruptRuntime(ctx, rt.Interrupt)
	ctx = modelregistry.WithRegistry(ctx, rt.ModelRegistry)
	ctx = modelproviders.WithRegistry(ctx, rt.Providers)
	ctx = apptools.WithRegistry(ctx, rt.ToolRegistry)
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
