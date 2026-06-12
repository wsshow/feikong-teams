package eino

import (
	"context"
	"fkteams/agentcore"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

func NewAgentTools(ctx context.Context, subAgents []agentcore.Agent, cfg agentcore.AgentToolConfig) ([]agentcore.Tool, error) {
	runnerAgents, err := AdaptAgentsForRunner(subAgents)
	if err != nil {
		return nil, err
	}
	agentTools := make([]agentcore.Tool, 0, len(runnerAgents))
	for i, subAgent := range runnerAgents {
		displayName := subAgent.Name(ctx)
		toolName := displayName
		if cfg.ToolName != nil {
			toolName = cfg.ToolName(displayName, i)
		}
		wrapped := &agentToolNameAgent{
			inner:       WrapErrorSafe(subAgent),
			toolName:    toolName,
			displayName: displayName,
		}
		if cfg.RegisterDisplay != nil {
			cfg.RegisterDisplay(wrapped.toolName, displayName)
		}
		agentTools = append(agentTools, WrapTool(adk.NewAgentTool(ctx, wrapped)))
	}
	return agentTools, nil
}

type agentToolNameAgent struct {
	inner       adk.Agent
	toolName    string
	displayName string
}

func (a *agentToolNameAgent) Name(context.Context) string {
	return a.toolName
}

func (a *agentToolNameAgent) Description(ctx context.Context) string {
	desc := a.inner.Description(ctx)
	if desc == "" {
		return fmt.Sprintf("指派给 %s 处理一个独立子任务。", a.displayName)
	}
	return fmt.Sprintf(`指派给 %s 处理一个独立子任务。

使用原则：
- 仅当该成员能力明显匹配，或任务需要并行/专业工具/独立视角时调用。
- request 中写清任务目标、必要上下文、期望输出和完成标准。
- 派发后等待其结果，不要同时重复执行同一子任务。
- 成员最终消息只返回给 coordinator，不直接展示给用户；你需要阅读、筛选并整合。

能力描述：%s`, a.displayName, desc)
}

func (a *agentToolNameAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	ctx, scope := a.contextWithMemberScope(ctx)
	innerIter := a.inner.Run(ctx, input, opts...)
	return a.wrapMemberEvents(innerIter, scope)
}

func (a *agentToolNameAgent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	inner, ok := a.inner.(adk.ResumableAgent)
	if !ok {
		return newErrorAgentIterator(fmt.Errorf("agent %q does not support resume", a.displayName))
	}
	ctx, scope := a.contextWithMemberScope(ctx)
	innerIter := inner.Resume(ctx, info, opts...)
	return a.wrapMemberEvents(innerIter, scope)
}

func (a *agentToolNameAgent) contextWithMemberScope(ctx context.Context) (context.Context, MemberScope) {
	scope := MemberScope{
		CallID:   compose.GetToolCallID(ctx),
		ToolName: a.toolName,
		Name:     a.displayName,
	}
	if scope.CallID != "" {
		ctx = agentcore.WithInterruptMetadata(ctx, agentcore.InterruptMetadata{
			MemberCallID:   scope.CallID,
			MemberToolName: scope.ToolName,
			MemberName:     scope.Name,
		})
	}
	return ctx, scope
}

func (a *agentToolNameAgent) wrapMemberEvents(innerIter *adk.AsyncIterator[*adk.AgentEvent], scope MemberScope) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	go func() {
		defer gen.Close()
		for {
			event, ok := innerIter.Next()
			if !ok {
				return
			}
			RegisterAgentEventScope(event, scope)
			gen.Send(event)
		}
	}()

	return iter
}
