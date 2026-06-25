package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"fkteams/events"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	cfg.defaults()

	if cfg.MaxConcurrency != defaultMaxConcurrency {
		t.Fatalf("MaxConcurrency = %d, want %d", cfg.MaxConcurrency, defaultMaxConcurrency)
	}
	if cfg.TaskTimeout != defaultTaskTimeout {
		t.Fatalf("TaskTimeout = %s, want %s", cfg.TaskTimeout, defaultTaskTimeout)
	}

	cfg = Config{MaxConcurrency: 8, TaskTimeout: time.Second}
	cfg.defaults()
	if cfg.MaxConcurrency != 8 || cfg.TaskTimeout != time.Second {
		t.Fatalf("defaults overwrote configured values: concurrency=%d timeout=%s", cfg.MaxConcurrency, cfg.TaskTimeout)
	}
}

func TestExecuteTasksEmptyInput(t *testing.T) {
	m := &middleware{maxConcurrency: 1, taskTimeout: time.Second}

	got, err := m.executeTasks(context.Background(), &dispatchInput{})
	if err != nil {
		t.Fatalf("executeTasks returned error: %v", err)
	}
	if got != `{"results":[]}` {
		t.Fatalf("executeTasks empty result = %q", got)
	}
}

func TestFailSetsStatusAndError(t *testing.T) {
	got := fail(taskResult{TaskIndex: 1, Description: "任务"}, statusError, "boom")
	if got.Status != statusError || got.Error != "boom" || got.TaskIndex != 1 || got.Description != "任务" {
		t.Fatalf("fail result = %#v", got)
	}
}

func TestSendEventDropsWhenChannelIsFull(t *testing.T) {
	ch := make(chan viewEvent, 1)
	sendEvent(ch, 0, "start", "")
	sendEvent(ch, 1, "done", "ignored")

	got := <-ch
	if got.TaskIndex != 0 || got.Type != "start" {
		t.Fatalf("first event = %#v, want start event", got)
	}
	select {
	case extra := <-ch:
		t.Fatalf("sendEvent should drop when full, got %#v", extra)
	default:
	}
}

func TestForwardEventsDispatchesMemberUpdates(t *testing.T) {
	ch := make(chan viewEvent, 3)
	ch <- viewEvent{TaskIndex: 0, Type: "start"}
	ch <- viewEvent{TaskIndex: 1, Type: "content", Content: "完成"}
	ch <- viewEvent{TaskIndex: 9, Type: "error", Content: "bad index"}
	close(ch)

	var got []events.Event
	ctx := events.WithCallback(events.WithNonInteractive(context.Background()), func(event events.Event) error {
		got = append(got, event)
		return nil
	})
	tasks := []taskItem{{Description: "第一项"}, {Description: "第二项"}}
	forwardEvents(ctx, tasks, ch)

	if len(got) != 3 {
		t.Fatalf("forwarded events count = %d, want 3", len(got))
	}
	if got[0].Type != events.EventMemberUpdate || got[0].ActionType != events.ActionType("start") {
		t.Fatalf("first forwarded event = %#v", got[0])
	}
	if got[1].Content != "完成" || got[1].ActionType != events.ActionType("content") {
		t.Fatalf("second forwarded event = %#v", got[1])
	}

	var detail struct {
		TaskIndex   int    `json:"task_index"`
		Description string `json:"description"`
		EventType   string `json:"event_type"`
		EventDetail string `json:"event_detail"`
	}
	if err := json.Unmarshal([]byte(got[1].Detail), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if detail.TaskIndex != 1 || detail.Description != "第二项" || detail.EventType != "content" || detail.EventDetail != "完成" {
		t.Fatalf("detail = %#v", detail)
	}

	if err := json.Unmarshal([]byte(got[2].Detail), &detail); err != nil {
		t.Fatalf("unmarshal out of range detail: %v", err)
	}
	if detail.Description != "" || detail.TaskIndex != 9 {
		t.Fatalf("out of range detail = %#v", detail)
	}
}

func TestConsumeMessageStreamCollectsContentAndOperations(t *testing.T) {
	longArgs := strings.Repeat("参数", 70)
	stream := schema.StreamReaderFromArray([]*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "一",
			ToolCalls: []schema.ToolCall{
				{Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"README.md"}`}},
				{Function: schema.FunctionCall{Arguments: "missing name"}},
			},
		},
		{
			Role:    schema.Assistant,
			Content: "二",
			ToolCalls: []schema.ToolCall{
				{Function: schema.FunctionCall{Name: "search", Arguments: longArgs}},
			},
		},
	})
	event := &adk.AgentEvent{
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{IsStreaming: true, MessageStream: stream},
		},
	}
	ch := make(chan viewEvent, 8)
	var content strings.Builder
	var operations []string

	if err := consumeMessageStream(event, 2, ch, &content, &operations); err != nil {
		t.Fatalf("consumeMessageStream returned error: %v", err)
	}
	close(ch)

	if content.String() != "一二" {
		t.Fatalf("content = %q, want 一二", content.String())
	}
	if len(operations) != 2 {
		t.Fatalf("operations count = %d, want 2: %#v", len(operations), operations)
	}
	if operations[0] != `read_file({"path":"README.md"})` {
		t.Fatalf("first operation = %q", operations[0])
	}
	if !strings.HasPrefix(operations[1], "search(") || !strings.HasSuffix(operations[1], "...") {
		t.Fatalf("second operation should be truncated, got %q", operations[1])
	}
	if len([]rune(operations[1])) != 123 {
		t.Fatalf("second operation length = %d, want 123 including ellipsis", len([]rune(operations[1])))
	}

	var eventTypes []string
	for e := range ch {
		eventTypes = append(eventTypes, e.Type)
	}
	wantTypes := []string{"op", "content", "op", "content"}
	if strings.Join(eventTypes, ",") != strings.Join(wantTypes, ",") {
		t.Fatalf("event types = %#v, want %#v", eventTypes, wantTypes)
	}
}

func TestConsumeMessageStreamReturnsStreamError(t *testing.T) {
	stream := schema.StreamReaderWithConvert(schema.StreamReaderFromArray([]string{"ok"}), func(string) (*schema.Message, error) {
		return nil, errors.New("stream failed")
	})
	event := &adk.AgentEvent{
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{IsStreaming: true, MessageStream: stream},
		},
	}
	ch := make(chan viewEvent, 1)
	var content strings.Builder
	var operations []string

	err := consumeMessageStream(event, 0, ch, &content, &operations)
	if err == nil || !strings.Contains(err.Error(), "stream failed") {
		t.Fatalf("consumeMessageStream error = %v, want stream failed", err)
	}
}
