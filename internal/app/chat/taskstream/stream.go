// Package taskstream 提供统一的任务事件流管理，支持 Push（WebSocket）和 Pull（SSE）两种消费模式。
//
// 设计原则：
//   - 事件日志：所有事件带递增 ID 持久保存，支持任意 offset 重连
//   - Push 订阅：多个 Subscriber（如多端 WS 连接），事件实时推送
//   - Pull 监听：多个 Watcher（如 SSE 连接），通过通知+轮询获取事件
//   - 连接分离：断连只解绑订阅者，不影响后台任务
//   - TTL 清理：任务完成后保留一段时间供重连客户端拉取残余事件
package taskstream

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	domainmessage "fkteams/internal/domain/message"

	"github.com/google/uuid"
)

// Event 是传输无关的任务通知事件。
type Event map[string]any

func (e Event) With(key string, value any) Event {
	e[key] = value
	return e
}

func (e Event) WithTurn(runID, turnID string) Event {
	if runID != "" {
		e["run_id"] = runID
	}
	if turnID != "" {
		e["turn_id"] = turnID
	}
	return e
}

func (e Event) WithContentParts(parts []domainmessage.ContentPart) Event {
	if len(parts) > 0 {
		e["content_parts"] = append([]domainmessage.ContentPart(nil), parts...)
	}
	return e
}

// IndexedEvent 带递增 ID 的事件，支持 offset 断点续传
type IndexedEvent struct {
	ID   uint64 `json:"id"`
	Data Event  `json:"data"`
}

// SubscriptionID 标识一个 Push 订阅者。
type SubscriptionID uint64

// InterruptKind 描述当前等待的人工输入类型。
type InterruptKind string

const (
	InterruptNone     InterruptKind = ""
	InterruptApproval InterruptKind = "approval"
	InterruptAsk      InterruptKind = "ask"
)

type QueueKind string

const (
	QueueFollowUp QueueKind = "follow_up"
	QueueSteering QueueKind = "steering"
)

type QueuedMessage struct {
	ID          string                      `json:"queue_id"`
	Kind        QueueKind                   `json:"kind"`
	Text        string                      `json:"text,omitempty"`
	Parts       []domainmessage.ContentPart `json:"parts,omitempty"`
	DisplayText string                      `json:"display_text,omitempty"`
	CreatedAt   time.Time                   `json:"created_at"`
	UpdatedAt   time.Time                   `json:"updated_at,omitempty"`
}

func (m QueuedMessage) Message() domainmessage.Message {
	msg := domainmessage.Message{
		Role:    domainmessage.RoleUser,
		Content: m.Text,
	}
	if len(m.Parts) > 0 {
		msg.Content = ""
		msg.ContentParts = append([]domainmessage.ContentPart(nil), m.Parts...)
	}
	return msg
}

// Subscriber 接收推送事件（Push 模式，如 WebSocket 连接）
type Subscriber interface {
	WriteEvent(event Event) error
}

// FuncSubscriber 将函数适配为 Subscriber 接口
type FuncSubscriber func(Event) error

func (f FuncSubscriber) WriteEvent(event Event) error { return f(event) }

const subscriberQueueSize = 256

type streamSubscription struct {
	target Subscriber
	queue  chan Event
	abort  chan struct{}
	state  atomic.Uint32
}

func newStreamSubscription(target Subscriber) *streamSubscription {
	return &streamSubscription{
		target: target,
		queue:  make(chan Event, subscriberQueueSize),
		abort:  make(chan struct{}),
	}
}

func (s *streamSubscription) enqueue(event Event) bool {
	if s == nil || s.state.Load() != 0 {
		return false
	}
	select {
	case s.queue <- event:
		return true
	case <-s.abort:
		return false
	default:
		return false
	}
}

func (s *streamSubscription) abortNow() {
	if s != nil && s.state.Swap(1) != 1 {
		close(s.abort)
	}
}

func (s *streamSubscription) finish() {
	if s != nil && s.state.CompareAndSwap(0, 2) {
		close(s.queue)
	}
}

