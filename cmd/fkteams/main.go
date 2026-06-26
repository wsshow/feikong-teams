package main

import (
	"context"
	clicommands "fkteams/internal/adapters/transport/cli/commands"
	bootstrapruntimes "fkteams/internal/bootstrap/runtimes"
	bootstraptools "fkteams/internal/bootstrap/tools"
	runtimeport "fkteams/internal/ports/runtime"
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
	if err := bootstraptools.RegisterDefaults(); err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	engine, err := bootstrapruntimes.DefaultEngine()
	if err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
	ctx := runtimeport.WithEngine(context.Background(), engine)
	if err := clicommands.Root().Run(ctx, os.Args); err != nil {
		pterm.Error.Println(err)
	}
}
