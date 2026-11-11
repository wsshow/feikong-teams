package commands

import (
	"slices"
	"testing"

	ucli "github.com/urfave/cli/v3"
)

func TestRootCommandStructure(t *testing.T) {
	cmd := Root()
	if cmd.Name != "fkteams" {
		t.Fatalf("root name = %q, want fkteams", cmd.Name)
	}
	if cmd.Action == nil {
		t.Fatal("root action should be set")
	}

	commandNames := make([]string, 0, len(cmd.Commands))
	for _, sub := range cmd.Commands {
		commandNames = append(commandNames, sub.Name)
	}
	for _, want := range []string{"web", "serve", "session", "update", "init", "generate", "agent", "tool", "skill", "model", "login", "logout", "auth"} {
		if !slices.Contains(commandNames, want) {
			t.Fatalf("root commands = %#v, missing %q", commandNames, want)
		}
	}

	flagNames := make([]string, 0, len(cmd.Flags))
	for _, flag := range cmd.Flags {
		flagNames = append(flagNames, flag.Names()[0])
	}
	for _, want := range []string{"query", "resume", "mode", "temporary", "approve"} {
		if !slices.Contains(flagNames, want) {
			t.Fatalf("root flags = %#v, missing %q", flagNames, want)
		}
	}
}

func TestSubCommandStructure(t *testing.T) {
	tests := []struct {
		name     string
		command  *ucli.Command
		children []string
		flags    []string
	}{
		{name: "web", command: webCommand(Options{}), flags: []string{"host", "port"}},
		{name: "serve", command: serveCommand(Options{}), flags: []string{"host", "port"}},
		{name: "generate", command: generateCommand(), children: []string{"config", "apikey"}},
		{name: "model", command: modelCommand(), children: []string{"ls", "lr", "sw", "rm"}},
		{name: "session", command: sessionCommand(), children: []string{"list"}},
		{name: "agent", command: agentCommand(Options{}), children: []string{"list"}, flags: []string{"name", "query", "temporary", "format", "approve"}},
		{name: "tool", command: toolCommand(), children: []string{"list"}},
		{name: "auth", command: authCommand(), children: []string{"enable", "disable", "status"}},
		{name: "login", command: loginCommand(), children: []string{"copilot", "openai", "deepseek", "claude", "gemini", "qwen", "ollama", "ark", "openrouter", "custom"}},
		{name: "logout", command: logoutCommand(), children: []string{"copilot", "openai", "deepseek", "claude", "gemini", "qwen", "ollama", "ark", "openrouter", "custom"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.command
			if command.Name != tt.name {
				t.Fatalf("command name = %q, want %q", command.Name, tt.name)
			}

			childNames := make([]string, 0, len(command.Commands))
			for _, child := range command.Commands {
				childNames = append(childNames, child.Name)
			}
			for _, want := range tt.children {
				if !slices.Contains(childNames, want) {
					t.Fatalf("%s children = %#v, missing %q", tt.name, childNames, want)
				}
			}

			flagNames := make([]string, 0, len(command.Flags))
			for _, flag := range command.Flags {
				flagNames = append(flagNames, flag.Names()[0])
			}
			for _, want := range tt.flags {
				if !slices.Contains(flagNames, want) {
					t.Fatalf("%s flags = %#v, missing %q", tt.name, flagNames, want)
				}
			}
		})
	}
}
