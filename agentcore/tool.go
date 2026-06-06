package agentcore

import "context"

type ToolInfo struct {
	Name  string
	Desc  string
	Extra map[string]any
}

type Tool interface {
	Info(ctx context.Context) (*ToolInfo, error)
	Handler() any
}

type functionTool struct {
	info    ToolInfo
	handler any
}

func InferTool(name, desc string, handler any) (Tool, error) {
	return NewTool(ToolInfo{Name: name, Desc: desc}, handler), nil
}

func NewTool(info ToolInfo, handler any) Tool {
	return &functionTool{info: info, handler: handler}
}

func (t *functionTool) Info(context.Context) (*ToolInfo, error) {
	info := t.info
	if info.Extra == nil {
		info.Extra = make(map[string]any)
	}
	t.info = info
	return &t.info, nil
}

func (t *functionTool) Handler() any {
	return t.handler
}
