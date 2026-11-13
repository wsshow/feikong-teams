package runtime

import (
	"context"
	"encoding/json"
	"errors"
	eventlog "fkteams/internal/adapters/storage/file/history"
	filequeue "fkteams/internal/adapters/storage/file/queue"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

type failingQueryRunner struct{ err error }

func (r failingQueryRunner) Run(_ context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if err := opts.Sink(event.Event{Type: event.TypeAssistantText, MessageID: "answer", Content: "partial", Role: message.RoleAssistant, AgentName: "assistant"}); err != nil {
		return nil, err
	}
	if r.err == nil {
		return &runtimeport.RunResult{}, opts.Sink(event.Event{Type: event.TypeAssistantCompleted, MessageID: "answer", Content: "partial", Role: message.RoleAssistant, AgentName: "assistant"})
	}
	return nil, r.err
}

func TestExecuteReturnsFailureAndKeepsTemporaryHistoryInMemory(t *testing.T) {
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	session := NewSession(ModeTeam, nil, nil)
	session.SetTemporary(true)
	session.activateSession(false)
	runErr := errors.New("model unavailable")
	executor := NewQueryExecutor(failingQueryRunner{err: runErr}, NewQueryState())
	executor.SetSession(session)
	executor.SetView(callbackQueryView{})
	if err := executor.Execute(context.Background(), "hello"); !errors.Is(err, runErr) {
		t.Fatalf("Execute = %v", err)
	}
	if session.recorder().GetMessageCount() == 0 {
		t.Fatal("temporary history was not retained")
	}
	if _, err := os.Stat(session.historyDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary history persisted: %v", err)
	}
}

func TestPresentationCallbackDoesNotOwnHistory(t *testing.T) {
	for _, temporary := range []bool{false, true} {
		t.Run(fmt.Sprint(temporary), func(t *testing.T) {
			t.Setenv("FEIKONG_APP_DIR", t.TempDir())
			session := NewSession(ModeTeam, nil, nil)
			session.SetTemporary(temporary)
			session.activateSession(false)
			executor := NewQueryExecutor(failingQueryRunner{}, NewQueryState())
			executor.SetSession(session)
			displayed := 0
			executor.SetCallbackBuilder(func() func(event.Event) error { return func(event.Event) error { displayed++; return nil } })
			if err := executor.Execute(context.Background(), "hello"); err != nil {
				t.Fatal(err)
			}
			if displayed != 2 || session.recorder().GetMessageCount() != 1 {
				t.Fatalf("displayed=%d messages=%d", displayed, session.recorder().GetMessageCount())
			}
			transcript := filepath.Join(session.historyDir, session.sessionID(), eventlog.TranscriptFileName)
			_, err := os.Stat(transcript)
			if temporary && !errors.Is(err, os.ErrNotExist) || !temporary && err != nil {
				t.Fatalf("temporary=%v transcript=%v", temporary, err)
			}
		})
	}
}

func TestSignalHandlerStopsAndUnsubscribes(t *testing.T) {
	session := NewSession(ModeTeam, nil, nil)
	signals, exit := make(chan os.Signal, 1), make(chan os.Signal, 1)
	stop := session.handleSignals(signals, exit)
	signals <- syscall.SIGTERM
	select {
	case sig := <-exit:
		if sig != syscall.SIGTERM {
			t.Fatal(sig)
		}
	case <-time.After(time.Second):
		t.Fatal("signal not forwarded")
	}
	stop()
	stop()
	signals <- syscall.SIGTERM
	select {
	case <-exit:
		t.Fatal("stopped handler forwarded signal")
	case <-time.After(20 * time.Millisecond):
	}
	// 真实 Notify 注册也必须可重复启动、停止，不留下 signal 包持有的通道。
	for range 3 {
		stopReal := session.StartSignalHandler(exit)
		stopReal()
		stopReal()
	}
}

type interactionQueryView struct{ callbackQueryView }

func (interactionQueryView) PromptAsk(context.Context, *ask.AskInfo) (*ask.AskResponse, error) {
	return &ask.AskResponse{Selected: []string{"B"}, FreeText: "terminal answer"}, nil
}
func (interactionQueryView) PromptApproval(context.Context, string) (int, error) { return 0, nil }

type questionQueryRunner struct{ queues *appchat.QueueService }

func (r questionQueryRunner) Run(ctx context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	emitter := events.NewEmitter(opts.RunID, events.TurnID(opts.RunID, 1), opts.Sink)
	callID := opts.RunID + ":ask"
	if err := emitter.Emit(events.ToolCallStarted(events.ToolEvent{ToolCallID: callID, ToolCallRef: "tool_call:" + callID, ToolName: "ask_questions", ToolArgs: `{"question":"choose?","options":["A","B"]}`})); err != nil {
		return nil, err
	}
	decisions, err := opts.InterruptHandler(ctx, []runtimeport.Interrupt{{ID: callID, IsRootCause: true, Info: &ask.AskInfo{Question: "choose?", Options: []string{"A", "B"}}}})
	if err != nil {
		return nil, err
	}
	response := decisions[callID].(*ask.AskResponse)
	encoded, _ := json.Marshal(ask.AskResult{Selected: response.Selected, FreeText: response.FreeText})
	if err := emitter.Emit(events.ToolCallCompleted(events.ToolEvent{ToolCallID: callID, ToolCallRef: "tool_call:" + callID, ToolName: "ask_questions", ToolResult: string(encoded)})); err != nil {
		return nil, err
	}
	sid, _ := domainsession.IDFromContext(ctx)
	view, _ := r.queues.Streams().Get(sid).PresentationSnapshot()
	if len(view.Items) == 0 || view.Items[len(view.Items)-1].Ask == nil || !view.Items[len(view.Items)-1].Ask.Answered {
		return nil, fmt.Errorf("web view did not receive terminal answer")
	}
	return &runtimeport.RunResult{}, emitter.Emit(events.TurnEnd(opts.RunID, events.TurnID(opts.RunID, 1)))
}

func TestTerminalQuestionsPublishAndReplayInSharedWebStream(t *testing.T) {
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	queues := appchat.NewQueueService(taskstream.NewManager(), filequeue.NewStore(t.TempDir()), nil)
	session := NewSession(ModeTeam, nil, nil)
	session.activateSession(false)
	executor := NewQueryExecutor(questionQueryRunner{queues}, NewQueryState())
	executor.SetSession(session)
	executor.SetView(interactionQueryView{})
	for range 2 {
		if err := executor.Execute(appchat.WithQueues(context.Background(), queues), "ask me"); err != nil {
			t.Fatal(err)
		}
	}
	stream := queues.Streams().Get(session.sessionID())
	live, _ := stream.PresentationSnapshot()
	facts, err := session.recorder().ReplayEvents()
	if err != nil {
		t.Fatal(err)
	}
	replay := presentation.New(session.sessionID())
	for _, fact := range facts {
		replay.Apply(fact)
	}
	if !reflect.DeepEqual(live, replay.Snapshot()) {
		t.Fatalf("shared stream differs from committed transcript: live=%+v replay=%+v", live, replay.Snapshot())
	}
	asks := 0
	for _, item := range live.Items {
		if item.Ask != nil {
			asks++
			if item.Ask.Active || !item.Ask.Answered || item.Ask.FreeText != "terminal answer" {
				t.Fatalf("answer=%+v", item.Ask)
			}
		}
	}
	if asks != 2 || stream.ClientStatus() != "completed" {
		t.Fatalf("asks=%d status=%s", asks, stream.ClientStatus())
	}
}
