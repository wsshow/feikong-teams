package services

import (
	"context"
	storageport "fkteams/internal/ports/storage"
	"fmt"
	"path/filepath"
	"sync"

	"fkteams/internal/adapters/scheduler/filecron"
	eventlog "fkteams/internal/adapters/storage/file/history"
	appagent "fkteams/internal/app/agent"
	"fkteams/internal/app/appdata"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/config"
	"fkteams/internal/app/execution"
	appschedule "fkteams/internal/app/schedule"
)

// SchedulerService 定时任务调度服务
type SchedulerService struct {
	Documents     storageport.Documents
	executionGate *config.ExecutionGate
	schedulerDir  string
	mu            sync.Mutex
	scheduler     *filecron.Scheduler
	service       *appschedule.Service
	stopParent    func() bool
}

// NewSchedulerService 创建调度服务
func NewSchedulerService(schedulerDir string, gates ...*config.ExecutionGate) *SchedulerService {
	var gate *config.ExecutionGate
	if len(gates) > 0 {
		gate = gates[0]
	}
	return &SchedulerService{
		schedulerDir:  schedulerDir,
		executionGate: gate,
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

	sched, err := filecron.NewScheduler(s.schedulerDir, s.Documents)
	if err != nil {
		return fmt.Errorf("initialize scheduler: %w", err)
	}

	appService := appschedule.NewService(sched)
	dependencies := execution.FromContext(ctx)
	store, err := filecron.NewResultStore(filepath.Join(s.schedulerDir, "tasks"))
	if err != nil {
		return err
	}
	executor, err := appschedule.NewBackgroundExecutor(appagent.CreateBackgroundTaskRunner, store)
	if err != nil {
		return fmt.Errorf("initialize scheduler executor: %w", err)
	}
	executor.WithExecutionGate(s.executionGate).WithContextHook(func(ctx context.Context) context.Context {
		return appschedule.WithService(dependencies.Context(ctx), appService)
	})
	histories := eventlog.NewSessionHistoryManager()
	histories.Documents = s.Documents
	executor.WithConversationHistory(func(sessionID string) (appchat.SessionHistory, func(), error) {
		recorder, release := histories.Acquire(sessionID, appdata.SessionsDir())
		return recorder, release, nil
	}, eventlog.NewChatSessionStore(appdata.SessionsDir(), s.Documents))
	sched.SetExecutor(executor)
	sched.Start()
	s.stopParent = context.AfterFunc(ctx, sched.BeginShutdown)
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
		if s.stopParent != nil {
			s.stopParent()
			s.stopParent = nil
		}
		s.scheduler = nil
	}
	s.service = nil
	return nil
}
