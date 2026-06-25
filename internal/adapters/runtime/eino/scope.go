package eino

import (
	"fkteams/agentcore"
	"sync"

	"github.com/cloudwego/eino/adk"
)

// MemberScope 标记成员工具调用内产生的 ADK 事件。
type MemberScope struct {
	CallID   string
	ToolName string
	Name     string
}

var agentEventScopes sync.Map

func RegisterAgentEventScope(event *adk.AgentEvent, scope MemberScope) {
	if event == nil || scope.CallID == "" {
		return
	}
	agentEventScopes.Store(event, scope)
}

func consumeAgentEventScope(event *adk.AgentEvent) (MemberScope, func()) {
	if event == nil {
		return MemberScope{}, func() {}
	}
	v, ok := agentEventScopes.Load(event)
	if !ok {
		return MemberScope{}, func() {}
	}
	scope, _ := v.(MemberScope)
	return scope, func() { agentEventScopes.Delete(event) }
}

func (s MemberScope) apply(event *agentcore.Event, c *converter) {
	if event == nil || s.CallID == "" {
		return
	}
	event.MemberCallID = s.CallID
	event.MemberToolName = s.ToolName
	event.MemberName = s.Name
	event.ParentToolCallID = s.CallID
	event.ParentToolName = s.ToolName
	if event.MemberOrder == nil && s.CallID != "" {
		if order, ok := c.identities.orderForID(s.CallID); ok {
			event.MemberOrder = intPtr(order)
		}
	}
}

func intPtr(v int) *int {
	return &v
}
