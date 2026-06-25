package main

import (
	"context"
	clicommands "fkteams/internal/adapters/transport/cli/commands"
	"log"
	"os"

	_ "fkteams/internal/bootstrap/runtimes"
	_ "fkteams/internal/bootstrap/tools"

	"github.com/pterm/pterm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Llongfile)
}

func main() {
	if err := clicommands.Root().Run(context.Background(), os.Args); err != nil {
		pterm.Error.Println(err)
	}
}
