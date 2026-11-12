package eventlog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainmessage "fkteams/internal/domain/message"
	runtimeevents "fkteams/internal/runtime/events"
)

func TestSessionMessageReaderLoadsActiveRecorder(t *testing.T) {
	manager := NewSessionHistoryManager()
	recorder := manager.GetOrCreate("active-session", t.TempDir())
	recorder.RecordEvent(runtimeevents.UserMessage("run-1", runtimeevents.TurnID("run-1", 1), "msg-1", domainmessage.Message{Role: domainmessage.RoleUser, Content: "hello"}))

	messages, err := NewSessionMessageReader(t.TempDir(), manager).LoadSessionMessages(context.Background(), "active-session")
	if err != nil {
		t.Fatalf("LoadSessionMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].GetTextContent() != "hello" {
		t.Fatalf("messages = %#v, want active recorder message", messages)
	}
}

func TestSessionMessageReaderRejectsEscapingSymlinks(t *testing.T) {
	for _, target := range []string{"session", "transcript", "subagents"} {
		t.Run(target, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			sessionDir := filepath.Join(root, "s")
			if target == "session" {
				if err := os.Symlink(outside, sessionDir); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(sessionDir, 0755); err != nil {
					t.Fatal(err)
				}
				name := TranscriptFileName
				if target == "subagents" {
					name = subagentsDirName
				}
				if err := os.Symlink(outside, filepath.Join(sessionDir, name)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := NewSessionMessageReader(root, nil).LoadSessionMessages(context.Background(), "s"); err == nil {
				t.Fatal("accepted escaping symlink")
			}
		})
	}
}

func TestSessionMessageReaderDoesNotRepairTranscript(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "s")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, TranscriptFileName)
	content := `{"type":"user_message","content":"hello"}` + "\n" + `{"type":`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSessionMessageReader(root, nil).LoadSessionMessages(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != content {
		t.Fatalf("query modified transcript: %q, %v", got, err)
	}
}

func TestSessionQueriesRejectMetadataSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "s"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outside, "metadata.json")
	if err := os.WriteFile(path, []byte(`{"id":"s","title":"outside"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(root, "s", "metadata.json")); err != nil {
		t.Fatal(err)
	}
	repo := NewSessionRepository(root)
	if _, err := repo.ListSessions(context.Background()); err == nil {
		t.Fatal("listed metadata outside root")
	}
	if _, err := repo.LoadSession(context.Background(), "s"); err == nil {
		t.Fatal("loaded metadata outside root")
	}
}

func TestSessionMessageReaderLoadsPersistedHistory(t *testing.T) {
	dir := t.TempDir()
	sessionID := "persisted-session"
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(filepath.Join(dir, sessionID))
	recorder.RecordEvent(runtimeevents.UserMessage("run-1", runtimeevents.TurnID("run-1", 1), "msg-1", domainmessage.Message{Role: domainmessage.RoleUser, Content: "from disk"}))
	if err := recorder.SaveToFile(filepath.Join(dir, sessionID, TranscriptFileName)); err != nil {
		t.Fatalf("SaveToFile returned error: %v", err)
	}

	messages, err := NewSessionMessageReader(dir, NewSessionHistoryManager()).LoadSessionMessages(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadSessionMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].GetTextContent() != "from disk" {
		t.Fatalf("messages = %#v, want persisted message", messages)
	}
}
