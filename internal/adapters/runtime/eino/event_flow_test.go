package eino

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	checkpointmemory "fkteams/internal/runtime/checkpoint/memory"
	"fkteams/internal/testmodel"
)

func TestRunnerEmitsLifecycleEventsInOrder(t *testing.T) {
	ctx := context.Background()
	model := testmodel.New().EnqueueStream(testmodel.AssistantMessage("done"))
	agent, err := NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:          "lifecycle",
		Description:   "lifecycle",
		Model:         model,
		MaxIterations: 2,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := runAgentForTestResult(t, ctx, agent, true)
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if len(got) < 6 {
		t.Fatalf("expected lifecycle and message events, got %#v", got)
	}
	if got[0].Type != domainevent.TypeAgentStarted || got[0].RunID != "event-flow-test" {
		t.Fatalf("first event = %#v, want agent_start", got[0])
	}
	if got[1].Type != domainevent.TypeTurnStarted || got[1].TurnID != "event-flow-test:turn:1" {
		t.Fatalf("second event = %#v, want turn_start", got[1])
	}

	userStartIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeUserMessage && event.Role == domainmessage.RoleUser && event.Content == "start"
	}, "user message")
	assistantIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantText &&
			event.Role == domainmessage.RoleAssistant &&
			event.Content == "done"
	}, "assistant output")
	turnEndIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeTurnCompleted
	}, "turn end")
	agentEndIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAgentCompleted
	}, "agent end")

	requireBefore(t, got, userStartIdx, assistantIdx, "user message", "assistant output")
	requireBefore(t, got, assistantIdx, turnEndIdx, "assistant output", "turn end")
	requireBefore(t, got, turnEndIdx, agentEndIdx, "turn end", "agent end")
	if agentEndIdx != len(got)-1 {
		t.Fatalf("agent_end should be final event; index=%d len=%d events=%#v", agentEndIdx, len(got), got)
	}
}

func TestRunnerEmitsErrorAndClosesLifecycleOnModelError(t *testing.T) {
	ctx := context.Background()
	modelErr := errors.New("model boom")
	model := testmodel.New().EnqueueGenerate(domainmessage.Message{}, modelErr)
	agent, err := NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:          "lifecycle-error",
		Description:   "lifecycle error",
		Model:         model,
		MaxIterations: 2,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := runAgentForTestResult(t, ctx, agent, false)
	if err != nil {
		t.Fatalf("run error = %v, want nil; events=%#v", err, got)
	}
	if len(got) < 3 {
		t.Fatalf("expected start and error events, got %#v", got)
	}
	if got[0].Type != domainevent.TypeAgentStarted || got[1].Type != domainevent.TypeTurnStarted {
		t.Fatalf("expected agent_start then turn_start, got %#v", got)
	}
	errorIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeError && strings.Contains(event.Error, "model boom")
	}, "model error")
	turnEndIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeTurnCompleted
	}, "turn end after error")
	agentEndIdx := requireEventIndex(t, got, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAgentCompleted
	}, "agent end after error")
	requireBefore(t, got, errorIdx, turnEndIdx, "model error", "turn end")
	requireBefore(t, got, turnEndIdx, agentEndIdx, "turn end", "agent end")
	if agentEndIdx != len(got)-1 {
		t.Fatalf("agent_end should be final event; index=%d len=%d events=%#v", agentEndIdx, len(got), got)
	}
}

