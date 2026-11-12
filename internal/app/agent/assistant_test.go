package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agents "fkteams/internal/app/agent/catalog"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestAssistantCapabilitiesReflectActualMembers(t *testing.T) {
	engine := &runnerTestEngine{}
	ctx := runtimeport.WithRuntime(context.Background(), engine)
	built := 0
	creator := func(context.Context) (runtimeport.Agent, error) { built++; return runnerTestAgent{name: "writer"}, nil }
	infos := []agents.AgentInfo{
		{Name: "writer", DisplayName: "写作", Description: "写作与编辑", Enabled: true, TeamMember: true, Creator: creator},
		{Name: "unavailable", Enabled: true, TeamMember: true, Creator: func(context.Context) (runtimeport.Agent, error) {
			return nil, errors.New("private initialization details")
		}},
		{Name: "disabled", Enabled: false, TeamMember: true, Creator: creator},
		{Name: domainsession.AssistantAgent, Enabled: true, TeamMember: true, Creator: creator},
		{Name: "coordinator", Enabled: true, TeamMember: true, Creator: creator},
	}
	tools, err := assistantMemberTools(ctx, infos)
	if err != nil {
		t.Fatal(err)
	}
	if built != 1 || len(tools) != 2 {
		t.Fatalf("built=%d tools=%d", built, len(tools))
	}
	catalog := tools[1]
	info, err := catalog.Info(ctx)
	if err != nil || !info.Policy.ReadOnly {
		t.Fatal("catalog policy", err)
	}
	result, err := catalog.Invoke(ctx, runtimeport.ToolInvocation{Arguments: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "private initialization") {
		t.Fatal("initialization details leaked")
	}
	var body struct {
		Members []AssistantCapability `json:"members"`
	}
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Members) != 2 || !body.Members[0].Available || body.Members[0].Tool != engine.toolNames[0] || body.Members[1].Available {
		t.Fatalf("capabilities: %+v", body)
	}
}
func TestAssistantWithoutMembersStillHasCapabilityCatalog(t *testing.T) {
	tools, err := assistantMemberTools(context.Background(), nil)
	if err != nil || len(tools) != 1 {
		t.Fatalf("tools=%d err=%v", len(tools), err)
	}
}
