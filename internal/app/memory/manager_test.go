package memory

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

type memoryStoreFake struct {
	entries          []MemoryEntry
	loadErr, saveErr error
}

func (s *memoryStoreFake) Load(limit int) ([]MemoryEntry, error) {
	return cloneEntries(s.entries[:min(max(limit, 0), len(s.entries))]), s.loadErr
}
func (s *memoryStoreFake) Save(entries []MemoryEntry) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.entries = cloneEntries(entries)
	return nil
}
func newTestManager(t *testing.T, store *memoryStoreFake, cfg *Config) *Manager {
	t.Helper()
	m, err := NewManager(store, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestManagerAtomicWritesAndRestart(t *testing.T) {
	store := &memoryStoreFake{}
	m := newTestManager(t, store, nil)
	entry, err := m.SaveEntry(EditRequest{Type: Preference, Summary: "回答简洁", Tags: []string{"风格"}, SessionID: "source"})
	if err != nil {
		t.Fatal(err)
	}
	entry.Tags[0] = "outside"
	if m.List()[0].Tags[0] != "风格" {
		t.Fatal("returned alias mutated state")
	}
	before := m.List()
	store.saveErr = errors.New("storage unavailable")
	if _, err := m.Delete("回答简洁"); err == nil {
		t.Fatal("delete hid failure")
	}
	if err := m.Clear(); err == nil {
		t.Fatal("clear hid failure")
	}
	if _, err := m.SaveEntry(EditRequest{ID: entry.ID, Version: entry.Version, Type: Preference, Summary: "回答详细"}); err == nil {
		t.Fatal("edit hid failure")
	}
	if !reflect.DeepEqual(m.List(), before) {
		t.Fatal("failed writes changed memory")
	}
	store.saveErr = nil
	m = newTestManager(t, store, nil)
	updated, err := m.SaveEntry(EditRequest{ID: entry.ID, Version: entry.Version, Type: Preference, Summary: "回答详细"})
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if err := m.Forget(updated.ID, updated.Version); err != nil {
		t.Fatal(err)
	}
	if newTestManager(t, store, nil).Count() != 0 {
		t.Fatal("deleted entry returned after restart")
	}
}
func TestManagerConcurrentEditsRejectStaleVersion(t *testing.T) {
	m := newTestManager(t, &memoryStoreFake{}, nil)
	entry, err := m.SaveEntry(EditRequest{Type: Fact, Summary: "主题", Detail: "初始"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, detail := range []string{"第一份修改", "第二份修改"} {
		wg.Go(func() {
			_, err := m.SaveEntry(EditRequest{ID: entry.ID, Version: entry.Version, Type: Fact, Summary: "主题", Detail: detail})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	if err := m.Forget(entry.ID, entry.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("stale delete accepted", err)
	}
}
func TestManagerCapacityAndScope(t *testing.T) {
	m := newTestManager(t, &memoryStoreFake{}, &Config{MaxEntries: 2, MaxChars: 100})
	pref, err := m.SaveEntry(EditRequest{Type: Preference, Summary: "简短回复"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.SaveEntry(EditRequest{Type: Preference, Scope: "项目A", Summary: "使用中文"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.SaveEntry(EditRequest{Type: Fact, Summary: "第三条"}); !errors.Is(err, ErrInvalidEntry) {
		t.Fatal("capacity not enforced", err)
	}
	if m.Count() != 2 {
		t.Fatal("capacity evicted existing data")
	}
	got := m.Search("使用中文", 5)
	if len(got) != 1 || got[0].ID != pref.ID {
		t.Fatalf("scoped preference leaked: %+v", got)
	}
	got, next, more := m.Recall("", "", 0, 1)
	if len(got) != 1 || !more || next != 1 {
		t.Fatalf("pagination: %+v %d %v", got, next, more)
	}
	got, _, _ = m.Recall("", "项目B", 0, 20)
	if len(got) != 1 || got[0].ID != pref.ID {
		t.Fatal("scope filter failed")
	}
	m.SetEnabled(false)
	if len(m.Search("简短", 5)) != 0 {
		t.Fatal("disabled context injection")
	}
}
func TestManagerNeverMergesByTagsOrSimilarity(t *testing.T) {
	m := newTestManager(t, &memoryStoreFake{}, nil)
	for _, summary := range []string{"喜欢咖啡", "不喜欢咖啡"} {
		if _, err := m.SaveEntry(EditRequest{Type: Preference, Summary: summary, Tags: []string{"饮食"}}); err != nil {
			t.Fatal(err)
		}
	}
	if m.Count() != 2 {
		t.Fatal("distinct facts were merged")
	}
	if _, err := m.SaveEntry(EditRequest{Type: Preference, Summary: "喜欢咖啡", Tags: []string{"饮食"}}); err != nil {
		t.Fatal(err)
	}
	if m.Count() != 2 {
		t.Fatal("identical retry duplicated memory")
	}
	if _, err := m.SaveEntry(EditRequest{Type: Preference, Summary: "喜欢咖啡", Detail: "different"}); !errors.Is(err, ErrConflict) {
		t.Fatal("implicit replacement accepted")
	}
}
func TestManagerDoesNotHideLoadFailure(t *testing.T) {
	failure := errors.New("storage unavailable")
	if _, err := NewManager(&memoryStoreFake{loadErr: failure}, nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
