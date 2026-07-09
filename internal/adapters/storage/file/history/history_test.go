package eventlog

import (
	appchat "fkteams/internal/app/chat"
	domainevent "fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranscriptProjectionBuildsTurnInput(t *testing.T) {
	sessionDir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(sessionDir)
	recorder.RecordEvent(Event{
		Type:    events.EventUserMessage,
		Content: "hello",
		Message: &message.Message{Role: message.RoleUser, Content: "hello"},
	})
	recorder.RecordEvent(Event{
		Type:      EventAssistantText,
		Role:      message.RoleAssistant,
		DeltaKind: events.DeltaOutput,
		AgentName: "coordinator",
		Content:   "world",
	})
	recorder.RecordEvent(Event{
		Type:      events.EventAssistantCompleted,
		Role:      message.RoleAssistant,
		AgentName: "coordinator",
		Content:   "world",
	})
	recorder.FinalizeCurrent()

	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(sessionDir, TranscriptFileName)); err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	input := appchat.BuildTurnInput(loaded, "next")
	if len(input.Context) != 2 {
		t.Fatalf("context count = %d, want 2: %#v", len(input.Context), input.Context)
	}
	if input.Context[0].Role != message.RoleUser || input.Context[0].Content != "hello" {
		t.Fatalf("user context = %#v", input.Context[0])
	}
	if input.Context[1].Role != message.RoleAssistant || input.Context[1].Content != "world" {
		t.Fatalf("assistant context = %#v", input.Context[1])
	}
	if input.Message.Content != "next" {
		t.Fatalf("input message = %q, want next", input.Message.Content)
	}
}

func TestTranscriptProjectionUsesLatestSummaryAsHistoryBoundary(t *testing.T) {
	sessionDir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(sessionDir)

	recorder.RecordEvent(Event{
		Type:    events.EventUserMessage,
		Content: "old question",
		Message: &message.Message{Role: message.RoleUser, Content: "old question"},
	})
	recorder.RecordEvent(Event{
		Type:      events.EventAssistantCompleted,
		Role:      message.RoleAssistant,
		AgentName: "coordinator",
		Content:   "old answer",
	})
	recorder.RecordEvent(Event{
		Type:      EventSystemNotice,
		AgentName: "系统",
		Content:   "对话上下文已压缩，旧消息已被总结摘要替代",
		Detail:    "summary of old conversation",
	})
	recorder.RecordEvent(Event{
		Type:    events.EventUserMessage,
		Content: "new question",
		Message: &message.Message{Role: message.RoleUser, Content: "new question"},
	})
	recorder.RecordEvent(Event{
		Type:      events.EventAssistantCompleted,
		Role:      message.RoleAssistant,
		AgentName: "coordinator",
		Content:   "new answer",
	})
	recorder.FinalizeCurrent()

	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(sessionDir, TranscriptFileName)); err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	summary, summarizedCount := loaded.GetSummary()
	if summary != "summary of old conversation" {
		t.Fatalf("summary = %q, want latest summary", summary)
	}
	if summarizedCount != 3 {
		t.Fatalf("summarized count = %d, want 3", summarizedCount)
	}

	input := appchat.BuildTurnInput(loaded, "next")
	if len(input.Context) != 3 {
		t.Fatalf("context count = %d, want 3: %#v", len(input.Context), input.Context)
	}
	if input.Context[0].Role != message.RoleSystem || !strings.Contains(input.Context[0].Content, "summary of old conversation") {
		t.Fatalf("summary context = %#v", input.Context[0])
	}
	if input.Context[1].Role != message.RoleUser || input.Context[1].Content != "new question" {
		t.Fatalf("new user context = %#v", input.Context[1])
	}
	if input.Context[2].Role != message.RoleAssistant || input.Context[2].Content != "new answer" {
		t.Fatalf("new assistant context = %#v", input.Context[2])
	}
	for _, ctx := range input.Context {
		if strings.Contains(ctx.Content, "old question") || strings.Contains(ctx.Content, "old answer") {
			t.Fatalf("context contains summarized old content: %#v", input.Context)
		}
		if strings.Contains(ctx.Content, "对话上下文已压缩") {
			t.Fatalf("context contains summary notice event: %#v", input.Context)
		}
	}
}

