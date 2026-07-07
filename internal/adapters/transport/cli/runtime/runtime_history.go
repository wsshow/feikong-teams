package runtime

import (
	"fkteams/internal/app/agent/catalog/toolmeta"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/runtime/events"

	"fkteams/internal/adapters/transport/cli/tui"

	"strings"
)

func (m *runtimeModel) appendBlock(kind runtimeBlockKind, title, content string) {
	m.blocks = append(m.blocks, runtimeBlock{Kind: kind, Title: title, Content: content})
	m.trimBlocks()
}

func (m *runtimeModel) appendHistoryBlock(block runtimeBlock) {
	m.blocks = append(m.blocks, block)
	m.trimBlocks()
}

func (m *runtimeModel) trimBlocks() {
	if len(m.blocks) > 200 {
		m.blocks = m.blocks[len(m.blocks)-200:]
	}
}

func (m *runtimeModel) appendLoadedHistory() {
	recorder := m.runtime.session.recorder()
	messages := recorder.GetMessages()
	if len(messages) == 0 {
		return
	}
	for _, msg := range messages {
		m.appendHistoryMessage(msg)
	}
	m.activeOutput = -1
	m.activeReason = -1
}

func (m *runtimeModel) appendHistoryMessage(msg eventlog.AgentMessage) {
	if msg.MemberCallID != "" {
		m.appendHistoryMemberMessage(msg)
		return
	}
	agent := msg.AgentName
	if agent == "" {
		agent = runtimeDefaultAgentName
	}
	if msg.MemberName != "" {
		agent = msg.MemberName
	}
	if agent == "user" || agent == "用户" {
		content := strings.TrimSpace(msg.GetTextContent())
		if content != "" {
			m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockUser, Title: "用户", Content: content})
		}
		return
	}
	for _, event := range msg.Events {
		switch event.Type {
		case eventlog.MsgTypeReasoning:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockReasoning, Title: agent, Content: event.Content})
			}
		case eventlog.MsgTypeText:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockAssistant, Title: agent, Content: event.Content})
			}
		case eventlog.MsgTypeToolCall:
			if event.ToolCall != nil {
				m.appendHistoryToolCall(event.ToolCall)
			}
		case eventlog.MsgTypeNotice:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockSystem, Title: "system_notice", Content: event.Content})
			}
		case eventlog.MsgTypeError:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockError, Title: agent, Content: event.Content})
			}
		}
	}
}

func (m *runtimeModel) appendHistoryMemberMessage(msg eventlog.AgentMessage) {
	key := msg.MemberCallID
	name := msg.MemberName
	if name == "" {
		name = msg.AgentName
	}
	member := m.ensureHistoryMember(key, name, "", "done")
	if member == nil {
		return
	}
	for _, event := range msg.Events {
		switch event.Type {
		case eventlog.MsgTypeReasoning:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockReasoning, Title: name, Content: event.Content})
			}
		case eventlog.MsgTypeText:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: name, Content: event.Content})
			}
		case eventlog.MsgTypeToolCall:
			if event.ToolCall != nil {
				m.appendHistoryMemberToolCall(member, event.ToolCall)
			}
		case eventlog.MsgTypeNotice:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockSystem, Title: "system_notice", Content: event.Content})
			}
		case eventlog.MsgTypeError:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockError, Title: name, Content: event.Content})
			}
		}
	}
	member.Status = "done"
	member.markDirty()
	m.syncMemberSummary(member)
}

func (m *runtimeModel) appendHistoryToolCall(tool *eventlog.ToolCallRecord) {
	name := tool.DisplayName
	if name == "" {
		name = tool.Name
	}
	if tool.Kind == toolmeta.ToolKindAgent {
		key := tool.ID
		member := m.ensureHistoryMember(key, tool.Target, runtimeAgentTaskFromCompleteArgs(tool.Arguments), "done")
		if member == nil {
			return
		}
		m.registerMemberTool(member.Key, tool.ID)
		if strings.TrimSpace(tool.Result) != "" && len(member.Blocks) == 0 {
			member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: member.Name, Content: tool.Result})
			member.markDirty()
		}
		m.syncMemberSummary(member)
		return
	}
	block := runtimeBlock{
		Kind:       runtimeBlockTool,
		ToolKey:    tool.Ref,
		ToolName:   emptyRuntimeToolName(name),
		ToolArgs:   tool.Arguments,
		ToolStatus: tui.ToolStatusDone,
	}
	if tool.Result != "" {
		block.ToolResult = tool.Result
		block.ToolHasResult = true
	}
	m.appendHistoryBlock(block)
}

