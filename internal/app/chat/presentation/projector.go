package presentation

import (
	"encoding/json"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
	"sort"
	"strings"
)

type messageState struct {
	node    *Node
	parent  string
	layout  message.OutputLayout
	done    bool
	buffers map[string]*strings.Builder
}

// Projector 由所属会话串行调用；历史与实时使用相同的归属、分段和状态规则。
type Projector struct {
	items          map[string]*Item
	messages       map[string]*messageState
	tools          map[string]*Tool
	members        map[string]*Member
	memberMessages map[string][]string
	asks           map[string]*Ask
	askOrders      map[string]int64
	askTools       map[string]string
	approvals      map[string]*Approval
	seen           map[string]bool
	changed        map[string]bool
	displays       toolmeta.Resolver
	lastSequence   int64
	sessionID      string
}

// New 创建一个会话级展示投影器。
func New(sessionID string, displays ...toolmeta.Resolver) *Projector {
	p := &Projector{items: map[string]*Item{}, messages: map[string]*messageState{}, tools: map[string]*Tool{}, members: map[string]*Member{}, memberMessages: map[string][]string{}, asks: map[string]*Ask{}, askOrders: map[string]int64{}, askTools: map[string]string{}, approvals: map[string]*Approval{}, seen: map[string]bool{}, sessionID: sessionID}
	if len(displays) > 0 {
		p.displays = displays[0]
	}
	return p
}

// Snapshot 返回按事实顺序排列的独立快照。
func (p *Projector) Snapshot() Snapshot {
	result := Snapshot{Items: []Item{}}
	for _, item := range p.items {
		if item.Kind == "message" && item.Node.Message.Role == message.RoleAssistant && len(item.Node.Parts) == 0 {
			continue
		}
		result.Items = append(result.Items, *item)
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].Order == result.Items[j].Order {
			return result.Items[i].ID < result.Items[j].ID
		}
		return result.Items[i].Order < result.Items[j].Order
	})
	for i := range result.Items {
		result.Items[i].Index = i
	}
	return result.Clone()
}