func TestTranscriptProjectionIgnoresNonSummaryNoticeDetails(t *testing.T) {
	sessionDir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(sessionDir)

	recorder.RecordEvent(Event{
		Type:      EventSystemNotice,
		AgentName: "系统",
		Content:   "dispatch progress",
		Detail:    `{"event_type":"op"}`,
	})
	recorder.FinalizeCurrent()

	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(sessionDir, TranscriptFileName)); err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	summary, summarizedCount := loaded.GetSummary()
	if summary != "" || summarizedCount != 0 {
		t.Fatalf("summary = %q/%d, want empty summary", summary, summarizedCount)
	}
}

func TestHistoryRecorderKeepsParentToolCallBeforeMemberMessage(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolCallStarted,
		AgentName:   "coordinator",
		ToolCallID:  "call_1",
		ToolCallRef: "tool_call:call_1",
		ToolCallRefs: map[int]string{
			0: "tool_call:call_1",
		},
		ToolCalls: []message.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: message.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventAssistantText,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaOutput,
		AgentName:      "researcher",
		Content:        "结果",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "Researcher",
		MemberOrder:    &toolIndex,
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].AgentName != "coordinator" {
		t.Fatalf("first message agent = %q, want coordinator", messages[0].AgentName)
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].Type != MsgTypeToolCall {
		t.Fatalf("first message event = %#v, want tool call", messages[0].Events)
	}
	if messages[1].MemberCallID != "call_1" {
		t.Fatalf("second message member_call_id = %q, want call_1", messages[1].MemberCallID)
	}
}

