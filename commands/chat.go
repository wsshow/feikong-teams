// Package commands 定义 CLI 命令行入口和子命令
package commands

import (
	"context"
	"fkteams/cli"
	commonPkg "fkteams/common"
	"fkteams/config"
	"fkteams/lifecycle"
	"fkteams/runner"
	"fkteams/version"
	"fmt"
	"log"
	"syscall"

	"github.com/cloudwego/eino/adk"
	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// chatAction 默认操作：启动交互模式或直接查询模式
func chatAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}

	workMode := cmd.String("mode")
	currentMode := cli.ParseWorkMode(workMode)
	query := cmd.String("query")
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
	resumeSession := cmd.String("resume")
	saveHistory := cmd.Bool("save")
	approve := cmd.String("approve")

	// 创建应用实例（CLI 模式排除 SIGINT，由 Session 处理）
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

	var r *adk.Runner
	app.OnSetup(func(ctx context.Context) error {
		var err error
		r, err = createModeRunner(ctx, currentMode)
		if err != nil {
			return err
		}
		if r == nil {
			return fmt.Errorf("暂不支持该模式: %s", workMode)
		}
		return nil
	})

	if cfg.MemoryEnabled {
		app.RegisterService(lifecycle.NewMemoryService(cfg.WorkspaceDir))
		pterm.Info.Println("全局长期记忆已启用")
	}
	if cfg.SchedulerEnabled {
		app.RegisterService(lifecycle.NewSchedulerService(cfg.WorkspaceDir, cfg.SchedulerOutputDir))
	}

	var session *cli.Session
	app.OnReady(func(ctx context.Context) error {
		session = cli.NewSession(currentMode, inputHistory, createModeRunner)
		session.ApproveStores = approve
		if resumeSession != "" {
			cli.SetResumeSessionID(resumeSession)
		}
		session.StartSignalHandler(app.ExitCh())

		if query != "" {
			session.HandleDirect(ctx, r, app.ExitCh(), query)
		} else {
			session.HandleInteractive(ctx, r, app.ExitCh())
		}
		return nil
	})

	app.OnPreStop(func(ctx context.Context) error {
		if saveHistory {
			cli.AutoSaveCLIHistory()
		}
		if cfg.MemoryEnabled {
			pterm.Info.Println("正在提取本次对话的记忆，请稍候...")
			cli.FlushSessionMemory()
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

	// 运行应用生命周期
	return app.Run(ctx)
}

// createModeRunner 根据工作模式创建对应的 Runner
func createModeRunner(ctx context.Context, mode cli.WorkMode) (*adk.Runner, error) {
	switch mode {
	case cli.ModeTeam:
		fmt.Printf("欢迎来到非空小队: %s\n", version.Get())
		return runner.CreateSupervisorRunner(ctx)
	case cli.ModeDeep:
		fmt.Printf("欢迎来到非空小队 - 深度模式: %s\n", version.Get())
		return runner.CreateDeepAgentsRunner(ctx)
	case cli.ModeGroup:
		fmt.Printf("欢迎来到非空小队 - 多智能体讨论模式: %s\n", version.Get())
		if err := runner.PrintLoopAgentsInfo(ctx); err != nil {
			return nil, err
		}
		return runner.CreateLoopAgentRunner(ctx)
	case cli.ModeCustom:
		fmt.Printf("欢迎来到非空小队 - 自定义会议模式: %s\n", version.Get())
		if err := runner.PrintCustomAgentsInfo(ctx); err != nil {
			return nil, err
		}
		return runner.CreateCustomSupervisorRunner(ctx)
	default:
		return nil, nil
	}
}
