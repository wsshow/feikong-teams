package bootstrap

import (
	"context"
	channel "fkteams/internal/adapters/transport/channel"
	httpserver "fkteams/internal/adapters/transport/http"
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/app/config"
	"fkteams/internal/app/lifecycle"
	"fkteams/internal/app/version"
	bootstrapchannels "fkteams/internal/bootstrap/channels"
	bootstrapskills "fkteams/internal/bootstrap/skills"
	"fkteams/internal/runtime/env"
	"fkteams/internal/runtime/log"
	"fmt"
	"os"
	"os/exec"
)

// RunServer 创建服务应用，装配后台服务和传输入口。
func (d *ExecutionDependencies) RunServer(ctx context.Context, mode httpserver.Mode, opts *httpserver.ServeOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := config.Get()

	app := lifecycle.New()
	appCfg := app.Config()
	state := app.State()

	// 注册关闭回调供 API 调用
	control := lifecycle.NewControl(func() { app.Shutdown() }, restartProcess)

	schedulerProvider := d.configureServices(app)

	host := "127.0.0.1"
	if cfg.Server.Host != "" {
		host = cfg.Server.Host
	}
	port := cfg.Server.Port
	if opts != nil {
		if opts.Host != "" {
			host = opts.Host
		}
		if opts.Port > 0 {
			port = opts.Port
		}
	}

	runtime := handler.NewRuntime(handler.RuntimeOptions{
		ExecutionGate: d.ExecutionGate,
		Runtime:       d.Runtime, Interrupt: d.InterruptRuntime, HookBus: d.HookBus,
		AgentRegistry: d.AgentRegistry, ToolRegistry: d.ToolRegistry, ToolDisplays: d.ToolDisplayRegistry,
		ModelRegistry: d.ModelRegistry, Providers: d.ModelProviderRegistry,
		SkillProviders: bootstrapskills.NewDefaultProviderRegistry(), Control: control,
	})

	httpSvc := httpserver.NewService(httpserver.ServiceOptions{
		Listen: httpserver.ServeOptions{Host: host, Port: port}, Mode: mode, LogLevel: cfg.Server.LogLevel,
		State: state, Runtime: runtime, SchedulerProvider: schedulerProvider,
	})
	app.RegisterService(httpSvc)

	// 注册消息通道
	if svc, err := channel.SetupWithOptions(cfg.Channels.List(), channel.SetupOptions{
		State:             state,
		SchedulerProvider: schedulerProvider,
		ExecutionGate:     d.ExecutionGate,
		FactoryRegistry:   bootstrapchannels.RegisterDefaults(),
	}); err != nil {
		return fmt.Errorf("setup channels: %w", err)
	} else if svc != nil {
		runtime.ResetChannels = svc.ResetRunners
		app.RegisterService(svc)
	}

	app.OnReady(func(ctx context.Context) error {
		addr := httpSvc.Addr()
		if mode == httpserver.ModeAPI {
			fmt.Printf("欢迎来到非空小队 - API 服务模式: %s\n", version.Get())
			fmt.Printf("当前服务运行在 [%s]\n", addr)
		} else {
			fmt.Printf("欢迎来到非空小队 - 服务端模式: %s\n", version.Get())
			fmt.Printf("当前服务运行在 [%s]\n", addr)
			fmt.Printf("前端页面地址: http://%s\n", addr)
		}
		if appCfg.MemoryEnabled {
			fmt.Println("全局长期记忆已启用")
		}
		return nil
	})

	app.OnCleanup(func(ctx context.Context) error {
		state.RunProcessCleanup()

		// 如有待重启请求，启动新进程
		if err := control.ExecutePendingRestart(); err != nil {
			return fmt.Errorf("restart application: %w", err)
		}

		fmt.Printf("服务退出\n")
		return nil
	})

	if err := app.Run(d.Context(ctx)); err != nil {
		return fmt.Errorf("application error: %w", err)
	}
	return nil
}

func restartProcess() error {
	if os.Getpid() == 1 {
		log.Info("skip self-restart: running as PID 1 (container), relying on restart policy")
		return nil
	}
	if env.Get(env.NoSelfRestart) != "" {
		log.Info("skip self-restart: FEIKONG_NO_SELF_RESTART is set, relying on external process manager")
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	log.Info("starting new process for restart")
	return cmd.Start()
}
