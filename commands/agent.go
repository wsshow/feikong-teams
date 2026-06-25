package commands

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"fkteams/agents"
	"fkteams/cli"
	"fkteams/config"
	"fkteams/events/view"
	inputhistory "fkteams/internal/adapters/storage/file/inputhistory"
	appagent "fkteams/internal/app/agent"
	"fkteams/internal/app/lifecycle"
	runtimeport "fkteams/internal/ports/runtime"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// agentCommand 创建 agent 子命令
func agentCommand() *ucli.Command {
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
					registry := agents.GetRegistry()
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
		Action: agentAction,
	}
}

// agentAction 执行单 Agent 任务
func agentAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.InitAndValidate(); err != nil {
		return err
	}

	agentName := cmd.String("name")
	if agentName == "" {
		return fmt.Errorf("请通过 --name/-n 指定 Agent 名称，或使用 agent list 查看可用列表")
	}
	query := cmd.String("query")
	if query == "" {
		query = cmd.Root().String("query")
	}
	if pipeInput, isPipe := cli.ReadPipeInput(); isPipe {
		if pipeInput != "" {
			if query != "" {
				query = query + "\n" + pipeInput
			} else {
				query = pipeInput
			}
		} else if query == "" {
			return fmt.Errorf("检测到管道输入但内容为空，请提供查询内容或使用 -q 参数")
		}
	}
	resumeSession := cmd.Root().String("resume")
	temporarySession := cmd.Bool("temporary") || cmd.Root().Bool("temporary")

	agentInfo := agents.GetAgentByName(agentName)
	if agentInfo == nil {
		pterm.Error.Printfln("未找到 Agent: %s", agentName)
		pterm.DefaultSection.Println("可用的 Agent 列表")
		var items []pterm.BulletListItem
		for _, info := range agents.GetRegistry() {
			items = append(items, pterm.BulletListItem{
				Level: 0,
				Text:  pterm.Bold.Sprint(info.Name) + "  " + pterm.FgGray.Sprint(info.Description),
			})
		}
		_ = pterm.DefaultBulletList.WithItems(items).Render()
		return nil
	}

	app := lifecycle.New(
		lifecycle.WithExitSignals(syscall.SIGTERM, syscall.SIGHUP),
	)
	cfg := app.Config()
	state := app.State()

	var inputHistory []string
	app.OnInit(func(ctx context.Context) error {
		var err error
		inputHistory, err = inputhistory.Load(cfg.InputHistoryPath, 100)
		if err != nil {
			return fmt.Errorf("加载输入历史失败: %w", err)
		}
		return nil
	})

	var agentRunner runtimeport.Runner
	app.OnSetup(func(ctx context.Context) error {
		var err error
		agent, err := agentInfo.Creator(ctx)
		if err != nil {
			return fmt.Errorf("create agent %s: %w", agentName, err)
		}
		agentRunner, err = appagent.CreateAgentRunner(ctx, agent)
		return err
	})

	var session *cli.Session
	app.OnReady(func(ctx context.Context) error {
		session = cli.NewSession(cli.ModeTeam, inputHistory, nil)
		session.SetMemoryManager(state.Memory())
		session.SetCurrentAgent(agentName)
		cli.SetTemporarySession(temporarySession)
		if resumeSession != "" {
			cli.SetResumeSessionID(resumeSession)
		}

		approve := cmd.String("approve")
		if approve == "" {
			approve = cmd.Root().String("approve")
		}
		session.ApproveStores = approve

		format := cmd.String("format")
		if format == "json" {
			session.SetCallbackBuilder(eventview.JSONEventCallback)
		}

		if query != "" {
			session.StartSignalHandler(app.ExitCh())
			session.HandleDirect(ctx, agentRunner, app.ExitCh(), query)
		} else {
			session.HandleInteractive(ctx, agentRunner, app.ExitCh())
		}
		return nil
	})

	app.OnPreStop(func(ctx context.Context) error {
		if !temporarySession {
			if cli.SaveCLISessionHistory() {
				if query == "" {
					cli.PrintResumeHint()
				}
			}
		}
		return nil
	})

	app.OnCleanup(func(ctx context.Context) error {
		state.RunProcessCleanup()
		history := inputHistory
		if session != nil {
			history = session.InputHistory
		}
		if err := inputhistory.Save(cfg.InputHistoryPath, history); err != nil {
			log.Printf("保存输入历史失败: %v", err)
		}
		return nil
	})

	return app.Run(ctx)
}
