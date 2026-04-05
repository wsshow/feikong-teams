# 配置指南

## 配置文件

运行以下命令生成示例配置文件：

```bash
fkteams generate config
```

编辑 `~/.fkteams/config/config.toml` 进行配置。

### 模型配置（必填）

```toml
[[models]]
name = "default"
provider = "openai"  # 可选，自动检测。支持: openai, deepseek, claude, ollama, ark, gemini, qwen, openrouter
base_url = "https://api.openai.com/v1"
api_key = "your_api_key_here"
model = "gpt-5"
# extra_headers = "X-Custom-Auth:your-token,X-Gateway-Key:your-key"  # 额外 HTTP 请求头（可选）
```

### 代理配置（可选）

```toml
[proxy]
url = "http://127.0.0.1:7890"
```

### 智能体开关

```toml
[agents]
searcher = true        # 搜索专家
assistant = true       # 个人全能助手（带审批以及子任务功能）
analyst = false        # 数据分析师

[agents.ssh_visitor]   # SSH 访问者智能体（可选）
enabled = false
host = "ip:port"
username = "your_ssh_user"
password = "your_ssh_password"
```

### 记忆配置

```toml
[memory]
enabled = true  # 全局长期记忆
```

### Web 认证（可选）

```toml
[server.auth]
enabled = false
username = "admin"
password = "your_password"
secret = "your_jwt_secret"
```

### OpenAI 兼容 API

服务内置了 OpenAI 兼容的 API 端点，任意支持 OpenAI API 的客户端（如 Cursor、ChatBox、Open WebUI 等）都可以直接接入。

在 `config.toml` 中添加配置段：

```toml
[openai_api]
api_keys = ["sk-fkteams-your-secret-key"]
```

> **注意**：必须配置 `api_keys` 才能访问 API 端点，未配置时所有请求将返回 401 错误。

密钥可自动生成：

```bash
fkteams generate apikey
```

客户端配置：

- **API Base URL**：`http://<host>:<port>/v1`
- **API Key**：配置文件中设置的密钥
- **Model**：配置文件中定义的模型名称（如 `default`、`deepseek` 等）

支持的端点：

| 端点                        | 说明                 |
| --------------------------- | -------------------- |
| `GET /v1/models`            | 获取可用模型列表     |
| `POST /v1/chat/completions` | 聊天补全（支持流式） |

### 环境变量

仅用于 Docker 等不便使用配置文件的场景，配置文件优先。

| 变量名                   | 说明                              | 默认值       |
| ------------------------ | --------------------------------- | ------------ |
| `FEIKONG_APP_DIR`        | 应用数据目录                      | `~/.fkteams` |
| `FEIKONG_PROXY_URL`      | 代理地址                          | -            |
| `FEIKONG_BASE_URL`       | 模型 API 地址（回退用）           | -            |
| `FEIKONG_API_KEY`        | 模型 API 密钥（回退用）           | -            |
| `FEIKONG_MODEL`          | 模型名称（回退用）                | -            |
| `FEIKONG_PROVIDER`       | 模型提供者类型（回退用）          | 自动检测     |
| `FEIKONG_EXTRA_HEADERS`  | 额外 HTTP 请求头                  | -            |
| `FEIKONG_MAX_ITERATIONS` | 智能体最大迭代次数（0/-1 不限制） | `60`         |

## 配置圆桌会议成员

在 `~/.fkteams/config/config.toml` 中配置圆桌会议成员、MCP 服务和自定义智能体：

```toml
[server]
port = 23456        # Web服务器端口
log_level = "info"  # 日志级别

# 圆桌会议配置
[roundtable]
max_iterations = 2  # 讨论轮数

[[roundtable.members]]
index = 0
name = '深度求索'
desc = '深度求索聊天模型，擅长逻辑分析'
base_url = 'https://api.deepseek.com/v1'
api_key = 'your_deepseek_api_key'
model_name = 'deepseek-chat'

[[roundtable.members]]
index = 1
name = '克劳德'
desc = '克劳德聊天模型，擅长创意思维'
base_url = 'https://api.anthropic.com/v1'
api_key = 'your_claude_api_key'
model_name = 'claude-3-sonnet'

# 自定义智能体配置
[custom]

# 配置自定义智能体
[[custom.agents]]
name = "数据分析师"
desc = "专业的数据分析智能体"
system_prompt = """你是一个专业的数据分析师，擅长数据处理和可视化。
你需要：
1. 分析用户提供的数据
2. 使用合适的工具进行数据处理
3. 生成可视化图表
4. 给出专业的分析建议
"""
base_url = "https://api.openai.com/v1"
api_key = "your_api_key"
model_name = "gpt-4"
tools = ["file", "command", "mcp-filesystem"]  # 可使用内置工具和MCP工具

# 配置 MCP 服务
[[custom.mcp_servers]]
name = "filesystem"  # MCP服务名称，使用时需加前缀：mcp-filesystem
desc = "文件系统操作工具"
enabled = true
timeout = 30
url = "http://127.0.0.1:3000/mcp"
transport_type = "http"  # 支持 http, sse, stdio

[[custom.mcp_servers]]
name = "database"
desc = "数据库操作工具"
enabled = true
timeout = 30
command = "npx"  # 或 "uvx" for Python
env_vars = ["DATABASE_URL=postgresql://localhost/mydb"]
args = ["-y", "@modelcontextprotocol/server-postgres"]
transport_type = "stdio"  # stdio 方式启动本地 MCP 服务
```

### 内置工具列表

- `file` - 文件读写操作（限制在 workspace 目录），支持 unified diff 批量修改
- `git` - Git 仓库操作
- `excel` - Excel 文件处理
- `doc` - 文档处理工具
- `command` - 命令执行（带安全审批），危险命令需用户确认后才执行
- `ssh` - SSH 远程连接
- `search` - 网络搜索（DuckDuckGo）
- `fetch` - 网页抓取工具
- `todo` - 待办事项管理
- `scheduler` - 定时任务调度
- `uv` - Python uv 脚本工具
- `bun` - JavaScript bun 脚本工具

### MCP 工具使用

- MCP 工具在配置时需要添加 `mcp-` 前缀
- 例如：名为 `filesystem` 的 MCP 服务，在工具列表中写作 `mcp-filesystem`
- 支持三种连接方式：
  - **HTTP**：连接远程 HTTP MCP 服务
  - **SSE**：通过 Server-Sent Events 连接
  - **Stdio**：启动本地 MCP 进程并通过标准输入输出通信

### 自定义智能体配置要点

- `name`：智能体名称，用于标识
- `desc`：智能体描述，帮助用户了解其能力
- `system_prompt`：系统提示词，定义智能体的行为和能力
- `tools`：工具列表，可包含内置工具和 MCP 工具
- `base_url`、`api_key`、`model_name`：AI 模型配置
- `provider`：模型提供者类型（可选），支持 `openai`、`deepseek`、`claude`、`ollama`、`ark`、`gemini`、`qwen`、`openrouter`，不设置时根据 `base_url` 和 `model_name` 自动检测
