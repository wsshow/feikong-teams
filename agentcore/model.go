package agentcore

import (
	"context"
	"io"
)

type ChatModel interface {
	Generate(ctx context.Context, input []Message) (Message, error)
	Stream(ctx context.Context, input []Message) (MessageStream, error)
	WithTools(tools []ToolInfo) (ChatModel, error)
}

type ModelCall struct {
	Input []Message
	Tools []ToolInfo
}

type MessageStream interface {
	Recv() (Message, error)
	Close()
}

type sliceMessageStream struct {
	messages []Message
	index    int
}

func NewMessageStream(messages []Message) MessageStream {
	copied := make([]Message, len(messages))
	copy(copied, messages)
	return &sliceMessageStream{messages: copied}
}

func (s *sliceMessageStream) Recv() (Message, error) {
	if s.index >= len(s.messages) {
		return Message{}, io.EOF
	}
	msg := s.messages[s.index]
	s.index++
	return msg, nil
}

func (s *sliceMessageStream) Close() {}
