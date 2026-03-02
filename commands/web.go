package commands

import (
	"context"
	"fkteams/server"
	"fmt"

	"github.com/joho/godotenv"
	ucli "github.com/urfave/cli/v3"
)

// webCommand 创建 web 子命令
func webCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "web",
		Usage: "启动 Web 服务器",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := godotenv.Load(); err != nil {
				fmt.Println("加载 .env 文件失败，请确保已创建该文件")
				fmt.Println("可以使用 generate env 子命令生成示例文件")
				return nil
			}
			server.Run()
			return nil
		},
	}
}
