package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/event"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/turn"
)

// interactionRecorder 将所有入口的人工交互纳入当前回合的事件管线。
type interactionRecorder struct {
	mu     sync.Mutex
	runID  string
	turnID string
	tools  map[string]event.Event
}

func newInteractionRecorder(runID string) *interactionRecorder {
	return &interactionRecorder{runID: runID, turnID: events.TurnID(runID, 1), tools: make(map[string]event.Event)}
}

func (r *interactionRecorder) observe(e event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.TurnID != "" && e.MemberCallID == "" {
		r.turnID = e.TurnID
	}
	switch e.Type {
	case event.TypeToolCallStarted:
		r.tools[e.ToolCallID] = e
	case event.TypeToolCallCompleted, event.TypeToolCallFailed:
		delete(r.tools, e.ToolCallID)
	}
}

func (r *interactionRecorder) base(metadata runtimeport.InterruptMetadata, toolID, toolName string) event.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	base := event.Event{RunID: r.runID, TurnID: r.turnID, MemberCallID: metadata.MemberCallID,
		MemberToolName: metadata.MemberToolName, MemberName: metadata.MemberName, MemberOrder: metadata.MemberOrder,
		ParentToolCallID: metadata.MemberCallID, ParentToolName: metadata.MemberToolName, ToolCallID: toolID, ToolName: toolName}
	var candidate *event.Event
	for _, tool := range r.tools {
		if tool.MemberCallID != metadata.MemberCallID || (toolID != "" && tool.ToolCallID != toolID) || (toolName != "" && tool.ToolName != toolName) {
			continue
		}
		if candidate != nil {
			candidate = nil
			break
		}
		copy := tool
		candidate = &copy
	}
	if candidate != nil {
		base.MessageID, base.AgentName, base.RunPath = candidate.MessageID, candidate.AgentName, candidate.RunPath
		base.TurnID, base.ToolCallID, base.ToolName = candidate.TurnID, candidate.ToolCallID, candidate.ToolName
		if base.TurnID == "" {
			base.TurnID = r.turnID
		}
	}
	if base.ToolCallID != "" {
		base.ToolCallRef = "tool_call:" + base.ToolCallID
	}
	return base
}

func emitInteraction(ctx context.Context, e event.Event) error {
	return events.NewEmitter(e.RunID, e.TurnID, events.Dispatch(ctx)).Emit(e)
}

func askRequested(base event.Event, id string, info *ask.AskInfo) event.Event {
	base.Type, base.Detail, base.Content = event.TypeAskRequested, id, info.Question
	base.Ask = &event.AskPayload{ID: id, Question: info.Question, Options: append([]string(nil), info.Options...), MultiSelect: info.MultiSelect}
	return base
}

func askAnswered(base event.Event, id string, response *ask.AskResponse) event.Event {
	base.Type, base.Detail = event.TypeAskAnswered, id
	base.Ask = &event.AskPayload{ID: id, Selected: append([]string(nil), response.Selected...), FreeText: response.FreeText}
	parts := append([]string(nil), response.Selected...)
	if response.FreeText != "" {
		parts = append(parts, response.FreeText)
	}
	base.Content = strings.Join(parts, " | ")
	return base
}

func (r *interactionRecorder) askHandler(handler ask.RuntimeHandler) ask.RuntimeHandler {
	return func(ctx context.Context, req ask.RuntimeRequest) (*ask.AskResponse, error) {
		if req.Info == nil || req.ID == "" {
			return nil, fmt.Errorf("invalid ask request")
		}
		base := r.base(req.Metadata, req.ToolCallID, req.ToolName)
		if err := emitInteraction(ctx, askRequested(base, req.ID, req.Info)); err != nil {
			return nil, err
		}
		response, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, fmt.Errorf("invalid ask response")
		}
		if err := emitInteraction(ctx, askAnswered(base, req.ID, response)); err != nil {
			return nil, err
		}
		return response, nil
	}
}

func (r *interactionRecorder) interruptHandler(handler runtimeport.InterruptHandler) runtimeport.InterruptHandler {
	if handler == nil {
		handler = turn.FixedDecisionHandler(0)
	}
	return func(ctx context.Context, interrupts []runtimeport.Interrupt) (runtimeport.InterruptDecisions, error) {
		hasRoot := false
		for _, interrupt := range interrupts {
			hasRoot = hasRoot || interrupt.IsRootCause
		}
		if !hasRoot {
			return handler(ctx, interrupts)
		}
		decisions := make(runtimeport.InterruptDecisions)
		for _, interrupt := range interrupts {
			if !interrupt.IsRootCause {
				continue
			}
			metadata := runtimeport.InterruptMetadata{MemberCallID: interrupt.MemberCallID, MemberToolName: interrupt.MemberToolName, MemberName: interrupt.MemberName, MemberOrder: interrupt.MemberOrder}
			name := ""
			info, isAsk := interrupt.Info.(*ask.AskInfo)
			if isAsk && info != nil {
				name = "ask_questions"
			}
			base := r.base(metadata, "", name)
			base.Detail = interrupt.ID
			if isAsk && info != nil {
				base = askRequested(base, interrupt.ID, info)
			} else {
				base.Type = event.TypeApprovalRequested
				base.Content = "需要审批"
				if interrupt.Info != nil {
					base.Content = fmt.Sprint(interrupt.Info)
				}
				base.Approval = &event.ApprovalPayload{ID: interrupt.ID, Message: base.Content}
			}
			if err := emitInteraction(ctx, base); err != nil {
				return nil, err
			}
			// 每个根中断独立收集决策，避免一次审批被扩散到其他并行操作。
			result, err := handler(ctx, []runtimeport.Interrupt{interrupt})
			if err != nil {
				return nil, err
			}
			decision, decided := result[interrupt.ID]
			if !decided {
				continue
			}
			decisions[interrupt.ID] = decision

			if base.Type == event.TypeAskRequested {
				response, ok := decision.(*ask.AskResponse)
				if !ok || response == nil {
					continue
				}
				base = askAnswered(base, interrupt.ID, response)
			} else {
				text := approvalDecision(decision)
				base.Type, base.Content = event.TypeApprovalAnswered, text
				base.Approval = &event.ApprovalPayload{ID: interrupt.ID, Decision: text}
			}
			if err := emitInteraction(ctx, base); err != nil {
				return nil, err
			}
		}
		return decisions, nil
	}
}

func approvalDecision(decision any) string {
	value, ok := decision.(int)
	if !ok {
		return fmt.Sprint(decision)
	}
	switch value {
	case 1:
		return "已允许（一次）"
	case 2:
		return "已允许（该项）"
	case 3:
		return "已全部允许"
	default:
		return "已拒绝"
	}
}
