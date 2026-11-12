package eventlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fkteams/internal/adapters/storage/sqlite"
	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
	storageport "fkteams/internal/ports/storage"
)

func TestClearSessionHistoryPreservesIdentityAndOtherSessions(t *testing.T) {
	for _, database := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "database"}[database], func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			var documents storageport.Documents
			if database {
				db, err := sqlite.Open(ctx, filepath.Join(root, "app.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				documents, err = sqlite.NewDocuments(db, root)
				if err != nil {
					t.Fatal(err)
				}
			}
			repo := NewSessionRepository(filepath.Join(root, "sessions"), documents)
			meta := domainsession.Metadata{ID: domainsession.AssistantID, Title: "小空", Mode: "team", CurrentAgent: domainsession.AssistantAgent, Favorite: true}
			if _, _, err := repo.CreateSession(ctx, meta); err != nil {
				t.Fatal(err)
			}
			if _, _, err := repo.CreateSession(ctx, domainsession.Metadata{ID: "other", Title: "other"}); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(repo.root, meta.ID)
			for _, path := range []string{TranscriptFileName, "queue.json", "subagents/member/transcript.jsonl", "tool-results/result.txt"} {
				if err := writeDocument(filepath.Join(dir, path), []byte("old content"), documents); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(dir, "attachments"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "attachments/file.txt"), []byte("attachment"), 0644); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dir, "attachments/linked")); err != nil {
				t.Fatal(err)
			}
			release := AcquireSessionLease(repo.root, meta.ID)
			if err := repo.ClearSessionHistory(ctx, meta.ID); !apperror.IsCode(err, apperror.CodeConflict) {
				t.Fatalf("active clear = %v", err)
			}
			release()
			if err := repo.ClearSessionHistory(ctx, meta.ID); err != nil {
				t.Fatal(err)
			}
			if got, err := repo.LoadSession(ctx, meta.ID); err != nil || got != meta {
				t.Fatalf("identity = %#v, %v", got, err)
			}
			if _, err := repo.LoadSession(ctx, "other"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "attachments")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("attachments retained: %v", err)
			}
			if data, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(data) != "keep" {
				t.Fatalf("linked attachment escaped: %q, %v", data, err)
			}
			for _, path := range []string{TranscriptFileName, "queue.json", "subagents/member/transcript.jsonl", "tool-results/result.txt"} {
				if _, err := readDocument(filepath.Join(dir, path), documents); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("retained %s: %v", path, err)
				}
			}
			manager := NewSessionHistoryManager()
			manager.Documents = documents
			if got := manager.GetOrCreate(meta.ID, repo.root).GetMessages(); len(got) != 0 {
				t.Fatalf("restored old messages: %v", got)
			}
			if err := repo.ClearSessionHistory(ctx, "../outside"); !apperror.IsCode(err, apperror.CodeInvalidArgument) {
				t.Fatalf("escape = %v", err)
			}
		})
	}
}

func TestClearSessionHistoryRejectsSymlinks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "metadata.json"), []byte(`{"id":"outside"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	repo := NewSessionRepository(root)
	if err := repo.ClearSessionHistory(ctx, "linked"); err == nil {
		t.Fatal("accepted linked session directory")
	}
	if err := os.Mkdir(filepath.Join(root, "session"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "metadata.json"), filepath.Join(root, "session/metadata.json")); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClearSessionHistory(ctx, "session"); err == nil {
		t.Fatal("accepted linked metadata")
	}
	if data, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(data) != "keep" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

type failingClearDocuments struct{ storageport.Documents }

func (s failingClearDocuments) RemoveTree(string, ...string) error {
	return errors.New("storage unavailable")
}

func TestClearSessionHistoryDoesNotAcknowledgeFailedCommit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := sqlite.Open(ctx, filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	documents, err := sqlite.NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewSessionRepository(filepath.Join(root, "sessions"), failingClearDocuments{documents})
	if _, _, err := repo.CreateSession(ctx, domainsession.Metadata{ID: domainsession.AssistantID, Title: "小空"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo.root, domainsession.AssistantID, TranscriptFileName)
	if err := documents.Append(path, []byte("old history")); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClearSessionHistory(ctx, domainsession.AssistantID); err == nil {
		t.Fatal("acknowledged failed clear")
	}
	if data, err := documents.Read(path); err != nil || string(data) != "old history" {
		t.Fatalf("history changed: %q, %v", data, err)
	}
}