func (m *runtimeModel) appendHistoryMemberToolCall(member *runtimeMemberState, tool *eventlog.ToolCallRecord) {
	if member == nil || tool == nil {
		return
	}
	name := tool.DisplayName
	if name == "" {
		name = tool.Name
	}
	block := runtimeBlock{
		Kind:       runtimeBlockTool,
		ToolKey:    tool.Ref,
		ToolName:   emptyRuntimeToolName(name),
		ToolArgs:   tool.Arguments,
		ToolStatus: tui.ToolStatusDone,
	}
	if strings.TrimSpace(tool.Result) != "" {
		block.ToolResult = tool.Result
		block.ToolHasResult = true
	}
	member.Blocks = append(member.Blocks, block)
	member.ToolCount++
	member.markDirty()
}

func (m *runtimeModel) ensureHistoryMember(key, name, task, status string) *runtimeMemberState {
	if key == "" {
		return nil
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         emptyRuntimeMemberName(name),
			Status:       status,
			Task:         task,
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		if member.Status == "" {
			member.Status = "done"
		}
		m.members[key] = member
		m.syncMemberSummary(member)
		return member
	}
	if name != "" {
		member.Name = name
	}
	if shouldReplaceRuntimeMemberTask(member.Task, task) {
		member.Task = task
	}
	if status != "" {
		member.Status = status
	}
	member.markDirty()
	m.syncMemberSummary(member)
	return member
}

func shouldReplaceRuntimeMemberTask(current, next string) bool {
	next = strings.TrimSpace(next)
	if next == "" {
		return false
	}
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	return strings.HasPrefix(current, "{") || strings.HasPrefix(current, "[")
}

func (m *runtimeModel) ensureMember(event events.Event) *runtimeMemberState {
	key := runtimeMemberKey(event)
	if mapped := m.memberKeyForAliases(runtimeMemberEventAliases(event)...); mapped != "" {
		key = mapped
	}
	if key == "" {
		return nil
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         runtimeMemberName(event),
			Status:       "running",
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		m.members[key] = member
		m.blocks = append(m.blocks, runtimeBlock{
			Kind:         runtimeBlockMember,
			MemberKey:    key,
			MemberName:   member.Name,
			MemberStatus: member.Status,
		})
	} else if name := runtimeMemberName(event); name != "" {
		if member.Name != name {
			member.markDirty()
		}
		member.Name = name
	}
	m.registerMemberTool(member.Key, runtimeMemberEventAliases(event)...)
	m.syncMemberSummary(member)
	return member
}

func (m *runtimeModel) ensureAgentToolMember(key, name, task string) *runtimeMemberState {
	if key == "" {
		return nil
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	if mapped := m.memberKeyForAliases(key); mapped != "" {
		key = mapped
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         emptyRuntimeMemberName(name),
			Status:       "running",
			Task:         runtimeAgentTaskFromCompleteArgs(task),
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		m.members[key] = member
	} else {
		if name != "" {
			if member.Name != name {
				member.markDirty()
			}
			member.Name = name
		}
		if parsed := runtimeAgentTaskFromCompleteArgs(task); shouldReplaceRuntimeMemberTask(member.Task, parsed) {
			member.Task = parsed
			member.markDirty()
		}
	}
	m.registerMemberTool(key, key)
	m.syncMemberSummary(member)
	return member
}

func (m *runtimeModel) registerMemberTool(memberKey string, aliases ...string) {
	if memberKey == "" {
		return
	}
	if m.memberTools == nil {
		m.memberTools = make(map[string]string)
	}
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		m.memberTools[alias] = memberKey
	}
}

func (m runtimeModel) memberKeyForAliases(aliases ...string) string {
	if m.memberTools == nil {
		return ""
	}
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		if key := m.memberTools[alias]; key != "" {
			return key
		}
	}
	return ""
}

func (m runtimeModel) memberForToolEvent(event events.Event) (*runtimeMemberState, string) {
	if m.members == nil || m.memberTools == nil {
		return nil, ""
	}
	for _, alias := range runtimeDirectToolEventAliases(event) {
		if alias == "" {
			continue
		}
		if key := m.memberTools[alias]; key != "" {
			return m.members[key], key
		}
	}
	return nil, ""
}

func (m *runtimeModel) syncMemberSummary(member *runtimeMemberState) {
	if member == nil {
		return
	}
	for i := range m.blocks {
		if m.blocks[i].Kind == runtimeBlockMember && m.blocks[i].MemberKey == member.Key {
			m.blocks[i].MemberName = member.Name
			m.blocks[i].MemberStatus = member.Status
			m.blocks[i].MemberTask = member.Task
			m.blocks[i].MemberTools = member.ToolCount
			return
		}
	}
	m.blocks = append(m.blocks, runtimeBlock{
		Kind:         runtimeBlockMember,
		MemberKey:    member.Key,
		MemberName:   member.Name,
		MemberStatus: member.Status,
		MemberTask:   member.Task,
		MemberTools:  member.ToolCount,
	})
}

