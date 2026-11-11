package memory

import (
	"errors"
	domainmemory "fkteams/internal/domain/memory"
	"reflect"
	"testing"
)

func TestEditorPersistsBeforePublishing(t *testing.T) {
	store := &memoryStoreFake{}
	manager := newTestManager(t, store, nil, nil)
	entry, err := manager.SaveEntry(EditRequest{Type: domainmemory.Fact, Summary: "原始内容", Detail: "详情"})
	if err != nil {
		t.Fatal(err)
	}
	before := manager.List()
	failure := errors.New("storage unavailable")
	store.saveErr = failure
	if _, err := manager.SaveEntry(EditRequest{ID: entry.ID, Type: domainmemory.Fact, Summary: "修改内容"}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := manager.DeleteEntry(entry.ID); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, manager.List()) {
		t.Fatal("failed write changed visible memory")
	}
	store.saveErr = nil
	saved, err := manager.SaveEntry(EditRequest{ID: entry.ID, Type: domainmemory.Fact, Summary: "修改内容"})
	if err != nil || saved.ID != entry.ID || saved.CreatedAt != entry.CreatedAt {
		t.Fatalf("identity changed: %#v, %v", saved, err)
	}
	if err := manager.DeleteEntry(entry.ID); err != nil {
		t.Fatal(err)
	}
	if manager.Count() != 0 {
		t.Fatal("entry not deleted")
	}
}

func TestEditorRejectsInvalidAndUnknownEntries(t *testing.T) {
	manager := newTestManager(t, &memoryStoreFake{}, nil, nil)
	for _, req := range []EditRequest{{Type: domainmemory.Fact}, {Type: "unknown", Summary: "summary"}} {
		if _, err := manager.SaveEntry(req); !errors.Is(err, ErrInvalidEntry) {
			t.Fatal(err)
		}
	}
	if _, err := manager.SaveEntry(EditRequest{ID: "missing", Type: domainmemory.Fact, Summary: "summary"}); !errors.Is(err, ErrEntryNotFound) {
		t.Fatal(err)
	}
	if manager.Count() != 0 {
		t.Fatal("invalid writes changed memory")
	}
}

func TestDisabledAutomaticMemoryStillAllowsManagement(t *testing.T) {
	store := &memoryStoreFake{}
	manager := newTestManager(t, store, nil, nil)
	manager.SetEnabled(false)
	entry, err := manager.SaveEntry(EditRequest{Type: domainmemory.Fact, Summary: "manual memory"})
	if err != nil {
		t.Fatal(err)
	}
	if manager.Count() != 1 {
		t.Fatal("manual memory unavailable")
	}
	if got := manager.Search("manual memory", 10); len(got) != 0 {
		t.Fatal("disabled automatic retrieval returned memory")
	}
	if err := manager.DeleteEntry(entry.ID); err != nil {
		t.Fatal(err)
	}
}

func TestManagerPreservesSnapshotBeyondConfiguredCapacity(t *testing.T) {
	store := &memoryStoreFake{entries: []MemoryEntry{{ID: "one", Type: domainmemory.Fact, Summary: "one"}, {ID: "two", Type: domainmemory.Fact, Summary: "two"}}}
	manager := newTestManager(t, store, nil, &Config{MaxEntries: 1})
	if manager.Count() != 2 {
		t.Fatal("stored memory was truncated")
	}
	if _, err := manager.SaveEntry(EditRequest{ID: "one", Type: domainmemory.Fact, Summary: "edited"}); err != nil {
		t.Fatal(err)
	}
	if len(store.entries) != 2 {
		t.Fatal("editing lost unviewed memory")
	}
}
