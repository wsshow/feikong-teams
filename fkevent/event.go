// Package fkevent 提供智能体事件的处理和分发机制。
// 支持流式和非流式消息、工具调用、动作事件的统一处理。
package fkevent

import (
	"context"
	"encoding/json"
	"errors"
	"fkteams/agenttool"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
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

const (
	spanKindAgent  = "agent"
	spanKindMember = "member"
	spanKindTool   = "tool"
	spanKindAction = "action"
)

var globalEventSequence int64
var globalStreamSequence int64
var toolCallRefsByID sync.Map
var toolCallOrdersByID sync.Map
var toolCallSpansByID sync.Map
var toolCallSpansByRef sync.Map
var eventDispatchMu sync.Mutex

func isInternalToolName(name string) bool {
	return name == internalContinueToolName
}

// IsInternalToolName 判断工具名是否为内部事件管线工具。
func IsInternalToolName(name string) bool {
	return isInternalToolName(name)
}

func isInternalContinueContent(content string) bool {
	return strings.Contains(content, "Your previous text output was truncated") ||
		strings.Contains(content, "Your previous tool call was truncated")
}

// IsInternalContinueContent 判断内容是否为自动续写内部提示。
func IsInternalContinueContent(content string) bool {
	return isInternalContinueContent(content)
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
	if event.ToolCallRef == "" && event.ToolCallID != "" {
		if ref, ok := toolCallRefsByID.Load(event.ToolCallID); ok {
			event.ToolCallRef, _ = ref.(string)
		}
	}
	if event.ExternalCallID == "" && event.ToolCallID != "" {
		event.ExternalCallID = event.ToolCallID
	}
	if event.MemberOrder == nil && event.ParentToolCallID != "" {
		if order, ok := toolCallOrdersByID.Load(event.ParentToolCallID); ok {
			if v, ok := order.(int); ok {
				event.MemberOrder = intPtr(v)
			}
		}
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.MemberCallID != "" {
		event.IsMemberEvent = true
	}
	if event.Phase == "" {
		event.Phase = defaultEventPhase(event.Type)
	}
	if event.ContentKind == "" {
		event.ContentKind = defaultContentKind(event.Type)
	}
	if !event.IsPartial && isPartialEventType(event.Type) {
		event.IsPartial = true
	}
	if !event.IsFinal && isFinalEventType(event.Type) {
		event.IsFinal = true
	}
	attachEventSpans(&event)
	return event
}

func defaultContentKind(eventType EventType) ContentKind {
	switch eventType {
	case EventReasoningChunk:
		return ContentKindReasoning
	case EventStreamChunk, EventMessage:
		return ContentKindOutput
	case EventToolCallsArgsDelta:
		return ContentKindToolArgs
	case EventToolResult, EventToolResultChunk:
		return ContentKindToolResult
	case EventError:
		return ContentKindError
	default:
		return ""
	}
}

func toolCallRef(event *adk.AgentEvent, scope MemberScope, group string, idx int) string {
	parts := []string{"tool", group, event.AgentName, formatRunPath(event.RunPath), fmt.Sprintf("idx:%d", idx)}
	if scope.CallID != "" {
		parts = append(parts, "member:"+scope.CallID)
	}
	return strings.Join(parts, "|")
}

func streamToolCallRef(streamBase string, idx int) string {
	return strings.Join([]string{"tool", streamBase, fmt.Sprintf("idx:%d", idx)}, "|")
}

func registerToolCallRef(id, ref string) {
	if id == "" || ref == "" {
		return
	}
	toolCallRefsByID.Store(id, ref)
}

func registerToolCallOrder(id string, idx int) {
	if id == "" {
		return
	}
	toolCallOrdersByID.Store(id, idx)
}

func registerToolCallSpan(id, ref, span string) {
	if span == "" {
		return
	}
	if id != "" {
		toolCallSpansByID.Store(id, span)
	}
	if ref != "" {
		toolCallSpansByRef.Store(ref, span)
	}
}

func lookupToolCallSpan(id, ref string) string {
	if id != "" {
		if span, ok := toolCallSpansByID.Load(id); ok {
			if v, ok := span.(string); ok {
				return v
			}
		}
	}
	if ref != "" {
		if span, ok := toolCallSpansByRef.Load(ref); ok {
			if v, ok := span.(string); ok {
				return v
			}
		}
	}
	return ""
}

func makeSpanID(kind, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	return kind + ":" + key
}

func agentSpanID(event Event) string {
	return makeSpanID(spanKindAgent, event.AgentName+"|"+event.RunPath)
}

func memberSpanID(callID string) string {
	return makeSpanID(spanKindMember, callID)
}

func resolvedMemberSpanID(callID string) string {
	if span := lookupToolCallSpan(callID, ""); strings.HasPrefix(span, spanKindMember+":") {
		return span
	}
	return memberSpanID(callID)
}

func toolSpanID(key string) string {
	return makeSpanID(spanKindTool, key)
}

func actionSpanID(event Event) string {
	key := fmt.Sprintf("%s|%s|%s|%d", event.AgentName, event.RunPath, event.ActionType, event.Sequence)
	return makeSpanID(spanKindAction, key)
}

func attachEventSpans(event *Event) {
	if event == nil {
		return
	}
	memberSpan := ""
	if event.MemberCallID != "" {
		memberSpan = resolvedMemberSpanID(event.MemberCallID)
	} else if event.ParentToolCallID != "" {
		memberSpan = resolvedMemberSpanID(event.ParentToolCallID)
	}

	switch event.Type {
	case EventToolCalls:
		attachToolCallSpans(event, memberSpan)
	case EventToolCallsPreparing:
		attachPreparingSpan(event, memberSpan)
	case EventToolCallsArgsDelta, EventToolResult, EventToolResultChunk:
		attachSingleToolSpan(event, memberSpan)
	case EventAction:
		if memberSpan != "" {
			setEventSpan(event, memberSpan, "")
		} else {
			setEventSpan(event, actionSpanID(*event), "")
		}
	default:
		if memberSpan != "" {
			setEventSpan(event, memberSpan, "")
		} else {
			setEventSpan(event, agentSpanID(*event), "")
		}
	}
}

func attachToolCallSpans(event *Event, memberSpan string) {
	if len(event.ToolCalls) == 0 {
		if memberSpan != "" {
			setEventSpan(event, memberSpan, "")
		}
		return
	}
	if event.ToolCallSpanIDs == nil {
		event.ToolCallSpanIDs = make(map[int]string, len(event.ToolCalls))
	}
	for i, tool := range event.ToolCalls {
		idx := i
		if tool.Index != nil {
			idx = *tool.Index
		}
		ref := ""
		if event.ToolCallRefs != nil {
			ref = event.ToolCallRefs[idx]
		}
		span := lookupToolCallSpan(tool.ID, ref)
		if span == "" {
			span = spanForToolCall(tool.Function.Name, tool.ID, ref, idx)
		}
		event.ToolCallSpanIDs[idx] = span
		registerToolCallSpan(tool.ID, ref, span)
	}
	if memberSpan != "" && event.ParentSpanID == "" {
		event.ParentSpanID = memberSpan
	}
	if len(event.ToolCalls) == 1 {
		idx := 0
		if event.ToolCalls[0].Index != nil {
			idx = *event.ToolCalls[0].Index
		}
		setEventSpan(event, event.ToolCallSpanIDs[idx], memberSpan)
	}
}

func attachPreparingSpan(event *Event, memberSpan string) {
	name := event.ToolName
	id := event.ToolCallID
	ref := event.ToolCallRef
	idx := -1
	if event.ToolCallIndex != nil {
		idx = *event.ToolCallIndex
	}
	if len(event.ToolCalls) > 0 {
		tool := event.ToolCalls[0]
		if name == "" {
			name = tool.Function.Name
		}
		if id == "" {
			id = tool.ID
		}
		if tool.Index != nil {
			idx = *tool.Index
			if ref == "" && event.ToolCallRefs != nil {
				ref = event.ToolCallRefs[idx]
			}
		}
	}
	span := lookupToolCallSpan(id, ref)
	if span == "" {
		span = spanForToolCall(name, id, ref, idx)
	}
	registerToolCallSpan(id, ref, span)
	setEventSpan(event, span, memberSpan)
	if idx >= 0 {
		if event.ToolCallSpanIDs == nil {
			event.ToolCallSpanIDs = map[int]string{}
		}
		event.ToolCallSpanIDs[idx] = span
	}
}

func attachSingleToolSpan(event *Event, memberSpan string) {
	span := lookupToolCallSpan(event.ToolCallID, event.ToolCallRef)
	if span == "" {
		idx := -1
		if event.ToolCallIndex != nil {
			idx = *event.ToolCallIndex
		}
		span = spanForToolCall(event.ToolName, event.ToolCallID, event.ToolCallRef, idx)
	}
	registerToolCallSpan(event.ToolCallID, event.ToolCallRef, span)
	setEventSpan(event, span, memberSpan)
}

func spanForToolCall(name, id, ref string, idx int) string {
	key := id
	if key == "" {
		key = ref
	}
	if key == "" && idx >= 0 {
		key = fmt.Sprintf("idx:%d", idx)
	}
	if key == "" {
		key = name
	}
	if isAgentToolName(name) {
		return memberSpanID(key)
	}
	return toolSpanID(key)
}

func isAgentToolName(name string) bool {
	if name == "" {
		return false
	}
	return agenttool.FormatToolDisplay(name).Kind == agenttool.ToolKindAgent ||
		strings.HasPrefix(name, agenttool.AgentToolPrefix)
}

func setEventSpan(event *Event, span, parent string) {
	if event.SpanID == "" {
		event.SpanID = span
	}
	if event.ParentSpanID == "" {
		event.ParentSpanID = parent
	}
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
	if eventType == EventToolResult {
		nEvent.Content = normalizeToolResultContent(nEvent.Content)
	}
	attachTokenUsage(&nEvent, msg.ResponseMeta)
	if len(msg.ToolCalls) > 0 {
		nEvent.ToolCalls = filterVisibleToolCalls(msg.ToolCalls)
		if nEvent.Content == "" && nEvent.ReasoningContent == "" && len(nEvent.ToolCalls) == 0 {
			return nil
		}
		refGroup := fmt.Sprintf("msg:%d", atomic.AddInt64(&globalStreamSequence, 1))
		nEvent.ToolCallRefs = make(map[int]string, len(nEvent.ToolCalls))
		for _, tc := range nEvent.ToolCalls {
			if tc.Index == nil {
				continue
			}
			idx := *tc.Index
			ref := toolCallRef(event, scope, refGroup, idx)
			nEvent.ToolCallRefs[idx] = ref
			registerToolCallRef(tc.ID, ref)
			registerToolCallOrder(tc.ID, idx)
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
	toolCallRefs     map[int]string            // 按 index 记录展示层稳定引用
	internalToolCall map[int]bool              // 按 index 标记内部工具调用
	streamBase       string
	chunkIndexes     map[string]int64
	segmentIndex     int64
	lastSegmentKey   string
	lastArgsDelta    time.Time
}

const argsDeltaInterval = 100 * time.Millisecond

func newStreamState(streamBase string) *streamState {
	return &streamState{
		toolCallsMap:     make(map[int][]*schema.Message),
		toolCallStarted:  make(map[int]bool),
		toolArgsBuffer:   make(map[int]string),
		toolCallIDs:      make(map[int]string),
		toolCallRefs:     make(map[int]string),
		internalToolCall: make(map[int]bool),
		streamBase:       streamBase,
		chunkIndexes:     make(map[string]int64),
	}
}

func streamBaseID(event *adk.AgentEvent, scope MemberScope, streamSeq int64) string {
	parts := []string{"stream", fmt.Sprintf("seq:%d", streamSeq), event.AgentName, formatRunPath(event.RunPath)}
	if scope.CallID != "" {
		parts = append(parts, "member:"+scope.CallID)
	}
	return strings.Join(parts, "|")
}

func (ss *streamState) applyStreamFields(event *Event, kind ContentKind, suffix string) {
	if event == nil {
		return
	}
	event.ContentKind = kind
	segmentKey := string(kind) + "|" + suffix
	if segmentKey != ss.lastSegmentKey {
		ss.segmentIndex++
		ss.lastSegmentKey = segmentKey
	}
	streamID := ss.streamBase + "|" + fmt.Sprintf("seg:%d", ss.segmentIndex) + "|" + string(kind)
	if suffix != "" {
		streamID += "|" + suffix
	}
	event.StreamID = streamID
	event.ChunkIndex = ss.chunkIndexes[streamID]
	ss.chunkIndexes[streamID]++
}

func (ss *streamState) markStreamSegment(kind ContentKind, suffix string) {
	segmentKey := string(kind) + "|" + suffix
	if segmentKey == ss.lastSegmentKey {
		return
	}
	ss.segmentIndex++
	ss.lastSegmentKey = segmentKey
}

// handleStreamingMessage 处理流式消息，通过 goroutine 异步接收以支持 context 取消
func handleStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream *schema.StreamReader[*schema.Message], scope MemberScope) error {
	streamSeq := atomic.AddInt64(&globalStreamSequence, 1)
	ss := newStreamState(streamBaseID(event, scope, streamSeq))

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
		attachTokenUsage(&nEvent, chunk.ResponseMeta)
		ss.applyStreamFields(&nEvent, ContentKindReasoning, "")
		scope.apply(&nEvent)
		if err := handleEvent(ctx, nEvent); err != nil {
			return err
		}
	}

	if chunk.Content != "" {
		if chunk.Role == schema.Tool && isInternalToolName(chunk.ToolName) {
			return nil
		}
		if chunk.Role == schema.Tool && agenttool.FormatToolDisplay(chunk.ToolName).Kind == "agent" {
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
		if eventType == EventToolResultChunk {
			nEvent.Content = normalizeToolResultContent(nEvent.Content)
		}
		attachTokenUsage(&nEvent, chunk.ResponseMeta)
		kind := ContentKindOutput
		suffix := ""
		if eventType == EventToolResultChunk {
			kind = ContentKindToolResult
			suffix = chunk.ToolCallID
			if suffix == "" {
				suffix = chunk.ToolName
			}
		}
		ss.applyStreamFields(&nEvent, kind, suffix)
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

	if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil &&
		chunk.Content == "" && chunk.ReasoningContent == "" && len(chunk.ToolCalls) == 0 {
		nEvent := Event{
			Type:      EventAction,
			Phase:     EventPhaseInfo,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
		}
		attachTokenUsage(&nEvent, chunk.ResponseMeta)
		scope.apply(&nEvent)
		if err := handleEvent(ctx, nEvent); err != nil {
			return err
		}
	}

	return nil
}

func attachTokenUsage(event *Event, meta *schema.ResponseMeta) {
	if meta == nil || meta.Usage == nil {
		return
	}
	event.PromptTokens = meta.Usage.PromptTokens
	event.CompletionTokens = meta.Usage.CompletionTokens
	event.TotalTokens = meta.Usage.TotalTokens
}

func normalizeToolResultContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return content
	}
	for _, key := range []string{"content", "result", "output", "text"} {
		if text := toolResultValueText(payload[key]); text != "" {
			return text
		}
	}
	return content
}

func toolResultValueText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := toolResultValueText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "result", "output"} {
			if text := toolResultValueText(v[key]); text != "" {
				return text
			}
		}
	}
	return ""
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
			registerToolCallOrder(tc.ID, idx)
		}
		if ss.toolCallRefs[idx] == "" {
			ss.toolCallRefs[idx] = streamToolCallRef(ss.streamBase, idx)
		}
		registerToolCallRef(tc.ID, ss.toolCallRefs[idx])
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
				ToolCallRef:   ss.toolCallRefs[idx],
				ToolCallRefs:  map[int]string{idx: ss.toolCallRefs[idx]},
				ToolCallID:    tc.ID,
				ToolCallIndex: tc.Index,
			}
			scope.apply(&nEvent)
			ss.markStreamSegment(ContentKindToolArgs, ss.toolCallRefs[idx])
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
				ToolCallRef:   ss.toolCallRefs[idx],
				ToolCallID:    ss.toolCallIDs[idx],
				ToolCallIndex: intPtr(idx),
			}
			ss.applyStreamFields(&nEvent, ContentKindToolArgs, ss.toolCallRefs[idx])
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
			ToolCallRef:   ss.toolCallRefs[idx],
			ToolCallID:    ss.toolCallIDs[idx],
			ToolCallIndex: intPtr(idx),
		}
		ss.applyStreamFields(&nEvent, ContentKindToolArgs, ss.toolCallRefs[idx])
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
	toolCallRefs := make(map[int]string, len(indices))
	for _, idx := range indices {
		concatenatedMsg, err := schema.ConcatMessages(ss.toolCallsMap[idx])
		if err != nil {
			return err
		}
		toolCallRefs[idx] = ss.toolCallRefs[idx]
		for _, tc := range concatenatedMsg.ToolCalls {
			registerToolCallRef(tc.ID, ss.toolCallRefs[idx])
			registerToolCallOrder(tc.ID, idx)
		}
		allToolCalls = append(allToolCalls, filterVisibleToolCalls(concatenatedMsg.ToolCalls)...)
	}

	if len(allToolCalls) > 0 {
		nEvent := Event{
			Type:         EventToolCalls,
			Phase:        EventPhaseComplete,
			IsFinal:      true,
			AgentName:    event.AgentName,
			RunPath:      formatRunPath(event.RunPath),
			ToolCalls:    allToolCalls,
			ToolCallRefs: toolCallRefs,
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

// handleEvent 分发事件到 context 中的回调。
func handleEvent(ctx context.Context, event Event) error {
	eventDispatchMu.Lock()
	defer eventDispatchMu.Unlock()

	event = normalizeEvent(event)
	if cb := getCallback(ctx); cb != nil {
		return cb(event)
	}
	return nil
}

// DispatchEvent 向 context 中的回调分发事件
func DispatchEvent(ctx context.Context, event Event) error {
	return handleEvent(ctx, event)
}
