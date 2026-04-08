package commands

import (
	"fkteams/version"

	ucli "github.com/urfave/cli/v3"
)

// Root 创建根命令
func Root() *ucli.Command {
	return &ucli.Command{
		Name:    "fkteams",
		Usage:   "多智能体协作 AI 助手",
		Authors: []any{"FeiKong"},
		Version: version.Get().String(),
		Commands: []*ucli.Command{
			webCommand(),
			serveCommand(),
			sessionCommand(),
			updateCommand(),
			initCommand(),
			generateCommand(),
			agentCommand(),
			toolCommand(),
			skillCommand(),
			modelCommand(),
			loginCommand(),
			logoutCommand(),
			authCommand(),
		},
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:    "query",
				Aliases: []string{"q"},
				Usage:   "直接查询模式，执行完查询后退出",
			},
			&ucli.StringFlag{
				Name:    "resume",
				Aliases: []string{"r"},
				Usage:   "恢复指定的聊天历史会话",
			},
			&ucli.StringFlag{
				Name:    "mode",
				Aliases: []string{"m"},
				Value:   "team",
				Usage:   "工作模式: team|deep|group|custom",
			},
			&ucli.BoolFlag{
				Name:  "save",
				Usage: "保存聊天历史",
			},
			&ucli.StringFlag{
				Name:  "approve",
				Usage: "自动批准指定操作类别 (all/command/file/dispatch，逗号分隔)",
			},
		},
		Action: chatAction,
	}
}
