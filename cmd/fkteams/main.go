package main

import (
	"context"
	"os"

	"fkteams/internal/bootstrap"

	"github.com/pterm/pterm"
)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	dependencies, err := bootstrap.NewExecutionDependencies()
	if err != nil {
		return err
	}
	return dependencies.Command().Run(dependencies.Context(ctx), args)
}
