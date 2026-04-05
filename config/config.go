// Package config 提供应用配置文件的加载、解析和示例生成
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"fkteams/common"

	"github.com/pelletier/go-toml/v2"
)

// ==================== 模型池 ====================

// ModelConfig 可复用的模型配置，通过 Name 引用
type ModelConfig struct {
	Name         string `toml:"name" json:"name"`
	Provider     string `toml:"provider,omitempty" json:"provider"`
	BaseURL      string `toml:"base_url" json:"base_url"`
	APIKey       string `toml:"api_key" json:"api_key"`
	Model        string `toml:"model" json:"model"`
	ExtraHeaders string `toml:"extra_headers,omitempty" json:"extra_headers"` // 格式: Key1:Value1,Key2:Value2
}

// ParseExtraHeaders 解析额外请求头字符串为 map
func (m *ModelConfig) ParseExtraHeaders() map[string]string {
	if m.ExtraHeaders == "" {
		return nil
	}
	headers := make(map[string]string)
	for _, pair := range strings.Split(m.ExtraHeaders, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}

// ==================== 代理 ====================

// Proxy 网络代理配置
type Proxy struct {
	URL string `toml:"url" json:"url"`
}

// ==================== 记忆 ====================

// Memory 长期记忆配置
type Memory struct {
	Enabled bool `toml:"enabled" json:"enabled"`
}

// ==================== 服务器 ====================

// ServerAuth Web 认证配置
type ServerAuth struct {
	Enabled  bool   `toml:"enabled" json:"enabled"`
	Username string `toml:"username" json:"username"`
	Password string `toml:"password" json:"password"`
	Secret   string `toml:"secret" json:"secret"`
}

// Server 服务端配置
type Server struct {
	Host     string     `toml:"host" json:"host"`
	Port     int        `toml:"port" json:"port"`
	LogLevel string     `toml:"log_level" json:"log_level"`
	Auth     ServerAuth `toml:"auth" json:"auth"`
}

// ==================== 智能体 ====================

// SSHVisitor SSH 远程访问智能体配置
type SSHVisitor struct {
	Enabled  bool   `toml:"enabled" json:"enabled"`
	Host     string `toml:"host" json:"host"`
	Username string `toml:"username" json:"username"`
	Password string `toml:"password" json:"password"`
}

// Agents 内置智能体开关
type Agents struct {
	Searcher   bool       `toml:"searcher" json:"searcher"`
	Assistant  bool       `toml:"assistant" json:"assistant"`
	Analyst    bool       `toml:"analyst" json:"analyst"`
	SSHVisitor SSHVisitor `toml:"ssh_visitor" json:"ssh_visitor"`
}

// ==================== 通道 ====================

// ChannelQQ QQ 机器人通道配置
type ChannelQQ struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	AppID     string `toml:"app_id" json:"app_id"`
	AppSecret string `toml:"app_secret" json:"app_secret"`
	Sandbox   bool   `toml:"sandbox" json:"sandbox"`
	Mode      string `toml:"mode" json:"mode"` // 运行模式: team(默认), deep, roundtable, custom 或智能体名称
}

// ChannelDiscord Discord 机器人通道配置
type ChannelDiscord struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	Token     string `toml:"token" json:"token"`
	AllowFrom string `toml:"allow_from" json:"allow_from"` // 允许的用户 ID，多个用逗号分隔（空则允许所有人）
	Mode      string `toml:"mode" json:"mode"`             // 运行模式: team(默认), deep, roundtable, custom 或智能体名称
}

// ChannelWeixin 微信机器人通道配置
type ChannelWeixin struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	BaseURL   string `toml:"base_url" json:"base_url"`     // 自定义 API 地址（可选）
	CredPath  string `toml:"cred_path" json:"cred_path"`   // 凭证存储路径（可选）
	LogLevel  string `toml:"log_level" json:"log_level"`   // 日志级别: debug, info, warn, error, silent
	AllowFrom string `toml:"allow_from" json:"allow_from"` // 允许的用户 ID，多个用逗号分隔（空则允许所有人）
	Mode      string `toml:"mode" json:"mode"`             // 运行模式: team(默认), deep, roundtable, custom 或智能体名称
}

