// Package commands 定义 CLI 命令行入口和子命令
package commands

import (
	"context"
	"fkteams/cli"
	commonPkg "fkteams/common"
	"fkteams/config"
	"fkteams/g"
	"fkteams/lifecycle"
	"fkteams/runner"
	"fmt"
	"log"
	"syscall"

	"github.com/cloudwego/eino/adk"
	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// chatAction 默认操作：启动交互模式或直接查询模式
func chatAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.InitAndValidate(); err != nil {
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
		if query != "" {
			pterm.Info.Println("全局长期记忆已启用")
		}
	}
	if cfg.SchedulerEnabled {
		app.RegisterService(lifecycle.NewSchedulerService(cfg.SchedulerDir))
	}

	var session *cli.Session
	app.OnReady(func(ctx context.Context) error {
		session = cli.NewSession(currentMode, inputHistory, createModeRunner)
		session.ApproveStores = approve
		if resumeSession != "" {
			cli.SetResumeSessionID(resumeSession)
		}

		if query != "" {
			session.StartSignalHandler(app.ExitCh())
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
		if cfg.MemoryEnabled && query != "" {
			pterm.Info.Println("正在提取本次对话的记忆，请稍候...")
			cli.FlushSessionMemory()
		} else if cfg.MemoryEnabled {
			cli.FlushSessionMemory()
		}
		return nil
	})

	app.OnCleanup(func(ctx context.Context) error {
		g.RunProcessCleanup()
		history := inputHistory
		if session != nil {
			history = session.InputHistory
		}
		if err := commonPkg.SaveHistory(cfg.InputHistoryPath, history); err != nil {
			log.Printf("保存输入历史失败: %v", err)
		}
		if query != "" {
			pterm.Success.Println("成功退出")
		}
		return nil
	})

	// 运行应用生命周期
	return app.Run(ctx)
}

// createModeRunner 根据工作模式创建对应的 Runner
func createModeRunner(ctx context.Context, mode cli.WorkMode) (*adk.Runner, error) {
	switch mode {
	case cli.ModeTeam:
		return runner.CreateTeamRunner(ctx)
	case cli.ModeDeep:
		return runner.CreateDeepAgentsRunner(ctx)
	case cli.ModeGroup:
		return runner.CreateLoopAgentRunner(ctx)
	case cli.ModeCustom:
		return runner.CreateCustomRunner(ctx)
	default:
		return nil, nil
	}
}
