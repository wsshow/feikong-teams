package eino

import (
	"context"
	"encoding/json"
	"fkteams/agentcore"
	"fmt"
	"reflect"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

func AdaptToolsForRunner(ctx context.Context, tools []agentcore.Tool) ([]tool.BaseTool, error) {
	result := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		if runtimeTool, ok := t.(interface{ runnerTool() tool.BaseTool }); ok {
			if runtimeTool.runnerTool() == nil {
				return nil, fmt.Errorf("tool is nil")
			}
			result = append(result, runtimeTool.runnerTool())
			continue
		}
		info, err := t.Info(ctx)
		if err != nil {
			return nil, err
		}
		baseTool, err := newReflectedTool(info, t.Handler())
		if err != nil {
			return nil, err
		}
		if info.Extra != nil {
			runnerInfo, err := baseTool.Info(ctx)
			if err != nil {
				return nil, err
			}
			if runnerInfo.Extra == nil {
				runnerInfo.Extra = make(map[string]any)
			}
			for key, value := range info.Extra {
				runnerInfo.Extra[key] = value
			}
		}
		result = append(result, baseTool)
	}
	return result, nil
}

type runtimeTool struct {
	inner tool.BaseTool
}

func WrapTool(inner tool.BaseTool) agentcore.Tool {
	return &runtimeTool{inner: inner}
}

func (t *runtimeTool) Info(ctx context.Context) (*agentcore.ToolInfo, error) {
	if t == nil || t.inner == nil {
		return nil, fmt.Errorf("tool is nil")
	}
	info, err := t.inner.Info(ctx)
	if err != nil {
		return nil, err
	}
	return &agentcore.ToolInfo{Name: info.Name, Desc: info.Desc, Extra: info.Extra}, nil
}

func (t *runtimeTool) Handler() any {
	return nil
}

func (t *runtimeTool) runnerTool() tool.BaseTool {
	if t == nil {
		return nil
	}
	return t.inner
}

type reflectedTool struct {
	info      *schema.ToolInfo
	handler   reflect.Value
	inputType reflect.Type
}

func newReflectedTool(info *agentcore.ToolInfo, handler any) (tool.InvokableTool, error) {
	if handler == nil {
		return nil, fmt.Errorf("tool %s handler is nil", info.Name)
	}
	handlerValue := reflect.ValueOf(handler)
	handlerType := handlerValue.Type()
	if handlerType.Kind() != reflect.Func || handlerType.NumIn() != 2 || handlerType.NumOut() != 2 {
		return nil, fmt.Errorf("tool %s handler must be func(context.Context, *Input) (*Output, error)", info.Name)
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !handlerType.In(0).Implements(contextType) {
		return nil, fmt.Errorf("tool %s first argument must be context.Context", info.Name)
	}
	inputType := handlerType.In(1)
	if inputType.Kind() != reflect.Pointer || inputType.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("tool %s second argument must be pointer to struct", info.Name)
	}
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if !handlerType.Out(1).Implements(errorType) {
		return nil, fmt.Errorf("tool %s second return value must be error", info.Name)
	}
	toolInfo := &schema.ToolInfo{
		Name:        info.Name,
		Desc:        info.Desc,
		Extra:       info.Extra,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(schemaForType(inputType.Elem())),
	}
	return &reflectedTool{info: toolInfo, handler: handlerValue, inputType: inputType}, nil
}

func (t *reflectedTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *reflectedTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	input := reflect.New(t.inputType.Elem())
	if strings.TrimSpace(argumentsInJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), input.Interface()); err != nil {
			return "", err
		}
	}
	out := t.handler.Call([]reflect.Value{reflect.ValueOf(ctx), input})
	if !out[1].IsNil() {
		return "", out[1].Interface().(error)
	}
	if isNilable(out[0].Kind()) && out[0].IsNil() {
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

func isNilable(kind reflect.Kind) bool {
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}

func schemaForType(t reflect.Type) *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()
	required := make([]string, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, omitempty := jsonFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		fieldSchema := schemaForField(field.Type)
		tagInfo := parseJSONSchemaTag(field.Tag.Get("jsonschema"))
		fieldSchema.Description = tagInfo.description
		properties.Set(name, fieldSchema)
		if !omitempty || tagInfo.required {
			required = append(required, name)
		}
	}
	return &jsonschema.Schema{
		Type:       string(schema.Object),
		Required:   required,
		Properties: properties,
	}
}

func schemaForField(t reflect.Type) *jsonschema.Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return &jsonschema.Schema{Type: string(schema.Boolean)}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return &jsonschema.Schema{Type: string(schema.Number)}
	case reflect.Slice, reflect.Array:
		return &jsonschema.Schema{Type: string(schema.Array), Items: schemaForField(t.Elem())}
	case reflect.Struct:
		return schemaForType(t)
	default:
		return &jsonschema.Schema{Type: string(schema.String)}
	}
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name, false
	}
	parts := strings.Split(tag, ",")
	omitempty := false
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitempty = true
			break
		}
	}
	return parts[0], omitempty
}

type fieldSchemaTag struct {
	description string
	required    bool
}

func parseJSONSchemaTag(tag string) fieldSchemaTag {
	var result fieldSchemaTag
	if tag == "" {
		return result
	}
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == "required" {
			result.required = true
		}
	}
	for _, prefix := range []string{"description=", "description:"} {
		idx := strings.Index(tag, prefix)
		if idx < 0 {
			continue
		}
		description := tag[idx+len(prefix):]
		if cut := strings.Index(description, ",required"); cut >= 0 {
			description = description[:cut]
		}
		result.description = strings.TrimSpace(strings.Trim(description, ","))
		break
	}
	return result
}
