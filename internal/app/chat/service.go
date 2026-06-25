package chat

import (
	"context"
	"fmt"

	"fkteams/engine"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

// EventHandler 处理一次对话运行期间产生的领域事件。
type EventHandler func(event.Event) error

// ContextHook 在运行前补充上下文能力，例如转向输入和请求级元数据。
type ContextHook func(context.Context) context.Context

// HistorySink 描述对话用例需要的最小历史写入能力。
type HistorySink interface {
	GetMessageCount() int
	RecordUserMessage(msg message.Message)
	SetSummary(summary string, beforeCount int)
}

// TurnRequest 描述一次用户输入到运行时执行的完整用例请求。
type TurnRequest struct {
	SessionID        string
	RunID            string
	Runner           runtimeport.Runner
	Input            message.TurnInput
	EventHandler     EventHandler
	History          HistorySink
	InterruptHandler runtimeport.InterruptHandler
	NonInteractive   bool
	ContextHooks     []ContextHook
	OnFinish         func(ctx context.Context, result *runtimeport.RunResult, err error)
}

// Service 是所有入口共享的聊天用例服务。
type Service struct{}

// NewService 创建聊天用例服务。
func NewService() *Service {
	return &Service{}
}

// RunTurn 执行一次对话回合，并将入口层能力转换为运行时选项。
func (s *Service) RunTurn(ctx context.Context, req TurnRequest) (*runtimeport.RunResult, error) {
	if req.Runner == nil {
		return nil, fmt.Errorf("chat turn runner is nil")
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("chat turn session ID is empty")
	}

	session := engine.NewSession(req.Runner, req.SessionID).
		WithInput(req.Input)
	if req.RunID != "" {
		session.WithRunID(req.RunID)
	}
	if req.EventHandler != nil {
		session.OnEvent(engine.EventHandler(req.EventHandler))
	}
	if req.History != nil {
		session.WithHistory(req.History)
	}
	if req.InterruptHandler != nil {
		session.OnInterrupt(engine.InterruptHandler(req.InterruptHandler))
	}
	if req.NonInteractive {
		session.NonInteractive()
	}
	for _, hook := range req.ContextHooks {
		if hook != nil {
			session.WithContext(engine.ContextHook(hook))
		}
	}
	if req.OnFinish != nil {
		session.OnFinish(func(ctx context.Context, result *runtimeport.RunResult, err error) {
			req.OnFinish(ctx, result, err)
		})
	}
	return session.Run(ctx)
}
