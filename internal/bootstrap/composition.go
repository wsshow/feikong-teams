// Package bootstrap 负责装配应用入口共享的进程级依赖。
package bootstrap

import (
	"context"
	localprocess "fkteams/internal/adapters/process/local"
	filequeue "fkteams/internal/adapters/storage/file/queue"
	sqlitestore "fkteams/internal/adapters/storage/sqlite"
	"fkteams/internal/app/appdata"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	appprocess "fkteams/internal/app/process"
	domainmigration "fkteams/internal/domain/migration"
	memoryport "fkteams/internal/ports/memory"
	storageport "fkteams/internal/ports/storage"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	javascripthooks "fkteams/internal/adapters/hooks/javascript"
	modelproviders "fkteams/internal/adapters/model/providers"
	mcpadapter "fkteams/internal/adapters/tools/mcp"
	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/config"
	"fkteams/internal/app/execution"
	bootstrapruntimes "fkteams/internal/bootstrap/runtimes"
	bootstraptools "fkteams/internal/bootstrap/tools"
	"fkteams/internal/runtime/hooks"
)

// ExecutionDependencies 保存一次应用实例所需的执行依赖。
// 各注册表均为实例所有，避免入口依赖可变的进程级全局状态。
type ExecutionDependencies struct {
	MigrationStatus     domainmigration.Status
	database            *sqlitestore.Database
	unlockStorage       func() error
	storageCloseOnce    sync.Once
	storageCloseError   error
	processBackendClose func() error
	MemoryStore         memoryport.Store
	execution.Dependencies
	ExecutionGate         *config.ExecutionGate
	ModelProviderRegistry *modelproviders.Registry
}

// NewExecutionDependencies 创建并连接默认 runtime、模型、工具和智能体注册表。
func NewExecutionDependencies() (*ExecutionDependencies, error) {
	return newExecutionDependencies(nil, nil)
}

func newExecutionDependencies(documents storageport.Documents, memories memoryport.Store) (*ExecutionDependencies, error) {
	mcpProvider := mcpadapter.NewProvider()
	runtimeDefaults, err := bootstrapruntimes.NewDefaults(bootstrapruntimes.Options{
		MCPProvider: mcpProvider,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize runtime dependencies: %w", err)
	}

	var processes *appprocess.Service
	queues := appchat.NewQueueService(taskstream.NewManager(), filequeue.NewStore(appdata.SessionsDir(), documents), nil)
	var backendClose func() error
	if documents != nil {
		backend, err := localprocess.New(filepath.Join(appdata.Dir(), "runtime", "processes"))
		if err != nil {
			return nil, fmt.Errorf("initialize process backend: %w", err)
		}
		processes, err = appprocess.NewService(backend, backend)
		if err != nil {
			backend.Close()
			return nil, err
		}
		backendClose = backend.Close
		processes.SetCompletionSink(queues.DeliverProcess)
	}
	toolRegistry, err := bootstraptools.RegisterWithProcesses(mcpProvider, documents, processes)
	if err != nil {
		if processes != nil {
			processes.Close()
			backendClose()
		}
		return nil, fmt.Errorf("register default tools: %w", err)
	}
	hookBus := hooks.NewBus()
	hookBus.Register(javascripthooks.NewHandler(func() []config.JavaScriptHook {
		return config.Get().JavaScript.Hooks
	}), hooks.Options{
		Timeout:     30 * time.Second,
		ErrorPolicy: hooks.ErrorFail,
		Concurrent:  true,
	})

	return &ExecutionDependencies{
		MemoryStore:           memories,
		processBackendClose:   backendClose,
		ExecutionGate:         &config.ExecutionGate{},
		ModelProviderRegistry: runtimeDefaults.ModelProviderRegistry,
		Dependencies: execution.Dependencies{
			Queues:        queues,
			Documents:     documents,
			Processes:     processes,
			Runtime:       runtimeDefaults.Runtime,
			Interrupt:     runtimeDefaults.Interrupt,
			ModelRegistry: runtimeDefaults.ModelRegistry,
			ToolRegistry:  toolRegistry,
			ToolDisplays:  toolmeta.NewRegistry(),
			AgentRegistry: agents.NewRegistry(),
			HookBus:       hookBus,
		},
	}, nil
}

// Context 将执行依赖绑定到父 context，供传输层和应用用例按需读取。
func (d *ExecutionDependencies) Context(parent context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if d == nil {
		return parent
	}

	return modelproviders.WithRegistry(d.Dependencies.Context(parent), d.ModelProviderRegistry)
}
