package channel

import (
	"context"
	"strings"
	"testing"

	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
)

func TestBuildUserInputCombinesContentAndAttachments(t *testing.T) {
	msg := Message{
		Content: "请看附件",
		Attachments: []Attachment{
			{Type: MsgImage, URL: "https://example.com/a.png", FileName: "a.png"},
			{Type: MsgFile, URL: "https://example.com/report.pdf"},
		},
	}

	got := buildUserInput(msg)
	want := "请看附件\n[图片 (a.png): https://example.com/a.png]\n[文件: https://example.com/report.pdf]"
	if got != want {
		t.Fatalf("buildUserInput = %q, want %q", got, want)
	}

	if got := buildUserInput(Message{}); got != "" {
		t.Fatalf("empty buildUserInput = %q, want empty", got)
	}
}

func TestSplitMessagePrefersNewlineAndHardSplitsLongText(t *testing.T) {
	text := "第一行数据\n第二行内容很长\n第三行"
	chunks := splitMessage(text, 10)
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 10 {
			t.Fatalf("chunk %q length = %d, want <= 10", chunk, len([]rune(chunk)))
		}
	}
	if strings.Join(chunks, "") != text {
		t.Fatalf("joined chunks = %q, want original %q", strings.Join(chunks, ""), text)
	}
	if chunks[0] != "第一行数据\n" {
		t.Fatalf("first chunk = %q, want newline split", chunks[0])
	}

	hardText := "abcdefghijklmnop"
	hardChunks := splitMessage(hardText, 5)
	if got, want := strings.Join(hardChunks, ""), hardText; got != want {
		t.Fatalf("joined hard chunks = %q, want %q", got, want)
	}
	for _, chunk := range hardChunks {
		if len([]rune(chunk)) > 5 {
			t.Fatalf("hard chunk %q length = %d, want <= 5", chunk, len([]rune(chunk)))
		}
	}
}

func TestWithChannelNameStoresName(t *testing.T) {
	ctx := WithChannelName(context.Background(), "wechat")
	if got, ok := ctx.Value(channelNameKey{}).(string); !ok || got != "wechat" {
		t.Fatalf("channel name = %q, ok=%v, want wechat", got, ok)
	}
}

func TestReplyCollectorFlushesTextOnAgentTransfer(t *testing.T) {
	channel := &fakeChannel{name: "reply_text"}
	manager := NewManager(nil, NewFactoryRegistry())
	manager.channels[channel.name] = channel
	rc := newReplyCollector(manager, channel.name, "chat-1")

	if err := rc.handleEvent(events.Event{
		Type:      events.EventAssistantText,
		AgentName: "assistant",
		DeltaKind: events.DeltaReasoning,
		Content:   "ignored",
	}); err != nil {
		t.Fatalf("handle reasoning delta returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:      events.EventAssistantText,
		AgentName: "assistant",
		DeltaKind: events.DeltaOutput,
		Content:   "hello",
	}); err != nil {
		t.Fatalf("handle first delta returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:      events.EventAssistantText,
		AgentName: "assistant",
		DeltaKind: events.DeltaOutput,
		Content:   " world",
	}); err != nil {
		t.Fatalf("handle second delta returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:    events.EventSystemNotice,
		Notice:  &events.NoticePayload{Code: "transfer"},
		Content: "transfer",
	}); err != nil {
		t.Fatalf("handle transfer returned error: %v", err)
	}

	if len(channel.sent) != 1 {
		t.Fatalf("sent count = %d, want 1: %#v", len(channel.sent), channel.sent)
	}
	if got := channel.sent[0].msg.Content; got != "hello world" {
		t.Fatalf("sent content = %q, want hello world", got)
	}
	if !rc.replied {
		t.Fatal("reply collector should mark replied after flush")
	}
}

func TestReplyCollectorSendsToolSummaryFromEnd(t *testing.T) {
	channel := &fakeChannel{name: "reply_tool_end"}
	manager := NewManager(nil, NewFactoryRegistry())
	manager.channels[channel.name] = channel
	rc := newReplyCollector(manager, channel.name, "chat-1")

	toolCall := &domainmessage.ToolCall{
		ID: "call-1",
		Function: domainmessage.FunctionCall{
			Name:      "search",
			Arguments: `{"q":"天气"}`,
		},
	}
	if err := rc.handleEvent(events.Event{
		Type:     events.EventToolCallStarted,
		ToolCall: toolCall,
	}); err != nil {
		t.Fatalf("handle tool start returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:       events.EventToolCallCompleted,
		ToolCallID: "call-1",
		Content:    "晴天",
	}); err != nil {
		t.Fatalf("handle tool end returned error: %v", err)
	}

	if len(channel.sent) != 1 {
		t.Fatalf("sent count = %d, want 1: %#v", len(channel.sent), channel.sent)
	}
	want := "[search] {\"q\":\"天气\"}\n-> 晴天"
	if got := channel.sent[0].msg.Content; got != want {
		t.Fatalf("tool summary = %q, want %q", got, want)
	}
}

func TestReplyCollectorFlushesToolUpdateChunksBeforeText(t *testing.T) {
	channel := &fakeChannel{name: "reply_tool_update"}
	manager := NewManager(nil, NewFactoryRegistry())
	manager.channels[channel.name] = channel
	rc := newReplyCollector(manager, channel.name, "chat-1")

	if err := rc.handleEvent(events.Event{
		Type: events.EventToolCallStarted,
		ToolCalls: []domainmessage.ToolCall{
			{
				ID: "call-1",
				Function: domainmessage.FunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				},
			},
		},
	}); err != nil {
		t.Fatalf("handle tool start returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:       events.EventToolCallResult,
		ToolCallID: "call-1",
		Content:    "part-1 ",
	}); err != nil {
		t.Fatalf("handle first tool update returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:       events.EventToolCallResult,
		ToolCallID: "call-1",
		Content:    "part-2",
	}); err != nil {
		t.Fatalf("handle second tool update returned error: %v", err)
	}
	if err := rc.handleEvent(events.Event{
		Type:      events.EventAssistantText,
		DeltaKind: events.DeltaOutput,
		Content:   "done",
	}); err != nil {
		t.Fatalf("handle message delta returned error: %v", err)
	}
	rc.flush()

	if len(channel.sent) != 2 {
		t.Fatalf("sent count = %d, want 2: %#v", len(channel.sent), channel.sent)
	}
	wantTool := "[read_file] {\"path\":\"README.md\"}\n-> part-1 part-2"
	if got := channel.sent[0].msg.Content; got != wantTool {
		t.Fatalf("tool update summary = %q, want %q", got, wantTool)
	}
	if got := channel.sent[1].msg.Content; got != "done" {
		t.Fatalf("text reply = %q, want done", got)
	}
}
