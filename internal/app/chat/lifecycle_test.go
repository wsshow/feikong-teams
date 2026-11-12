package chat

import (
	"context"
	"errors"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"testing"
)

type metadataStoreSpy struct {
	update MetadataUpdate
}

func (s *metadataStoreSpy) UpdateMetadata(_ context.Context, update MetadataUpdate) error {
	s.update = update
	return nil
}

func TestMarkProcessingCreatesMissingSessionMetadata(t *testing.T) {
	store := &metadataStoreSpy{}
	lifecycle := NewSessionLifecycle(store)

	if err := lifecycle.MarkProcessing(context.Background(), "session-1", "用户问题"); err != nil {
		t.Fatalf("mark processing: %v", err)
	}

	if !store.update.CreateIfMissing {
		t.Fatal("processing metadata update should create missing session metadata")
	}
	if store.update.SessionID != "session-1" || store.update.TitleSource != "用户问题" || store.update.Status != session.StatusProcessing {
		t.Fatalf("metadata update = %#v", store.update)
	}
}

func TestMarkProcessingWithTargetPersistsExecutionSelection(t *testing.T) {
	store := &metadataStoreSpy{}
	lifecycle := NewSessionLifecycle(store)

	if err := lifecycle.MarkProcessingWithTarget(context.Background(), "session-1", "用户问题", ExecutionTarget{
		Mode:         "deep",
		CurrentAgent: "coder",
	}); err != nil {
		t.Fatalf("mark processing with target: %v", err)
	}

	if store.update.Mode == nil || *store.update.Mode != "deep" {
		t.Fatalf("mode update = %#v", store.update.Mode)
	}
	if store.update.CurrentAgent == nil || *store.update.CurrentAgent != "coder" {
		t.Fatalf("agent update = %#v", store.update.CurrentAgent)
	}
}

type failingHistory struct {
	events        []event.Event
	err           error
	finalized     bool
	finalizations int
	flushes       int
	cancellations int
}

func (h *failingHistory) FinalizeCurrent()          { h.finalized = true; h.finalizations++ }
func (h *failingHistory) RecordEvent(e event.Event) { h.events = append(h.events, e) }
func (h *failingHistory) RecordCancelled(string)    { h.cancellations++ }
func (h *failingHistory) Flush() error              { h.flushes++; return h.err }

type lifecycleRunner struct{ err error }

func (r lifecycleRunner) Run(context.Context, message.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return nil, r.err
}

type reportingErrorRunner struct{ err error }

func (r reportingErrorRunner) Run(_ context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if err := opts.Sink(event.Event{Type: event.TypeError, Error: r.err.Error()}); err != nil {
		return nil, err
	}
	return nil, r.err
}

func TestRunTurnRecordsReturnedErrorOnlyWhenNotAlreadyReported(t *testing.T) {
	runErr := errors.New("deepseek does not support image_url type")
	for _, runner := range []runtimeport.Runner{lifecycleRunner{runErr}, reportingErrorRunner{runErr}} {
		history := &failingHistory{}
		_, err := NewService().RunTurn(context.Background(), TurnRequest{SessionID: "session", Runner: runner, History: history})
		if !errors.Is(err, runErr) {
			t.Fatalf("lost run error: %v", err)
		}
		if len(history.events) != 1 || history.events[0].Type != event.TypeError {
			t.Fatalf("history should contain one error: %+v", history.events)
		}
	}
}

func TestTurnFinalizesHistoryOnceBeforeMetadata(t *testing.T) {
	for _, runErr := range []error{nil, context.Canceled, errors.New("model failed")} {
		for _, persistErr := range []error{nil, errors.New("disk full"), context.Canceled} {
			store := &metadataStoreSpy{}
			history := &failingHistory{err: persistErr}
			_, err := NewService().RunTurn(context.Background(), TurnRequest{
				SessionID: "session", Runner: lifecycleRunner{err: runErr}, History: history,
				OnFinish: func(ctx context.Context, _ *runtimeport.RunResult, resultErr error) error {
					if history.flushes != 1 || history.finalizations != 1 {
						t.Fatalf("history=%+v", history)
					}
					return NewSessionLifecycle(store).Finish(ctx, FinishRequest{SessionID: "session", Error: resultErr})
				},
			})
			want := session.StatusCompleted
			if runErr == context.Canceled {
				want = session.StatusCancelled
			} else if runErr != nil {
				want = session.StatusError
			}
			if persistErr != nil {
				want = session.StatusError
			}
			if store.update.Status != want || CompletionStatus(err) != want || history.flushes != 1 || history.finalizations != 1 {
				t.Fatalf("run=%v persist=%v metadata=%s task=%s history=%+v", runErr, persistErr, store.update.Status, CompletionStatus(err), history)
			}
			if runErr != nil && !errors.Is(err, runErr) || persistErr != nil && !errors.Is(err, persistErr) {
				t.Fatalf("lost error: %v", err)
			}
		}
	}
}

func TestCompletionStatusDoesNotHideOtherFailuresBehindCancellation(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want session.Status
	}{
		{fmt.Errorf("runner: %w", context.Canceled), session.StatusCancelled},
		{errors.Join(context.Canceled, errors.New("disk failed")), session.StatusError},
		{errors.Join(context.Canceled, finalizationFailure(context.Canceled)), session.StatusError},
	} {
		if got := CompletionStatus(tc.err); got != tc.want {
			t.Fatalf("%v: %s != %s", tc.err, got, tc.want)
		}
	}
}

func (h *failingHistory) RecordAndEmit(e event.Event, sink func(event.Event) error) error {
	h.RecordEvent(e)
	if sink != nil {
		return sink(e)
	}
	return nil
}
