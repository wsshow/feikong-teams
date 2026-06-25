package chat

import (
	"context"
	"fmt"

	"fkteams/internal/app/tools/approval"
	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
	"fkteams/internal/runtime/turn"
)

// EventHandler 处理一次对话运行期间产生的领域事件。
type EventHandler func(event.Event) error

// EventRecorder 描述运行事件的最小记录能力。
type EventRecorder interface {
	RecordEvent(event.Event)
}

type EventRecorderFunc func(event.Event)

func (fn EventRecorderFunc) RecordEvent(event event.Event) {
	if fn != nil {
		fn(event)
	}
}

// ContextHook 在运行前补充上下文能力，例如转向输入和请求级元数据。
type ContextHook func(context.Context) context.Context

// HistorySink 描述对话用例需要的最小历史写入能力。
type HistorySink interface {
	GetMessageCount() int
	RecordUserMessage(msg message.Message)
	SetSummary(summary string, beforeCount int)
}

// TurnRequest 描述一次用户输入到运行时执行的最小请求。
type TurnRequest struct {
	SessionID string
	Runner    runtimeport.Runner
	Input     message.TurnInput
}

type turnOptions struct {
	runID            string
	eventHandler     EventHandler
	eventRecorder    EventRecorder
	history          HistorySink
	interruptHandler runtimeport.InterruptHandler
	nonInteractive   bool
	approvalRegistry *approval.Registry
	steeringSource   runtimeport.SteeringSource
	askHandler       ask.RuntimeHandler
	hookBus          *hooks.Bus
	contextHooks     []ContextHook
	onFinish         func(ctx context.Context, result *runtimeport.RunResult, err error)
}

// TurnOption 为一次运行补充可选入口能力。
type TurnOption func(*turnOptions)

func WithRunID(runID string) TurnOption {
	return func(opts *turnOptions) {
		opts.runID = runID
	}
}

func OnEvent(handler EventHandler) TurnOption {
	return func(opts *turnOptions) {
		opts.eventHandler = handler
	}
}

func WithEventRecorder(recorder EventRecorder) TurnOption {
	return func(opts *turnOptions) {
		opts.eventRecorder = recorder
	}
}

func WithEventRecorderFunc(fn func(event.Event)) TurnOption {
	return WithEventRecorder(EventRecorderFunc(fn))
}

func WithHistory(history HistorySink) TurnOption {
	return func(opts *turnOptions) {
		opts.history = history
	}
}

func OnInterrupt(handler runtimeport.InterruptHandler) TurnOption {
	return func(opts *turnOptions) {
		opts.interruptHandler = handler
	}
}

func NonInteractive() TurnOption {
	return func(opts *turnOptions) {
		opts.nonInteractive = true
	}
}

func WithApprovalRegistry(registry *approval.Registry) TurnOption {
	return func(opts *turnOptions) {
		opts.approvalRegistry = registry
	}
}

func WithSteeringSource(source runtimeport.SteeringSource) TurnOption {
	return func(opts *turnOptions) {
		opts.steeringSource = source
	}
}

func WithAskRuntimeHandler(handler ask.RuntimeHandler) TurnOption {
	return func(opts *turnOptions) {
		opts.askHandler = handler
	}
}

func WithHookBus(bus *hooks.Bus) TurnOption {
	return func(opts *turnOptions) {
		opts.hookBus = bus
	}
}

func WithContext(hook ContextHook) TurnOption {
	return func(opts *turnOptions) {
		if hook != nil {
			opts.contextHooks = append(opts.contextHooks, hook)
		}
	}
}

func OnFinish(handler func(ctx context.Context, result *runtimeport.RunResult, err error)) TurnOption {
	return func(opts *turnOptions) {
		opts.onFinish = handler
	}
}

// Service 是所有入口共享的聊天用例服务。
type Service struct{}

// NewService 创建聊天用例服务。
func NewService() *Service {
	return &Service{}
}

// RunTurn 执行一次对话回合，并将入口层能力转换为运行时选项。
func (s *Service) RunTurn(ctx context.Context, req TurnRequest, options ...TurnOption) (*runtimeport.RunResult, error) {
	if req.Runner == nil {
		return nil, fmt.Errorf("chat turn runner is nil")
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("chat turn session ID is empty")
	}

	opts := turnOptions{}
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}

	session := turn.NewSession(req.Runner, req.SessionID).
		WithInput(req.Input)
	if opts.runID != "" {
		session.WithRunID(opts.runID)
	}
	if opts.eventRecorder != nil || opts.eventHandler != nil {
		session.OnEvent(func(event event.Event) error {
			if opts.eventRecorder != nil {
				opts.eventRecorder.RecordEvent(event)
			}
			if opts.eventHandler != nil {
				return opts.eventHandler(event)
			}
			return nil
		})
	}
	if opts.history != nil {
		session.WithHistory(opts.history)
	}
	if opts.interruptHandler != nil {
		session.OnInterrupt(turn.InterruptHandler(opts.interruptHandler))
	}
	if opts.nonInteractive {
		session.NonInteractive()
	}
	if opts.approvalRegistry != nil {
		session.WithContext(func(ctx context.Context) context.Context {
			return approval.WithRegistry(ctx, opts.approvalRegistry)
		})
	}
	if opts.steeringSource != nil {
		session.WithContext(func(ctx context.Context) context.Context {
			return runtimeport.WithSteeringSource(ctx, opts.steeringSource)
		})
	}
	if opts.askHandler != nil {
		session.WithContext(func(ctx context.Context) context.Context {
			return ask.WithRuntimeHandler(ctx, opts.askHandler)
		})
	}
	if opts.hookBus != nil {
		session.WithHookBus(opts.hookBus)
	}
	for _, hook := range opts.contextHooks {
		if hook != nil {
			session.WithContext(turn.ContextHook(hook))
		}
	}
	if opts.onFinish != nil {
		session.OnFinish(func(ctx context.Context, result *runtimeport.RunResult, err error) {
			opts.onFinish(ctx, result, err)
		})
	}
	return session.Run(ctx)
}
