# 配置指南

配置文件位于 `~/.fkteams/config/config.toml`，可通过下面命令生成示例：

```bash
fkteams generate config
```

## 模型池

`[[models]]` 是全局模型池。其他配置只通过 `id` 引用模型，不直接重复填写 `base_url`、`api_key` 等连接参数。

```toml
[[models]]
id = "main"
name = "主力模型"
use_for = ["chat", "agent"]
provider = "openai"
base_url = "https://api.openai.com/v1"
api_key = "your_api_key"
model = "gpt-5"

[[models]]
id = "fast"
name = "快速模型"
use_for = ["title", "summary"]
provider = "deepseek"
base_url = "https://api.deepseek.com/v1"
api_key = "your_deepseek_key"
model = "deepseek-chat"
```

字段说明：

| 字段 | 说明 |
| ---- | ---- |
| `id` | 稳定引用 ID，智能体、圆桌和 API 都用它引用模型 |
| `name` | 展示名称，可随时调整 |
| `use_for` | 默认用途，可选 `chat`、`agent`、`title`、`summary` |
| `provider` | 模型提供商，如 `openai`、`deepseek`、`claude`、`ollama`、`ark`、`gemini`、`qwen`、`openrouter`、`copilot` |
| `base_url` | 模型服务地址 |
| `api_key` | 模型服务密钥 |
| `model` | 上游真实模型名 |
| `extra_headers` | 额外 HTTP 请求头，格式为 `Key:Value,Other:Value` |

`use_for` 不能在多个模型中重复配置。必须有一个模型包含 `chat`；`agent`、`title`、`summary` 未配置时会回退到 `chat` 模型。

## 服务与认证

```toml
[server]
host = "127.0.0.1"
port = 23456
log_level = "info"
allow_origins = ["http://localhost:5173", "http://127.0.0.1:5173"]

[server.auth]
enabled = false
username = "admin"
password = "your_password"
secret = "your_jwt_secret"
```

认证配置支持热更新。启用认证或修改用户名、密码、Secret 后，已登录的 Web 页面会要求原地重新登录；关闭认证会立即停止校验。重新登录不会取消后台任务，任务输出会在认证恢复后继续同步。

## OpenAI 兼容 API

```toml
[openai_api]
api_keys = ["sk-fkteams-your-secret-key"]
```

客户端使用 `http://<host>:<port>/v1` 作为 Base URL，`model` 字段填写本地模型 `id`。

## 内置智能体

```toml
[[agents.items]]
id = "researcher"
name = "研究员"
description = "网络研究员，负责检索、抓取、交叉验证和整理时效信息。"
prompt = "" # 留空时使用后端返回的内置提示词；在 Web 配置页保存后会写入完整提示词
tools = ["search", "fetch"]
enabled = true

[[agents.items]]
id = "frontend"
name = "前端开发专家"
description = "专注于前端开发的智能体"
prompt = "你是一个专业的前端开发工程师。"
model_id = "main"
tools = ["file", "command", "search"]
enabled = true

[[agents.items]]
id = "remote-prod"
name = "生产服务器"
description = "通过 SSH 管理生产服务器"
prompt = ""
model_id = "main"
tools = ["ssh"]
ssh = { host = "ip:port", username = "your_ssh_user", password = "your_ssh_password" }
enabled = true
```

`agents.items` 是全局智能体目录。内置智能体使用稳定 `id` 覆盖默认名称、描述、提示词、模型和工具；非内置 `id` 会作为用户新增智能体注册，可在聊天 `@`、`agent` 子命令和团队模式中使用。需要 SSH 能力的智能体在自身条目中配置 `ssh`，因此可以同时配置多个面向不同服务器的远程智能体。

字段说明：

| 字段 | 说明 |
| ---- | ---- |
| `id` | 稳定智能体 ID，用于 `@` 引用、命令参数和工具标识 |
| `name` | 展示名称 |
| `description` | 能力描述 |
| `prompt` | 系统提示词 |
| `model_id` | 引用 `[[models]].id` |
| `tools` | 可用工具列表，可包含内置工具和 `mcp-<server_id>` |

## 圆桌讨论

圆桌成员顺序由 `[[roundtable.members]]` 数组顺序决定，不再单独配置序号。

```toml
[roundtable]
max_iterations = 2

[[roundtable.members]]
id = "logic"
name = "逻辑分析"
description = "擅长结构化推理和反驳"
model_id = "main"
prompt = ""

[[roundtable.members]]
id = "creative"
name = "创意视角"
description = "擅长发散思考和补充方案"
model_id = "fast"
prompt = "你是一个擅长发散思考的圆桌讨论者。请优先补充不同视角和替代方案。"
```

`prompt` 是可选字段，留空时使用内置圆桌讨论者提示词。

## 工具权限审批

Web 对话默认会在危险命令、外部文件访问、Git 写操作和任务分发前弹出审批。可以在设置页的“权限”页签配置，也可以手动编辑：

```toml
[tools.approval]
auto_approve = []
```

`auto_approve` 可选值为 `command`、`file`、`git`、`dispatch`。设置为 `["all"]` 时 Web 对话不再弹出工具审批框。

## MCP 服务

```toml
[[tools.mcp_servers]]
id = "filesystem"
name = "文件系统"
description = "文件系统操作工具"
enabled = true
timeout = "30s"
url = "http://127.0.0.1:3000/mcp"
transport = "http"

[[tools.mcp_servers]]
id = "postgres"
name = "PostgreSQL"
description = "数据库查询工具"
enabled = true
timeout = "30s"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
env = { DATABASE_URL = "postgresql://localhost/mydb" }
transport = "stdio"
```

MCP 工具名为 `mcp-<server_id>`，例如上面的 `filesystem` 服务在智能体 `tools` 中写作 `mcp-filesystem`。

## 消息通道

通道 `mode` 只表示运行模式。需要绑定单个智能体时使用 `mode = "agent"` 和 `agent_id`。

```toml
[channels.qq]
enabled = false
app_id = "your_app_id"
app_secret = "your_app_secret"
sandbox = true
mode = "team"
agent_id = ""

[channels.discord]
enabled = false
token = "your_discord_bot_token"
allow_from = ""
mode = "team"
agent_id = ""

[channels.weixin]
enabled = false
base_url = "https://ilinkai.weixin.qq.com"
cred_path = "channels/weixin/credentials.json"
log_level = "info"
allow_from = ""
mode = "team"
agent_id = ""
```

## 数据目录与环境变量

默认应用目录为 `~/.fkteams`，可通过 `FEIKONG_APP_DIR` 覆盖。常用子目录包括 `workspace`、`sessions`、`scheduler`、`history`、`config`、`log`、`share` 和 `runtime`。

| 变量名 | 说明 | 默认值 |
| ------ | ---- | ------ |
| `FEIKONG_APP_DIR` | 应用数据目录 | `~/.fkteams` |
| `FEIKONG_PROXY_URL` | 代理地址 | - |
| `FEIKONG_MAX_ITERATIONS` | 智能体最大迭代次数，`0` 或 `-1` 表示不限制 | `60` |
