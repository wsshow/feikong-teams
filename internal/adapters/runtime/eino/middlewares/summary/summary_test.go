package summary

import (
	"context"
	"strings"
	"testing"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/testmodel"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestNewValidatesConfig(t *testing.T) {
	if _, err := New(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("New(nil) error = %v, want config error", err)
	}
	if _, err := New(context.Background(), &Config{}); err == nil || !strings.Contains(err.Error(), "model is nil") {
		t.Fatalf("New(empty) error = %v, want model error", err)
	}
}

func TestNewCreatesSummaryMiddleware(t *testing.T) {
	middleware, err := New(context.Background(), &Config{
		MaxTokensBeforeSummary: 1,
		Model:                  testmodel.New(testmodel.AssistantMessage("summary")),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if middleware.Name() != "summary" {
		t.Fatalf("middleware name = %q, want summary", middleware.Name())
	}
	if _, err := einoruntime.AdaptAgentMiddlewareForRunner(middleware); err != nil {
		t.Fatalf("adapt middleware: %v", err)
	}
}

func TestLatestSummaryTextReturnsLastNonEmptyContent(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("old summary", nil),
		nil,
		schema.AssistantMessage("", nil),
		schema.ToolMessage("tool result", "call-1", schema.WithToolName("fetch_url")),
		schema.AssistantMessage("new summary", nil),
	}

	got := latestSummaryText(messages)
	if got != "new summary" {
		t.Fatalf("latestSummaryText() = %q, want new summary", got)
	}
}

func TestLatestSummaryTextReturnsEmptyWhenNoContent(t *testing.T) {
	got := latestSummaryText([]*schema.Message{
		nil,
		schema.AssistantMessage("", nil),
	})
	if got != "" {
		t.Fatalf("latestSummaryText() = %q, want empty", got)
	}
}

func TestHandleSummaryCallbackPersistsAndDispatchesEvent(t *testing.T) {
	var persisted string
	var dispatched events.Event
	ctx := runtimeport.WithSummaryPersistCallback(context.Background(), func(summaryText string) {
		persisted = summaryText
	})
	ctx = events.WithCallback(ctx, func(event events.Event) error {
		dispatched = event
		return nil
	})

	err := handleSummaryCallback(ctx, adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.AssistantMessage("old", nil),
			schema.AssistantMessage("compressed summary", nil),
		},
	})
	if err != nil {
		t.Fatalf("handleSummaryCallback() error = %v", err)
	}
	if persisted != "compressed summary" {
		t.Fatalf("persisted summary = %q, want compressed summary", persisted)
	}
	if dispatched.Type != events.EventSystemNotice {
		t.Fatalf("event type = %q, want system notice", dispatched.Type)
	}
	if dispatched.Notice == nil || dispatched.Notice.Code != "context_compress" {
		t.Fatalf("notice = %#v, want context compress", dispatched.Notice)
	}
	if dispatched.Detail != "compressed summary" {
		t.Fatalf("event detail = %q, want compressed summary", dispatched.Detail)
	}
}
