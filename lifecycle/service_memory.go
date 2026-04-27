package lifecycle

import (
	"context"
	"fkteams/agents/common"
	"fkteams/g"
	"fkteams/log"
	"fkteams/memory"
)

// MemoryService 长期记忆服务，封装 memory.Manager 的生命周期管理
type MemoryService struct {
	workspaceDir string
}

// NewMemoryService 创建记忆服务
func NewMemoryService(workspaceDir string) *MemoryService {
	return &MemoryService{
		workspaceDir: workspaceDir,
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
	g.MemoryManager = memory.NewManager(s.workspaceDir, memory.NewLLMClient(chatModel), nil)
	log.Println("[memory] 长期记忆服务已启动")
	return nil
}

// Stop 等待记忆提取完成后停止服务
func (s *MemoryService) Stop(ctx context.Context) error {
	if g.MemoryManager != nil {
		log.Println("[memory] 正在等待记忆提取完成...")
		g.MemoryManager.Wait()
		log.Println("[memory] 记忆提取完成")
	}
	return nil
}
