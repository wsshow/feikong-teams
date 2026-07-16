package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"fkteams/internal/adapters/scheduler/filecron"
	appagent "fkteams/internal/app/agent"
	agents "fkteams/internal/app/agent/catalog"
	appschedule "fkteams/internal/app/schedule"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
)

// SchedulerService 定时任务调度服务
type SchedulerService struct {
	schedulerDir string
	mu           sync.Mutex
	scheduler    *filecron.Scheduler
	service      *appschedule.Service
}

// NewSchedulerService 创建调度服务
func NewSchedulerService(schedulerDir string) *SchedulerService {
	return &SchedulerService{
		schedulerDir: schedulerDir,
	}
}

// Name 返回服务名称
func (s *SchedulerService) Name() string { return "scheduler" }

// AppService 返回已启动的调度用例服务。
func (s *SchedulerService) AppService() *appschedule.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.service
}

// Start 初始化调度器并启动定时任务服务
func (s *SchedulerService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scheduler != nil {
		return nil
	}

	sched, err := filecron.NewScheduler(s.schedulerDir)
	if err != nil {
		return fmt.Errorf("initialize scheduler: %w", err)
	}

	appService := appschedule.NewService(sched)
	runtime, _ := runtimeport.RuntimeFromContext(ctx)
	interrupt, _ := runtimeport.InterruptRuntimeFromContext(ctx)
	agentRegistry, _ := agents.RegistryFromContext(ctx)
	models, _ := modelregistry.RegistryFromContext(ctx)
	tools, _ := apptools.RegistryFromContext(ctx)
	executor, err := appschedule.NewBackgroundExecutor(appagent.CreateBackgroundTaskRunner, filepath.Join(s.schedulerDir, "tasks"))
	if err != nil {
		return fmt.Errorf("initialize scheduler executor: %w", err)
	}
	executor.WithContextHook(func(ctx context.Context) context.Context {
		ctx = runtimeport.WithRuntime(ctx, runtime)
		ctx = runtimeport.WithInterruptRuntime(ctx, interrupt)
		ctx = agents.WithRegistry(ctx, agentRegistry)
		ctx = modelregistry.WithRegistry(ctx, models)
		ctx = apptools.WithRegistry(ctx, tools)
		return appschedule.WithService(ctx, appService)
	})
	sched.SetExecutor(executor)
	sched.Start()
	s.scheduler = sched
	s.service = appService
	return nil
}

// Stop 停止定时任务调度服务
func (s *SchedulerService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scheduler != nil {
		if err := s.scheduler.Stop(ctx); err != nil {
			return err
		}
		s.scheduler = nil
	}
	s.service = nil
	return nil
}
