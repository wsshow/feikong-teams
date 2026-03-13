package commands

import (
	"context"
	"fkteams/server"
	"fmt"

	"github.com/joho/godotenv"
	ucli "github.com/urfave/cli/v3"
)

// serveCommand 创建 serve 子命令
func serveCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "serve",
		Usage: "启动纯 API 服务（无 Web 界面）",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:  "host",
				Usage: "监听地址",
			},
			&ucli.IntFlag{
				Name:  "port",
				Usage: "监听端口",
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := godotenv.Load(); err != nil {
				fmt.Println("加载 .env 文件失败，请确保已创建该文件")
				fmt.Println("可以使用 generate env 子命令生成示例文件")
				return nil
			}
			server.RunServe(server.ServeOptions{
				Host: cmd.String("host"),
				Port: int(cmd.Int("port")),
			})
			return nil
		},
	}
}
