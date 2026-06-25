package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"fkteams/internal/adapters/transport/cli/tui"
	"fkteams/internal/app/tools/ask"

	tea "charm.land/bubbletea/v2"
)

type runtimeAskState struct {
	ID          string
	MemberKey   string
	MemberName  string
	Question    string
	Options     []string
	MultiSelect bool
	ToolCallID  string
	ToolName    string
	Answered    bool
	Selected    []string
	FreeText    string
}

type runtimeAskBroker struct {
	mu      sync.Mutex
	pending map[string]chan *ask.AskResponse
	send    func(tea.Msg)
}

func newRuntimeAskBroker(send func(tea.Msg)) *runtimeAskBroker {
	return &runtimeAskBroker{
		pending: make(map[string]chan *ask.AskResponse),
		send:    send,
	}
}

func (b *runtimeAskBroker) Handle(ctx context.Context, req ask.RuntimeRequest) (*ask.AskResponse, error) {
	if b == nil || req.Info == nil || req.ID == "" {
		return nil, fmt.Errorf("invalid ask request")
	}
	responseCh := make(chan *ask.AskResponse, 1)
	b.mu.Lock()
	if _, exists := b.pending[req.ID]; exists {
		b.mu.Unlock()
		return nil, fmt.Errorf("duplicate ask request")
	}
	b.pending[req.ID] = responseCh
	b.mu.Unlock()

	state := runtimeAskState{
		ID:          req.ID,
		MemberKey:   req.Metadata.MemberCallID,
		MemberName:  req.Metadata.MemberName,
		Question:    req.Info.Question,
		Options:     append([]string(nil), req.Info.Options...),
		MultiSelect: req.Info.MultiSelect,
		ToolCallID:  req.ToolCallID,
		ToolName:    req.ToolName,
	}
	b.sendMsg(runtimeAskPendingMsg{ask: state})

	select {
	case <-ctx.Done():
		b.complete(req.ID)
		b.sendMsg(runtimeAskCancelledMsg{askID: req.ID})
		return nil, ctx.Err()
	case resp := <-responseCh:
		b.complete(req.ID)
		if resp == nil {
			return nil, fmt.Errorf("invalid ask response")
		}
		if resp.AskID == "" {
			resp.AskID = req.ID
		}
		b.sendMsg(runtimeAskAnsweredMsg{
			askID:    req.ID,
			selected: append([]string(nil), resp.Selected...),
			freeText: resp.FreeText,
		})
		return resp, nil
	}
}

func (b *runtimeAskBroker) Submit(askID string, resp *ask.AskResponse) bool {
	if b == nil || askID == "" || resp == nil {
		return false
	}
	b.mu.Lock()
	ch := b.pending[askID]
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- resp:
		return true
	default:
		return false
	}
}

func (b *runtimeAskBroker) sendMsg(msg tea.Msg) {
	if b != nil && b.send != nil {
		b.send(msg)
	}
}

func (b *runtimeAskBroker) complete(askID string) {
	b.mu.Lock()
	delete(b.pending, askID)
	b.mu.Unlock()
}

func parseRuntimeAskResponse(info runtimeAskState, input string) *ask.AskResponse {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	selected := parseRuntimeAskSelected(info, input)
	resp := &ask.AskResponse{AskID: info.ID}
	if len(selected) > 0 {
		resp.Selected = selected
		return resp
	}
	resp.FreeText = input
	return resp
}

func parseRuntimeAskSelected(info runtimeAskState, input string) []string {
	if len(info.Options) == 0 {
		return nil
	}
	if info.MultiSelect {
		return parseRuntimeAskMultiSelected(info.Options, input)
	}
	if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(info.Options) {
		return []string{info.Options[idx-1]}
	}
	for _, option := range info.Options {
		if input == option {
			return []string{option}
		}
	}
	return nil
}

func parseRuntimeAskMultiSelected(options []string, input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\t' || r == '\n'
	})
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(fields))
	selected := make([]string, 0, len(fields))
	for _, field := range fields {
		idx, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || idx < 1 || idx > len(options) || seen[idx] {
			return nil
		}
		seen[idx] = true
		selected = append(selected, options[idx-1])
	}
	return selected
}

