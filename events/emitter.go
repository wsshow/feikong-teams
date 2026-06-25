package events

import (
	"fmt"

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
	return Event{Type: EventAgentStart, RunID: runID}
}

func AgentEnd(runID string) Event {
	return Event{Type: EventAgentEnd, RunID: runID}
}

func AgentError(runID string, err error) Event {
	event := AgentEnd(runID)
	if err != nil {
		event.Error = err.Error()
	}
	return event
}

func TurnStart(runID, turnID string) Event {
	return Event{Type: EventTurnStart, RunID: runID, TurnID: turnID}
}

func TurnEnd(runID, turnID string) Event {
	return Event{Type: EventTurnEnd, RunID: runID, TurnID: turnID}
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

func MessageStart(meta MessageEvent) Event {
	return Event{
		Type:        EventMessageStart,
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

func MessageDelta(meta MessageEvent, delta string) Event {
	return Event{
		Type:        EventMessageDelta,
		MessageID:   meta.MessageID,
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

func MessageEnd(meta MessageEvent) Event {
	return Event{
		Type:             EventMessageEnd,
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

func ToolStart(meta ToolEvent) Event {
	content := firstNonEmpty(meta.Content, meta.ToolArgs)
	return Event{
		Type:          EventToolStart,
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

func ToolUpdate(meta ToolEvent) Event {
	return Event{
		Type:        EventToolUpdate,
		AgentName:   meta.AgentName,
		RunPath:     meta.RunPath,
		ToolCallID:  meta.ToolCallID,
		ToolCallRef: meta.ToolCallRef,
		ToolName:    meta.ToolName,
		Content:     meta.Content,
		DeltaKind:   DeltaToolResult,
	}
}

func ToolEnd(meta ToolEvent) Event {
	content := firstNonEmpty(meta.Content, meta.ToolResult)
	return Event{
		Type:        EventToolEnd,
		AgentName:   meta.AgentName,
		RunPath:     meta.RunPath,
		ToolCallID:  meta.ToolCallID,
		ToolCallRef: meta.ToolCallRef,
		ToolName:    meta.ToolName,
		Content:     content,
		ToolResult:  firstNonEmpty(meta.ToolResult, content),
	}
}

func Action(agentName, runPath string, actionType ActionType, content string) Event {
	return Event{
		Type:       EventAction,
		AgentName:  agentName,
		RunPath:    runPath,
		ActionType: actionType,
		Content:    content,
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
		Type:             EventUsage,
		AgentName:        agentName,
		RunPath:          runPath,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

func UserMessagePair(runID, turnID, messageID string, msg message.Message) (Event, Event) {
	content := msg.DisplayText()
	meta := MessageEvent{
		MessageID: messageID,
		Role:      message.RoleUser,
		Content:   content,
		Message:   &msg,
	}
	start := MessageStart(meta)
	end := MessageEnd(meta)
	start.RunID = runID
	start.TurnID = turnID
	end.RunID = runID
	end.TurnID = turnID
	return start, end
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
