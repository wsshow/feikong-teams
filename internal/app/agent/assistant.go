package agent

import (
	"context"
	"fmt"

	agents "fkteams/internal/app/agent/catalog"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/toolpolicy"
)

// AssistantCapability 描述已连接的成员能力；不可用项不暴露内部初始化错误。
type AssistantCapability struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	Tool              string `json:"tool,omitempty"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// CreateAssistantRunner 让个人助手使用原生团队成员能力，保持统一执行和审批链路。
func CreateAssistantRunner(ctx context.Context) (runtimeport.Runner, error) {
	registry, err := agents.RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	tools, err := assistantMemberTools(ctx, registry.List())
	if err != nil {
		return nil, err
	}
	assistant, err := agents.NewBuiltinAgent(ctx, domainsession.AssistantAgent, tools...)
	if err != nil {
		return nil, fmt.Errorf("create personal assistant: %w", err)
	}
	return newRunner(ctx, assistant)
}

func assistantMemberTools(ctx context.Context, infos []agents.AgentInfo) ([]runtimeport.Tool, error) {
	var members []runtimeport.Agent
	var capabilities []AssistantCapability
	var availableIndexes []int
	for _, info := range infos {
		if !info.Enabled || !info.TeamMember || info.Name == domainsession.AssistantAgent || info.Name == "coordinator" {
			continue
		}
		capability := AssistantCapability{Name: info.DisplayName, Description: info.Description}
		if capability.Name == "" {
			capability.Name = info.Name
		}
		if info.Creator == nil {
			capability.UnavailableReason = "成员未配置"
		} else {
			member, err := info.Creator(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				capability.UnavailableReason = "成员初始化失败，请检查该成员的模型、工具或连接配置"
			} else if member == nil {
				capability.UnavailableReason = "成员未提供可用执行器"
			} else {
				capability.Available = true
				availableIndexes = append(availableIndexes, len(capabilities))
				members = append(members, member)
			}
		}
		capabilities = append(capabilities, capability)
	}
	var tools []runtimeport.Tool
	if len(members) > 0 {
		var err error
		tools, err = buildAgentTools(ctx, members)
		if err != nil {
			return nil, err
		}
		for i, tool := range tools {
			info, err := tool.Info(ctx)
			if err != nil {
				return nil, err
			}
			capabilities[availableIndexes[i]].Tool = info.Name
		}
	}
	type request struct{}
	type response struct {
		Members []AssistantCapability `json:"members"`
	}
	catalog, err := runtimeport.NewTools([]runtimeport.ToolDefinition{{Name: "assistant_capabilities", Description: "查看当前已连接的专业成员及其实际调用工具。根据任务按需委派；不可用成员不能调用，说明缺少配置即可。普通任务直接处理，用户无须选择模式或角色。", Handler: func(context.Context, *request) (*response, error) { return &response{Members: capabilities}, nil }}})
	if err != nil {
		return nil, err
	}
	if err := toolpolicy.MarkPolicyRequired(catalog); err != nil {
		return nil, err
	}
	if err := toolpolicy.ClassifyTools(catalog); err != nil {
		return nil, err
	}
	return append(tools, catalog...), nil
}
