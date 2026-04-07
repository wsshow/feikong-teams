// Package taskstream 提供统一的任务事件流管理，支持 Push（WebSocket）和 Pull（SSE）两种消费模式。
//
// 设计原则：
//   - 事件日志：所有事件带递增 ID 持久保存，支持任意 offset 重连
//   - Push 订阅：至多一个 Subscriber（如 WS 连接），事件实时推送
//   - Pull 监听：多个 Watcher（如 SSE 连接），通过通知+轮询获取事件
//   - 优雅断连：可配置 GracePeriod，断连后任务继续运行一段时间
//   - TTL 清理：任务完成后保留一段时间供重连客户端拉取残余事件
package taskstream

import (
	"log"
	"sync"
	"time"
)

// Event 是传输无关的任务事件
type Event = map[string]any

// IndexedEvent 带递增 ID 的事件，支持 offset 断点续传
type IndexedEvent struct {
	ID   uint64 `json:"id"`
	Data Event  `json:"data"`
}

// Subscriber 接收推送事件（Push 模式，如 WebSocket 连接）
type Subscriber interface {
	WriteEvent(event Event) error
}

// FuncSubscriber 将函数适配为 Subscriber 接口
type FuncSubscriber func(any) error

func (f FuncSubscriber) WriteEvent(event Event) error { return f(event) }

// StreamConfig 创建 Stream 时的配置
type StreamConfig struct {
	SessionID   string
	Cancel      func()        // 取消任务的函数
	GracePeriod time.Duration // 断连后任务继续运行的宽限期（0=不因断连取消）
	CleanupTTL  time.Duration // 任务完成后保留数据的时间（0=立即清理）

	// 元数据（可选）
	Mode      string // 协作模式
	AgentName string // 智能体名称
}

// Stream 代表单个任务的事件流，是事件投递的核心抽象。
// 同时支持 Push（Subscriber）和 Pull（Watcher + EventsSince）两种消费方式。
type Stream struct {
	mu     sync.Mutex
	config StreamConfig

	// 事件日志（带递增 ID，支持断点续传）
	events []IndexedEvent
	nextID uint64

	// Push 订阅者（至多一个，如 WS 连接）
	sub           Subscriber
	subEpoch      uint64 // 版本号，防止旧连接延迟 Unsubscribe 覆盖新连接
	lastPushedIdx int    // 最后一次成功推送到订阅者的事件索引（Subscribe 回放时只发送此后的事件）

	// Pull 监听者（多个，如 SSE 连接）
	watchers    map[uint64]chan struct{}
	watcherNext uint64

	// 生命周期
	graceTimer  *time.Timer
	done        bool
	status      string // "processing", "completed", "error", "cancelled"
	createdAt   time.Time
	doneAt      time.Time
	interruptCh chan any // 审批/ask 通道

	// 所属 Manager 引用（用于 grace timer 自动移除）
	manager *Manager
}

// Publish 发布事件到流。有订阅者时推送，同时写入日志，通知所有监听者。
func (s *Stream) Publish(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 写入事件日志
	id := s.nextID
	s.nextID++
	s.events = append(s.events, IndexedEvent{ID: id, Data: event})

	// 推送给 Push 订阅者
	if s.sub != nil {
		if err := s.sub.WriteEvent(event); err != nil {
			s.sub = nil // 推送失败，自动解绑（连接可能已断开）
		} else {
			s.lastPushedIdx = len(s.events)
		}
	}

	// 通知所有 Pull 监听者
	for _, ch := range s.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Subscribe 绑定 Push 订阅者并回放错过的事件。
// 返回 (false, 0) 表示流已结束/过期，调用方需自行通知客户端。
// 返回 (true, epoch) 表示绑定成功，调用方应保存 epoch 用于后续 Unsubscribe。
func (s *Stream) Subscribe(sub Subscriber) (bool, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		// 流已结束，不回放事件（避免重连时重复渲染）
		// 已完成的任务数据应通过历史 API 获取
		return false, 0
	}

	// 停止宽限期计时器
	if s.graceTimer != nil {
		s.graceTimer.Stop()
		s.graceTimer = nil
	}

	s.subEpoch++

	// 仅回放自上次成功推送以来的错过事件
	// 首次 Subscribe: lastPushedIdx=0 → 回放全部已有事件
	// 重连 Subscribe: lastPushedIdx=N → 仅回放断连期间积压的事件
	for _, e := range s.events[s.lastPushedIdx:] {
		_ = sub.WriteEvent(e.Data)
	}
	s.lastPushedIdx = len(s.events)
	s.sub = sub
	return true, s.subEpoch
}

