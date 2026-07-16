package inputhistory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSaveAndLoadHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "input.txt")
	if err := Save(path, []string{"one", "two", "three"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path, 2)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{"two", "three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %#v, want %#v", got, want)
	}
}

func TestLoadMissingHistory(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.txt"), 10)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Load missing = %#v, want empty", got)
	}
}

func TestSaveAndLoadMultilineHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	want := []string{"first line\nsecond line", "普通输入"}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save multiline: %v", err)
	}
	got, err := Load(path, 10)
	if err != nil {
		t.Fatalf("Load multiline: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %#v, want %#v", got, want)
	}
}

func TestSaveBoundsHistoryAndLineSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	history := make([]string, maxStoredHistoryLines+5)
	for i := range history {
		history[i] = strings.Repeat("你", maxHistoryLineBytes)
	}
	if err := Save(path, history); err != nil {
		t.Fatalf("Save bounded history: %v", err)
	}
	got, err := Load(path, maxStoredHistoryLines+10)
	if err != nil {
		t.Fatalf("Load bounded history: %v", err)
	}
	if len(got) != maxStoredHistoryLines {
		t.Fatalf("history line count = %d, want %d", len(got), maxStoredHistoryLines)
	}
	for i, line := range got {
		if len(line) > maxHistoryLineBytes || !utf8.ValidString(line) {
			t.Fatalf("history line %d is invalid or oversized", i)
		}
	}
}

func TestLoadRejectsOversizedHistoryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("history"), 0644); err != nil {
		t.Fatalf("write history: %v", err)
	}
	if err := os.Truncate(path, maxHistoryFileBytes+1); err != nil {
		t.Fatalf("truncate history: %v", err)
	}
	if _, err := Load(path, 10); err == nil {
		t.Fatal("expected oversized history file to be rejected")
	}
}
