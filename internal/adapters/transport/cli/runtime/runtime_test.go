package runtime

import (
	"context"
	"fkteams/internal/adapters/transport/cli/tui"
	"fkteams/internal/app/tools/ask"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRuntimeCtrlCCancelsRunningTask(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	cancelled := false
	state.SetCancelFunc(func() { cancelled = true })

	session := NewSession(ModeTeam, nil, nil)
	session.queryState = state
	rt := &Runtime{
		ctx:         context.Background(),
		session:     session,
		exitSignals: make(chan os.Signal, 1),
	}
	model := newRuntimeModel(rt)
	model.running = true

	updated, cmd := model.Update(ctrlCKeyMsg())
	model = updated.(runtimeModel)
	if !model.cancelling {
		t.Fatal("Ctrl+C while running should mark the task as cancelling")
	}
	if cmd == nil {
		t.Fatal("Ctrl+C while running should request cancellation")
	}
	msg := cmd()
	if _, ok := msg.(runtimeCancellingMsg); !ok {
		t.Fatalf("unexpected cancellation message: %T", msg)
	}
	if !cancelled {
		t.Fatal("query cancel func was not called")
	}
}

func TestRuntimeCtrlCConfirmsExitWhenIdle(t *testing.T) {
	rt := &Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	}
	model := newRuntimeModel(rt)

	updated, cmd := model.Update(ctrlCKeyMsg())
	model = updated.(runtimeModel)
	if !model.isExitConfirming() {
		t.Fatal("first idle Ctrl+C should enter exit confirmation")
	}
	if cmd == nil {
		t.Fatal("first idle Ctrl+C should start countdown")
	}
}

func TestRuntimeEnterWhileRunningQueuesSteering(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	session := NewSession(ModeTeam, nil, nil)
	session.queryState = state
	executor := NewQueryExecutor(nil, state)
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     session,
		executor:    executor,
		exitSignals: make(chan os.Signal, 1),
	})
	model.running = true
	model.input.SetValue("change direction")
	model.input.CursorEnd()
	blockCount := len(model.blocks)

	updated, cmd := model.Update(keyMsg("enter", "", 0))
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("steering submit should not start a new query command")
	}
	if got := model.input.Value(); got != "" {
		t.Fatalf("steering submit should clear input, got %q", got)
	}
	if len(model.blocks) != blockCount {
		t.Fatalf("steering submit should not append transcript blocks before execution, got %#v", model.blocks[blockCount:])
	}
	if model.status != "已排队转向，等待下一次模型调用..." {
		t.Fatalf("steering submit should update status, got %q", model.status)
	}
	messages := executor.takeSteeringMessages(1)
	if len(messages) != 1 || messages[0].Content != "change direction" {
		t.Fatalf("expected queued steering message, got %#v", messages)
	}
}

func TestRuntimeApprovalSubmitDoesNotQueueSteering(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	session := NewSession(ModeTeam, nil, nil)
	session.queryState = state
	executor := NewQueryExecutor(nil, state)
	broker := newRuntimeApprovalBroker(func(tea.Msg) {})
	responseCh := make(chan int, 1)
	broker.pending["approval-1"] = responseCh
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     session,
		executor:    executor,
		approval:    broker,
		exitSignals: make(chan os.Signal, 1),
	})
	model.running = true
	model.approval = &runtimeApprovalState{ID: "approval-1", Info: "file_list", Selected: 0}
	model.input.SetValue("should not steer")
	model.input.CursorEnd()

	updated, cmd := model.Update(keyMsg("enter", "", 0))
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("approval submit should not start a command")
	}
	if messages := executor.takeSteeringMessages(1); len(messages) != 0 {
		t.Fatalf("approval submit should not queue steering, got %#v", messages)
	}
	select {
	case decision := <-responseCh:
		if decision != approval.ApproveOnce {
			t.Fatalf("approval decision = %d, want %d", decision, approval.ApproveOnce)
		}
	default:
		t.Fatal("expected approval decision to be submitted")
	}
}

