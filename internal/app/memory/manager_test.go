package memory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerFlushExtractPersistsAndLoadsMarkdown(t *testing.T) {
	workspace := t.TempDir()
	llm := &fakeLLMClient{response: `[{"type":"preference","summary":"偏好中文","detail":"回复使用中文且简洁","tags":["中文","简洁"]}]`}
	manager := NewManager(workspace, llm, nil)

	manager.FlushExtract(context.Background(), longConversationMessages(), "session-1")

	if manager.Count() != 1 {
		t.Fatalf("count = %d, want 1", manager.Count())
	}
	if llm.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", llm.calls)
	}
	if _, err := os.Stat(filepath.Join(workspace, "memory", "preference.md")); err != nil {
		t.Fatalf("preference.md should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "memory", "MEMORY.md")); err != nil {
		t.Fatalf("MEMORY.md should exist: %v", err)
	}

	reloaded := NewManager(workspace, nil, nil)
	entries := reloaded.List()
	if len(entries) != 1 || entries[0].Summary != "偏好中文" || entries[0].Type != Preference {
		t.Fatalf("reloaded entries = %#v", entries)
	}
}

func TestManagerFlushExtractSkipsShortContent(t *testing.T) {
	llm := &fakeLLMClient{response: `[]`}
	manager := NewManager(t.TempDir(), llm, nil)

	manager.FlushExtract(context.Background(), []Message{{Role: "user", Content: "短内容"}}, "session-1")

	if llm.calls != 0 {
		t.Fatalf("llm calls = %d, want 0", llm.calls)
	}
	if manager.Count() != 0 {
		t.Fatalf("count = %d, want 0", manager.Count())
	}
}

func TestManagerListDeleteAndClear(t *testing.T) {
	manager := NewManager(t.TempDir(), nil, nil)
	manager.entries = []MemoryEntry{
		{ID: "1", Type: Preference, Summary: "偏好中文", Detail: "中文回复", CreatedAt: time.Now()},
		{ID: "2", Type: Fact, Summary: "Go 工程师", Detail: "熟悉 Go", CreatedAt: time.Now()},
	}
	manager.rebuildIndex()

	list := manager.List()
	list[0].Summary = "外部修改"
	if manager.List()[0].Summary != "偏好中文" {
		t.Fatal("List should return a copy")
	}

	if deleted := manager.Delete(" 偏好中文 "); deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if manager.Count() != 1 {
		t.Fatalf("count after delete = %d, want 1", manager.Count())
	}
	if deleted := manager.Delete("missing"); deleted != 0 {
		t.Fatalf("deleted missing = %d, want 0", deleted)
	}

	manager.Clear()
	if manager.Count() != 0 {
		t.Fatalf("count after clear = %d, want 0", manager.Count())
	}
	if _, err := os.Stat(filepath.Join(manager.storeDir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("MEMORY.md should be removed after clear, err=%v", err)
	}
}

func TestManagerShouldExtract(t *testing.T) {
	manager := NewManager(t.TempDir(), nil, nil)
	longMessages := []Message{
		{Role: "user", Content: strings.Repeat("用户偏好", 80)},
		{Role: "assistant", Content: "收到"},
		{Role: "user", Content: strings.Repeat("继续补充", 80)},
	}

	if !manager.shouldExtract(longMessages, time.Time{}) {
		t.Fatal("expected long conversation with two user messages to extract")
	}
	if manager.shouldExtract(longMessages[:2], time.Time{}) {
		t.Fatal("expected single user message to skip extraction")
	}
	if manager.shouldExtract(longMessages, time.Now()) {
		t.Fatal("expected cooldown to skip extraction")
	}
	if manager.shouldExtract([]Message{{Role: "user", Content: "短"}, {Role: "user", Content: "短"}}, time.Time{}) {
		t.Fatal("expected short content to skip extraction")
	}
}

func TestManagerWaitTracksAsyncWorkAndRejectsNewTasks(t *testing.T) {
	llm := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(t.TempDir(), llm, nil)
	messages := []Message{
		{Role: "user", Content: strings.Repeat("用户偏好", 80)},
		{Role: "assistant", Content: "收到"},
		{Role: "user", Content: strings.Repeat("继续补充", 80)},
	}

	if !manager.ExtractAndStoreAsync(messages, "session-1") {
		t.Fatal("async extraction should be accepted before shutdown")
	}
	<-llm.started
	waited := make(chan struct{})
	go func() {
		_ = manager.Wait(context.Background())
		close(waited)
	}()

	select {
	case <-waited:
		t.Fatal("Wait returned before extraction completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(llm.release)
	<-waited

	if manager.ExtractAndStoreAsync(messages, "session-2") {
		t.Fatal("async extraction should be rejected after shutdown starts")
	}
	if calls := llm.calls.Load(); calls != 1 {
		t.Fatalf("llm calls = %d, want 1", calls)
	}
}

func TestManagerWaitTracksHitStatsUpdate(t *testing.T) {
	manager := NewManager(t.TempDir(), nil, nil)
	manager.entries = []MemoryEntry{{
		ID: "1", Type: Preference, Summary: "偏好中文", Detail: "使用中文回复",
	}}
	manager.rebuildIndex()

	if entries := manager.Search("偏好中文", 1); len(entries) != 1 {
		t.Fatalf("search entries = %#v, want one", entries)
	}
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if entries := manager.List(); entries[0].HitCount != 1 {
		t.Fatalf("hit count = %d, want 1", entries[0].HitCount)
	}
}

func TestManagerWaitHonorsContextDeadline(t *testing.T) {
	llm := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(t.TempDir(), llm, nil)
	messages := []Message{
		{Role: "user", Content: strings.Repeat("用户偏好", 80)},
		{Role: "assistant", Content: "收到"},
		{Role: "user", Content: strings.Repeat("继续补充", 80)},
	}
	if !manager.ExtractAndStoreAsync(messages, "session-1") {
		t.Fatal("async extraction should be accepted")
	}
	<-llm.started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := manager.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait error = %v, want deadline exceeded", err)
	}
	close(llm.release)
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatalf("second Wait returned error: %v", err)
	}
}

func TestManagerRejectsDuplicateSessionExtraction(t *testing.T) {
	llm := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(t.TempDir(), llm, nil)
	messages := asyncExtractionMessages()
	if !manager.ExtractAndStoreAsync(messages, "session-1") {
		t.Fatal("first extraction should be accepted")
	}
	<-llm.started
	if manager.ExtractAndStoreAsync(messages, "session-1") {
		t.Fatal("duplicate extraction for the same session should be rejected")
	}
	close(llm.release)
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatalf("wait for extraction: %v", err)
	}
	if calls := llm.calls.Load(); calls != 1 {
		t.Fatalf("llm calls = %d, want 1", calls)
	}
}

