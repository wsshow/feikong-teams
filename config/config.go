package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Server struct {
	Port     int    `toml:"port"`
	LogLevel string `toml:"log_level"`
}

type TeamMember struct {
	Index     int    `toml:"index"`
	Name      string `toml:"name"`
	Desc      string `toml:"desc"`
	BaseURL   string `toml:"base_url"`
	APIKey    string `toml:"api_key"`
	ModelName string `toml:"model_name"`
}

type Roundtable struct {
	Members       []TeamMember `toml:"members"`
	MaxIterations int          `toml:"max_iterations"`
}

type Custom struct {
	Moderator  Agent       `toml:"moderator"`
	Agents     []Agent     `toml:"agents"`
	MCPServers []MCPServer `toml:"mcp_servers"`
}

type Agent struct {
	Name         string   `toml:"name"`
	Desc         string   `toml:"desc"`
	SystemPrompt string   `toml:"system_prompt"`
	BaseURL      string   `toml:"base_url"`
	APIKey       string   `toml:"api_key"`
	ModelName    string   `toml:"model_name"`
	Tools        []string `toml:"tools,omitempty"`
}

type MCPServer struct {
	Name          string   `toml:"name"`
	Desc          string   `toml:"desc"`
	Enabled       bool     `toml:"enabled"`
	Timeout       int      `toml:"timeout"`
	URL           string   `toml:"url,omitempty"`
	Command       string   `toml:"command,omitempty"`  // Command: "uvx" or "npx"
	EnvVars       []string `toml:"env_vars,omitempty"` // Environment variables for stdio
	Args          []string `toml:"args,omitempty"`     // Command arguments array
	TransportType string   `toml:"transport_type"`
}

type Config struct {
	Server     Server     `toml:"server"`
	Roundtable Roundtable `toml:"roundtable"`
	Custom     Custom     `toml:"custom"`
}

func Unmarshal(filePath string, v any) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	err = toml.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

func Get() (*Config, error) {
	var config Config
	err := Unmarshal("config/config.toml", &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func GenerateExample() error {
	filePath := "config/config.toml"
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}
	defaultConfig := Config{
		Custom: Custom{
			Agents: []Agent{
				{
					Name:         "智能体名称",
					Desc:         "智能体描述",
					SystemPrompt: "你是一个有帮助的助手。",
					BaseURL:      "https://api.example.com/v1",
					APIKey:       "your_api_key_here",
					ModelName:    "模型名称",
					Tools:        []string{"工具名称，例如：command", "MCP工具要求添加【mcp-】前缀，例如：mcp-服务名称"},
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
		Roundtable: Roundtable{
			Members: []TeamMember{
				{
					Index:     0,
					Name:      "Deepseek Chat",
					Desc:      "深度求索聊天模型",
					BaseURL:   "https://api.deepseek.com/v1",
					APIKey:    "xxx",
					ModelName: "deepseek-chat",
				},
			},
			MaxIterations: 2,
		},
	}
	data, err := toml.Marshal(defaultConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}
