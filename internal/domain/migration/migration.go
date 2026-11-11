// Package migration 定义存储升级的状态和快照。
package migration

import domainmemory "fkteams/internal/domain/memory"

// Document 是待迁入数据库的逻辑文档。
type Document struct {
	Path       string
	Data       []byte
	BackupFile string
	SHA256     string
}

// Snapshot 保存一次备份对应的完整迁移输入。
type Snapshot struct {
	Documents  []Document
	Memories   []domainmemory.MemoryEntry
	BackupPath string
}

// Status 是各入口共享的升级进度。
type Status struct {
	State      string `json:"state"`
	Stage      string `json:"stage"`
	Completed  int    `json:"completed"`
	Total      int    `json:"total"`
	BackupPath string `json:"backup_path,omitempty"`
	Error      string `json:"error,omitempty"`
}
