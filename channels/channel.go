// Package channels 提供多平台消息通道的抽象接口和管理器
package channels

import (
	"context"
	"fmt"
	"sync"
)

// MessageType 消息类型
type MessageType int

const (
	MsgText  MessageType = iota // 文本消息
	MsgImage                    // 图片消息
	MsgAudio                    // 语音消息
	MsgVideo                    // 视频消息
	MsgFile                     // 文件消息
)

// Attachment 消息附件（接收或发送的多媒体内容）
type Attachment struct {
	Type     MessageType // 附件类型
	URL      string      // 文件 URL（HTTP/HTTPS）
	FileName string      // 文件名
}

// TypeName 返回附件类型的中文描述
func (a Attachment) TypeName() string {
	switch a.Type {
	case MsgImage:
		return "图片"
	case MsgAudio:
		return "语音"
	case MsgVideo:
		return "视频"
	case MsgFile:
		return "文件"
	default:
		return "附件"
	}
}

// Message 统一消息结构（用于接收和发送）
type Message struct {
	Type        MessageType  // 消息类型
	Content     string       // 文本内容
	Attachments []Attachment // 附件列表（图片、语音、视频、文件等）
}

// Channel 消息通道接口，所有平台（QQ、微信、Telegram 等）都需要实现此接口
type Channel interface {
	// Name 返回通道名称（如 "qq"、"wechat"）
	Name() string
	// Start 启动通道，开始接收消息
	Start(ctx context.Context) error
	// Stop 停止通道
	Stop(ctx context.Context) error
	// Send 向指定会话发送消息（支持文本和多媒体）
	Send(ctx context.Context, chatID string, msg Message) error
	// IsRunning 返回通道是否在运行中
	IsRunning() bool
}

// MessageHandler 消息处理回调，由 Manager 注入到 Channel 中
// chatID: 会话标识（私聊用户ID / 群ID）
// senderID: 发送者ID
// msg: 统一消息结构（包含文本和附件）
// isGroup: 是否为群消息
type MessageHandler func(ctx context.Context, chatID, senderID string, msg Message, isGroup bool)

// Factory 通道工厂函数，接收配置创建通道实例
type Factory func(cfg ChannelConfig, handler MessageHandler) (Channel, error)

// ChannelConfig 通道通用配置
type ChannelConfig struct {
	Enabled bool              `toml:"enabled"`
	Extra   map[string]string `toml:"extra"` // 平台特定配置（如 app_id、app_secret 等）
}

var (
	factoryMu sync.RWMutex
	factories = make(map[string]Factory)
)

// RegisterFactory 注册通道工厂（通常在 init() 中调用）
func RegisterFactory(name string, factory Factory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories[name] = factory
}

// GetFactory 获取已注册的工厂
func GetFactory(name string) (Factory, bool) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	f, ok := factories[name]
	return f, ok
}

// Manager 通道管理器，管理所有通道的生命周期和消息路由
type Manager struct {
	channels map[string]Channel
	handler  MessageHandler
	mu       sync.RWMutex
}

// NewManager 创建通道管理器
func NewManager(handler MessageHandler) *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		handler:  handler,
	}
}

// SetHandler 设置消息处理回调（用于解决 Manager 和 Bridge 的循环依赖）
func (m *Manager) SetHandler(handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
}

// Register 使用配置创建并注册通道
func (m *Manager) Register(name string, cfg ChannelConfig) error {
	if !cfg.Enabled {
		return nil
	}
	factory, ok := GetFactory(name)
	if !ok {
		return fmt.Errorf("unknown channel: %s", name)
	}
	ch, err := factory(cfg, m.handler)
	if err != nil {
		return fmt.Errorf("create channel %s: %w", name, err)
	}
	m.mu.Lock()
	m.channels[name] = ch
	m.mu.Unlock()
	return nil
}

// StartAll 启动所有已注册的通道
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, ch := range m.channels {
		if err := ch.Start(ctx); err != nil {
			return fmt.Errorf("start channel %s: %w", name, err)
		}
	}
	return nil
}

// StopAll 停止所有通道
func (m *Manager) StopAll(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.channels {
		_ = ch.Stop(ctx)
	}
}

// Get 获取指定通道
func (m *Manager) Get(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

// SendText 向指定通道的会话发送文本（便捷方法）
func (m *Manager) SendText(ctx context.Context, channelName, chatID, text string) error {
	return m.Send(ctx, channelName, chatID, Message{Type: MsgText, Content: text})
}

// Send 向指定通道的会话发送消息
func (m *Manager) Send(ctx context.Context, channelName, chatID string, msg Message) error {
	ch, ok := m.Get(channelName)
	if !ok {
		return fmt.Errorf("channel not found: %s", channelName)
	}
	return ch.Send(ctx, chatID, msg)
}