func TestStreamingRunEmitsOrderedToolFlowEvents(t *testing.T) {
	ctx := context.Background()
	toolCallIndex := 0
	echoTool, err := runtimeport.InferTool("flow_echo", "echo text", func(_ context.Context, req *flowEchoRequest) (*flowEchoResponse, error) {
		return &flowEchoResponse{Text: "echo:" + req.Text}, nil
	})
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	model := testmodel.New().
		EnqueueStream(
			domainmessage.Message{Role: domainmessage.RoleAssistant, ReasoningContent: "think "},
			domainmessage.Message{Role: domainmessage.RoleAssistant, Content: "draft "},
			domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{{
				ID:    "flow-tool-call",
				Index: &toolCallIndex,
				Type:  "function",
				Function: domainmessage.FunctionCall{
					Name:      "flow_echo",
					Arguments: `{"text":`,
				},
			}}},
			domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{{
				Index: &toolCallIndex,
				Type:  "function",
				Function: domainmessage.FunctionCall{
					Arguments: `"hello"}`,
				},
			}}},
		).
		EnqueueStream(testmodel.AssistantMessage("final"))
	agent, err := NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:               "flow",
		Description:        "flow",
		Model:              model,
		Tools:              []runtimeport.Tool{echoTool},
		MaxIterations:      4,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	events := runAgentForTest(t, ctx, agent, true)
	reasoningIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantReasoning &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaReasoning &&
			event.Content == "think "
	}, "reasoning delta")
	outputIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantText &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaOutput &&
			event.Content == "draft "
	}, "output delta")
	firstArgsIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallArguments &&
			event.DeltaKind == domainevent.DeltaToolArgs &&
			event.ToolCallID == "flow-tool-call" &&
			event.ToolName == "flow_echo" &&
			event.Content == `{"text":`
	}, "first tool args delta")
	secondArgsIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallArguments &&
			event.DeltaKind == domainevent.DeltaToolArgs &&
			event.ToolCallID == "flow-tool-call" &&
			event.ToolName == "flow_echo" &&
			event.Content == `"hello"}`
	}, "second tool args delta")
	messageEndIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantCompleted &&
			event.Role == domainmessage.RoleAssistant &&
			event.Content == "draft " &&
			event.ReasoningContent == "think " &&
			len(event.ToolCalls) == 1 &&
			event.ToolCalls[0].ID == "flow-tool-call" &&
			event.ToolCalls[0].Function.Name == "flow_echo" &&
			event.ToolCalls[0].Function.Arguments == `{"text":"hello"}`
	}, "assistant message end with aggregated tool call")
	toolStartIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallStarted &&
			event.ToolCallID == "flow-tool-call" &&
			event.ToolCallRef != "" &&
			event.ToolName == "flow_echo" &&
			event.ToolArgs == `{"text":"hello"}` &&
			event.ToolCallIndex != nil &&
			*event.ToolCallIndex == 0
	}, "tool start")
	toolStart := events[toolStartIdx]
	toolEndIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallCompleted &&
			event.ToolCallID == "flow-tool-call" &&
			event.ToolCallRef == toolStart.ToolCallRef &&
			event.ToolName == "flow_echo" &&
			strings.Contains(event.ToolResult, "echo:hello")
	}, "tool end")
	finalIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantText &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaOutput &&
			event.Content == "final"
	}, "final assistant delta")

	requireBefore(t, events, reasoningIdx, outputIdx, "reasoning", "output")
	requireBefore(t, events, outputIdx, firstArgsIdx, "output", "first tool args")
	requireBefore(t, events, firstArgsIdx, secondArgsIdx, "first tool args", "second tool args")
	requireBefore(t, events, secondArgsIdx, messageEndIdx, "second tool args", "message end")
	requireBefore(t, events, messageEndIdx, toolStartIdx, "message end", "tool start")
	requireBefore(t, events, toolStartIdx, toolEndIdx, "tool start", "tool end")
	requireBefore(t, events, toolEndIdx, finalIdx, "tool end", "final output")

	firstArgs := events[firstArgsIdx]
	secondArgs := events[secondArgsIdx]
	messageEnd := events[messageEndIdx]
	if firstArgs.ToolCallRef == "" {
		t.Fatalf("first tool args delta missing tool_call_ref: %#v", firstArgs)
	}
	if secondArgs.ToolCallRef != firstArgs.ToolCallRef {
		t.Fatalf("tool args refs changed: first=%q second=%q", firstArgs.ToolCallRef, secondArgs.ToolCallRef)
	}
	if messageEnd.ToolCallRefs[0] != firstArgs.ToolCallRef {
		t.Fatalf("message_end tool call ref = %q, want %q", messageEnd.ToolCallRefs[0], firstArgs.ToolCallRef)
	}
	if toolStart.ToolCallRef != firstArgs.ToolCallRef {
		t.Fatalf("tool_start ref = %q, want args ref %q", toolStart.ToolCallRef, firstArgs.ToolCallRef)
	}
}

