package chat

import (
	"context"
	"errors"
	"fkteams/internal/domain/event"
	"fmt"
	"strings"

	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/message"
	"fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"
	"fkteams/internal/runtime/turn"
)

// EventHandler 处理一次对话运行期间产生的领域事件。
type EventHandler = runtimeport.EventSink

// ContextHook 在运行前补充上下文能力，例如转向输入和请求级元数据。
type ContextHook = turn.ContextHook

// TurnRequest 描述一次用户输入到运行时执行的最小请求。
type TurnRequest struct {
	SessionID        string
	RunID            string
	Runner           runtimeport.Runner
	Input            message.TurnInput
	EventSink        EventHandler
	Summary          turn.SummarySink
	History          SessionHistory
	InterruptHandler runtimeport.InterruptHandler
	NonInteractive   bool
	ApprovalRegistry *approval.Registry
	SteeringSource   runtimeport.SteeringSource
	AskHandler       ask.RuntimeHandler
	HookBus          *hooks.Bus
	ContextHooks     []ContextHook
	OnFinish         func(ctx context.Context, result *runtimeport.RunResult, err error) error
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
	if req.HookBus == nil {
		req.HookBus = hooks.FromContext(ctx)
	}

	contextHooks := append([]ContextHook(nil), req.ContextHooks...)
	if req.ApprovalRegistry != nil {
		contextHooks = append(contextHooks, func(ctx context.Context) context.Context {
			return approval.WithRegistry(ctx, req.ApprovalRegistry)
		})
	}
	if req.AskHandler != nil {
		contextHooks = append(contextHooks, func(ctx context.Context) context.Context {
			return ask.WithRuntimeHandler(ctx, req.AskHandler)
		})
	}

	if req.RunID == "" {
		req.RunID = req.SessionID + ":" + session.NewID()
	}
	sink, finish := req.EventSink, req.OnFinish
	req.EventSink = func(event events.Event) error {
		if req.History != nil {
			req.History.RecordEvent(event)
		}
		if sink != nil {
			return sink(event)
		}
		return nil
	}
	req.OnFinish = func(ctx context.Context, result *runtimeport.RunResult, runErr error) error {
		persistErr := finalizationFailure(finishHistory(req.History, runErr))
		if finish == nil {
			return persistErr
		}
		// 元数据回调只接收完整结果，不再拥有历史记录或定稿职责。
		finishErr := finish(ctx, result, errors.Join(runErr, persistErr))
		return errors.Join(persistErr, finalizationFailure(finishErr))
	}
	if req.SteeringSource != nil {
		source := req.SteeringSource
		sequence := 0
		wrapped := func(ctx context.Context) ([]message.Message, error) {
			messages, err := source(ctx)
			if err != nil {
				return nil, err
			}
			for _, msg := range messages {
				sequence++
				turnID := events.TurnID(req.RunID, 1)
				if err := req.EventSink(events.UserMessage(req.RunID, turnID, fmt.Sprintf("%s:steering:%d", turnID, sequence), msg)); err != nil {
					return nil, err
				}
			}
			return messages, nil
		}
		contextHooks = append(contextHooks, func(ctx context.Context) context.Context { return runtimeport.WithSteeringSource(ctx, wrapped) })
	}

	return turn.NewExecutor().Run(ctx, turn.Request{
		Runner:         req.Runner,
		SessionID:      req.SessionID,
		RunID:          req.RunID,
		Input:          req.Input,
		EventSink:      req.EventSink,
		Summary:        req.Summary,
		OnInterrupt:    turn.InterruptHandler(req.InterruptHandler),
		NonInteractive: req.NonInteractive,
		ContextHooks:   contextHooks,
		HookBus:        req.HookBus,
		OnFinish:       req.OnFinish,
	})
}

// AssistantText 投影主智能体的可见回答；完整消息替换对应增量，避免重复拼接工具或用户内容。
func AssistantText(events []event.Event) string {
	type output struct {
		text     string
		complete bool
	}
	var order []string
	messages := make(map[string]output)
	for _, e := range events {
		if e.MemberCallID != "" || e.ParentToolCallID != "" {
			continue
		}
		if e.Type != event.TypeAssistantText && e.Type != event.TypeAssistantCompleted {
			continue
		}
		key := e.RunID + ":" + e.MessageID
		current, exists := messages[key]
		if !exists {
			order = append(order, key)
		}
		if e.Type == event.TypeAssistantCompleted {
			if e.Content != "" {
				current.text = e.Content
			}
			current.complete = true
		} else if !current.complete {
			current.text += e.Content
		}
		messages[key] = current
	}
	var text strings.Builder
	for _, key := range order {
		text.WriteString(messages[key].text)
	}
	return text.String()
}
