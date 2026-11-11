// Package lifecycle 提供应用程序生命周期管理框架。
// 阶段: Init → Setup → Start → Ready → [wait] → PreStop → Stop → Cleanup
package lifecycle

import (
	"context"
	"errors"
	"fkteams/internal/app/appstate"
	"fkteams/internal/runtime/log"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Phase 生命周期阶段
type Phase int

const (
	PhaseInit    Phase = iota // 初始化阶段
	PhaseSetup                // 配置准备阶段
	PhaseStart                // 启动阶段
	PhaseReady                // 就绪阶段
	PhasePreStop              // 停止前阶段
	PhaseStop                 // 停止阶段
	PhaseCleanup              // 清理阶段
)

// String 返回阶段名称
func (p Phase) String() string {
	switch p {
	case PhaseInit:
		return "Init"
	case PhaseSetup:
		return "Setup"
	case PhaseStart:
		return "Start"
	case PhaseReady:
		return "Ready"
	case PhasePreStop:
		return "PreStop"
	case PhaseStop:
		return "Stop"
	case PhaseCleanup:
		return "Cleanup"
	default:
		return fmt.Sprintf("Unknown(%d)", p)
	}
}

// HookFunc 生命周期钩子函数
type HookFunc func(ctx context.Context) error

// Service 可插拔的后台服务接口
type Service interface {
	// Name 返回服务名称
	Name() string
	// Start 启动服务
	Start(ctx context.Context) error
	// Stop 停止服务
	Stop(ctx context.Context) error
}

// Application 应用程序生命周期管理器
type Application struct {
	config       *AppConfig           // 应用配置
	state        *appstate.State      // 应用运行时状态
	hooks        map[Phase][]HookFunc // 各阶段的钩子函数
	services     []Service            // 注册的后台服务
	started      []Service            // 本轮已成功启动的服务
	mu           sync.Mutex           // 保护并发访问
	running      bool                 // 是否正在执行生命周期
	currentPhase Phase                // 当前生命周期阶段
	exitCh       chan os.Signal       // 退出信号通道
}

// New 创建 Application 实例
func New(opts ...Option) *Application {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &Application{
		config: cfg,
		state:  appstate.New(),
		hooks:  make(map[Phase][]HookFunc),
		exitCh: make(chan os.Signal, 1),
	}
}

// Config 返回应用配置
func (app *Application) Config() *AppConfig {
	return app.config
}

// State 返回应用实例的运行时状态。
func (app *Application) State() *appstate.State {
	return app.state
}

// CurrentPhase 返回当前生命周期阶段
func (app *Application) CurrentPhase() Phase {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.currentPhase
}

// OnPhase 注册指定阶段的钩子函数
func (app *Application) OnPhase(phase Phase, hook HookFunc) {
	if hook == nil {
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	app.hooks[phase] = append(app.hooks[phase], hook)
}

// OnInit 注册初始化阶段钩子
func (app *Application) OnInit(hook HookFunc) { app.OnPhase(PhaseInit, hook) }

// OnSetup 注册配置准备阶段钩子
func (app *Application) OnSetup(hook HookFunc) { app.OnPhase(PhaseSetup, hook) }

// OnStart 注册启动阶段钩子
func (app *Application) OnStart(hook HookFunc) { app.OnPhase(PhaseStart, hook) }

// OnReady 注册就绪阶段钩子
func (app *Application) OnReady(hook HookFunc) { app.OnPhase(PhaseReady, hook) }

// OnPreStop 注册停止前阶段钩子
func (app *Application) OnPreStop(hook HookFunc) { app.OnPhase(PhasePreStop, hook) }

// OnStop 注册停止阶段钩子
func (app *Application) OnStop(hook HookFunc) { app.OnPhase(PhaseStop, hook) }

// OnCleanup 注册清理阶段钩子
func (app *Application) OnCleanup(hook HookFunc) { app.OnPhase(PhaseCleanup, hook) }

// RegisterService 注册服务（Start 时按序启动，Stop 时逆序停止）
func (app *Application) RegisterService(svc Service) {
	if svc == nil {
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	app.services = append(app.services, svc)
}

// ExitCh 返回退出信号通道
func (app *Application) ExitCh() chan os.Signal {
	return app.exitCh
}

// Run 执行完整生命周期，阻塞直到收到退出信号或 context 取消
func (app *Application) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	app.mu.Lock()
	if app.running {
		app.mu.Unlock()
		return fmt.Errorf("application lifecycle is already running")
	}
	app.running = true
	app.started = nil
	app.mu.Unlock()
	defer func() {
		app.mu.Lock()
		app.running = false
		app.mu.Unlock()
	}()

	appCtx, cancel := context.WithCancel(ctx)
	appCtx = appstate.WithState(appCtx, app.state)
	defer cancel()

	if err := app.executePhase(appCtx, PhaseInit); err != nil {
		cancel()
		return errors.Join(fmt.Errorf("init failed: %w", err), app.cleanupStartupFailure())
	}

	if err := app.executePhase(appCtx, PhaseSetup); err != nil {
		cancel()
		return errors.Join(fmt.Errorf("setup failed: %w", err), app.cleanupStartupFailure())
	}

	// 启动所有注册的服务
	if err := app.startServices(appCtx); err != nil {
		cancel()
		return errors.Join(fmt.Errorf("start failed: %w", err), app.cleanupStartupFailure())
	}
	if err := app.executePhase(appCtx, PhaseStart); err != nil {
		cancel()
		return errors.Join(fmt.Errorf("start hooks failed: %w", err), app.cleanupStartupFailure())
	}

	if err := app.executePhase(appCtx, PhaseReady); err != nil {
		cancel()
		return errors.Join(fmt.Errorf("ready failed: %w", err), app.cleanupStartupFailure())
	}

	// 阻塞等待退出信号
	app.waitForExit(appCtx)
	cancel()
	return app.shutdownStartedServices()
}

func (app *Application) cleanupStartupFailure() error {
	ctx, cancel := app.shutdownContext()
	defer cancel()
	stopErr := app.stopServices(ctx)
	return errors.Join(stopErr, app.executePhaseBounded(ctx, PhaseCleanup))
}

func (app *Application) shutdownStartedServices() error {
	ctx, cancel := app.shutdownContext()
	defer cancel()
	preErr := app.executePhaseBounded(ctx, PhasePreStop)
	serviceErr := app.stopServices(ctx)
	stopErr := app.executePhaseBounded(ctx, PhaseStop)
	cleanupErr := app.executePhaseBounded(ctx, PhaseCleanup)
	return errors.Join(preErr, serviceErr, stopErr, cleanupErr)
}

func (app *Application) shutdownContext() (context.Context, context.CancelFunc) {
	timeout := app.config.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// Shutdown 主动触发优雅退出
func (app *Application) Shutdown() {
	select {
	case app.exitCh <- syscall.SIGTERM:
	default:
	}
}

// executePhase 执行指定阶段的所有钩子
func (app *Application) executePhase(ctx context.Context, phase Phase) error {
	app.mu.Lock()
	app.currentPhase = phase
	hooks := make([]HookFunc, len(app.hooks[phase]))
	copy(hooks, app.hooks[phase])
	app.mu.Unlock()

	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			return fmt.Errorf("[%s] hook error: %w", phase, err)
		}
	}
	return nil
}

func (app *Application) executePhaseBounded(ctx context.Context, phase Phase) error {
	app.mu.Lock()
	app.currentPhase = phase
	hooks := append([]HookFunc(nil), app.hooks[phase]...)
	app.mu.Unlock()

	var errs []error
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			errs = append(errs, fmt.Errorf("[%s] hook error: %w", phase, err))
		}
	}
	return errors.Join(errs...)
}

