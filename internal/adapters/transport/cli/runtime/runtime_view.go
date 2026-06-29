package runtime

import (
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/runtime/events"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const runtimeDeltaFlushInterval = 35 * time.Millisecond

type runtimeQueryView struct {
	program *tea.Program
	mu      sync.Mutex
	pending *events.Event
	timer   *time.Timer
}

func (v *runtimeQueryView) Start(input string) {
	v.flushPending()
	v.send(runtimeQueryStartedMsg{input: input})
}

func (v *runtimeQueryView) EventCallback(recorder *eventlog.HistoryRecorder) func(events.Event) error {
	return func(event events.Event) error {
		recorder.RecordEvent(event)
		v.sendEvent(event)
		return nil
	}
}

func (v *runtimeQueryView) Flush() {
	v.flushPending()
}

func (v *runtimeQueryView) Interrupted() {
	v.flushPending()
	v.send(runtimeQueryInterruptedMsg{})
}

func (v *runtimeQueryView) Error(err error) {
	v.flushPending()
	v.send(runtimeQueryErrorMsg{err: err})
}

func (v *runtimeQueryView) Done(elapsed time.Duration) {
	v.flushPending()
	v.send(runtimeQueryDoneMsg{elapsed: elapsed})
}

func (v *runtimeQueryView) CancelRequested() {
	v.send(runtimeCancellingMsg{})
}

func (v *runtimeQueryView) AutoReject() {
	v.send(runtimeStatusMsg{text: "非交互模式，自动拒绝危险命令"})
}

func (v *runtimeQueryView) sendEvent(event events.Event) {
	if !runtimeCanCoalesceDelta(event) {
		v.flushPending()
		v.send(runtimeAgentEventMsg{event: event})
		return
	}

	var flushed *events.Event
	v.mu.Lock()
	if v.pending != nil && runtimeCanMergeDelta(*v.pending, event) {
		runtimeMergeDelta(v.pending, event)
	} else {
		if v.pending != nil {
			copied := *v.pending
			flushed = &copied
		}
		copied := event
		v.pending = &copied
	}
	if v.timer == nil {
		v.timer = time.AfterFunc(runtimeDeltaFlushInterval, v.flushPending)
	}
	v.mu.Unlock()

	if flushed != nil {
		v.send(runtimeAgentEventMsg{event: *flushed})
	}
}

func (v *runtimeQueryView) flushPending() {
	var event *events.Event
	v.mu.Lock()
	if v.pending != nil {
		copied := *v.pending
		event = &copied
		v.pending = nil
	}
	if v.timer != nil {
		v.timer.Stop()
		v.timer = nil
	}
	v.mu.Unlock()
	if event != nil {
		v.send(runtimeAgentEventMsg{event: *event})
	}
}

func runtimeCanCoalesceDelta(event events.Event) bool {
	return (event.Type == events.EventAssistantText || event.Type == events.EventAssistantReasoning) &&
		event.DeltaKind != events.DeltaToolArgs
}

func runtimeCanMergeDelta(a, b events.Event) bool {
	return a.Type == b.Type &&
		a.DeltaKind == b.DeltaKind &&
		a.AgentName == b.AgentName &&
		a.MemberCallID == b.MemberCallID &&
		a.MemberName == b.MemberName &&
		a.ToolName == b.ToolName &&
		a.RunID == b.RunID &&
		a.TurnID == b.TurnID &&
		a.MessageID == b.MessageID
}

func runtimeMergeDelta(base *events.Event, next events.Event) {
	base.Content += next.Content
	base.ReasoningContent += next.ReasoningContent
	base.Sequence = next.Sequence
	if !next.CreatedAt.IsZero() {
		base.CreatedAt = next.CreatedAt
	}
	if next.TotalTokens > 0 {
		base.TotalTokens = next.TotalTokens
	}
}

func (v *runtimeQueryView) send(msg tea.Msg) {
	if v.program != nil {
		v.program.Send(msg)
	}
}

type runtimeQueryStartedMsg struct {
	input string
}

type runtimeAgentEventMsg struct {
	event events.Event
}

type runtimeCancellingMsg struct{}

type runtimeQueryInterruptedMsg struct{}

type runtimeQueryDoneMsg struct {
	elapsed time.Duration
}

type runtimeQueryErrorMsg struct {
	err error
}

type runtimeStatusMsg struct {
	text string
}

type runtimeInternalErrorMsg struct {
	err error
}

type runtimeExitTickMsg time.Time
