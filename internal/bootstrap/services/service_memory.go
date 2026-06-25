package services

import (
	"context"
	"fkteams/agents/common"
	"fkteams/appstate"
	"fkteams/log"
	"fkteams/memory"
)

// MemoryService 长期记忆服务，封装 memory.Manager 的生命周期管理。
type MemoryService struct {
	workspaceDir string
	state        *appstate.State
}

// NewMemoryService 创建记忆服务
func NewMemoryService(workspaceDir string, state *appstate.State) *MemoryService {
	return &MemoryService{
		workspaceDir: workspaceDir,
		state:        state,
	}
}

// Name 返回服务名称
func (s *MemoryService) Name() string { return "memory" }

// Start 初始化并启动长期记忆服务
func (s *MemoryService) Start(ctx context.Context) error {
	chatModel, err := common.NewChatModel()
	if err != nil {
		log.Printf("[memory] 创建模型失败，记忆服务未启动: %v", err)
		return nil
	}
	llmClient, err := memory.NewLLMClient(chatModel)
	if err != nil {
		log.Printf("[memory] 适配模型失败，记忆服务未启动: %v", err)
		return nil
	}
	s.state.SetMemory(memory.NewManager(s.workspaceDir, llmClient, nil))
	log.Println("[memory] 长期记忆服务已启动")
	return nil
}

// Stop 等待记忆提取完成后停止服务
func (s *MemoryService) Stop(ctx context.Context) error {
	if manager := s.state.Memory(); manager != nil {
		log.Println("[memory] 正在等待记忆提取完成...")
		manager.Wait()
		log.Println("[memory] 记忆提取完成")
	}
	return nil
}
