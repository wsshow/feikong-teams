package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	runtimeport "fkteams/internal/ports/runtime"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

func AdaptToolsForRunner(ctx context.Context, tools []runtimeport.Tool) ([]tool.BaseTool, error) {
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
		if info == nil {
			return nil, fmt.Errorf("tool info is nil")
		}
		baseTool, err := newCoreTool(info, t)
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

func WrapTool(inner tool.BaseTool) runtimeport.Tool {
	return &runtimeTool{inner: inner}
}

func (t *runtimeTool) Info(ctx context.Context) (*runtimeport.ToolInfo, error) {
	if t == nil || t.inner == nil {
		return nil, fmt.Errorf("tool is nil")
	}
	info, err := t.inner.Info(ctx)
	if err != nil {
		return nil, err
	}
	return &runtimeport.ToolInfo{Name: info.Name, Desc: info.Desc, Extra: info.Extra}, nil
}

func (t *runtimeTool) Invoke(ctx context.Context, invocation runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	if t == nil || t.inner == nil {
		return nil, fmt.Errorf("tool is nil")
	}
	invokable, ok := t.inner.(tool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("tool is not invokable")
	}
	result, err := invokable.InvokableRun(ctx, invocation.Arguments)
	if err != nil {
		return nil, err
	}
	return &runtimeport.ToolResult{Content: result}, nil
}

func (t *runtimeTool) runnerTool() tool.BaseTool {
	if t == nil {
		return nil
	}
	return t.inner
}

type reflectedTool struct {
	info      *schema.ToolInfo
	inputType reflect.Type
	inner     runtimeport.Tool
}

func newCoreTool(info *runtimeport.ToolInfo, inner runtimeport.Tool) (tool.InvokableTool, error) {
	if provider, ok := inner.(runtimeport.ToolInputSchemaProvider); ok {
		inputSchema, err := dynamicInputSchema(provider.InputSchema())
		if err != nil {
			return nil, fmt.Errorf("tool %s input schema: %w", info.Name, err)
		}
		toolInfo := &schema.ToolInfo{
			Name:        info.Name,
			Desc:        info.Desc,
			Extra:       info.Extra,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(inputSchema),
		}
		return &reflectedTool{info: toolInfo, inner: inner}, nil
	}
	inputType := reflect.TypeOf(struct{}{})
	if provider, ok := inner.(runtimeport.ToolInputTypeProvider); ok {
		inputType = provider.InputType()
		if inputType == nil {
			return nil, fmt.Errorf("tool %s input type is nil", info.Name)
		}
		if inputType.Kind() != reflect.Pointer || inputType.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("tool %s input type must be pointer to struct", info.Name)
		}
		inputType = inputType.Elem()
	}
	toolInfo := &schema.ToolInfo{
		Name:        info.Name,
		Desc:        info.Desc,
		Extra:       info.Extra,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(schemaForType(inputType)),
	}
	return &reflectedTool{info: toolInfo, inputType: inputType, inner: inner}, nil
}

func dynamicInputSchema(value map[string]any) (*jsonschema.Schema, error) {
	if value == nil {
		value = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result jsonschema.Schema
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result.Type != string(schema.Object) {
		return nil, fmt.Errorf("root type must be object")
	}
	return &result, nil
}

func (t *reflectedTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *reflectedTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	callID := compose.GetToolCallID(ctx)
	ctx = runtimeport.WithToolRuntimeMetadata(ctx, runtimeport.ToolRuntimeMetadata{
		CallID: callID,
		Name:   t.info.Name,
	})
	result, err := t.inner.Invoke(ctx, runtimeport.ToolInvocation{
		Name:      t.info.Name,
		CallID:    callID,
		Arguments: argumentsInJSON,
	})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return result.Content, nil
}

func schemaForType(t reflect.Type) *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()
	required := make([]string, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		embedded := field.Type
		for embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if field.Anonymous && field.Tag.Get("json") == "" && embedded.Kind() == reflect.Struct {
			flattened := schemaForType(embedded)
			for pair := flattened.Properties.Oldest(); pair != nil; pair = pair.Next() {
				properties.Set(pair.Key, pair.Value)
			}
			required = append(required, flattened.Required...)
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
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &jsonschema.Schema{Type: string(schema.Integer)}
	case reflect.Float32, reflect.Float64:
		return &jsonschema.Schema{Type: string(schema.Number)}
	case reflect.Slice, reflect.Array:
		return &jsonschema.Schema{Type: string(schema.Array), Items: schemaForField(t.Elem())}
	case reflect.Struct:
		return schemaForType(t)
	case reflect.Map:
		return &jsonschema.Schema{Type: string(schema.Object), AdditionalProperties: schemaForField(t.Elem())}
	case reflect.Interface:
		return &jsonschema.Schema{}
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
