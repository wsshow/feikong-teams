package runtime

import (
	"fkteams/internal/adapters/transport/cli/tui"

	"time"

	"charm.land/bubbles/v2/textinput"

	lipgloss "charm.land/lipgloss/v2"
)

type runtimeModel struct {
	runtime      *Runtime
	input        textinput.Model
	width        int
	height       int
	blocks       []runtimeBlock
	activeOutput int
	activeReason int
	historyIndex int
	savedInput   string
	pastes       []string
	picker       *runtimePicker
	scrollOffset int
	selection    tui.TextSelection
	running      bool
	cancelling   bool
	status       string
	totalTokens  int
	exitUntil    time.Time
	copiedUntil  time.Time
	welcome      tui.WelcomeInfo
	members      map[string]*runtimeMemberState
	memberTools  map[string]string
	memberView   string
	approval     *runtimeApprovalState
}

type runtimeSelectionCopiedTickMsg time.Time

type runtimeBlockKind string

const (
	runtimeBlockUser      runtimeBlockKind = "user"
	runtimeBlockAssistant runtimeBlockKind = "assistant"
	runtimeBlockReasoning runtimeBlockKind = "reasoning"
	runtimeBlockTool      runtimeBlockKind = "tool"
	runtimeBlockSystem    runtimeBlockKind = "system"
	runtimeBlockError     runtimeBlockKind = "error"
	runtimeBlockDone      runtimeBlockKind = "done"
	runtimeBlockMeta      runtimeBlockKind = "meta"
	runtimeBlockBanner    runtimeBlockKind = "banner"
	runtimeBlockWelcome   runtimeBlockKind = "welcome"
	runtimeBlockInterrupt runtimeBlockKind = "interrupt"
	runtimeBlockMember    runtimeBlockKind = "member"
)

type runtimeBlock struct {
	Kind          runtimeBlockKind
	Title         string
	Content       string
	ToolKey       string
	ToolName      string
	ToolArgs      string
	ToolResult    string
	ToolStatus    tui.ToolStatus
	ToolHasResult bool
	MemberKey     string
	MemberName    string
	MemberStatus  string
	MemberTask    string
	MemberTools   int
}

type runtimeMemberState struct {
	Key          string
	Name         string
	Status       string
	Task         string
	Blocks       []runtimeBlock
	ActiveOutput int
	ActiveReason int
	ToolCount    int
	PendingAsks  []runtimeAskState
	ScrollOffset int
	RenderCache  string
	RenderDirty  bool
}

func newRuntimeModel(r *Runtime) runtimeModel {
	ti := textinput.New()
	ti.Prompt = "❯ "
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	ti.SetStyles(s)
	ti.SetWidth(80)
	ti.Focus()
	model := runtimeModel{
		runtime:      r,
		input:        ti,
		activeOutput: -1,
		activeReason: -1,
		historyIndex: len(r.session.InputHistory),
		status:       "就绪",
		welcome:      runtimeWelcomeInfo(r.session),
		members:      make(map[string]*runtimeMemberState),
		memberTools:  make(map[string]string),
	}
	model.appendBlock(runtimeBlockWelcome, "欢迎", "")
	model.appendLoadedHistory()
	return model
}
