package chat

import (
	"context"
	"errors"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/session"
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
	err       error
	finalized bool
}

func (h *failingHistory) FinalizeCurrent()      { h.finalized = true }
func (*failingHistory) RecordEvent(event.Event) {}
func (*failingHistory) RecordCancelled(string)  {}
func (h *failingHistory) Flush() error          { return h.err }

func TestFinishPersistsErrorStatusWhenHistoryFails(t *testing.T) {
	store := &metadataStoreSpy{}
	history := &failingHistory{err: errors.New("disk full")}
	err := NewSessionLifecycle(store).Finish(context.Background(), FinishRequest{
		SessionID: "session", Status: session.StatusCompleted, History: history, FinalizeHistory: true,
	})
	if !errors.Is(err, history.err) || store.update.Status != session.StatusError || !history.finalized {
		t.Fatalf("finish = %v, metadata = %+v, finalized = %v", err, store.update, history.finalized)
	}
}
