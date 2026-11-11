package taskstream

import (
	domainmessage "fkteams/internal/domain/message"
	"github.com/google/uuid"
	"time"
)

// Queue 是未消费输入的事务草稿，只能在 ChangeQueue 回调内修改。
// 它不持有事件流或运行任务；保存成功后才替换正式状态。
type Queue struct {
	config      StreamConfig
	accepting   bool
	steering    []QueuedMessage
	followUps   []QueuedMessage
	queuedBytes int
	queueClosed bool
}

// NewQueue 创建使用默认容量限制的离线队列。
func NewQueue() *Queue {
	return &Queue{config: StreamConfig{MaxQueuedMessages: defaultMaxQueuedMessages, MaxQueuedMessageBytes: defaultMaxQueuedMessageBytes}, accepting: true}
}

func (q *Queue) clone() *Queue {
	copy := *q
	copy.steering = append([]QueuedMessage(nil), q.steering...)
	copy.followUps = append([]QueuedMessage(nil), q.followUps...)
	return &copy
}

// ChangeQueue 在流锁内修改草稿并保存，失败时原队列保持不变。
func (s *Stream) ChangeQueue(change func(*Queue) error, save func([]QueuedMessage) error) ([]QueuedMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft := s.queue.clone()
	draft.config = s.config
	draft.accepting = !s.done && s.status == "processing" && !s.config.QueueDisabled
	if err := change(draft); err != nil {
		return nil, err
	}
	snapshot := draft.QueueSnapshot()
	if save != nil {
		if err := save(snapshot); err != nil {
			return nil, err
		}
	}
	s.queue = *draft
	return snapshot, nil
}
func (s *Queue) EnqueueMessage(msg QueuedMessage) (QueuedMessage, error) {
	return s.enqueueMessageLocked(msg)
}

// EnqueueMessageIfProcessing 仅在任务仍可消费队列时追加消息。
func (s *Queue) EnqueueMessageIfProcessing(msg QueuedMessage) (QueuedMessage, error) {
	if !s.accepting || s.queueClosed {
		return QueuedMessage{}, ErrQueueNotProcessing
	}
	return s.enqueueMessageLocked(msg)
}

func (s *Queue) enqueueMessageLocked(msg QueuedMessage) (QueuedMessage, error) {
	msg = normalizeQueuedMessage(msg)
	size := queuedMessageSize(msg)
	if s.config.MaxQueuedMessageBytes > 0 && size > s.config.MaxQueuedMessageBytes {
		return QueuedMessage{}, ErrQueueMessageTooLarge
	}
	if (s.config.MaxQueuedMessages > 0 && len(s.steering)+len(s.followUps) >= s.config.MaxQueuedMessages) ||
		(s.config.MaxQueuedMessageBytes > 0 && s.queuedBytes+size > s.config.MaxQueuedMessageBytes) {
		return QueuedMessage{}, ErrQueueFull
	}
	s.appendQueuedMessageLocked(msg, size)
	return msg, nil
}

func (s *Queue) appendQueuedMessageLocked(msg QueuedMessage, size int) {
	switch msg.Kind {
	case QueueSteering:
		s.steering = append(s.steering, msg)
	default:
		msg.Kind = QueueFollowUp
		s.followUps = append(s.followUps, msg)
	}
	s.queuedBytes += size
}

func normalizeQueuedMessage(msg QueuedMessage) QueuedMessage {
	now := time.Now()
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	if msg.Kind == "" {
		msg.Kind = QueueFollowUp
	}
	if msg.Kind != QueueSteering {
		msg.Kind = QueueFollowUp
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}
	if msg.DisplayText == "" {
		msg.DisplayText = msg.Text
	}
	msg.Parts = append([]domainmessage.ContentPart(nil), msg.Parts...)
	return msg
}

func queuedMessageSize(msg QueuedMessage) int {
	size := len(msg.ID) + len(msg.Kind) + len(msg.Text) + len(msg.DisplayText) + 32
	for _, part := range msg.Parts {
		size += len(part.Type) + len(part.Name) + len(part.Text) + len(part.URL) + len(part.Base64Data) + len(part.MIMEType) + len(part.Detail)
	}
	return size
}

func (s *Queue) TakeSteeringMessages(limit int) []QueuedMessage {
	if !s.accepting || len(s.steering) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(s.steering) {
		limit = len(s.steering)
	}
	result := make([]QueuedMessage, limit)
	copy(result, s.steering[:limit])
	s.steering = s.steering[limit:]
	for _, msg := range result {
		s.queuedBytes -= queuedMessageSize(msg)
	}
	return result
}