// ChannelEntry 统一通道配置条目
type ChannelEntry struct {
	Name  string
	Mode  string
	Extra map[string]string
}

// Channels 消息通道配置
type Channels struct {
	QQ      ChannelQQ      `toml:"qq" json:"qq"`
	Discord ChannelDiscord `toml:"discord" json:"discord"`
	Weixin  ChannelWeixin  `toml:"weixin" json:"weixin"`
}

// List 返回所有已启用的通道配置（供统一注册使用）
func (c Channels) List() []ChannelEntry {
	var entries []ChannelEntry
	if c.QQ.Enabled {
		entries = append(entries, ChannelEntry{
			Name: "qq",
			Mode: c.QQ.Mode,
			Extra: map[string]string{
				"app_id":     c.QQ.AppID,
				"app_secret": c.QQ.AppSecret,
				"sandbox":    fmt.Sprintf("%v", c.QQ.Sandbox),
			},
		})
	}
	if c.Discord.Enabled {
		entries = append(entries, ChannelEntry{
			Name: "discord",
			Mode: c.Discord.Mode,
			Extra: map[string]string{
				"token":      c.Discord.Token,
				"allow_from": c.Discord.AllowFrom,
			},
		})
	}
	if c.Weixin.Enabled {
		entries = append(entries, ChannelEntry{
			Name: "weixin",
			Mode: c.Weixin.Mode,
			Extra: map[string]string{
				"base_url":   c.Weixin.BaseURL,
				"cred_path":  c.Weixin.CredPath,
				"log_level":  c.Weixin.LogLevel,
				"allow_from": c.Weixin.AllowFrom,
			},
		})
	}
	return entries
}

// ==================== 圆桌讨论 ====================

// TeamMember 圆桌讨论模式的成员配置
type TeamMember struct {
	Index int    `toml:"index" json:"index"`
	Name  string `toml:"name" json:"name"`
	Desc  string `toml:"desc" json:"desc"`
	Model string `toml:"model" json:"model"` // 引用 models 中的 name
}

// Roundtable 圆桌讨论模式配置
type Roundtable struct {
	Members       []TeamMember `toml:"members" json:"members"`
	MaxIterations int          `toml:"max_iterations" json:"max_iterations"`
}

// ==================== 自定义模式 ====================

// CustomAgent 自定义智能体配置
type CustomAgent struct {
	Name         string   `toml:"name" json:"name"`
	Desc         string   `toml:"desc" json:"desc"`
	SystemPrompt string   `toml:"system_prompt" json:"system_prompt"`
	Model        string   `toml:"model" json:"model"` // 引用 models 中的 name
	Tools        []string `toml:"tools,omitempty" json:"tools"`
}

// MCPServer MCP 服务配置，支持 HTTP 和 stdio 两种传输方式
type MCPServer struct {
	Name          string   `toml:"name" json:"name"`
	Desc          string   `toml:"desc" json:"desc"`
	Enabled       bool     `toml:"enabled" json:"enabled"`
	Timeout       int      `toml:"timeout" json:"timeout"`
	URL           string   `toml:"url,omitempty" json:"url"`
	Command       string   `toml:"command,omitempty" json:"command"`   // Command: "uvx" or "npx"
	EnvVars       []string `toml:"env_vars,omitempty" json:"env_vars"` // Environment variables for stdio
	Args          []string `toml:"args,omitempty" json:"args"`         // Command arguments array
	TransportType string   `toml:"transport_type" json:"transport_type"`
}

// Custom 自定义会议模式配置
type Custom struct {
	Moderator  CustomAgent   `toml:"moderator" json:"moderator"`
	Agents     []CustomAgent `toml:"agents" json:"agents"`
	MCPServers []MCPServer   `toml:"mcp_servers" json:"mcp_servers"`
}

// ==================== OpenAI 兼容 API ====================

// OpenAIAPI OpenAI 兼容 API 配置
type OpenAIAPI struct {
	APIKeys []string `toml:"api_keys,omitempty" json:"api_keys"` // 访问密钥
}

