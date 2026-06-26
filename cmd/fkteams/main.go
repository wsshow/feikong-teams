package main

import (
	"context"
	modelproviders "fkteams/internal/adapters/model/providers"
	clicommands "fkteams/internal/adapters/transport/cli/commands"
	agents "fkteams/internal/app/agent/catalog"
	apptools "fkteams/internal/app/tools"
	bootstrapruntimes "fkteams/internal/bootstrap/runtimes"
	bootstraptools "fkteams/internal/bootstrap/tools"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
	"log"
	"os"

	"github.com/pterm/pterm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Llongfile)
}

func main() {
	if err := bootstrapruntimes.RegisterDefaults(); err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	toolRegistry, err := bootstraptools.RegisterDefaults()
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	engine, err := bootstrapruntimes.DefaultEngine()
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	modelRegistry, err := bootstrapruntimes.DefaultModelRegistry()
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	modelProviderRegistry, err := bootstrapruntimes.DefaultModelProviderRegistry()
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	ctx := runtimeport.WithEngine(context.Background(), engine)
	ctx = runtimeport.WithInterruptRuntime(ctx, bootstrapruntimes.DefaultInterruptRuntime())
	ctx = modelregistry.WithRegistry(ctx, modelRegistry)
	ctx = modelproviders.WithRegistry(ctx, modelProviderRegistry)
	ctx = apptools.WithRegistry(ctx, toolRegistry)
	ctx = agents.WithRegistry(ctx, agents.NewRegistry())
	if err := clicommands.Root().Run(ctx, os.Args); err != nil {
		pterm.Error.Println(err)
	}
}