func (s *Queue) DequeueNextMessage() (QueuedMessage, bool) {
	return s.dequeueNextMessageLocked()
}

// DequeueNextMessageOrSeal 原子地取得下一条消息；队列为空时关闭入队，状态在持久化收尾后更新。
// 与 EnqueueMessageIfProcessing 配合，保证已接受的消息不会落入“检查空队列”和“任务完成”之间的缝隙。
func (s *Queue) DequeueNextMessageOrSeal() (QueuedMessage, bool) {
	if !s.accepting {
		return QueuedMessage{}, false
	}
	if msg, ok := s.dequeueNextMessageLocked(); ok {
		return msg, true
	}
	s.queueClosed = true
	return QueuedMessage{}, false
}

func (s *Queue) dequeueNextMessageLocked() (QueuedMessage, bool) {
	if len(s.steering) > 0 {
		msg := s.steering[0]
		s.steering = s.steering[1:]
		s.queuedBytes -= queuedMessageSize(msg)
		return msg, true
	}
	if len(s.followUps) > 0 {
		msg := s.followUps[0]
		s.followUps = s.followUps[1:]
		s.queuedBytes -= queuedMessageSize(msg)
		return msg, true
	}
	return QueuedMessage{}, false
}

func (s *Queue) QueueSnapshot() []QueuedMessage {
	return s.queueSnapshotLocked()
}

func (s *Queue) RestoreQueue(queue []QueuedMessage) {

	// 恢复期间可能已有并发请求写入新队列项。磁盘快照应排在新消息之前，
	// 但不能覆盖已经向调用方确认接收的内存消息。
	current := s.queueSnapshotLocked()
	currentIDs := make(map[string]struct{}, len(current))
	for _, msg := range current {
		if msg.ID != "" {
			currentIDs[msg.ID] = struct{}{}
		}
	}

	currentBytes := 0
	for _, msg := range current {
		currentBytes += queuedMessageSize(msg)
	}
	s.steering = nil
	s.followUps = nil
	s.queuedBytes = 0
	for _, msg := range queue {
		msg = normalizeQueuedMessage(msg)
		if _, exists := currentIDs[msg.ID]; exists {
			continue
		}
		size := queuedMessageSize(msg)
		if (s.config.MaxQueuedMessages > 0 && len(s.steering)+len(s.followUps)+len(current) >= s.config.MaxQueuedMessages) ||
			(s.config.MaxQueuedMessageBytes > 0 && (size > s.config.MaxQueuedMessageBytes || s.queuedBytes+size+currentBytes > s.config.MaxQueuedMessageBytes)) {
			continue
		}
		s.appendQueuedMessageLocked(msg, size)
	}
	for _, msg := range current {
		msg = normalizeQueuedMessage(msg)
		s.appendQueuedMessageLocked(msg, queuedMessageSize(msg))
	}
}

func (s *Queue) queueSnapshotLocked() []QueuedMessage {
	queue := make([]QueuedMessage, 0, len(s.steering)+len(s.followUps))
	queue = append(queue, s.steering...)
	queue = append(queue, s.followUps...)
	for i := range queue {
		queue[i].Parts = append([]domainmessage.ContentPart(nil), queue[i].Parts...)
	}
	return queue
}

func (s *Queue) UpdateQueuedMessage(id, text string, parts []domainmessage.ContentPart, displayText string) (QueuedMessage, error) {
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, ErrQueuedMessageNotFound
	}
	msg := (*queue)[index]
	msg.Text = text
	msg.Parts = append([]domainmessage.ContentPart(nil), parts...)
	msg.DisplayText = displayText
	if msg.DisplayText == "" {
		msg.DisplayText = msg.Text
	}
	msg.UpdatedAt = time.Now()
	oldSize := queuedMessageSize((*queue)[index])
	newSize := queuedMessageSize(msg)
	if s.config.MaxQueuedMessageBytes > 0 && (newSize > s.config.MaxQueuedMessageBytes || s.queuedBytes-oldSize+newSize > s.config.MaxQueuedMessageBytes) {
		if newSize > s.config.MaxQueuedMessageBytes {
			return QueuedMessage{}, ErrQueueMessageTooLarge
		}
		return QueuedMessage{}, ErrQueueFull
	}
	(*queue)[index] = msg
	s.queuedBytes += newSize - oldSize
	return msg, nil
}

