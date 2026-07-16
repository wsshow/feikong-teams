package eventlog

import (
	"context"
	"testing"
	"time"

	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
)

func TestSessionHistoryManagerEvictsLeastRecentlyUsedRecorder(t *testing.T) {
	manager := NewSessionHistoryManagerWithCapacity(2)
	dir := t.TempDir()
	first := manager.GetOrCreate("session-1", dir)
	manager.GetOrCreate("session-2", dir)
	if manager.Get("session-1") != first {
		t.Fatal("expected first recorder to remain cached")
	}
	manager.GetOrCreate("session-3", dir)

	if manager.Get("session-2") != nil {
		t.Fatal("least recently used recorder should be evicted")
	}
	if manager.Get("session-1") == nil || manager.Get("session-3") == nil {
		t.Fatal("recent recorders should remain cached")
	}
}

func TestSessionHistoryManagerDoesNotEvictAcquiredRecorder(t *testing.T) {
	manager := NewSessionHistoryManagerWithCapacity(1)
	dir := t.TempDir()
	first, release := manager.Acquire("session-1", dir)
	manager.GetOrCreate("session-2", dir)

	if manager.Get("session-1") != first {
		t.Fatal("acquired recorder should not be evicted")
	}
	if manager.Remove("session-1") {
		t.Fatal("acquired recorder should not be removable")
	}
	release()

	if manager.Get("session-1") != first {
		t.Fatal("recently released recorder should remain cached")
	}
	if manager.Get("session-2") != nil {
		t.Fatal("cache should return to its configured capacity after release")
	}
	if !manager.Remove("session-1") {
		t.Fatal("idle recorder should be removable")
	}
}

func TestSessionHistoryManagerReleaseIsIdempotent(t *testing.T) {
	manager := NewSessionHistoryManagerWithCapacity(1)
	recorder, release := manager.Acquire("session-1", t.TempDir())
	release()
	release()
	if manager.Get("session-1") != recorder {
		t.Fatal("duplicate release should not corrupt the cache entry")
	}
}

func TestSessionRepositoryRejectsDeleteWhileRecorderIsAcquired(t *testing.T) {
	root := t.TempDir()
	sessionID := "session-1"
	if err := SaveMetadata(sessionDirPathForTest(root, sessionID), &SessionMetadata{
		ID:        sessionID,
		Title:     "active",
		Status:    domainsession.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	manager := NewSessionHistoryManager()
	_, release := manager.Acquire(sessionID, root)
	repository := NewSessionRepository(root)
	if err := repository.DeleteSession(context.Background(), sessionID); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("DeleteSession() error = %v, want conflict", err)
	}

	release()
	if err := repository.DeleteSession(context.Background(), sessionID); err != nil {
		t.Fatalf("DeleteSession() after release: %v", err)
	}
}
