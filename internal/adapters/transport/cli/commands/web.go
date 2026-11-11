package commands

import (
	"context"
	httpserver "fkteams/internal/adapters/transport/http"
	"fkteams/internal/app/config"
	"fmt"

	ucli "github.com/urfave/cli/v3"
)

// webCommand 创建 web 子命令
func webCommand(opts Options) *ucli.Command {
	return &ucli.Command{
		Name:  "web",
		Usage: "启动 Web 服务器",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:        "host",
				DefaultText: "不设置则从配置文件读取，默认 127.0.0.1",
				Usage:       "监听地址",
			},
			&ucli.IntFlag{
				Name:        "port",
				DefaultText: "不设置则从配置文件读取，默认 23456",
				Usage:       "监听端口",
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := config.Init(); err != nil {
				return err
			}
			if opts.RunServer == nil {
				return fmt.Errorf("server application is not configured")
			}
			return opts.RunServer(ctx, httpserver.ModeWeb, &httpserver.ServeOptions{
				Host: cmd.String("host"),
				Port: int(cmd.Int("port")),
			})
		},
	}
}
