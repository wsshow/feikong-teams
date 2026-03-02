package main

import (
	"context"
	"fkteams/commands"
	"log"
	"os"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Llongfile)
}

func main() {
	if err := commands.Root().Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
