package events

import (
	"fmt"

	domainevent "fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

type Emitter struct {
	runID     string
	turnID    string
	sink      runtimeport.EventSink
	lastEvent Event
}

func NewEmitter(runID, turnID string, sink runtimeport.EventSink) *Emitter {
	if sink == nil {
		sink = runtimeport.NoopEventSink
	}
	return &Emitter{
		runID:  runID,
		turnID: turnID,
		sink:   sink,
	}
}

func (e *Emitter) Emit(event Event) error {
	event.RunID = firstNonEmpty(event.RunID, e.runID)
	event.TurnID = firstNonEmpty(event.TurnID, e.turnID)
	e.lastEvent = NormalizeEvent(event)
	if err := ValidateEventContract(e.lastEvent); err != nil {
		return err
	}
	return e.sink(e.lastEvent)
}

func (e *Emitter) LastEvent() Event {
	if e == nil {
		return Event{}
	}
	return e.lastEvent
}

func AgentStart(runID string) Event {
	return Event{Type: EventAgentStarted, RunID: runID}
}

func AgentEnd(runID string) Event {
	return Event{Type: EventAgentCompleted, RunID: runID}
}

func AgentError(runID string, err error) Event {
	event := AgentEnd(runID)
	if err != nil {
		event.Error = err.Error()
	}
	return event
}

func TurnStart(runID, turnID string) Event {
	return Event{Type: EventTurnStarted, RunID: runID, TurnID: turnID}
}

func TurnEnd(runID, turnID string) Event {
	return Event{Type: EventTurnCompleted, RunID: runID, TurnID: turnID}
}

type MessageEvent struct {
	MessageID        string
	Role             message.Role
	AgentName        string
	RunPath          string
	Content          string
	DeltaKind        DeltaKind
	Message          *message.Message
	ToolCallID       string
	ToolCallRef      string
	ToolName         string
	ToolCalls        []message.ToolCall
	ToolCallRefs     map[int]string
	ReasoningContent string
}

func AssistantStarted(meta MessageEvent) Event {
	return Event{
		Type:        EventAssistantStarted,
		MessageID:   meta.MessageID,
		Role:        meta.Role,
		AgentName:   meta.AgentName,
		RunPath:     meta.RunPath,
		Content:     meta.Content,
		Message:     meta.Message,
		ToolCallID:  meta.ToolCallID,
		ToolCallRef: meta.ToolCallRef,
		ToolName:    meta.ToolName,
	}
}

func AssistantDelta(meta MessageEvent, delta string) Event {
	blockType := string(meta.DeltaKind)
	eventType := EventAssistantText
	switch meta.DeltaKind {
	case DeltaReasoning:
		eventType = EventAssistantReasoning
	case DeltaOutput, "":
		eventType = EventAssistantText
		blockType = string(DeltaOutput)
	case DeltaToolArgs:
		eventType = EventToolCallArguments
	case DeltaToolResult:
		eventType = EventToolCallResult
	}
	return Event{
		Type:        eventType,
		MessageID:   meta.MessageID,
		BlockID:     blockID(meta.MessageID, blockType, meta.ToolCallRef),
		BlockType:   blockType,
		Role:        meta.Role,
		AgentName:   meta.AgentName,
		RunPath:     meta.RunPath,
		DeltaKind:   meta.DeltaKind,
		Content:     delta,
		ToolCallID:  meta.ToolCallID,
		ToolCallRef: meta.ToolCallRef,
		ToolName:    meta.ToolName,
	}
}

func AssistantCompleted(meta MessageEvent) Event {
	return Event{
		Type:             EventAssistantCompleted,
		MessageID:        meta.MessageID,
		Role:             meta.Role,
		AgentName:        meta.AgentName,
		RunPath:          meta.RunPath,
		Content:          meta.Content,
		ReasoningContent: meta.ReasoningContent,
		Message:          meta.Message,
		ToolCallID:       meta.ToolCallID,
		ToolCallRef:      meta.ToolCallRef,
		ToolName:         meta.ToolName,
		ToolCalls:        meta.ToolCalls,
		ToolCallRefs:     meta.ToolCallRefs,
	}
}

type ToolEvent struct {
	AgentName     string
	RunPath       string
	ToolCallID    string
	ToolCallRef   string
	ToolName      string
	ToolArgs      string
	ToolResult    string
	Content       string
	ToolCall      *message.ToolCall
	ToolCallIndex *int
}

func ToolCallStarted(meta ToolEvent) Event {
	content := firstNonEmpty(meta.Content, meta.ToolArgs)
	return Event{
		Type:          EventToolCallStarted,
		AgentName:     meta.AgentName,
		RunPath:       meta.RunPath,
		ToolCallID:    meta.ToolCallID,
		ToolCallRef:   meta.ToolCallRef,
		ToolName:      meta.ToolName,
		ToolArgs:      meta.ToolArgs,
		Content:       content,
		ToolCall:      meta.ToolCall,
		ToolCallIndex: meta.ToolCallIndex,
	}
}

func ToolCallResultDelta(meta ToolEvent) Event {
	return Event{
		Type:        EventToolCallResult,
		AgentName:   meta.AgentName,
		RunPath:     meta.RunPath,
		ToolCallID:  meta.ToolCallID,
		ToolCallRef: meta.ToolCallRef,
		ToolName:    meta.ToolName,
		Content:     meta.Content,
		DeltaKind:   DeltaToolResult,
	}
}

func ToolCallCompleted(meta ToolEvent) Event {
	content := firstNonEmpty(meta.Content, meta.ToolResult)
	eventType := EventToolCallCompleted
	if meta.ToolResult == "" && meta.Content == "" {
		eventType = EventToolCallFailed
	}
	return Event{
		Type:        eventType,
		AgentName:   meta.AgentName,
		RunPath:     meta.RunPath,
		ToolCallID:  meta.ToolCallID,
		ToolCallRef: meta.ToolCallRef,
		ToolName:    meta.ToolName,
		Content:     content,
		ToolResult:  firstNonEmpty(meta.ToolResult, content),
	}
}

func SystemNotice(agentName, runPath, code, content string) Event {
	return Event{
		Type:      EventSystemNotice,
		AgentName: agentName,
		RunPath:   runPath,
		Content:   content,
		Notice: &NoticePayload{
			Level:   "info",
			Code:    code,
			Message: content,
		},
	}
}

func Error(agentName, runPath string, err error) Event {
	event := Event{
		Type:      EventError,
		AgentName: agentName,
		RunPath:   runPath,
	}
	if err != nil {
		event.Error = err.Error()
	}
	return event
}

func Usage(agentName, runPath string, promptTokens, completionTokens, totalTokens int) Event {
	return Event{
		Type:             EventUsageReported,
		AgentName:        agentName,
		RunPath:          runPath,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Usage: &domainevent.UsagePayload{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		},
	}
}

func UserMessage(runID, turnID, messageID string, msg message.Message) Event {
	content := msg.DisplayText()
	event := Event{Type: EventUserMessage, MessageID: messageID, Role: message.RoleUser, Content: content, Message: &msg}
	event.RunID = runID
	event.TurnID = turnID
	return event
}

func blockID(messageID, blockType, toolCallRef string) string {
	if messageID == "" || blockType == "" {
		return ""
	}
	if toolCallRef != "" {
		return messageID + ":" + blockType + ":" + toolCallRef
	}
	return messageID + ":" + blockType
}

func TurnID(runID string, index int) string {
	return fmt.Sprintf("%s:turn:%d", runID, index)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
