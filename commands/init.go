package commands

import (
	"context"
	"fkteams/bootstrap"

	ucli "github.com/urfave/cli/v3"
)

// initCommand 创建 init 子命令
func initCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "init",
		Usage: "初始化运行环境（安装/升级 uv 等依赖）",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			bootstrap.Run()
			return nil
		},
	}
}
