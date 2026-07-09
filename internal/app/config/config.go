// Package config 提供应用配置文件的加载、解析和示例生成
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"fkteams/internal/app/appdata"

	"github.com/pelletier/go-toml/v2"
)

// ==================== 模型池 ====================

const (
	ModelUseChat    = "chat"
	ModelUseAgent   = "agent"
	ModelUseTitle   = "title"
	ModelUseSummary = "summary"
)

// ModelConfig 可复用的模型配置，通过 ID 稳定引用，Name 仅用于展示。
type ModelConfig struct {
	ID           string   `toml:"id" json:"id"`
	Name         string   `toml:"name" json:"name"`
	UseFor       []string `toml:"use_for,omitempty" json:"use_for,omitempty"`
	Provider     string   `toml:"provider,omitempty" json:"provider"`
	BaseURL      string   `toml:"base_url" json:"base_url"`
	APIKey       string   `toml:"api_key" json:"api_key"`
	Model        string   `toml:"model" json:"model"`
	ExtraHeaders string   `toml:"extra_headers,omitempty" json:"extra_headers"` // 格式: Key1:Value1,Key2:Value2
	HasAPIKey    bool     `toml:"-" json:"has_api_key,omitempty"`               // 是否已配置 APIKey（前端展示用）
	OriginalID   string   `toml:"-" json:"original_id,omitempty"`               // 前端加载时的原始 ID，用于 APIKey 还原匹配
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
	Host         string     `toml:"host" json:"host"`
	Port         int        `toml:"port" json:"port"`
	LogLevel     string     `toml:"log_level" json:"log_level"`
	AllowOrigins []string   `toml:"allow_origins,omitempty" json:"allow_origins"`
	Auth         ServerAuth `toml:"auth" json:"auth"`
}

// ==================== 智能体 ====================

// AgentSSH 描述智能体专属 SSH 连接信息。
type AgentSSH struct {
	Host     string `toml:"host" json:"host"`
	Username string `toml:"username" json:"username"`
	Password string `toml:"password" json:"password"`
}

// AgentConfig 描述一个可全局调用的智能体配置。
type AgentConfig struct {
	ID          string    `toml:"id" json:"id"`
	Name        string    `toml:"name" json:"name"`
	Description string    `toml:"description" json:"description"`
	Prompt      string    `toml:"prompt" json:"prompt"`
	ModelID     string    `toml:"model_id,omitempty" json:"model_id,omitempty"`
	Tools       []string  `toml:"tools,omitempty" json:"tools"`
	SSH         *AgentSSH `toml:"ssh,omitempty" json:"ssh,omitempty"`
	Enabled     bool      `toml:"enabled" json:"enabled"`
	Builtin     bool      `toml:"-" json:"builtin,omitempty"`
	TeamMember  bool      `toml:"-" json:"team_member,omitempty"`
}

// Agents 全局智能体目录配置。
type Agents struct {
	Items []AgentConfig `toml:"items" json:"items"`
}

// ==================== 通道 ====================

// ChannelQQ QQ 机器人通道配置
type ChannelQQ struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	AppID     string `toml:"app_id" json:"app_id"`
	AppSecret string `toml:"app_secret" json:"app_secret"`
	Sandbox   bool   `toml:"sandbox" json:"sandbox"`
	Mode      string `toml:"mode" json:"mode"` // 运行模式: team(默认), deep, roundtable, agent
	AgentID   string `toml:"agent_id,omitempty" json:"agent_id,omitempty"`
}

// ChannelDiscord Discord 机器人通道配置
type ChannelDiscord struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	Token     string `toml:"token" json:"token"`
	AllowFrom string `toml:"allow_from" json:"allow_from"` // 允许的用户 ID，多个用逗号分隔（空则允许所有人）
	Mode      string `toml:"mode" json:"mode"`             // 运行模式: team(默认), deep, roundtable, agent
	AgentID   string `toml:"agent_id,omitempty" json:"agent_id,omitempty"`
}

