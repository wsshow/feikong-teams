package eino

import (
	"context"
	"fkteams/agentcore"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func WrapAgent(agent adk.Agent) agentcore.Agent {
	if agent == nil {
		return nil
	}
	ctx := context.Background()
	return WrapNamedAgent(agent.Name(ctx), agent.Description(ctx), agent)
}

func WrapNamedAgent(name, description string, agent adk.Agent) agentcore.Agent {
	return &runtimeAgent{name: name, description: description, inner: agent}
}

type runtimeAgent struct {
	name        string
	description string
	inner       adk.Agent
}

func (a *runtimeAgent) Name() string {
	if a == nil {
		return ""
	}
	return a.name
}

func (a *runtimeAgent) Description() string {
	if a == nil {
		return ""
	}
	return a.description
}

func (a *runtimeAgent) runnerAgent() adk.Agent {
	if a == nil {
		return nil
	}
	return a.inner
}

func AdaptAgentForRunner(agent agentcore.Agent) (adk.Agent, error) {
	if agent == nil {
		return nil, fmt.Errorf("agent is nil")
	}
	runnerAgent, ok := agent.(interface{ runnerAgent() adk.Agent })
	if !ok || runnerAgent.runnerAgent() == nil {
		return nil, fmt.Errorf("unsupported agent: %T", agent)
	}
	return runnerAgent.runnerAgent(), nil
}

func AdaptAgentsForRunner(agents []agentcore.Agent) ([]adk.Agent, error) {
	result := make([]adk.Agent, 0, len(agents))
	for _, agent := range agents {
		runnerAgent, err := AdaptAgentForRunner(agent)
		if err != nil {
			return nil, err
		}
		result = append(result, runnerAgent)
	}
	return result, nil
}