func (s *streamSubscription) run(onError func()) {
	for {
		select {
		case <-s.abort:
			return
		case event, ok := <-s.queue:
			if !ok {
				return
			}
			if s.state.Load() == 1 {
				return
			}
			if err := s.target.WriteEvent(event); err != nil {
				onError()
				return
			}
		}
	}
}

// StreamConfig 创建 Stream 时的配置
type StreamConfig struct {
	SessionID  string
	Cancel     func()        // 取消任务的函数
	CleanupTTL time.Duration // 任务完成后保留数据的时间（0=立即清理）

	// 元数据（可选）
	Mode      string // 协作模式
	AgentName string // 智能体名称
	runID     string
	turnID    string
}

// Stream 代表单个任务的事件流，是事件投递的核心抽象。
// 同时支持 Push（Subscriber）和 Pull（Watcher + EventsSince）两种消费方式。
type Stream struct {
	mu     sync.Mutex
	config StreamConfig

	// 事件日志（带递增 ID，支持断点续传）
	events []IndexedEvent
	nextID uint64

	// Push 订阅者（多个，如多端 WS 连接）
	subs    map[SubscriptionID]*streamSubscription
	subNext SubscriptionID

	// Pull 监听者（多个，如 SSE 连接）
	watchers    map[uint64]chan struct{}
	watcherNext uint64

	// 生命周期
	done         bool
	status       string // "processing", "completed", "error", "cancelled"
	createdAt    time.Time
	doneAt       time.Time
	interruptCh  chan any // 审批/ask 通道
	pendingKind  InterruptKind
	pendingID    string
	pendingIDs   map[string]bool
	submittedIDs map[string]bool
	pendingAsks  map[string]chan any
	submitted    bool
	steering     []QueuedMessage
	followUps    []QueuedMessage

	// 所属 Manager 引用（用于 grace timer 自动移除）
	manager *Manager
	cancel  sync.Once
}

// Publish 发布事件到流。有订阅者时推送，同时写入日志，通知所有监听者。
func (s *Stream) Publish(event Event) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}

	// 写入事件日志
	id := s.nextID
	s.nextID++
	storedEvent := cloneEvent(event)
	storedEvent["stream_event_id"] = id
	s.events = append(s.events, IndexedEvent{ID: id, Data: storedEvent})

	// 入队后立即释放任务锁，网络写入由订阅者各自的 worker 串行完成。
	var overflowed []*streamSubscription
	for subID, sub := range s.subs {
		if !sub.enqueue(cloneEvent(storedEvent)) {
			delete(s.subs, subID)
			overflowed = append(overflowed, sub)
		}
	}

	// 通知所有 Pull 监听者
	for _, ch := range s.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()

	for _, sub := range overflowed {
		sub.abortNow()
	}
}

func (s *Stream) SetTurn(runID, turnID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.runID = runID
	s.config.turnID = turnID
}

func (s *Stream) CurrentTurn() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config.runID, s.config.turnID
}

// Subscribe 绑定 Push 订阅者并回放错过的事件。
// 返回 (false, 0) 表示流已结束/过期，调用方需自行通知客户端。
// 返回 (true, id) 表示绑定成功，调用方应保存 id 用于后续 Unsubscribe。
func (s *Stream) Subscribe(sub Subscriber, offset uint64) (bool, SubscriptionID) {
	if sub == nil {
		return false, 0
	}
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return false, 0
	}

	replay := s.eventsSinceLocked(offset)
	if s.subs == nil {
		s.subs = make(map[SubscriptionID]*streamSubscription)
	}
	s.subNext++
	subID := s.subNext
	subscription := newStreamSubscription(sub)
	s.subs[subID] = subscription
	s.mu.Unlock()

	// 回放可能包含网络写入，不能占用任务总锁。回放期间的新事件先进入该订阅的有界队列。
	for _, event := range replay {
		if subscription.state.Load() == 1 || sub.WriteEvent(event.Data) != nil {
			s.removeSubscription(subID, subscription)
			return false, 0
		}
	}
	if subscription.state.Load() == 1 {
		return false, 0
	}
	if subscription.state.Load() == 2 {
		for event := range subscription.queue {
			if sub.WriteEvent(event) != nil {
				subscription.abortNow()
				return false, 0
			}
		}
		return false, 0
	}
	go subscription.run(func() {
		s.removeSubscription(subID, subscription)
	})
	return true, subID
}

