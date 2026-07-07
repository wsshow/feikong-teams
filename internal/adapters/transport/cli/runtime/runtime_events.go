package runtime

import (
	"encoding/json"

	domainmessage "fkteams/internal/domain/message"

	"fkteams/internal/app/agent/catalog/toolmeta"

	"fkteams/internal/runtime/events"

	"fkteams/internal/adapters/transport/cli/tui"

	"strings"

	"time"

	tea "charm.land/bubbletea/v2"
)

func (m *runtimeModel) applyEvent(event events.Event) {
	if events.IsInternalContinueContent(event.Content) {
		return
	}
	if event.TotalTokens > 0 {
		m.totalTokens = event.TotalTokens
	}
	if event.MemberCallID != "" || event.MemberName != "" {
		m.applyMemberEvent(event)
		return
	}
	agent := event.AgentName
	if agent == "" {
		agent = runtimeDefaultAgentName
	}
	switch event.Type {
	case events.EventType(events.NotifyProcessingStart):
		if event.Detail == "steering" && strings.TrimSpace(event.Content) != "" {
			m.activeOutput = -1
			m.activeReason = -1
			m.appendBlock(runtimeBlockSystem, "转向消息", event.Content)
		}
	case events.EventAssistantText, events.EventAssistantReasoning, events.EventToolCallArguments:
		switch event.DeltaKind {
		case events.DeltaReasoning:
			content := event.ReasoningContent
			if content == "" {
				content = event.Content
			}
			m.appendReasoning(agent, content)
		case events.DeltaToolArgs:
			m.activeOutput = -1
			m.activeReason = -1
			if member, _ := m.memberForToolEvent(event); member != nil {
				member.Status = "running"
				m.syncMemberSummary(member)
				return
			}
			if _, ok := runtimeAgentToolDisplay(event.ToolName); ok {
				return
			}
			if event.ToolName == "" && runtimeLikelyPendingAgentToolArgs(event) {
				return
			}
			m.upsertToolCall(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusRunning)
		default:
			m.appendOutput(agent, event.Content)
		}
	case events.EventToolCallStarted:
		m.activeOutput = -1
		m.activeReason = -1
		for i, tool := range events.ToolCallsFromEvent(event) {
			if display, ok := runtimeAgentToolDisplay(tool.Function.Name); ok {
				aliases := runtimeAgentToolCallAliases(tool)
				key := runtimeAgentToolCallKey(tool)
				if mapped := m.memberKeyForAliases(aliases...); mapped != "" {
					key = mapped
				}
				if member := m.ensureAgentToolMember(key, display.Target, tool.Function.Arguments); member != nil {
					m.registerMemberTool(member.Key, aliases...)
				}
				continue
			}
			display := toolmeta.FormatToolDisplay(tool.Function.Name)
			key := events.ToolCallRefAt(event, tool, i)
			m.upsertToolCall(key, display.DisplayName, tool.Function.Arguments, tui.ToolStatusRunning)
		}
	case events.EventToolCallCompleted, events.EventToolCallResult:
		m.activeOutput = -1
		m.activeReason = -1
		if m.applyAgentToolResult(event) {
			return
		}
		m.upsertToolResult(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusDone, event.Type == events.EventToolCallResult)
	case events.EventSystemNotice:
		m.activeOutput = -1
		m.activeReason = -1
		if event.Content != "" {
			m.appendBlock(runtimeBlockSystem, "system_notice", event.Content)
		}
	case events.EventError:
		msg := event.Error
		if msg == "" {
			msg = event.Content
		}
		if member, _ := m.memberForToolEvent(event); member != nil {
			member.Status = "error"
			if msg != "" {
				member.markDirty()
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockError, Title: member.Name, Content: msg})
			}
			m.syncMemberSummary(member)
			return
		}
		if event.ToolName != "" {
			m.upsertToolResult(runtimeToolEventKey(event), event.ToolName, msg, tui.ToolStatusError, false)
			return
		}
		m.appendBlock(runtimeBlockError, agent, msg)
	}
}

func (m *runtimeModel) applyMemberEvent(event events.Event) {
	member := m.ensureMember(event)
	if member == nil {
		return
	}
	member.setStatusRunning()
	agent := event.AgentName
	if agent == "" {
		agent = member.Name
	}
	switch event.Type {
	case events.EventAssistantText, events.EventAssistantReasoning, events.EventToolCallArguments:
		switch event.DeltaKind {
		case events.DeltaReasoning:
			content := event.ReasoningContent
			if content == "" {
				content = event.Content
			}
			member.appendReasoning(agent, content)
		case events.DeltaToolArgs:
			member.ActiveOutput = -1
			member.ActiveReason = -1
			member.upsertToolCall(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusRunning)
		default:
			member.appendOutput(agent, event.Content)
		}
	case events.EventAssistantCompleted:
		if event.Content != "" && len(member.Blocks) == 0 {
			member.appendOutput(agent, event.Content)
		}
		member.setStatusDone()
	case events.EventToolCallStarted:
		member.ActiveOutput = -1
		member.ActiveReason = -1
		for i, tool := range events.ToolCallsFromEvent(event) {
			key := events.ToolCallRefAt(event, tool, i)
			display := toolmeta.FormatToolDisplay(tool.Function.Name)
			member.upsertToolCall(key, display.DisplayName, tool.Function.Arguments, tui.ToolStatusRunning)
		}
	case events.EventToolCallCompleted, events.EventToolCallResult:
		member.ActiveOutput = -1
		member.ActiveReason = -1
		member.upsertToolResult(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusDone, event.Type == events.EventToolCallResult)
	case events.EventError:
		msg := event.Error
		if msg == "" {
			msg = event.Content
		}
		member.Status = "error"
		if event.ToolName != "" {
			member.upsertToolResult(runtimeToolEventKey(event), event.ToolName, msg, tui.ToolStatusError, false)
		} else {
			member.markDirty()
			member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockError, Title: agent, Content: msg})
		}
	case events.EventAgentCompleted:
		member.setStatusDone()
	case events.EventSystemNotice:
		if event.Content != "" {
			member.markDirty()
			member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockSystem, Title: "system_notice", Content: event.Content})
		}
	}
	m.syncMemberSummary(member)
}

