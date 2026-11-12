// Package task 提供跨回合任务记录与续办用例。
package task

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	domaintask "fkteams/internal/domain/task"
	taskport "fkteams/internal/ports/task"
	"github.com/google/uuid"
)

// ErrConflict 表示用户任务已被其他执行修改。
var ErrConflict = errors.New("task changed; read the current version before updating")

// Service 串行化应用实例内的快照修改，存储是唯一事实来源。
type Service struct {
	mu    sync.Mutex
	store taskport.Store
}

// NewService 创建任务用例。
func NewService(store taskport.Store) *Service { return &Service{store: store} }

// SaveRequest 描述任务完整状态，更新时携带 ID 和版本。
type SaveRequest struct {
	ID         string            `json:"id,omitempty"`
	Version    uint64            `json:"version,omitempty"`
	Key        string            `json:"key,omitempty"`
	Title      string            `json:"title"`
	Goal       string            `json:"goal"`
	Acceptance string            `json:"acceptance"`
	Status     domaintask.Status `json:"status"`
	Progress   string            `json:"progress,omitempty"`
	NextStep   string            `json:"next_step,omitempty"`
	Result     string            `json:"result,omitempty"`
	ScheduleID string            `json:"schedule_id,omitempty"`
}

// QueryRequest 查询指定任务或按目标和状态分页检索。
type QueryRequest struct {
	ID              string `json:"id,omitempty"`
	Query           string `json:"query,omitempty"`
	IncludeFinished bool   `json:"include_finished,omitempty"`
	Offset          int    `json:"offset,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

// QueryResult 返回任务快照与分页位置。
type QueryResult struct {
	Tasks      []domaintask.Task `json:"tasks"`
	NextOffset int               `json:"next_offset"`
	HasMore    bool              `json:"has_more"`
}

// Query 返回持久任务，不执行或重新触发其中的操作。
func (s *Service) Query(req QueryRequest) (QueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.load()
	if err != nil {
		return QueryResult{}, err
	}
	matches := make([]domaintask.Task, 0)
	query := strings.ToLower(strings.TrimSpace(req.Query))
	for _, t := range tasks {
		if req.ID != "" {
			if t.ID == req.ID {
				matches = append(matches, t)
			}
			continue
		}
		if t.Status.Terminal() && !req.IncludeFinished {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(t.Title+" "+t.Goal+" "+t.Progress+" "+t.Result), query) {
			continue
		}
		matches = append(matches, t)
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].UpdatedAt.After(matches[j].UpdatedAt) })
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset := min(max(req.Offset, 0), len(matches))
	end := offset + min(limit, len(matches)-offset)
	return QueryResult{Tasks: matches[offset:end], NextOffset: end, HasMore: end < len(matches)}, nil
}

// Save 保存任务状态，创建重试按来源会话和稳定 key 去重。
func (s *Service) Save(sessionID string, req SaveRequest) (domaintask.Task, error) {
	req.Title, req.Goal, req.Acceptance = strings.TrimSpace(req.Title), strings.TrimSpace(req.Goal), strings.TrimSpace(req.Acceptance)
	if sessionID == "" || req.Title == "" || req.Goal == "" || req.Acceptance == "" || !req.Status.Valid() {
		return domaintask.Task{}, fmt.Errorf("session, title, goal, acceptance and valid status are required")
	}
	if len(req.Title) > 512 || len(req.Goal) > 8192 || len(req.Acceptance) > 4096 || len(req.Progress) > 8192 || len(req.NextStep) > 4096 || len(req.Result) > 16384 || len(req.Key) > 128 || len(req.ScheduleID) > 200 {
		return domaintask.Task{}, fmt.Errorf("task fields exceed size limits")
	}
	if req.Status == domaintask.WaitingTime && req.ScheduleID == "" {
		return domaintask.Task{}, fmt.Errorf("waiting_time requires a saved schedule ID")
	}
	if req.Status == domaintask.Completed && strings.TrimSpace(req.Result) == "" {
		return domaintask.Task{}, fmt.Errorf("completed task requires a verified result")
	}
	if (req.Status == domaintask.WaitingUser || req.Status == domaintask.WaitingTime || req.Status == domaintask.Failed) && strings.TrimSpace(req.NextStep) == "" {
		return domaintask.Task{}, fmt.Errorf("blocked task requires a reason and next step")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.load()
	if err != nil {
		return domaintask.Task{}, err
	}
	index := -1
	for i, t := range tasks {
		if req.ID != "" && t.ID == req.ID {
			index = i
		}
		if req.ID == "" && req.Key != "" && t.Key == req.Key && t.SessionID == sessionID {
			if t.Title != req.Title || t.Goal != req.Goal || t.Acceptance != req.Acceptance {
				return domaintask.Task{}, ErrConflict
			}
			return t, nil
		}
	}
	if req.ID != "" && index < 0 {
		return domaintask.Task{}, fmt.Errorf("task not found")
	}
	if index >= 0 && (req.Version == 0 || req.Version != tasks[index].Version) {
		return domaintask.Task{}, ErrConflict
	}
	if index < 0 {
		if req.Key == "" || req.Version != 0 || req.Status != domaintask.Pending {
			return domaintask.Task{}, fmt.Errorf("new task requires a stable key, pending status and no version")
		}
		if len(tasks) >= 1000 {
			return domaintask.Task{}, fmt.Errorf("task capacity reached")
		}
		tasks = append(tasks, domaintask.Task{ID: uuid.NewString(), Key: req.Key, SessionID: sessionID, CreatedAt: time.Now()})
		index = len(tasks) - 1
	}
	task := tasks[index]
	task.Version++
	task.UpdatedAt = time.Now()
	task.Title, task.Goal, task.Acceptance = req.Title, req.Goal, req.Acceptance
	task.Status, task.Progress, task.NextStep, task.Result, task.ScheduleID = req.Status, req.Progress, req.NextStep, req.Result, req.ScheduleID
	tasks[index] = task
	if err := s.store.Save(tasks); err != nil {
		return domaintask.Task{}, fmt.Errorf("save task: %w", err)
	}
	return task, nil
}
func (s *Service) load() ([]domaintask.Task, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("task service is unavailable")
	}
	tasks, err := s.store.Load()
	if err != nil {
		return nil, fmt.Errorf("load tasks: %w", err)
	}
	return slices.Clone(tasks), nil
}
