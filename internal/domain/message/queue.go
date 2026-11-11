package message

import "time"

// QueueKind 表示转向输入或后续任务。
type QueueKind string

const (
	QueueFollowUp QueueKind = "follow_up"
	QueueSteering QueueKind = "steering"
)

// QueuedMessage 是尚未消费的用户输入。
type QueuedMessage struct {
	ID          string        `json:"queue_id"`
	Kind        QueueKind     `json:"kind"`
	Text        string        `json:"text,omitempty"`
	Parts       []ContentPart `json:"parts,omitempty"`
	DisplayText string        `json:"display_text,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at,omitempty"`
}

func (m QueuedMessage) Message() Message {
	msg := Message{
		Role:    RoleUser,
		Content: m.Text,
	}
	if len(m.Parts) > 0 {
		msg.Content = ""
		msg.ContentParts = append([]ContentPart(nil), m.Parts...)
	}
	return msg
}
