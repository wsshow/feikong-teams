// Package lifecycle 提供应用程序生命周期管理框架。
// 阶段: Init → Setup → Start → Ready → [wait] → PreStop → Stop → Cleanup
package lifecycle

import (
	"context"
	"fkteams/log"
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
	hooks        map[Phase][]HookFunc // 各阶段的钩子函数
	services     []Service            // 注册的后台服务
	mu           sync.Mutex           // 保护并发访问
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
		hooks:  make(map[Phase][]HookFunc),
		exitCh: make(chan os.Signal, 1),
	}
}

// Config 返回应用配置
func (app *Application) Config() *AppConfig {
	return app.config
}

// CurrentPhase 返回当前生命周期阶段
func (app *Application) CurrentPhase() Phase {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.currentPhase
}

// OnPhase 注册指定阶段的钩子函数
func (app *Application) OnPhase(phase Phase, hook HookFunc) {
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
	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := app.executePhase(appCtx, PhaseInit); err != nil {
		return fmt.Errorf("init failed: %w", err)
	}

	if err := app.executePhase(appCtx, PhaseSetup); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	// 启动所有注册的服务
	if err := app.startServices(appCtx); err != nil {
		app.stopServices(appCtx)
		return fmt.Errorf("start failed: %w", err)
	}
	if err := app.executePhase(appCtx, PhaseStart); err != nil {
		app.stopServices(appCtx)
		return fmt.Errorf("start hooks failed: %w", err)
	}

	if err := app.executePhase(appCtx, PhaseReady); err != nil {
		app.stopServices(appCtx)
		return fmt.Errorf("ready failed: %w", err)
	}

	// 阻塞等待退出信号
	app.waitForExit(appCtx)
	cancel()

	if err := app.executePhase(context.Background(), PhasePreStop); err != nil {
		log.Printf("[lifecycle] prestop error: %v", err)
	}

	// 逆序停止服务（LIFO）
	app.stopServices(context.Background())
	if err := app.executePhase(context.Background(), PhaseStop); err != nil {
		log.Printf("[lifecycle] stop hooks error: %v", err)
	}

	if err := app.executePhase(context.Background(), PhaseCleanup); err != nil {
		log.Printf("[lifecycle] cleanup error: %v", err)
	}

	return nil
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

// startServices 按注册顺序启动所有服务
func (app *Application) startServices(ctx context.Context) error {
	app.mu.Lock()
	services := make([]Service, len(app.services))
	copy(services, app.services)
	app.mu.Unlock()

	for _, svc := range services {
		log.Printf("[lifecycle] starting service: %s", svc.Name())
		if err := svc.Start(ctx); err != nil {
			return fmt.Errorf("service %s start failed: %w", svc.Name(), err)
		}
		log.Printf("[lifecycle] service started: %s", svc.Name())
	}
	return nil
}

// stopServices 按注册逆序停止所有服务（LIFO）
func (app *Application) stopServices(ctx context.Context) {
	app.mu.Lock()
	services := make([]Service, len(app.services))
	copy(services, app.services)
	app.mu.Unlock()

	for i := len(services) - 1; i >= 0; i-- {
		svc := services[i]
		log.Printf("[lifecycle] stopping service: %s", svc.Name())
		if err := svc.Stop(ctx); err != nil {
			log.Printf("[lifecycle] service %s stop error: %v", svc.Name(), err)
		} else {
			log.Printf("[lifecycle] service stopped: %s", svc.Name())
		}
	}
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