// Unsubscribe 解绑 Push 订阅者。断连只影响当前订阅，不取消后台任务。
func (s *Stream) Unsubscribe(id SubscriptionID) {
	s.mu.Lock()
	sub := s.subs[id]
	delete(s.subs, id)
	s.mu.Unlock()
	if sub != nil {
		sub.abortNow()
	}
}

func (s *Stream) removeSubscription(id SubscriptionID, expected *streamSubscription) {
	s.mu.Lock()
	if s.subs[id] == expected {
		delete(s.subs, id)
	}
	s.mu.Unlock()
	expected.abortNow()
}

// SubscriptionCount 返回当前 Push 订阅者数量。
func (s *Stream) SubscriptionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
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
	return s.eventsSinceLocked(offset)
}

func (s *Stream) eventsSinceLocked(offset uint64) []IndexedEvent {
	start := sort.Search(len(s.events), func(i int) bool { return s.events[i].ID >= offset })
	if start >= len(s.events) {
		return nil
	}
	result := make([]IndexedEvent, len(s.events)-start)
	for i, event := range s.events[start:] {
		result[i] = IndexedEvent{ID: event.ID, Data: cloneEvent(event.Data)}
	}
	return result
}

func cloneEvent(event Event) Event {
	cloned := make(Event, len(event)+1)
	for key, value := range event {
		cloned[key] = value
	}
	return cloned
}

// Done 标记流已完成。通知所有监听者。
func (s *Stream) Done() {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	s.doneAt = time.Now()
	subs := make([]*streamSubscription, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	s.subs = nil

	// 通知所有 Pull 监听者（使其退出等待循环）
	for _, ch := range s.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()

	// 已入队事件（包括 processing_end）按顺序排空后结束订阅 worker。
	for _, sub := range subs {
		sub.finish()
	}
	s.Cancel()
}

// Cancel 取消底层任务
func (s *Stream) Cancel() {
	s.cancel.Do(func() {
		if s.config.Cancel != nil {
			s.config.Cancel()
		}
	})
}

func (s *Stream) EnqueueMessage(msg QueuedMessage) QueuedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg = normalizeQueuedMessage(msg)
	switch msg.Kind {
	case QueueSteering:
		s.steering = append(s.steering, msg)
	default:
		msg.Kind = QueueFollowUp
		s.followUps = append(s.followUps, msg)
	}
	return msg
}

func normalizeQueuedMessage(msg QueuedMessage) QueuedMessage {
	now := time.Now()
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	if msg.Kind == "" {
		msg.Kind = QueueFollowUp
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}
	if msg.DisplayText == "" {
		msg.DisplayText = msg.Text
	}
	return msg
}

func (s *Stream) TakeSteeringMessages(limit int) []QueuedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.steering) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(s.steering) {
		limit = len(s.steering)
	}
	result := make([]QueuedMessage, limit)
	copy(result, s.steering[:limit])
	s.steering = s.steering[limit:]
	return result
}

func (s *Stream) DequeueNextMessage() (QueuedMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.steering) > 0 {
		msg := s.steering[0]
		s.steering = s.steering[1:]
		return msg, true
	}
	if len(s.followUps) > 0 {
		msg := s.followUps[0]
		s.followUps = s.followUps[1:]
		return msg, true
	}
	return QueuedMessage{}, false
}

func (s *Stream) QueueSnapshot() []QueuedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queueSnapshotLocked()
}