func TestRuntimeApprovalKeysDoNotNavigateHistory(t *testing.T) {
	session := NewSession(ModeTeam, nil, nil)
	session.InputHistory = []string{"old", "new"}
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     session,
		exitSignals: make(chan os.Signal, 1),
	})
	model.running = true
	model.approval = &runtimeApprovalState{ID: "approval-1", Info: "file_list", Selected: 0}

	updated, _ := model.Update(keyMsg("up", "", 0))
	model = updated.(runtimeModel)
	if got := model.input.Value(); got != "" {
		t.Fatalf("approval up should not load history, got %q", got)
	}
	if got := model.approval.Selected; got != len(runtimeApprovalOptions())-1 {
		t.Fatalf("approval up should wrap to last option, got %d", got)
	}
	updated, _ = model.Update(keyMsg("down", "", 0))
	model = updated.(runtimeModel)
	if got := model.approval.Selected; got != 0 {
		t.Fatalf("approval down should move selection, got %d", got)
	}
}

func TestRuntimeApprovalCtrlCRejectsAndCancels(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	cancelled := false
	state.SetCancelFunc(func() { cancelled = true })
	session := NewSession(ModeTeam, nil, nil)
	session.queryState = state
	broker := newRuntimeApprovalBroker(func(tea.Msg) {})
	responseCh := make(chan int, 1)
	broker.pending["approval-1"] = responseCh
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     session,
		approval:    broker,
		exitSignals: make(chan os.Signal, 1),
	})
	model.running = true
	model.approval = &runtimeApprovalState{ID: "approval-1", Info: "file_list", Selected: 0}

	updated, cmd := model.Update(ctrlCKeyMsg())
	model = updated.(runtimeModel)
	if cmd == nil {
		t.Fatal("approval Ctrl+C should request cancellation")
	}
	msg := cmd()
	if _, ok := msg.(runtimeCancellingMsg); !ok {
		t.Fatalf("unexpected cancellation message: %T", msg)
	}
	if !cancelled {
		t.Fatal("approval Ctrl+C should cancel the running query")
	}
	select {
	case decision := <-responseCh:
		if decision != approval.Reject {
			t.Fatalf("approval decision = %d, want %d", decision, approval.Reject)
		}
	default:
		t.Fatal("expected approval Ctrl+C to submit rejection")
	}
	if model.approval != nil {
		t.Fatal("approval panel should be cleared after Ctrl+C")
	}
}

func TestRuntimeMemberAskSubmitDoesNotQueueSteering(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	session := NewSession(ModeTeam, nil, nil)
	session.queryState = state
	executor := NewQueryExecutor(nil, state)
	broker := newRuntimeAskBroker(func(tea.Msg) {})
	responseCh := make(chan *ask.AskResponse, 1)
	broker.pending["ask-1"] = responseCh
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     session,
		executor:    executor,
		askBroker:   broker,
		exitSignals: make(chan os.Signal, 1),
	})
	model.running = true
	model.members["member-1"] = &runtimeMemberState{
		Key:    "member-1",
		Name:   "tester",
		Status: "waiting",
		PendingAsks: []runtimeAskState{{
			ID:        "ask-1",
			MemberKey: "member-1",
			Question:  "Choose one",
			Options:   []string{"A", "B"},
		}},
	}
	model.memberView = "member-1"
	model.input.SetValue("2")
	model.input.CursorEnd()

	updated, cmd := model.Update(keyMsg("enter", "", 0))
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("member ask submit should not start a command")
	}
	if got := model.input.Value(); got != "" {
		t.Fatalf("member ask submit should clear input, got %q", got)
	}
	if messages := executor.takeSteeringMessages(1); len(messages) != 0 {
		t.Fatalf("member ask submit should not queue steering, got %#v", messages)
	}
	select {
	case resp := <-responseCh:
		if resp.AskID != "ask-1" || len(resp.Selected) != 1 || resp.Selected[0] != "B" {
			t.Fatalf("unexpected ask response: %#v", resp)
		}
	default:
		t.Fatal("expected ask response to be submitted")
	}
}

