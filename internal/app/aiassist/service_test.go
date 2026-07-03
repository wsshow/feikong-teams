package aiassist

import (
	"context"
	"strings"
	"testing"

	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/testmodel"
)

func TestGenerateAgentsNormalizesDrafts(t *testing.T) {
	model := testmodel.New(domainmessage.Message{
		Role: domainmessage.RoleAssistant,
		Content: `{
  "agents": [
    {
      "id": "Research Agent",
      "name": "资料研究员",
      "description": "检索并整理资料",
      "prompt": "你是资料研究员。",
      "tools": ["fetch", "unknown", "fetch"],
      "enabled": false
    }
  ]
}`,
	})
	service := New(model)

	got, err := service.GenerateAgents(context.Background(), AgentDraftRequest{
		Instruction:     "创建研究智能体",
		ExistingAgents:  []string{"research_agent"},
		AvailableTools:  []string{"fetch"},
		AvailableModels: []string{"default-model"},
	})
	if err != nil {
		t.Fatalf("GenerateAgents returned error: %v", err)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("agent count = %d, want 1", len(got.Agents))
	}
	agent := got.Agents[0]
	if agent.ID != "research_agent_2" {
		t.Fatalf("id = %q, want research_agent_2", agent.ID)
	}
	if agent.ModelID != "default-model" {
		t.Fatalf("model id = %q, want default-model", agent.ModelID)
	}
	if len(agent.Tools) != 1 || agent.Tools[0] != "fetch" {
		t.Fatalf("tools = %#v, want only fetch", agent.Tools)
	}
	if !agent.Enabled {
		t.Fatal("agent should be enabled")
	}
}

func TestRewriteTextDecodesFencedJSON(t *testing.T) {
	model := testmodel.New(domainmessage.Message{
		Role:    domainmessage.RoleAssistant,
		Content: "```json\n{\"text\":\"改写后的提示词\"}\n```",
	})
	service := New(model)

	got, err := service.RewriteText(context.Background(), RewriteTextRequest{
		Scenario:    "agent_prompt",
		Instruction: "更简洁",
		Text:        "旧提示词",
	})
	if err != nil {
		t.Fatalf("RewriteText returned error: %v", err)
	}
	if got.Text != "改写后的提示词" {
		t.Fatalf("text = %q, want rewritten text", got.Text)
	}
}

func TestGenerateAgentsSendsInstructionToModel(t *testing.T) {
	model := testmodel.New(domainmessage.Message{Role: domainmessage.RoleAssistant, Content: `{"agents":[{"name":"A"}]}`})
	service := New(model)

	_, err := service.GenerateAgents(context.Background(), AgentDraftRequest{Instruction: "创建 A"})
	if err != nil {
		t.Fatalf("GenerateAgents returned error: %v", err)
	}
	calls := model.GenerateCalls()
	if len(calls) != 1 {
		t.Fatalf("generate calls = %d, want 1", len(calls))
	}
	if len(calls[0].Input) != 2 {
		t.Fatalf("input count = %d, want 2", len(calls[0].Input))
	}
	if !strings.Contains(calls[0].Input[1].Content, "创建 A") {
		t.Fatalf("input payload = %q, want instruction", calls[0].Input[1].Content)
	}
}
