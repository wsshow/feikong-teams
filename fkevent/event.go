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
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// formatRunPath 将运行路径格式化为字符串
func formatRunPath(runPath []adk.RunStep) string {
	return fmt.Sprintf("%v", runPath)
}

// callbackKey 用于在 context 中存储事件回调的 key
type callbackKey struct{}

// nonInteractiveKey 标记非交互模式（如 Web 服务），禁止终端 TUI
type nonInteractiveKey struct{}

const internalContinueToolName = "continue_output"

var globalEventSequence int64

func isInternalToolName(name string) bool {
	return name == internalContinueToolName
}

func isInternalContinueContent(content string) bool {
	return strings.Contains(content, "Your previous text output was truncated") ||
		strings.Contains(content, "Your previous tool call was truncated")
}

func filterVisibleToolCalls(toolCalls []schema.ToolCall) []schema.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	visible := toolCalls[:0]
	for _, tc := range toolCalls {
		if isInternalToolName(tc.Function.Name) {
			continue
		}
		visible = append(visible, tc)
	}
	if len(visible) == 0 {
		return nil
	}
	return visible
}

func intPtr(v int) *int {
	return &v
}

func normalizeEvent(event Event) Event {
	if event.Sequence == 0 {
		event.Sequence = atomic.AddInt64(&globalEventSequence, 1)
	}
	if event.EventID == "" {
		event.EventID = fmt.Sprintf("evt_%d", event.Sequence)
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.MemberCallID != "" {
		event.IsMemberEvent = true
	}
	if event.Phase == "" {
		event.Phase = defaultEventPhase(event.Type)
	}
	if !event.IsPartial && isPartialEventType(event.Type) {
		event.IsPartial = true
	}
	if !event.IsFinal && isFinalEventType(event.Type) {
		event.IsFinal = true
	}
	return event
}

// NormalizeEvent 补齐事件协议字段，供直接记录事件的调用方复用。
func NormalizeEvent(event Event) Event {
	return normalizeEvent(event)
}

func defaultEventPhase(eventType EventType) EventPhase {
	switch eventType {
	case EventToolCallsPreparing:
		return EventPhaseStart
	case EventReasoningChunk, EventStreamChunk, EventToolResultChunk, EventToolCallsArgsDelta:
		return EventPhaseDelta
	case EventMessage, EventToolResult, EventToolCalls:
		return EventPhaseComplete
	case EventError:
		return EventPhaseError
	default:
		return EventPhaseInfo
	}
}

func isPartialEventType(eventType EventType) bool {
	switch eventType {
	case EventReasoningChunk, EventStreamChunk, EventToolResultChunk, EventToolCallsArgsDelta, EventToolCallsPreparing:
		return true
	default:
		return false
	}
}

func isFinalEventType(eventType EventType) bool {
	switch eventType {
	case EventMessage, EventToolResult, EventToolCalls, EventError:
		return true
	default:
		return false
	}
}

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
	scope, cleanupScope := consumeAgentEventScope(event)
	defer cleanupScope()

	if event.Err != nil {
		if isContextCanceled(ctx, event.Err) {
			return nil
		}
		nEvent := Event{
			Type:      EventError,
			Phase:     EventPhaseError,
			IsFinal:   true,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Error:     event.Err.Error(),
		}
		scope.apply(&nEvent)
		return handleEvent(ctx, nEvent)
	}

	// 先处理动作（如 transfer），再处理输出（如工具结果），保证显示顺序正确
	if event.Action != nil {
		if err := handleAction(ctx, event, scope); err != nil {
			return err
		}
	}

	if event.Output != nil && event.Output.MessageOutput != nil {
		if err := handleMessageOutput(ctx, event, scope); err != nil {
			return err
		}
	}

	return nil
}

// handleMessageOutput 处理消息输出，区分完整消息和流式消息
func handleMessageOutput(ctx context.Context, event *adk.AgentEvent, scope MemberScope) error {
	msgOutput := event.Output.MessageOutput

	if msg := msgOutput.Message; msg != nil {
		return handleRegularMessage(ctx, event, msg, scope)
	}

	if stream := msgOutput.MessageStream; stream != nil {
		return handleStreamingMessage(ctx, event, stream, scope)
	}

	return nil
}

