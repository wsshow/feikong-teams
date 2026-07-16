package handler

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChunkUploadStoreLimitsActiveUploads(t *testing.T) {
	store := NewChunkUploadStore(chunkUploadStoreOptions{
		rootDir:   t.TempDir(),
		maxActive: 1,
		maxBytes:  1024,
	})
	first, err := store.getOrCreate("first", 1, "/tmp/first", "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.getOrCreate("second", 1, "/tmp/second", "second"); !errors.Is(err, errChunkUploadCapacity) {
		t.Fatalf("second getOrCreate() error = %v, want capacity", err)
	}

	first.mu.Lock()
	first.Completed = true
	store.finish(first)
	first.mu.Unlock()
	if _, err := store.getOrCreate("second", 1, "/tmp/second", "second"); err != nil {
		t.Fatalf("getOrCreate() after finish: %v", err)
	}
}

func TestChunkUploadStoreLimitsAggregateBytes(t *testing.T) {
	store := NewChunkUploadStore(chunkUploadStoreOptions{
		rootDir:   t.TempDir(),
		maxActive: 2,
		maxBytes:  100,
	})
	if !store.reserveBytes(60) {
		t.Fatal("first reservation should succeed")
	}
	if store.reserveBytes(50) {
		t.Fatal("reservation beyond aggregate limit should fail")
	}
	if !store.reserveBytes(-20) || !store.reserveBytes(50) {
		t.Fatal("released capacity should be reusable")
	}
	if got := store.activeBytes.Load(); got != 90 {
		t.Fatalf("active bytes = %d, want 90", got)
	}
}

func TestChunkUploadStoreLimitsConcurrentRequests(t *testing.T) {
	store := NewChunkUploadStore(chunkUploadStoreOptions{
		rootDir:       t.TempDir(),
		maxActive:     2,
		maxBytes:      100,
		maxConcurrent: 1,
	})
	if !store.beginRequest() {
		t.Fatal("first request should acquire a slot")
	}
	if store.beginRequest() {
		t.Fatal("second concurrent request should be rejected")
	}
	store.endRequest()
	if !store.beginRequest() {
		t.Fatal("released request slot should be reusable")
	}
	store.endRequest()
}

func TestChunkUploadStoreExpirationReleasesAccounting(t *testing.T) {
	root := t.TempDir()
	store := NewChunkUploadStore(chunkUploadStoreOptions{
		rootDir:   root,
		maxActive: 2,
		maxBytes:  100,
	})
	meta, err := store.getOrCreate("expired", 1, "/tmp/file", "file")
	if err != nil {
		t.Fatal(err)
	}
	if !store.reserveBytes(60) {
		t.Fatal("reservation should succeed")
	}
	meta.mu.Lock()
	meta.TotalBytes = 60
	meta.UpdatedAt = time.Now().Add(-chunkUploadTTL - time.Second)
	chunkDir := meta.ChunkDir
	meta.mu.Unlock()
	if err := os.MkdirAll(chunkDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunkDir, "0"), []byte("chunk"), 0600); err != nil {
		t.Fatal(err)
	}

	store.removeExpired(time.Now())
	if got := store.activeBytes.Load(); got != 0 {
		t.Fatalf("active bytes after expiration = %d, want 0", got)
	}
	store.Lock()
	active := store.active
	remaining := len(store.m)
	store.Unlock()
	if active != 0 || remaining != 0 {
		t.Fatalf("store after expiration: active=%d remaining=%d", active, remaining)
	}
	if _, err := os.Stat(chunkDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired chunk directory still exists: %v", err)
	}
}
