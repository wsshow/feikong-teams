package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"

	claudeModel "github.com/cloudwego/eino-ext/components/model/claude"
	geminiModel "github.com/cloudwego/eino-ext/components/model/gemini"
	"google.golang.org/genai"
)

// TestClaudeReportsCacheUsageThroughRunner 同时覆盖供应商解析与应用用量事件。
// Claude v0.1.26 在合并流的前导空分片时会并发写入返回值 err；-race 可由本测试复现。
func TestClaudeReportsCacheUsageThroughRunner(t *testing.T) {
	// 本地响应测试不读取开发者机器上的认证信息。
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"message\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"mock\",\"content\":[],\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":3,\"cache_creation_input_tokens\":4}}}\n\n")
					fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
					fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"done\"}}\n\n")
					fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
					fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n")
					fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"message","type":"message","role":"assistant","model":"mock","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"cache_read_input_tokens":3,"cache_creation_input_tokens":4,"output_tokens":2}}`)
			}))
			defer server.Close()
			model, err := claudeModel.NewChatModel(context.Background(), &claudeModel.Config{
				Model: "mock", MaxTokens: 128, BaseURL: &server.URL, HTTPClient: server.Client(), APIKey: "test-only",
			})
			if err != nil {
				t.Fatal(err)
			}
			agent, err := NewChatModelAgent(context.Background(), &runtimeport.ChatAgentConfig{
				Name: "assistant", Model: WrapChatModel(model), MaxIterations: 2,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := runAgentForTest(t, context.Background(), agent, streaming)
			var found bool
			for _, event := range got {
				if event.Type == domainevent.TypeAssistantCompleted && event.Usage != nil {
					found = true
					usage := event.Usage
					if usage.CachedTokens != 3 || usage.CacheWriteTokens != 4 || usage.PromptTokens != 17 || usage.CompletionTokens != 2 {
						t.Fatalf("cache usage = %#v", usage)
					}
				}
			}
			if !found {
				t.Fatal("model usage was not reported")
			}
		})
	}
}

func TestGeminiSendsProviderFileReferences(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	var request struct {
		Contents []struct {
			Parts []struct {
				FileData struct {
					FileURI  string `json:"fileUri"`
					MIMEType string `json:"mimeType"`
				} `json:"fileData"`
			} `json:"parts"`
		} `json:"contents"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend: genai.BackendVertexAI, Project: "test-project", Location: "test-location", HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	model, err := geminiModel.NewChatModel(context.Background(), &geminiModel.Config{Client: client, Model: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = WrapChatModel(model).Generate(context.Background(), []domainmessage.Message{{
		Role: domainmessage.RoleUser, ContentParts: []domainmessage.ContentPart{{
			Type: domainmessage.ContentPartFileURL, FileID: "gs://test-bucket/report.pdf", MIMEType: "application/pdf",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Contents) != 1 || len(request.Contents[0].Parts) != 1 {
		t.Fatalf("request contents = %#v", request.Contents)
	}
	file := request.Contents[0].Parts[0].FileData
	if file.FileURI != "gs://test-bucket/report.pdf" || file.MIMEType != "application/pdf" {
		t.Fatalf("provider file reference = %#v", file)
	}
}