func TestRuntimeAskBrokerRoutesResponsesByAskID(t *testing.T) {
	broker := newRuntimeAskBroker(func(tea.Msg) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstDone := make(chan *ask.AskResponse, 1)
	secondDone := make(chan *ask.AskResponse, 1)
	go func() {
		resp, _ := broker.Handle(ctx, ask.RuntimeRequest{
			ID:       "ask-1",
			Info:     &ask.AskInfo{Question: "First?"},
			Metadata: runtimeport.InterruptMetadata{MemberCallID: "member-1"},
		})
		firstDone <- resp
	}()
	go func() {
		resp, _ := broker.Handle(ctx, ask.RuntimeRequest{
			ID:       "ask-2",
			Info:     &ask.AskInfo{Question: "Second?"},
			Metadata: runtimeport.InterruptMetadata{MemberCallID: "member-2"},
		})
		secondDone <- resp
	}()

	deadline := time.Now().Add(time.Second)
	for {
		broker.mu.Lock()
		pending := len(broker.pending)
		broker.mu.Unlock()
		if pending == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected both asks to become pending")
		}
		time.Sleep(time.Millisecond)
	}
	if !broker.Submit("ask-2", &ask.AskResponse{AskID: "ask-2", Selected: []string{"B"}}) {
		t.Fatal("expected ask-2 submit to succeed")
	}

	select {
	case resp := <-secondDone:
		if resp.AskID != "ask-2" || len(resp.Selected) != 1 || resp.Selected[0] != "B" {
			t.Fatalf("unexpected second response: %#v", resp)
		}
	case <-time.After(time.Second):
		t.Fatal("ask-2 should resume")
	}
	select {
	case resp := <-firstDone:
		t.Fatalf("ask-1 should not receive ask-2 response: %#v", resp)
	default:
	}
}

func TestRuntimeMemberWaitingStatusSurvivesMemberEvents(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.applyAskPending(runtimeAskState{
		ID:        "ask-1",
		MemberKey: "member-1",
		Question:  "Choose one",
	})
	model.applyEvent(events.Event{
		Type:         events.EventAssistantText,
		DeltaKind:    events.DeltaReasoning,
		Content:      "thinking",
		MemberCallID: "member-1",
		MemberName:   "tester",
	})

	member := model.members["member-1"]
	if member == nil {
		t.Fatal("expected member to exist")
	}
	if member.Status != "waiting" {
		t.Fatalf("pending ask member should stay waiting, got %q", member.Status)
	}
}

func TestRuntimeSteeringExecutionNoticeAppendsBlock(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	blockCount := len(model.blocks)

	model.applyEvent(events.Event{
		Type:    events.EventType(events.NotifyProcessingStart),
		Detail:  "steering",
		Content: "change direction",
	})

	if len(model.blocks) != blockCount+1 {
		t.Fatalf("steering execution should append one block, got %#v", model.blocks[blockCount:])
	}
	block := model.blocks[blockCount]
	if block.Kind != runtimeBlockSystem || block.Title != "转向消息" || block.Content != "change direction" {
		t.Fatalf("unexpected steering execution block: %#v", block)
	}
}

func TestQueryExecutorDrainsSteeringAsMergedMessage(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	executor := NewQueryExecutor(nil, state)
	executor.QueueSteering("first")
	executor.QueueSteering("second")

	message, ok := executor.drainSteeringMessage()
	if !ok {
		t.Fatal("expected merged steering message")
	}
	if got := message.Content; !strings.Contains(got, "1. first") || !strings.Contains(got, "2. second") {
		t.Fatalf("expected merged steering content, got %q", got)
	}
	if messages := executor.takeSteeringMessages(1); len(messages) != 0 {
		t.Fatalf("expected steering queue to be drained, got %#v", messages)
	}
}

