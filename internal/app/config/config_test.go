package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetConfigForTest(t *testing.T) string {
	t.Helper()

	appDir := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", appDir)
	globalConfig.Store((*Config)(nil))
	configOnce = sync.Once{}
	configInitErr = nil
	return appDir
}

func TestModelConfigParseExtraHeaders(t *testing.T) {
	if got := (&ModelConfig{}).ParseExtraHeaders(); got != nil {
		t.Fatalf("empty headers = %#v, want nil", got)
	}

	headers := (&ModelConfig{ExtraHeaders: "X-Foo: bar, Authorization: Bearer token, invalid, X-Empty:"}).ParseExtraHeaders()
	if headers["X-Foo"] != "bar" {
		t.Fatalf("X-Foo header = %q", headers["X-Foo"])
	}
	if headers["Authorization"] != "Bearer token" {
		t.Fatalf("Authorization header = %q", headers["Authorization"])
	}
	if _, ok := headers["invalid"]; ok {
		t.Fatalf("invalid header should be ignored: %#v", headers)
	}
	if headers["X-Empty"] != "" {
		t.Fatalf("X-Empty header = %q, want empty value", headers["X-Empty"])
	}
}

func TestServerAuthValidate(t *testing.T) {
	tests := []struct {
		name    string
		auth    ServerAuth
		wantErr bool
	}{
		{name: "disabled", auth: ServerAuth{}},
		{name: "valid", auth: ServerAuth{Enabled: true, Username: "admin", Password: "password", Secret: "secret"}},
		{name: "missing username", auth: ServerAuth{Enabled: true, Password: "password", Secret: "secret"}, wantErr: true},
		{name: "blank username", auth: ServerAuth{Enabled: true, Username: "  ", Password: "password", Secret: "secret"}, wantErr: true},
		{name: "missing password", auth: ServerAuth{Enabled: true, Username: "admin", Secret: "secret"}, wantErr: true},
		{name: "missing secret", auth: ServerAuth{Enabled: true, Username: "admin", Password: "password"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerValidateTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
		wantErr bool
	}{
		{name: "empty"},
		{name: "addresses", proxies: []string{"127.0.0.1", "::1"}},
		{name: "networks", proxies: []string{"10.0.0.0/8", "2001:db8::/32"}},
		{name: "blank", proxies: []string{""}, wantErr: true},
		{name: "hostname", proxies: []string{"proxy.example.com"}, wantErr: true},
		{name: "invalid cidr", proxies: []string{"10.0.0.0/99"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (Server{TrustedProxies: tt.proxies}).Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChannelsList(t *testing.T) {
	channels := Channels{
		QQ: ChannelQQ{
			Enabled:   true,
			AppID:     "app",
			AppSecret: "secret",
			Sandbox:   true,
			Mode:      "team",
		},
		Discord: ChannelDiscord{
			Enabled:   true,
			Token:     "token",
			AllowFrom: "user1,user2",
			Mode:      "deep",
		},
		Weixin: ChannelWeixin{
			Enabled:   true,
			BaseURL:   "https://weixin.example.com",
			CredPath:  "cred.json",
			LogLevel:  "debug",
			AllowFrom: "wx",
			Mode:      "assistant",
		},
	}

	entries := channels.List()
	if len(entries) != 3 {
		t.Fatalf("entries count = %d, want 3: %#v", len(entries), entries)
	}
	if entries[0].Name != "qq" || entries[0].Mode != "team" || entries[0].Extra["sandbox"] != "true" {
		t.Fatalf("qq entry = %#v", entries[0])
	}
	if entries[1].Name != "discord" || entries[1].Extra["allow_from"] != "user1,user2" {
		t.Fatalf("discord entry = %#v", entries[1])
	}
	if entries[2].Name != "weixin" || entries[2].Extra["log_level"] != "debug" {
		t.Fatalf("weixin entry = %#v", entries[2])
	}
}

func TestConfigResolveModelAndWorkspaceDir(t *testing.T) {
	appDir := resetConfigForTest(t)
	cfg := &Config{Models: []ModelConfig{
		{ID: "main", Name: "主力模型", UseFor: []string{ModelUseChat}, Provider: "openai", Model: "gpt-5"},
		{ID: "deepseek", Name: "DeepSeek", Provider: "deepseek", Model: "deepseek-chat"},
	}}

	if got := cfg.ResolveModel(""); got == nil || got.ID != "main" {
		t.Fatalf("ResolveModel empty = %#v", got)
	}
	if got := cfg.ResolveModel("deepseek"); got == nil || got.Model != "deepseek-chat" {
		t.Fatalf("ResolveModel deepseek = %#v", got)
	}
	if got := cfg.ResolveModel("missing"); got != nil {
		t.Fatalf("ResolveModel missing = %#v, want nil", got)
	}
	if got, want := cfg.WorkspaceDir(), filepath.Join(appDir, "workspace"); got != want {
		t.Fatalf("WorkspaceDir = %q, want %q", got, want)
	}
}

func TestDefaultConfigAndGet(t *testing.T) {
	resetConfigForTest(t)

	cfg := Get()
	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 23456 || cfg.Server.LogLevel != "info" {
		t.Fatalf("default config = %#v", cfg.Server)
	}

	custom := &Config{Server: Server{Host: "0.0.0.0", Port: 8080}}
	globalConfig.Store(custom)
	if got := Get(); got != custom {
		t.Fatalf("Get returned %#v, want stored config", got)
	}
}

func TestSaveReloadAndInit(t *testing.T) {
	appDir := resetConfigForTest(t)
	cfg := &Config{
		Models: []ModelConfig{{ID: "main", Name: "主力模型", UseFor: []string{ModelUseChat}, Provider: "openai", APIKey: "sk-test", Model: "gpt-5"}},
		Server: Server{Host: "127.0.0.1", Port: 1234, LogLevel: "debug"},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	configPath := filepath.Join(appDir, "config", "config.toml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file was not written: %v", err)
	}
	if Get() == cfg {
		t.Fatal("Save should publish an isolated config snapshot")
	}
	cfg.Models[0].APIKey = "mutated"
	if got := Get().Models[0].APIKey; got != "sk-test" {
		t.Fatalf("saved config was mutated through caller pointer: %q", got)
	}

	globalConfig.Store((*Config)(nil))
	if err := Reload(); err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}
	if got := Get().ResolveDefaultModel(ModelUseChat); got == nil || got.APIKey != "sk-test" {
		t.Fatalf("reloaded chat model = %#v", got)
	}

	globalConfig.Store((*Config)(nil))
	configOnce = sync.Once{}
	configInitErr = nil
	if err := Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if got := Get().Server.Port; got != 1234 {
		t.Fatalf("Init loaded port = %d, want 1234", got)
	}
}

func TestSnapshotDeepCopiesMutableConfiguration(t *testing.T) {
	resetConfigForTest(t)
	original := &Config{
		Models:    []ModelConfig{{ID: "model", UseFor: []string{ModelUseChat}}},
		Server:    Server{AllowOrigins: []string{"https://example.com"}, TrustedProxies: []string{"127.0.0.1"}},
		OpenAIAPI: OpenAIAPI{APIKeys: []string{"secret"}},
		Agents: Agents{Items: []AgentConfig{{
			ID: "agent", Tools: []string{"search"}, SSH: &AgentSSH{Password: "password"},
		}}},
		Deep: Deep{ExtraTools: []string{"fetch"}},
		Tools: ToolSettings{MCPServers: []MCPServer{{
			ID: "mcp", Args: []string{"serve"}, Env: map[string]string{"TOKEN": "secret"},
		}}, Approval: ToolApprovalSettings{AutoApprove: []string{"search"}}},
	}
	globalConfig.Store(original)

	snapshot := Snapshot()
	snapshot.Models[0].UseFor[0] = "changed"
	snapshot.Server.AllowOrigins[0] = "changed"
	snapshot.OpenAIAPI.APIKeys[0] = "changed"
	snapshot.Agents.Items[0].Tools[0] = "changed"
	snapshot.Agents.Items[0].SSH.Password = "changed"
	snapshot.Deep.ExtraTools[0] = "changed"
	snapshot.Tools.MCPServers[0].Args[0] = "changed"
	snapshot.Tools.MCPServers[0].Env["TOKEN"] = "changed"
	snapshot.Tools.Approval.AutoApprove[0] = "changed"

	if original.Models[0].UseFor[0] != ModelUseChat ||
		original.Server.AllowOrigins[0] != "https://example.com" ||
		original.OpenAIAPI.APIKeys[0] != "secret" ||
		original.Agents.Items[0].Tools[0] != "search" ||
		original.Agents.Items[0].SSH.Password != "password" ||
		original.Deep.ExtraTools[0] != "fetch" ||
		original.Tools.MCPServers[0].Args[0] != "serve" ||
		original.Tools.MCPServers[0].Env["TOKEN"] != "secret" ||
		original.Tools.Approval.AutoApprove[0] != "search" {
		t.Fatal("snapshot mutation leaked into stored configuration")
	}
}

func TestInitAndValidate(t *testing.T) {
	resetConfigForTest(t)
	if err := InitAndValidate(); err == nil {
		t.Fatalf("InitAndValidate missing default error = %v", err)
	}

	resetConfigForTest(t)
	if err := Save(&Config{Models: []ModelConfig{{ID: "main", Name: "主力模型", UseFor: []string{ModelUseChat}, Provider: "openai"}}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	configOnce = sync.Once{}
	configInitErr = nil
	if err := InitAndValidate(); err != nil {
		t.Fatalf("InitAndValidate with provider returned error: %v", err)
	}
}

func TestLoadAndUnmarshal(t *testing.T) {
	appDir := resetConfigForTest(t)
	cfg, err := load()
	if err != nil {
		t.Fatalf("load missing config returned error: %v", err)
	}
	if cfg.Server.Port != 23456 {
		t.Fatalf("load missing config port = %d, want default", cfg.Server.Port)
	}

	configPath := filepath.Join(appDir, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("invalid = ["), 0644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if _, err := load(); err == nil {
		t.Fatal("load invalid config should return error")
	}

	if err := os.WriteFile(configPath, []byte("[server]\nport = 3456\n"), 0644); err != nil {
		t.Fatalf("write valid config: %v", err)
	}
	var out Config
	if err := Unmarshal(configPath, &out); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if out.Server.Port != 3456 {
		t.Fatalf("unmarshaled port = %d, want 3456", out.Server.Port)
	}
	if err := Unmarshal(filepath.Join(appDir, "missing.toml"), &out); err == nil {
		t.Fatal("Unmarshal missing file should return error")
	}
	if err := os.WriteFile(configPath, []byte("#"+strings.Repeat("x", maxConfigFileBytes)), 0644); err != nil {
		t.Fatalf("write oversized config: %v", err)
	}
	if err := Unmarshal(configPath, &out); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized config error = %v", err)
	}
}

func TestGenerateExample(t *testing.T) {
	appDir := resetConfigForTest(t)

	if err := GenerateExample(); err != nil {
		t.Fatalf("GenerateExample returned error: %v", err)
	}
	configPath := filepath.Join(appDir, "config", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(data)
	for _, want := range []string{"GPT-5", "deepseek-chat", "sk-fkteams-your-api-key", "channels/weixin/credentials.json", "MCP服务名称", "auto_approve"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated config missing %q", want)
		}
	}

	var generated Config
	if err := Unmarshal(configPath, &generated); err != nil {
		t.Fatalf("generated config should be valid TOML: %v", err)
	}
	if generated.ResolveDefaultModel(ModelUseChat) == nil || generated.Server.Auth.Username != "admin" {
		t.Fatalf("generated config = %#v", generated)
	}
}
