package testmodel

import (
	"context"
	"fkteams/agentcore"
	"fmt"
	"sync"
)

type GenerateResult struct {
	Message agentcore.Message
	Err     error
}

type StreamResult struct {
	Chunks []agentcore.Message
	Err    error
}

type Call struct {
	Input []agentcore.Message
	Tools []agentcore.ToolInfo
}

type Model struct {
	state *state
	tools []agentcore.ToolInfo
}

type state struct {
	mu             sync.Mutex
	generateQueue  []GenerateResult
	streamQueue    []StreamResult
	generateCalls  []Call
	streamCalls    []Call
	withToolsCalls [][]agentcore.ToolInfo
}

var _ agentcore.ChatModel = (*Model)(nil)

func New(responses ...agentcore.Message) *Model {
	m := &Model{state: &state{}}
	for _, resp := range responses {
		m.EnqueueGenerate(resp, nil)
	}
	return m
}

func AssistantMessage(content string) agentcore.Message {
	return agentcore.Message{Role: agentcore.RoleAssistant, Content: content}
}

func UserMessage(content string) agentcore.Message {
	return agentcore.Message{Role: agentcore.RoleUser, Content: content}
}

func (m *Model) EnqueueGenerate(message agentcore.Message, err error) *Model {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.generateQueue = append(m.state.generateQueue, GenerateResult{Message: message, Err: err})
	return m
}

func (m *Model) EnqueueStream(chunks ...agentcore.Message) *Model {
	m.EnqueueStreamResult(chunks, nil)
	return m
}

func (m *Model) EnqueueStreamResult(chunks []agentcore.Message, err error) *Model {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	copied := copyMessages(chunks)
	m.state.streamQueue = append(m.state.streamQueue, StreamResult{Chunks: copied, Err: err})
	return m
}

func (m *Model) Generate(_ context.Context, input []agentcore.Message) (agentcore.Message, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()

	m.state.generateCalls = append(m.state.generateCalls, Call{
		Input: copyMessages(input),
		Tools: copyTools(m.tools),
	})
	if len(m.state.generateQueue) == 0 {
		return agentcore.Message{}, fmt.Errorf("testmodel: no queued generate response")
	}

	resp := m.state.generateQueue[0]
	m.state.generateQueue = m.state.generateQueue[1:]
	return resp.Message, resp.Err
}

func (m *Model) Stream(_ context.Context, input []agentcore.Message) (agentcore.MessageStream, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()

	m.state.streamCalls = append(m.state.streamCalls, Call{
		Input: copyMessages(input),
		Tools: copyTools(m.tools),
	})
	if len(m.state.streamQueue) == 0 {
		return nil, fmt.Errorf("testmodel: no queued stream response")
	}

	resp := m.state.streamQueue[0]
	m.state.streamQueue = m.state.streamQueue[1:]
	if resp.Err != nil {
		return nil, resp.Err
	}
	return agentcore.NewMessageStream(resp.Chunks), nil
}

func (m *Model) WithTools(tools []agentcore.ToolInfo) (agentcore.ChatModel, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	copied := copyTools(tools)
	m.state.withToolsCalls = append(m.state.withToolsCalls, copied)
	return &Model{state: m.state, tools: copied}, nil
}

func (m *Model) GenerateCalls() []Call {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	return copyCalls(m.state.generateCalls)
}

func (m *Model) StreamCalls() []Call {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	return copyCalls(m.state.streamCalls)
}

func (m *Model) WithToolsCalls() [][]agentcore.ToolInfo {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	calls := make([][]agentcore.ToolInfo, len(m.state.withToolsCalls))
	for i, call := range m.state.withToolsCalls {
		calls[i] = copyTools(call)
	}
	return calls
}

func copyMessages(in []agentcore.Message) []agentcore.Message {
	out := make([]agentcore.Message, len(in))
	copy(out, in)
	return out
}

func copyTools(in []agentcore.ToolInfo) []agentcore.ToolInfo {
	out := make([]agentcore.ToolInfo, len(in))
	copy(out, in)
	return out
}

func copyCalls(in []Call) []Call {
	out := make([]Call, len(in))
	for i, call := range in {
		out[i] = Call{
			Input: copyMessages(call.Input),
			Tools: copyTools(call.Tools),
		}
	}
	return out
}
