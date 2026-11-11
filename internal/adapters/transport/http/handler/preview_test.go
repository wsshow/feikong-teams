package handler

import (
	"context"
	sharestore "fkteams/internal/adapters/storage/file/share"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeStartRejectsCorruptedShareState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "share.json")
	if err := os.WriteFile(path, []byte(`{broken`), 0644); err != nil {
		t.Fatal(err)
	}
	store := sharestore.NewPreviewLinkStore(path)
	if store.LoadError() == nil {
		t.Fatal("expected corrupted share state to fail loading")
	}
	runtime := NewRuntime(RuntimeOptions{PreviewLinks: store})
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("expected runtime start to reject corrupted share state")
	}
}

func newPreviewTestStore(t *testing.T, entries map[string]*sharestore.PreviewEntry) *sharestore.PreviewLinkStore {
	t.Helper()
	store := sharestore.NewPreviewLinkStore(filepath.Join(t.TempDir(), "share.json"))
	for id, entry := range entries {
		if err := store.Put(id, entry); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func newPreviewTestRuntime(t *testing.T, entries map[string]*sharestore.PreviewEntry) *Runtime {
	t.Helper()
	return NewRuntime(RuntimeOptions{PreviewLinks: newPreviewTestStore(t, entries)})
}
