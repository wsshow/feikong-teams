package runtime

import (
	"context"

	appagent "fkteams/internal/app/agent"
	"fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/tools/ask"
	runtimeport "fkteams/internal/ports/runtime"

	"fmt"
	"os"

	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	runtimeExitConfirmWindow = 2 * time.Second
	runtimeExitConfirmTick   = time.Second
	runtimeSelectionNotice   = 2 * time.Second
	runtimeHorizontalGutter  = 1
	runtimeDefaultAgentName  = "assistant"
	runtimeDefaultToolName   = "tool"
	runtimeWheelLines        = 1
	runtimeFastWheelLines    = 5
)

type Runtime struct {
	ctx         context.Context
	session     *Session
	runner      runtimeport.Runner
	executor    *QueryExecutor
	askBroker   *runtimeAskBroker
	approval    *runtimeApprovalBroker
	exitSignals chan os.Signal
	program     *tea.Program
}

func NewRuntime(ctx context.Context, session *Session, r runtimeport.Runner, exitSignals chan os.Signal) *Runtime {
	executor := NewQueryExecutor(r, session.queryState)
	executor.SetSession(session)
	executor.SetMemoryManager(session.memory)
	executor.SetScheduleService(session.scheduler)
	return &Runtime{
		ctx:         ctx,
		session:     session,
		runner:      r,
		executor:    executor,
		exitSignals: exitSignals,
	}
}

func (r *Runtime) Run() error {
	defer resetTerminalModes()

	view := &runtimeQueryView{}
	askBroker := newRuntimeAskBroker(view.send)
	approvalBroker := newRuntimeApprovalBroker(view.send)
	r.askBroker = askBroker
	r.approval = approvalBroker
	r.executor.SetApproveStores(r.session.ApproveStores)
	r.executor.SetView(view)
	r.executor.SetAskRuntimeHandler(askBroker.Handle)
	view.approval = approvalBroker

	model := newRuntimeModel(r)
	p := tea.NewProgram(model)
	r.program = p
	view.program = p

	_, err := p.Run()
	return err
}

func resetTerminalModes() {
	_, _ = fmt.Fprint(os.Stdout, "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1015l\x1b[?1049l\x1b[?25h")
}

func (r *Runtime) submitQuery(input string) tea.Cmd {
	return func() tea.Msg {
		if err := r.executor.Execute(r.ctx, input); err != nil {
			return runtimeInternalErrorMsg{err: err}
		}
		return nil
	}
}

func (r *Runtime) requestCancel() tea.Cmd {
	return func() tea.Msg {
		if r.session.queryState.Cancel() {
			return runtimeCancellingMsg{}
		}
		return nil
	}
}

func (r *Runtime) queueSteering(input string) bool {
	if r == nil || r.executor == nil {
		return false
	}
	return r.executor.QueueSteering(input)
}

func (r *Runtime) drainSteeringText() string {
	if r == nil || r.executor == nil {
		return ""
	}
	return r.executor.DrainSteeringText()
}

func (r *Runtime) submitAsk(askID string, resp *ask.AskResponse) bool {
	if r == nil || r.askBroker == nil {
		return false
	}
	return r.askBroker.Submit(askID, resp)
}

func (r *Runtime) submitApproval(id string, decision int) bool {
	if r == nil || r.approval == nil {
		return false
	}
	return r.approval.Submit(id, decision)
}

func (r *Runtime) requestExit() {
	select {
	case r.exitSignals <- syscall.SIGTERM:
	default:
	}
}

func (r *Runtime) switchAgent(agentName string) (string, error) {
	agentInfo, err := agents.AgentByName(r.ctx, agentName)
	if err != nil {
		return "", err
	}
	if agentInfo == nil {
		return "", fmt.Errorf("agent not found: %s", agentName)
	}
	newAgent, err := agentInfo.Creator(r.ctx)
	if err != nil {
		return "", fmt.Errorf("create agent %s: %w", agentName, err)
	}
	newRunner, err := appagent.CreateAgentRunner(r.ctx, newAgent)
	if err != nil {
		return "", err
	}
	r.executor.SetRunner(newRunner)
	r.session.currentAgent = agentName
	return fmt.Sprintf("已切换到智能体: %s (%s)", agentName, agentInfo.Description), nil
}