func TestGenerateRunEmitsRegularMessageAndToolEvents(t *testing.T) {
	ctx := context.Background()
	toolCallIndex := 0
	echoTool, err := runtimeport.InferTool("generate_echo", "echo text", func(_ context.Context, req *flowEchoRequest) (*flowEchoResponse, error) {
		return &flowEchoResponse{Text: "echo:" + req.Text}, nil
	})
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	model := testmodel.New(
		domainmessage.Message{
			Role:             domainmessage.RoleAssistant,
			Content:          "regular-draft",
			ReasoningContent: "regular-thinking",
			ToolCalls: []domainmessage.ToolCall{{
				ID:    "generate-tool-call",
				Index: &toolCallIndex,
				Type:  "function",
				Function: domainmessage.FunctionCall{
					Name:      "generate_echo",
					Arguments: `{"text":"hello"}`,
				},
			}},
		},
		testmodel.AssistantMessage("regular-final"),
	)
	agent, err := NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:               "generate-flow",
		Description:        "generate flow",
		Model:              model,
		Tools:              []runtimeport.Tool{echoTool},
		MaxIterations:      4,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	events := runAgentForTest(t, ctx, agent, false)
	reasoningIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantReasoning &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaReasoning &&
			event.Content == "regular-thinking"
	}, "regular reasoning delta")
	outputIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantText &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaOutput &&
			event.Content == "regular-draft"
	}, "regular output delta")
	messageEndIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantCompleted &&
			event.Role == domainmessage.RoleAssistant &&
			event.Content == "regular-draft" &&
			event.ReasoningContent == "regular-thinking" &&
			len(event.ToolCalls) == 1 &&
			event.ToolCalls[0].ID == "generate-tool-call" &&
			event.ToolCalls[0].Function.Name == "generate_echo" &&
			event.ToolCalls[0].Function.Arguments == `{"text":"hello"}`
	}, "regular message end with tool call")
	toolStartIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallStarted &&
			event.ToolCallID == "generate-tool-call" &&
			event.ToolName == "generate_echo" &&
			event.ToolCallRef != "" &&
			event.ToolArgs == `{"text":"hello"}`
	}, "regular tool start")
	toolStart := events[toolStartIdx]
	toolEndIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallCompleted &&
			event.ToolCallID == "generate-tool-call" &&
			event.ToolCallRef == toolStart.ToolCallRef &&
			event.ToolName == "generate_echo" &&
			strings.Contains(event.ToolResult, "echo:hello")
	}, "regular tool end")
	finalIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantText &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaOutput &&
			event.Content == "regular-final"
	}, "regular final delta")

	requireBefore(t, events, reasoningIdx, outputIdx, "regular reasoning", "regular output")
	requireBefore(t, events, outputIdx, messageEndIdx, "regular output", "regular message end")
	requireBefore(t, events, messageEndIdx, toolStartIdx, "regular message end", "regular tool start")
	requireBefore(t, events, toolStartIdx, toolEndIdx, "regular tool start", "regular tool end")
	requireBefore(t, events, toolEndIdx, finalIdx, "regular tool end", "regular final output")

	messageEnd := events[messageEndIdx]
	if messageEnd.ToolCallRefs[0] != toolStart.ToolCallRef {
		t.Fatalf("regular message_end tool call ref = %q, want %q", messageEnd.ToolCallRefs[0], toolStart.ToolCallRef)
	}
}