func TestManagerLimitsConcurrentExtractions(t *testing.T) {
	llm := &concurrentLLMClient{
		started: make(chan struct{}, maxConcurrentExtractions),
		release: make(chan struct{}),
	}
	manager := NewManager(t.TempDir(), llm, nil)
	messages := asyncExtractionMessages()
	for i := 0; i < maxConcurrentExtractions; i++ {
		if !manager.ExtractAndStoreAsync(messages, fmt.Sprintf("session-%d", i)) {
			t.Fatalf("extraction %d should be accepted", i)
		}
	}
	if manager.ExtractAndStoreAsync(messages, "session-overflow") {
		t.Fatal("extraction above the global limit should be rejected")
	}
	for i := 0; i < maxConcurrentExtractions; i++ {
		select {
		case <-llm.started:
		case <-time.After(time.Second):
			t.Fatal("accepted extraction did not start")
		}
	}
	close(llm.release)
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatalf("wait for extractions: %v", err)
	}
	if maximum := llm.maximum.Load(); maximum > maxConcurrentExtractions {
		t.Fatalf("maximum concurrent extractions = %d", maximum)
	}
}

func TestManagerBoundsTrackedSessionProgress(t *testing.T) {
	manager := NewManager(t.TempDir(), nil, nil)
	manager.mu.Lock()
	for i := 0; i <= maxTrackedSessions; i++ {
		manager.setSessionProgressLocked(fmt.Sprintf("session-%d", i), i, time.Now())
	}
	count := len(manager.sessionAccess)
	_, newestExists := manager.sessionAccess[fmt.Sprintf("session-%d", maxTrackedSessions)]
	manager.mu.Unlock()
	if count != maxTrackedSessions {
		t.Fatalf("tracked session count = %d, want %d", count, maxTrackedSessions)
	}
	if !newestExists {
		t.Fatal("newest session progress should be retained")
	}
}

func TestManagerDuplicateDetection(t *testing.T) {
	manager := NewManager(t.TempDir(), nil, nil)
	manager.entries = []MemoryEntry{{
		Type:    Preference,
		Summary: "偏好简洁回复",
		Detail:  "少说废话",
		Tags:    []string{"风格", "简洁", "中文"},
	}}

	if action, _ := manager.checkDuplicate(MemoryEntry{Type: Preference, Summary: "偏好简洁回复"}); action != actionSkip {
		t.Fatalf("same summary action = %v, want skip", action)
	}
	if action, _ := manager.checkDuplicate(MemoryEntry{Type: Preference, Summary: "用户偏好简洁回复"}); action != actionUpdate {
		t.Fatalf("similar summary action = %v, want update", action)
	}
	if action, _ := manager.checkDuplicate(MemoryEntry{Type: Preference, Summary: "其他", Tags: []string{"风格", "简洁"}}); action != actionUpdate {
		t.Fatalf("overlap tags action = %v, want update", action)
	}
	if action, _ := manager.checkDuplicate(MemoryEntry{Type: Fact, Summary: "偏好简洁回复"}); action != actionAdd {
		t.Fatalf("different type action = %v, want add", action)
	}
}

type blockingLLMClient struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

type concurrentLLMClient struct {
	started chan struct{}
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func asyncExtractionMessages() []Message {
	return []Message{
		{Role: "user", Content: strings.Repeat("用户偏好", 80)},
		{Role: "assistant", Content: "收到"},
		{Role: "user", Content: strings.Repeat("继续补充", 80)},
	}
}

func (c *concurrentLLMClient) Complete(ctx context.Context, _ string) (string, error) {
	active := c.active.Add(1)
	defer c.active.Add(-1)
	for {
		maximum := c.maximum.Load()
		if active <= maximum || c.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	c.started <- struct{}{}
	select {
	case <-c.release:
		return `[]`, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *blockingLLMClient) Complete(ctx context.Context, _ string) (string, error) {
	c.calls.Add(1)
	close(c.started)
	select {
	case <-c.release:
		return `[]`, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
