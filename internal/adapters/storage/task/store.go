// Package task 通过应用文档存储保存持久任务。
package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	domaintask "fkteams/internal/domain/task"
	storageport "fkteams/internal/ports/storage"
)

// Store 仅操作组合根传入的固定文档键，不接受用户路径。
type Store struct {
	documents storageport.Documents
	key       string
}

// NewStore 绑定应用存储；未装配时由访问操作返回明确错误。
func NewStore(documents storageport.Documents, key string) *Store {
	return &Store{documents: documents, key: key}
}

// Load 加载任务；读取失败不能视为没有任务。
func (s *Store) Load() ([]domaintask.Task, error) {
	if s.documents == nil {
		return nil, fmt.Errorf("task storage is unavailable")
	}
	data, err := s.documents.Read(s.key)
	if errors.Is(err, fs.ErrNotExist) {
		return []domaintask.Task{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("task snapshot exceeds size limit")
	}
	var tasks []domaintask.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("decode tasks: %w", err)
	}
	return tasks, nil
}

// Save 原子替换任务快照。
func (s *Store) Save(tasks []domaintask.Task) error {
	if s.documents == nil {
		return fmt.Errorf("task storage is unavailable")
	}
	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	if len(data) > 16<<20 {
		return fmt.Errorf("task snapshot exceeds size limit")
	}
	return s.documents.Write(s.key, data)
}
