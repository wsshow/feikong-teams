package model

import (
	"context"
	"strings"
	"testing"

	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/testmodel"
)

func TestVisionSettingGuardsGenerateStreamAndToolBinding(t *testing.T) {
	input := []message.Message{{Role: message.RoleUser, ContentParts: []message.ContentPart{{Type: message.ContentPartImageURL, Base64Data: "image"}}}}
	for _, setting := range []*bool{nil, new(true), new(false)} {
		registry := NewRegistry()
		inner := testmodel.New()
		for range 4 {
			inner.EnqueueGenerate(testmodel.AssistantMessage("ok"), nil)
			inner.EnqueueStream(testmodel.AssistantMessage("ok"))
		}
		registry.Register(OpenAI, func(context.Context, *Config) (runtimeport.ChatModel, error) { return inner, nil })
		model, err := registry.NewChatModel(context.Background(), &Config{Provider: OpenAI, SupportsVision: setting})
		if err != nil {
			t.Fatal(err)
		}
		bound, err := model.WithTools([]runtimeport.ToolInfo{{Name: "read"}})
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []runtimeport.ChatModel{model, bound} {
			_, generateErr := current.Generate(context.Background(), input)
			stream, streamErr := current.Stream(context.Background(), input)
			if stream != nil {
				stream.Close()
			}
			blocked := setting != nil && !*setting
			for _, callErr := range []error{generateErr, streamErr} {
				if blocked && (callErr == nil || !strings.Contains(callErr.Error(), "supports_vision=false")) || !blocked && callErr != nil {
					t.Fatalf("setting=%v call error=%v", setting, callErr)
				}
			}
			if _, err := current.Generate(context.Background(), []message.Message{{Role: message.RoleUser, Content: "text"}}); err != nil {
				t.Fatalf("text request: %v", err)
			}
		}
		if setting != nil && !*setting && (len(inner.GenerateCalls()) != 2 || len(inner.StreamCalls()) != 0) {
			t.Fatal("image request reached text-only model")
		}
	}
}
