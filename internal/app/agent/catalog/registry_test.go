package agents

import (
	"context"
	"fmt"
	"testing"

	"fkteams/internal/app/config"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestGetRegistryReturnsCopy(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{Name: "coder", Description: "code", Aliases: []string{"小码"}},
		{Name: "researcher", Description: "research"},
	})

	got := registry.List()
	if len(got) != 2 {
		t.Fatalf("registry length = %d, want 2", len(got))
	}
	got[0].Name = "mutated"
	got[0].Aliases[0] = "mutated"

	again := registry.List()
	if again[0].Name != "coder" {
		t.Fatalf("registry was mutated through returned slice: %v", again)
	}
	if again[0].Aliases[0] != "小码" {
		t.Fatalf("registry aliases were mutated through returned slice: %v", again[0].Aliases)
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
			Name:       "coder",
			TeamMember: true,
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				return fakeAgent{name: "coder"}, nil
			},
		},
		{
			Name:       "researcher",
			TeamMember: true,
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

func TestGetTeamAgentsSkipsNonTeamMembers(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{
			Name: "coordinator",
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				t.Fatal("coordinator should not be created as a team member")
				return nil, nil
			},
		},
		{
			Name:       "coder",
			TeamMember: true,
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				return fakeAgent{name: "coder"}, nil
			},
		},
	})

	team, err := registry.TeamAgents(context.Background())
	if err != nil {
		t.Fatalf("GetTeamAgents() error = %v", err)
	}
	if len(team) != 1 || team[0].Name() != "coder" {
		t.Fatalf("team = %#v, want only coder", team)
	}
}

func TestGetTeamAgentsReturnsCreatorError(t *testing.T) {
	registry := newTestRegistry([]AgentInfo{
		{
			Name:       "broken",
			TeamMember: true,
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

func TestConfigItemsIgnoreBuiltinOverrides(t *testing.T) {
	items := ConfigItems(&config.Config{
		Agents: config.Agents{
			Items: []config.AgentConfig{
				{ID: "coordinator", Name: "本地覆盖", Prompt: "local prompt", Enabled: false},
				{ID: "coder", Name: "本地代码工程师", Prompt: "local coder prompt", Enabled: false},
				{ID: "custom-agent", Name: "自定义智能体", Enabled: true},
			},
		},
	})

	var coordinator, coder, custom *config.AgentConfig
	for i := range items {
		switch items[i].ID {
		case "coordinator":
			coordinator = &items[i]
		case "coder":
			coder = &items[i]
		case "custom-agent":
			custom = &items[i]
		}
	}
	if coordinator == nil {
		t.Fatal("coordinator not found")
	}
	if coordinator.Name == "本地覆盖" || coordinator.Prompt == "local prompt" || !coordinator.Enabled {
		t.Fatalf("coordinator override should be ignored, got %#v", coordinator)
	}
	if coder == nil {
		t.Fatal("coder not found")
	}
	if coder.Name == "本地代码工程师" || coder.Prompt == "local coder prompt" || coder.Enabled {
		t.Fatalf("builtin non-coordinator should only apply enabled override, got %#v", coder)
	}
	if custom == nil || custom.Builtin || custom.Name != "自定义智能体" {
		t.Fatalf("custom agent = %#v, want persisted custom agent", custom)
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
