package eventlog

import (
	"fkteams/internal/app/agent/catalog/toolmeta"
	domainhistory "fkteams/internal/domain/history"
	"fkteams/internal/domain/message"
	storageport "fkteams/internal/ports/storage"

	"fkteams/internal/runtime/events"

	"sync"
	"time"
)

type Event = events.Event

const (
	EventAssistantReasoning = events.EventAssistantReasoning
	EventAssistantText      = events.EventAssistantText
	EventToolCallStarted    = events.EventToolCallStarted
	EventToolCallResult     = events.EventToolCallResult
	EventToolCallCompleted  = events.EventToolCallCompleted
	EventSystemNotice       = events.EventSystemNotice
	EventUsageReported      = events.EventUsageReported
	EventError              = events.EventError
)

const TranscriptFileName = "transcript.jsonl"

type ToolCallRecord = domainhistory.ToolCallRecord
type AskRecord = domainhistory.AskRecord
type UsageRecord = domainhistory.UsageRecord
type FriendlyError = domainhistory.FriendlyError
type TranscriptEvent = domainhistory.TranscriptEvent
type TranscriptEventType = domainhistory.TranscriptEventType
type SubagentMetadata = domainhistory.SubagentMetadata
type ToolResultArtifact = domainhistory.ToolResultArtifact
type MsgEventType = domainhistory.MsgEventType
type MessageEvent = domainhistory.MessageEvent
type AgentMessage = domainhistory.AgentMessage
type AttachmentRef = domainhistory.AttachmentRef

const (
	TranscriptUserMessage      = domainhistory.TranscriptUserMessage
	TranscriptAgentStep        = domainhistory.TranscriptAgentStep
	TranscriptAssistantMessage = domainhistory.TranscriptAssistantMessage
	TranscriptToolCallStart    = domainhistory.TranscriptToolCallStart
	TranscriptToolCallEnd      = domainhistory.TranscriptToolCallEnd
	TranscriptAskRequested     = domainhistory.TranscriptAskRequested
	TranscriptAskAnswered      = domainhistory.TranscriptAskAnswered
	TranscriptSystemNotice     = domainhistory.TranscriptSystemNotice
	TranscriptError            = domainhistory.TranscriptError
	TranscriptCancelled        = domainhistory.TranscriptCancelled
	TranscriptRuntime          = domainhistory.TranscriptRuntime
)

const (
	MsgTypeText          = domainhistory.MsgTypeText
	MsgTypeReasoning     = domainhistory.MsgTypeReasoning
	MsgTypeToolCall      = domainhistory.MsgTypeToolCall
	MsgTypeAsk           = domainhistory.MsgTypeAsk
	MsgTypeNotice        = domainhistory.MsgTypeNotice
	MsgTypeUsageReported = domainhistory.MsgTypeUsageReported
	MsgTypeError         = domainhistory.MsgTypeError
	MsgTypeCancelled     = domainhistory.MsgTypeCancelled
)

// NoisyToolPrefixes 定义高输出量噪声工具的名称前缀列表。
// 这类工具（如网页抓取、文档读取）会产生大量输出，在历史上下文中属于冗余内容。
var NoisyToolPrefixes = []string{"fetch", "doc"}

// 错误内容最大长度（rune），超出时保留头尾并截断中间部分
const maxErrorContentLen = 1200

// pendingToolCall 待匹配的工具调用
type pendingToolCall struct {
	Ref         string
	ID          string
	Index       *int
	EventIndex  int
	EventID     string
	Sequence    int64
	CreatedAt   time.Time
	RunID       string
	TurnID      string
	MessageID   string
	Name        string
	Display     toolmeta.ToolDisplay
	DisplayName string
	Kind        string
	Target      string
	Arguments   string
	Extra       map[string]any
}

type assistantMessageKey struct {
	runID     string
	messageID string
}

type pendingAssistantMessage struct {
	event      Event
	firstEvent int
	layout     message.OutputLayout
}

type activeMessageContext struct {
	pendingAssistant map[assistantMessageKey]pendingAssistantMessage
	assistantOrder   []assistantMessageKey
	msg              AgentMessage
	pendingToolCalls []pendingToolCall
	toolResultChunks map[string]string
	order            int
	createdSeq       int64
}

type subagentRun struct {
	AgentRunID       string
	ParentToolCallID string
	ToolName         string
	AgentName        string
	TranscriptPath   string
}

