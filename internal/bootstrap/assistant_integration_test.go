package bootstrap_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/agent/catalog/workspaceassistant"
	appchat "fkteams/internal/app/chat"
	appsession "fkteams/internal/app/session"
	"fkteams/internal/bootstrap"
	domainmessage "fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/testmodel"
)

func TestAssistantUIRoundTripPreservesHistoryAndUsesExistingTools(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", root)
	dependencies, err := bootstrap.NewExecutionDependencies()
	if err != nil {
		t.Fatal(err)
	}
	ctx := domainsession.WithID(dependencies.Context(context.Background()), domainsession.AssistantID)
	sessionDir := filepath.Join(root, "sessions", domainsession.AssistantID)
	sessions := appsession.NewService(eventlog.NewSessionRepository(filepath.Dir(sessionDir)))
	if _, err := sessions.OpenAssistant(ctx); err != nil {
		t.Fatal(err)
	}
	const ui = "请填写名字。\n```openui-lang\nroot = Card(\"更换称呼\", [Form(\"identity\", [Input(\"name\", \"新名字\", \"小空\", true)], \"提交\", \"请修改你的名字\")])\n```"
	const action = "【助手界面操作】\n" + `{"version":1,"source":"answer:text:0","message":"请修改你的名字","formState":{"identity":{"name":{"value":"小空"}}}}`
	model := testmodel.New()
	model.EnqueueStream(testmodel.AssistantMessage(ui))
	model.EnqueueStream(domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{{ID: "rename", Type: "function", Function: domainmessage.FunctionCall{Name: "assistant_rename", Arguments: `{"name":"小空"}`}}}})
	model.EnqueueStream(testmodel.AssistantMessage("以后叫我小空。"))
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
	history := eventlog.NewHistoryRecorder()
	history.SetSessionDir(sessionDir)
	service := appchat.NewService()
	for i, input := range []string{"用表单让我填写你的新名字", action} {
		_, err := service.RunTurn(ctx, appchat.TurnRequest{
			SessionID: domainsession.AssistantID, RunID: fmt.Sprintf("ui-turn-%d", i), Runner: runner,
			Input: domainmessage.TurnInput{Context: history.ProjectContextMessages(0), Message: testmodel.UserMessage(input)}, History: history,
		})
		if err != nil {
			t.Fatal(err)
		}
		// 每轮从持久化事实恢复，避免只验证同一内存实例。
		reloaded := eventlog.NewHistoryRecorder()
		if err := reloaded.LoadFromFile(filepath.Join(sessionDir, eventlog.TranscriptFileName)); err != nil {
			t.Fatal(err)
		}
		history = reloaded
	}
	meta, err := sessions.Get(ctx, domainsession.AssistantID)
	if err != nil || meta.Title != "小空" {
		t.Fatalf("UI follow-up did not execute the existing rename tool: %#v, %v", meta, err)
	}
	calls := model.StreamCalls()
	if len(calls) != 3 {
		t.Fatalf("model calls = %d, want 3", len(calls))
	}
	for _, expected := range []string{ui, action} {
		found := false
		for _, message := range calls[1].Input {
			found = found || message.Content == expected
		}
		if !found {
			t.Fatalf("second turn lost UI context %q", expected)
		}
	}
}

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
