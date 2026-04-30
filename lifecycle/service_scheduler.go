package lifecycle

import (
	"context"
	"fkteams/log"
	"fkteams/runner"
	"fkteams/tools/scheduler"
	"path/filepath"
)

// SchedulerService 定时任务调度服务
type SchedulerService struct {
	schedulerDir string
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
	sched, err := scheduler.InitGlobal(s.schedulerDir)
	if err != nil {
		log.Printf("[scheduler] 初始化定时任务调度器失败: %v", err)
		return nil // 调度器初始化失败不阻止应用启动
	}

	executor := scheduler.NewBackgroundExecutor(runner.CreateBackgroundTaskRunner, filepath.Join(s.schedulerDir, "tasks"))
	sched.SetExecutor(executor)
	sched.Start()
	log.Println("[scheduler] 定时任务调度服务已启动")
	return nil
}

// Stop 停止定时任务调度服务
func (s *SchedulerService) Stop(ctx context.Context) error {
	if sched := scheduler.Global(); sched != nil {
		sched.Stop()
		log.Println("[scheduler] 定时任务调度服务已停止")
	}
	return nil
}