func TestRuntimeEscWhileRunningRestoresQueuedSteeringToInput(t *testing.T) {
	state := NewQueryState()
	state.StartQuery()
	cancelled := false
	state.SetCancelFunc(func() { cancelled = true })
	session := NewSession(ModeTeam, nil, nil)
	session.queryState = state
	executor := NewQueryExecutor(nil, state)
	if !executor.QueueSteering("first") || !executor.QueueSteering("second") {
		t.Fatal("expected steering queue setup to succeed")
	}
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     session,
		executor:    executor,
		exitSignals: make(chan os.Signal, 1),
	})
	model.running = true
	model.input.SetValue("draft")
	model.input.CursorEnd()

	updated, cmd := model.Update(keyMsg("esc", "", 0))
	model = updated.(runtimeModel)
	if cmd == nil {
		t.Fatal("esc while running should request cancellation")
	}
	_ = cmd()
	if !cancelled {
		t.Fatal("esc should cancel the running query")
	}
	got := model.expandInput()
	if !strings.Contains(got, "1. first") || !strings.Contains(got, "2. second") || !strings.Contains(got, "draft") {
		t.Fatalf("expected queued steering and draft to return to input, got %q", got)
	}
	if messages := executor.takeSteeringMessages(1); len(messages) != 0 {
		t.Fatalf("expected steering queue to be drained, got %#v", messages)
	}
}

func TestRuntimeHistoryNavigation(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, []string{"one", "two"}, nil),
		exitSignals: make(chan os.Signal, 1),
	})

	updated, _ := model.Update(keyMsg("up", "", 0))
	model = updated.(runtimeModel)
	if got := model.input.Value(); got != "two" {
		t.Fatalf("up should show latest history, got %q", got)
	}
	updated, _ = model.Update(keyMsg("up", "", 0))
	model = updated.(runtimeModel)
	if got := model.input.Value(); got != "one" {
		t.Fatalf("second up should show older history, got %q", got)
	}
	updated, _ = model.Update(keyMsg("down", "", 0))
	model = updated.(runtimeModel)
	if got := model.input.Value(); got != "two" {
		t.Fatalf("down should move forward in history, got %q", got)
	}
}

func TestRuntimeSlashOpensCommandPicker(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})

	updated, _ := model.Update(keyMsg("", "/", '/'))
	model = updated.(runtimeModel)
	if model.picker == nil || model.picker.kind != runtimePickerCommand {
		t.Fatalf("/ should open command picker, got %#v", model.picker)
	}
}

func TestRuntimeCommandPickerFillsInput(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.picker = newRuntimePicker(runtimePickerCommand, "选择命令", []runtimePickerItem{
		{Label: "help - 帮助信息", Value: "help"},
	}, 10)
	blockCount := len(model.blocks)

	updated, cmd := model.acceptPicker()
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("command picker should fill the input instead of executing the command")
	}
	if got := model.input.Value(); got != "/help" {
		t.Fatalf("command picker should fill selected command, got %q", got)
	}
	if len(model.blocks) != blockCount {
		t.Fatalf("command picker should not append transcript blocks before submission")
	}
}

func TestRuntimeAgentPickerFillsInput(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.picker = newRuntimePicker(runtimePickerAgent, "选择智能体", []runtimePickerItem{
		{Label: "coder - 编码助手", Value: "coder"},
	}, 10)
	blockCount := len(model.blocks)

	updated, cmd := model.acceptPicker()
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("agent picker should fill the input instead of switching immediately")
	}
	if got := model.input.Value(); got != "@coder " {
		t.Fatalf("agent picker should fill selected agent mention, got %q", got)
	}
	if model.runtime.session.currentAgent != "" {
		t.Fatalf("agent picker should not switch immediately, got current agent %q", model.runtime.session.currentAgent)
	}
	if len(model.blocks) != blockCount {
		t.Fatalf("agent picker should not append transcript blocks before submission")
	}
}