// Unsubscribe 解绑 Push 订阅者，启动宽限期计时器。
// epoch 参数防止旧连接的延迟 Unsubscribe 解绑新连接的订阅者。
func (s *Stream) Unsubscribe(epoch uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.subEpoch != epoch {
		return // epoch 不匹配，说明新连接已 Subscribe，忽略旧连接的 Unsubscribe
	}

	s.sub = nil

	if s.done {
		return
	}

	if s.config.GracePeriod > 0 {
		expectedEpoch := s.subEpoch
		s.graceTimer = time.AfterFunc(s.config.GracePeriod, func() {
			s.mu.Lock()
			if s.subEpoch != expectedEpoch {
				// 宽限期内有新订阅者绑定，不取消
				s.mu.Unlock()
				return
			}
			s.done = true
			s.doneAt = time.Now()
			s.mu.Unlock()
			log.Printf("[taskstream] grace period expired, cancelling: session=%s", s.config.SessionID)
			s.config.Cancel()
			if s.manager != nil {
				s.manager.RemoveIfMatch(s.config.SessionID, s)
			}
		})
	} else if s.config.GracePeriod == 0 {
		// 无宽限期：SSE 模式，不取消任务（依赖 context 取消）
	}
}

// SubEpoch 返回当前订阅者版本号（Unsubscribe 时需要传入）
func (s *Stream) SubEpoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subEpoch
}

// Watch 返回事件通知通道和取消函数（Pull 模式，用于 SSE）。
// 当有新事件发布时，通知通道会收到信号。
func (s *Stream) Watch() (<-chan struct{}, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan struct{}, 1)
	id := s.watcherNext
	s.watcherNext++
	if s.watchers == nil {
		s.watchers = make(map[uint64]chan struct{})
	}
	s.watchers[id] = ch

	unwatch := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.watchers, id)
	}
	return ch, unwatch
}

// EventsSince 返回从 offset 开始的所有事件（Pull 模式，用于 SSE 和 HTTP 轮询）。
func (s *Stream) EventsSince(offset uint64) []IndexedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 二分查找起始位置
	start := 0
	for start < len(s.events) && s.events[start].ID < offset {
		start++
	}
	if start >= len(s.events) {
		return nil
	}
	result := make([]IndexedEvent, len(s.events)-start)
	copy(result, s.events[start:])
	return result
}

// Done 标记流已完成。通知所有监听者，停止宽限期计时器。
func (s *Stream) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.done = true
	s.doneAt = time.Now()
	if s.graceTimer != nil {
		s.graceTimer.Stop()
		s.graceTimer = nil
	}

	// 通知所有 Pull 监听者（使其退出等待循环）
	for _, ch := range s.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Cancel 取消底层任务
func (s *Stream) Cancel() {
	s.config.Cancel()
}

// IsDone 检查流是否已完成
func (s *Stream) IsDone() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

// --- 状态与元数据 ---

// SetStatus 设置任务状态
func (s *Stream) SetStatus(status string) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

// Status 返回任务状态
func (s *Stream) Status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Mode 返回协作模式
func (s *Stream) Mode() string {
	return s.config.Mode
}

// AgentName 返回智能体名称
func (s *Stream) AgentName() string {
	return s.config.AgentName
}

// CreatedAt 返回流创建时间
func (s *Stream) CreatedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createdAt
}

// DoneAt 返回流完成的时间
func (s *Stream) DoneAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doneAt
}

// SessionID 返回关联的会话 ID
func (s *Stream) SessionID() string {
	return s.config.SessionID
}

// InterruptCh 返回审批/ask 中断通道
func (s *Stream) InterruptCh() chan any {
	return s.interruptCh
}

// EventCount 返回事件日志中的事件数量
func (s *Stream) EventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}
