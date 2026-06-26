package channel

import (
	"context"
	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/config"
	appschedule "fkteams/internal/app/schedule"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/log"
	modelregistry "fkteams/internal/runtime/model"
	"fmt"
)

// Setup 从配置中创建并注册所有启用的通道，返回可注册到 lifecycle 的 Service
// 如果没有启用任何通道则返回 nil
func Setup(entries []config.ChannelEntry) (*Service, error) {
	return SetupWithState(entries, nil)
}

// SetupWithState 从配置中创建通道，并把应用状态传给通道桥接器。
func SetupWithState(entries []config.ChannelEntry, state *appstate.State) (*Service, error) {
	return SetupWithOptions(entries, SetupOptions{State: state})
}

// SetupOptions 描述通道服务的显式依赖。
type SetupOptions struct {
	State             *appstate.State
	SchedulerProvider func() *appschedule.Service
}

// SetupWithOptions 从配置中创建通道，并注入入口依赖。
func SetupWithOptions(entries []config.ChannelEntry, options SetupOptions) (*Service, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	mgr := NewManager(nil)
	historyDir := appdata.SessionsDir()
	sessions := eventlog.NewSessionHistoryManager()

	// 为每个通道创建独立的 Bridge（支持不同 mode）
	bridges := make(map[string]*Bridge)
	for _, entry := range entries {
		bridge := NewBridgeWithOptions(mgr, entry.Mode, BridgeOptions{
			State:             options.State,
			HistoryDir:        historyDir,
			Sessions:          sessions,
			SchedulerProvider: options.SchedulerProvider,
		})
		bridges[entry.Name] = bridge
	}

	// 设置统一消息处理：根据通道名称路由到对应的 Bridge
	mgr.SetHandler(func(ctx context.Context, chatID, senderID string, msg Message, isGroup bool) {
		channelName := "unknown"
		if name, ok := ctx.Value(channelNameKey{}).(string); ok {
			channelName = name
		}
		if bridge, ok := bridges[channelName]; ok {
			bridge.HandleMessage(ctx, chatID, senderID, msg, isGroup)
		}
	})

	for _, entry := range entries {
		if err := mgr.Register(entry.Name, ChannelConfig{
			Enabled: true,
			Extra:   entry.Extra,
		}); err != nil {
			return nil, fmt.Errorf("register channel %s: %w", entry.Name, err)
		}
		log.Printf("[channels] registered channel: %s (mode=%s)", entry.Name, entry.Mode)
	}

	bridgeList := make([]*Bridge, 0, len(bridges))
	for _, b := range bridges {
		bridgeList = append(bridgeList, b)
	}
	return NewService(mgr, bridgeList...), nil
}

// Service 实现 lifecycle.Service 接口，管理所有通道的生命周期
type Service struct {
	manager *Manager
	bridges []*Bridge
}

// NewService 创建通道服务
func NewService(manager *Manager, bridges ...*Bridge) *Service {
	return &Service{manager: manager, bridges: bridges}
}

// ResetRunners 重置当前通道服务实例的 runner 缓存。
func (s *Service) ResetRunners() {
	if s == nil {
		return
	}
	for _, bridge := range s.bridges {
		bridge.ResetRunner()
	}
}

// Name 返回服务名称
func (s *Service) Name() string { return "channels" }

// Start 启动所有通道
func (s *Service) Start(ctx context.Context) error {
	engine, _ := runtimeport.EngineFromContext(ctx)
	interrupt, _ := runtimeport.InterruptRuntimeFromContext(ctx)
	models, _ := modelregistry.RegistryFromContext(ctx)
	tools, _ := apptools.RegistryFromContext(ctx)
	for _, bridge := range s.bridges {
		bridge.SetRuntimeDependencies(engine, interrupt, models, tools)
	}
	log.Printf("[channels] starting all channels...")
	return s.manager.StartAll(ctx)
}

// Stop 停止所有通道
func (s *Service) Stop(ctx context.Context) error {
	log.Printf("[channels] stopping all channels...")
	s.manager.StopAll(ctx)
	return nil
}

// Manager 返回底层管理器
func (s *Service) Manager() *Manager {
	return s.manager
}
