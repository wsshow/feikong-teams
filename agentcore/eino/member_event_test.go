package eino

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"fkteams/agentcore"
	"fkteams/common"
	"fkteams/internal/testmodel"
	"fkteams/tools/ask"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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

	got := runAgentForTest(t, ctx, parentAgent, true)

	parentStartIdx := requireEventIndex(t, got, func(event agentcore.Event) bool {
		return event.Type == agentcore.EventToolStart &&
			event.ToolCallID == "parent-member-call" &&
			event.ToolName == "ask_fkagent_member" &&
			event.ToolCallRef != ""
	}, "parent member tool start")
	memberReasoningIdx := requireEventIndex(t, got, func(event agentcore.Event) bool {
		return event.MemberCallID == "parent-member-call" &&
			event.ParentToolCallID == "parent-member-call" &&
			event.MemberToolName == "ask_fkagent_member" &&
			event.MemberName == "member" &&
			event.DeltaKind == agentcore.DeltaReasoning &&
			strings.Contains(event.Content, "member-thinking")
	}, "member-scoped reasoning")
	memberToolStartIdx := requireEventIndex(t, got, func(event agentcore.Event) bool {
		return event.MemberCallID == "parent-member-call" &&
			event.Type == agentcore.EventToolStart &&
			event.ToolName == "member_echo" &&
			event.ToolCallRef != "" &&
			event.ToolCallIndex != nil &&
			*event.ToolCallIndex == 0
	}, "member-scoped tool start")
	memberToolResultIdx := requireEventIndex(t, got, func(event agentcore.Event) bool {
		return event.MemberCallID == "parent-member-call" &&
			(event.Type == agentcore.EventToolUpdate || event.Type == agentcore.EventToolEnd) &&
			event.ToolName == "member_echo" &&
			event.ToolCallRef != ""
	}, "member-scoped tool result")

	requireBefore(t, got, parentStartIdx, memberReasoningIdx, "parent member tool start", "member reasoning")
	requireBefore(t, got, memberReasoningIdx, memberToolStartIdx, "member reasoning", "member tool start")
	requireBefore(t, got, memberToolStartIdx, memberToolResultIdx, "member tool start", "member tool result")
}

func TestAdaptInterruptsKeepsMemberScope(t *testing.T) {
	order := 2
	got := adaptInterruptsFromRunner([]*adk.InterruptCtx{{
		ID:          "interrupt-1",
		IsRootCause: true,
		Info:        "question",
	}}, MemberScope{
		CallID:   "member-call-1",
		ToolName: "ask_fkagent_researcher",
		Name:     "researcher",
	}, &order)

	if len(got) != 1 {
		t.Fatalf("interrupt count = %d, want 1", len(got))
	}
	if got[0].MemberCallID != "member-call-1" {
		t.Fatalf("member_call_id = %q, want member-call-1", got[0].MemberCallID)
	}
	if got[0].MemberToolName != "ask_fkagent_researcher" {
		t.Fatalf("member_tool_name = %q", got[0].MemberToolName)
	}
	if got[0].MemberName != "researcher" {
		t.Fatalf("member_name = %q", got[0].MemberName)
	}
	if got[0].MemberOrder == nil || *got[0].MemberOrder != order {
		t.Fatalf("member_order = %#v, want %d", got[0].MemberOrder, order)
	}
}

