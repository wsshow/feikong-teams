package handler

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
	withPreviewStore(t, map[string]*previewLinkEntry{
		"link-1": {
			FilePaths: []string{"docs/report.pdf"},
			CreatedAt: time.Unix(100, 0),
		},
	})

	saveShareLinks()

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
}

func TestLoadShareLinksReadsFromAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	withPreviewStore(t, map[string]*previewLinkEntry{})

	shareDir := filepath.Join(appDir, "share")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"link-2":{"file_paths":["docs/manual.md"],"created_at":200}}`)
	if err := os.WriteFile(filepath.Join(shareDir, "share.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loadShareLinks()

	previewLinkStore.RLock()
	entry := previewLinkStore.m["link-2"]
	previewLinkStore.RUnlock()
	if entry == nil {
		t.Fatal("expected share link to be loaded")
	}
	if got := entry.FilePaths[0]; got != "docs/manual.md" {
		t.Fatalf("unexpected loaded file path: %q", got)
	}
}

func withPreviewStore(t *testing.T, store map[string]*previewLinkEntry) {
	t.Helper()

	previewLinkStore.Lock()
	old := previewLinkStore.m
	previewLinkStore.m = store
	previewLinkStore.Unlock()

	t.Cleanup(func() {
		previewLinkStore.Lock()
		previewLinkStore.m = old
		previewLinkStore.Unlock()
	})
}
