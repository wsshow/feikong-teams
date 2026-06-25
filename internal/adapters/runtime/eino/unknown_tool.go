package eino

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/compose"
)

type unknownToolContextKey struct{}

type unknownToolReport struct {
	AgentName  string
	ToolCallID string
	ToolName   string
	ToolArgs   string
	ToolResult string
}

type unknownToolRecorder struct {
	mu      sync.Mutex
	reports []unknownToolReport
}

func newUnknownToolRecorder() *unknownToolRecorder {
	return &unknownToolRecorder{}
}

func withUnknownToolRecorder(ctx context.Context, recorder *unknownToolRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, unknownToolContextKey{}, recorder)
}

func recordUnknownToolResult(ctx context.Context, report unknownToolReport) {
	recorder, ok := ctx.Value(unknownToolContextKey{}).(*unknownToolRecorder)
	if !ok || recorder == nil {
		return
	}
	if report.ToolCallID == "" {
		report.ToolCallID = compose.GetToolCallID(ctx)
	}
	recorder.add(report)
}

func (r *unknownToolRecorder) add(report unknownToolReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, report)
}

func (r *unknownToolRecorder) take() []unknownToolReport {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	reports := r.reports
	r.reports = nil
	return reports
}
