package channels

import (
	"context"
	"fkteams/config"
	"fkteams/log"
	"fmt"
)

// Setup 从配置中创建并注册所有启用的通道，返回可注册到 lifecycle 的 Service
// 如果没有启用任何通道则返回 nil
func Setup(entries []config.ChannelEntry) (*Service, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	mgr := NewManager(nil)

	// 为每个通道创建独立的 Bridge（支持不同 mode）
	bridges := make(map[string]*Bridge)
	for _, entry := range entries {
		bridge := NewBridge(mgr, entry.Mode)
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

	return NewService(mgr), nil
}

// Service 实现 lifecycle.Service 接口，管理所有通道的生命周期
type Service struct {
	manager *Manager
}

// NewService 创建通道服务
func NewService(manager *Manager) *Service {
	return &Service{manager: manager}
}

// Name 返回服务名称
func (s *Service) Name() string { return "channels" }

// Start 启动所有通道
func (s *Service) Start(ctx context.Context) error {
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