func (s *Queue) SetQueuedMessageKind(id string, kind QueueKind) (QueuedMessage, bool) {
	if kind == "" {
		kind = QueueFollowUp
	}
	if kind != QueueFollowUp && kind != QueueSteering {
		return QueuedMessage{}, false
	}
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, false
	}
	msg := (*queue)[index]
	if msg.Kind == kind {
		return msg, true
	}
	oldSize := queuedMessageSize(msg)
	*queue = append((*queue)[:index], (*queue)[index+1:]...)
	msg.Kind = kind
	msg.UpdatedAt = time.Now()
	switch kind {
	case QueueSteering:
		s.steering = append(s.steering, msg)
	default:
		s.followUps = append(s.followUps, msg)
	}
	s.queuedBytes += queuedMessageSize(msg) - oldSize
	return msg, true
}

func (s *Queue) RemoveQueuedMessage(id string) (QueuedMessage, bool) {
	queue, index, ok := s.findQueuedMessageLocked(id)
	if !ok {
		return QueuedMessage{}, false
	}
	msg := (*queue)[index]
	*queue = append((*queue)[:index], (*queue)[index+1:]...)
	s.queuedBytes -= queuedMessageSize(msg)
	return msg, true
}

func (s *Queue) MoveQueuedMessage(id string, direction int) (QueuedMessage, bool) {
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

func (s *Queue) findQueuedMessageLocked(id string) (*[]QueuedMessage, int, bool) {
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

func (s *Queue) QueuedCount() int {
	return len(s.steering) + len(s.followUps)
}

// QueueSnapshot 返回已提交的队列快照。
func (s *Stream) QueueSnapshot() []QueuedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.QueueSnapshot()
}

// QueuedCount 返回已提交的未消费条数。
func (s *Stream) QueuedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.QueuedCount()
}

func (s *Stream) EnqueueMessage(msg QueuedMessage) (QueuedMessage, error) {
	var result QueuedMessage
	var outcome error
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.EnqueueMessage(msg); return nil }, nil)
	return result, outcome
}

func (s *Stream) EnqueueMessageIfProcessing(msg QueuedMessage) (QueuedMessage, error) {
	var result QueuedMessage
	var outcome error
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.EnqueueMessageIfProcessing(msg); return nil }, nil)
	return result, outcome
}

func (s *Stream) TakeSteeringMessages(limit int) []QueuedMessage {
	var result []QueuedMessage
	_, _ = s.ChangeQueue(func(q *Queue) error { result = q.TakeSteeringMessages(limit); return nil }, nil)
	return result
}

func (s *Stream) DequeueNextMessage() (QueuedMessage, bool) {
	var result QueuedMessage
	var outcome bool
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.DequeueNextMessage(); return nil }, nil)
	return result, outcome
}

func (s *Stream) DequeueNextMessageOrSeal() (QueuedMessage, bool) {
	var result QueuedMessage
	var outcome bool
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.DequeueNextMessageOrSeal(); return nil }, nil)
	return result, outcome
}

func (s *Stream) UpdateQueuedMessage(id, text string, parts []domainmessage.ContentPart, displayText string) (QueuedMessage, error) {
	var result QueuedMessage
	var outcome error
	_, _ = s.ChangeQueue(func(q *Queue) error {
		result, outcome = q.UpdateQueuedMessage(id, text, parts, displayText)
		return nil
	}, nil)
	return result, outcome
}

func (s *Stream) SetQueuedMessageKind(id string, kind QueueKind) (QueuedMessage, bool) {
	var result QueuedMessage
	var outcome bool
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.SetQueuedMessageKind(id, kind); return nil }, nil)
	return result, outcome
}

func (s *Stream) RemoveQueuedMessage(id string) (QueuedMessage, bool) {
	var result QueuedMessage
	var outcome bool
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.RemoveQueuedMessage(id); return nil }, nil)
	return result, outcome
}

func (s *Stream) MoveQueuedMessage(id string, direction int) (QueuedMessage, bool) {
	var result QueuedMessage
	var outcome bool
	_, _ = s.ChangeQueue(func(q *Queue) error { result, outcome = q.MoveQueuedMessage(id, direction); return nil }, nil)
	return result, outcome
}

func (s *Stream) RestoreQueue(queue []QueuedMessage) {
	_, _ = s.ChangeQueue(func(q *Queue) error { q.RestoreQueue(queue); return nil }, nil)
}
