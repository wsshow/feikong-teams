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
| `FEIKONG_PROXY_URL`      | 代理地址（唯一的代理配置方式）    | -            |
| `FEIKONG_MAX_ITERATIONS` | 智能体最大迭代次数（0/-1 不限制） | `60`         |

## 配置圆桌会议成员

在 `~/.fkteams/config/config.toml` 中配置圆桌会议成员、MCP 服务和自定义智能体。

> **注意**：圆桌会议成员和自定义智能体的模型配置均通过 `model` 字段引用 `[[models]]` 中已定义的模型名称，不直接配置 `base_url`、`api_key` 等参数。

```toml
# 先定义模型池
[[models]]
name = "default"
provider = "openai"
base_url = "https://api.openai.com/v1"
api_key = "your_openai_api_key"
model = "gpt-4"

[[models]]
name = "deepseek"
provider = "deepseek"
base_url = "https://api.deepseek.com/v1"
api_key = "your_deepseek_api_key"
model = "deepseek-chat"

[[models]]
name = "claude"
provider = "claude"
base_url = "https://api.anthropic.com"
api_key = "your_claude_api_key"
model = "claude-3-sonnet"

[server]
port = 23456        # Web服务器端口
log_level = "info"  # 日志级别

# 圆桌会议配置
[roundtable]
max_iterations = 2  # 讨论轮数

[[roundtable.members]]
index = 0
name = "深度求索"
desc = "深度求索聊天模型，擅长逻辑分析"
model = "deepseek"  # 引用 [[models]] 中 name = "deepseek" 的模型

[[roundtable.members]]
index = 1
name = "克劳德"
desc = "克劳德聊天模型，擅长创意思维"
model = "claude"    # 引用 [[models]] 中 name = "claude" 的模型

# 自定义主持人（可选，不配置则使用内置主持人）
[custom.moderator]
name = "主持人"
desc = "负责协调讨论的主持人"
model = "default"
system_prompt = "你是一个公正的主持人，负责根据任务需求合理分配给团队成员。"

# 自定义智能体配置
[[custom.agents]]
name = "数据分析师"
desc = "专业的数据分析智能体"
model = "default"  # 引用 [[models]] 中的模型名称
system_prompt = """你是一个专业的数据分析师，擅长数据处理和可视化。
你需要：
1. 分析用户提供的数据
2. 使用合适的工具进行数据处理
3. 生成可视化图表
4. 给出专业的分析建议
"""
tools = ["file", "command", "uv", "mcp-filesystem"]  # 可使用内置工具和MCP工具

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

| 名称        | 说明                                 |
| ----------- | ------------------------------------ |
| `file`      | 文件读写操作（沙箱隔离在 workspace） |
| `git`       | Git 仓库操作                         |
| `excel`     | Excel 文件处理                       |
| `doc`       | 文档处理工具                         |
| `command`   | 命令执行（带安全审批）               |
| `ssh`       | SSH 远程连接                         |
| `search`    | 网络搜索（DuckDuckGo）               |
| `fetch`     | 网页抓取工具                         |
| `todo`      | 待办事项管理                         |
| `scheduler` | 定时任务调度                         |
| `uv`        | Python uv 脚本工具                   |
| `bun`       | JavaScript bun 脚本工具              |
| `ask`       | 向用户提问工具                       |
| `mcp-*`     | MCP 协议工具（`mcp-` + 服务名称）    |

### MCP 工具使用

- MCP 工具在配置时需要添加 `mcp-` 前缀
- 例如：名为 `filesystem` 的 MCP 服务，在工具列表中写作 `mcp-filesystem`
- 支持三种连接方式：
  - **HTTP**：连接远程 HTTP MCP 服务
  - **SSE**：通过 Server-Sent Events 连接
  - **Stdio**：启动本地 MCP 进程并通过标准输入输出通信

### 自定义智能体配置要点

- `name`：智能体名称，用于 `@` 引用和显示标识
- `desc`：智能体描述，帮助用户和主持人了解其能力
- `system_prompt`：系统提示词，定义智能体的行为和能力
- `model`：引用 `[[models]]` 中的模型名称（不设置时使用 `name = "default"` 的模型）
- `tools`：工具列表，可包含内置工具和 MCP 工具

> **提示**：自定义智能体配置后会自动注册到全局智能体注册表，在任意工作模式下都可以通过 `@智能体名` 或 `fkteams agent -n 智能体名` 使用。详细说明请参考 [自定义智能体指南](./custom-agents.md)。
