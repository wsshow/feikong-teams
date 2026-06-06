package eino

import (
	"context"
	"fkteams/agentcore"
	"fmt"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func AdaptChatModelForRunner(m agentcore.ChatModel) (model.ToolCallingChatModel, error) {
	if m == nil {
		return nil, fmt.Errorf("model is nil")
	}
	if runtimeModel, ok := m.(interface {
		runnerModel() model.ToolCallingChatModel
	}); ok {
		if runtimeModel.runnerModel() == nil {
			return nil, fmt.Errorf("model is nil")
		}
		return runtimeModel.runnerModel(), nil
	}
	return &nativeChatModelAdapter{inner: m}, nil
}

func WrapChatModel(inner model.ToolCallingChatModel) agentcore.ChatModel {
	return &runtimeChatModelAdapter{inner: inner}
}

type runtimeChatModelAdapter struct {
	inner model.ToolCallingChatModel
}

func (m *runtimeChatModelAdapter) runnerModel() model.ToolCallingChatModel {
	if m == nil {
		return nil
	}
	return m.inner
}

func (m *runtimeChatModelAdapter) Generate(ctx context.Context, input []agentcore.Message) (agentcore.Message, error) {
	msg, err := m.inner.Generate(ctx, adaptMessagesForRunner(input))
	if err != nil {
		return agentcore.Message{}, err
	}
	return adaptMessageFromRunner(msg), nil
}

func (m *runtimeChatModelAdapter) Stream(ctx context.Context, input []agentcore.Message) (agentcore.MessageStream, error) {
	stream, err := m.inner.Stream(ctx, adaptMessagesForRunner(input))
	if err != nil {
		return nil, err
	}
	return &runtimeMessageStreamAdapter{inner: stream}, nil
}

func (m *runtimeChatModelAdapter) WithTools(tools []agentcore.ToolInfo) (agentcore.ChatModel, error) {
	runnerTools := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		runnerTools = append(runnerTools, &schema.ToolInfo{Name: t.Name, Desc: t.Desc, Extra: t.Extra})
	}
	next, err := m.inner.WithTools(runnerTools)
	if err != nil {
		return nil, err
	}
	return WrapChatModel(next), nil
}

type runtimeMessageStreamAdapter struct {
	inner *schema.StreamReader[*schema.Message]
}

func (s *runtimeMessageStreamAdapter) Recv() (agentcore.Message, error) {
	msg, err := s.inner.Recv()
	if err != nil {
		return agentcore.Message{}, err
	}
	return adaptMessageFromRunner(msg), nil
}

func (s *runtimeMessageStreamAdapter) Close() {
	s.inner.Close()
}

type nativeChatModelAdapter struct{ inner agentcore.ChatModel }

func (m *nativeChatModelAdapter) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := m.inner.Generate(ctx, adaptMessagesFromRunner(input))
	if err != nil {
		return nil, err
	}
	return adaptMessageForRunner(msg), nil
}

func (m *nativeChatModelAdapter) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	stream, err := m.inner.Stream(ctx, adaptMessagesFromRunner(input))
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		defer stream.Close()
		for {
			msg, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					return
				}
				writer.Send(nil, err)
				return
			}
			if writer.Send(adaptMessageForRunner(msg), nil) {
				return
			}
		}
	}()
	return reader, nil
}

func (m *nativeChatModelAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	coreTools := make([]agentcore.ToolInfo, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		coreTools = append(coreTools, agentcore.ToolInfo{Name: t.Name, Desc: t.Desc, Extra: t.Extra})
	}
	next, err := m.inner.WithTools(coreTools)
	if err != nil {
		return nil, err
	}
	runnerModel, err := AdaptChatModelForRunner(next)
	if err != nil {
		return nil, err
	}
	return runnerModel, nil
}

func adaptMessageForRunner(msg agentcore.Message) *schema.Message {
	messages := adaptMessagesForRunner([]agentcore.Message{msg})
	if len(messages) == 0 || messages[0] == nil {
		return &schema.Message{}
	}
	return messages[0]
}

func adaptMessagesFromRunner(messages []*schema.Message) []agentcore.Message {
	result := make([]agentcore.Message, 0, len(messages))
	for _, msg := range messages {
		result = append(result, adaptMessageFromRunner(msg))
	}
	return result
}
