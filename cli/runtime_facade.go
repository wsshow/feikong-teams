package cli

import (
	"fkteams/appstate"
	cliruntime "fkteams/cli/runtime"
	"fkteams/events"
	eventlog "fkteams/internal/adapters/storage/file/history"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

type ModeRunnerCreator = cliruntime.ModeRunnerCreator
type Session = cliruntime.Session
type QueryState = cliruntime.QueryState
type QueryExecutor = cliruntime.QueryExecutor
type QueryView = cliruntime.QueryView
type TerminalQueryView = cliruntime.TerminalQueryView

const CLISessionID = cliruntime.CLISessionID

var CLIHistoryDir = cliruntime.CLIHistoryDir

func NewSession(mode WorkMode, inputHistory []string, createModeRunner ModeRunnerCreator) *Session {
	return cliruntime.NewSession(mode, inputHistory, createModeRunner)
}

func NewQueryState() *QueryState {
	return cliruntime.NewQueryState()
}

func NewQueryExecutor(runner runtimeport.Runner, state *QueryState) *QueryExecutor {
	return cliruntime.NewQueryExecutor(runner, state)
}

func NewTerminalQueryView() *TerminalQueryView {
	return cliruntime.NewTerminalQueryView()
}

func SetResumeSessionID(sessionID string) {
	cliruntime.SetResumeSessionID(sessionID)
}

func SetTemporarySession(v bool) {
	cliruntime.SetTemporarySession(v)
}

func IsTemporarySession() bool {
	return cliruntime.IsTemporarySession()
}

func BuildTurnInput(input string) domainmessage.TurnInput {
	return cliruntime.BuildTurnInput(input)
}

func BuildTurnInputWithMemory(input string, manager appstate.MemoryManager) domainmessage.TurnInput {
	return cliruntime.BuildTurnInputWithMemory(input, manager)
}

func HandleCtrlC(state *QueryState) {
	cliruntime.HandleCtrlC(state)
}

func SaveChatHistoryToHTML() (string, error) {
	return cliruntime.SaveChatHistoryToHTML()
}

func FlushSessionMemory() {
	cliruntime.FlushSessionMemory()
}

func FlushSessionMemoryWithManager(manager appstate.MemoryManager) {
	cliruntime.FlushSessionMemoryWithManager(manager)
}

func SaveCLISessionHistory() bool {
	return cliruntime.SaveCLISessionHistory()
}

func ListSessions(interactive ...bool) {
	cliruntime.ListSessions(interactive...)
}

func PrintResumeHint() {
	cliruntime.PrintResumeHint()
}

func GetWorkspaceDir() string {
	return cliruntime.GetWorkspaceDir()
}

func SetCallbackBuilder(session *Session, cb func(*eventlog.HistoryRecorder) func(events.Event) error) {
	session.SetCallbackBuilder(cb)
}
