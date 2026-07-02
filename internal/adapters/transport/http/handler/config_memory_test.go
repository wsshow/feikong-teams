package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"fkteams/internal/app/appstate"
	"fkteams/internal/app/config"
	"fkteams/internal/app/memory"

	"github.com/gin-gonic/gin"
)

func TestGetConfigHandlerMasksSensitiveFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saveHandlerConfig(t, config.Config{
		Models: []config.ModelConfig{{
			ID:     "main",
			Name:   "主力模型",
			UseFor: []string{config.ModelUseChat},
			APIKey: "sk-secret",
		}},
		Server: config.Server{Auth: config.ServerAuth{
			Password: "password",
			Secret:   "auth-secret",
		}},
		Agents: config.Agents{
			Items: []config.AgentConfig{{
				ID:      "remote-prod",
				Name:    "生产服务器",
				Enabled: true,
				Tools:   []string{"ssh"},
				SSH:     &config.AgentSSH{Host: "prod:22", Username: "root", Password: "prod-password"},
			}},
		},
		Channels: config.Channels{
			QQ:      config.ChannelQQ{AppSecret: "qq-secret"},
			Discord: config.ChannelDiscord{Token: "discord-secret"},
		},
	})

	router := gin.New()
	router.GET("/config", GetConfigHandler())

	resp := performRequest(router, http.MethodGet, "/config", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("get config status = %d: %s", resp.Code, resp.Body.String())
	}
	var got config.Config
	decodeRawData(t, resp, &got)
	if len(got.Models) != 1 || got.Models[0].APIKey != "" || !got.Models[0].HasAPIKey {
		t.Fatalf("model api key should be hidden with presence flag, got %#v", got.Models)
	}
	if got.Server.Auth.Password != sensitivePassword || got.Server.Auth.Secret != sensitivePassword {
		t.Fatalf("auth sensitive fields were not masked: %#v", got.Server.Auth)
	}
	gotAgent := findAgentConfigForTest(got.Agents.Items, "remote-prod")
	if gotAgent == nil || gotAgent.SSH == nil || gotAgent.SSH.Password != sensitivePassword {
		t.Fatalf("agent ssh password was not masked: %#v", got.Agents.Items)
	}
	if got.Channels.QQ.AppSecret != sensitivePassword {
		t.Fatalf("qq secret was not masked: %#v", got.Channels.QQ)
	}
	if got.Channels.Discord.Token == "discord-secret" || !isMasked(got.Channels.Discord.Token) {
		t.Fatalf("discord token was not masked: %#v", got.Channels.Discord)
	}
}

func TestUpdateConfigHandlerRestoresSensitiveFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saveHandlerConfig(t, config.Config{
		Models: []config.ModelConfig{{
			ID:     "main",
			Name:   "主力模型",
			UseFor: []string{config.ModelUseChat},
			APIKey: "old-key",
		}},
		Server: config.Server{Auth: config.ServerAuth{
			Enabled:  true,
			Username: "admin",
			Password: "old-password",
			Secret:   "old-secret",
		}},
		Agents: config.Agents{
			Items: []config.AgentConfig{{
				ID:      "remote-prod",
				Name:    "生产服务器",
				Enabled: true,
				Tools:   []string{"ssh"},
				SSH:     &config.AgentSSH{Host: "prod:22", Username: "root", Password: "old-prod-ssh"},
			}},
		},
		Channels: config.Channels{
			QQ:      config.ChannelQQ{AppSecret: "old-qq"},
			Discord: config.ChannelDiscord{Token: "old-discord"},
		},
	})

	next := config.Config{
		Models: []config.ModelConfig{{
			ID:         "renamed",
			Name:       "重命名模型",
			UseFor:     []string{config.ModelUseChat},
			OriginalID: "main",
		}},
		Server: config.Server{Auth: config.ServerAuth{
			Enabled:  true,
			Username: "root",
			Password: sensitivePassword,
			Secret:   sensitivePassword,
		}},
		Agents: config.Agents{
			Items: []config.AgentConfig{{
				ID:      "remote-prod",
				Name:    "生产服务器",
				Enabled: true,
				Tools:   []string{"ssh"},
				SSH:     &config.AgentSSH{Host: "prod:22", Username: "root", Password: sensitivePassword},
			}},
		},
		Channels: config.Channels{
			QQ:      config.ChannelQQ{AppSecret: sensitivePassword},
			Discord: config.ChannelDiscord{Token: maskAPIKey("old-discord")},
		},
	}
	body, err := json.Marshal(next)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	router := gin.New()
	router.POST("/config", UpdateConfigHandlerWithState(nil))
	resp := performJSON(router, http.MethodPost, "/config", string(body))
	if resp.Code != http.StatusOK {
		t.Fatalf("update config status = %d: %s", resp.Code, resp.Body.String())
	}
	var data map[string]bool
	decodeRawData(t, resp, &data)
	if !data["auth_changed"] {
		t.Fatalf("expected auth_changed, got %#v", data)
	}

	got := config.Get()
	if got.Models[0].APIKey != "old-key" {
		t.Fatalf("model api key was not restored: %#v", got.Models[0])
	}
	if got.Server.Auth.Password != "old-password" || got.Server.Auth.Secret != "old-secret" {
		t.Fatalf("auth sensitive fields were not restored: %#v", got.Server.Auth)
	}
	gotAgent := findAgentConfigForTest(got.Agents.Items, "remote-prod")
	if gotAgent == nil || gotAgent.SSH == nil || gotAgent.SSH.Password != "old-prod-ssh" {
		t.Fatalf("agent ssh password was not restored: %#v", got.Agents.Items)
	}
	if got.Channels.QQ.AppSecret != "old-qq" || got.Channels.Discord.Token != "old-discord" {
		t.Fatalf("channel secrets were not restored: %#v", got.Channels)
	}
}

