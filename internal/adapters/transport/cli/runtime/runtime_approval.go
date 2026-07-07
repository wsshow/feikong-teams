package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"fkteams/internal/adapters/transport/cli/tui"
	"fkteams/internal/runtime/approval"

	tea "charm.land/bubbletea/v2"
)

type runtimeApprovalState struct {
	ID       string
	Info     string
	Selected int
}

type runtimeApprovalBroker struct {
	mu      sync.Mutex
	nextID  uint64
	pending map[string]chan int
	send    func(tea.Msg)
}

func newRuntimeApprovalBroker(send func(tea.Msg)) *runtimeApprovalBroker {
	return &runtimeApprovalBroker{
		pending: make(map[string]chan int),
		send:    send,
	}
}

func (b *runtimeApprovalBroker) Handle(ctx context.Context, info string) (int, error) {
	if b == nil {
		return approval.Reject, fmt.Errorf("approval broker is not initialized")
	}
	id := fmt.Sprintf("approval-%d", atomic.AddUint64(&b.nextID, 1))
	ch := make(chan int, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()

	b.sendMsg(runtimeApprovalPendingMsg{approval: runtimeApprovalState{
		ID:       id,
		Info:     strings.TrimSpace(info),
		Selected: 0,
	}})

	select {
	case <-ctx.Done():
		b.complete(id)
		b.sendMsg(runtimeApprovalCancelledMsg{id: id})
		return approval.Reject, ctx.Err()
	case decision := <-ch:
		b.complete(id)
		b.sendMsg(runtimeApprovalAnsweredMsg{id: id, decision: decision})
		return decision, nil
	}
}

func (b *runtimeApprovalBroker) Submit(id string, decision int) bool {
	if b == nil || id == "" {
		return false
	}
	b.mu.Lock()
	ch := b.pending[id]
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}

func (b *runtimeApprovalBroker) sendMsg(msg tea.Msg) {
	if b != nil && b.send != nil {
		b.send(msg)
	}
}

func (b *runtimeApprovalBroker) complete(id string) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

func (m runtimeModel) updateApproval(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.approval == nil {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		if m.runtime.submitApproval(m.approval.ID, approval.Reject) {
			m.status = "审批已拒绝，正在取消当前任务..."
			m.approval = nil
		}
		return m.startRuntimeCancel()
	case "up", "k":
		m.approval.Selected = (m.approval.Selected + len(runtimeApprovalOptions()) - 1) % len(runtimeApprovalOptions())
		return m, nil
	case "down", "j", "tab":
		m.approval.Selected = (m.approval.Selected + 1) % len(runtimeApprovalOptions())
		return m, nil
	case "esc":
		return m.submitApprovalDecision(approval.Reject)
	case "enter":
		return m.submitApprovalDecision(runtimeApprovalOptions()[m.approval.Selected].Decision)
	default:
		return m, nil
	}
}

func (m runtimeModel) submitApprovalDecision(decision int) (tea.Model, tea.Cmd) {
	if m.approval == nil {
		return m, nil
	}
	id := m.approval.ID
	if m.runtime.submitApproval(id, decision) {
		m.status = "已提交审批决定"
		return m, nil
	}
	m.appendBlock(runtimeBlockError, "审批失败", "approval request is no longer pending")
	m.approval = nil
	return m, nil
}

func (m *runtimeModel) applyApprovalPending(state runtimeApprovalState) {
	if state.Selected < 0 || state.Selected >= len(runtimeApprovalOptions()) {
		state.Selected = 0
	}
	m.approval = &state
	m.status = "等待权限审批..."
}

func (m *runtimeModel) applyApprovalAnswered(id string, decision int) {
	if m.approval != nil && m.approval.ID == id {
		m.approval = nil
	}
	m.status = "审批已处理: " + runtimeApprovalDecisionLabel(decision)
}

func (m *runtimeModel) applyApprovalCancelled(id string) {
	if m.approval != nil && m.approval.ID == id {
		m.approval = nil
	}
}

func (m runtimeModel) renderApprovalPanel() string {
	if m.approval == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(tui.PickerTitle("权限审批"))
	if m.approval.Info != "" {
		for _, line := range strings.Split(m.approval.Info, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			sb.WriteString("\n")
			sb.WriteString(tui.Dim(truncateRuntimeText(line, max(20, m.contentWidth()-8))))
		}
	}
	for i, option := range runtimeApprovalOptions() {
		prefix := "  "
		label := option.Label
		if i == m.approval.Selected {
			prefix = "> "
			label = tui.PickerSelected(label)
		}
		sb.WriteString("\n")
		sb.WriteString(prefix)
		sb.WriteString(label)
	}
	return tui.PickerBox(max(24, m.contentWidth()-2), sb.String())
}

func runtimeApprovalDecisionLabel(decision int) string {
	for _, option := range runtimeApprovalOptions() {
		if option.Decision == decision {
			return option.Label
		}
	}
	return "拒绝执行"
}

type runtimeApprovalOption struct {
	Label    string
	Decision int
}

func runtimeApprovalOptions() []runtimeApprovalOption {
	return []runtimeApprovalOption{
		{Label: "允许一次", Decision: approval.ApproveOnce},
		{Label: "该会话允许该项", Decision: approval.ApproveItem},
		{Label: "该会话允许所有", Decision: approval.ApproveAll},
		{Label: "拒绝执行", Decision: approval.Reject},
	}
}

type runtimeApprovalPendingMsg struct {
	approval runtimeApprovalState
}

type runtimeApprovalAnsweredMsg struct {
	id       string
	decision int
}

type runtimeApprovalCancelledMsg struct {
	id string
}