// ChannelWeixin 微信机器人通道配置
type ChannelWeixin struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	BaseURL   string `toml:"base_url" json:"base_url"`     // 自定义 API 地址（可选）
	CredPath  string `toml:"cred_path" json:"cred_path"`   // 凭证存储路径（可选）
	LogLevel  string `toml:"log_level" json:"log_level"`   // 日志级别: debug, info, warn, error, silent
	AllowFrom string `toml:"allow_from" json:"allow_from"` // 允许的用户 ID，多个用逗号分隔（空则允许所有人）
	Mode      string `toml:"mode" json:"mode"`             // 运行模式: team(默认), deep, roundtable, agent
	AgentID   string `toml:"agent_id,omitempty" json:"agent_id,omitempty"`
}

// ChannelEntry 统一通道配置条目
type ChannelEntry struct {
	Name    string
	Mode    string
	AgentID string
	Extra   map[string]string
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
			Name:    "qq",
			Mode:    c.QQ.Mode,
			AgentID: c.QQ.AgentID,
			Extra: map[string]string{
				"app_id":     c.QQ.AppID,
				"app_secret": c.QQ.AppSecret,
				"sandbox":    fmt.Sprintf("%v", c.QQ.Sandbox),
			},
		})
	}
	if c.Discord.Enabled {
		entries = append(entries, ChannelEntry{
			Name:    "discord",
			Mode:    c.Discord.Mode,
			AgentID: c.Discord.AgentID,
			Extra: map[string]string{
				"token":      c.Discord.Token,
				"allow_from": c.Discord.AllowFrom,
			},
		})
	}
	if c.Weixin.Enabled {
		entries = append(entries, ChannelEntry{
			Name:    "weixin",
			Mode:    c.Weixin.Mode,
			AgentID: c.Weixin.AgentID,
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
	ID          string `toml:"id" json:"id"`
	Name        string `toml:"name" json:"name"`
	Description string `toml:"description" json:"description"`
	ModelID     string `toml:"model_id" json:"model_id"` // 引用 models 中的 id
	Prompt      string `toml:"prompt,omitempty" json:"prompt,omitempty"`
}

// Roundtable 圆桌讨论模式配置
type Roundtable struct {
	Members       []TeamMember `toml:"members" json:"members"`
	MaxIterations int          `toml:"max_iterations" json:"max_iterations"`
}

// DeepPlanning 配置 Deep 模式的内建计划能力。
type DeepPlanning struct {
	Enabled bool `toml:"enabled" json:"enabled"`
}

// DeepWorkspace 配置 Deep 模式的工作区文件能力。
type DeepWorkspace struct {
	Enabled bool `toml:"enabled" json:"enabled"`
}

// DeepShell 配置 Deep 模式的命令执行能力。
type DeepShell struct {
	Enabled   bool   `toml:"enabled" json:"enabled"`
	Streaming bool   `toml:"streaming" json:"streaming"`
	Timeout   string `toml:"timeout,omitempty" json:"timeout,omitempty"`
}

// DeepDelegation 配置 Deep 模式对子智能体的任务委派能力。
type DeepDelegation struct {
	GeneralAgent        bool   `toml:"general_agent" json:"general_agent"`
	TaskToolDescription string `toml:"task_tool_description,omitempty" json:"task_tool_description,omitempty"`
}

// DeepContext 配置 Deep 模式使用的项目上下文增强。
type DeepContext struct {
	Summary  bool `toml:"summary" json:"summary"`
	AgentsMD bool `toml:"agents_md" json:"agents_md"`
}

// DeepOutput 配置 Deep 模式的运行输出。
type DeepOutput struct {
	Key string `toml:"key,omitempty" json:"key,omitempty"`
}

// Deep 配置深度智能体模式。
type Deep struct {
	Instruction   string         `toml:"instruction,omitempty" json:"instruction,omitempty"`
	MaxIterations int            `toml:"max_iterations" json:"max_iterations"`
	Planning      DeepPlanning   `toml:"planning" json:"planning"`
	Workspace     DeepWorkspace  `toml:"workspace" json:"workspace"`
	Shell         DeepShell      `toml:"shell" json:"shell"`
	Delegation    DeepDelegation `toml:"delegation" json:"delegation"`
	Context       DeepContext    `toml:"context" json:"context"`
	Output        DeepOutput     `toml:"output" json:"output"`
	ExtraTools    []string       `toml:"extra_tools,omitempty" json:"extra_tools,omitempty"`
}

func DefaultDeep() Deep {
	return Deep{
		MaxIterations: 20,
		Planning: DeepPlanning{
			Enabled: true,
		},
		Workspace: DeepWorkspace{
			Enabled: true,
		},
		Shell: DeepShell{
			Enabled: true,
			Timeout: "30s",
		},
		Delegation: DeepDelegation{
			GeneralAgent: true,
		},
		Context: DeepContext{
			Summary:  true,
			AgentsMD: true,
		},
		ExtraTools: []string{"doc", "search", "fetch", "ask"},
	}
}

func (d Deep) WithDefaults() Deep {
	defaults := DefaultDeep()
	if d.isZero() {
		return defaults
	}
	if d.MaxIterations == 0 {
		d.MaxIterations = defaults.MaxIterations
	}
	if d.Shell.Timeout == "" {
		d.Shell.Timeout = defaults.Shell.Timeout
	}
	if d.ExtraTools == nil {
		d.ExtraTools = defaults.ExtraTools
	}
	return d
}

func (d Deep) isZero() bool {
	return d.Instruction == "" &&
		d.MaxIterations == 0 &&
		!d.Planning.Enabled &&
		!d.Workspace.Enabled &&
		!d.Shell.Enabled &&
		!d.Shell.Streaming &&
		d.Shell.Timeout == "" &&
		!d.Delegation.GeneralAgent &&
		d.Delegation.TaskToolDescription == "" &&
		!d.Context.Summary &&
		!d.Context.AgentsMD &&
		d.Output.Key == "" &&
		len(d.ExtraTools) == 0
}

// MCPServer MCP 服务配置，支持 HTTP 和 stdio 两种传输方式
type MCPServer struct {
	ID          string            `toml:"id" json:"id"`
	Name        string            `toml:"name" json:"name"`
	Description string            `toml:"description" json:"description"`
	Enabled     bool              `toml:"enabled" json:"enabled"`
	Timeout     string            `toml:"timeout,omitempty" json:"timeout,omitempty"`
	URL         string            `toml:"url,omitempty" json:"url"`
	Command     string            `toml:"command,omitempty" json:"command"` // Command: "uvx" or "npx"
	Env         map[string]string `toml:"env,omitempty" json:"env,omitempty"`
	Args        []string          `toml:"args,omitempty" json:"args"` // Command arguments array
	Transport   string            `toml:"transport" json:"transport"`
}

// ToolSettings 工具配置。
type ToolSettings struct {
	MCPServers []MCPServer          `toml:"mcp_servers" json:"mcp_servers"`
	Approval   ToolApprovalSettings `toml:"approval" json:"approval"`
}

// ToolApprovalSettings 工具审批配置。
type ToolApprovalSettings struct {
	AutoApprove []string `toml:"auto_approve" json:"auto_approve"`
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
	Memory     Memory        `toml:"memory" json:"memory"`
	Server     Server        `toml:"server" json:"server"`
	OpenAIAPI  OpenAIAPI     `toml:"openai_api" json:"openai_api"`
	Agents     Agents        `toml:"agents" json:"agents"`
	Channels   Channels      `toml:"channels" json:"channels"`
	Roundtable Roundtable    `toml:"roundtable" json:"roundtable"`
	Deep       Deep          `toml:"deep" json:"deep"`
	Tools      ToolSettings  `toml:"tools" json:"tools"`
}

// ResolveModel 通过稳定 ID 查找模型配置，空 ID 返回默认对话模型。
func (c *Config) ResolveModel(id string) *ModelConfig {
	if id == "" {
		return c.ResolveDefaultModel(ModelUseChat)
	}
	for i := range c.Models {
		if c.Models[i].ID == id {
			return &c.Models[i]
		}
	}
	return nil
}

// ResolveDefaultModel 返回指定用途的默认模型；非 chat 用途未配置时回退到 chat。
func (c *Config) ResolveDefaultModel(use string) *ModelConfig {
	if use == "" {
		use = ModelUseChat
	}
	for i := range c.Models {
		for _, item := range c.Models[i].UseFor {
			if item == use {
				return &c.Models[i]
			}
		}
	}
	if use != ModelUseChat {
		return c.ResolveDefaultModel(ModelUseChat)
	}
	return nil
}

func (c *Config) ValidateModels() error {
	modelIDs := make(map[string]struct{}, len(c.Models))
	uses := make(map[string]string)
	for _, m := range c.Models {
		if m.ID == "" {
			return fmt.Errorf("model id is required")
		}
		if _, ok := modelIDs[m.ID]; ok {
			return fmt.Errorf("duplicate model id: %s", m.ID)
		}
		modelIDs[m.ID] = struct{}{}
		for _, use := range m.UseFor {
			if use == "" {
				continue
			}
			if existing := uses[use]; existing != "" {
				return fmt.Errorf("model use_for %q is configured by both %s and %s", use, existing, m.ID)
			}
			uses[use] = m.ID
		}
	}
	if _, ok := uses[ModelUseChat]; !ok {
		return fmt.Errorf("model use_for %q is required", ModelUseChat)
	}
	return nil
}

func (c *Config) ValidateRoundtable() error {
	if c == nil {
		return nil
	}
	if c.Roundtable.MaxIterations < 0 {
		return fmt.Errorf("roundtable.max_iterations must be >= 0")
	}
	return nil
}

func (c *Config) ValidateDeep() error {
	if c == nil {
		return nil
	}
	if c.Deep.MaxIterations < 0 {
		return fmt.Errorf("deep.max_iterations must be >= 0")
	}
	return nil
}

// WorkspaceDir 返回工作区目录（固定为 ~/.fkteams/workspace）
func (c *Config) WorkspaceDir() string {
	return filepath.Join(appdata.Dir(), "workspace")
}

// ==================== 全局单例 ====================

var (
	globalConfig atomic.Pointer[Config]
	configOnce   sync.Once
	configMu     sync.Mutex // 保护写操作
)

func configFilePath() string {
	return filepath.Join(appdata.Dir(), "config", "config.toml")
}

// Init 初始化全局配置（应在启动时调用一次）
func Init() error {
	var initErr error
	configOnce.Do(func() {
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
	if err := cfg.ValidateModels(); err != nil {
		return err
	}
	if mc := cfg.ResolveDefaultModel(ModelUseChat); mc != nil && (mc.APIKey != "" || mc.Provider != "") {
		return nil
	}
	configPath := filepath.Join(appdata.Dir(), "config", "config.toml")
	return fmt.Errorf("未配置默认模型，请先完成配置后再使用\n\n"+
		"  生成配置文件并编辑\n"+
		"    fkteams generate config\n"+
		"    编辑 %s", configPath)
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
	if err := Unmarshal(filepath.Join(appdata.Dir(), "config", "config.toml"), &config); err != nil {
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
	filePath := filepath.Join(appdata.Dir(), "config", "config.toml")
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}
	exampleConfig := Config{
		Models: []ModelConfig{
			{
				ID:       "main",
				Name:     "主力模型",
				UseFor:   []string{ModelUseChat, ModelUseAgent},
				Provider: "openai",
				BaseURL:  "https://api.openai.com/v1",
				APIKey:   "xxxxx",
				Model:    "GPT-5",
			},
			{
				ID:       "fast",
				Name:     "快速模型",
				UseFor:   []string{ModelUseTitle, ModelUseSummary},
				Provider: "deepseek",
				BaseURL:  "https://api.deepseek.com/v1",
				APIKey:   "xxxxx",
				Model:    "deepseek-chat",
			},
			{
				ID:       "copilot",
				Name:     "Copilot",
				Provider: "copilot",
				Model:    "gpt-4o",
			},
		},
		Memory: Memory{
			Enabled: false,
		},
		Server: Server{
			Host:     "127.0.0.1",
			Port:     23456,
			LogLevel: "info",
			// 默认允许同源和 localhost；跨域部署时在这里显式添加前端地址。
			AllowOrigins: []string{"http://localhost:23456", "http://127.0.0.1:23456"},
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
			Items: []AgentConfig{
				{
					ID:          "coordinator",
					Name:        "协调者",
					Description: "核心工程智能体，直接完成常规工程任务，并按需指派专业成员。",
					Prompt:      "",
					Tools:       []string{"todo", "file", "command", "scheduler", "ask"},
					Enabled:     true,
				},
				{
					ID:          "coder",
					Name:        "代码工程师",
					Description: "软件工程师，负责代码实现、调试、重构和工程验证。",
					Prompt:      "",
					Tools:       []string{"file", "command"},
					Enabled:     true,
				},
				{
					ID:          "researcher",
					Name:        "研究员",
					Description: "网络研究员，负责检索、抓取、交叉验证和整理时效信息。",
					Prompt:      "",
					Tools:       []string{"search", "fetch"},
					Enabled:     true,
				},
				{
					ID:          "analyst",
					Name:        "数据分析师",
					Description: "数据分析师，负责使用表格、脚本和文档工具提取洞察。",
					Prompt:      "",
					Tools:       []string{"todo", "excel", "file", "uv", "doc"},
					Enabled:     false,
				},
				{
					ID:          "remote",
					Name:        "远程运维",
					Description: "远程运维专家，负责通过 SSH 管理服务器、执行命令和传输文件。",
					Prompt:      "",
					Tools:       []string{"ssh"},
					SSH: &AgentSSH{
						Host:     "ip:port",
						Username: "your_ssh_user",
						Password: "your_ssh_password",
					},
					Enabled: false,
				},
				{
					ID:          "generalist",
					Name:        "通用助手",
					Description: "通用执行助手，负责综合命令、文件、搜索和文档工具完成开放任务。",
					Prompt:      "",
					Tools:       []string{"command", "file", "search", "fetch", "ask", "doc"},
					Enabled:     false,
				},
			},
		},
		Channels: Channels{
			QQ: ChannelQQ{
				Enabled:   false,
				AppID:     "your_app_id",
				AppSecret: "your_app_secret",
				Sandbox:   true,
				Mode:      "team",
				AgentID:   "",
			},
			Discord: ChannelDiscord{
				Enabled: false,
				Token:   "your_discord_bot_token",
				Mode:    "team",
				AgentID: "",
			},
			Weixin: ChannelWeixin{
				Enabled:   false,
				BaseURL:   "https://ilinkai.weixin.qq.com",
				CredPath:  "channels/weixin/credentials.json",
				LogLevel:  "info",
				AllowFrom: "",
				Mode:      "team",
				AgentID:   "",
			},
		},
		Roundtable: Roundtable{
			Members: []TeamMember{
				{
					ID:          "deepseek",
					Name:        "Deepseek Chat",
					Description: "深度求索聊天模型",
					ModelID:     "fast",
					Prompt:      "",
				},
			},
			MaxIterations: 2,
		},
		Deep: DefaultDeep(),
		Tools: ToolSettings{
			Approval: ToolApprovalSettings{
				AutoApprove: []string{},
			},
			MCPServers: []MCPServer{
				{
					ID:          "remote_mcp",
					Name:        "MCP服务名称",
					Description: "MCP服务描述",
					Enabled:     false,
					Timeout:     "30s",
					URL:         "http://127.0.0.1:12345/mcp",
					Transport:   "http",
				},
				{
					ID:          "local_stdio_mcp",
					Name:        "本地Stdio MCP服务",
					Description: "通过stdio通信的本地MCP服务",
					Enabled:     false,
					Timeout:     "30s",
					Command:     "go",
					Env:         map[string]string{"FEIKONG_MCP_LOG_LEVEL": "info"},
					Args:        []string{"run", "main.go"},
					Transport:   "stdio",
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
