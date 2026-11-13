package channel

import (
	"context"
	eventlog "fkteams/internal/adapters/storage/file/history"
	filequeue "fkteams/internal/adapters/storage/file/queue"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/execution"
	domainprocess "fkteams/internal/domain/process"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
)

type approvalProbeRunner struct {
	called bool
	err    error
}

type completionRunnerFunc func(context.Context, domainmessage.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error)

func (f completionRunnerFunc) Run(ctx context.Context, input domainmessage.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return f(ctx, input, opts)
}

func TestBridgeConsumesCompletionThroughApplicationQueue(t *testing.T) {
	dir := t.TempDir()
	queues := appchat.NewQueueService(taskstream.NewManager(), filequeue.NewStore(dir), nil)
	ch := &fakeChannel{name: "completion"}
	manager := NewManager(nil, NewFactoryRegistry())
	manager.channels[ch.name] = ch
	bridge := NewBridgeWithOptions(manager, "team", BridgeOptions{HistoryDir: dir})
	bridge.SetRuntimeDependencies(execution.Dependencies{Queues: queues})
	var inputs []domainmessage.TurnInput
	bridge.runner = completionRunnerFunc(func(ctx context.Context, input domainmessage.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		inputs = append(inputs, input)
		if err := approval.Require(ctx, approval.StoreCommand, "probe", "probe"); err != nil {
			t.Fatalf("channel completion changed approval policy: %v", err)
		}
		answer := "started"
		if len(inputs) == 1 {
			if err := queues.DeliverProcess(domainprocess.Record{ID: "process-channel", Owner: domainprocess.Owner{SessionID: "session"}, State: domainprocess.Succeeded, Stdout: "channel result"}); err != nil {
				return nil, err
			}
		} else {
			answer = "completion received"
		}
		emitter := events.NewEmitter(opts.RunID, events.TurnID(opts.RunID, 1), opts.Sink)
		meta := events.MessageEvent{MessageID: opts.RunID + ":answer", Role: domainmessage.RoleAssistant, Content: answer}
		if err := emitter.Emit(events.AssistantDelta(meta, answer)); err != nil {
			return nil, err
		}
		return &runtimeport.RunResult{}, emitter.Emit(events.AssistantCompleted(meta))
	})
	bridge.processBatch(context.Background(), "session", []queuedMessage{{channelName: ch.name, chatID: "chat", userInput: "start"}})
	stream := queues.Streams().Get("session")
	live, _ := stream.PresentationSnapshot()
	facts, err := bridge.sessions.Get("session").ReplayEvents()
	if err != nil {
		t.Fatal(err)
	}
	replay := presentation.New("session")
	for _, fact := range facts {
		replay.Apply(fact)
	}
	if len(live.Items) != 2 || !reflect.DeepEqual(live, replay.Snapshot()) || stream.ClientStatus() != "completed" {
		t.Fatalf("channel web stream differs from transcript: live=%+v replay=%+v status=%s", live, replay.Snapshot(), stream.ClientStatus())
	}
	if _, err := eventlog.LoadMetadata(filepath.Join(dir, "session")); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || !strings.Contains(inputs[1].Message.Content, "channel result") {
		t.Fatalf("channel completion: %+v", inputs)
	}
	found := false
	for _, sent := range ch.sent {
		if strings.Contains(sent.msg.Content, "completion received") {
			found = true
		}
	}
	if !found {
		t.Fatalf("completion reply was not delivered: %+v", ch.sent)
	}
}

func (r *approvalProbeRunner) Run(ctx context.Context, _ domainmessage.TurnInput, _ runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	r.called = true
	r.err = approval.Require(ctx, approval.StoreCommand, "probe", "probe approval")
	return &runtimeport.RunResult{}, r.err
}

func TestBridgeUsesAutomaticApprovalPolicy(t *testing.T) {
	channel := &fakeChannel{name: "approval_policy"}
	manager := NewManager(nil, NewFactoryRegistry())
	manager.channels[channel.name] = channel
	bridge := NewBridgeWithOptions(manager, "team", BridgeOptions{HistoryDir: t.TempDir()})
	runner := &approvalProbeRunner{}
	bridge.runner = runner
	bridge.processBatch(context.Background(), "session", []queuedMessage{{channelName: channel.name, chatID: "chat", userInput: "test"}})
	if !runner.called || runner.err != nil {
		t.Fatalf("called=%v, approval=%v", runner.called, runner.err)
	}
}

func TestEnqueueSessionMessagePublishesAndCountsPending(t *testing.T) {
	q := &sessionQueue{ch: make(chan queuedMessage, 1)}
	pos, ok := enqueueSessionMessage(q, queuedMessage{userInput: "hello"})
	if !ok || pos != 1 {
		t.Fatalf("enqueue result = (%d, %v), want (1, true)", pos, ok)
	}
	if pending := q.pending.Load(); pending != 1 {
		t.Fatalf("pending after enqueue = %d, want 1", pending)
	}
	if received := <-q.ch; received.userInput != "hello" {
		t.Fatalf("received message = %#v", received)
	}
}

func TestEnqueueSessionMessageRollsBackPendingWhenFull(t *testing.T) {
	q := &sessionQueue{ch: make(chan queuedMessage, 1)}
	if _, ok := enqueueSessionMessage(q, queuedMessage{userInput: "first"}); !ok {
		t.Fatal("first enqueue should succeed")
	}
	if pos, ok := enqueueSessionMessage(q, queuedMessage{userInput: "second"}); ok || pos != 0 {
		t.Fatalf("full enqueue result = (%d, %v), want (0, false)", pos, ok)
	}
	if pending := q.pending.Load(); pending != 1 {
		t.Fatalf("pending after rejected enqueue = %d, want 1", pending)
	}
}

func TestBridgeStopDrainsQueuedMessagesAndRejectsNewInput(t *testing.T) {
	bridge := NewBridgeWithOptions(NewManager(nil, NewFactoryRegistry()), "team", BridgeOptions{HistoryDir: t.TempDir()})
	bridge.Start(context.Background())
	var released atomic.Int32
	queue := &sessionQueue{ch: make(chan queuedMessage, 2)}
	for i := 0; i < 2; i++ {
		if _, ok := enqueueSessionMessage(queue, queuedMessage{releaseLease: func() { released.Add(1) }}); !ok {
			t.Fatal("enqueue should succeed")
		}
	}
	bridge.queueMu.Lock()
	bridge.queues["session-1"] = queue
	bridge.queueMu.Unlock()

	if err := bridge.Stop(context.Background()); err != nil {
		t.Fatalf("Stop(): %v", err)
	}
	if got := released.Load(); got != 2 {
		t.Fatalf("released leases = %d, want 2", got)
	}
	if pending := queue.pending.Load(); pending != 0 {
		t.Fatalf("pending = %d, want 0", pending)
	}
	bridge.HandleMessage(context.Background(), "chat", "sender", Message{Content: "ignored"}, false)
	bridge.queueMu.Lock()
	defer bridge.queueMu.Unlock()
	if len(bridge.queues) != 0 {
		t.Fatalf("queues after stopped input = %d, want 0", len(bridge.queues))
	}
}

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
