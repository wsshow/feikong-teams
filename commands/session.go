package commands

import (
	"context"
	"fkteams/cli"

	ucli "github.com/urfave/cli/v3"
)

// sessionCommand 创建 session 子命令
func sessionCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "session",
		Usage: "聊天历史会话管理",
		Commands: []*ucli.Command{
			{
				Name:  "list",
				Usage: "列出所有可用的聊天历史会话",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					cli.ListChatHistoryFiles()
					return nil
				},
			},
		},
	}
}
