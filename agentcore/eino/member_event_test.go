package eino

import (
	"context"
	"strings"
	"testing"

	"fkteams/agentcore"
	"fkteams/common"
	"fkteams/internal/testmodel"
)

func TestAgentToolMemberEventsKeepScopeForReasoningAndTools(t *testing.T) {
	ctx := context.Background()
	memberTool, err := agentcore.InferTool("member_echo", "member echo", func(_ context.Context, req *memberEchoRequest) (*memberEchoResponse, error) {
		return &memberEchoResponse{Text: "tool:" + req.Text}, nil
	})
	if err != nil {
		t.Fatalf("create member tool: %v", err)
	}

	memberModel := testmodel.New().
		EnqueueStream(
			agentcore.Message{Role: agentcore.RoleAssistant, ReasoningContent: "member-thinking"},
			agentcore.Message{Role: agentcore.RoleAssistant, ToolCalls: []agentcore.ToolCall{{
				ID:    "member-tool-call",
				Index: intPtr(0),
				Type:  "function",
				Function: agentcore.FunctionCall{
					Name:      "member_echo",
					Arguments: `{"text":"hello"}`,
				},
			}}},
		).
		EnqueueStream(testmodel.AssistantMessage("member-done"))
	memberAgent, err := NewChatModelAgent(ctx, &agentcore.ChatAgentConfig{
		Name:               "member",
		Description:        "member",
		Model:              memberModel,
		Tools:              []agentcore.Tool{memberTool},
		MaxIterations:      4,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create member agent: %v", err)
	}

	agentTools, err := NewAgentTools(ctx, []agentcore.Agent{memberAgent}, agentcore.AgentToolConfig{
		ToolName: func(string, int) string { return "ask_fkagent_member" },
	})
	if err != nil {
		t.Fatalf("create agent tools: %v", err)
	}

	parentModel := testmodel.New().
		EnqueueStream(agentcore.Message{Role: agentcore.RoleAssistant, ToolCalls: []agentcore.ToolCall{{
			ID:    "parent-member-call",
			Index: intPtr(0),
			Type:  "function",
			Function: agentcore.FunctionCall{
				Name:      "ask_fkagent_member",
				Arguments: `{"request":"do member task"}`,
			},
		}}}).
		EnqueueStream(testmodel.AssistantMessage("parent-done"))
	parentAgent, err := NewChatModelAgent(ctx, &agentcore.ChatAgentConfig{
		Name:               "parent",
		Description:        "parent",
		Model:              parentModel,
		Tools:              agentTools,
		MaxIterations:      4,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create parent agent: %v", err)
	}

	runner, err := NewRunnerFromConfig(ctx, agentcore.RunnerConfig{
		Agent:           parentAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	var got []agentcore.Event
	_, err = runner.Run(ctx, agentcore.TurnInput{
		Message: agentcore.Message{Role: agentcore.RoleUser, Content: "start"},
	}, agentcore.RunOptions{
		RunID:        "member-scope-test",
		CheckpointID: "member-scope-test",
		Sink: func(event agentcore.Event) error {
			got = append(got, event)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}

	var sawMemberReasoning, sawMemberToolStart, sawMemberToolResult bool
	for _, event := range got {
		if event.MemberCallID == "parent-member-call" && event.DeltaKind == agentcore.DeltaReasoning && strings.Contains(event.Content, "member-thinking") {
			sawMemberReasoning = true
		}
		if event.MemberCallID == "parent-member-call" && event.Type == agentcore.EventToolStart && event.ToolName == "member_echo" && event.ToolCallRef != "" {
			sawMemberToolStart = true
		}
		if event.MemberCallID == "parent-member-call" && (event.Type == agentcore.EventToolUpdate || event.Type == agentcore.EventToolEnd) && event.ToolName == "member_echo" && event.ToolCallRef != "" {
			sawMemberToolResult = true
		}
	}
	if !sawMemberReasoning {
		t.Fatalf("missing member-scoped reasoning event; events=%#v", got)
	}
	if !sawMemberToolStart {
		t.Fatalf("missing member-scoped tool start event; events=%#v", got)
	}
	if !sawMemberToolResult {
		t.Fatalf("missing member-scoped tool result event; events=%#v", got)
	}
}

type memberEchoRequest struct {
	Text string `json:"text"`
}

type memberEchoResponse struct {
	Text string `json:"text"`
}