// ==================== 全局配置 ====================

// Config 应用全局配置
type Config struct {
	Models     []ModelConfig `toml:"models" json:"models"`
	Proxy      Proxy         `toml:"proxy" json:"proxy"`
	Memory     Memory        `toml:"memory" json:"memory"`
	Server     Server        `toml:"server" json:"server"`
	OpenAIAPI  OpenAIAPI     `toml:"openai_api" json:"openai_api"`
	Agents     Agents        `toml:"agents" json:"agents"`
	Channels   Channels      `toml:"channels" json:"channels"`
	Roundtable Roundtable    `toml:"roundtable" json:"roundtable"`
	Custom     Custom        `toml:"custom" json:"custom"`
}

// ResolveModel 通过名称查找模型配置，空名称返回 "default" 模型
func (c *Config) ResolveModel(name string) *ModelConfig {
	if name == "" {
		name = "default"
	}
	for i := range c.Models {
		if c.Models[i].Name == name {
			return &c.Models[i]
		}
	}
	return nil
}

// ProxyURL 返回代理 URL（配置文件优先，环境变量回退）
func (c *Config) ProxyURL() string {
	if c != nil && c.Proxy.URL != "" {
		return c.Proxy.URL
	}
	return os.Getenv("FEIKONG_PROXY_URL")
}

// WorkspaceDir 返回工作区目录（固定为 ~/.fkteams/workspace）
func (c *Config) WorkspaceDir() string {
	return filepath.Join(common.AppDir(), "workspace")
}

// ==================== 全局单例 ====================

var (
	globalConfig atomic.Pointer[Config]
	configOnce   sync.Once
	configPath   string
	configMu     sync.Mutex // 保护写操作
)

func configFilePath() string {
	return filepath.Join(common.AppDir(), "config", "config.toml")
}

// Init 初始化全局配置（应在启动时调用一次）
func Init() error {
	var initErr error
	configOnce.Do(func() {
		configPath = configFilePath()
		cfg, err := load()
		if err != nil {
			initErr = err
			return
		}
		globalConfig.Store(cfg)
	})
	return initErr
}

// Get 返回全局配置（未初始化时返回默认值）
func Get() *Config {
	if cfg := globalConfig.Load(); cfg != nil {
		return cfg
	}
	return defaultConfig()
}

// Reload 重新从文件加载配置（热重载）
func Reload() error {
	cfg, err := load()
	if err != nil {
		return err
	}
	globalConfig.Store(cfg)
	return nil
}

// Save 保存配置到文件（先写临时文件再 rename，防数据丢失）
func Save(cfg *Config) error {
	configMu.Lock()
	defer configMu.Unlock()

	filePath := configFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	// 更新内存中的配置
	globalConfig.Store(cfg)
	return nil
}

// EnsureDefaultModel 检查是否配置了默认模型，未配置时返回引导信息
func ensureDefaultModel() error {
	cfg := Get()
	// 配置文件中有 default 模型
	if mc := cfg.ResolveModel("default"); mc != nil && (mc.APIKey != "" || mc.Provider != "") {
		return nil
	}
	// 环境变量回退
	if os.Getenv("FEIKONG_API_KEY") != "" {
		return nil
	}
	configPath := filepath.Join(common.AppDir(), "config", "config.toml")
	return fmt.Errorf("未配置默认模型，请先完成配置后再使用\n\n"+
		"  方式一：生成配置文件并编辑\n"+
		"    fkteams generate config\n"+
		"    编辑 %s\n\n"+
		"  方式二：设置环境变量\n"+
		"    export FEIKONG_API_KEY=your_api_key\n"+
		"    export FEIKONG_BASE_URL=https://api.openai.com/v1\n"+
		"    export FEIKONG_MODEL=gpt-5", configPath)
}

// InitAndValidate 初始化配置并校验必要参数
func InitAndValidate() error {
	if err := Init(); err != nil {
		return err
	}
	return ensureDefaultModel()
}

