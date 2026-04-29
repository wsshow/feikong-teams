// Package fkevent 提供智能体事件的处理和分发机制。
// 支持流式和非流式消息、工具调用、动作事件的统一处理。
package fkevent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// EventType 事件类型
type EventType string

const (
	EventError              EventType = "error"
	EventMessage            EventType = "message"
	EventToolResult         EventType = "tool_result"
	EventReasoningChunk     EventType = "reasoning_chunk"
	EventStreamChunk        EventType = "stream_chunk"
	EventToolResultChunk    EventType = "tool_result_chunk"
	EventToolCallsPreparing EventType = "tool_calls_preparing"
	EventToolCallsArgsDelta EventType = "tool_calls_args_delta"
	EventToolCalls          EventType = "tool_calls"
	EventAction             EventType = "action"
	EventDispatchProgress   EventType = "dispatch_progress"
)

// ActionType 动作类型（EventAction 事件下的子类型）
type ActionType string

const (
	ActionTransfer             ActionType = "transfer"
	ActionInterrupted          ActionType = "interrupted"
	ActionExit                 ActionType = "exit"
	ActionAskQuestions         ActionType = "ask_questions"
	ActionAskResponse          ActionType = "ask_response"
	ActionApprovalRequired     ActionType = "approval_required"
	ActionApprovalDecision     ActionType = "approval_decision"
	ActionContextCompressStart ActionType = "context_compress_start"
	ActionContextCompress      ActionType = "context_compress"
)

// Event 统一的事件结构，承载各类智能体输出
type Event struct {
	Type             EventType         `json:"type"`
	AgentName        string            `json:"agent_name,omitempty"`
	RunPath          string            `json:"run_path,omitempty"`
	Content          string            `json:"content,omitempty"`
	Detail           string            `json:"detail,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"` // 推理模型思考内容
	ToolCalls        []schema.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"` // 工具结果对应的调用 ID
	ActionType       ActionType        `json:"action_type,omitempty"`
	Error            string            `json:"error,omitempty"`
}

// formatRunPath 将运行路径格式化为字符串
func formatRunPath(runPath []adk.RunStep) string {
	return fmt.Sprintf("%v", runPath)
}

// callbackKey 用于在 context 中存储事件回调的 key
type callbackKey struct{}

// nonInteractiveKey 标记非交互模式（如 Web 服务），禁止终端 TUI
type nonInteractiveKey struct{}

// WithCallback 将事件回调绑定到 context
func WithCallback(ctx context.Context, cb func(Event) error) context.Context {
	return context.WithValue(ctx, callbackKey{}, cb)
}

// WithNonInteractive 标记当前 context 为非交互模式
func WithNonInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonInteractiveKey{}, true)
}

// IsNonInteractive 检查 context 是否为非交互模式
func IsNonInteractive(ctx context.Context) bool {
	v, _ := ctx.Value(nonInteractiveKey{}).(bool)
	return v
}

// getCallback 从 context 中获取事件回调函数
func getCallback(ctx context.Context) func(Event) error {
	if cb, ok := ctx.Value(callbackKey{}).(func(Event) error); ok {
		return cb
	}
	return nil
}

