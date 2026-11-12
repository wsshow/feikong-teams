package agentkit

import (
	"context"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	kit "github.com/wsshow/agentkit"
)

// observedModel 补齐 AgentKit 公共事件未暴露的工具参数增量、消息 ID 和
// 多模态输出。观察的是同一次模型调用，工具调度和中断恢复仍由 AgentKit 执行。
type observedModel struct {
	*adk.BaseChatModelAgentMiddleware
	current   *modelObservation
	inner     kit.ChatModel
	converter *converter
	name      string
	streaming bool
}

func (c *converter) observeModel(inner kit.ChatModel, name string, streaming bool) *observedModel {
	return &observedModel{BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{}, inner: inner, converter: c, name: name, streaming: streaming}
}

func (m *observedModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	output, err := m.inner.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	observation, err := m.start()
	if err != nil {
		return nil, err
	}
	return observation.chunk(output)
}

func (m *observedModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if !m.streaming {
		output, err := m.Generate(ctx, input, opts...)
		if err != nil {
			return nil, err
		}
		return schema.StreamReaderFromArray([]*schema.Message{output}), nil
	}
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	observation, err := m.start()
	if err != nil {
		stream.Close()
		return nil, err
	}
	return schema.StreamReaderWithConvert(stream, observation.chunk), nil
}

type modelObservation struct {
	converter *converter
	meta      events.MessageEvent
	scope     MemberScope
	calls     map[int]domainmessage.ToolCall
	layout    domainmessage.OutputLayout
}

func (m *observedModel) start() (*modelObservation, error) {
	c := m.converter
	c.mu.Lock()
	defer c.mu.Unlock()
	o := &modelObservation{converter: c, scope: c.scopes[m.name], calls: make(map[int]domainmessage.ToolCall), meta: events.MessageEvent{
		MessageID: "msg_" + uuid.NewString(), AgentName: m.name, Role: domainmessage.RoleAssistant,
	}}
	m.current = o
	event := events.AssistantStarted(o.meta)
	o.scope.apply(&event, c)
	err := c.emitLocked(event)
	o.meta.MessageOrder = c.emitter.LastEvent().Sequence
	return o, err
}

func (o *modelObservation) chunk(chunk *schema.Message) (*schema.Message, error) {
	if chunk == nil {
		return nil, nil
	}
	c := o.converter
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := *chunk
	copy.ToolCalls = append([]schema.ToolCall(nil), chunk.ToolCalls...)
	delta := func(kind domainevent.DeltaKind, content string, call *domainmessage.ToolCall) error {
		if content == "" {
			return nil
		}
		meta := o.meta
		meta.DeltaKind = kind
		if call != nil {
			meta.ToolCallID, meta.ToolCallRef, meta.ToolName = call.ID, "tool_call:"+call.ID, call.Function.Name
		}
		event := events.AssistantDelta(meta, content)
		event.MessageOrder = o.meta.MessageOrder
		partType := "text"
		if kind == domainevent.DeltaReasoning {
			partType = "reasoning"
		}
		callID := ""
		if call != nil {
			partType, callID = "tool", call.ID
		}
		part := o.layout.Append(o.meta.MessageID, partType, content, callID)
		event.BlockID = part.ID
		if call != nil {
			event.ToolCallIndex = call.Index
		}
		o.scope.apply(&event, c)
		return c.emitLocked(event)
	}
	if err := delta(domainevent.DeltaReasoning, chunk.ReasoningContent, nil); err != nil {
		return nil, err
	}
	if err := delta(domainevent.DeltaOutput, chunk.Content, nil); err != nil {
		return nil, err
	}
	for i := range copy.ToolCalls {
		tc := &copy.ToolCalls[i]
		index := i
		if tc.Index != nil {
			index = *tc.Index
		} else {
			tc.Index = &index
		}
		current, exists := o.calls[index]
		if !exists {
			current = adaptToolCallFromRunner(*tc)
			current.Function.Arguments = ""
			c.identities.ensure(o.meta.MessageID, index, o.scope, &current)
		}
		// 第一个参数分片就确定 ID，后续分片不能改变协议引用或执行 ID。
		tc.ID = current.ID
		if exists {
			tc.ID = ""
		}
		if tc.Function.Name != "" {
			current.Function.Name = tc.Function.Name
		}
		current.Function.Arguments += tc.Function.Arguments
		o.calls[index] = current
		if !events.IsInternalToolName(current.Function.Name) {
			if err := delta(domainevent.DeltaToolArgs, tc.Function.Arguments, &current); err != nil {
				return nil, err
			}
		}
	}
	return &copy, nil
}

