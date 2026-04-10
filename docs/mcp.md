# MCP 工具集成指南

## 什么是 MCP？

Model Context Protocol (MCP) 是一个开放的协议标准，用于 AI 应用与外部工具和数据源的集成。fkteams 完整支持 MCP 协议，可以轻松接入丰富的 MCP 工具生态。

## MCP 服务配置

在 `config/config.toml` 中配置 MCP 服务：

```toml
[[custom.mcp_servers]]
name = "filesystem"
desc = "文件系统操作工具"
enabled = true          # 是否启用
timeout = 30           # 超时时间（秒）
url = "http://127.0.0.1:3000/mcp"
transport_type = "http"

[[custom.mcp_servers]]
name = "postgres"
desc = "PostgreSQL 数据库工具"
enabled = true
timeout = 30
command = "npx"        # 启动命令
env_vars = ["DATABASE_URL=postgresql://localhost/mydb"]  # 环境变量
args = ["-y", "@modelcontextprotocol/server-postgres"]   # 命令参数
transport_type = "stdio"
```

## 支持的连接方式

1. **HTTP 方式**
   - 适合：远程 MCP 服务
   - 配置：设置 `url` 和 `transport_type = "http"`

2. **SSE 方式**
   - 适合：需要服务器推送的场景
   - 配置：设置 `url` 和 `transport_type = "sse"`

3. **Stdio 方式**
   - 适合：本地 MCP 工具
   - 配置：设置 `command`、`args` 和 `transport_type = "stdio"`
   - 支持通过 `env_vars` 配置环境变量

## 在自定义智能体中使用 MCP 工具

```toml
[[custom.agents]]
name = "数据处理专家"
desc = "专门处理数据相关任务"
model = "default"  # 引用 [[models]] 中的模型名称
system_prompt = "你是一个数据处理专家..."
tools = [
  "file",              # 内置文件工具
  "mcp-filesystem",    # MCP 文件系统工具（需加 mcp- 前缀）
  "mcp-postgres"       # MCP 数据库工具
]
```

## MCP 工具命名规则

- MCP 服务在配置文件中使用 `name` 字段定义
- 在智能体的 `tools` 列表中引用时，需要添加 `mcp-` 前缀
- 例如：`name = "filesystem"` → 使用时写作 `mcp-filesystem`

## 常用 MCP 服务示例

```toml
# 文件系统操作
[[custom.mcp_servers]]
name = "filesystem"
desc = "文件系统读写工具"
enabled = true
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/directory"]
transport_type = "stdio"

# GitHub 集成
[[custom.mcp_servers]]
name = "github"
desc = "GitHub API 工具"
enabled = true
command = "npx"
env_vars = ["GITHUB_TOKEN=your_github_token"]
args = ["-y", "@modelcontextprotocol/server-github"]
transport_type = "stdio"

# Google Drive
[[custom.mcp_servers]]
name = "gdrive"
desc = "Google Drive 工具"
enabled = true
command = "npx"
args = ["-y", "@modelcontextprotocol/server-gdrive"]
transport_type = "stdio"

# Brave Search
[[custom.mcp_servers]]
name = "brave-search"
desc = "Brave 搜索引擎"
enabled = true
command = "npx"
env_vars = ["BRAVE_API_KEY=your_api_key"]
args = ["-y", "@modelcontextprotocol/server-brave-search"]
transport_type = "stdio"
```

更多 MCP 服务请访问：https://github.com/modelcontextprotocol/servers
