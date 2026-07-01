package tools

import (
	"context"
	domainhistory "fkteams/internal/domain/history"
	"fkteams/internal/runtime/toolpolicy"
	"slices"
	"testing"
)

func TestSessionAttachmentIsBuiltinCapabilityNotConfigurableTool(t *testing.T) {
	ctx := WithRegistry(context.Background(), NewToolGroupRegistry())
	if slices.Contains(BuiltinToolNames(ctx), "attachment") {
		t.Fatal("attachment should not be exposed as a configurable tool group")
	}
	if slices.Contains(GetAllToolNames(ctx), "attachment") {
		t.Fatal("attachment should not be exposed in all configurable tool names")
	}
	if _, err := GetToolsByName(ctx, "attachment"); err == nil {
		t.Fatal("attachment should not be resolved by configurable tool lookup")
	}
	if !slices.Contains(BuiltinCapabilityNames(), "session_attachment") {
		t.Fatal("session_attachment should be registered as a builtin capability")
	}
}

func TestBuiltinCapabilityToolsRequirePolicies(t *testing.T) {
	ctx := WithRegistry(context.Background(), NewToolGroupRegistry(ToolResolveContext{
		HistoryReader: fakeCapabilityHistoryReader{},
	}))
	capabilityTools, err := GetBuiltinCapabilityTools(ctx)
	if err != nil {
		t.Fatalf("get builtin capability tools: %v", err)
	}
	if len(capabilityTools) == 0 {
		t.Fatal("expected builtin capability tools")
	}
	for _, tool := range capabilityTools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info: %v", err)
		}
		if !toolpolicy.IsPolicyRequired(info) {
			t.Fatalf("tool %s should require an explicit policy", info.Name)
		}
	}
	if err := toolpolicy.ClassifyTools(capabilityTools); err != nil {
		t.Fatalf("classify builtin capability tools: %v", err)
	}
}

type fakeCapabilityHistoryReader struct{}

func (fakeCapabilityHistoryReader) LoadSessionMessages(context.Context, string) ([]domainhistory.AgentMessage, error) {
	return nil, nil
}