func TestUnknownToolCallEmitsToolEndWithHandlerResult(t *testing.T) {
	ctx := context.Background()
	toolCallIndex := 0
	dummyTool, err := runtimeport.InferTool("known_tool", "known tool", func(context.Context, *flowEchoRequest) (string, error) {
		return "known", nil
	})
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}
	model := testmodel.New(
		domainmessage.Message{
			Role: domainmessage.RoleAssistant,
			ToolCalls: []domainmessage.ToolCall{{
				ID:    "unknown-tool-call",
				Index: &toolCallIndex,
				Type:  "function",
				Function: domainmessage.FunctionCall{
					Name:      "search",
					Arguments: `{"query":"anthropic ipo"}`,
				},
			}},
		},
		testmodel.AssistantMessage("final"),
	)
	agent, err := NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:               "unknown-tool-flow",
		Description:        "unknown tool flow",
		Model:              model,
		Tools:              []runtimeport.Tool{dummyTool},
		MaxIterations:      4,
		EmitInternalEvents: true,
		UnknownToolHandler: func(context.Context, string, string) (string, error) {
			return "Tool 'search' does not exist. Please check the available tools and try again.", nil
		},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	events := runAgentForTest(t, ctx, agent, false)
	toolStartIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallStarted &&
			event.ToolCallID == "unknown-tool-call" &&
			event.ToolName == "search" &&
			event.ToolCallRef != ""
	}, "unknown tool start")
	toolStart := events[toolStartIdx]
	toolEndIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallCompleted &&
			event.ToolCallID == "unknown-tool-call" &&
			event.ToolCallRef == toolStart.ToolCallRef &&
			event.ToolName == "search" &&
			strings.Contains(event.ToolResult, "does not exist")
	}, "unknown tool end")
	finalIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantText &&
			event.Role == domainmessage.RoleAssistant &&
			event.DeltaKind == domainevent.DeltaOutput &&
			event.Content == "final"
	}, "unknown tool final delta")
	requireBefore(t, events, toolStartIdx, toolEndIdx, "unknown tool start", "unknown tool end")
	requireBefore(t, events, toolEndIdx, finalIdx, "unknown tool end", "unknown tool final output")
}

