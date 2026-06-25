package appstate

import (
	"context"
	"errors"
	"fkteams/memory"
	"testing"
)

func TestStateStoresMemoryManager(t *testing.T) {
	state := New()
	manager := &fakeMemoryManager{}

	if state.Memory() != nil {
		t.Fatal("new state should not have memory manager")
	}
	state.SetMemory(manager)
	if state.Memory() != manager {
		t.Fatal("state should return configured memory manager")
	}
}

func TestStateRunProcessCleanupExecutesAndClearsCallbacks(t *testing.T) {
	state := New()
	var order []string
	state.Cleaner().Add(func() error {
		order = append(order, "first")
		return errors.New("boom")
	})
	state.Cleaner().Add(func() error {
		order = append(order, "second")
		return nil
	})

	state.RunProcessCleanup()
	state.RunProcessCleanup()

	if len(order) != 2 || order[0] != "second" || order[1] != "first" {
		t.Fatalf("cleanup order = %v", order)
	}
}

type fakeMemoryManager struct{}

func (m *fakeMemoryManager) Search(string, int) []memory.MemoryEntry { return nil }
func (m *fakeMemoryManager) ExtractAndStore(context.Context, []memory.Message, string) {
}
func (m *fakeMemoryManager) FlushExtract(context.Context, []memory.Message, string) {}
func (m *fakeMemoryManager) List() []memory.MemoryEntry                             { return nil }
func (m *fakeMemoryManager) Delete(string) int                                      { return 0 }
func (m *fakeMemoryManager) Count() int                                             { return 0 }
func (m *fakeMemoryManager) Clear()                                                 {}
func (m *fakeMemoryManager) ResetLLM(memory.LLMClient)                              {}
func (m *fakeMemoryManager) Wait()                                                  {}
