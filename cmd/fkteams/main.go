package main

import (
	"context"
	modelproviders "fkteams/internal/adapters/model/providers"
	mcpadapter "fkteams/internal/adapters/tools/mcp"
	clicommands "fkteams/internal/adapters/transport/cli/commands"
	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	apptools "fkteams/internal/app/tools"
	bootstrapruntimes "fkteams/internal/bootstrap/runtimes"
	bootstraptools "fkteams/internal/bootstrap/tools"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
	"os"

	"github.com/pterm/pterm"
)

func main() {
	mcpProvider := mcpadapter.NewProvider()
	toolDisplays := toolmeta.NewRegistry()
	runtimeDefaults, err := bootstrapruntimes.NewDefaults(bootstrapruntimes.Options{
		MCPProvider: mcpProvider,
	})
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	toolRegistry, err := bootstraptools.RegisterDefaults(mcpProvider)
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	ctx := runtimeport.WithRuntime(context.Background(), runtimeDefaults.Runtime)
	ctx = runtimeport.WithInterruptRuntime(ctx, runtimeDefaults.Interrupt)
	ctx = modelregistry.WithRegistry(ctx, runtimeDefaults.ModelRegistry)
	ctx = modelproviders.WithRegistry(ctx, runtimeDefaults.ModelProviderRegistry)
	ctx = apptools.WithRegistry(ctx, toolRegistry)
	ctx = toolmeta.WithRegistry(ctx, toolDisplays)
	ctx = agents.WithRegistry(ctx, agents.NewRegistry())
	if err := clicommands.Root().Run(ctx, os.Args); err != nil {
		pterm.Error.Println(err)
	}
}