// Apply 接收一次运行事实，产生可幂等应用的节点更新。
func (p *Projector) Apply(e event.Event) Update {
	if e.SessionSequence > 0 {
		if e.SessionSequence <= p.lastSequence {
			return Update{}
		}
		p.lastSequence = e.SessionSequence
		e.Sequence = e.SessionSequence
	} else {
		if e.EventID != "" && p.seen[e.EventID] {
			return Update{}
		}
		if e.EventID != "" {
			p.seen[e.EventID] = true
		}
	}
	p.changed = map[string]bool{}
	var appendOp *Append
	switch e.Type {
	case event.TypeUserMessage:
		s := p.ensureMessage(e, message.RoleUser)
		s.node.Message.Content = e.Content
		if e.Message != nil {
			s.node.Message.Content = e.Message.DisplayText()
			s.node.Message.ContentParts = e.Message.Clone().ContentParts
		}
		p.touchMessage(e.MessageID)
	case event.TypeAssistantStarted:
		p.ensureMessage(e, message.RoleAssistant)
		p.touchMessage(e.MessageID)
	case event.TypeAssistantText, event.TypeAssistantReasoning:
		if e.Role == message.RoleTool {
			break
		}
		s := p.ensureMessage(e, message.RoleAssistant)
		if s.done {
			break
		}
		kind := "text"
		if e.Type == event.TypeAssistantReasoning {
			kind = "reasoning"
		}
		part := s.layout.Append(e.MessageID, kind, e.Content, "")
		key := part.ID
		if e.BlockID != "" {
			key = e.BlockID
			s.layout.Parts[len(s.layout.Parts)-1].ID = key
		}
		root := p.rootFor(e.MessageID)
		found := false
		for i := range s.node.Parts {
			if s.node.Parts[i].Key == key {
				if s.buffers == nil {
					s.buffers = map[string]*strings.Builder{}
				}
				buffer := s.buffers[key]
				if buffer == nil {
					buffer = &strings.Builder{}
					buffer.WriteString(s.node.Parts[i].Content)
					s.buffers[key] = buffer
				}
				buffer.WriteString(e.Content)
				s.node.Parts[i].Content = buffer.String()
				s.node.Parts[i].Streaming = true
				found = true
				break
			}
		}
		if !found {
			s.node.Parts = append(s.node.Parts, Part{Type: kind, Key: key, Content: e.Content, Streaming: true})
		} else {
			appendOp = &Append{ItemID: root, Key: key, Content: e.Content, MessageID: e.MessageID, Type: kind}
		}
		if kind == "text" {
			s.node.Message.Content = s.layout.Text
		}
		p.touchMessage(e.MessageID)
	case event.TypeToolCallArguments:
		s := p.ensureMessage(e, message.RoleAssistant)
		if s.done {
			break
		}
		call := message.ToolCall{ID: e.ToolCallID, Function: message.FunctionCall{Name: e.ToolName}}
		tool := p.ensureTool(e, call)
		tool.Arguments += e.Content
		p.ensureToolPart(s, tool)
		s.layout.Append(e.MessageID, "tool", "", tool.ID)
		p.touchMessage(e.MessageID)
	case event.TypeAssistantCompleted:
		if e.Role == message.RoleTool || e.Role == message.RoleUser {
			break
		}
		s := p.ensureMessage(e, message.RoleAssistant)
		msg := message.Message{Role: message.RoleAssistant, Content: e.Content, ReasoningContent: e.ReasoningContent, ToolCalls: events.ToolCallsFromEvent(e)}
		if e.Message != nil {
			msg = e.Message.Clone()
		}
		for _, call := range msg.ToolCalls {
			if events.IsInternalToolName(call.Function.Name) {
				continue
			}
			p.ensureTool(e, call)
		}
		layout := e.OutputParts
		if len(layout) == 0 {
			layout = s.layout.Complete(e.MessageID, msg)
		}
		parts := []Part{}
		for _, part := range layout {
			switch part.Type {
			case "reasoning", "text":
				content := msg.DisplayText()
				if part.Type == "reasoning" {
					content = msg.ReasoningContent
				}
				if part.Start < 0 || part.End < part.Start || part.End > len(content) {
					continue
				}
				if part.End > part.Start {
					parts = append(parts, Part{Type: part.Type, Key: part.ID, Content: content[part.Start:part.End]})
				}
			case "tool":
				tool := p.tools[toolRef(part.ToolCallID)]
				if tool != nil && tool.Name != "ask_questions" {
					parts = append(parts, Part{Type: "tool", Key: part.ID, Tool: tool, Member: p.members[tool.ID]})
				}
			}
		}
		s.node.Parts = parts
		s.node.Message.CreatedAt = e.CreatedAt
		s.node.Message.Content = msg.DisplayText()
		s.node.Message.ContentParts = msg.ContentParts
		s.node.ShowCopyAction = len(msg.ToolCalls) == 0 && msg.DisplayText() != ""
		s.done = true
		s.layout = message.OutputLayout{}
		s.buffers = nil
		p.touchMessage(e.MessageID)
	case event.TypeToolCallStarted, event.TypeToolCallResult, event.TypeToolCallCompleted, event.TypeToolCallFailed:
		tool := p.ensureTool(e, message.ToolCall{ID: e.ToolCallID, Function: message.FunctionCall{Name: e.ToolName, Arguments: e.ToolArgs}})
		if e.Type == event.TypeToolCallStarted {
			tool.Status = "running"
		} else if e.Type == event.TypeToolCallResult {
			tool.Status = "running"
			tool.Result += e.Content
		} else {
			tool.Status = "completed"
			if e.Type == event.TypeToolCallFailed {
				tool.Status = "error"
			}
			tool.Result = e.ToolResult
			if tool.Result == "" {
				tool.Result = e.Content
			}
			if e.ToolStatus != "" {
				tool.Status = string(e.ToolStatus)
			} else if tool.Status == "completed" {
				tool.Status = string(event.ToolResultStatus(tool.Result))
			}
			if member := p.members[tool.ID]; member != nil {
				member.Completed = true
			}
		}
		s := p.ensureMessageForTool(e, tool)
		p.ensureToolPart(s, tool)
		p.touchMessage(tool.MessageID)
		if e.Type == event.TypeToolCallCompleted && tool.Name == "ask_questions" && tool.Status == "completed" {
			p.recoverQuestion(e, tool)
		}
	case event.TypeAskRequested, event.TypeAskAnswered:
		if e.Ask == nil || e.Ask.ID == "" {
			break
		}
		if e.ToolCallID != "" {
			p.askTools[toolRef(e.ToolCallID)] = e.Ask.ID
		}
		a := p.asks[e.Ask.ID]
		if a == nil {
			a = &Ask{ID: e.Ask.ID, SessionID: p.sessionID, MessageID: e.MessageID, Options: []string{}, Selected: []string{}, MemberID: e.MemberCallID, Active: true, RunID: e.RunID}
			p.asks[a.ID] = a
			p.askOrders[a.ID] = e.Sequence
		}
		if e.Ask.Question != "" {
			a.Question = e.Ask.Question
		}
		if e.Ask.Options != nil {
			a.Options = append([]string{}, e.Ask.Options...)
		}
		if e.Type == event.TypeAskRequested {
			a.MultiSelect = e.Ask.MultiSelect
		}
		if e.Type == event.TypeAskAnswered {
			a.Answered = true
			a.Active = false
			a.Selected = append([]string{}, e.Ask.Selected...)
			a.FreeText = e.Ask.FreeText
		}
		id := "ask:" + a.ID
		if e.MemberCallID != "" {
			m := p.ensureMember(e)
			exists := false
			for i := range m.Parts {
				if m.Parts[i].Ask == a {
					exists = true
				}
			}
			if !exists {
				m.Parts = append(m.Parts, Part{Type: "ask", Key: id, Ask: a})
			}
			p.touchMember(e.MemberCallID)
		} else {
			if p.items[id] == nil {
				p.items[id] = &Item{ID: id, Kind: "ask", Order: e.Sequence, Ask: a}
			}
			p.changed[id] = true
		}
	case event.TypeApprovalRequested, event.TypeApprovalAnswered:
		id := e.Detail
		if e.Approval != nil && e.Approval.ID != "" {
			id = e.Approval.ID
		}
		if id == "" {
			break
		}
		a := p.approvals[id]
		if a == nil {
			a = &Approval{ID: id, Order: e.Sequence, Message: e.Content, Active: true, RunID: e.RunID}
			p.approvals[id] = a
			p.items["approval:"+id] = &Item{ID: "approval:" + id, Kind: "approval", Order: e.Sequence, Approval: a}
		}
		if e.Approval != nil {
			if e.Approval.Message != "" {
				a.Message = e.Approval.Message
			}
			if e.Approval.Decision != "" {
				a.Decision = e.Approval.Decision
			}
		}
		if e.Type == event.TypeApprovalAnswered {
			a.Answered = true
			a.Active = false
		}
		p.changed["approval:"+id] = true
	case event.TypeError:
		f := events.NormalizeFriendlyError(e.Error)
		id := "error:" + e.EventID
		p.items[id] = &Item{ID: id, Kind: "error", Order: e.Sequence, Error: &Error{ID: id, Order: e.Sequence, Title: f.Title, Message: f.Message, Suggestions: f.Suggestions, TechnicalDetail: f.TechnicalDetail}}
		p.changed[id] = true
		p.closeRun(e, "error")
	case event.TypeCancelled, event.TypeTurnCancelled:
		p.closeRun(e, "cancelled")
		if e.Type == event.TypeCancelled {
			id := "cancelled:" + e.EventID
			p.items[id] = &Item{ID: id, Kind: "message", Order: e.Sequence, Node: &Node{Message: ViewMessage{ID: id, Role: message.RoleSystem, Content: e.Content, CreatedAt: e.CreatedAt}, Parts: []Part{}}}
			p.changed[id] = true
		}
	case event.TypeTurnFailed:
		p.closeRun(e, "error")
	case event.TypeTurnCompleted, event.TypeProcessingEnd:
		p.closeRun(e, "interrupted")
	case event.TypeMemberCompleted, event.TypeAgentCompleted:
		if e.MemberCallID != "" {
			p.ensureMember(e).Completed = true
			p.touchMember(e.MemberCallID)
		}
	}
	update := Update{}
	if appendOp != nil && len(p.changed) == 1 && p.changed[appendOp.ItemID] {
		update.Appends = []Append{*appendOp}
		return update
	}
	snap := p.Snapshot()
	visible := make(map[string]bool, len(snap.Items))
	for _, item := range snap.Items {
		visible[item.ID] = true
		if p.changed[item.ID] {
			update.Items = append(update.Items, item)
		}
	}
	for id := range p.changed {
		if !visible[id] {
			update.Remove = append(update.Remove, id)
		}
	}
	sort.Strings(update.Remove)
	return update
}