func TestHistoryRecorderPreservesMemberEventSequences(t *testing.T) {
	recorder := NewHistoryRecorder()
	memberOrder := 0

	recorder.RecordEvent(Event{
		Sequence:       10,
		Type:           EventAssistantReasoning,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaReasoning,
		AgentName:      "ask-member",
		Content:        "thinking",
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.RecordEvent(Event{
		Sequence:       11,
		Type:           EventAssistantText,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaOutput,
		AgentName:      "ask-member",
		Content:        "about to ask",
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.RecordEvent(Event{
		Sequence:       12,
		Type:           events.EventAskRequested,
		AgentName:      "ask-member",
		Content:        "Choose?",
		Detail:         "ask-1",
		Ask:            &domainevent.AskPayload{ID: "ask-1", Question: "Choose?"},
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	events := messages[0].Events
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3: %#v", len(events), events)
	}
	want := []int64{10, 11, 12}
	for i, sequence := range want {
		if events[i].Sequence != sequence {
			t.Fatalf("event %d sequence = %d, want %d", i, events[i].Sequence, sequence)
		}
	}
}

func TestHistoryRecorderStoresUsageAsUsageEvent(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Sequence:  1,
		Type:      EventAssistantText,
		Role:      message.RoleAssistant,
		DeltaKind: events.DeltaOutput,
		AgentName: "coordinator",
		Content:   "ok",
	})
	recorder.RecordEvent(Event{
		Sequence:         2,
		Type:             EventUsageReported,
		AgentName:        "coordinator",
		PromptTokens:     3,
		CompletionTokens: 4,
		TotalTokens:      7,
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 2 {
		t.Fatalf("event count = %d, want 2", len(messages[0].Events))
	}
	usageEvent := messages[0].Events[1]
	if usageEvent.Type != MsgTypeUsageReported {
		t.Fatalf("usage event type = %q, want %q", usageEvent.Type, MsgTypeUsageReported)
	}
	if usageEvent.Usage == nil || usageEvent.Usage.TotalTokens != 7 {
		t.Fatalf("usage event usage = %#v, want total tokens 7", usageEvent.Usage)
	}
}

func TestHistoryRecorderStoresFriendlyError(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Type:      EventError,
		AgentName: "coordinator",
		Error:     "deepseek does not support image_url type",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 || len(messages[0].Events) != 1 {
		t.Fatalf("messages = %#v, want one error event", messages)
	}
	event := messages[0].Events[0]
	if event.Type != MsgTypeError || event.Error == nil {
		t.Fatalf("event = %#v, want friendly error record", event)
	}
	if event.Error.Code != "model_unsupported_image_input" {
		t.Fatalf("error code = %q, want model_unsupported_image_input", event.Error.Code)
	}
	if event.Content == "" || event.Content == event.Error.TechnicalDetail {
		t.Fatalf("content = %q, technical = %q, want friendly content", event.Content, event.Error.TechnicalDetail)
	}
}

func TestHistoryRecorderRecordsCancellationForActiveMessages(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolCallStarted,
		AgentName:   "coordinator",
		ToolCallRef: "tool_call:call_1",
		ToolCalls: []message.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: message.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventAssistantText,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaReasoning,
		AgentName:      "researcher",
		Content:        "working",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "Researcher",
		MemberOrder:    &toolIndex,
	})

	recorder.RecordCancelled("任务已取消")

	messages := recorder.GetMessages()
	if len(messages) < 2 {
		t.Fatalf("message count = %d, want at least active message and cancellation notice", len(messages))
	}
	for i, msg := range messages[:len(messages)-1] {
		if hasEventType(msg, MsgTypeCancelled) {
			t.Fatalf("message %d events = %#v, want no cancelled marker", i, msg.Events)
		}
	}
	last := messages[len(messages)-1]
	if last.AgentName != "system" || !hasEventType(last, MsgTypeCancelled) {
		t.Fatalf("last message = %#v, want system cancelled notice", last)
	}
}

func TestHistoryRecorderRecordsToolRoleMessageEndAsToolResult(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:      1,
		Type:          EventToolCallStarted,
		AgentName:     "assistant",
		ToolCallID:    "call_1",
		ToolCallRef:   "ref_1",
		ToolName:      "echo",
		ToolArgs:      `{"text":"hello"}`,
		ToolCallIndex: &toolIndex,
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        events.EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("events = %#v, want one tool call", messages[0].Events)
	}
	toolCall := messages[0].Events[0].ToolCall
	if toolCall.Result != "echo: hello" {
		t.Fatalf("tool result = %q, want echo: hello", toolCall.Result)
	}
}

func TestHistoryRecorderUsesPositionToolRefsWhenToolCallIndexMissing(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Sequence:  1,
		Type:      events.EventAssistantCompleted,
		Role:      message.RoleAssistant,
		AgentName: "assistant",
		ToolCalls: []message.ToolCall{
			{
				ID: "call_1",
				Function: message.FunctionCall{
					Name:      "echo",
					Arguments: `{"text":"hello"}`,
				},
			},
		},
		ToolCallRefs: map[int]string{0: "tool_call:call_1"},
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        events.EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "tool_call:call_1",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("events = %#v, want one tool call", messages[0].Events)
	}
	toolCall := messages[0].Events[0].ToolCall
	if toolCall.Ref != "tool_call:call_1" {
		t.Fatalf("tool ref = %q, want tool_call:call_1", toolCall.Ref)
	}
	if toolCall.Result != "echo: hello" {
		t.Fatalf("tool result = %q, want echo: hello", toolCall.Result)
	}
}

func TestHistoryRecorderMergesToolResultByIDWhenRefDiffers(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolCallStarted,
		AgentName:   "assistant",
		ToolCallRef: "ref_from_args",
		ToolCallRefs: map[int]string{
			0: "ref_from_args",
		},
		ToolCalls: []message.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: message.FunctionCall{
				Name:      "echo",
				Arguments: `{"text":"hello"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_from_result",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("events = %#v, want one merged tool call", messages[0].Events)
	}
	toolCall := messages[0].Events[0].ToolCall
	if toolCall.Arguments != `{"text":"hello"}` {
		t.Fatalf("tool args = %q, want original args", toolCall.Arguments)
	}
	if toolCall.Result != "echo: hello" {
		t.Fatalf("tool result = %q, want echo: hello", toolCall.Result)
	}
}

func TestHistoryRecorderDoesNotDuplicateToolEndAndToolRoleMessageEnd(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:      1,
		Type:          EventToolCallStarted,
		AgentName:     "assistant",
		ToolCallID:    "call_1",
		ToolCallRef:   "ref_1",
		ToolName:      "echo",
		ToolArgs:      `{"text":"hello"}`,
		ToolCallIndex: &toolIndex,
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
		ToolResult:  "echo: hello",
	})
	recorder.RecordEvent(Event{
		Sequence:    3,
		Type:        events.EventAssistantCompleted,
		Role:        message.RoleTool,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 {
		t.Fatalf("event count = %d, want 1: %#v", len(messages[0].Events), messages[0].Events)
	}
}

func TestTranscriptRecorderAppendsEventsInWriteOrder(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)

	recorder.RecordEvent(events.UserMessage("run-1", events.TurnID("run-1", 1), "msg-1", message.Message{Role: message.RoleUser, Content: "hello"}))
	recorder.RecordEvent(Event{Type: EventAssistantText, AgentName: "coordinator", Content: "world"})
	recorder.RecordEvent(Event{Type: events.EventAssistantCompleted, AgentName: "coordinator", Content: "world"})

	transcript, err := LoadTranscriptFromFile(filepath.Join(dir, TranscriptFileName))
	if err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	if len(transcript) != 2 {
		t.Fatalf("event count = %d, want 2: %#v", len(transcript), transcript)
	}
	if !strings.HasPrefix(transcript[0].ID, "msg_") || !strings.HasPrefix(transcript[1].ID, "msg_") {
		t.Fatalf("ids = %q,%q want msg_ prefixes", transcript[0].ID, transcript[1].ID)
	}
	if transcript[0].At.IsZero() || transcript[1].At.IsZero() {
		t.Fatalf("transcript timestamps must be set: %#v", transcript)
	}
	if transcript[0].Type != TranscriptUserMessage || transcript[1].Type != TranscriptAssistantMessage {
		t.Fatalf("types = %s,%s", transcript[0].Type, transcript[1].Type)
	}
	if transcript[1].Content != "world" {
		t.Fatalf("assistant content = %#v", transcript[1])
	}
}

func TestTranscriptRecorderAggregatesAssistantDeltas(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)

	recorder.RecordEvent(Event{Type: events.EventAssistantReasoning, AgentName: "coordinator", MessageID: "msg-1", Content: "think "})
	recorder.RecordEvent(Event{Type: events.EventAssistantReasoning, AgentName: "coordinator", MessageID: "msg-1", Content: "more"})
	recorder.RecordEvent(Event{Type: EventAssistantText, AgentName: "coordinator", MessageID: "msg-1", Content: "hello "})
	recorder.RecordEvent(Event{Type: EventAssistantText, AgentName: "coordinator", MessageID: "msg-1", Content: "world"})
	recorder.RecordEvent(Event{Type: events.EventAssistantCompleted, AgentName: "coordinator", MessageID: "msg-1", Content: "hello world", ReasoningContent: "think more"})

	transcript, err := LoadTranscriptFromFile(filepath.Join(dir, TranscriptFileName))
	if err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	if len(transcript) != 1 {
		t.Fatalf("event count = %d, want 1: %#v", len(transcript), transcript)
	}
	if transcript[0].Type != TranscriptAssistantMessage {
		t.Fatalf("type = %s, want assistant_message", transcript[0].Type)
	}
	if transcript[0].Content != "hello world" || transcript[0].Reasoning != "think more" {
		t.Fatalf("transcript = %#v", transcript[0])
	}

	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(dir, TranscriptFileName)); err != nil {
		t.Fatalf("load recorder: %v", err)
	}
	messages := loaded.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if messages[0].GetTextContent() != "hello world" || messages[0].GetReasoningContent() != "think more" {
		t.Fatalf("projected message text=%q reasoning=%q", messages[0].GetTextContent(), messages[0].GetReasoningContent())
	}
}

func TestTranscriptProjectionMarksUnfinishedToolCallRunning(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)

	recorder.RecordEvent(Event{
		Type:       EventToolCallStarted,
		AgentName:  "coordinator",
		ToolCallID: "call_1",
		ToolName:   "file_read",
		ToolArgs:   `{"filepath":"agent-loop.ts"}`,
	})

	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(dir, TranscriptFileName)); err != nil {
		t.Fatalf("load recorder: %v", err)
	}
	messages := loaded.GetMessages()
	if len(messages) != 1 || len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("messages = %#v, want one unfinished tool call", messages)
	}
	tool := messages[0].Events[0].ToolCall
	if tool.Status != "running" {
		t.Fatalf("tool status = %q, want running", tool.Status)
	}
	if tool.Result != "" {
		t.Fatalf("tool result = %q, want empty", tool.Result)
	}
}

func TestTranscriptRecorderStoresReasoningOnlyModelOutputAsAgentStep(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)

	recorder.RecordEvent(Event{
		Type:             events.EventAssistantCompleted,
		AgentName:        "researcher",
		ReasoningContent: "I should search first",
		PromptTokens:     3,
		CompletionTokens: 4,
		TotalTokens:      7,
	})

	transcript, err := LoadTranscriptFromFile(filepath.Join(dir, TranscriptFileName))
	if err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	if len(transcript) != 1 {
		t.Fatalf("event count = %d, want 1: %#v", len(transcript), transcript)
	}
	if transcript[0].Type != TranscriptAgentStep {
		t.Fatalf("type = %s, want agent_step", transcript[0].Type)
	}
	if !strings.HasPrefix(transcript[0].ID, "step_") {
		t.Fatalf("id = %q, want step_ prefix", transcript[0].ID)
	}
	if transcript[0].Content != "" || transcript[0].Reasoning != "I should search first" {
		t.Fatalf("agent step = %#v", transcript[0])
	}
	if transcript[0].Usage == nil || transcript[0].Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want total tokens 7", transcript[0].Usage)
	}
}

func TestTranscriptRecorderExternalizesLongToolResults(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	longResult := strings.Repeat("x", longToolResultChars+10)

	recorder.RecordEvent(Event{Type: EventToolCallStarted, AgentName: "coordinator", ToolCallID: "call_1", ToolName: "fetch", ToolArgs: `{"url":"https://example.com"}`})
	recorder.RecordEvent(Event{Type: EventToolCallCompleted, AgentName: "coordinator", ToolCallID: "call_1", ToolName: "fetch", Content: longResult})

	transcript, err := LoadTranscriptFromFile(filepath.Join(dir, TranscriptFileName))
	if err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	if len(transcript) != 2 {
		t.Fatalf("event count = %d, want 2", len(transcript))
	}
	start := transcript[0]
	if start.CallID != "call_1" || start.Name != "fetch" || start.Args != `{"url":"https://example.com"}` {
		t.Fatalf("tool start = %#v, want flat call fields", start)
	}
	end := transcript[1]
	if end.Result != "" || end.ResultRef == "" || !end.Truncated {
		t.Fatalf("tool record = %#v, want externalized result", end)
	}
	if end.CallID != "call_1" || end.Name != "" || end.Args != "" {
		t.Fatalf("tool end = %#v, want only call id and result fields", end)
	}
	if _, err := os.Stat(filepath.Join(dir, end.ResultRef)); err != nil {
		t.Fatalf("result artifact missing: %v", err)
	}
}

func TestTranscriptRecorderWritesSubagentTranscriptSeparately(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)

	recorder.RecordEvent(Event{Type: EventToolCallStarted, AgentName: "coordinator", ToolCallID: "call_1", ToolName: "ask_fkagent_researcher"})
	recorder.RecordEvent(Event{
		Type:           EventAssistantText,
		AgentName:      "researcher",
		Content:        "member result",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "researcher",
	})
	recorder.RecordEvent(Event{
		Type:           events.EventAssistantCompleted,
		AgentName:      "researcher",
		Content:        "member result",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "researcher",
	})
	recorder.RecordEvent(Event{Type: EventToolCallCompleted, AgentName: "coordinator", ToolCallID: "call_1", ToolName: "ask_fkagent_researcher", Content: "member result"})

	main, err := LoadTranscriptFromFile(filepath.Join(dir, TranscriptFileName))
	if err != nil {
		t.Fatalf("load main transcript: %v", err)
	}
	if len(main) != 2 {
		t.Fatalf("main transcript event count = %d, want parent tool start/end: %#v", len(main), main)
	}
	if main[0].Type != TranscriptToolCallStart || main[1].Type != TranscriptToolCallEnd {
		t.Fatalf("main transcript types = %s,%s", main[0].Type, main[1].Type)
	}
	matches, err := filepath.Glob(filepath.Join(dir, subagentsDirName, "*", TranscriptFileName))
	if err != nil {
		t.Fatalf("glob subagents: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("subagent transcript files = %#v, want one", matches)
	}
	sub, err := LoadTranscriptFromFile(matches[0])
	if err != nil {
		t.Fatalf("load subagent transcript: %v", err)
	}
	if len(sub) != 1 || sub[0].Type != TranscriptAssistantMessage || sub[0].Content != "member result" {
		t.Fatalf("subagent transcript = %#v", sub)
	}
	rawSubLine, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read subagent transcript: %v", err)
	}
	if strings.Contains(string(rawSubLine), "agent_run_id") || strings.Contains(string(rawSubLine), "parent_tool_call_id") {
		t.Fatalf("subagent transcript line should not repeat run metadata: %s", rawSubLine)
	}
	metadata, err := loadSubagentMetadata(filepath.Join(filepath.Dir(matches[0]), "metadata.json"))
	if err != nil {
		t.Fatalf("load subagent metadata: %v", err)
	}
	if metadata.ParentCallID != "call_1" || metadata.Agent != "researcher" || metadata.ToolName != "ask_fkagent_researcher" {
		t.Fatalf("subagent metadata = %#v", metadata)
	}
}

func hasEventType(msg AgentMessage, typ MsgEventType) bool {
	for _, event := range msg.Events {
		if event.Type == typ {
			return true
		}
	}
	return false
}
