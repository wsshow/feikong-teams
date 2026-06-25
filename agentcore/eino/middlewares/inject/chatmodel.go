// Package inject 提供 ChatModel 包装器，在每次模型调用前注入动态上下文。
// 注入的消息仅存在于当次 API 请求，不会污染界面、历史记录或缓存。
package inject

import (
	"context"
	"fkteams/agentcore"
	einoruntime "fkteams/agentcore/eino"
	rootcommon "fkteams/common"
	"fkteams/common/typeutil"
	"fkteams/hooks"
	"fmt"
	"reflect"
	"runtime"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// injectChatModel 包装 ToolCallingChatModel，在调用前注入动态系统上下文。
type injectChatModel struct {
	inner                 model.ToolCallingChatModel
	innerHandlesCallbacks bool
}

// New 创建注入包装器
func New(inner model.ToolCallingChatModel) model.ToolCallingChatModel {
	innerHandlesCallbacks := false
	if ch, ok := inner.(components.Checker); ok {
		innerHandlesCallbacks = ch.IsCallbacksEnabled()
	}
	return &injectChatModel{inner: inner, innerHandlesCallbacks: innerHandlesCallbacks}
}

func NewForModel(inner agentcore.ChatModel) (agentcore.ChatModel, error) {
	runnerModel, err := einoruntime.AdaptChatModelForRunner(inner)
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(New(runnerModel)), nil
}

func (m *injectChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	newInner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	innerHandlesCallbacks := false
	if ch, ok := newInner.(components.Checker); ok {
		innerHandlesCallbacks = ch.IsCallbacksEnabled()
	}
	return &injectChatModel{inner: newInner, innerHandlesCallbacks: innerHandlesCallbacks}, nil
}

func (m *injectChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	enriched := m.injectDynamicContext(input)
	var hookErr error
	enriched, hookErr = invokeBeforeModelRequest(ctx, enriched)
	if hookErr != nil {
		return nil, hookErr
	}

	var out *schema.Message
	var err error
	if m.innerHandlesCallbacks {
		out, err = m.inner.Generate(ctx, enriched, opts...)
	} else {
		out, err = m.generateWithProxyCallbacks(ctx, enriched, opts...)
	}
	if hookErr := invokeAfterModelResponse(ctx, out, err); hookErr != nil && err == nil {
		err = hookErr
	}
	return out, err
}

func (m *injectChatModel) generateWithProxyCallbacks(ctx context.Context,
	input []*schema.Message, opts ...model.Option) (*schema.Message, error) {

	ctx = callbacks.EnsureRunInfo(ctx, m.GetType(), components.ComponentOfChatModel)
	nCtx := callbacks.OnStart(ctx, &model.CallbackInput{Messages: input})

	out, err := m.inner.Generate(nCtx, input, opts...)
	if err != nil {
		callbacks.OnError(nCtx, err)
		return nil, err
	}

	callbacks.OnEnd(nCtx, &model.CallbackOutput{Message: out})
	return out, nil
}

func (m *injectChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
	*schema.StreamReader[*schema.Message], error) {

	enriched := m.injectDynamicContext(input)
	var hookErr error
	enriched, hookErr = invokeBeforeModelRequest(ctx, enriched)
	if hookErr != nil {
		return nil, hookErr
	}

	var stream *schema.StreamReader[*schema.Message]
	var err error
	if m.innerHandlesCallbacks {
		stream, err = m.inner.Stream(ctx, enriched, opts...)
	} else {
		stream, err = m.streamWithProxyCallbacks(ctx, enriched, opts...)
	}
	if hookErr := invokeAfterModelResponse(ctx, nil, err); hookErr != nil && err == nil {
		err = hookErr
	}
	return stream, err
}

func (m *injectChatModel) streamWithProxyCallbacks(ctx context.Context,
	input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {

	ctx = callbacks.EnsureRunInfo(ctx, m.GetType(), components.ComponentOfChatModel)
	nCtx := callbacks.OnStart(ctx, &model.CallbackInput{Messages: input})

	stream, err := m.inner.Stream(nCtx, input, opts...)
	if err != nil {
		callbacks.OnError(nCtx, err)
		return nil, err
	}

	out := schema.StreamReaderWithConvert(stream, func(m *schema.Message) (*model.CallbackOutput, error) {
		return &model.CallbackOutput{Message: m}, nil
	})
	_, out = callbacks.OnEndWithStreamOutput(nCtx, out)
	return schema.StreamReaderWithConvert(out, func(o *model.CallbackOutput) (*schema.Message, error) {
		return o.Message, nil
	}), nil
}

func (m *injectChatModel) GetType() string {
	if gt, ok := m.inner.(components.Typer); ok {
		return gt.GetType()
	}
	return typeutil.ParseTypeName(reflect.ValueOf(m.inner))
}

func (m *injectChatModel) IsCallbacksEnabled() bool { return true }

// injectDynamicContext 向消息列表末尾注入一条临时用户消息，包含动态上下文。
// 放在末尾不破坏前缀缓存（静态 system prompt 在最前）；作为独立 UserMessage
// 便于后续扩展（操作系统、环境变量等），新增内容只需追加到 buildDynamicContext。
func (m *injectChatModel) injectDynamicContext(input []*schema.Message) []*schema.Message {
	dynamicContext := buildDynamicContext()
	if dynamicContext == "" {
		return input
	}
	enriched := make([]*schema.Message, len(input), len(input)+1)
	copy(enriched, input)
	enriched = append(enriched, schema.UserMessage(dynamicContext))
	return enriched
}

// buildDynamicContext 构建动态上下文文本，后续可扩展更多信息。
func buildDynamicContext() string {
	contextMsg := fmt.Sprintf(`<system-reminder>
在回答用户问题时，你可以参考以下背景信息：
- 当前时间：%s
- 操作系统：%s (%s)
- 工作目录：{%s}
重要提示：此背景信息可能与你的任务相关，也可能不相关。除非与任务高度相关，否则你不应针对此背景信息进行回应。
</system-reminder>`,
		time.Now().Format("2006-01-02 15:04:05"),
		runtime.GOOS,
		runtime.GOARCH,
		rootcommon.WorkspaceDir(),
	)
	return contextMsg
}

func invokeBeforeModelRequest(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
	messages := einoruntime.AdaptMessagesFromRunner(input)
	messages, err := hooks.FromContext(ctx).InvokeBeforeModelRequest(ctx, messages)
	if err != nil {
		return input, err
	}
	return einoruntime.AdaptMessagesForRunner(messages), nil
}

func invokeAfterModelResponse(ctx context.Context, output *schema.Message, modelErr error) error {
	var message agentcore.Message
	if output != nil {
		message = einoruntime.AdaptMessageFromRunner(output)
	}
	return hooks.FromContext(ctx).InvokeAfterModelResponse(ctx, hooks.AfterModelResponsePayload{
		Message: message,
		Error:   modelErr,
	})
}
