package commands

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"fkteams/agents"
	"fkteams/cli"
	commonPkg "fkteams/common"
	"fkteams/lifecycle"
	"fkteams/runner"

	"github.com/cloudwego/eino/adk"
	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// agentCommand 创建 agent 子命令
func agentCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "agent",
		Usage: "指定单个 Agent 执行任务",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Agent 名称",
				Required: true,
			},
			&ucli.StringFlag{
				Name:    "query",
				Aliases: []string{"q"},
				Usage:   "直接查询模式，执行完查询后退出",
			},
		},
		Action: agentAction,
	}
}

// agentAction 执行单 Agent 任务
func agentAction(ctx context.Context, cmd *ucli.Command) error {
	if err := godotenv.Load(); err != nil {
		fmt.Println("加载 .env 文件失败，请确保已创建该文件")
		fmt.Println("可以使用 generate env 子命令生成示例文件")
		return nil
	}

	agentName := cmd.String("name")
	query := cmd.String("query")
	if query == "" {
		query = cmd.Root().String("query")
	}

	agentInfo := agents.GetAgentByName(agentName)
	if agentInfo == nil {
		pterm.Error.Printfln("未找到 Agent: %s", agentName)
		fmt.Println("可用的 Agent 列表:")
		for _, info := range agents.GetRegistry() {
			fmt.Printf("  - %s: %s\n", info.Name, info.Description)
		}
		return nil
	}

	app := lifecycle.New(
		lifecycle.WithExitSignals(syscall.SIGTERM, syscall.SIGHUP),
	)
	cfg := app.Config()

	var inputHistory []string
	app.OnInit(func(ctx context.Context) error {
		var err error
		inputHistory, err = commonPkg.LoadHistory(cfg.InputHistoryPath, 100)
		if err != nil {
			return fmt.Errorf("加载输入历史失败: %w", err)
		}
		return nil
	})

	var agentRunner *adk.Runner
	app.OnSetup(func(ctx context.Context) error {
		agent := agentInfo.Creator(ctx)
		agentRunner = runner.CreateAgentRunner(ctx, agent)
		fmt.Printf("已启用 Agent: %s (%s)\n", agentInfo.Name, agentInfo.Description)
		return nil
	})

	var session *cli.Session
	app.OnReady(func(ctx context.Context) error {
		session = cli.NewSession(cli.ModeTeam, inputHistory, nil)
		session.SetCurrentAgent(agentName)
		session.StartSignalHandler(app.ExitCh())

		if query != "" {
			session.HandleDirect(ctx, agentRunner, app.ExitCh(), query)
		} else {
			session.HandleInteractive(ctx, agentRunner, app.ExitCh())
		}
		return nil
	})

	app.OnCleanup(func(ctx context.Context) error {
		history := inputHistory
		if session != nil {
			history = session.InputHistory
		}
		if err := commonPkg.SaveHistory(cfg.InputHistoryPath, history); err != nil {
			log.Printf("保存输入历史失败: %v", err)
		}
		pterm.Success.Println("成功退出")
		return nil
	})

	return app.Run(ctx)
}
