package tools

import (
	"context"
	"encoding/json"
	storageport "fkteams/internal/ports/storage"
	"fmt"
	"strings"
	"sync"

	javascripttool "fkteams/internal/adapters/tools/javascript"
	"fkteams/internal/app/appstate"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"
	"fkteams/internal/runtime/resources"
	"fkteams/internal/runtime/toolpolicy"
)

const maxJavaScriptNestedToolResultBytes = 1 << 20

type javascriptHost struct {
	registry *apptools.ToolGroupRegistry
	state    javascripttool.StateStore
	serial   sync.Mutex
}

func newJavaScriptHost(registry *apptools.ToolGroupRegistry, stores ...storageport.Documents) (*javascriptHost, error) {
	state, err := javascripttool.NewFileStateStore(appdataJavaScriptStateDir(), stores...)
	if err != nil {
		return nil, err
	}
	return &javascriptHost{registry: registry, state: state}, nil
}

func (h *javascriptHost) options() javascripttool.Options {
	return javascripttool.Options{
		ToolCaller:    h,
		StateStore:    h.state,
		NoticeEmitter: h,
	}
}

func (h *javascriptHost) Call(ctx context.Context, request javascripttool.ToolCallRequest) (string, error) {
	if h == nil || h.registry == nil {
		return "", fmt.Errorf("javascript tool host is not initialized")
	}
	var cleaner *resources.Cleaner
	if state := appstate.FromContext(ctx); state != nil {
		cleaner = state.Cleaner()
	}
	resolved, err := h.registry.GetToolsByNameWithCleaner(ctx, request.Group, cleaner)
	if err != nil {
		return "", fmt.Errorf("resolve tool group %s: %w", request.Group, err)
	}
	if !strings.HasPrefix(request.Group, "mcp-") {
		if err := toolpolicy.MarkPolicyRequired(resolved); err != nil {
			return "", fmt.Errorf("mark tool group %s policy: %w", request.Group, err)
		}
	}
	if err := toolpolicy.ClassifyTools(resolved); err != nil {
		return "", fmt.Errorf("classify tool group %s: %w", request.Group, err)
	}
	target, info, err := findJavaScriptTarget(ctx, resolved, request.Tool)
	if err != nil {
		return "", err
	}
	if info.Policy.Serialize {
		h.serial.Lock()
		defer h.serial.Unlock()
	}
	arguments, err := json.Marshal(request.Args)
	if err != nil {
		return "", fmt.Errorf("encode nested tool arguments: %w", err)
	}
	meta := map[string]any{
		"call_id":   request.CallID,
		"source":    "javascript",
		"script_id": request.ScriptID,
		"group":     request.Group,
	}
	before, err := hooks.FromContext(ctx).InvokeBeforeToolCall(ctx, hooks.BeforeToolCallPayload{
		ToolName: request.Tool,
		Args:     string(arguments),
		Meta:     meta,
	})
	if err != nil {
		return "", err
	}
	result, toolErr := target.Invoke(ctx, runtimeport.ToolInvocation{
		Name:      request.Tool,
		CallID:    request.CallID,
		Arguments: before.Args,
		Meta:      meta,
	})
	content := ""
	if result != nil {
		content = result.Content
	}
	if len(content) > maxJavaScriptNestedToolResultBytes {
		content = content[:maxJavaScriptNestedToolResultBytes]
		if toolErr == nil {
			toolErr = fmt.Errorf("nested tool result exceeds %d bytes", maxJavaScriptNestedToolResultBytes)
		}
	}
	afterErr := hooks.FromContext(ctx).InvokeAfterToolCall(ctx, hooks.AfterToolCallPayload{
		ToolName: request.Tool,
		Args:     before.Args,
		Result:   content,
		Error:    toolErr,
		Meta:     meta,
	})
	if toolErr != nil {
		return "", toolErr
	}
	if afterErr != nil {
		return "", afterErr
	}
	return content, nil
}

func (h *javascriptHost) Notice(ctx context.Context, scriptID, level, message string) error {
	event := events.SystemNotice("", "", "javascript_extension", message)
	event.Notice.Level = level
	event.Notice.Code = "javascript_extension:" + scriptID
	return events.DispatchEvent(ctx, event)
}

func findJavaScriptTarget(ctx context.Context, tools []runtimeport.Tool, name string) (runtimeport.Tool, *runtimeport.ToolInfo, error) {
	for _, tool := range tools {
		info, err := tool.Info(ctx)
		if err != nil {
			return nil, nil, err
		}
		if info != nil && info.Name == name {
			return tool, info, nil
		}
	}
	return nil, nil, fmt.Errorf("tool %s not found in group", name)
}