func findAgentConfigForTest(items []config.AgentConfig, id string) *config.AgentConfig {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func TestUpdateConfigHandlerRejectsDuplicateModelNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saveHandlerConfig(t, config.Config{})

	body := `{"models":[{"id":"main","name":"主力模型","use_for":["chat"]},{"id":"main","name":"重复模型"}]}`
	router := gin.New()
	router.POST("/config", UpdateConfigHandlerWithState(nil))

	resp := performJSON(router, http.MethodPost, "/config", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("duplicate model status = %d, want 400", resp.Code)
	}
}

func TestMemoryHandlersUseInjectedState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	manager := &handlerFakeMemory{
		entries: []memory.MemoryEntry{{ID: "1", Summary: "偏好摘要"}},
		deleted: map[string]int{
			"偏好摘要": 1,
		},
		count: 2,
	}
	state := appstate.New()
	state.SetMemory(manager)

	router := gin.New()
	router.GET("/memory", GetMemoryListHandlerWithState(state))
	router.DELETE("/memory", DeleteMemoryHandlerWithState(state))
	router.DELETE("/memory/clear", ClearMemoryHandlerWithState(state))

	resp := performRequest(router, http.MethodGet, "/memory", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("memory list status = %d: %s", resp.Code, resp.Body.String())
	}
	var entries []memory.MemoryEntry
	decodeRawData(t, resp, &entries)
	if len(entries) != 1 || entries[0].Summary != "偏好摘要" {
		t.Fatalf("unexpected memory list: %#v", entries)
	}

	resp = performJSON(router, http.MethodDelete, "/memory", `{"summary":"偏好摘要"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("memory delete status = %d: %s", resp.Code, resp.Body.String())
	}
	if manager.deleteCalls != 1 {
		t.Fatalf("expected delete to be called once, got %d", manager.deleteCalls)
	}

	resp = performJSON(router, http.MethodDelete, "/memory", `{"summary":"不存在"}`)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("missing memory delete status = %d, want 404", resp.Code)
	}

	resp = performRequest(router, http.MethodDelete, "/memory/clear", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("memory clear status = %d: %s", resp.Code, resp.Body.String())
	}
	var cleared map[string]float64
	decodeRawData(t, resp, &cleared)
	if cleared["cleared"] != 2 || !manager.cleared {
		t.Fatalf("unexpected clear result cleared=%#v manager=%#v", cleared, manager)
	}
}

func TestMemoryHandlersWithoutState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/memory", GetMemoryListHandlerWithState(nil))
	router.DELETE("/memory", DeleteMemoryHandlerWithState(nil))
	router.DELETE("/memory/clear", ClearMemoryHandlerWithState(nil))

	resp := performRequest(router, http.MethodGet, "/memory", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("nil memory list status = %d: %s", resp.Code, resp.Body.String())
	}
	var entries []memory.MemoryEntry
	decodeRawData(t, resp, &entries)
	if len(entries) != 0 {
		t.Fatalf("expected empty memory list, got %#v", entries)
	}

	resp = performJSON(router, http.MethodDelete, "/memory", `{"summary":"x"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("nil memory delete status = %d, want 400", resp.Code)
	}

	resp = performRequest(router, http.MethodDelete, "/memory/clear", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("nil memory clear status = %d, want 400", resp.Code)
	}
}

type handlerFakeMemory struct {
	entries     []memory.MemoryEntry
	deleted     map[string]int
	count       int
	deleteCalls int
	cleared     bool
}

func (m *handlerFakeMemory) Search(string, int) []memory.MemoryEntry { return nil }
func (m *handlerFakeMemory) ExtractAndStore(context.Context, []memory.Message, string) {
}
func (m *handlerFakeMemory) FlushExtract(context.Context, []memory.Message, string) {}
func (m *handlerFakeMemory) List() []memory.MemoryEntry                             { return m.entries }
func (m *handlerFakeMemory) Delete(summary string) int {
	m.deleteCalls++
	return m.deleted[summary]
}
func (m *handlerFakeMemory) Count() int { return m.count }
func (m *handlerFakeMemory) Clear()     { m.cleared = true }
func (m *handlerFakeMemory) ResetLLM(memory.LLMClient) {
}
func (m *handlerFakeMemory) Wait() {
}

var _ appstate.MemoryManager = (*handlerFakeMemory)(nil)
