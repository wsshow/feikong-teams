// Package commands 定义 CLI 命令行入口和子命令
package commands

import (
	"context"
	inputhistory "fkteams/internal/adapters/storage/file/inputhistory"
	"fkteams/internal/adapters/transport/cli/eventview"
	cliruntime "fkteams/internal/adapters/transport/cli/runtime"
	appagent "fkteams/internal/app/agent"
	"fkteams/internal/app/config"
	"fkteams/internal/app/lifecycle"
	appschedule "fkteams/internal/app/schedule"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/log"
	"fmt"
	"syscall"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// chatAction 默认操作：启动交互模式或直接查询模式
func runChat(ctx context.Context, cmd *ucli.Command, opts Options, agentName string) error {
	if err := config.InitAndValidate(); err != nil {
		return err
	}

	workMode := cmd.String("mode")
	currentMode := cliruntime.ParseWorkMode(workMode)
	query := cmd.String("query")
	if query == "" {
		query = cmd.Root().String("query")
	}
	pipeInput, isPipe, err := cliruntime.ReadPipeInput()
	if err != nil {
		return fmt.Errorf("read piped input: %w", err)
	}
	if isPipe {
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
	approve := cmd.String("approve")
	if approve == "" {
		approve = cmd.Root().String("approve")
	}

	// 创建应用实例（CLI 模式排除 SIGINT，由 Session 处理）
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

	var r runtimeport.Runner
	app.OnSetup(func(ctx context.Context) error {
		var err error
		if agentName != "" {
			r, err = appagent.NewCache().Resolve(ctx, "team", agentName)
		} else {
			r, err = createModeRunner(ctx, currentMode)
		}
		if err != nil {
			return err
		}
		if r == nil {
			return fmt.Errorf("暂不支持该模式: %s", workMode)
		}
		return nil
	})

	var schedulerProvider func() *appschedule.Service
	if opts.ConfigureServices == nil {
		return fmt.Errorf("CLI application services are not configured")
	}
	schedulerProvider = opts.ConfigureServices(app)
	if cfg.MemoryEnabled && query != "" && cmd.String("format") != "json" {
		pterm.Info.Println("全局长期记忆已启用")
	}

	var session *cliruntime.Session
	app.OnReady(func(ctx context.Context) error {
		session = cliruntime.NewSession(currentMode, inputHistory, createModeRunner)
		session.SetMemoryManager(state.Memory())
		if schedulerProvider != nil {
			session.SetScheduleService(schedulerProvider())
		}
		session.ApproveStores = approve
		if agentName != "" {
			session.SetCurrentAgent(agentName)
		}
		if cmd.String("format") == "json" {
			session.SetCallbackBuilder(eventview.JSONEventCallback)
		}
		session.SetTemporary(temporarySession)
		if resumeSession != "" {
			session.SetResumeSessionID(resumeSession)
		}

		if query != "" {
			session.StartSignalHandler(app.ExitCh())
			return session.HandleDirect(ctx, r, app.ExitCh(), query)
		} else {
			session.HandleInteractive(ctx, r, app.ExitCh())
		}
		return nil
	})

	app.OnPreStop(func(ctx context.Context) error {
		var saveErr error
		if !temporarySession && session != nil {
			var saved bool
			saved, saveErr = session.SaveHistory(ctx)
			if saved && query == "" {
				session.PrintResumeHint()
			}
		}
		if cfg.MemoryEnabled && query != "" && cmd.String("format") != "json" {
			pterm.Info.Println("正在提取本次对话的记忆，请稍候...")
			if session != nil {
				session.FlushMemoryWithManager(ctx, state.Memory())
			}
		} else if cfg.MemoryEnabled {
			if session != nil {
				session.FlushMemoryWithManager(ctx, state.Memory())
			}
		}
		return saveErr
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

	// 运行应用生命周期
	return app.Run(ctx)
}

// createModeRunner 根据工作模式创建对应的 Runner
func createModeRunner(ctx context.Context, mode cliruntime.WorkMode) (runtimeport.Runner, error) {
	switch mode {
	case cliruntime.ModeTeam:
		return appagent.CreateTeamRunner(ctx)
	case cliruntime.ModeDeep:
		return appagent.CreateDeepAgentsRunner(ctx)
	case cliruntime.ModeGroup:
		return appagent.CreateLoopAgentRunner(ctx)
	default:
		return nil, nil
	}
}
