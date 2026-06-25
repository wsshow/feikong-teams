package trimresult

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestIsNoisyMatchesConfiguredPrefixes(t *testing.T) {
	if !isNoisy("fetch_url", []string{"fetch_", "doc_"}) {
		t.Fatal("fetch_url should match fetch_ prefix")
	}
	if isNoisy("file_read", []string{"fetch_", "doc_"}) {
		t.Fatal("file_read should not match noisy prefixes")
	}
}

func TestOmittedMsgCountsRunes(t *testing.T) {
	got := omittedMsg("[omitted]", "你好ab")
	want := "[omitted] (~4 chars)"
	if got != want {
		t.Fatalf("omittedMsg() = %q, want %q", got, want)
	}
}

func TestTrimNoisyResultsOnlyTrimsDigestedToolResults(t *testing.T) {
	messages := []*schema.Message{
		schema.ToolMessage("large fetch result", "call-1", schema.WithToolName("fetch_url")),
		schema.ToolMessage("read result", "call-2", schema.WithToolName("file_read")),
		schema.AssistantMessage("I have read the result.", nil),
		schema.ToolMessage("active fetch result", "call-3", schema.WithToolName("fetch_url")),
	}

	got := trimNoisyResults(messages, []string{"fetch_"}, "[omitted]")

	if got[0] == messages[0] {
		t.Fatal("trimmed message should be copied before mutation")
	}
	if messages[0].Content != "large fetch result" {
		t.Fatalf("original message content = %q, want unchanged", messages[0].Content)
	}
	if !strings.HasPrefix(got[0].Content, "[omitted] (~") {
		t.Fatalf("trimmed content = %q, want omitted placeholder", got[0].Content)
	}
	if got[1] != messages[1] || got[1].Content != "read result" {
		t.Fatalf("non-noisy message changed: %#v", got[1])
	}
	if got[3] != messages[3] || got[3].Content != "active fetch result" {
		t.Fatalf("active tool result changed: %#v", got[3])
	}
}

func TestTrimNoisyResultsKeepsActiveChainWithoutAssistantText(t *testing.T) {
	messages := []*schema.Message{
		schema.ToolMessage("large fetch result", "call-1", schema.WithToolName("fetch_url")),
		schema.AssistantMessage("", nil),
	}

	got := trimNoisyResults(messages, []string{"fetch_"}, "[omitted]")
	if got[0] != messages[0] || got[0].Content != "large fetch result" {
		t.Fatalf("active chain changed: %#v", got[0])
	}
}

func TestBeforeModelRewriteStateTrimsMessages(t *testing.T) {
	h := &handler{
		prefixes:    []string{"doc_"},
		placeholder: "[hidden]",
	}
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.ToolMessage("document body", "call-1", schema.WithToolName("doc_read")),
			schema.AssistantMessage("summary", nil),
		},
	}

	_, next, err := h.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState() error = %v", err)
	}
	if next != state {
		t.Fatal("state pointer changed")
	}
	if !strings.HasPrefix(next.Messages[0].Content, "[hidden] (~") {
		t.Fatalf("trimmed content = %q, want hidden placeholder", next.Messages[0].Content)
	}
}

func TestBeforeModelRewriteStateHandlesNilOrShortState(t *testing.T) {
	h := &handler{prefixes: []string{"fetch_"}, placeholder: "[hidden]"}
	ctx := context.Background()

	gotCtx, gotState, err := h.BeforeModelRewriteState(ctx, nil, nil)
	if err != nil {
		t.Fatalf("nil state error = %v", err)
	}
	if gotCtx != ctx || gotState != nil {
		t.Fatalf("nil state result = (%v, %#v), want original context and nil state", gotCtx, gotState)
	}

	state := &adk.ChatModelAgentState{Messages: []*schema.Message{schema.ToolMessage("body", "call-1", schema.WithToolName("fetch_url"))}}
	_, gotState, err = h.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("short state error = %v", err)
	}
	if gotState.Messages[0].Content != "body" {
		t.Fatalf("short state content = %q, want unchanged", gotState.Messages[0].Content)
	}
}