func TestRunGeneratesToolIdentityWhenModelOmitsToolCallID(t *testing.T) {
	ctx := context.Background()
	toolCallIndex := 0
	echoTool, err := runtimeport.InferTool("generated_id_echo", "echo text", func(_ context.Context, req *flowEchoRequest) (*flowEchoResponse, error) {
		return &flowEchoResponse{Text: "echo:" + req.Text}, nil
	})
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	model := testmodel.New().
		EnqueueStream(
			domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{{
				Index: &toolCallIndex,
				Type:  "function",
				Function: domainmessage.FunctionCall{
					Name:      "generated_id_echo",
					Arguments: `{"text":`,
				},
			}}},
			domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{{
				Index: &toolCallIndex,
				Type:  "function",
				Function: domainmessage.FunctionCall{
					Arguments: `"hello"}`,
				},
			}}},
		).
		EnqueueStream(testmodel.AssistantMessage("final"))
	agent, err := NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:               "generated-id-flow",
		Description:        "generated id flow",
		Model:              model,
		Tools:              []runtimeport.Tool{echoTool},
		MaxIterations:      4,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	events := runAgentForTest(t, ctx, agent, true)
	firstArgsIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallArguments &&
			event.DeltaKind == domainevent.DeltaToolArgs &&
			event.ToolName == "generated_id_echo" &&
			event.Content == `{"text":`
	}, "first generated-id tool args")
	secondArgsIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallArguments &&
			event.DeltaKind == domainevent.DeltaToolArgs &&
			event.ToolName == "generated_id_echo" &&
			event.Content == `"hello"}`
	}, "second generated-id tool args")
	messageEndIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeAssistantCompleted &&
			len(event.ToolCalls) == 1 &&
			event.ToolCalls[0].Function.Name == "generated_id_echo"
	}, "generated-id message end")
	toolStartIdx := requireEventIndex(t, events, func(event domainevent.Event) bool {
		return event.Type == domainevent.TypeToolCallStarted &&
			event.ToolName == "generated_id_echo"
	}, "generated-id tool start")

	firstArgs := events[firstArgsIdx]
	secondArgs := events[secondArgsIdx]
	messageEnd := events[messageEndIdx]
	toolStart := events[toolStartIdx]
	if !strings.HasPrefix(firstArgs.ToolCallID, "fk_tool_") {
		t.Fatalf("generated tool_call_id = %q, want fk_tool_ prefix", firstArgs.ToolCallID)
	}
	if firstArgs.ToolCallRef != "tool_call:"+firstArgs.ToolCallID {
		t.Fatalf("generated ref = %q, want tool_call:%s", firstArgs.ToolCallRef, firstArgs.ToolCallID)
	}
	if secondArgs.ToolCallID != firstArgs.ToolCallID || secondArgs.ToolCallRef != firstArgs.ToolCallRef {
		t.Fatalf("generated identity changed: first=%s/%s second=%s/%s", firstArgs.ToolCallID, firstArgs.ToolCallRef, secondArgs.ToolCallID, secondArgs.ToolCallRef)
	}
	if messageEnd.ToolCalls[0].ID != firstArgs.ToolCallID {
		t.Fatalf("message_end tool id = %q, want %q", messageEnd.ToolCalls[0].ID, firstArgs.ToolCallID)
	}
	if messageEnd.ToolCallRefs[0] != firstArgs.ToolCallRef {
		t.Fatalf("message_end ref = %q, want %q", messageEnd.ToolCallRefs[0], firstArgs.ToolCallRef)
	}
	if toolStart.ToolCallID != firstArgs.ToolCallID || toolStart.ToolCallRef != firstArgs.ToolCallRef {
		t.Fatalf("tool_start identity = %s/%s, want %s/%s", toolStart.ToolCallID, toolStart.ToolCallRef, firstArgs.ToolCallID, firstArgs.ToolCallRef)
	}
}

func runAgentForTest(t *testing.T, ctx context.Context, agent runtimeport.Agent, streaming bool) []domainevent.Event {
	t.Helper()

	events, err := runAgentForTestResult(t, ctx, agent, streaming)
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	return events
}

func runAgentForTestResult(t *testing.T, ctx context.Context, agent runtimeport.Agent, streaming bool) ([]domainevent.Event, error) {
	t.Helper()

	runner, err := NewRunnerFromConfig(ctx, runtimeport.RunnerConfig{
		Agent:           agent,
		EnableStreaming: streaming,
		CheckpointStore: checkpointmemory.NewStore(),
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	var events []domainevent.Event
	_, err = runner.Run(ctx, domainmessage.TurnInput{
		Message: domainmessage.Message{Role: domainmessage.RoleUser, Content: "start"},
	}, runtimeport.RunOptions{
		RunID:        "event-flow-test",
		CheckpointID: "event-flow-test",
		Sink: func(event domainevent.Event) error {
			events = append(events, event)
			return nil
		},
	})
	return events, err
}

func requireEventIndex(t *testing.T, events []domainevent.Event, match func(domainevent.Event) bool, name string) int {
	t.Helper()
	for i, event := range events {
		if match(event) {
			return i
		}
	}
	t.Fatalf("missing %s event; events=%#v", name, events)
	return -1
}

func requireBefore(t *testing.T, events []domainevent.Event, before, after int, beforeName, afterName string) {
	t.Helper()
	if before >= after {
		t.Fatalf("expected %s before %s; before=%d after=%d events=%#v", beforeName, afterName, before, after, events)
	}
}

type flowEchoRequest struct {
	Text string `json:"text"`
}

type flowEchoResponse struct {
	Text string `json:"text"`
}
