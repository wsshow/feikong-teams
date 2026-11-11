package memory

import (
	"context"
	domainmemory "fkteams/internal/domain/memory"
)

// LLMClient 是长期记忆提取需要的最小模型能力。
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Store 保存与加载长期记忆；加载失败不能作为空记忆继续运行。
type Store interface {
	Load(limit int) ([]domainmemory.MemoryEntry, error)
	Save([]domainmemory.MemoryEntry) error
}
