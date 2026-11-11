// Package migration 编排备份、导入及校验，供 Web、API 和 CLI 共用。
package migration

import (
	"context"
	domainmigration "fkteams/internal/domain/migration"
	"fmt"
	"sync"
)

// Source 从停止写入的旧存储生成并校验备份快照。
type Source interface {
	Prepare(context.Context, func(string, int, int)) (domainmigration.Snapshot, error)
}

// Target 原子导入并校验数据，成功后记录持久化完成标记。
type Target interface {
	Completed(context.Context) (bool, error)
	Import(context.Context, domainmigration.Snapshot, func(string, int, int)) error
}

// Service 管理单个应用实例的迁移任务。
type Service struct {
	mu     sync.Mutex
	source Source
	target Target
	status domainmigration.Status
	done   chan struct{}
}

// NewService 读取持久化标记，避免已完成的迁移再次执行。
func NewService(ctx context.Context, source Source, target Target) (*Service, error) {
	complete, err := target.Completed(ctx)
	if err != nil {
		return nil, err
	}
	status := domainmigration.Status{State: "pending"}
	if complete {
		status.State = "completed"
	}
	return &Service{source: source, target: target, status: status}, nil
}

// Status 返回线程安全的进度快照。
func (s *Service) Status() domainmigration.Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

// Start 启动唯一后台任务；ctx 必须属于应用生命周期。
func (s *Service) Start(ctx context.Context) bool {
	s.mu.Lock()
	if s.status.State == "running" || s.status.State == "completed" {
		s.mu.Unlock()
		return false
	}
	s.status = domainmigration.Status{State: "running", Stage: "backup"}
	s.done = make(chan struct{})
	s.mu.Unlock()
	go s.run(ctx)
	return true
}

// Wait 等待迁移退出；取消等待不会取消应用持有的迁移任务。
func (s *Service) Wait(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	status := s.status
	s.mu.Unlock()
	if done == nil {
		if status.State == "completed" {
			return nil
		}
		return fmt.Errorf("migration has not started")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	status = s.Status()
	if status.State != "completed" {
		return fmt.Errorf("migration failed: %s", status.Error)
	}
	return nil
}
func (s *Service) progress(stage string, completed, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Stage = stage
	s.status.Completed = completed
	s.status.Total = total
}
func (s *Service) run(ctx context.Context) {
	snapshot, err := s.source.Prepare(ctx, s.progress)
	s.mu.Lock()
	s.status.BackupPath = snapshot.BackupPath
	s.mu.Unlock()
	if err == nil {
		err = s.target.Import(ctx, snapshot, s.progress)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.status.State = "failed"
		s.status.Error = err.Error()
	} else {
		s.status.State = "completed"
		s.status.Stage = "completed"
	}
	close(s.done)
}
