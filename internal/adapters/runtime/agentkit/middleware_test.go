package agentkit

import (
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
	"github.com/cloudwego/eino/compose"
)

func TestMiddlewareRejectsForeignBindings(t *testing.T) {
	if _, err := AdaptAgentMiddlewareForRunner(runtimeport.BindAgentMiddleware("other", "agent", struct{}{})); err == nil {
		t.Fatal("foreign agent middleware accepted")
	}
	if _, err := AdaptToolMiddlewareForRunner(runtimeport.BindToolMiddleware("other", "tool", compose.ToolMiddleware{})); err == nil {
		t.Fatal("foreign tool middleware accepted")
	}
	if _, err := AdaptToolMiddlewareForRunner(runtimeport.BindToolMiddleware("agentkit", "invalid", struct{}{})); err == nil {
		t.Fatal("invalid native middleware accepted")
	}
	m := WrapToolMiddleware("tool", compose.ToolMiddleware{})
	if m.Name() != "tool" {
		t.Fatalf("name = %q", m.Name())
	}
	if _, err := AdaptToolMiddlewareForRunner(m); err != nil {
		t.Fatal(err)
	}
}
