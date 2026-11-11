package runtime

import (
	"context"
	"errors"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"os"
	"testing"
)

type failingQueryRunner struct{ err error }

func (r failingQueryRunner) Run(_ context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if err := opts.Sink(event.Event{Type: event.TypeAssistantText, MessageID: "answer", Content: "partial", Role: message.RoleAssistant, AgentName: "assistant"}); err != nil {
		return nil, err
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