func (m *runtimeModel) applyAgentToolResult(event events.Event) bool {
	member, _ := m.memberForToolEvent(event)
	if member == nil {
		display, ok := runtimeAgentToolDisplay(event.ToolName)
		if !ok {
			return false
		}
		aliases := runtimeAgentToolEventAliases(event)
		key := runtimeAgentToolEventKey(event)
		if mapped := m.memberKeyForAliases(aliases...); mapped != "" {
			key = mapped
		}
		if key == "" {
			return true
		}
		member = m.ensureAgentToolMember(key, display.Target, "")
		if member == nil {
			return true
		}
		m.registerMemberTool(member.Key, aliases...)
	}
	member.ActiveOutput = -1
	member.ActiveReason = -1
	if event.Type == events.EventToolCallResult {
		member.setStatusRunning()
	} else {
		member.setStatusDone()
	}
	if event.Content != "" && len(member.Blocks) == 0 {
		member.appendOutput(member.Name, event.Content)
	}
	m.syncMemberSummary(member)
	return true
}

func (m runtimeModel) isExitConfirming() bool {
	return !m.exitUntil.IsZero() && time.Now().Before(m.exitUntil)
}

func (m runtimeModel) isCopiedNoticeVisible() bool {
	return !m.copiedUntil.IsZero() && time.Now().Before(m.copiedUntil)
}

func isRuntimeShiftEnter(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return (key.Code == tea.KeyEnter && key.Mod&tea.ModShift != 0) || msg.Keystroke() == "shift+enter"
}

func runtimeExitTickCmd() tea.Cmd {
	return tea.Tick(runtimeExitConfirmTick, func(t time.Time) tea.Msg {
		return runtimeExitTickMsg(t)
	})
}

func runtimeSelectionCopiedTickCmd() tea.Cmd {
	return tea.Tick(runtimeSelectionNotice, func(t time.Time) tea.Msg {
		return runtimeSelectionCopiedTickMsg(t)
	})
}

func emptyRuntimeToolName(name string) string {
	if name == "" {
		return runtimeDefaultToolName
	}
	return name
}

func emptyRuntimeMemberName(name string) string {
	if name == "" {
		return "member"
	}
	return name
}

func runtimeAgentToolDisplay(name string) (toolmeta.ToolDisplay, bool) {
	if name == "" {
		return toolmeta.ToolDisplay{}, false
	}
	display := toolmeta.FormatToolDisplay(name)
	if display.Kind == toolmeta.ToolKindAgent {
		return display, true
	}
	if strings.HasPrefix(name, toolmeta.AgentToolPrefix) {
		target := strings.TrimPrefix(name, toolmeta.AgentToolPrefix)
		return toolmeta.ToolDisplay{
			Name:        name,
			DisplayName: "指派给 " + target,
			Kind:        toolmeta.ToolKindAgent,
			Target:      target,
		}, true
	}
	return display, false
}

func runtimeAgentTaskFromCompleteArgs(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return ""
	}
	for _, key := range []string{"request", "task", "goal", "objective", "description"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func runtimeAgentToolEventAliases(event events.Event) []string {
	return compactRuntimeAliases([]string{event.ToolCallID})
}

func runtimeAgentToolEventKey(event events.Event) string {
	return event.ToolCallID
}

func runtimeLikelyPendingAgentToolArgs(event events.Event) bool {
	if (event.Type != events.EventAssistantText && event.Type != events.EventToolCallArguments) || event.DeltaKind != events.DeltaToolArgs {
		return false
	}
	return event.ToolCallID != "" || event.ToolCallRef != ""
}

func runtimeAgentToolCallAliases(tool domainmessage.ToolCall) []string {
	return compactRuntimeAliases([]string{tool.ID})
}

func runtimeAgentToolCallKey(tool domainmessage.ToolCall) string {
	return tool.ID
}

func runtimeDirectToolEventAliases(event events.Event) []string {
	return compactRuntimeAliases([]string{event.ToolCallID})
}

func runtimeMemberEventAliases(event events.Event) []string {
	aliases := []string{
		event.MemberCallID,
		event.ParentToolCallID,
	}
	return compactRuntimeAliases(aliases)
}

func compactRuntimeAliases(aliases []string) []string {
	seen := make(map[string]bool, len(aliases))
	result := aliases[:0]
	for _, alias := range aliases {
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		result = append(result, alias)
	}
	return result
}

func runtimeToolEventKey(event events.Event) string {
	if event.ToolCallRef != "" {
		return event.ToolCallRef
	}
	return ""
}

func runtimeMemberKey(event events.Event) string {
	if event.MemberCallID != "" {
		return event.MemberCallID
	}
	if event.ParentToolCallID != "" {
		return event.ParentToolCallID
	}
	return ""
}

func runtimeMemberName(event events.Event) string {
	if event.MemberName != "" {
		return event.MemberName
	}
	if event.AgentName != "" {
		return event.AgentName
	}
	if event.MemberToolName != "" {
		return event.MemberToolName
	}
	return "member"
}

func truncateRuntimeText(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
