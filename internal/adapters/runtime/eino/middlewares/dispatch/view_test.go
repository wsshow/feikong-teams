package dispatch

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDispatchModelApplyEvent(t *testing.T) {
	model := newDispatchModel([]taskItem{{Description: "任务一"}}, nil)

	model.applyEvent(viewEvent{TaskIndex: -1, Type: "start"})
	if model.tasks[0].status != "waiting" {
		t.Fatalf("invalid event changed status to %q", model.tasks[0].status)
	}

	model.applyEvent(viewEvent{TaskIndex: 0, Type: "start"})
	model.applyEvent(viewEvent{TaskIndex: 0, Type: "op", Content: "read_file()"})
	model.applyEvent(viewEvent{TaskIndex: 0, Type: "content", Content: "结果"})
	if model.tasks[0].status != "running" {
		t.Fatalf("status = %q, want running", model.tasks[0].status)
	}
	if got := strings.Join(model.tasks[0].operations, ","); got != "read_file()" {
		t.Fatalf("operations = %q", got)
	}
	if got := model.tasks[0].content.String(); got != "结果" {
		t.Fatalf("content = %q", got)
	}

	model.applyEvent(viewEvent{TaskIndex: 0, Type: "done"})
	if model.tasks[0].status != "done" {
		t.Fatalf("status = %q, want done", model.tasks[0].status)
	}
	model.applyEvent(viewEvent{TaskIndex: 0, Type: "error", Content: "失败"})
	if model.tasks[0].status != "error" || !strings.Contains(model.tasks[0].content.String(), "错误: 失败") {
		t.Fatalf("error card = %#v content=%q", model.tasks[0], model.tasks[0].content.String())
	}
	model.applyEvent(viewEvent{TaskIndex: 0, Type: "timeout"})
	if model.tasks[0].status != "timeout" {
		t.Fatalf("status = %q, want timeout", model.tasks[0].status)
	}
}

func TestDispatchModelUpdateNavigationAndCancel(t *testing.T) {
	ch := make(chan viewEvent)
	model := newDispatchModel([]taskItem{{Description: "一"}, {Description: "二"}}, ch)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = updated.(dispatchModel)
	if model.width != 120 {
		t.Fatalf("width = %d, want 120", model.width)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = updated.(dispatchModel)
	if model.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", model.cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(dispatchModel)
	if model.expanded != 1 {
		t.Fatalf("expanded after enter = %d, want 1", model.expanded)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = updated.(dispatchModel)
	if model.scrollY != 1 {
		t.Fatalf("scrollY after down in expanded card = %d, want 1", model.scrollY)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = updated.(dispatchModel)
	if model.expanded != -1 || model.scrollY != 0 {
		t.Fatalf("after esc expanded=%d scrollY=%d, want collapsed", model.expanded, model.scrollY)
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	model = updated.(dispatchModel)
	if !model.cancelled {
		t.Fatal("ctrl+c should mark model cancelled")
	}
}

func TestDispatchModelUpdateEventsAndView(t *testing.T) {
	ch := make(chan viewEvent, 1)
	model := newDispatchModel([]taskItem{{Description: "任务一"}}, ch)

	updated, _ := model.Update(viewEvent{TaskIndex: 0, Type: "start"})
	model = updated.(dispatchModel)
	if model.tasks[0].status != "running" {
		t.Fatalf("status = %q, want running", model.tasks[0].status)
	}
	view := model.View().Content
	if !strings.Contains(view, "并行分发 1 个子任务") || !strings.Contains(view, "任务一") {
		t.Fatalf("view missing expected text: %q", view)
	}

	updated, _ = model.Update(allDoneMsg{})
	model = updated.(dispatchModel)
	if !model.allDone {
		t.Fatal("allDoneMsg should mark allDone")
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	model = updated.(dispatchModel)
	if !model.allDone {
		t.Fatal("q after all done should preserve model state")
	}
}

func TestStatusHelpers(t *testing.T) {
	tests := map[string]string{
		"waiting": "○",
		"running": "◐",
		"done":    "✓",
		"error":   "✗",
		"timeout": "T",
		"other":   "?",
	}
	for status, want := range tests {
		if got := statusIcon(status); got != want {
			t.Fatalf("statusIcon(%q) = %q, want %q", status, got, want)
		}
	}
	for _, status := range []string{"waiting", "running", "done", "error", "timeout"} {
		_ = statusColor(status).Render("x")
		_ = cardBorderColor(status, false)
	}
	_ = cardBorderColor("waiting", true)
}