// handleRegularMessage 处理非流式的完整消息
func handleRegularMessage(ctx context.Context, event *adk.AgentEvent, msg *schema.Message, scope MemberScope) error {
	eventType := EventMessage
	if msg.Role == schema.Tool {
		if isInternalToolName(msg.ToolName) || isInternalContinueContent(msg.Content) {
			return nil
		}
		eventType = EventToolResult
	}

	nEvent := Event{
		Type:             eventType,
		AgentName:        event.AgentName,
		RunPath:          formatRunPath(event.RunPath),
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolName:         msg.ToolName,
		ToolCallID:       msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		nEvent.ToolCalls = filterVisibleToolCalls(msg.ToolCalls)
		if nEvent.Content == "" && nEvent.ReasoningContent == "" && len(nEvent.ToolCalls) == 0 {
			return nil
		}
	}

	scope.apply(&nEvent)
	return handleEvent(ctx, nEvent)
}

// streamState 持有流式消息处理过程中的工具调用聚合状态
type streamState struct {
	toolCallsMap     map[int][]*schema.Message // 按 index 聚合工具调用分片
	toolCallStarted  map[int]bool              // 记录已发送准备事件的工具调用
	toolArgsBuffer   map[int]string            // 按 index 缓冲未发送的参数增量
	toolCallIDs      map[int]string            // 按 index 记录工具调用 ID
	internalToolCall map[int]bool              // 按 index 标记内部工具调用
	lastArgsDelta    time.Time
}

const argsDeltaInterval = 100 * time.Millisecond

func newStreamState() *streamState {
	return &streamState{
		toolCallsMap:     make(map[int][]*schema.Message),
		toolCallStarted:  make(map[int]bool),
		toolArgsBuffer:   make(map[int]string),
		toolCallIDs:      make(map[int]string),
		internalToolCall: make(map[int]bool),
	}
}

// handleStreamingMessage 处理流式消息，通过 goroutine 异步接收以支持 context 取消
func handleStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream *schema.StreamReader[*schema.Message], scope MemberScope) error {
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
				nEvent := Event{
					Type:      EventError,
					Phase:     EventPhaseError,
					IsFinal:   true,
					AgentName: event.AgentName,
					RunPath:   formatRunPath(event.RunPath),
					Error:     fmt.Sprintf("stream error: %v", r.err),
				}
				scope.apply(&nEvent)
				return handleEvent(ctx, nEvent)
			}
			if err := processStreamChunk(ctx, event, r.chunk, ss, scope); err != nil {
				return err
			}
		}
	}

	flushToolArgsBuffer(ctx, event, ss, scope)

	if ctx.Err() != nil {
		return nil
	}

	return emitMergedToolCalls(ctx, event, ss, scope)
}

// processStreamChunk 处理单个流式消息分片（推理内容、文本内容、工具调用）
func processStreamChunk(ctx context.Context, event *adk.AgentEvent, chunk *schema.Message, ss *streamState, scope MemberScope) error {
	if chunk.ReasoningContent != "" {
		nEvent := Event{
			Type:      EventReasoningChunk,
			Phase:     EventPhaseDelta,
			IsPartial: true,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Content:   chunk.ReasoningContent,
		}
		scope.apply(&nEvent)
		if err := handleEvent(ctx, nEvent); err != nil {
			return err
		}
	}

	if chunk.Content != "" {
		if chunk.Role == schema.Tool && isInternalToolName(chunk.ToolName) {
			return nil
		}
		var eventType EventType = EventStreamChunk
		if chunk.Role == schema.Tool {
			eventType = EventToolResultChunk
		}
		nEvent := Event{
			Type:       eventType,
			Phase:      EventPhaseDelta,
			IsPartial:  true,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			Content:    chunk.Content,
			ToolName:   chunk.ToolName,
			ToolCallID: chunk.ToolCallID,
		}
		scope.apply(&nEvent)
		if err := handleEvent(ctx, nEvent); err != nil {
			return err
		}
	}

	if len(chunk.ToolCalls) > 0 {
		if err := collectToolCallChunks(ctx, event, chunk, ss, scope); err != nil {
			return err
		}
	}

	return nil
}

