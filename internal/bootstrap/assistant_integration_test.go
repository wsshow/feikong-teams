package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/agent/catalog/workspaceassistant"
	appsession "fkteams/internal/app/session"
	"fkteams/internal/bootstrap"
	domainmessage "fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/testmodel"
)

func TestAssistantRunsWorkspaceHistoryAndRenameTools(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", root)
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("workspace notes"), 0644); err != nil {
		t.Fatal(err)
	}
	sessionsDir := filepath.Join(root, "sessions")
	service := appsession.NewService(eventlog.NewSessionRepository(sessionsDir))
	ctx := context.Background()
	if _, err := service.OpenAssistant(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Create(ctx, appsession.CreateRequest{SessionID: "prior", Title: "项目讨论"}); err != nil {
		t.Fatal(err)
	}
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(filepath.Join(sessionsDir, "prior"))
	recorder.RecordEvent(events.UserMessage("run", "turn", "message", domainmessage.Message{Role: domainmessage.RoleUser, Content: "预算方案已经确定"}))
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	dependencies, err := bootstrap.NewExecutionDependencies()
	if err != nil {
		t.Fatal(err)
	}
	ctx = domainsession.WithID(dependencies.Context(ctx), domainsession.AssistantID)
	model := testmodel.New()
	for _, call := range []domainmessage.ToolCall{
		{ID: "search", Type: "function", Function: domainmessage.FunctionCall{Name: "session_search", Arguments: `{"query":"预算"}`}},
		{ID: "read", Type: "function", Function: domainmessage.FunctionCall{Name: "session_read", Arguments: `{"session_id":"prior"}`}},
		{ID: "files", Type: "function", Function: domainmessage.FunctionCall{Name: "file_list", Arguments: `{"dirpath":"."}`}},
		{ID: "rename", Type: "function", Function: domainmessage.FunctionCall{Name: "assistant_rename", Arguments: `{"name":"小空"}`}},
	} {
		model.EnqueueStream(domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{call}})
	}
	model.EnqueueStream(testmodel.AssistantMessage("done"))
	definition := workspaceassistant.DefaultDefinition()
	definition.Model = model
	agent, err := common.BuildAgent(ctx, definition)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := dependencies.Runtime.NewRunner(ctx, runtimeport.RunnerConfig{Agent: agent, EnableStreaming: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(ctx, domainmessage.TurnInput{Message: testmodel.UserMessage("查找之前的预算和工作区文件，以后叫你小空")}, runtimeport.RunOptions{RunID: "assistant-test"}); err != nil {
		t.Fatal(err)
	}
	meta, err := service.Get(ctx, domainsession.AssistantID)
	if err != nil || meta.Title != "小空" {
		t.Fatalf("rename was not executed: %#v, %v", meta, err)
	}
	calls := model.StreamCalls()
	if len(calls) != 5 {
		t.Fatalf("model calls = %d", len(calls))
	}
	var output strings.Builder
	for _, msg := range calls[len(calls)-1].Input {
		if msg.Role == domainmessage.RoleTool {
			output.WriteString(msg.Content)
		}
	}
	for _, want := range []string{"预算方案已经确定", "notes.txt", "小空"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("tool results missing %q: %s", want, output.String())
		}
	}
}
