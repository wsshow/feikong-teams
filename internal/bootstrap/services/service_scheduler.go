package services

import (
	"context"
	"path/filepath"
	"sync"

	"fkteams/internal/adapters/scheduler/filecron"
	appagent "fkteams/internal/app/agent"
	appschedule "fkteams/internal/app/schedule"
	"fkteams/log"
)

// SchedulerService 定时任务调度服务
type SchedulerService struct {
	schedulerDir string
	mu           sync.Mutex
	scheduler    *filecron.Scheduler
}

// NewSchedulerService 创建调度服务
func NewSchedulerService(schedulerDir string) *SchedulerService {
	return &SchedulerService{
		schedulerDir: schedulerDir,
	}
}

// Name 返回服务名称
func (s *SchedulerService) Name() string { return "scheduler" }

// Start 初始化调度器并启动定时任务服务
func (s *SchedulerService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scheduler != nil {
		return nil
	}

	sched, err := filecron.NewScheduler(s.schedulerDir)
	if err != nil {
		log.Printf("[scheduler] 初始化定时任务调度器失败: %v", err)
		return nil // 调度器初始化失败不阻止应用启动
	}

	executor := appschedule.NewBackgroundExecutor(appagent.CreateBackgroundTaskRunner, filepath.Join(s.schedulerDir, "tasks"))
	sched.SetExecutor(executor)
	appschedule.SetDefault(appschedule.NewService(sched))
	sched.Start()
	s.scheduler = sched
	log.Println("[scheduler] 定时任务调度服务已启动")
	return nil
}

// Stop 停止定时任务调度服务
func (s *SchedulerService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scheduler != nil {
		s.scheduler.Stop()
		s.scheduler = nil
	}
	appschedule.SetDefault(nil)
	log.Println("[scheduler] 定时任务调度服务已停止")
	return nil
}
