package runtime

import (
	"context"
	"errors"
	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"os"
	"path/filepath"
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