func TestRuntimeNativeCommandsOpenPickers(t *testing.T) {
	tests := []struct {
		command string
		kind    runtimePickerKind
	}{
		{command: "/clear_chat_history", kind: runtimePickerConfirm},
		{command: "/clear_memory", kind: runtimePickerConfirm},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			model := newRuntimeModel(&Runtime{
				ctx:         context.Background(),
				session:     NewSession(ModeTeam, nil, nil),
				exitSignals: make(chan os.Signal, 1),
			})

			updated, _ := model.handleSubmit(tt.command)
			model = updated.(runtimeModel)
			if model.picker == nil || model.picker.kind != tt.kind {
				t.Fatalf("%s should open %s picker, got %#v", tt.command, tt.kind, model.picker)
			}
		})
	}
}

func TestRuntimeCommandIsRecordedAsUserInput(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})

	updated, cmd := model.handleSubmit("/help")
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("/help should be handled by runtime command dispatch")
	}
	if len(model.blocks) < 2 {
		t.Fatalf("/help should append user and result blocks, got %#v", model.blocks)
	}
	userBlock := model.blocks[len(model.blocks)-2]
	if userBlock.Kind != runtimeBlockUser || userBlock.Content != "/help" {
		t.Fatalf("/help should be recorded as user input, got %#v", userBlock)
	}
}

func TestRuntimeUnknownSlashCommandDoesNotSubmitQuery(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})

	updated, cmd := model.handleSubmit("/unknown_command")
	model = updated.(runtimeModel)
	if cmd != nil {
		t.Fatal("unknown slash command should not submit a query")
	}
	userBlock := model.blocks[len(model.blocks)-2]
	if userBlock.Kind != runtimeBlockUser || userBlock.Content != "/unknown_command" {
		t.Fatalf("unknown slash command should be recorded as user input, got %#v", userBlock)
	}
	last := model.blocks[len(model.blocks)-1]
	if last.Kind != runtimeBlockError || !strings.Contains(last.Content, "unknown_command") {
		t.Fatalf("unknown slash command should append an error block, got %#v", last)
	}
}

func TestRuntimeMouseWheelScrollsTranscript(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.width = 80
	model.height = 10
	for i := range 20 {
		model.appendBlock(runtimeBlockSystem, "行", "content "+string(rune('A'+i)))
	}

	bottomView := model.View().Content
	if strings.Contains(bottomView, "content A") {
		t.Fatalf("initial view should follow the bottom, view: %q", bottomView)
	}

	updated, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	model = updated.(runtimeModel)
	if model.scrollOffset == 0 {
		t.Fatal("mouse wheel up should increase transcript scroll offset")
	}
}

func TestRuntimeMouseSelectionCopiesVisibleText(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.width = 80
	model.height = 10
	model.blocks = []runtimeBlock{{Kind: runtimeBlockTool, Content: "abcdef"}}

	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{X: 2, Y: 0, Button: tea.MouseLeft}))
	model = updated.(runtimeModel)
	updated, _ = model.Update(tea.MouseMotionMsg(tea.Mouse{X: 5, Y: 0, Button: tea.MouseLeft}))
	model = updated.(runtimeModel)
	if !model.selection.Active {
		t.Fatal("mouse drag should start an active selection")
	}

	updated, _ = model.Update(tea.MouseReleaseMsg(tea.Mouse{X: 5, Y: 0, Button: tea.MouseLeft}))
	model = updated.(runtimeModel)
	if model.selection.Active {
		t.Fatal("mouse release should finish the selection")
	}
	if model.selection.Copied != "bcd" {
		t.Fatalf("mouse release should copy selected visible text, got %q", model.selection.Copied)
	}

	model.copiedUntil = time.Now().Add(-time.Second)
	updated, _ = model.Update(runtimeSelectionCopiedTickMsg(time.Now()))
	model = updated.(runtimeModel)
	if strings.Contains(model.View().Content, "已复制") {
		t.Fatal("copied notice should disappear after the notice window")
	}
}