func (s *Stream) RestoreQueue(queue []QueuedMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steering = nil
	s.followUps = nil
	for _, msg := range queue {
		msg = normalizeQueuedMessage(msg)
		switch msg.Kind {
		case QueueSteering:
			s.steering = append(s.steering, msg)
		default:
			msg.Kind = QueueFollowUp
			s.followUps = append(s.followUps, msg)
		}
	}
}

func (s *Stream) queueSnapshotLocked() []QueuedMessage {
	queue := make([]QueuedMessage, 0, len(s.steering)+len(s.followUps))
	queue = append(queue, s.steering...)
	queue = append(queue, s.followUps...)
	return queue
}

func (s *Stream) UpdateQueuedMessage(id, text string, parts []domainmessage.ContentPart, displayText string) (QueuedMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, false
	}
	msg := (*queue)[index]
	msg.Text = text
	msg.Parts = append([]domainmessage.ContentPart(nil), parts...)
	msg.DisplayText = displayText
	if msg.DisplayText == "" {
		msg.DisplayText = msg.Text
	}
	msg.UpdatedAt = time.Now()
	(*queue)[index] = msg
	return msg, true
}

func (s *Stream) SetQueuedMessageKind(id string, kind QueueKind) (QueuedMessage, bool) {
	if kind == "" {
		kind = QueueFollowUp
	}
	if kind != QueueFollowUp && kind != QueueSteering {
		return QueuedMessage{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, false
	}
	msg := (*queue)[index]
	if msg.Kind == kind {
		return msg, true
	}
	*queue = append((*queue)[:index], (*queue)[index+1:]...)
	msg.Kind = kind
	msg.UpdatedAt = time.Now()
	switch kind {
	case QueueSteering:
		s.steering = append(s.steering, msg)
	default:
		s.followUps = append(s.followUps, msg)
	}
	return msg, true
}

func (s *Stream) RemoveQueuedMessage(id string) (QueuedMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, false
	}
	msg := (*queue)[index]
	*queue = append((*queue)[:index], (*queue)[index+1:]...)
	return msg, true
}

func (s *Stream) MoveQueuedMessage(id string, direction int) (QueuedMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, false
	}
	next := index + direction
	if next < 0 || next >= len(*queue) {
		return (*queue)[index], true
	}
	(*queue)[index], (*queue)[next] = (*queue)[next], (*queue)[index]
	(*queue)[next].UpdatedAt = time.Now()
	return (*queue)[next], true
}

func (s *Stream) findQueuedMessageLocked(id string) (*[]QueuedMessage, int, bool) {
	for i := range s.steering {
		if s.steering[i].ID == id {
			return &s.steering, i, true
		}
	}
	for i := range s.followUps {
		if s.followUps[i].ID == id {
			return &s.followUps, i, true
		}
	}
	return nil, -1, false
}

func (s *Stream) QueuedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.steering) + len(s.followUps)
}

// BeginInterrupt 标记当前流正在等待指定类型的人工输入。
func (s *Stream) BeginInterrupt(kind InterruptKind) {
	s.BeginInterruptWithID(kind, "")
}

// BeginInterruptWithID 标记当前流正在等待指定 ID 的人工输入。
func (s *Stream) BeginInterruptWithID(kind InterruptKind, id string) {
	var ids []string
	if id != "" {
		ids = []string{id}
	}
	s.BeginInterruptWithIDs(kind, ids)
}

// BeginInterruptWithIDs 标记当前流正在等待一组指定 ID 的人工输入。
func (s *Stream) BeginInterruptWithIDs(kind InterruptKind, ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainInterruptLocked()
	s.pendingKind = kind
	s.pendingID = ""
	s.pendingIDs = make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if s.pendingID == "" {
			s.pendingID = id
		}
		s.pendingIDs[id] = true
	}
	if len(s.pendingIDs) == 0 {
		s.pendingIDs = nil
	}
	s.submittedIDs = make(map[string]bool, len(s.pendingIDs))
	s.submitted = false
}