// recoverQuestion 仅从已经结束的工具事实恢复旧入口遗漏的只读问答，不补造可提交的中断。
func (p *Projector) recoverQuestion(e event.Event, tool *Tool) {
	if p.askTools[tool.Ref] != "" {
		return
	}
	var info struct {
		Question    string   `json:"question"`
		Options     []string `json:"options"`
		MultiSelect bool     `json:"multi_select"`
	}
	var response struct {
		Selected []string `json:"selected"`
		FreeText string   `json:"free_text"`
	}
	if json.Unmarshal([]byte(tool.Arguments), &info) != nil || info.Question == "" || json.Unmarshal([]byte(tool.Result), &response) != nil {
		return
	}
	for _, existing := range p.asks {
		if existing.RunID == e.RunID && existing.MemberID == e.MemberCallID && existing.Question == info.Question {
			bound := false
			for _, askID := range p.askTools {
				if askID == existing.ID {
					bound = true
					break
				}
			}
			if bound {
				continue
			}
			p.askTools[tool.Ref] = existing.ID
			return
		}
	}
	id := tool.Ref
	a := &Ask{ID: id, SessionID: p.sessionID, MessageID: tool.MessageID, RunID: e.RunID, MemberID: e.MemberCallID,
		Question: info.Question, Options: append([]string{}, info.Options...), MultiSelect: info.MultiSelect,
		Selected: append([]string{}, response.Selected...), FreeText: response.FreeText,
		Answered: len(response.Selected) > 0 || response.FreeText != ""}
	p.asks[id], p.askOrders[id], p.askTools[tool.Ref] = a, e.Sequence, id
	if e.MemberCallID != "" {
		p.ensureMember(e)
		p.touchMember(e.MemberCallID)
	} else {
		key := "ask:" + id
		p.items[key] = &Item{ID: key, Kind: "ask", Order: e.Sequence, Ask: a}
		p.changed[key] = true
	}
}
func (p *Projector) ensureMessage(e event.Event, role message.Role) *messageState {
	id := e.MessageID
	if id == "" {
		id = e.EventID
	}
	e.MessageID = id
	if s := p.messages[id]; s != nil {
		return s
	}
	order := e.MessageOrder
	if order == 0 {
		order = e.Sequence
	}
	node := &Node{Message: ViewMessage{ID: id, Role: role, Agent: e.AgentName, CreatedAt: e.CreatedAt, RunID: e.RunID}, Parts: []Part{}, Order: order}
	s := &messageState{node: node, parent: e.MemberCallID}
	p.messages[id] = s
	if e.MemberCallID != "" {
		m := p.ensureMember(e)
		p.memberMessages[m.ID] = append(p.memberMessages[m.ID], id)
	} else {
		node.ShowAgentLabel = role == message.RoleAssistant
		var previous *Item
		for _, item := range p.items {
			if item.Kind == "message" && item.Order < order && (previous == nil || item.Order > previous.Order) {
				previous = item
			}
		}
		if previous != nil && previous.Node.Message.Role == role && previous.Node.Message.Agent == e.AgentName {
			node.ShowAgentLabel = false
		}
		p.items[id] = &Item{ID: id, Kind: "message", Order: order, Node: node}
	}
	return s
}
func (p *Projector) ensureTool(e event.Event, call message.ToolCall) *Tool {
	id := call.ID
	if id == "" {
		id = e.ToolCallID
	}
	ref := toolRef(id)
	tool := p.tools[ref]
	if tool == nil {
		owner := e.MessageID
		if owner == "" {
			owner = "tool:" + id
		}
		tool = &Tool{ID: id, Ref: ref, Name: call.Function.Name, Status: "pending", MessageID: owner}
		p.tools[ref] = tool
	}
	if call.Function.Name != "" {
		tool.Name = call.Function.Name
	}
	if call.Function.Arguments != "" {
		tool.Arguments = call.Function.Arguments
	}
	display := toolmeta.FallbackDisplay(tool.Name)
	if p.displays != nil {
		display = p.displays.FormatToolDisplay(tool.Name)
	}
	tool.DisplayName, tool.Kind, tool.Target = display.DisplayName, display.Kind, display.Target
	return tool
}
func (p *Projector) ensureMessageForTool(e event.Event, t *Tool) *messageState {
	if s := p.messages[t.MessageID]; s != nil {
		return s
	}
	e.MessageID = t.MessageID
	return p.ensureMessage(e, message.RoleAssistant)
}
func (p *Projector) ensureToolPart(s *messageState, t *Tool) {
	if t.Name == "ask_questions" {
		return
	}
	for _, part := range s.node.Parts {
		if part.Tool == t {
			return
		}
	}
	s.node.Parts = append(s.node.Parts, Part{Type: "tool", Key: s.node.Message.ID + ":tool:" + t.ID, Tool: t, Member: p.members[t.ID]})
}
func (p *Projector) ensureMember(e event.Event) *Member {
	id := e.MemberCallID
	if m := p.members[id]; m != nil {
		return m
	}
	m := &Member{ID: id, Name: e.MemberName, Parts: []Part{}}
	if m.Name == "" {
		m.Name = e.AgentName
	}
	p.members[id] = m
	ref := toolRef(id)
	if t := p.tools[ref]; t != nil {
		s := p.messages[t.MessageID]
		if s != nil {
			for i := range s.node.Parts {
				if s.node.Parts[i].Tool == t {
					s.node.Parts[i].Member = m
				}
			}
		}
	} else {
		key := "member:" + id
		p.items[key] = &Item{ID: key, Kind: "member", Order: e.Sequence, Member: m}
	}
	return m
}
func (p *Projector) touchMessage(id string) {
	s := p.messages[id]
	if s == nil {
		return
	}
	if s.parent != "" {
		p.touchMember(s.parent)
	} else {
		p.changed[id] = true
	}
}
func (p *Projector) touchMember(id string) {
	m := p.members[id]
	if m == nil {
		return
	}
	type segment struct {
		order int64
		parts []Part
	}
	segments := []segment{}
	for _, mid := range p.memberMessages[id] {
		s := p.messages[mid]
		segments = append(segments, segment{s.node.Order, s.node.Parts})
	}
	for _, ask := range p.asks {
		if ask.MemberID == id {
			segments = append(segments, segment{p.askOrders[ask.ID], []Part{{Type: "ask", Key: "ask:" + ask.ID, Ask: ask}}})
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].order < segments[j].order })
	m.Parts = []Part{}
	m.ToolCount = 0
	for _, segment := range segments {
		m.Parts = append(m.Parts, segment.parts...)
		for _, part := range segment.parts {
			if part.Tool != nil {
				m.ToolCount++
			}
		}
	}
	if t := p.tools[toolRef(id)]; t != nil {
		p.touchMessage(t.MessageID)
	} else {
		p.changed["member:"+id] = true
	}
}
func (p *Projector) rootFor(id string) string {
	s := p.messages[id]
	if s != nil && s.parent != "" {
		if t := p.tools[toolRef(s.parent)]; t != nil {
			return p.rootFor(t.MessageID)
		}
		return "member:" + s.parent
	}
	return id
}
func (p *Projector) closeRun(e event.Event, status string) {
	for _, ask := range p.asks {
		if ask.Active && (e.RunID == "" || ask.RunID == e.RunID) {
			ask.Active = false
			if ask.MemberID != "" {
				p.touchMember(ask.MemberID)
			} else {
				p.changed["ask:"+ask.ID] = true
			}
		}
	}
	for _, approval := range p.approvals {
		if approval.Active && (e.RunID == "" || approval.RunID == e.RunID) {
			approval.Active = false
			p.changed["approval:"+approval.ID] = true
		}
	}

	for id, s := range p.messages {
		if e.RunID != "" && s.node.Message.RunID != e.RunID {
			continue
		}
		s.done = true
		for i := range s.node.Parts {
			s.node.Parts[i].Streaming = false
		}
		for _, part := range s.node.Parts {
			if part.Tool != nil && (part.Tool.Status == "pending" || part.Tool.Status == "running") {
				part.Tool.Status = status
			}
		}
		p.touchMessage(id)
	}
}
func toolRef(id string) string { return "tool_call:" + strings.TrimPrefix(id, "tool_call:") }
