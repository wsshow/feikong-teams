package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	javascripttool "fkteams/internal/adapters/tools/javascript"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/config"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type javaScriptToolTestRequest struct {
	Tool  config.JavaScriptTool `json:"tool"`
	Input map[string]any        `json:"input"`
}

type javaScriptTestNotice struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type javaScriptToolTestResponse struct {
	Result  any                    `json:"result"`
	Raw     string                 `json:"raw"`
	Notices []javaScriptTestNotice `json:"notices"`
}

// TestJavaScriptToolHandler 使用未保存的工具定义在真实受控宿主中试运行脚本。
func (rt *Runtime) TestJavaScriptToolHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rt == nil || rt.ToolRegistry == nil {
			Fail(c, http.StatusServiceUnavailable, "tool registry is not configured")
			return
		}
		releaseExecution := rt.ExecutionGate.Begin()
		defer releaseExecution()
		var req javaScriptToolTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 125*time.Second)
		defer cancel()
		ctx = appstate.WithState(ctx, state)
		ctx = hooks.WithBus(ctx, rt.HookBus)
		resp, err := testJavaScriptTool(ctx, rt.ToolRegistry, req)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		OK(c, resp)
	}
}

func testJavaScriptTool(ctx context.Context, registry *apptools.ToolGroupRegistry, req javaScriptToolTestRequest) (javaScriptToolTestResponse, error) {
	req.Tool.Enabled = true
	cfg := &config.Config{JavaScript: config.JavaScriptSettings{Tools: []config.JavaScriptTool{req.Tool}}}
	if err := cfg.ValidateJavaScript(); err != nil {
		return javaScriptToolTestResponse{}, err
	}
	if err := javascripttool.ValidateDefinitions(cfg.JavaScript.Tools); err != nil {
		return javaScriptToolTestResponse{}, err
	}
	if registry == nil {
		return javaScriptToolTestResponse{}, fmt.Errorf("tool registry is not configured")
	}
	notices := make([]javaScriptTestNotice, 0)
	ctx = events.WithCallback(ctx, func(event events.Event) error {
		if event.Type == events.EventSystemNotice && event.Notice != nil {
			notices = append(notices, javaScriptTestNotice{Level: event.Notice.Level, Message: event.Notice.Message})
		}
		return nil
	})
	ctx = apptools.WithResolveContextPatch(ctx, apptools.ToolResolveContext{Config: cfg})
	resolved, err := registry.GetToolsByName(ctx, "javascript")
	if err != nil {
		return javaScriptToolTestResponse{}, fmt.Errorf("initialize javascript test tool: %w", err)
	}
	target, err := findTestJavaScriptTool(ctx, resolved, req.Tool.ID)
	if err != nil {
		return javaScriptToolTestResponse{}, err
	}
	arguments, err := json.Marshal(req.Input)
	if err != nil {
		return javaScriptToolTestResponse{}, fmt.Errorf("encode javascript test input: %w", err)
	}
	result, err := target.Invoke(ctx, runtimeport.ToolInvocation{
		Name:      req.Tool.ID,
		CallID:    "javascript-test:" + uuid.NewString(),
		Arguments: string(arguments),
		Meta:      map[string]any{"source": "javascript_test"},
	})
	if err != nil {
		return javaScriptToolTestResponse{}, err
	}
	raw := ""
	if result != nil {
		raw = result.Content
	}
	return javaScriptToolTestResponse{Result: decodeJavaScriptTestResult(raw), Raw: raw, Notices: notices}, nil
}

func findTestJavaScriptTool(ctx context.Context, tools []runtimeport.Tool, name string) (runtimeport.Tool, error) {
	for _, tool := range tools {
		info, err := tool.Info(ctx)
		if err != nil {
			return nil, err
		}
		if info != nil && info.Name == name {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("javascript test tool %s not found", name)
}

func decodeJavaScriptTestResult(raw string) any {
	var result any
	if json.Unmarshal([]byte(raw), &result) == nil {
		return result
	}
	return raw
}
