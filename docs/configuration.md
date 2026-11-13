# 配置指南

推荐先用 `fkteams login` 配置模型。需要手动编辑时，配置文件位于 `~/.fkteams/config/config.toml`，可生成完整示例：

```bash
fkteams generate config
```

也可在 Web“系统配置”中编辑，点击“确认并保存”后生效。任务运行期间保存会返回冲突，请在任务结束后重试。监听地址、后台服务启停和通道连接配置变更后需重启应用。

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
supports_vision = true

[[models]]
id = "fast"
name = "快速模型"
use_for = ["title", "summary"]
provider = "deepseek"
base_url = "https://api.deepseek.com/v1"
api_key = "your_deepseek_key"
model = "deepseek-chat"
supports_vision = false
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
| `supports_vision` | 视觉输入能力：`true` 允许发送图片，`false` 在请求前拒绝图片；省略时沿用供应商适配器行为 |
| `extra_headers` | 额外 HTTP 请求头，格式为 `Key:Value,Other:Value` |

`use_for` 不能在多个模型中重复配置。必须有一个模型包含 `chat`；`agent`、`title`、`summary` 未配置时会回退到 `chat` 模型。

视觉能力应按上游模型实际支持情况设置，详见[多模态支持](multimodal.md#视觉输入配置)。

## 服务与认证

```toml
[server]
host = "127.0.0.1"
port = 23456
log_level = "info"
allow_origins = ["http://localhost:5173", "http://127.0.0.1:5173"]
trusted_proxies = []

[server.auth]
enabled = false
username = "admin"
password = "your_password"
secret = "your_jwt_secret"
```

认证配置支持热更新。启用认证或修改用户名、密码、Secret 后，已登录的 Web 页面会要求原地重新登录；关闭认证会立即停止校验。重新登录不会取消后台任务，任务输出会在认证恢复后继续同步。

`allow_origins` 按协议、主机和有效端口精确匹配，必须填写完整 Origin（如 `https://app.example.com`），不能只写主机名。`*` 允许无凭据跨域访问，但不会启用跨域凭据。

`trusted_proxies` 默认留空，此时服务端忽略 `X-Forwarded-For` 等代理来源头，防止客户端伪造 IP 绕过认证限流。通过 Nginx、Caddy 等反向代理部署时，只填写实际代理的 IP 或 CIDR（如 `127.0.0.1`、`10.0.0.0/8`），修改后需重启服务。不要使用 `0.0.0.0/0` 或 `::/0`。

## OpenAI 兼容 API

```toml
[openai_api]
api_keys = ["sk-fkteams-your-secret-key"]
```

客户端使用 `http://<host>:<port>/v1` 作为 Base URL，`model` 字段填写本地模型 `id`。

## 智能体与圆桌

`[[agents.items]]` 是全局智能体目录。使用内置 ID 可覆盖该角色的名称、提示词、模型和工具；使用新 ID 则注册自定义角色，可在聊天 `@`、CLI 和团队模式中调用。

- [自定义智能体](custom-agents.md)：配置示例、字段和工具选择，也适用于覆盖内置角色。
- [圆桌讨论](roundtable.md)：成员顺序、模型和讨论轮次。
- [JavaScript 扩展](javascript.md)：自定义工具与流程 Hooks。

需要 SSH 的角色在自身条目配置连接：

```toml
[[agents.items]]
id = "remote-prod"
name = "生产服务器"
tools = ["ssh"]
ssh = { host = "ip:port", username = "your_ssh_user", password = "your_ssh_password", known_hosts_file = "~/.ssh/known_hosts" }
enabled = true
```

SSH 校验服务器主机密钥，默认读取 `~/.ssh/known_hosts`。连接前通过可信渠道确认目标密钥；不匹配时拒绝连接。上传、下载的本地路径仅限工作区，单文件上限 1 GiB；下载完成前不会覆盖已有目标文件。

## 深度模式

`fkteams -m deep` 默认启用规划、工作区、命令、成员委派、摘要和项目规则，最多 20 次迭代，内建 shell 超时 30 秒。`[deep]` 可配置 instruction、max_iterations、extra_tools，以及 planning、workspace、shell、delegation、context 等分项。请从生成的完整示例修改；只填写部分分项时，未写的布尔开关可能保持关闭。

## 工具权限审批

Web 对话默认会在危险命令、外部文件访问、Git 写操作和任务分发前弹出审批。可以在设置页的“权限”页签配置，也可以手动编辑：

```toml
[tools.approval]
auto_approve = []
```

`auto_approve` 可选值为 `command`、`file`、`git`、`dispatch`。设置为 `["all"]` 时 Web 对话不再弹出工具审批框。CLI 使用 `--approve`；消息通道的自动审批和后台默认拒绝行为见[安全说明](security.md#工具权限)。

## 外部集成

- [MCP 服务](mcp.md)：在 `[[tools.mcp_servers]]` 配置 HTTP、SSE 或 stdio 服务，智能体通过 `mcp-<server_id>` 引用工具。
- [消息通道](channels.md)：在 `[channels.*]` 配置 Discord、QQ 或微信；单智能体绑定使用 `mode = "agent"` 和 `agent_id`。

## 长期记忆

```toml
[memory]
enabled = true
```

开关控制模型读写和上下文注入，关闭后仍可在 Web 手动管理已有记忆。行为与容量见[长期记忆](memory.md)。

## 数据目录与环境变量

默认应用目录为 `~/.fkteams`，可通过 `FEIKONG_APP_DIR` 覆盖。配置保存在 `config/config.toml`，业务数据主要保存在 `data/fkteams.db`，文件工作区为 `workspace/`。附件、报告和缓存仍使用文件目录；备份与升级见[存储说明](storage.md)。

| 变量名 | 说明 | 默认值 |
| ------ | ---- | ------ |
| `FEIKONG_APP_DIR` | 应用数据目录 | `~/.fkteams` |
| `FEIKONG_PROXY_URL` | 代理地址 | - |
| `FEIKONG_MAX_ITERATIONS` | 智能体最大迭代次数，`0` 或 `-1` 表示不限制 | `60` |