// load 从文件加载配置
func load() (*Config, error) {
	var config Config
	if err := Unmarshal(filepath.Join(common.AppDir(), "config", "config.toml"), &config); err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}
	return &config, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: Server{
			Host:     "127.0.0.1",
			Port:     23456,
			LogLevel: "info",
		},
	}
}

// Unmarshal 从 TOML 文件反序列化配置
func Unmarshal(filePath string, v any) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return toml.Unmarshal(data, v)
}

// GenerateExample 生成示例配置文件
func GenerateExample() error {
	filePath := filepath.Join(common.AppDir(), "config", "config.toml")
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}
	exampleConfig := Config{
		Models: []ModelConfig{
			{
				Name:     "default",
				Provider: "openai",
				BaseURL:  "https://api.openai.com/v1",
				APIKey:   "xxxxx",
				Model:    "GPT-5",
			},
			{
				Name:     "deepseek",
				Provider: "deepseek",
				BaseURL:  "https://api.deepseek.com/v1",
				APIKey:   "xxxxx",
				Model:    "deepseek-chat",
			},
			{
				Name:     "copilot",
				Provider: "copilot",
				Model:    "gpt-4o",
			},
		},
		Proxy: Proxy{
			URL: "http://127.0.0.1:7890",
		},
		Memory: Memory{
			Enabled: false,
		},
		Server: Server{
			Host:     "127.0.0.1",
			Port:     23456,
			LogLevel: "info",
			Auth: ServerAuth{
				Enabled:  false,
				Username: "admin",
				Password: "admin",
				Secret:   "your_jwt_secret_here",
			},
		},
		OpenAIAPI: OpenAIAPI{
			APIKeys: []string{"sk-fkteams-your-api-key"},
		},
		Agents: Agents{
			Searcher:  true,
			Assistant: true,
			Analyst:   false,
			SSHVisitor: SSHVisitor{
				Enabled:  false,
				Host:     "",
				Username: "",
				Password: "",
			},
		},
		Channels: Channels{
			QQ: ChannelQQ{
				Enabled:   false,
				AppID:     "your_app_id",
				AppSecret: "your_app_secret",
				Sandbox:   true,
				Mode:      "team",
			},
			Discord: ChannelDiscord{
				Enabled: false,
				Token:   "your_discord_bot_token",
				Mode:    "team",
			},
			Weixin: ChannelWeixin{
				Enabled:   false,
				BaseURL:   "https://ilinkai.weixin.qq.com",
				CredPath:  "channels/weixin/credentials.json",
				LogLevel:  "info",
				AllowFrom: "",
				Mode:      "team",
			},
		},
		Roundtable: Roundtable{
			Members: []TeamMember{
				{
					Index: 0,
					Name:  "Deepseek Chat",
					Desc:  "深度求索聊天模型",
					Model: "deepseek",
				},
			},
			MaxIterations: 2,
		},
		Custom: Custom{
			Moderator: CustomAgent{
				Name:         "主持人名称",
				Desc:         "主持人描述",
				SystemPrompt: "你是一个公正的主持人，负责引导讨论。",
				Model:        "default",
			},
			Agents: []CustomAgent{
				{
					Name:         "智能体名称",
					Desc:         "智能体描述",
					SystemPrompt: "你是一个有帮助的助手。",
					Model:        "default",
					Tools:        []string{"command", "mcp-服务名称"},
				},
			},
			MCPServers: []MCPServer{
				{
					Name:          "MCP服务名称",
					Desc:          "MCP服务描述",
					Enabled:       false,
					Timeout:       30,
					URL:           "http://127.0.0.1:12345/mcp",
					TransportType: "http",
				},
				{
					Name:          "本地Stdio MCP服务",
					Desc:          "通过stdio通信的本地MCP服务",
					Enabled:       false,
					Timeout:       30,
					Command:       "go",
					EnvVars:       []string{"FEIKONG_MCP_LOG_LEVEL=info"},
					Args:          []string{"run", "main.go"},
					TransportType: "stdio",
				},
			},
		},
	}
	data, err := toml.Marshal(exampleConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}
