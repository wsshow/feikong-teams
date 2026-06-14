package eino

import (
	"fkteams/agentcore"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type toolIdentityTracker struct {
	refsByID             sync.Map
	ordersByID           sync.Map
	idsByKey             sync.Map
	pendingResultsByName map[string][]string
	activeResultsByName  map[string]string
}

func newToolIdentityTracker() *toolIdentityTracker {
	return &toolIdentityTracker{
		pendingResultsByName: make(map[string][]string),
		activeResultsByName:  make(map[string]string),
	}
}

func (t *toolIdentityTracker) ensure(sourceMessageID string, position int, scope MemberScope, tc *agentcore.ToolCall) string {
	if tc == nil {
		return ""
	}
	if tc.ID == "" {
		key := t.syntheticKey(sourceMessageID, position, scope, tc)
		if existing, ok := t.idsByKey.Load(key); ok {
			if value, ok := existing.(string); ok && value != "" {
				tc.ID = value
			}
		}
		if tc.ID == "" {
			tc.ID = "fk_tool_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			t.idsByKey.Store(key, tc.ID)
		}
	}
	ref := "tool_call:" + tc.ID
	t.refsByID.Store(tc.ID, ref)
	if tc.Index != nil {
		t.ordersByID.Store(tc.ID, *tc.Index)
	}
	return ref
}

func (t *toolIdentityTracker) refsFor(toolCalls []agentcore.ToolCall) map[int]string {
	if len(toolCalls) == 0 {
		return nil
	}
	refs := make(map[int]string, len(toolCalls))
	for position, tc := range toolCalls {
		if tc.ID == "" {
			continue
		}
		ref, ok := t.refsByID.Load(tc.ID)
		if !ok {
			continue
		}
		value, ok := ref.(string)
		if !ok || value == "" {
			continue
		}
		key := position
		if tc.Index != nil {
			key = *tc.Index
		}
		refs[key] = value
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func (t *toolIdentityTracker) attach(event *agentcore.Event) {
	if event == nil {
		return
	}
	if event.ToolCallID == "" && event.ToolName != "" {
		event.ToolCallID = t.resultIDByName(event.ToolName, event.Type == agentcore.EventToolEnd)
	}
	if event.ToolCallID == "" {
		return
	}
	if ref, ok := t.refsByID.Load(event.ToolCallID); ok {
		if value, ok := ref.(string); ok && value != "" {
			event.ToolCallRef = value
		}
	}
	if event.ToolCallRef == "" && event.ToolName != "" {
		if mappedID := t.resultIDByName(event.ToolName, event.Type == agentcore.EventToolEnd); mappedID != "" {
			event.ToolCallID = mappedID
			if ref, ok := t.refsByID.Load(event.ToolCallID); ok {
				if value, ok := ref.(string); ok && value != "" {
					event.ToolCallRef = value
				}
			}
		}
	}
	if order, ok := t.orderForID(event.ToolCallID); ok {
		event.ToolCallIndex = &order
	}
	if event.Type == agentcore.EventToolEnd && event.ToolName != "" {
		delete(t.activeResultsByName, event.ToolName)
		t.consumePendingResult(event.ToolName, event.ToolCallID)
	}
}

func (t *toolIdentityTracker) rememberResult(name, id string) {
	if name == "" || id == "" {
		return
	}
	t.pendingResultsByName[name] = append(t.pendingResultsByName[name], id)
}

func (t *toolIdentityTracker) orderForID(id string) (int, bool) {
	order, ok := t.ordersByID.Load(id)
	if !ok {
		return 0, false
	}
	value, ok := order.(int)
	return value, ok
}

func (t *toolIdentityTracker) syntheticKey(sourceMessageID string, position int, scope MemberScope, tc *agentcore.ToolCall) string {
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

func (t *toolIdentityTracker) resultIDByName(name string, done bool) string {
	if name == "" {
		return ""
	}
	if id := t.activeResultsByName[name]; id != "" {
		if done {
			delete(t.activeResultsByName, name)
			t.consumePendingResult(name, id)
		}
		return id
	}
	queue := t.pendingResultsByName[name]
	if len(queue) == 0 {
		return ""
	}
	id := queue[0]
	if done {
		t.pendingResultsByName[name] = queue[1:]
	} else {
		t.activeResultsByName[name] = id
	}
	return id
}

func (t *toolIdentityTracker) consumePendingResult(name, id string) {
	queue := t.pendingResultsByName[name]
	if len(queue) == 0 {
		return
	}
	if queue[0] == id {
		t.pendingResultsByName[name] = queue[1:]
		return
	}
	for i, queued := range queue {
		if queued != id {
			continue
		}
		t.pendingResultsByName[name] = append(queue[:i], queue[i+1:]...)
		return
	}
}
