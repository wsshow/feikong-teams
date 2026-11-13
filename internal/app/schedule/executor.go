package schedule

import (
	"context"
	"errors"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/config"
	"fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	schedulerport "fkteams/internal/ports/scheduler"
	"fmt"
)

// RunnerCreator 为每次后台任务创建独立运行器。
type RunnerCreator func(ctx context.Context) (runtimeport.Runner, error)

// BackgroundExecutor 将调度任务转换为一次后台聊天运行。
type BackgroundExecutor struct {
	executionGate *config.ExecutionGate
	createRunner  RunnerCreator
	store         schedulerport.ResultStore
	chat          *appchat.Service
	contextHook   func(context.Context) context.Context
}

// NewBackgroundExecutor 创建后台任务执行器。
func NewBackgroundExecutor(createRunner RunnerCreator, store schedulerport.ResultStore) (*BackgroundExecutor, error) {
	if store == nil {
		return nil, fmt.Errorf("scheduler result store is required")
	}
	return &BackgroundExecutor{createRunner: createRunner, store: store, chat: appchat.NewService()}, nil
}

// WithContextHook 设置每次执行前的上下文装配逻辑。
func (e *BackgroundExecutor) WithContextHook(hook func(context.Context) context.Context) *BackgroundExecutor {
	e.contextHook = hook
	return e
}

// WithExecutionGate 将任务纳入应用配置更新边界。
func (e *BackgroundExecutor) WithExecutionGate(gate *config.ExecutionGate) *BackgroundExecutor {
	e.executionGate = gate
	return e
}

// Execute 执行调度任务并写入当前结果和历史快照。
func (e *BackgroundExecutor) Execute(ctx context.Context, taskID string, task string) (string, error) {
	release := e.executionGate.Begin()
	defer release()
	if !domainsession.ValidID(taskID) || len(taskID) > 160 {
		return "", fmt.Errorf("invalid task ID")
	}
	if e.contextHook != nil {
		ctx = e.contextHook(ctx)
	}
	if e.createRunner == nil {
		return "", fmt.Errorf("create runner: runner creator is nil")
	}

	r, err := e.createRunner(ctx)
	if err != nil {
		return "", fmt.Errorf("create runner: %w", err)
	}

	callback, getResult := newMarkdownCollector()
	input := message.TurnInput{
		Message: message.Message{Role: message.RoleUser, Content: task},
	}

	first := appchat.TurnRequest{
		SessionID: "fkteams_scheduler_" + taskID,
		Runner:    r,
		Input:     input,
		EventSink: callback,
	}
	err = e.chat.RunQueued(ctx, appchat.QueueRequest{
		First: first,
		Next: func(queued message.QueuedMessage) (appchat.TurnRequest, error) {
			// 调度没有会话历史，复用有界报告延续上下文，避免另建无界事件缓冲。
			next := first
			next.Input = message.TurnInput{
				Context: []message.Message{input.Message, {Role: message.RoleAssistant, Content: getResult()}},
				Message: queued.Message(),
			}
			next.RunID = appchat.QueuedRunID(first.SessionID, queued)
			return next, nil
		},
	})
	if err != nil {
		errMsg := fmt.Sprintf("execution error: %v", err)
		if writeErr := e.store.SaveResult(taskID, task, errMsg); writeErr != nil {
			return "", errors.Join(err, writeErr)
		}
		return "", err
	}

	output := getResult()
	if err := e.store.SaveResult(taskID, task, output); err != nil {
		return "", err
	}
	return output, nil
}
