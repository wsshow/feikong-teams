package commands

import (
	"context"
	"fkteams/bootstrap"
	"strings"

	ucli "github.com/urfave/cli/v3"
)

// initCommand 创建 init 子命令
func initCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "init",
		Usage: "初始化运行环境（安装/升级 uv 等依赖）",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:  "env",
				Usage: "指定要初始化的环境（逗号分隔，如 uv,bun），设置后跳过交互选择；可选: " + strings.Join(bootstrap.Names(), ", "),
			},
			&ucli.BoolFlag{
				Name:  "all",
				Usage: "初始化全部环境（跳过交互选择）",
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if cmd.Bool("all") {
				bootstrap.RunWith(nil)
				return nil
			}
			if envStr := cmd.String("env"); envStr != "" {
				bootstrap.RunWith(strings.Split(envStr, ","))
				return nil
			}
			bootstrap.Run()
			return nil
		},
	}
}