// collectToolCallChunks 收集工具调用分片，节流发送参数增量
func collectToolCallChunks(ctx context.Context, event *adk.AgentEvent, chunk *schema.Message, ss *streamState, scope MemberScope) error {
	for _, tc := range chunk.ToolCalls {
		if tc.Index == nil {
			continue
		}
		idx := *tc.Index
		if tc.ID != "" {
			ss.toolCallIDs[idx] = tc.ID
		}
		if isInternalToolName(tc.Function.Name) {
			ss.internalToolCall[idx] = true
		}
		if ss.internalToolCall[idx] {
			continue
		}

		if !ss.toolCallStarted[idx] && tc.Function.Name != "" {
			ss.toolCallStarted[idx] = true
			nEvent := Event{
				Type:      EventToolCallsPreparing,
				Phase:     EventPhaseStart,
				IsPartial: true,
				AgentName: event.AgentName,
				RunPath:   formatRunPath(event.RunPath),
				ToolCalls: []schema.ToolCall{{
					ID:       tc.ID,
					Index:    tc.Index,
					Function: schema.FunctionCall{Name: tc.Function.Name},
				}},
				ToolName:      tc.Function.Name,
				ToolCallID:    tc.ID,
				ToolCallIndex: tc.Index,
			}
			scope.apply(&nEvent)
			if err := handleEvent(ctx, nEvent); err != nil {
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
			if ss.internalToolCall[idx] {
				continue
			}
			nEvent := Event{
				Type:          EventToolCallsArgsDelta,
				Phase:         EventPhaseDelta,
				IsPartial:     true,
				AgentName:     event.AgentName,
				RunPath:       formatRunPath(event.RunPath),
				Content:       delta,
				Detail:        fmt.Sprintf("%d", idx),
				ToolCallID:    ss.toolCallIDs[idx],
				ToolCallIndex: intPtr(idx),
			}
			scope.apply(&nEvent)
			if err := handleEvent(ctx, nEvent); err != nil {
				return err
			}
		}
		ss.toolArgsBuffer = make(map[int]string)
		ss.lastArgsDelta = now
	}

	return nil
}

// flushToolArgsBuffer 发送最后一批缓冲的参数增量
func flushToolArgsBuffer(ctx context.Context, event *adk.AgentEvent, ss *streamState, scope MemberScope) {
	for idx, delta := range ss.toolArgsBuffer {
		if delta == "" {
			continue
		}
		if ss.internalToolCall[idx] {
			continue
		}
		nEvent := Event{
			Type:          EventToolCallsArgsDelta,
			Phase:         EventPhaseDelta,
			IsPartial:     true,
			AgentName:     event.AgentName,
			RunPath:       formatRunPath(event.RunPath),
			Content:       delta,
			Detail:        fmt.Sprintf("%d", idx),
			ToolCallID:    ss.toolCallIDs[idx],
			ToolCallIndex: intPtr(idx),
		}
		scope.apply(&nEvent)
		_ = handleEvent(ctx, nEvent)
	}
}

// emitMergedToolCalls 合并所有工具调用分片，按 index 排序后统一发送
func emitMergedToolCalls(ctx context.Context, event *adk.AgentEvent, ss *streamState, scope MemberScope) error {
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
		allToolCalls = append(allToolCalls, filterVisibleToolCalls(concatenatedMsg.ToolCalls)...)
	}

	if len(allToolCalls) > 0 {
		nEvent := Event{
			Type:      EventToolCalls,
			Phase:     EventPhaseComplete,
			IsFinal:   true,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			ToolCalls: allToolCalls,
		}
		scope.apply(&nEvent)
		return handleEvent(ctx, nEvent)
	}

	return nil
}

// handleAction 处理智能体动作事件（转发、中断、退出）
func handleAction(ctx context.Context, event *adk.AgentEvent, scope MemberScope) error {
	action := event.Action

	if action.TransferToAgent != nil {
		nEvent := Event{
			Type:       EventAction,
			Phase:      EventPhaseInfo,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: ActionTransfer,
			Content:    fmt.Sprintf("Transfer to agent: %s", action.TransferToAgent.DestAgentName),
		}
		scope.apply(&nEvent)
		return handleEvent(ctx, nEvent)
	}

	if action.Interrupted != nil {
		for _, ic := range action.Interrupted.InterruptContexts {
			content := fmt.Sprintf("%v", ic.Info)
			if stringer, ok := ic.Info.(fmt.Stringer); ok {
				content = stringer.String()
			}

			nEvent := Event{
				Type:       EventAction,
				Phase:      EventPhaseInfo,
				AgentName:  event.AgentName,
				RunPath:    formatRunPath(event.RunPath),
				ActionType: ActionInterrupted,
				Content:    content,
			}
			scope.apply(&nEvent)
			if err := handleEvent(ctx, nEvent); err != nil {
				return err
			}
		}
	}

	if action.Exit {
		nEvent := Event{
			Type:       EventAction,
			Phase:      EventPhaseComplete,
			IsFinal:    true,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: ActionExit,
			Content:    "Agent execution completed",
		}
		scope.apply(&nEvent)
		return handleEvent(ctx, nEvent)
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
	event = normalizeEvent(event)
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
