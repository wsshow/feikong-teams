package handler

import (
	"context"
	sharestore "fkteams/internal/adapters/storage/file/share"
	"fkteams/internal/runtime/log"
	"fmt"
	"path/filepath"
	"sync"
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
	"fkteams/internal/app/config"
	"fkteams/internal/app/lifecycle"
	appschedule "fkteams/internal/app/schedule"
	appsession "fkteams/internal/app/session"
	appskill "fkteams/internal/app/skill"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
	modelregistry "fkteams/internal/runtime/model"
)

// Runtime 持有单个 HTTP server 实例的运行态依赖。
type Runtime struct {
	RuntimeOptions

	sessionOperationsMu sync.Mutex
	sessionOperations   map[string]*sessionOperationLock
	tasksMu             sync.Mutex
	tasks               sync.WaitGroup
	closing             bool
	shutdownOnce        sync.Once
	resourceCloseOnce   sync.Once
	shutdownDone        chan struct{}
	taskContext         context.Context
	cancelTasks         context.CancelFunc
}

// RuntimeOptions 用于测试或嵌入式场景显式替换 HTTP runtime 依赖。
type RuntimeOptions struct {
	ExecutionGate  *config.ExecutionGate
	Control        *lifecycle.Control
	Streams        *taskstream.Manager
	Sessions       *eventlog.SessionHistoryManager
	HistoryDir     string
	RunnerCache    *appagent.Cache
	Connections    *WebSocketHub
	ChunkUploads   *ChunkUploadStore
	PreviewLinks   *sharestore.PreviewLinkStore
	SessionShares  *sharestore.SessionShareStore
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
	HookBus        *hooks.Bus
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
	opt.Streams = streams
	if opt.ExecutionGate == nil {
		opt.ExecutionGate = &config.ExecutionGate{}
	}
	taskContext, cancelTasks := context.WithCancel(context.Background())
	rt := &Runtime{RuntimeOptions: opt, shutdownDone: make(chan struct{}), taskContext: taskContext, cancelTasks: cancelTasks}

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
		rt.PreviewLinks = sharestore.NewPreviewLinkStore("")
	}
	if rt.SessionShares == nil {
		rt.SessionShares = sharestore.NewSessionShareStore("")
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

// Go 启动由当前 HTTP runtime 负责回收的后台任务。
func (rt *Runtime) Go(task func()) bool {
	if task == nil {
		return false
	}
	release, accepted := rt.beginTask()
	if !accepted {
		return false
	}
	go func() { defer release(); task() }()
	return true
}

func (rt *Runtime) beginTask() (func(), bool) {
	if rt == nil {
		return nil, false
	}
	rt.tasksMu.Lock()
	defer rt.tasksMu.Unlock()
	if rt.closing {
		return nil, false
	}
	rt.tasks.Add(1)
	var once sync.Once
	return func() { once.Do(rt.tasks.Done) }, true
}

// BeginShutdown 拒绝新后台任务并取消仍在运行的流。
func (rt *Runtime) BeginShutdown() {
	if rt == nil {
		return
	}
	rt.shutdownOnce.Do(func() {
		rt.tasksMu.Lock()
		rt.closing = true
		rt.cancelTasks()
		rt.tasksMu.Unlock()
		if rt.Streams != nil {
			rt.Streams.StopCleanup()
			rt.Streams.CancelAll()
		}
		go func() {
			rt.tasks.Wait()
			close(rt.shutdownDone)
		}()
	})
}

// Shutdown 停止后台任务、关闭后台资源并等待任务退出。
func (rt *Runtime) Shutdown(ctx context.Context) error {
	if rt == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rt.BeginShutdown()
	select {
	case <-rt.shutdownDone:
		rt.resourceCloseOnce.Do(func() {
			if rt.ChunkUploads != nil {
				rt.ChunkUploads.Close()
			}
		})
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for HTTP runtime tasks: %w", ctx.Err())
	}
}

// Close 无超时关闭 runtime，供尚未开始接收请求的启动失败路径使用。
func (rt *Runtime) Close() {
	_ = rt.Shutdown(context.Background())
}

func (rt *Runtime) acquireRecorder(sessionID string) (*eventlog.HistoryRecorder, func()) {
	unlock := rt.lockSessionOperation(sessionID)
	defer unlock()
	return rt.acquireRecorderLocked(sessionID)
}

func (rt *Runtime) acquireRecorderLocked(sessionID string) (*eventlog.HistoryRecorder, func()) {
	recorder, release := rt.Sessions.Acquire(sessionID, rt.HistoryDir)
	recorder.SetToolDisplayResolver(rt.ToolDisplays)
	return recorder, release
}

type sessionOperationLock struct {
	mu   sync.Mutex
	refs int
}

// lockSessionOperation 将同一会话的短时注册、删除和 recorder 获取串行化。
func (rt *Runtime) lockSessionOperation(sessionID string) func() {
	rt.sessionOperationsMu.Lock()
	if rt.sessionOperations == nil {
		rt.sessionOperations = make(map[string]*sessionOperationLock)
	}
	operation := rt.sessionOperations[sessionID]
	if operation == nil {
		operation = &sessionOperationLock{}
		rt.sessionOperations[sessionID] = operation
	}
	operation.refs++
	rt.sessionOperationsMu.Unlock()

	operation.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			operation.mu.Unlock()
			rt.sessionOperationsMu.Lock()
			operation.refs--
			if operation.refs == 0 && rt.sessionOperations[sessionID] == operation {
				delete(rt.sessionOperations, sessionID)
			}
			rt.sessionOperationsMu.Unlock()
		})
	}
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
	ctx = hooks.WithBus(ctx, rt.HookBus)
	return appschedule.WithService(ctx, rt.Scheduler)
}

func (rt *Runtime) chatLifecycle() *appchat.SessionLifecycle {
	store := eventlog.NewChatSessionStore(rt.HistoryDir)
	return appchat.NewSessionLifecycle(store)
}

func (rt *Runtime) updateSessionExecutionMetadata(sessionID, userInput, mode, currentAgent string) error {
	return rt.chatLifecycle().MarkProcessingWithTarget(context.Background(), sessionID, userInput, appchat.ExecutionTarget{
		Mode:         mode,
		CurrentAgent: currentAgent,
	})
}

// finishChat 将运行结果映射为会话状态，并透传持久化失败。
func (rt *Runtime) finishChat(ctx context.Context, recorder *eventlog.HistoryRecorder, sessionID, userInput string, manager appstate.MemoryManager, runErr error) error {
	return rt.chatLifecycle().Finish(context.WithoutCancel(ctx), appchat.FinishRequest{
		SessionID: sessionID, TitleSource: userInput, Status: appchat.CompletionStatus(runErr),
		History: recorder, FinalizeHistory: true, Error: runErr,
		Memory: manager, MemoryMessages: eventlog.ConvertMemoryMessages(recorder),
	})
}