func TestRuntimeCopiedNoticeCountsLinesAndCharacters(t *testing.T) {
	got := tui.CopiedNotice("你好\nworld")
	want := "已复制 2 行 · 7 字符"
	if got != want {
		t.Fatalf("CopiedNotice() = %q, want %q", got, want)
	}
}

func TestRuntimeReasoningChunksAreMerged(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})

	model.applyEvent(events.Event{Type: events.EventAssistantReasoning, DeltaKind: events.DeltaReasoning, Content: "用户"})
	model.applyEvent(events.Event{Type: events.EventAssistantReasoning, DeltaKind: events.DeltaReasoning, Content: "问好"})

	var reasoningBlocks []runtimeBlock
	for _, block := range model.blocks {
		if block.Kind == runtimeBlockReasoning {
			reasoningBlocks = append(reasoningBlocks, block)
		}
	}
	if len(reasoningBlocks) != 1 {
		t.Fatalf("reasoning chunks should be merged into one block, got %d blocks", len(reasoningBlocks))
	}
	if reasoningBlocks[0].Content != "用户问好" {
		t.Fatalf("unexpected merged reasoning content: %q", reasoningBlocks[0].Content)
	}
}

func TestRuntimePasteMsgInsertsMultilinePlaceholder(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})

	updated, _ := model.Update(tea.PasteMsg{Content: "第一行\n第二行\n第三行"})
	model = updated.(runtimeModel)
	if !strings.Contains(model.input.Value(), "[粘贴3行内容]") {
		t.Fatalf("multiline paste should insert placeholder, got %q", model.input.Value())
	}

	updated, _ = model.Update(keyMsg("enter", "", 0))
	model = updated.(runtimeModel)
	if len(model.blocks) == 0 {
		t.Fatal("enter should append a user block")
	}
	got := model.blocks[len(model.blocks)-1].Content
	if got != "第一行\n第二行\n第三行" {
		t.Fatalf("submitted input should expand paste content, got %q", got)
	}
}

func TestRuntimeShiftEnterInsertsLineBreak(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.input.SetValue("第一行")
	model.input.CursorEnd()

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}))
	model = updated.(runtimeModel)
	model.input.SetValue(model.input.Value() + "第二行")
	inputView := model.renderInputValue()
	if strings.Contains(inputView, tui.InlineLineBreakTag) {
		t.Fatalf("shift+enter should render as a real line break, got %q", inputView)
	}
	if !strings.Contains(inputView, "\n") {
		t.Fatalf("shift+enter should visibly break the input line, got %q", inputView)
	}

	updated, _ = model.Update(keyMsg("enter", "", 0))
	model = updated.(runtimeModel)
	got := model.blocks[len(model.blocks)-1].Content
	if got != "第一行\n第二行" {
		t.Fatalf("shift+enter should submit as a real newline, got %q", got)
	}
}