// startServices 按注册顺序启动所有服务
func (app *Application) startServices(ctx context.Context) error {
	app.mu.Lock()
	services := make([]Service, len(app.services))
	copy(services, app.services)
	app.mu.Unlock()

	for _, svc := range services {
		if err := svc.Start(ctx); err != nil {
			return fmt.Errorf("service %s start failed: %w", svc.Name(), err)
		}
		app.mu.Lock()
		app.started = append(app.started, svc)
		app.mu.Unlock()
		log.Printf("[lifecycle] service started: %s", svc.Name())
	}
	return nil
}

// stopServices 按注册逆序停止所有服务（LIFO）
func (app *Application) stopServices(ctx context.Context) error {
	app.mu.Lock()
	services := make([]Service, len(app.started))
	copy(services, app.started)
	app.started = nil
	app.mu.Unlock()

	var errs []error
	for i := len(services) - 1; i >= 0; i-- {
		svc := services[i]
		err := svc.Stop(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("service %s stop failed: %w", svc.Name(), err))
		} else {
			log.Printf("[lifecycle] service stopped: %s", svc.Name())
		}
	}
	return errors.Join(errs...)
}

// waitForExit 等待退出信号（系统信号或 context 取消）
func (app *Application) waitForExit(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	if len(app.config.ExitSignals) > 0 {
		signal.Notify(sigCh, app.config.ExitSignals...)
	}

	select {
	case sig := <-sigCh:
		log.Printf("[lifecycle] received signal: %v", sig)
	case sig := <-app.exitCh:
		log.Printf("[lifecycle] received exit signal: %v", sig)
	case <-ctx.Done():
		log.Printf("[lifecycle] context cancelled")
	}

	signal.Stop(sigCh)
}
