package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type ToolInfo struct {
	Name   string
	Desc   string
	Policy ToolPolicyMetadata
	Extra  map[string]any
}

type ToolPolicyMetadata struct {
	Required      bool
	ReadOnly      bool
	Destructive   bool
	Serialize     bool
	ApprovalStore string
	ExternalPath  bool
}

type ToolInvocation struct {
	Name      string
	CallID    string
	Arguments string
	Meta      map[string]any
}

type toolRuntimeMetadataKey struct{}

type ToolRuntimeMetadata struct {
	CallID string
	Name   string
}

func WithToolRuntimeMetadata(ctx context.Context, metadata ToolRuntimeMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolRuntimeMetadataKey{}, metadata)
}

func ToolRuntimeMetadataFromContext(ctx context.Context) (ToolRuntimeMetadata, bool) {
	if ctx == nil {
		return ToolRuntimeMetadata{}, false
	}
	metadata, ok := ctx.Value(toolRuntimeMetadataKey{}).(ToolRuntimeMetadata)
	return metadata, ok
}

type ToolResult struct {
	Content string
}

type Tool interface {
	Info(ctx context.Context) (*ToolInfo, error)
	Invoke(ctx context.Context, invocation ToolInvocation) (*ToolResult, error)
}

type ToolInputTypeProvider interface {
	InputType() reflect.Type
}

// ToolInputSchemaProvider 允许动态工具直接提供 JSON Schema 参数契约。
type ToolInputSchemaProvider interface {
	InputSchema() map[string]any
}

// ToolDefinition 声明工具元信息与处理函数，供批量构建复用。
type ToolDefinition struct {
	Name        string
	Description string
	Handler     any
	Policy      ToolPolicyMetadata
}

// NewTools 按声明顺序构建工具；名称重复或任一声明无效时返回错误，不返回部分结果。
func NewTools(definitions []ToolDefinition) ([]Tool, error) {
	tools := make([]Tool, 0, len(definitions))
	seen := make(map[string]bool, len(definitions))
	for _, def := range definitions {
		if strings.TrimSpace(def.Name) == "" || seen[def.Name] {
			return nil, fmt.Errorf("empty or duplicate tool name: %q", def.Name)
		}
		seen[def.Name] = true
		tool, err := NewTool(ToolInfo{Name: def.Name, Desc: def.Description, Policy: def.Policy}, def.Handler)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

type functionTool struct {
	info      ToolInfo
	handler   reflect.Value
	inputType reflect.Type
}

func InferTool(name, desc string, handler any) (Tool, error) {
	return NewTool(ToolInfo{Name: name, Desc: desc}, handler)
}

func NewTool(info ToolInfo, handler any) (Tool, error) {
	handlerValue, inputType, err := validateFunctionToolHandler(info.Name, handler)
	if err != nil {
		return nil, err
	}
	return &functionTool{info: info, handler: handlerValue, inputType: inputType}, nil
}

func (t *functionTool) Info(context.Context) (*ToolInfo, error) {
	return &t.info, nil
}

func (t *functionTool) InputType() reflect.Type {
	return t.inputType
}

func (t *functionTool) Invoke(ctx context.Context, invocation ToolInvocation) (*ToolResult, error) {
	content, err := invokeFunctionToolHandler(ctx, t.handler, t.inputType, invocation.Arguments)
	if err != nil {
		return nil, err
	}
	return &ToolResult{Content: content}, nil
}

func validateFunctionToolHandler(name string, handler any) (reflect.Value, reflect.Type, error) {
	if handler == nil {
		return reflect.Value{}, nil, fmt.Errorf("tool %s handler is nil", name)
	}
	handlerValue := reflect.ValueOf(handler)
	handlerType := handlerValue.Type()
	if handlerType.Kind() != reflect.Func || handlerValue.IsNil() || handlerType.NumIn() != 2 || handlerType.NumOut() != 2 {
		return reflect.Value{}, nil, fmt.Errorf("tool %s handler must be func(context.Context, *Input) (Output, error)", name)
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !handlerType.In(0).Implements(contextType) {
		return reflect.Value{}, nil, fmt.Errorf("tool %s first argument must be context.Context", name)
	}
	inputType := handlerType.In(1)
	if inputType.Kind() != reflect.Pointer || inputType.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("tool %s second argument must be pointer to struct", name)
	}
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if !handlerType.Out(1).Implements(errorType) {
		return reflect.Value{}, nil, fmt.Errorf("tool %s second return value must be error", name)
	}
	return handlerValue, inputType, nil
}

func invokeFunctionToolHandler(ctx context.Context, handler reflect.Value, inputType reflect.Type, arguments string) (string, error) {
	input := reflect.New(inputType.Elem())
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), input.Interface()); err != nil {
			return "", err
		}
	}
	out := handler.Call([]reflect.Value{reflect.ValueOf(ctx), input})
	if !out[1].IsNil() {
		return "", out[1].Interface().(error)
	}
	if isNilableToolValue(out[0].Kind()) && out[0].IsNil() {
		return "", nil
	}
	if text, ok := out[0].Interface().(string); ok {
		return text, nil
	}
	data, err := json.Marshal(out[0].Interface())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isNilableToolValue(kind reflect.Kind) bool {
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}
