package handler

import (
	"context"
	javascripttool "fkteams/internal/adapters/tools/javascript"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"testing"

	"fkteams/internal/app/config"
	apptools "fkteams/internal/app/tools"
)

func TestJavaScriptToolTestRunsDraftAndCollectsNotices(t *testing.T) {
	registry := apptools.NewToolGroupRegistry(apptools.ToolResolveContext{})
	if err := registry.Register(apptools.ToolGroupRegistration{
		Info: apptools.ToolGroupInfo{Name: "javascript", DisplayName: "JavaScript", Description: "test tools", Category: "extension"},
		Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
			tool, err := javascripttool.NewToolWithOptions(ctx.Config.JavaScript.Tools[0], javascripttool.Options{NoticeEmitter: testNoticeEmitter{}})
			if err != nil {
				return nil, err
			}
			return []runtimeport.Tool{tool}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := testJavaScriptTool(context.Background(), registry, javaScriptToolTestRequest{
		Tool: config.JavaScriptTool{
			ID:          "preview",
			Name:        "预览",
			Description: "试运行测试",
			Source: `function execute(input, context) {
	context.notify("preview completed");
  return {value: input.value + 1};
}`,
		},
		Input: map[string]any{"value": 1},
	})
	if err != nil {
		t.Fatalf("testJavaScriptTool() error = %v", err)
	}
	if resp.Raw != `{"value":2}` {
		t.Fatalf("raw result = %q", resp.Raw)
	}
	if len(resp.Notices) != 1 || resp.Notices[0].Message != "preview completed" {
		t.Fatalf("notices = %#v", resp.Notices)
	}
}

func TestJavaScriptToolTestRejectsInvalidDefinition(t *testing.T) {
	_, err := testJavaScriptTool(context.Background(), nil, javaScriptToolTestRequest{
		Tool: config.JavaScriptTool{ID: "invalid id"},
	})
	if err == nil {
		t.Fatal("testJavaScriptTool() error = nil, want validation error")
	}
}

type testNoticeEmitter struct{}

func (testNoticeEmitter) Notice(ctx context.Context, scriptID, level, message string) error {
	event := events.SystemNotice("", "", "javascript_extension", message)
	event.Notice.Level = level
	return events.DispatchEvent(ctx, event)
}