func TestAdaptInterruptsUnwrapsInterruptPayload(t *testing.T) {
	metadataOrder := 4
	fallbackOrder := 7
	got := adaptInterruptsFromRunner([]*adk.InterruptCtx{{
		ID:          "interrupt-1",
		IsRootCause: true,
		Info: agentcore.InterruptPayload{
			Info: "question",
			Metadata: agentcore.InterruptMetadata{
				MemberCallID:   "member-call-from-payload",
				MemberToolName: "ask_fkagent_writer",
				MemberName:     "writer",
				MemberOrder:    &metadataOrder,
			},
		},
	}}, MemberScope{
		CallID:   "member-call-from-scope",
		ToolName: "ask_fkagent_researcher",
		Name:     "researcher",
	}, &fallbackOrder)

	if len(got) != 1 {
		t.Fatalf("interrupt count = %d, want 1", len(got))
	}
	if got[0].Info != "question" {
		t.Fatalf("info = %#v, want question", got[0].Info)
	}
	if got[0].MemberCallID != "member-call-from-payload" {
		t.Fatalf("member_call_id = %q, want member-call-from-payload", got[0].MemberCallID)
	}
	if got[0].MemberToolName != "ask_fkagent_writer" {
		t.Fatalf("member_tool_name = %q", got[0].MemberToolName)
	}
	if got[0].MemberName != "writer" {
		t.Fatalf("member_name = %q", got[0].MemberName)
	}
	if got[0].MemberOrder == nil || *got[0].MemberOrder != metadataOrder {
		t.Fatalf("member_order = %#v, want %d", got[0].MemberOrder, metadataOrder)
	}
}

func TestAgentToolWrapperKeepsResumeCapability(t *testing.T) {
	ctx := context.Background()
	inner := &resumeRecordingAgent{name: "member"}
	safe := WrapErrorSafe(inner)
	if _, ok := safe.(adk.ResumableAgent); !ok {
		t.Fatalf("WrapErrorSafe result does not implement adk.ResumableAgent")
	}

	wrapped := &agentToolNameAgent{
		inner:       safe,
		toolName:    "ask_fkagent_member",
		displayName: "member",
	}
	resumable, ok := any(wrapped).(adk.ResumableAgent)
	if !ok {
		t.Fatalf("agentToolNameAgent does not implement adk.ResumableAgent")
	}

	iter := resumable.Resume(ctx, &adk.ResumeInfo{WasInterrupted: true})
	event, ok := iter.Next()
	if !ok {
		t.Fatalf("resume iterator ended without event")
	}
	if event.Err != nil {
		t.Fatalf("resume event error = %v", event.Err)
	}
	if !inner.resumed {
		t.Fatalf("inner Resume was not called")
	}
	if event.Output == nil || event.Output.MessageOutput == nil || event.Output.MessageOutput.Message == nil {
		t.Fatalf("resume event missing message output: %#v", event)
	}
	if event.Output.MessageOutput.Message.Content != "resumed" {
		t.Fatalf("resume content = %q, want resumed", event.Output.MessageOutput.Message.Content)
	}
}

func TestMemberAskInterruptResumesInsideMemberAgent(t *testing.T) {
	ctx := context.Background()
	askTools, err := ask.GetTools()
	if err != nil {
		t.Fatalf("create ask tools: %v", err)
	}

	memberModel := testmodel.New().
		EnqueueStream(agentcore.Message{Role: agentcore.RoleAssistant, ToolCalls: []agentcore.ToolCall{{
			ID:    "member-ask-call",
			Index: intPtr(0),
			Type:  "function",
			Function: agentcore.FunctionCall{
				Name:      "ask_questions",
				Arguments: `{"question":"Need input?","options":["A","B"]}`,
			},
		}}}).
		EnqueueStream(testmodel.AssistantMessage("member resumed with answer"))
	memberAgent, err := NewChatModelAgent(ctx, &agentcore.ChatAgentConfig{
		Name:               "member",
		Description:        "member",
		Model:              memberModel,
		Tools:              askTools,
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
				Arguments: `{"request":"ask the user and continue"}`,
			},
		}}}).
		EnqueueStream(testmodel.AssistantMessage("parent done"))
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
	var seenMemberInterrupt bool
	_, err = runner.Run(ctx, agentcore.TurnInput{
		Message: agentcore.Message{Role: agentcore.RoleUser, Content: "start"},
	}, agentcore.RunOptions{
		RunID:        "member-ask-resume-test",
		CheckpointID: "member-ask-resume-test",
		Sink: func(event agentcore.Event) error {
			got = append(got, event)
			return nil
		},
		InterruptHandler: func(_ context.Context, interrupts []agentcore.Interrupt) (map[string]any, error) {
			result := make(map[string]any, len(interrupts))
			for _, ic := range interrupts {
				if ic.MemberCallID == "parent-member-call" && ic.MemberToolName == "ask_fkagent_member" {
					seenMemberInterrupt = true
				}
				if ic.IsRootCause {
					result[ic.ID] = &ask.AskResponse{Selected: []string{"A"}}
				}
			}
			return result, nil
		},
	})
	if err != nil {
		t.Fatalf("run with member ask interrupt: %v; events=%#v", err, got)
	}
	if !seenMemberInterrupt {
		t.Fatalf("member ask interrupt was not marked with parent member call")
	}

	requireEventIndex(t, got, func(event agentcore.Event) bool {
		return event.MemberCallID == "parent-member-call" &&
			event.Type == agentcore.EventMessageEnd &&
			event.Content == "member resumed with answer"
	}, "member resumed output")
	requireEventIndex(t, got, func(event agentcore.Event) bool {
		return event.Type == agentcore.EventMessageEnd &&
			event.Role == agentcore.RoleAssistant &&
			event.Content == "parent done"
	}, "parent final output")

	memberCalls := memberModel.StreamCalls()
	if len(memberCalls) < 2 {
		t.Fatalf("member model stream calls = %d, want at least 2", len(memberCalls))
	}
	if !messagesContain(memberCalls[1].Input, "A") {
		t.Fatalf("member resume input does not contain ask answer: %#v", memberCalls[1].Input)
	}
}

