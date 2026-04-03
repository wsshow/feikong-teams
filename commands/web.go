package commands

import (
	"context"
	"fkteams/config"
	"fkteams/server"

	ucli "github.com/urfave/cli/v3"
)

// webCommand 创建 web 子命令
func webCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "web",
		Usage: "启动 Web 服务器",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := config.Init(); err != nil {
				return err
			}
			return server.Run()
		},
	}
}