// ProcessAgentEvent 处理智能体事件，按顺序分发动作和消息输出
func ProcessAgentEvent(ctx context.Context, event *adk.AgentEvent) error {
	if event.Err != nil {
		if isContextCanceled(ctx, event.Err) {
			return nil
		}
		return handleEvent(ctx, Event{
			Type:      EventError,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Error:     event.Err.Error(),
		})
	}

	// 先处理动作（如 transfer），再处理输出（如工具结果），保证显示顺序正确
	if event.Action != nil {
		if err := handleAction(ctx, event); err != nil {
			return err
		}
	}

	if event.Output != nil && event.Output.MessageOutput != nil {
		if err := handleMessageOutput(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// handleMessageOutput 处理消息输出，区分完整消息和流式消息
func handleMessageOutput(ctx context.Context, event *adk.AgentEvent) error {
	msgOutput := event.Output.MessageOutput

	if msg := msgOutput.Message; msg != nil {
		return handleRegularMessage(ctx, event, msg)
	}

	if stream := msgOutput.MessageStream; stream != nil {
		return handleStreamingMessage(ctx, event, stream)
	}

	return nil
}

// handleRegularMessage 处理非流式的完整消息
func handleRegularMessage(ctx context.Context, event *adk.AgentEvent, msg *schema.Message) error {
	eventType := EventMessage
	if msg.Role == schema.Tool {
		eventType = EventToolResult
	}

	nEvent := Event{
		Type:             eventType,
		AgentName:        event.AgentName,
		RunPath:          formatRunPath(event.RunPath),
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCallID:       msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		nEvent.ToolCalls = msg.ToolCalls
	}

	return handleEvent(ctx, nEvent)
}

// streamState 持有流式消息处理过程中的工具调用聚合状态
type streamState struct {
	toolCallsMap    map[int][]*schema.Message // 按 index 聚合工具调用分片
	toolCallStarted map[int]bool              // 记录已发送准备事件的工具调用
	toolArgsBuffer  map[int]string            // 按 index 缓冲未发送的参数增量
	lastArgsDelta   time.Time
}

const argsDeltaInterval = 100 * time.Millisecond

func newStreamState() *streamState {
	return &streamState{
		toolCallsMap:    make(map[int][]*schema.Message),
		toolCallStarted: make(map[int]bool),
		toolArgsBuffer:  make(map[int]string),
	}
}

// handleStreamingMessage 处理流式消息，通过 goroutine 异步接收以支持 context 取消
func handleStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream *schema.StreamReader[*schema.Message]) error {
	ss := newStreamState()

	type recvResult struct {
		chunk *schema.Message
		err   error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		defer close(recvCh)
		for {
			chunk, err := stream.Recv()
			recvCh <- recvResult{chunk, err}
			if err != nil {
				return
			}
		}
	}()

	cancelled := false
	for !cancelled {
		select {
		case <-ctx.Done():
			stream.Close()
			cancelled = true
		case r, ok := <-recvCh:
			if !ok || errors.Is(r.err, io.EOF) {
				cancelled = true
				break
			}
			if r.err != nil {
				if isContextCanceled(ctx, r.err) {
					cancelled = true
					break
				}
				return handleEvent(ctx, Event{
					Type:      EventError,
					AgentName: event.AgentName,
					RunPath:   formatRunPath(event.RunPath),
					Error:     fmt.Sprintf("stream error: %v", r.err),
				})
			}
			if err := processStreamChunk(ctx, event, r.chunk, ss); err != nil {
				return err
			}
		}
	}

	flushToolArgsBuffer(ctx, event, ss)

	if ctx.Err() != nil {
		return nil
	}

	return emitMergedToolCalls(ctx, event, ss)
}

// processStreamChunk 处理单个流式消息分片（推理内容、文本内容、工具调用）
func processStreamChunk(ctx context.Context, event *adk.AgentEvent, chunk *schema.Message, ss *streamState) error {
	if chunk.ReasoningContent != "" {
		if err := handleEvent(ctx, Event{
			Type:      EventReasoningChunk,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Content:   chunk.ReasoningContent,
		}); err != nil {
			return err
		}
	}

	if chunk.Content != "" {
		var eventType EventType = EventStreamChunk
		if chunk.Role == schema.Tool {
			eventType = EventToolResultChunk
		}
		if err := handleEvent(ctx, Event{
			Type:      eventType,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Content:   chunk.Content,
		}); err != nil {
			return err
		}
	}

	if len(chunk.ToolCalls) > 0 {
		if err := collectToolCallChunks(ctx, event, chunk, ss); err != nil {
			return err
		}
	}

	return nil
}

// collectToolCallChunks 收集工具调用分片，节流发送参数增量
func collectToolCallChunks(ctx context.Context, event *adk.AgentEvent, chunk *schema.Message, ss *streamState) error {
	for _, tc := range chunk.ToolCalls {
		if tc.Index == nil {
			continue
		}
		idx := *tc.Index

		if !ss.toolCallStarted[idx] && tc.Function.Name != "" {
			ss.toolCallStarted[idx] = true
			if err := handleEvent(ctx, Event{
				Type:      EventToolCallsPreparing,
				AgentName: event.AgentName,
				RunPath:   formatRunPath(event.RunPath),
				ToolCalls: []schema.ToolCall{
					{Function: schema.FunctionCall{Name: tc.Function.Name}},
				},
			}); err != nil {
				return err
			}
		}

		ss.toolCallsMap[idx] = append(ss.toolCallsMap[idx], &schema.Message{
			Role: chunk.Role,
			ToolCalls: []schema.ToolCall{{
				ID:    tc.ID,
				Type:  tc.Type,
				Index: tc.Index,
				Function: schema.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}},
		})

		if tc.Function.Arguments != "" {
			ss.toolArgsBuffer[idx] += tc.Function.Arguments
		}
	}

	// 节流发送参数增量
	now := time.Now()
	if now.Sub(ss.lastArgsDelta) >= argsDeltaInterval && len(ss.toolArgsBuffer) > 0 {
		for idx, delta := range ss.toolArgsBuffer {
			if delta == "" {
				continue
			}
			if err := handleEvent(ctx, Event{
				Type:      EventToolCallsArgsDelta,
				AgentName: event.AgentName,
				RunPath:   formatRunPath(event.RunPath),
				Content:   delta,
				Detail:    fmt.Sprintf("%d", idx),
			}); err != nil {
				return err
			}
		}
		ss.toolArgsBuffer = make(map[int]string)
		ss.lastArgsDelta = now
	}

	return nil
}

// flushToolArgsBuffer 发送最后一批缓冲的参数增量
func flushToolArgsBuffer(ctx context.Context, event *adk.AgentEvent, ss *streamState) {
	for idx, delta := range ss.toolArgsBuffer {
		if delta == "" {
			continue
		}
		_ = handleEvent(ctx, Event{
			Type:      EventToolCallsArgsDelta,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Content:   delta,
			Detail:    fmt.Sprintf("%d", idx),
		})
	}
}

// emitMergedToolCalls 合并所有工具调用分片，按 index 排序后统一发送
func emitMergedToolCalls(ctx context.Context, event *adk.AgentEvent, ss *streamState) error {
	indices := make([]int, 0, len(ss.toolCallsMap))
	for idx := range ss.toolCallsMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var allToolCalls []schema.ToolCall
	for _, idx := range indices {
		concatenatedMsg, err := schema.ConcatMessages(ss.toolCallsMap[idx])
		if err != nil {
			return err
		}
		allToolCalls = append(allToolCalls, concatenatedMsg.ToolCalls...)
	}

	if len(allToolCalls) > 0 {
		return handleEvent(ctx, Event{
			Type:      EventToolCalls,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			ToolCalls: allToolCalls,
		})
	}

	return nil
}

// handleAction 处理智能体动作事件（转发、中断、退出）
func handleAction(ctx context.Context, event *adk.AgentEvent) error {
	action := event.Action

	if action.TransferToAgent != nil {
		return handleEvent(ctx, Event{
			Type:       EventAction,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: ActionTransfer,
			Content:    fmt.Sprintf("Transfer to agent: %s", action.TransferToAgent.DestAgentName),
		})
	}

	if action.Interrupted != nil {
		for _, ic := range action.Interrupted.InterruptContexts {
			content := fmt.Sprintf("%v", ic.Info)
			if stringer, ok := ic.Info.(fmt.Stringer); ok {
				content = stringer.String()
			}

			if err := handleEvent(ctx, Event{
				Type:       EventAction,
				AgentName:  event.AgentName,
				RunPath:    formatRunPath(event.RunPath),
				ActionType: ActionInterrupted,
				Content:    content,
			}); err != nil {
				return err
			}
		}
	}

	if action.Exit {
		return handleEvent(ctx, Event{
			Type:       EventAction,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: ActionExit,
			Content:    "Agent execution completed",
		})
	}

	return nil
}

// isContextCanceled 判断是否为 context 取消导致的错误，这类错误不需要展示给用户
func isContextCanceled(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded")
}

// handleEvent 分发事件到 context 中的回调，无回调时仅打印
func handleEvent(ctx context.Context, event Event) error {
	if cb := getCallback(ctx); cb != nil {
		return cb(event)
	}
	PrintEvent(event)
	return nil
}

// DispatchEvent 向 context 中的回调分发事件
func DispatchEvent(ctx context.Context, event Event) error {
	return handleEvent(ctx, event)
}
