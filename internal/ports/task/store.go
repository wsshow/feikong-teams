// Package task 定义持久用户任务的存储契约。
package task

import domaintask "fkteams/internal/domain/task"

// Store 原子加载和保存应用任务快照。
type Store interface {
	Load() ([]domaintask.Task, error)
	Save([]domaintask.Task) error
}
