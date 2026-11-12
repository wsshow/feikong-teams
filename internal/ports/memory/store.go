package memory

import domainmemory "fkteams/internal/domain/memory"

// Store 原子保存与加载长期记忆；加载失败不能作为空记忆继续运行。
type Store interface {
	Load(limit int) ([]domainmemory.MemoryEntry, error)
	Save([]domainmemory.MemoryEntry) error
}
