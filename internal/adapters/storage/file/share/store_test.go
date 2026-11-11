package share

import (
	"bytes"
	"errors"
	"fkteams/internal/runtime/env"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestShareStoreSnapshotsAndFailedWritesStayIsolated(t *testing.T) {
	store := NewPreviewLinkStore(filepath.Join(t.TempDir(), "shares.json"))
	entry := NewPreviewEntry([]string{"report.html"}, nil, "", time.Time{}, time.Now())
	if err := store.Put("link", entry); err != nil {
		t.Fatal(err)
	}
	entry.FilePaths[0] = "changed.html"
	snapshot := store.Snapshot()
	snapshot["link"].FilePaths[0] = "snapshot.html"
	current, ok := store.Lookup("link")
	if !ok || current.FilePaths[0] != "report.html" {
		t.Fatalf("entry changed through external alias: %+v", current)
	}
	store.filePath = t.TempDir()
	if err := store.DeleteMany([]string{"link"}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if _, ok := store.Lookup("link"); !ok {
		t.Fatal("failed deletion changed live state")
	}
	if err := store.Put("other", entry); err == nil {
		t.Fatal("expected persistence failure")
	}
	if _, ok := store.Lookup("other"); ok {
		t.Fatal("failed insertion changed live state")
	}
}

func TestSessionShareLookupCanRunWhileTouching(t *testing.T) {
	store := NewSessionShareStore(filepath.Join(t.TempDir(), "shares.json"))
	if err := store.Put("link", &SessionEntry{SessionID: "session", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 50 {
			entry, ok, _ := store.EntryByID("link")
			if ok {
				entry.Title = "private copy"
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			if err := store.Touch("link", time.Now()); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
	if entry, _, _ := store.EntryByID("link"); entry.Title != "" {
		t.Fatal("lookup exposed mutable entry")
	}
}

func TestPersistedShareStoreRejectsOversizedAndMalformedFiles(t *testing.T) {
	oversizedPath := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(oversizedPath, bytes.Repeat([]byte{'x'}, maxPersistedShareStoreBytes+1), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPersistedShareStore(oversizedPath); err == nil {
		t.Fatal("oversized share store was accepted")
	}

	previewPath := filepath.Join(t.TempDir(), "preview.json")
	if err := os.WriteFile(previewPath, []byte(`{"broken":null}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := NewPreviewLinkStore(previewPath).LoadError(); err == nil {
		t.Fatal("malformed preview link entry was accepted")
	}

	sessionPath := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(sessionPath, []byte(`{"broken":null}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := NewSessionShareStore(sessionPath).LoadError(); err == nil {
		t.Fatal("malformed session share entry was accepted")
	}
}

func TestShareStoresEnforceEntryLimit(t *testing.T) {
	previewStore := NewPreviewLinkStore(filepath.Join(t.TempDir(), "preview.json"))
	previewStore.mu.Lock()
	for index := range maxPersistedShareEntries {
		previewStore.m[fmt.Sprintf("link-%d", index)] = NewPreviewEntry([]string{"file.txt"}, nil, "", time.Time{}, time.Now())
	}
	previewStore.mu.Unlock()
	if err := previewStore.Put("overflow", NewPreviewEntry([]string{"file.txt"}, nil, "", time.Time{}, time.Now())); !errors.Is(err, errShareStoreFull) {
		t.Fatalf("preview overflow error = %v", err)
	}

	sessionStore := NewSessionShareStore(filepath.Join(t.TempDir(), "session.json"))
	sessionStore.mu.Lock()
	for index := range maxPersistedShareEntries {
		sessionStore.m[fmt.Sprintf("share-%d", index)] = &SessionEntry{SessionID: "session-1"}
	}
	sessionStore.mu.Unlock()
	if err := sessionStore.Put("overflow", &SessionEntry{SessionID: "session-1"}); !errors.Is(err, errShareStoreFull) {
		t.Fatalf("session overflow error = %v", err)
	}
}

func TestSessionShareTouchThrottlesPersistence(t *testing.T) {
	store := NewSessionShareStore(filepath.Join(t.TempDir(), "session.json"))
	store.mu.Lock()
	store.m["share-1"] = &SessionEntry{SessionID: "session-1", CreatedAt: time.Now()}
	store.mu.Unlock()

	first := time.Unix(1000, 0)
	if err := store.Touch("share-1", first); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch("share-1", first.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	store.mu.RLock()
	lastAccessedAt := store.m["share-1"].LastAccessedAt
	store.mu.RUnlock()
	if !lastAccessedAt.Equal(first) {
		t.Fatalf("last access = %v, want throttled value %v", lastAccessedAt, first)
	}

	second := first.Add(6 * time.Minute)
	if err := store.Touch("share-1", second); err != nil {
		t.Fatal(err)
	}
	store.mu.RLock()
	lastAccessedAt = store.m["share-1"].LastAccessedAt
	store.mu.RUnlock()
	if !lastAccessedAt.Equal(second) {
		t.Fatalf("last access = %v, want %v", lastAccessedAt, second)
	}
}

func TestSessionSharesFilePathUsesAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	want := filepath.Join(appDir, "share", "session_share.json")
	if got := sessionSharesFilePath(); got != want {
		t.Fatalf("unexpected session share file path: got %q, want %q", got, want)
	}
}
