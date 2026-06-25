package common_test

import (
	"context"
	"strings"
	"testing"

	agentscommon "fkteams/agents/common"
	_ "fkteams/bootstrap/runtimes"
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	checkpointmemory "fkteams/internal/runtime/checkpoint/memory"
	runtimeregistry "fkteams/internal/runtime/registry"
	"fkteams/internal/testmodel"
)

func TestAgentBuilderRunsWithInjectedTestModel(t *testing.T) {
	ctx := context.Background()
	cm := testmodel.New().EnqueueStream(testmodel.AssistantMessage("builder-ok"))

	agent, err := agentscommon.NewAgentBuilder("builder_test", "builder test agent").
		WithModel(cm).
		WithInstruction("you are a {role}").
		WithTemplateVar("role", "tester").
		Build(ctx)
	if err != nil {
		t.Fatalf("build agent: %v", err)
	}

	runner, err := runtimeregistry.Engine().NewRunner(ctx, runtimeport.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: checkpointmemory.NewStore(),
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	var events []domainevent.Event
	_, err = runner.Run(ctx, domainmessage.TurnInput{
		Message: domainmessage.Message{Role: domainmessage.RoleUser, Content: "ping"},
	}, runtimeport.RunOptions{
		RunID:        "builder-test",
		CheckpointID: "builder-test",
		Sink: func(event domainevent.Event) error {
			events = append(events, event)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	calls := cm.StreamCalls()
	if len(calls) == 0 {
		t.Fatal("expected model calls")
	}
	input := calls[0].Input
	if len(input) < 3 {
		t.Fatalf("expected system, user and injected context messages, got %#v", input)
	}
	if input[0].Role != domainmessage.RoleSystem || !strings.Contains(input[0].Content, "you are a tester") {
		t.Fatalf("expected formatted system prompt, got %#v", input[0])
	}
	if input[len(input)-2].Role != domainmessage.RoleUser || input[len(input)-2].Content != "ping" {
		t.Fatalf("expected user message before dynamic context, got %#v", input)
	}
	assertInjectedContext(t, input[len(input)-1])
}

func assertInjectedContext(t *testing.T, msg domainmessage.Message) {
	t.Helper()

	if msg.Role != domainmessage.RoleUser {
		t.Fatalf("expected injected context to be user message, got %s", msg.Role)
	}
	for _, want := range []string{"<system-reminder>", "当前时间", "工作目录"} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("expected injected context to contain %q, got %q", want, msg.Content)
		}
	}
}
