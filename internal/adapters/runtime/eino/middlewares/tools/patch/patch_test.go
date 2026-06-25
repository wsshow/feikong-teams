package patch

import (
	"context"
	"testing"

	einoruntime "fkteams/internal/adapters/runtime/eino"
)

func TestNewCreatesPatchToolCallsMiddleware(t *testing.T) {
	middleware, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if middleware == nil {
		t.Fatal("New() returned nil middleware")
	}
	if middleware.Name() != "patch_tool_calls" {
		t.Fatalf("middleware name = %q, want patch_tool_calls", middleware.Name())
	}
	if _, err := einoruntime.AdaptAgentMiddlewareForRunner(middleware); err != nil {
		t.Fatalf("adapt middleware: %v", err)
	}
}
