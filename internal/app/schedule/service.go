package schedule

import (
	"context"

	"fkteams/internal/domain/apperror"
	domainschedule "fkteams/internal/domain/schedule"
	schedulerport "fkteams/internal/ports/scheduler"
)

// Service 是调度任务的应用用例入口。
type Service struct {
	scheduler schedulerport.TaskService
}

type serviceContextKey struct{}

// NewService 创建调度用例服务。
func NewService(scheduler schedulerport.TaskService) *Service {
	return &Service{scheduler: scheduler}
}

// WithService 将调度用例服务注入当前请求上下文。
func WithService(ctx context.Context, service *Service) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if service == nil {
		return ctx
	}
	return context.WithValue(ctx, serviceContextKey{}, service)
}

// FromContext 从当前请求上下文读取调度用例服务。
func FromContext(ctx context.Context) *Service {
	if ctx == nil {
		return nil
	}
	service, _ := ctx.Value(serviceContextKey{}).(*Service)
	return service
}

// SchedulerNotReadyError 表示调度服务尚未完成组合根初始化。
type SchedulerNotReadyError struct{}

func (SchedulerNotReadyError) Error() string {
	return "scheduler service is not initialized"
}

func (s *Service) requireScheduler() (schedulerport.TaskService, error) {
	if s == nil || s.scheduler == nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler service is not initialized", SchedulerNotReadyError{})
	}
	return s.scheduler, nil
}

// AddTask 创建调度任务。
func (s *Service) AddTask(ctx context.Context, req schedulerport.AddTaskRequest) (*domainschedule.Task, error) {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return nil, err
	}
	return scheduler.AddTask(ctx, req)
}

// UpdateTask 更新非运行中的调度任务。
func (s *Service) UpdateTask(ctx context.Context, taskID string, req schedulerport.AddTaskRequest) (*domainschedule.Task, error) {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return nil, err
	}
	if taskID == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "task ID is required")
	}
	return scheduler.UpdateTask(ctx, taskID, req)
}

// ListTasks 列出调度任务。
func (s *Service) ListTasks(ctx context.Context, statusFilter domainschedule.Status) ([]domainschedule.Task, error) {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return nil, err
	}
	if statusFilter != "" && !domainschedule.ValidStatus(statusFilter) {
		return nil, apperror.Errorf(apperror.CodeInvalidArgument, "invalid task status: %s", statusFilter)
	}
	return scheduler.ListTasks(ctx, statusFilter)
}

// CancelTask 取消待执行任务，或请求停止正在执行的任务。
func (s *Service) CancelTask(ctx context.Context, taskID string) error {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return err
	}
	if taskID == "" {
		return apperror.New(apperror.CodeInvalidArgument, "task ID is required")
	}
	return scheduler.CancelTask(ctx, taskID)
}

// DeleteTask 删除非运行中的任务及其结果。
func (s *Service) DeleteTask(ctx context.Context, taskID string) error {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return err
	}
	if taskID == "" {
		return apperror.New(apperror.CodeInvalidArgument, "task ID is required")
	}
	return scheduler.DeleteTask(ctx, taskID)
}

// ReadTaskResult 读取最新执行结果。
func (s *Service) ReadTaskResult(ctx context.Context, taskID string) (string, error) {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return "", err
	}
	if taskID == "" {
		return "", apperror.New(apperror.CodeInvalidArgument, "task ID is required")
	}
	return scheduler.ReadTaskResult(ctx, taskID)
}

// ListHistoryEntries 列出历史执行结果。
func (s *Service) ListHistoryEntries(ctx context.Context, taskID string) ([]domainschedule.HistoryEntry, error) {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return nil, err
	}
	if taskID == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "task ID is required")
	}
	return scheduler.ListHistoryEntries(ctx, taskID)
}

// ReadHistoryFile 读取指定历史结果。
func (s *Service) ReadHistoryFile(ctx context.Context, taskID string, filename string) (string, error) {
	scheduler, err := s.requireScheduler()
	if err != nil {
		return "", err
	}
	if taskID == "" || filename == "" {
		return "", apperror.New(apperror.CodeInvalidArgument, "task ID and filename are required")
	}
	return scheduler.ReadHistoryFile(ctx, taskID, filename)
}
