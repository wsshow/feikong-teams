package commands

import (
	"context"
	httpserver "fkteams/internal/adapters/transport/http"
	"fkteams/internal/app/lifecycle"
	appschedule "fkteams/internal/app/schedule"
	appskill "fkteams/internal/app/skill"
	"fkteams/internal/app/version"

	ucli "github.com/urfave/cli/v3"
)

// Options 是 CLI 所需的应用装配能力，由组合根显式提供。
type Options struct {
	PrepareStorage    func(context.Context) (context.Context, error)
	CloseStorage      func() error
	RunServer         func(context.Context, httpserver.Mode, *httpserver.ServeOptions) error
	ConfigureServices func(*lifecycle.Application) func() *appschedule.Service
	SkillProviders    *appskill.ProviderRegistry
}

// Root 创建根命令
func Root(options ...Options) *ucli.Command {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	return &ucli.Command{
		Name:    "fkteams",
		Usage:   "多智能体协作 AI 助手",
		Authors: []any{"FeiKong"},
		Version: version.Get().String(),
		Commands: []*ucli.Command{
			webCommand(opts),
			serveCommand(opts),
			sessionCommand(opts),
			updateCommand(),
			initCommand(),
			generateCommand(),
			agentCommand(opts),
			toolCommand(),
			skillCommand(opts),
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
				Usage:   "工作模式: team|deep|group",
			},
			&ucli.BoolFlag{
				Name:    "temporary",
				Aliases: []string{"temp"},
				Usage:   "开启临时会话，不保存聊天历史且不显示恢复命令",
			},
			&ucli.StringFlag{
				Name:  "approve",
				Usage: "自动批准指定操作类别 (all/command/file/git/dispatch，逗号分隔)",
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error { return runChat(ctx, cmd, opts, "") },
	}
}
