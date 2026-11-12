package agentkit

import (
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// toolIdentityTracker 由 converter 的互斥锁保护，不在多个运行之间共享。
type toolIdentityTracker struct {
	ownersByID           map[string]string
	refsByID             map[string]string
	ordersByID           map[string]int
	idsByKey             map[string]string
	pendingResultsByName map[string][]string
	activeResultsByName  map[string]string
}

func newToolIdentityTracker() *toolIdentityTracker {
	return &toolIdentityTracker{
		ownersByID: make(map[string]string),
		refsByID:   make(map[string]string), ordersByID: make(map[string]int), idsByKey: make(map[string]string),
		pendingResultsByName: make(map[string][]string),
		activeResultsByName:  make(map[string]string),
	}
}

func (t *toolIdentityTracker) ensure(sourceMessageID string, position int, scope MemberScope, tc *domainmessage.ToolCall) string {
	if tc == nil {
		return ""
	}
	if tc.ID == "" {
		key := t.syntheticKey(sourceMessageID, position, scope, tc)
		tc.ID = t.idsByKey[key]
		if tc.ID == "" {
			tc.ID = "fk_tool_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			t.idsByKey[key] = tc.ID
		}
	}
	ref := "tool_call:" + tc.ID
	t.refsByID[tc.ID] = ref
	t.ownersByID[tc.ID] = sourceMessageID
	if tc.Index != nil {
		t.ordersByID[tc.ID] = *tc.Index
	}
	return ref
}

func (t *toolIdentityTracker) refsFor(toolCalls []domainmessage.ToolCall) map[int]string {
	if len(toolCalls) == 0 {
		return nil
	}
	refs := make(map[int]string, len(toolCalls))
	for position, tc := range toolCalls {
		if tc.ID == "" {
			continue
		}
		ref := t.refsByID[tc.ID]
		if ref == "" {
			continue
		}
		key := position
		if tc.Index != nil {
			key = *tc.Index
		}
		refs[key] = ref
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func (t *toolIdentityTracker) attach(event *domainevent.Event, scope MemberScope) {
	if event == nil {
		return
	}
	if event.ToolCallID == "" && event.ToolName != "" {
		event.ToolCallID = t.resultIDByName(event.ToolName, scope, event.Type == domainevent.TypeToolCallCompleted)
	}
	if event.ToolCallID == "" {
		return
	}
	if ref := t.refsByID[event.ToolCallID]; ref != "" {
		event.ToolCallRef = ref
	}
	if event.ToolCallRef == "" && event.ToolName != "" {
		if mappedID := t.resultIDByName(event.ToolName, scope, event.Type == domainevent.TypeToolCallCompleted); mappedID != "" {
			event.ToolCallID = mappedID
			event.ToolCallRef = t.refsByID[mappedID]
		}
	}
	if order, ok := t.orderForID(event.ToolCallID); ok {
		event.ToolCallIndex = &order
	}
	event.MessageID = t.ownersByID[event.ToolCallID]
	if event.Type == domainevent.TypeToolCallCompleted && event.ToolName != "" {
		key := t.resultNameKey(event.ToolName, scope)
		delete(t.activeResultsByName, key)
		t.consumePendingResult(key, event.ToolCallID)
	}
}

func (t *toolIdentityTracker) rememberResult(name, id string, scope MemberScope) {
	if name == "" || id == "" {
		return
	}
	key := t.resultNameKey(name, scope)
	t.pendingResultsByName[key] = append(t.pendingResultsByName[key], id)
}

func (t *toolIdentityTracker) orderForID(id string) (int, bool) {
	order, ok := t.ordersByID[id]
	return order, ok
}

func (t *toolIdentityTracker) syntheticKey(sourceMessageID string, position int, scope MemberScope, tc *domainmessage.ToolCall) string {
	idx := position
	if tc != nil && tc.Index != nil {
		idx = *tc.Index
	}
	parts := []string{sourceMessageID, fmt.Sprintf("idx:%d", idx)}
	if scope.CallID != "" {
		parts = append(parts, "member:"+scope.CallID)
	}
	return strings.Join(parts, "|")
}

func (t *toolIdentityTracker) resultIDByName(name string, scope MemberScope, done bool) string {
	if name == "" {
		return ""
	}
	key := t.resultNameKey(name, scope)
	if id := t.activeResultsByName[key]; id != "" {
		if done {
			delete(t.activeResultsByName, key)
			t.consumePendingResult(key, id)
		}
		return id
	}
	queue := t.pendingResultsByName[key]
	if len(queue) == 0 {
		return ""
	}
	id := queue[0]
	if done {
		t.pendingResultsByName[key] = queue[1:]
	} else {
		t.activeResultsByName[key] = id
	}
	return id
}

func (t *toolIdentityTracker) consumePendingResult(key, id string) {
	queue := t.pendingResultsByName[key]
	if len(queue) == 0 {
		return
	}
	if queue[0] == id {
		t.pendingResultsByName[key] = queue[1:]
		return
	}
	for i, queued := range queue {
		if queued != id {
			continue
		}
		t.pendingResultsByName[key] = append(queue[:i], queue[i+1:]...)
		return
	}
}

func (t *toolIdentityTracker) resultNameKey(name string, scope MemberScope) string {
	if scope.CallID == "" {
		return name
	}
	return scope.CallID + "\x00" + name
}