func runtimeMemberToolChainItems(member *runtimeMemberState) []tui.ToolChainItem {
	if member == nil {
		return nil
	}
	items := make([]tui.ToolChainItem, 0)
	for _, block := range member.Blocks {
		if block.Kind != runtimeBlockTool {
			continue
		}
		item := tui.ToolChainItem{
			Name:   block.ToolName,
			Args:   block.ToolArgs,
			Status: string(block.ToolStatus),
		}
		if block.ToolStatus == tui.ToolStatusError {
			item.Error = block.ToolResult
		}
		items = append(items, item)
	}
	return items
}

func (m *runtimeModel) upsertToolCall(key, name, args string, status tui.ToolStatus) {
	if key == "" {
		return
	}
	idx := m.findToolBlock(key)
	if idx < 0 {
		m.blocks = append(m.blocks, runtimeBlock{
			Kind:       runtimeBlockTool,
			ToolKey:    key,
			ToolName:   emptyRuntimeToolName(name),
			ToolArgs:   args,
			ToolStatus: status,
		})
		return
	}
	block := &m.blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if args != "" {
		block.ToolArgs = args
	}
	block.ToolStatus = status
}

func (m *runtimeModel) upsertToolResult(key, name, result string, status tui.ToolStatus, appendResult bool) {
	if key == "" {
		return
	}
	idx := m.findToolBlock(key)
	if idx < 0 {
		m.blocks = append(m.blocks, runtimeBlock{
			Kind:          runtimeBlockTool,
			ToolKey:       key,
			ToolName:      emptyRuntimeToolName(name),
			ToolResult:    result,
			ToolStatus:    status,
			ToolHasResult: true,
		})
		return
	}
	block := &m.blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if appendResult {
		block.ToolResult += result
	} else {
		block.ToolResult = result
	}
	block.ToolStatus = status
	block.ToolHasResult = true
}

func (m runtimeModel) findToolBlock(key string) int {
	if key == "" {
		return -1
	}
	for i := len(m.blocks) - 1; i >= 0; i-- {
		block := m.blocks[i]
		if block.Kind == runtimeBlockTool && block.ToolKey == key {
			return i
		}
	}
	return -1
}

func (s *runtimeMemberState) markDirty() {
	if s == nil {
		return
	}
	s.RenderDirty = true
}

func (s *runtimeMemberState) hasPendingAsk() bool {
	if s == nil {
		return false
	}
	for _, askState := range s.PendingAsks {
		if !askState.Answered {
			return true
		}
	}
	return false
}

func (s *runtimeMemberState) firstPendingAsk() *runtimeAskState {
	if s == nil {
		return nil
	}
	for i := range s.PendingAsks {
		if !s.PendingAsks[i].Answered {
			return &s.PendingAsks[i]
		}
	}
	return nil
}

func (s *runtimeMemberState) askByID(askID string) *runtimeAskState {
	if s == nil || askID == "" {
		return nil
	}
	for i := range s.PendingAsks {
		if s.PendingAsks[i].ID == askID {
			return &s.PendingAsks[i]
		}
	}
	return nil
}

func (s *runtimeMemberState) askForToolKey(toolKey string) *runtimeAskState {
	if s == nil || toolKey == "" {
		return nil
	}
	for i := range s.PendingAsks {
		if runtimeAskToolKey(s.PendingAsks[i]) == toolKey {
			return &s.PendingAsks[i]
		}
	}
	return nil
}

func (s *runtimeMemberState) upsertAsk(askState runtimeAskState) {
	if s == nil || askState.ID == "" {
		return
	}
	s.markDirty()
	for i := range s.PendingAsks {
		if s.PendingAsks[i].ID == askState.ID {
			s.PendingAsks[i] = askState
			return
		}
	}
	s.PendingAsks = append(s.PendingAsks, askState)
}

func (s *runtimeMemberState) markAskAnswered(askID string, selected []string, freeText string) bool {
	if s == nil || askID == "" {
		return false
	}
	for i := range s.PendingAsks {
		if s.PendingAsks[i].ID != askID {
			continue
		}
		s.PendingAsks[i].Answered = true
		s.PendingAsks[i].Selected = append([]string(nil), selected...)
		s.PendingAsks[i].FreeText = freeText
		s.markDirty()
		return true
	}
	return false
}

