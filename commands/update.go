package commands

import (
	"context"
	"fkteams/update"
	"fmt"

	"github.com/joho/godotenv"
	ucli "github.com/urfave/cli/v3"
)

// updateCommand 创建 update 子命令
func updateCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "update",
		Usage: "检查更新并升级到最新版本",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := godotenv.Load(); err != nil {
				fmt.Println("加载 .env 文件失败，请确保已创建该文件")
				fmt.Println("可以使用 generate env 子命令生成示例文件")
				return nil
			}
			return update.SelfUpdate("wsshow", "feikong-teams")
		},
	}
}
