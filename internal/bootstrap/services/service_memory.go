package services

import (
	"context"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/memory"
	memoryport "fkteams/internal/ports/memory"
	"fmt"
)

// MemoryService 初始化主动记忆服务，不创建后台任务或独立模型。
type MemoryService struct {
	Enabled      bool
	store        memoryport.Store
	workspaceDir string
	state        *appstate.State
}

// NewMemoryService 绑定应用拥有的持久存储。
func NewMemoryService(workspaceDir string, state *appstate.State, stores ...memoryport.Store) *MemoryService {
	var store memoryport.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &MemoryService{Enabled: true, store: store, workspaceDir: workspaceDir, state: state}
}

// Name 返回服务名称。
func (s *MemoryService) Name() string { return "memory" }

// Start 加载记忆，即使未配置模型也可查询和管理。
func (s *MemoryService) Start(context.Context) error {
	if s.state == nil {
		return fmt.Errorf("application state is required")
	}
	manager, err := memory.NewManager(s.store, nil)
	if err != nil {
		return err
	}
	manager.SetEnabled(s.Enabled)
	s.state.SetMemory(manager)
	return nil
}

// Stop 无需排空；所有变更均在工具或管理请求返回前持久化。
func (s *MemoryService) Stop(context.Context) error { return nil }