func TestRuntimeParallelSameAgentMembersDoNotMix(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	firstIndex := 0
	secondIndex := 1

	model.applyEvent(events.Event{
		Type:      events.EventToolCallStarted,
		AgentName: "coordinator",
		ToolCalls: []domainmessage.ToolCall{
			{
				ID:    "call_first",
				Index: &firstIndex,
				Function: domainmessage.FunctionCall{
					Name:      "ask_fkagent_researcher",
					Arguments: `{"task":"first task"}`,
				},
			},
			{
				ID:    "call_second",
				Index: &secondIndex,
				Function: domainmessage.FunctionCall{
					Name:      "ask_fkagent_researcher",
					Arguments: `{"task":"second task"}`,
				},
			},
		},
	})
	model.applyEvent(events.Event{
		Type:         events.EventAssistantText,
		DeltaKind:    events.DeltaOutput,
		AgentName:    "researcher",
		Content:      "second output",
		MemberCallID: "call_second",
		MemberName:   "researcher",
	})
	model.applyEvent(events.Event{
		Type:         events.EventAssistantText,
		DeltaKind:    events.DeltaOutput,
		AgentName:    "researcher",
		Content:      "first output",
		MemberCallID: "call_first",
		MemberName:   "researcher",
	})

	first := model.members["call_first"]
	second := model.members["call_second"]
	if first == nil || second == nil {
		t.Fatalf("expected separate members, got keys %#v", model.members)
	}
	if got := first.Blocks[0].Content; got != "first output" {
		t.Fatalf("first member content = %q", got)
	}
	if got := second.Blocks[0].Content; got != "second output" {
		t.Fatalf("second member content = %q", got)
	}
	if got := model.memberTools["member:0"]; got != "" {
		t.Fatalf("unstable member order alias should not be registered, got %q", got)
	}
	if got := model.memberTools["member:1"]; got != "" {
		t.Fatalf("unstable member order alias should not be registered, got %q", got)
	}
}

func TestRuntimeAgentMemberStartsAfterCompleteToolCall(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	callIndex := 0

	model.applyEvent(events.Event{
		Type:          events.EventAssistantText,
		DeltaKind:     events.DeltaToolArgs,
		ToolName:      "ask_fkagent_researcher",
		ToolCallID:    "call_full",
		ToolCallIndex: &callIndex,
		Content:       "{",
	})
	model.applyEvent(events.Event{
		Type:          events.EventAssistantText,
		DeltaKind:     events.DeltaToolArgs,
		ToolName:      "ask_fkagent_researcher",
		ToolCallID:    "call_full",
		ToolCallIndex: &callIndex,
		Content:       `{"request": "`,
	})
	if len(model.members) != 0 {
		t.Fatalf("partial agent tool args should not create members, got %#v", model.members)
	}

	model.applyEvent(events.Event{
		Type:      events.EventToolCallStarted,
		AgentName: "coordinator",
		ToolCalls: []domainmessage.ToolCall{{
			ID:    "call_full",
			Index: &callIndex,
			Function: domainmessage.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"request":"完整任务目标"}`,
			},
		}},
	})

	member := model.members["call_full"]
	if member == nil {
		t.Fatalf("complete agent tool call should create member, got %#v", model.members)
	}
	if member.Task != "完整任务目标" {
		t.Fatalf("member task = %q, want complete request", member.Task)
	}
}

func TestRuntimeUnnamedAgentArgsDeltaDoesNotRenderToolBlock(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	blockCount := len(model.blocks)

	model.applyEvent(events.Event{
		Type:        events.EventAssistantText,
		DeltaKind:   events.DeltaToolArgs,
		ToolCallRef: "tool|stream|seq:1|coordinator|idx:0",
		Content:     `{"request":"partial`,
	})

	if len(model.blocks) != blockCount {
		t.Fatalf("unnamed agent args delta should not render as ordinary tool, got %#v", model.blocks[blockCount:])
	}
}

func ctrlCKeyMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
}

func keyMsg(name, text string, code rune) tea.KeyPressMsg {
	if name != "" {
		switch name {
		case "up":
			code = tea.KeyUp
		case "down":
			code = tea.KeyDown
		case "enter":
			code = tea.KeyEnter
		case "backspace":
			code = tea.KeyBackspace
		case "esc":
			code = tea.KeyEsc
		case "f2":
			code = tea.KeyF2
		}
	}
	return tea.KeyPressMsg(tea.Key{Text: text, Code: code})
}
