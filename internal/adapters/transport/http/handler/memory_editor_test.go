package handler

import (
	"errors"
	"fkteams/internal/app/appstate"
	appmemory "fkteams/internal/app/memory"
	domainmemory "fkteams/internal/domain/memory"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type editorStore struct {
	entries []domainmemory.MemoryEntry
	failure error
}

func (s *editorStore) Load(int) ([]domainmemory.MemoryEntry, error) { return s.entries, nil }
func (s *editorStore) Save(entries []domainmemory.MemoryEntry) error {
	if s.failure != nil {
		return s.failure
	}
	s.entries = entries
	return nil
}

func TestMemoryEditorReportsPersistenceFailure(t *testing.T) {
	store := &editorStore{}
	manager, err := appmemory.NewManager(store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := appstate.New()
	state.SetMemory(manager)
	router := gin.New()
	router.PUT("/memory", SaveMemoryEntryHandler(state))
	router.DELETE("/memory/:id", DeleteMemoryEntryHandler(state))
	request := func(method, path, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := request("PUT", "/memory", `{"type":"fact","summary":"摘要"}`); got != http.StatusOK {
		t.Fatal(got)
	}
	entry := manager.List()[0]
	store.failure = errors.New("private storage failure")
	if got := request("DELETE", "/memory/"+entry.ID, ""); got != http.StatusInternalServerError {
		t.Fatal(got)
	}
	if manager.Count() != 1 {
		t.Fatal("failed deletion removed memory")
	}
	if got := request("PUT", "/memory", `{"type":"fact","summary":"新内容"}`); got != http.StatusInternalServerError {
		t.Fatal(got)
	}
	if got := request("PUT", "/memory", `{"type":"invalid","summary":"新内容"}`); got != http.StatusBadRequest {
		t.Fatal(got)
	}
}
