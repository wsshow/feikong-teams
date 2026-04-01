package commands

import (
	"context"
	"fkteams/config"
	"fkteams/server"

	ucli "github.com/urfave/cli/v3"
)

// serveCommand 创建 serve 子命令
func serveCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "serve",
		Usage: "启动纯 API 服务（无 Web 界面）",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:        "host",
				DefaultText: "不设置则从配置文件读取，默认 127.0.0.1",
				Usage:       "监听地址",
			},
			&ucli.IntFlag{
				Name:        "port",
				DefaultText: "不设置则从配置文件读取，默认为 23456",
				Usage:       "监听端口",
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := config.Init(); err != nil {
				return err
			}
			return server.RunServe(server.ServeOptions{
				Host: cmd.String("host"),
				Port: int(cmd.Int("port")),
			})
		},
	}
}
