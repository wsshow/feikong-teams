package eventlog

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appchat "fkteams/internal/app/chat"
	domainsession "fkteams/internal/domain/session"
)

func TestMetadataUpdatesAcrossStoresDoNotLoseFields(t *testing.T) {
	root := t.TempDir()
	sessionID := "session-1"
	sessionDir := sessionDirPathForTest(root, sessionID)
	if err := SaveMetadata(sessionDir, &SessionMetadata{
		ID:        sessionID,
		Title:     "initial",
		Status:    domainsession.StatusIdle,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	repository := NewSessionRepository(root)
	chatStore := NewChatSessionStore(root)
	updateEntered := make(chan struct{})
	releaseUpdate := make(chan struct{})
	repositoryDone := make(chan error, 1)
	go func() {
		_, err := repository.UpdateSession(context.Background(), sessionID, func(meta *domainsession.Metadata) error {
			meta.Favorite = true
			close(updateEntered)
			<-releaseUpdate
			return nil
		})
		repositoryDone <- err
	}()
	<-updateEntered

	chatDone := make(chan error, 1)
	go func() {
		chatDone <- chatStore.UpdateMetadata(context.Background(), appchat.MetadataUpdate{
			SessionID: sessionID,
			Status:    appchat.SessionStatusCompleted,
		})
	}()
	close(releaseUpdate)
	if err := <-repositoryDone; err != nil {
		t.Fatal(err)
	}
	if err := <-chatDone; err != nil {
		t.Fatal(err)
	}

	meta, err := LoadMetadata(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Favorite || meta.Status != domainsession.StatusCompleted {
		t.Fatalf("metadata fields were lost: %#v", meta)
	}
}

func sessionDirPathForTest(root, sessionID string) string {
	return filepath.Join(root, sessionID)
}
