# 配置指南

## 配置环境变量

环境变量支持两种方式配置：

- **Shell 环境变量**（推荐）：在 `~/.bashrc`、`~/.zshrc` 等 Shell 配置文件中 `export` 相关变量，对所有终端会话生效。
- **`.env` 文件**：在运行 `fkteams` 的目录下创建 `.env` 文件，程序启动时自动加载。可运行 `fkteams generate env` 生成示例文件。

必要的配置项：

```env
# 模型配置
FEIKONG_API_KEY=your_api_key_here
FEIKONG_BASE_URL=https://api.openai.com/v1
FEIKONG_MODEL=gpt-5

# 模型提供者类型（可选，自动检测）: openai, deepseek, claude, ollama, ark, gemini, qwen, openrouter
# FEIKONG_PROVIDER=openai

# 额外 HTTP 请求头（用于网关认证等，格式: Key1:Value1,Key2:Value2）
# FEIKONG_EXTRA_HEADERS=X-Custom-Auth:your-token,X-Gateway-Key:your-key

# 网络搜索工具配置（可选）
FEIKONG_PROXY_URL=http://127.0.0.1:7890

# 应用数据目录，默认为: ~/.fkteams/
# 所有数据（配置、日志、会话、历史记录等）均存储于此目录下
# FEIKONG_APP_DIR = ~/.fkteams

# 工作目录配置, 默认为: ~/.fkteams/workspace
# 可单独指定工作目录，覆盖 FEIKONG_APP_DIR 中的 workspace 子目录
# FEIKONG_WORKSPACE_DIR = /custom/workspace/path

# 代码助手
FEIKONG_CODER_ENABLED = true

# 本地命令行助手
FEIKONG_CMDER_ENABLED = true

# 数据分析师
FEIKONG_ANALYST_ENABLED = false

# 个人全能助手（带审批以及子任务功能）
FEIKONG_ASSISTANT_ENABLED = true

# 全局长期记忆
FEIKONG_MEMORY_ENABLED = true

# SSH 访问者智能体配置（可选）
FEIKONG_SSH_VISITOR_ENABLED=false # 设置为 true 启用小访智能体
FEIKONG_SSH_HOST=ip:port
FEIKONG_SSH_USERNAME=your_ssh_user
FEIKONG_SSH_PASSWORD=your_ssh_password

# Web 页面登录认证（可选，设置 ENABLED=true 后启用）
FEIKONG_LOGIN_ENABLED=false
FEIKONG_LOGIN_SECRET=your_random_secret_key
FEIKONG_LOGIN_USERNAME=admin
FEIKONG_LOGIN_PASSWORD=your_password
```

## 配置圆桌会议成员

生成示例配置文件：

```bash
fkteams generate config
```

编辑 `~/.fkteams/config/config.toml` 配置圆桌会议成员、MCP 服务和自定义智能体：

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
