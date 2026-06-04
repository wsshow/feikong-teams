package cli

import (
	"fkteams/eventlog"
	"fkteams/fkevent"
	"time"

	tea "charm.land/bubbletea/v2"
)

type runtimeQueryView struct {
	program *tea.Program
}

func (v *runtimeQueryView) Start(input string) {
	v.send(runtimeQueryStartedMsg{input: input})
}

func (v *runtimeQueryView) EventCallback(recorder *eventlog.HistoryRecorder) func(fkevent.Event) error {
	return func(event fkevent.Event) error {
		recorder.RecordEvent(event)
		v.send(runtimeAgentEventMsg{event: event})
		return nil
	}
}

func (v *runtimeQueryView) Flush() {}

func (v *runtimeQueryView) Interrupted() {
	v.send(runtimeQueryInterruptedMsg{})
}

func (v *runtimeQueryView) Error(err error) {
	v.send(runtimeQueryErrorMsg{err: err})
}

func (v *runtimeQueryView) Done(elapsed time.Duration) {
	v.send(runtimeQueryDoneMsg{elapsed: elapsed})
}

func (v *runtimeQueryView) CancelRequested() {
	v.send(runtimeCancellingMsg{})
}

func (v *runtimeQueryView) AutoReject() {
	v.send(runtimeStatusMsg{text: "非交互模式，自动拒绝危险命令"})
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
	event fkevent.Event
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

type runtimeLegacyCommandDoneMsg struct {
	command string
}

type runtimeInternalErrorMsg struct {
	err error
}

type runtimeExitTickMsg time.Time