func (s *runtimeMemberState) removeAsk(askID string) bool {
	if s == nil || askID == "" {
		return false
	}
	for i := range s.PendingAsks {
		if s.PendingAsks[i].ID != askID {
			continue
		}
		s.PendingAsks = append(s.PendingAsks[:i], s.PendingAsks[i+1:]...)
		s.markDirty()
		return true
	}
	return false
}

func (s *runtimeMemberState) setStatusRunning() {
	if s == nil || s.hasPendingAsk() {
		return
	}
	s.Status = "running"
}

func (s *runtimeMemberState) setStatusDone() {
	if s == nil || s.hasPendingAsk() {
		return
	}
	s.Status = "done"
}

func (m *runtimeModel) markMembersDirty() {
	for _, member := range m.members {
		member.markDirty()
	}
}

func (s *runtimeMemberState) appendOutput(agent, content string) {
	if content == "" {
		return
	}
	s.markDirty()
	s.ActiveReason = -1
	if s.ActiveOutput >= 0 && s.ActiveOutput < len(s.Blocks) && s.Blocks[s.ActiveOutput].Kind == runtimeBlockAssistant {
		s.Blocks[s.ActiveOutput].Content += content
		return
	}
	s.Blocks = append(s.Blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: agent, Content: content})
	s.ActiveOutput = len(s.Blocks) - 1
}

func (s *runtimeMemberState) appendReasoning(agent, content string) {
	if content == "" {
		return
	}
	s.markDirty()
	s.ActiveOutput = -1
	if s.ActiveReason >= 0 && s.ActiveReason < len(s.Blocks) && s.Blocks[s.ActiveReason].Kind == runtimeBlockReasoning {
		s.Blocks[s.ActiveReason].Content += content
		return
	}
	s.Blocks = append(s.Blocks, runtimeBlock{Kind: runtimeBlockReasoning, Title: agent, Content: content})
	s.ActiveReason = len(s.Blocks) - 1
}

func (s *runtimeMemberState) upsertToolCall(key, name, args string, status tui.ToolStatus) {
	if key == "" {
		return
	}
	s.markDirty()
	idx := s.findToolBlock(key)
	if idx < 0 {
		s.Blocks = append(s.Blocks, runtimeBlock{
			Kind:       runtimeBlockTool,
			ToolKey:    key,
			ToolName:   emptyRuntimeToolName(name),
			ToolArgs:   args,
			ToolStatus: status,
		})
		s.ToolCount++
		return
	}
	block := &s.Blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if args != "" {
		block.ToolArgs = args
	}
	block.ToolStatus = status
}

func (s *runtimeMemberState) upsertToolResult(key, name, result string, status tui.ToolStatus, appendResult bool) {
	if key == "" {
		return
	}
	s.markDirty()
	idx := s.findToolBlock(key)
	if idx < 0 {
		s.Blocks = append(s.Blocks, runtimeBlock{
			Kind:          runtimeBlockTool,
			ToolKey:       key,
			ToolName:      emptyRuntimeToolName(name),
			ToolResult:    result,
			ToolStatus:    status,
			ToolHasResult: true,
		})
		s.ToolCount++
		return
	}
	block := &s.Blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if appendResult {
		block.ToolResult += result
	} else {
		block.ToolResult = result
	}
	block.ToolStatus = status
	block.ToolHasResult = true
}

func (s runtimeMemberState) findToolBlock(key string) int {
	if key == "" {
		return -1
	}
	for i := len(s.Blocks) - 1; i >= 0; i-- {
		block := s.Blocks[i]
		if block.Kind == runtimeBlockTool && block.ToolKey == key {
			return i
		}
	}
	return -1
}

func (m *runtimeModel) appendOutput(agent, content string) {
	if content == "" {
		return
	}
	m.activeReason = -1
	if m.activeOutput >= 0 && m.activeOutput < len(m.blocks) && m.blocks[m.activeOutput].Kind == runtimeBlockAssistant {
		m.blocks[m.activeOutput].Content += content
		return
	}
	m.blocks = append(m.blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: agent, Content: content})
	m.activeOutput = len(m.blocks) - 1
}

func (m *runtimeModel) appendReasoning(agent, content string) {
	if content == "" {
		return
	}
	m.activeOutput = -1
	if m.activeReason >= 0 && m.activeReason < len(m.blocks) && m.blocks[m.activeReason].Kind == runtimeBlockReasoning {
		m.blocks[m.activeReason].Content += content
		return
	}
	m.blocks = append(m.blocks, runtimeBlock{Kind: runtimeBlockReasoning, Title: agent, Content: content})
	m.activeReason = len(m.blocks) - 1
}
