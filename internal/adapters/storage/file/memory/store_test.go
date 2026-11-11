package memory

import (
	domainmemory "fkteams/internal/domain/memory"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadAllMarkdownRoundTrip(t *testing.T) {
	dir := t.TempDir()
	lastHit := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	created := time.Date(2026, 1, 1, 2, 3, 4, 0, time.Local)
	entries := []domainmemory.MemoryEntry{
		{Type: domainmemory.Preference, Summary: "偏好中文", Detail: "回答使用中文", Tags: []string{"中文", "风格"}, CreatedAt: created, HitCount: 3, LastHitAt: &lastHit},
		{Type: domainmemory.Lesson, Summary: "先跑测试", Detail: "提交前完整验证", Tags: []string{"测试"}, CreatedAt: created, HitCount: 1},
	}

	if err := saveAllMarkdown(dir, entries); err != nil {
		t.Fatalf("saveAllMarkdown: %v", err)
	}

	index, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if !strings.Contains(string(index), "用户偏好") || !strings.Contains(string(index), "避坑记录") {
		t.Fatalf("index content = %s", string(index))
	}

	loaded, err := loadAllMarkdown(dir, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded = %#v, want 2 entries", loaded)
	}
	if loaded[0].Summary != "偏好中文" || loaded[0].HitCount != 3 || loaded[0].LastHitAt == nil {
		t.Fatalf("loaded preference = %#v", loaded[0])
	}
	if loaded[1].Summary != "先跑测试" || loaded[1].Type != domainmemory.Lesson {
		t.Fatalf("loaded lesson = %#v", loaded[1])
	}
}

func TestSaveAllMarkdownRemovesEmptyTypeFilesAndIndex(t *testing.T) {
	dir := t.TempDir()
	if err := saveAllMarkdown(dir, []domainmemory.MemoryEntry{{
		Type:      domainmemory.Preference,
		Summary:   "偏好中文",
		Detail:    "回答使用中文",
		CreatedAt: time.Now(),
	}}); err != nil {
		t.Fatalf("save non-empty: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "preference.md")); err != nil {
		t.Fatalf("preference.md should exist: %v", err)
	}

	if err := saveAllMarkdown(dir, nil); err != nil {
		t.Fatalf("save empty: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "preference.md")); !os.IsNotExist(err) {
		t.Fatalf("preference.md should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("MEMORY.md should be removed, err=%v", err)
	}
}

func TestLoadAllMarkdownLimitsEntries(t *testing.T) {
	dir := t.TempDir()
	entries := []domainmemory.MemoryEntry{
		{Type: domainmemory.Preference, Summary: "one", CreatedAt: time.Now()},
		{Type: domainmemory.Preference, Summary: "two", CreatedAt: time.Now()},
		{Type: domainmemory.Preference, Summary: "three", CreatedAt: time.Now()},
	}
	if err := saveAllMarkdown(dir, entries); err != nil {
		t.Fatalf("save markdown: %v", err)
	}
	if loaded, err := loadAllMarkdown(dir, 2); err != nil || len(loaded) != 2 {
		t.Fatalf("loaded entry count = %d, want 2", len(loaded))
	}
}

func TestLoadMarkdownRejectsOversizedFileAndLine(t *testing.T) {
	dir := t.TempDir()
	oversized := filepath.Join(dir, "oversized.md")
	if err := os.WriteFile(oversized, []byte("# memory\n"), 0644); err != nil {
		t.Fatalf("write oversized file: %v", err)
	}
	if err := os.Truncate(oversized, maxMemoryMarkdownBytes+1); err != nil {
		t.Fatalf("truncate oversized file: %v", err)
	}
	if _, err := loadMarkdownFile(oversized, domainmemory.Preference, 1); err == nil {
		t.Fatal("expected oversized memory file to be rejected")
	}

	longLine := filepath.Join(dir, "long-line.md")
	content := "## " + strings.Repeat("x", maxMemoryLineBytes+1)
	if err := os.WriteFile(longLine, []byte(content), 0644); err != nil {
		t.Fatalf("write long-line file: %v", err)
	}
	if _, err := loadMarkdownFile(longLine, domainmemory.Preference, 1); err == nil {
		t.Fatal("expected oversized memory line to be rejected")
	}
}
