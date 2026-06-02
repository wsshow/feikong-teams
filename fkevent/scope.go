package fkevent

import (
	"sync"

	"github.com/cloudwego/eino/adk"
)

// MemberScope 标记 AgentTool 内部事件所属的父级工具调用。
type MemberScope struct {
	CallID   string
	ToolName string
	Name     string
}

var agentEventScopes sync.Map

// RegisterAgentEventScope 为即将转发的 ADK 事件登记成员调用信息。
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

func (s MemberScope) apply(event *Event) {
	if event == nil || s.CallID == "" {
		return
	}
	event.IsMemberEvent = true
	event.MemberCallID = s.CallID
	event.MemberToolName = s.ToolName
	event.MemberName = s.Name
}
