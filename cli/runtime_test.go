package cli

import (
	"context"
	"fkteams/fkevent"
	"fkteams/tui"
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

	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{X: 1, Y: 0, Button: tea.MouseLeft}))
	model = updated.(runtimeModel)
	updated, _ = model.Update(tea.MouseMotionMsg(tea.Mouse{X: 4, Y: 0, Button: tea.MouseLeft}))
	model = updated.(runtimeModel)
	if !model.selection.Active {
		t.Fatal("mouse drag should start an active selection")
	}

	updated, _ = model.Update(tea.MouseReleaseMsg(tea.Mouse{X: 4, Y: 0, Button: tea.MouseLeft}))
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

	model.applyEvent(fkevent.Event{Type: fkevent.EventReasoningChunk, Content: "用户"})
	model.applyEvent(fkevent.Event{Type: fkevent.EventReasoningChunk, Content: "问好"})

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

func TestRuntimeConversationMarkersArePlainText(t *testing.T) {
	for _, marker := range []string{tui.PromptMarker(), tui.DoneMarker()} {
		if strings.Contains(marker, "\x1b[") {
			t.Fatalf("conversation markers should not contain ANSI styles, got %q", marker)
		}
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

func TestRuntimeTranscriptWrapsLongLines(t *testing.T) {
	model := newRuntimeModel(&Runtime{
		ctx:         context.Background(),
		session:     NewSession(ModeTeam, nil, nil),
		exitSignals: make(chan os.Signal, 1),
	})
	model.width = 30
	model.appendBlock(runtimeBlockTool, "工具", strings.Repeat("长文本", 20))

	transcript := model.transcriptText()
	if !strings.Contains(transcript, strings.Repeat("长文本", 4)) {
		t.Fatalf("wrapped transcript should keep long content, got %q", transcript)
	}
	if tui.LineCount(transcript) < 3 {
		t.Fatalf("long transcript line should wrap into multiple lines, got %q", transcript)
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
		case "f2":
			code = tea.KeyF2
		}
	}
	return tea.KeyPressMsg(tea.Key{Text: text, Code: code})
}