// CompleteInterrupt 清除当前人工输入等待状态。
func (s *Stream) CompleteInterrupt(kind InterruptKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingKind == kind {
		s.pendingKind = InterruptNone
		s.pendingID = ""
		s.pendingIDs = nil
		s.submittedIDs = nil
		s.submitted = false
	}
}

// BeginAsk 开始一个独立 ask 等待项。
func (s *Stream) BeginAsk(id string) (<-chan any, error) {
	if id == "" {
		return nil, fmt.Errorf("ask id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.status != "processing" {
		return nil, fmt.Errorf("task is not processing")
	}
	if s.pendingAsks == nil {
		s.pendingAsks = make(map[string]chan any)
	}
	if _, exists := s.pendingAsks[id]; exists {
		return nil, fmt.Errorf("ask request already pending")
	}
	ch := make(chan any, 1)
	s.pendingAsks[id] = ch
	return ch, nil
}

// CompleteAsk 清除一个独立 ask 等待项。
func (s *Stream) CompleteAsk(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingAsks == nil {
		return
	}
	delete(s.pendingAsks, id)
	if len(s.pendingAsks) == 0 {
		s.pendingAsks = nil
	}
}

// SubmitAskResponse 提交指定 ask ID 的回答。
func (s *Stream) SubmitAskResponse(id string, value any) error {
	s.mu.Lock()
	if s.done || s.status != "processing" {
		s.mu.Unlock()
		return fmt.Errorf("task is not processing")
	}
	if id == "" && len(s.pendingAsks) == 1 {
		for pendingID := range s.pendingAsks {
			id = pendingID
		}
	}
	if ch, ok := s.pendingAsks[id]; ok {
		delete(s.pendingAsks, id)
		if len(s.pendingAsks) == 0 {
			s.pendingAsks = nil
		}
		s.mu.Unlock()
		ch <- value
		return nil
	}
	s.mu.Unlock()
	return s.SubmitInterruptWithID(InterruptAsk, id, value)
}

// SubmitInterrupt 提交人工输入。仅当前确实存在同类型 pending 时才接受。
func (s *Stream) SubmitInterrupt(kind InterruptKind, value any) error {
	return s.SubmitInterruptWithID(kind, "", value)
}

// SubmitInterruptWithID 提交指定 ID 的人工输入。
func (s *Stream) SubmitInterruptWithID(kind InterruptKind, id string, value any) error {
	s.mu.Lock()
	if s.done || s.status != "processing" {
		s.mu.Unlock()
		return fmt.Errorf("task is not processing")
	}
	if s.pendingKind != kind {
		s.mu.Unlock()
		return fmt.Errorf("no pending %s request", kind)
	}
	if len(s.pendingIDs) > 0 {
		if id == "" && len(s.pendingIDs) == 1 {
			for pendingID := range s.pendingIDs {
				id = pendingID
			}
		}
		if !s.pendingIDs[id] {
			s.mu.Unlock()
			return fmt.Errorf("pending %s request id mismatch", kind)
		}
		if s.submittedIDs[id] {
			s.mu.Unlock()
			return fmt.Errorf("%s request already submitted", kind)
		}
		s.submittedIDs[id] = true
	} else {
		if s.pendingID != "" && id != s.pendingID {
			s.mu.Unlock()
			return fmt.Errorf("pending %s request id mismatch", kind)
		}
		if s.submitted {
			s.mu.Unlock()
			return fmt.Errorf("%s request already submitted", kind)
		}
		s.submitted = true
	}
	ch := s.interruptCh
	s.mu.Unlock()

	select {
	case ch <- value:
		return nil
	default:
		s.mu.Lock()
		if s.pendingKind == kind {
			if len(s.pendingIDs) > 0 && id != "" {
				s.submittedIDs[id] = false
			} else {
				s.submitted = false
			}
		}
		s.mu.Unlock()
		return fmt.Errorf("%s request is not ready", kind)
	}
}

func (s *Stream) drainInterruptLocked() {
	for {
		select {
		case <-s.interruptCh:
		default:
			return
		}
	}
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
