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
		Version: version.Get().String(),
		Commands: []*ucli.Command{
			webCommand(),
			sessionCommand(),
			updateCommand(),
			initCommand(),
			generateCommand(),
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
		},
		Action: chatAction,
	}
}
