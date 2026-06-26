package agents

import (
	"context"
	"fmt"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
)

func TestGetRegistryReturnsCopy(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{Name: "coder", Description: "code"},
		{Name: "researcher", Description: "research"},
	})

	got := registry.List()
	if len(got) != 2 {
		t.Fatalf("registry length = %d, want 2", len(got))
	}
	got[0].Name = "mutated"

	again := registry.List()
	if again[0].Name != "coder" {
		t.Fatalf("registry was mutated through returned slice: %v", again)
	}
}

func TestGetAgentByNameFindsNameAndAlias(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{Name: "coder", Aliases: []string{"小码"}},
		{Name: "researcher", Aliases: []string{"小搜"}},
	})

	if got := registry.AgentByName("coder"); got == nil || got.Name != "coder" {
		t.Fatalf("GetAgentByName(coder) = %#v", got)
	}
	if got := registry.AgentByName("小搜"); got == nil || got.Name != "researcher" {
		t.Fatalf("GetAgentByName(alias) = %#v", got)
	}
	if got := registry.AgentByName("missing"); got != nil {
		t.Fatalf("GetAgentByName(missing) = %#v, want nil", got)
	}
}

func TestGetTeamAgentsCreatesAgentsInRegistryOrder(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{
			Name: "coder",
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				return fakeAgent{name: "coder"}, nil
			},
		},
		{
			Name: "researcher",
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				return fakeAgent{name: "researcher"}, nil
			},
		},
	})

	team, err := registry.TeamAgents(context.Background())
	if err != nil {
		t.Fatalf("GetTeamAgents() error = %v", err)
	}
	if len(team) != 2 {
		t.Fatalf("team length = %d, want 2", len(team))
	}
	if team[0].Name() != "coder" || team[1].Name() != "researcher" {
		t.Fatalf("team order = [%s %s], want coder researcher", team[0].Name(), team[1].Name())
	}
}

func TestGetTeamAgentsReturnsCreatorError(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{
			Name: "broken",
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				return nil, fmt.Errorf("create failed")
			},
		},
	})

	team, err := registry.TeamAgents(context.Background())
	if err == nil {
		t.Fatal("GetTeamAgents() error = nil, want creator error")
	}
	if team != nil {
		t.Fatalf("team = %#v, want nil", team)
	}
}

func newTestRegistry(values []AgentInfo) *Registry {
	return &Registry{loaded: true, agents: values}
}

type fakeAgent struct {
	name string
}

func (a fakeAgent) Name() string { return a.name }

func (a fakeAgent) Description() string { return a.name + " description" }
