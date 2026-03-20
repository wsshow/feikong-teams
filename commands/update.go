package commands

import (
	"context"
	"fkteams/update"

	ucli "github.com/urfave/cli/v3"
)

// updateCommand 创建 update 子命令
func updateCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "update",
		Usage: "检查更新并升级到最新版本",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if err := loadEnv(); err != nil {
				return nil
			}
			return update.SelfUpdate("fkteams", "wsshow", "feikong-teams")
		},
	}
}
