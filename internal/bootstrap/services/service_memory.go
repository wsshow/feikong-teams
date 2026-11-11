package services

import (
	"context"
	memorymodel "fkteams/internal/adapters/model/memory"
	memorystore "fkteams/internal/adapters/storage/file/memory"
	"fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/memory"
	memoryport "fkteams/internal/ports/memory"
	"fkteams/internal/runtime/log"
	"fmt"
	"path/filepath"
)

// MemoryService 长期记忆服务，封装 memory.Manager 的生命周期管理。
type MemoryService struct {
	Enabled      bool
	store        memoryport.Store
	workspaceDir string
	state        *appstate.State
}

// NewMemoryService 创建记忆服务
func NewMemoryService(workspaceDir string, state *appstate.State, stores ...memoryport.Store) *MemoryService {
	var store memoryport.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	if store == nil {
		store = memorystore.NewStore(filepath.Join(workspaceDir, "memory"))
	}
	return &MemoryService{
		Enabled:      true,
		store:        store,
		workspaceDir: workspaceDir,
		state:        state,
	}
}

// Name 返回服务名称
func (s *MemoryService) Name() string { return "memory" }

// Start 初始化并启动长期记忆服务
func (s *MemoryService) Start(ctx context.Context) error {
	manager, err := memory.NewManager(s.store, nil, nil)
	if err != nil {
		return err
	}
	manager.SetEnabled(false)
	s.state.SetMemory(manager)
	if !s.Enabled {
		return nil
	}
	chatModel, err := common.NewChatModel(ctx)
	if err != nil {
		log.Printf("[memory] 创建模型失败，自动提取未启动: %v", err)
		return nil
	}
	llmClient, err := memorymodel.NewLLMClient(chatModel)
	if err != nil {
		return err
	}
	manager.ResetLLM(llmClient)
	manager.SetEnabled(true)

	return nil
}

// Stop 等待记忆提取完成后停止服务
func (s *MemoryService) Stop(ctx context.Context) error {
	if manager := s.state.Memory(); manager != nil {
		log.Println("[memory] 正在等待记忆提取完成...")
		if err := manager.Wait(ctx); err != nil {
			return fmt.Errorf("wait for memory extraction: %w", err)
		}
		log.Println("[memory] 记忆提取完成")
	}
	return nil
}
