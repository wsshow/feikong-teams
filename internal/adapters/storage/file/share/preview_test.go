package share

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fkteams/internal/runtime/env"
)

func TestShareLinksFilePathUsesAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	want := filepath.Join(appDir, "share", "share.json")
	if got := shareLinksFilePath(); got != want {
		t.Fatalf("unexpected share file path: got %q, want %q", got, want)
	}
}

func TestSaveShareLinksWritesToAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	store := NewPreviewLinkStore("")
	store.mu.Lock()
	store.m = map[string]*PreviewEntry{
		"link-1": {
			FilePaths:     []string{"docs/report.pdf"},
			ResourcePaths: []string{"docs/report.pdf", "docs/cover.png"},
			CreatedAt:     time.Unix(100, 0),
		},
	}
	store.mu.Unlock()

	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(appDir, "share", "share.json"))
	if err != nil {
		t.Fatalf("read share file: %v", err)
	}
	var entries map[string]*shareFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal share file: %v", err)
	}
	if got := entries["link-1"].FilePaths[0]; got != "docs/report.pdf" {
		t.Fatalf("unexpected saved file path: %q", got)
	}
	if got := entries["link-1"].ResourcePaths; len(got) != 2 || got[1] != "docs/cover.png" {
		t.Fatalf("unexpected saved resource paths: %#v", got)
	}
}

func TestLoadShareLinksReadsFromAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	shareDir := filepath.Join(appDir, "share")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"link-2":{"file_paths":["docs/manual.md"],"created_at":200}}`)
	if err := os.WriteFile(filepath.Join(shareDir, "share.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	store := NewPreviewLinkStore("")

	store.mu.RLock()
	entry := store.m["link-2"]
	store.mu.RUnlock()
	if entry == nil {
		t.Fatal("expected share link to be loaded")
	}
	if got := entry.FilePaths[0]; got != "docs/manual.md" {
		t.Fatalf("unexpected loaded file path: %q", got)
	}
	if !entry.AllowsResource("docs/manual.md") || entry.AllowsResource("docs/private.txt") {
		t.Fatalf("legacy entry resource boundary is invalid: %#v", entry.ResourcePaths)
	}
}

func TestPreviewStoreSaveReportsFilesystemFailure(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	store := NewPreviewLinkStore(filepath.Join(blocker, "share.json"))
	if err := store.Put("link-1", &PreviewEntry{FilePaths: []string{"a.txt"}, CreatedAt: time.Now()}); err == nil {
		t.Fatal("expected save failure")
	}
	store.mu.RLock()
	_, exists := store.m["link-1"]
	store.mu.RUnlock()
	if exists {
		t.Fatal("failed transaction should roll back in-memory state")
	}
}