func TestMemberRuntimeAskDoesNotBlockParallelMember(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	askTools, err := ask.GetTools()
	if err != nil {
		t.Fatalf("create ask tools: %v", err)
	}

	askerModel := testmodel.New().
		EnqueueStream(agentcore.Message{Role: agentcore.RoleAssistant, ToolCalls: []agentcore.ToolCall{{
			ID:    "asker-ask-call",
			Index: intPtr(0),
			Type:  "function",
			Function: agentcore.FunctionCall{
				Name:      "ask_questions",
				Arguments: `{"question":"Need input?","options":["yes","no"]}`,
			},
		}}}).
		EnqueueStream(testmodel.AssistantMessage("asker done"))
	askerAgent, err := NewChatModelAgent(ctx, &agentcore.ChatAgentConfig{
		Name:               "asker",
		Description:        "asker",
		Model:              askerModel,
		Tools:              askTools,
		MaxIterations:      4,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create asker agent: %v", err)
	}

	workerModel := testmodel.New().EnqueueStream(testmodel.AssistantMessage("worker done"))
	workerAgent, err := NewChatModelAgent(ctx, &agentcore.ChatAgentConfig{
		Name:               "worker",
		Description:        "worker",
		Model:              workerModel,
		MaxIterations:      2,
		EmitInternalEvents: true,
	})
	if err != nil {
		t.Fatalf("create worker agent: %v", err)
	}

	agentTools, err := NewAgentTools(ctx, []agentcore.Agent{askerAgent, workerAgent}, agentcore.AgentToolConfig{
		ToolName: func(name string, _ int) string { return "ask_fkagent_" + name },
	})
	if err != nil {
		t.Fatalf("create agent tools: %v", err)
	}

	parentModel := testmodel.New().
		EnqueueStream(agentcore.Message{Role: agentcore.RoleAssistant, ToolCalls: []agentcore.ToolCall{
			{
				ID:    "parent-asker-call",
				Index: intPtr(0),
				Type:  "function",
				Function: agentcore.FunctionCall{
					Name:      "ask_fkagent_asker",
					Arguments: `{"request":"ask the user"}`,
				},
			},
			{
				ID:    "parent-worker-call",
				Index: intPtr(1),
				Type:  "function",
				Function: agentcore.FunctionCall{
					Name:      "ask_fkagent_worker",
					Arguments: `{"request":"finish independently"}`,
				},
			},
		}}).
		EnqueueStream(testmodel.AssistantMessage("parent done"))
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

	askReqCh := make(chan ask.RuntimeRequest, 1)
	askRespCh := make(chan *ask.AskResponse, 1)
	ctx = ask.WithRuntimeHandler(ctx, func(ctx context.Context, req ask.RuntimeRequest) (*ask.AskResponse, error) {
		askReqCh <- req
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case resp := <-askRespCh:
			return resp, nil
		}
	})

	eventCh := make(chan agentcore.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(ctx, agentcore.TurnInput{
			Message: agentcore.Message{Role: agentcore.RoleUser, Content: "start"},
		}, agentcore.RunOptions{
			RunID:        "member-runtime-ask-parallel-test",
			CheckpointID: "member-runtime-ask-parallel-test",
			Sink: func(event agentcore.Event) error {
				select {
				case eventCh <- event:
				case <-ctx.Done():
				}
				return nil
			},
			InterruptHandler: func(context.Context, []agentcore.Interrupt) (map[string]any, error) {
				return nil, fmt.Errorf("member runtime ask reached parent interrupt handler")
			},
		})
		errCh <- runErr
	}()

	req := waitAskRuntimeRequest(t, ctx, askReqCh)
	if req.Metadata.MemberCallID != "parent-asker-call" {
		t.Fatalf("ask member call ID = %q, want parent-asker-call", req.Metadata.MemberCallID)
	}

	waitEvent(t, ctx, eventCh, func(event agentcore.Event) bool {
		return event.MemberCallID == "parent-worker-call" &&
			event.Type == agentcore.EventMessageEnd &&
			event.Content == "worker done"
	}, "worker completion before ask answer")

	askRespCh <- &ask.AskResponse{AskID: req.ID, Selected: []string{"yes"}}

	waitEvent(t, ctx, eventCh, func(event agentcore.Event) bool {
		return event.MemberCallID == "parent-asker-call" &&
			event.Type == agentcore.EventMessageEnd &&
			event.Content == "asker done"
	}, "asker completion after answer")
	waitEvent(t, ctx, eventCh, func(event agentcore.Event) bool {
		return event.Type == agentcore.EventMessageEnd &&
			event.Role == agentcore.RoleAssistant &&
			event.Content == "parent done"
	}, "parent completion")

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runner error = %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("runner did not finish: %v", ctx.Err())
	}
}

