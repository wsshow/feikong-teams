package testmodel

import (
	"context"
	"errors"
	"testing"

	"fkteams/agentcore"
)

func TestGenerateDequeuesResponsesAndRecordsCalls(t *testing.T) {
	m := New(AssistantMessage("first"), AssistantMessage("second"))

	resp, err := m.Generate(context.Background(), []agentcore.Message{UserMessage("hello")})
	if err != nil {
		t.Fatalf("generate first: %v", err)
	}
	if resp.Content != "first" {
		t.Fatalf("unexpected first response: %q", resp.Content)
	}

	resp, err = m.Generate(context.Background(), []agentcore.Message{UserMessage("again")})
	if err != nil {
		t.Fatalf("generate second: %v", err)
	}
	if resp.Content != "second" {
		t.Fatalf("unexpected second response: %q", resp.Content)
	}

	calls := m.GenerateCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Input[0].Content != "hello" {
		t.Fatalf("unexpected first input: %q", calls[0].Input[0].Content)
	}
}

func TestStreamDequeuesChunks(t *testing.T) {
	m := New()
	m.EnqueueStream(
		AssistantMessage("a"),
		AssistantMessage("b"),
	)

	sr, err := m.Stream(context.Background(), []agentcore.Message{UserMessage("hello")})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer sr.Close()

	first, err := sr.Recv()
	if err != nil {
		t.Fatalf("recv first: %v", err)
	}
	if first.Content != "a" {
		t.Fatalf("unexpected first chunk: %q", first.Content)
	}
}

func TestWithToolsReturnsToolBoundModel(t *testing.T) {
	m := New(AssistantMessage("ok"))
	bound, err := m.WithTools([]agentcore.ToolInfo{{Name: "test_tool"}})
	if err != nil {
		t.Fatalf("with tools: %v", err)
	}

	if _, err := bound.Generate(context.Background(), []agentcore.Message{UserMessage("hello")}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	calls := m.GenerateCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 generate call, got %d", len(calls))
	}
	if len(calls[0].Tools) != 1 || calls[0].Tools[0].Name != "test_tool" {
		t.Fatalf("expected bound tool to be recorded, got %#v", calls[0].Tools)
	}
}

func TestGenerateReturnsQueuedError(t *testing.T) {
	want := errors.New("model failed")
	m := New().EnqueueGenerate(agentcore.Message{}, want)

	if _, err := m.Generate(context.Background(), nil); !errors.Is(err, want) {
		t.Fatalf("expected queued error, got %v", err)
	}
}
