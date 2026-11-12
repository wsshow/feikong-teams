package task

import (
	"errors"
	"slices"
	"sync"
	"testing"

	domaintask "fkteams/internal/domain/task"
)

type testStore struct {
	tasks            []domaintask.Task
	loadErr, saveErr error
}

func (s *testStore) Load() ([]domaintask.Task, error) { return slices.Clone(s.tasks), s.loadErr }
func (s *testStore) Save(tasks []domaintask.Task) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.tasks = slices.Clone(tasks)
	return nil
}
func newRequest() SaveRequest {
	return SaveRequest{Key: "presentation", Title: "准备分享", Goal: "准备下周的分享材料", Acceptance: "有提纲、案例和最终文档", Status: domaintask.Pending}
}
func TestTaskLifecycleSurvivesServiceRestart(t *testing.T) {
	store := &testStore{}
	s := NewService(store)
	req := newRequest()
	task, err := s.Save("first", req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Save("first", req)
	if err != nil || again.ID != task.ID || len(store.tasks) != 1 {
		t.Fatal("create retry duplicated task")
	}
	req.ID, req.Version = task.ID, task.Version
	req.Status = domaintask.WaitingUser
	req.NextStep = "等待用户提供案例"
	req.Progress = "提纲已完成"
	task, err = s.Save("first", req)
	if err != nil {
		t.Fatal(err)
	}
	resumed := NewService(store)
	result, err := resumed.Query(QueryRequest{Query: "分享"})
	if err != nil || len(result.Tasks) != 1 || result.Tasks[0].NextStep != req.NextStep {
		t.Fatalf("resume: %+v %v", result, err)
	}
	req.Version = task.Version
	req.Status = domaintask.Completed
	req.NextStep = ""
	req.Result = "提纲和案例已核对，最终文档保存在分享目录"
	task, err = resumed.Save("second", req)
	if err != nil {
		t.Fatal(err)
	}
	if task.SessionID != "first" {
		t.Fatal("lost source session")
	}
	open, err := resumed.Query(QueryRequest{})
	if err != nil || len(open.Tasks) != 0 {
		t.Fatal("completed task still open")
	}
	finished, err := resumed.Query(QueryRequest{IncludeFinished: true})
	if err != nil || len(finished.Tasks) != 1 || finished.Tasks[0].Result == "" {
		t.Fatal("lost deliverable")
	}
}
func TestTaskPersistenceFailureAndStaleUpdates(t *testing.T) {
	store := &testStore{}
	s := NewService(store)
	req := newRequest()
	task, err := s.Save("session", req)
	if err != nil {
		t.Fatal(err)
	}
	req.ID, req.Version = task.ID, task.Version
	req.Status = domaintask.InProgress
	store.saveErr = errors.New("storage failure")
	if _, err := s.Save("session", req); err == nil {
		t.Fatal("write failure hidden")
	}
	if store.tasks[0].Version != 1 {
		t.Fatal("failed write published")
	}
	store.saveErr = nil
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := s.Save("session", req); results <- err })
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent updates overwritten")
	}
	store.loadErr = errors.New("read failure")
	if _, err := s.Query(QueryRequest{}); err == nil {
		t.Fatal("read error treated as no tasks")
	}
	if _, err := s.Save("session", newRequest()); err == nil {
		t.Fatal("read failure overwritten")
	}
}
func TestTaskRejectsFalseCompletionAndWaiting(t *testing.T) {
	s := NewService(&testStore{})
	req := newRequest()
	task, err := s.Save("session", req)
	if err != nil {
		t.Fatal(err)
	}
	req.ID, req.Version = task.ID, task.Version
	for _, status := range []domaintask.Status{domaintask.Completed, domaintask.WaitingUser, domaintask.WaitingTime, domaintask.Failed, "unknown"} {
		req.Status = status
		if _, err := s.Save("session", req); err == nil {
			t.Fatalf("invalid status payload accepted: %s", status)
		}
	}
}
