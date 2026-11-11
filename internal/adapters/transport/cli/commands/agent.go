package commands

import (
	"context"
	"fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/config"
	"fmt"
	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// agentCommand 创建 agent 子命令
func agentCommand(opts Options) *ucli.Command {
	return &ucli.Command{
		Name:  "agent",
		Usage: "指定单个 Agent 执行任务",
		Commands: []*ucli.Command{
			{
				Name:  "list",
				Usage: "列出所有可用的 Agent",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.Init(); err != nil {
						return err
					}
					registry, err := agents.List(ctx)
					if err != nil {
						return err
					}
					if len(registry) == 0 {
						fmt.Println("暂无可用的 Agent")
						return nil
					}
					pterm.DefaultSection.Println("可用的 Agent 列表")
					var items []pterm.BulletListItem
					for _, info := range registry {
						items = append(items, pterm.BulletListItem{
							Level: 0,
							Text:  pterm.Bold.Sprint(info.Name) + "  " + pterm.FgGray.Sprint(info.Description),
						})
					}
					return pterm.DefaultBulletList.WithItems(items).Render()
				},
			},
		},
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Agent 名称",
			},
			&ucli.StringFlag{
				Name:    "query",
				Aliases: []string{"q"},
				Usage:   "直接查询模式，执行完查询后退出",
			},
			&ucli.BoolFlag{
				Name:    "temporary",
				Aliases: []string{"temp"},
				Usage:   "开启临时会话，不保存聊天历史且不显示恢复命令",
			},
			&ucli.StringFlag{
				Name:  "format",
				Value: "default",
				Usage: "输出格式: default（格式化输出）或 json（原始 JSON 事件）",
			},
			&ucli.StringFlag{
				Name:  "approve",
				Usage: "自动批准指定操作类别 (all/command/file/git/dispatch，逗号分隔)",
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			name := cmd.String("name")
			if name == "" {
				return fmt.Errorf("agent name is required; use --name or agent list")
			}
			return runChat(ctx, cmd, opts, name)
		},
	}
}