func (m runtimeModel) currentMemberPendingAsk() *runtimeAskState {
	member := m.currentMember()
	if member == nil {
		return nil
	}
	return member.firstPendingAsk()
}

func (m runtimeModel) submitCurrentMemberAsk() (tea.Model, tea.Cmd) {
	askState := m.currentMemberPendingAsk()
	if askState == nil {
		return m, nil
	}
	input := strings.TrimSpace(m.expandInput())
	if input == "" {
		return m, nil
	}
	resp := parseRuntimeAskResponse(*askState, input)
	if resp == nil {
		return m, nil
	}
	m.input.SetValue("")
	m.pastes = nil
	m.savedInput = ""
	m.historyIndex = len(m.runtime.session.InputHistory)
	if m.runtime.submitAsk(askState.ID, resp) {
		m.status = "已提交成员回答"
		return m, nil
	}
	m.appendBlock(runtimeBlockError, "ask_questions", "ask request is no longer pending")
	return m, nil
}

func (m *runtimeModel) applyAskPending(askState runtimeAskState) {
	member := m.ensureAskMember(askState)
	if member == nil {
		return
	}
	if askState.ToolName == "" {
		askState.ToolName = "ask_questions"
	}
	member.upsertAsk(askState)
	member.Status = "waiting"
	member.upsertToolCall(runtimeAskToolKey(askState), askState.ToolName, askState.Question, tui.ToolStatusRunning)
	m.syncMemberSummary(member)
}

func (m *runtimeModel) applyAskAnswered(askID string, selected []string, freeText string) {
	member := m.memberForAsk(askID)
	if member == nil {
		return
	}
	askState := member.askByID(askID)
	toolKey := runtimeAskToolKey(runtimeAskState{ID: askID})
	if askState != nil {
		toolKey = runtimeAskToolKey(*askState)
	}
	member.markAskAnswered(askID, selected, freeText)
	member.upsertToolResult(toolKey, "ask_questions", runtimeAskResponseSummary(selected, freeText), tui.ToolStatusDone, false)
	if !member.hasPendingAsk() {
		member.Status = "running"
	}
	m.syncMemberSummary(member)
}

func (m *runtimeModel) applyAskCancelled(askID string) {
	member := m.memberForAsk(askID)
	if member == nil {
		return
	}
	member.removeAsk(askID)
	if !member.hasPendingAsk() && member.Status == "waiting" {
		member.Status = "running"
	}
	m.syncMemberSummary(member)
}

func (m *runtimeModel) ensureAskMember(askState runtimeAskState) *runtimeMemberState {
	key := askState.MemberKey
	if mapped := m.memberKeyForAliases(key, askState.ToolCallID); mapped != "" {
		key = mapped
	}
	if key == "" {
		key = "ask:" + askState.ID
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         emptyRuntimeMemberName(askState.MemberName),
			Status:       "waiting",
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		m.members[key] = member
	}
	if askState.MemberName != "" {
		member.Name = askState.MemberName
	}
	m.registerMemberTool(member.Key, askState.MemberKey, askState.ToolCallID)
	return member
}

func (m runtimeModel) memberForAsk(askID string) *runtimeMemberState {
	if askID == "" {
		return nil
	}
	for _, member := range m.members {
		for _, askState := range member.PendingAsks {
			if askState.ID == askID {
				return member
			}
		}
	}
	return nil
}

func runtimeAskToolKey(askState runtimeAskState) string {
	if askState.ToolCallID != "" {
		return "tool_call:" + askState.ToolCallID
	}
	return "ask:" + askState.ID
}

func runtimeAskResponseSummary(selected []string, freeText string) string {
	if len(selected) > 0 {
		return strings.Join(selected, ", ")
	}
	return strings.TrimSpace(freeText)
}

type runtimeAskPendingMsg struct {
	ask runtimeAskState
}

type runtimeAskAnsweredMsg struct {
	askID    string
	selected []string
	freeText string
}

type runtimeAskCancelledMsg struct {
	askID string
}