func waitAskRuntimeRequest(t *testing.T, ctx context.Context, ch <-chan ask.RuntimeRequest) ask.RuntimeRequest {
	t.Helper()
	select {
	case req := <-ch:
		return req
	case <-ctx.Done():
		t.Fatalf("timed out waiting for ask runtime request: %v", ctx.Err())
		return ask.RuntimeRequest{}
	}
}

func waitEvent(t *testing.T, ctx context.Context, ch <-chan agentcore.Event, match func(agentcore.Event) bool, label string) agentcore.Event {
	t.Helper()
	for {
		select {
		case event := <-ch:
			if match(event) {
				return event
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s: %v", label, ctx.Err())
			return agentcore.Event{}
		}
	}
}

type memberEchoRequest struct {
	Text string `json:"text"`
}

type memberEchoResponse struct {
	Text string `json:"text"`
}

type resumeRecordingAgent struct {
	name    string
	resumed bool
}

func (a *resumeRecordingAgent) Name(context.Context) string {
	return a.name
}

func (a *resumeRecordingAgent) Description(context.Context) string {
	return "resume recording agent"
}

func (a *resumeRecordingAgent) Run(context.Context, *adk.AgentInput, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	return singleMessageAgentIter("run")
}

func (a *resumeRecordingAgent) Resume(context.Context, *adk.ResumeInfo, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	a.resumed = true
	return singleMessageAgentIter("resumed")
}

func singleMessageAgentIter(content string) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()
		gen.Send(&adk.AgentEvent{
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: schema.AssistantMessage(content, nil),
					Role:    schema.Assistant,
				},
			},
		})
	}()
	return iter
}

func messagesContain(messages []agentcore.Message, text string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}
