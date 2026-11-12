package register

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelproviders "fkteams/internal/adapters/model/providers"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
)

func TestDeepSeekVisionSettingAcrossRegistries(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(string(body), `"image_url"`) || r.Header.Get("X-Vision-Test") != "configured" {
			t.Errorf("image or configured header missing: %s, %v", body, err)
		}
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"reply\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"seen\"},\"finish_reason\":null}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"reply","choices":[{"index":0,"message":{"role":"assistant","content":"seen"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	providers, runtime := modelproviders.NewRegistry(), modelregistry.NewRegistry()
	RegisterDefaults(providers, runtime)
	input := []message.Message{{Role: message.RoleUser, ContentParts: []message.ContentPart{{Type: message.ContentPartImageURL, URL: "https://example.com/image.png"}}}}
	for _, setting := range []*bool{nil, new(false), new(true)} {
		models := []func() (runtimeport.ChatModel, error){
			func() (runtimeport.ChatModel, error) {
				return providers.NewChatModel(context.Background(), &modelproviders.Config{Provider: modelproviders.DeepSeek, APIKey: "test-key", BaseURL: server.URL, Model: "custom-model", SupportsVision: setting, ExtraHeaders: map[string]string{"X-Vision-Test": "configured"}})
			},
			func() (runtimeport.ChatModel, error) {
				return runtime.NewChatModel(context.Background(), &modelregistry.Config{Provider: modelregistry.DeepSeek, APIKey: "test-key", BaseURL: server.URL, Model: "custom-model", SupportsVision: setting, ExtraHeaders: map[string]string{"X-Vision-Test": "configured"}})
			},
		}
		for _, create := range models {
			model, err := create()
			if err != nil {
				t.Fatal(err)
			}
			_, generateErr := model.Generate(context.Background(), input)
			stream, streamErr := model.Stream(context.Background(), input)
			if stream != nil {
				_, err = stream.Recv()
				stream.Close()
				if err != nil {
					t.Fatalf("stream receive: %v", err)
				}
			}
			if setting != nil && *setting {
				if generateErr != nil || streamErr != nil {
					t.Fatalf("enabled vision: Generate=%v Stream=%v", generateErr, streamErr)
				}
			} else if generateErr == nil || streamErr == nil {
				t.Fatalf("expected image rejection: Generate=%v Stream=%v", generateErr, streamErr)
			}
		}
	}
	if requests.Load() != 4 {
		t.Fatalf("upstream requests=%d, want only four explicitly enabled calls", requests.Load())
	}
}