// HistoryRecorder 事件历史记录器
type HistoryRecorder struct {
	eventMu         sync.Mutex
	nextSequence    int64
	messageOrders   map[string]int64
	Documents       storageport.Documents
	mu              sync.RWMutex
	sessionDir      string
	messages        []AgentMessage
	activeMessages  map[string]*activeMessageContext
	activeOrder     []string
	subagents       map[string]*subagentRun
	agentToolCalls  map[string]pendingToolCall
	toolDisplays    toolmeta.Resolver
	persistErr      error
	summary         string // 上下文压缩摘要
	summarizedCount int    // 已被摘要覆盖的消息数量
}

// SetSessionDir 绑定持久化目录；空目录将记录器切换为仅内存模式。
func (h *HistoryRecorder) SetSessionDir(sessionDir string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessionDir = sessionDir
}

// SetToolDisplayResolver 设置当前 recorder 使用的工具展示解析器。
func (h *HistoryRecorder) SetToolDisplayResolver(resolver toolmeta.Resolver) {
	if h == nil || resolver == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.toolDisplays = resolver
}

func (h *HistoryRecorder) formatToolDisplay(name string) toolmeta.ToolDisplay {
	if h != nil && h.toolDisplays != nil {
		return h.toolDisplays.FormatToolDisplay(name)
	}
	return toolmeta.FallbackDisplay(name)
}

func toolCallRecordFromPending(tc pendingToolCall, result string) ToolCallRecord {
	record := ToolCallRecord{
		Ref:         tc.Ref,
		ID:          tc.ID,
		Index:       tc.Index,
		Name:        tc.Name,
		DisplayName: tc.DisplayName,
		Kind:        tc.Kind,
		Target:      tc.Target,
		Arguments:   tc.Arguments,
		Extra:       message.CloneMetadata(tc.Extra),
		Result:      result,
	}
	if record.DisplayName == "" || record.Kind == "" {
		display := tc.Display
		if display.Name == "" {
			display = toolmeta.FallbackDisplay(tc.Name)
		}
		if record.DisplayName == "" {
			record.DisplayName = display.DisplayName
		}
		if record.Kind == "" {
			record.Kind = display.Kind
		}
		if record.Target == "" {
			record.Target = display.Target
		}
	}
	return record
}

func ptrToolCallRecord(record ToolCallRecord) *ToolCallRecord {
	return &record
}

func historyEventEnvelope(event Event, typ MsgEventType) MessageEvent {
	return MessageEvent{
		Incomplete: event.Incomplete,
		Type:       typ,
		EventID:    event.EventID,
		Sequence:   event.Sequence,
		CreatedAt:  event.CreatedAt,
		RunID:      event.RunID,
		TurnID:     event.TurnID,
		MessageID:  event.MessageID,
	}
}

func (h *HistoryRecorder) pendingToolCallFromEvent(event Event, ref, id string, index *int, name, arguments string, extra map[string]any) pendingToolCall {
	display := h.formatToolDisplay(name)
	return pendingToolCall{
		Ref:         ref,
		ID:          id,
		Index:       index,
		EventIndex:  -1,
		EventID:     event.EventID,
		Sequence:    event.Sequence,
		CreatedAt:   event.CreatedAt,
		RunID:       event.RunID,
		TurnID:      event.TurnID,
		MessageID:   event.MessageID,
		Name:        name,
		Display:     display,
		DisplayName: display.DisplayName,
		Kind:        display.Kind,
		Target:      display.Target,
		Arguments:   arguments,
		Extra:       message.CloneMetadata(extra),
	}
}

func (h *HistoryRecorder) appendToolCallEvent(ctx *activeMessageContext, tc pendingToolCall) int {
	record := toolCallRecordFromPending(tc, "")
	ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
		Type:      MsgTypeToolCall,
		EventID:   tc.EventID,
		Sequence:  tc.Sequence,
		CreatedAt: tc.CreatedAt,
		RunID:     tc.RunID,
		TurnID:    tc.TurnID,
		MessageID: tc.MessageID,
		ToolCall:  ptrToolCallRecord(record),
	})
	return len(ctx.msg.Events) - 1
}

func (h *HistoryRecorder) updateToolCallEvent(ctx *activeMessageContext, tc pendingToolCall, result string) bool {
	if tc.EventIndex < 0 || tc.EventIndex >= len(ctx.msg.Events) {
		return false
	}
	if ctx.msg.Events[tc.EventIndex].Type != MsgTypeToolCall || ctx.msg.Events[tc.EventIndex].ToolCall == nil {
		return false
	}
	record := toolCallRecordFromPending(tc, result)
	ctx.msg.Events[tc.EventIndex].ToolCall = ptrToolCallRecord(record)
	ctx.msg.Events[tc.EventIndex].Message = &message.Message{Role: message.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name, Content: result}
	return true
}
