package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appschedule "fkteams/internal/app/schedule"
	domainschedule "fkteams/internal/domain/schedule"
	schedulerport "fkteams/internal/ports/scheduler"
)

func TestToolsDelegateToScheduleService(t *testing.T) {
	fake := &fakeScheduler{}
	service := appschedule.NewService(fake)
	tools := NewTools(nil)
	ctx := appschedule.WithService(context.Background(), service)

	addResp, err := tools.ScheduleAdd(ctx, &ScheduleAddRequest{
		Task:      "生成日报",
		ExecuteAt: time.Now().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("ScheduleAdd returned error: %v", err)
	}
	if !addResp.Success || addResp.Task == nil || fake.addReq.Task != "生成日报" {
		t.Fatalf("ScheduleAdd response = %#v, addReq = %#v", addResp, fake.addReq)
	}

	listResp, err := tools.ScheduleList(ctx, &ScheduleListRequest{StatusFilter: string(domainschedule.StatusPending)})
	if err != nil {
		t.Fatalf("ScheduleList returned error: %v", err)
	}
	if !listResp.Success || listResp.TotalCount != 1 || fake.listStatus != domainschedule.StatusPending {
		t.Fatalf("ScheduleList response = %#v, status = %s", listResp, fake.listStatus)
	}

	cancelResp, err := tools.ScheduleCancel(ctx, &ScheduleCancelRequest{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("ScheduleCancel returned error: %v", err)
	}
	if !cancelResp.Success || fake.cancelID != "task-1" {
		t.Fatalf("ScheduleCancel response = %#v, cancelID = %s", cancelResp, fake.cancelID)
	}

	deleteResp, err := tools.ScheduleDelete(ctx, &ScheduleDeleteRequest{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("ScheduleDelete returned error: %v", err)
	}
	if !deleteResp.Success || fake.deleteID != "task-1" {
		t.Fatalf("ScheduleDelete response = %#v, deleteID = %s", deleteResp, fake.deleteID)
	}
}

func TestToolsReturnErrorMessageWhenServiceMissing(t *testing.T) {
	tools := NewTools(nil)
	resp, err := tools.ScheduleList(context.Background(), &ScheduleListRequest{})
	if err != nil {
		t.Fatalf("ScheduleList returned error: %v", err)
	}
	if resp.Success || !strings.Contains(resp.ErrorMessage, "not initialized") {
		t.Fatalf("ScheduleList response = %#v", resp)
	}
}

func TestToolsExposeRuntimeTools(t *testing.T) {
	tools, err := NewTools(nil).GetTools()
	if err != nil {
		t.Fatalf("GetTools: %v", err)
	}
	if len(tools) != 6 {
		t.Fatalf("tool count = %d, want 6", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info: %v", err)
	}
	if info.Name != "schedule_add" {
		t.Fatalf("first tool = %s", info.Name)
	}
}

func TestFormatTasksForDisplay(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	got := FormatTasksForDisplay([]domainschedule.Task{{
		ID:        "task-1",
		Task:      "整理测试",
		Status:    domainschedule.StatusPending,
		CronExpr:  "0 9 * * *",
		NextRunAt: now,
	}})

	for _, want := range []string{"1 scheduled tasks", "整理测试", "task-1", "0 9 * * *"} {
		if !strings.Contains(got, want) {
			t.Fatalf("display = %q, want containing %q", got, want)
		}
	}
	if got := FormatTasksForDisplay(nil); got != "no scheduled tasks" {
		t.Fatalf("empty display = %q", got)
	}
}

type fakeScheduler struct {
	addReq     schedulerport.AddTaskRequest
	updateReq  schedulerport.AddTaskRequest
	updateID   string
	listStatus domainschedule.Status
	cancelID   string
	deleteID   string
}

func (s *fakeScheduler) SetExecutor(schedulerport.TaskExecutor) {}
func (s *fakeScheduler) Start()                                 {}
func (s *fakeScheduler) Stop()                                  {}

func (s *fakeScheduler) AddTask(ctx context.Context, req schedulerport.AddTaskRequest) (*domainschedule.Task, error) {
	s.addReq = req
	return &domainschedule.Task{ID: "task-1", Task: req.Task, Status: domainschedule.StatusPending}, nil
}

func (s *fakeScheduler) UpdateTask(ctx context.Context, taskID string, req schedulerport.AddTaskRequest) (*domainschedule.Task, error) {
	s.updateID = taskID
	s.updateReq = req
	return &domainschedule.Task{ID: taskID, Task: req.Task, Status: domainschedule.StatusPending}, nil
}

func (s *fakeScheduler) ListTasks(ctx context.Context, statusFilter domainschedule.Status) ([]domainschedule.Task, error) {
	s.listStatus = statusFilter
	return []domainschedule.Task{{ID: "task-1", Task: "生成日报", Status: domainschedule.StatusPending}}, nil
}

func (s *fakeScheduler) CancelTask(ctx context.Context, taskID string) error {
	s.cancelID = taskID
	return nil
}

func (s *fakeScheduler) DeleteTask(ctx context.Context, taskID string) error {
	s.deleteID = taskID
	return nil
}

func (s *fakeScheduler) ReadTaskResult(ctx context.Context, taskID string) (string, error) {
	return "", errors.New("not used")
}

func (s *fakeScheduler) ListHistoryEntries(ctx context.Context, taskID string) ([]domainschedule.HistoryEntry, error) {
	return nil, errors.New("not used")
}

func (s *fakeScheduler) ReadHistoryFile(ctx context.Context, taskID string, filename string) (string, error) {
	return "", errors.New("not used")
}

func (s *fakeScheduler) ComputeNextRun(expr string, after time.Time) (time.Time, error) {
	return time.Time{}, errors.New("not used")
}

type resultScheduler struct {
	fakeScheduler
	content string
	err     error
}

func (s *resultScheduler) ReadTaskResult(context.Context, string) (string, error) {
	return s.content, s.err
}

func TestScheduleResultUnicodePagingAndErrors(t *testing.T) {
	fake := &resultScheduler{content: "完成每日报告"}
	tools := NewTools(func(context.Context) *appschedule.Service { return appschedule.NewService(fake) })
	ctx := context.Background()
	first, err := tools.ScheduleResult(ctx, &ScheduleResultRequest{TaskID: "task-1", Limit: 2})
	if err != nil || first.Content != "完成" || first.NextOffset != 2 || !first.HasMore {
		t.Fatalf("page: %+v %v", first, err)
	}
	next, err := tools.ScheduleResult(ctx, &ScheduleResultRequest{TaskID: "task-1", Offset: first.NextOffset})
	if err != nil || next.Content != "每日报告" || next.HasMore {
		t.Fatalf("next: %+v %v", next, err)
	}
	fake.err = errors.New("result unavailable")
	failed, err := tools.ScheduleResult(ctx, &ScheduleResultRequest{TaskID: "task-1"})
	if err != nil || failed.ErrorMessage == "" {
		t.Fatalf("error hidden: %+v %v", failed, err)
	}
}

func TestScheduleUpdateKeepsIdentityAndPassesTimeZone(t *testing.T) {
	fake := &fakeScheduler{}
	tools := NewTools(func(context.Context) *appschedule.Service { return appschedule.NewService(fake) })
	resp, err := tools.ScheduleUpdate(context.Background(), &ScheduleUpdateRequest{TaskID: "existing", ScheduleAddRequest: ScheduleAddRequest{Task: "日报", CronExpr: "CRON_TZ=Asia/Shanghai 0 9 * * *"}})
	if err != nil || !resp.Success || fake.updateID != "existing" || fake.updateReq.CronExpr != "CRON_TZ=Asia/Shanghai 0 9 * * *" {
		t.Fatalf("update: %+v, %+v, %v", resp, fake, err)
	}
}
