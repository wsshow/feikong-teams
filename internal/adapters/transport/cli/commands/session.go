package commands

import (
	"context"
	cliruntime "fkteams/internal/adapters/transport/cli/runtime"
	"fkteams/internal/app/execution"

	ucli "github.com/urfave/cli/v3"
)

// sessionCommand 创建 session 子命令
func sessionCommand(options ...Options) *ucli.Command {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	return &ucli.Command{
		Name:  "session",
		Usage: "聊天历史会话管理",
		Commands: []*ucli.Command{
			{
				Name:  "list",
				Usage: "列出所有可用的聊天历史会话",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if opts.PrepareStorage != nil {
						var err error
						ctx, err = opts.PrepareStorage(ctx)
						if err != nil {
							return err
						}
					}
					cliruntime.ListSessionsWithDocuments(execution.FromContext(ctx).Documents)
					if opts.CloseStorage != nil {
						return opts.CloseStorage()
					}
					return nil
				},
			},
		},
	}
}
