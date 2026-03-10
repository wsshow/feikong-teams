package lifecycle

import (
	"context"
	"fkteams/agents/common"
	"fkteams/g"
	"fkteams/memory"
	"log"
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
	g.MemManager = memory.NewManager(s.workspaceDir, memory.NewLLMClient(common.NewChatModel()))
	log.Println("[memory] 长期记忆服务已启动")
	return nil
}

// Stop 等待记忆提取完成后停止服务
func (s *MemoryService) Stop(ctx context.Context) error {
	if g.MemManager != nil {
		log.Println("[memory] 正在等待记忆提取完成...")
		g.MemManager.Wait()
		log.Println("[memory] 记忆提取完成")
	}
	return nil
}