// BeforeAgent 将成员归属注入工具审批上下文。
func (m *observedModel) BeforeAgent(ctx context.Context, state *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	scope := m.converter.memberScope(m.name)
	if scope.CallID != "" {
		ctx = runtimeport.WithInterruptMetadata(ctx, runtimeport.InterruptMetadata{
			MemberCallID: scope.CallID, MemberToolName: scope.ToolName, MemberName: scope.Name,
		})
	}
	return ctx, state, nil
}

// AfterModelRewriteState 在产品中间件完成修复后发布完整消息，不保留 SDK 会修改的流分片。
func (m *observedModel) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	c := m.converter
	c.mu.Lock()
	defer c.mu.Unlock()
	observation := m.current
	m.current = nil
	if observation == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	return ctx, state, observation.finishLocked(state.Messages[len(state.Messages)-1])
}

func (o *modelObservation) finishLocked(output *schema.Message) error {
	if output == nil {
		return nil
	}
	c := o.converter
	message := adaptMessageFromRunner(output)
	meta := o.meta
	meta.Message, meta.Content, meta.ReasoningContent = &message, message.Content, message.ReasoningContent
	meta.ToolCalls, meta.ToolCallRefs = message.ToolCalls, c.identities.refsFor(message.ToolCalls)
	meta.OutputParts = o.layout.Complete(meta.MessageID, message)
	end := events.AssistantCompleted(meta)
	if output.ResponseMeta != nil && output.ResponseMeta.Usage != nil {
		usage := output.ResponseMeta.Usage
		end.PromptTokens, end.CompletionTokens, end.TotalTokens = usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens
		end.Usage = &domainevent.UsagePayload{
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
			CachedTokens: usage.PromptTokenDetails.CachedTokens, CacheWriteTokens: usage.PromptTokenDetails.CacheWriteTokens,
		}
	}
	o.scope.apply(&end, c)
	if err := c.emitLocked(end); err != nil {
		return err
	}
	for _, tc := range message.ToolCalls {
		if events.IsInternalToolName(tc.Function.Name) {
			continue
		}
		c.identities.rememberResult(tc.Function.Name, tc.ID, o.scope)
		start := events.ToolCallStarted(events.ToolEvent{AgentName: meta.AgentName, ToolCallID: tc.ID, ToolCallRef: "tool_call:" + tc.ID, ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments, ToolCall: &tc, ToolCallIndex: tc.Index})
		start.MessageID, start.MessageOrder = meta.MessageID, meta.MessageOrder
		c.identities.ownersByID[tc.ID] = meta.MessageID
		o.scope.apply(&start, c)
		if err := c.emitLocked(start); err != nil {
			return err
		}
		if name, ok := c.members[tc.Function.Name]; ok {
			c.scopes[tc.Function.Name] = MemberScope{CallID: tc.ID, ToolName: tc.Function.Name, Name: name}
		}
	}
	return nil
}

func (c *converter) toolPolicy(policy *kit.ToolPolicy, name string) *kit.ToolPolicy {
	if policy == nil {
		return nil
	}
	result := *policy
	if policy.UnknownTool != nil {
		result.UnknownTool = func(ctx context.Context, toolName, arguments string) (string, error) {
			output, err := policy.UnknownTool(ctx, toolName, arguments)
			if err == nil {
				c.consume(kit.Event{Type: kit.EventToolEnd, Agent: name, ToolCallID: compose.GetToolCallID(ctx), ToolName: toolName, ToolArguments: arguments, Content: output})
			}
			return output, err
		}
	}
	return &result
}
