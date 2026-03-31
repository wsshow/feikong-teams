package main

import (
	"context"
	"fkteams/commands"
	"log"
	"os"

	"github.com/pterm/pterm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Llongfile)
}

func main() {
	if err := commands.Root().Run(context.Background(), os.Args); err != nil {
		pterm.Error.Println(err)
	}
}
