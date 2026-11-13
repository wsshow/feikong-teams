# MCP 工具集成

在 `config/config.toml` 的 `[[tools.mcp_servers]]` 中配置外部 MCP 服务，再通过工具组名 `mcp-<id>` 提供给智能体。

## 配置示例

HTTP 服务：

```toml
[[tools.mcp_servers]]
id = "remote-tools"
name = "远程工具"
enabled = true
timeout = "30s"
transport = "http"
url = "http://127.0.0.1:3000/mcp"
```

本地 stdio 服务：

```toml
[[tools.mcp_servers]]
id = "local-tools"
name = "本地工具"
enabled = true
timeout = "30s"
transport = "stdio"
command = "node"
args = ["/absolute/path/to/mcp-server.js"]
env = { SERVICE_URL = "http://127.0.0.1:3000" }
```

示例地址和脚本路径需替换为实际服务；stdio 的运行时和服务程序需提前安装。

| 字段 | 说明 |
| --- | --- |
| `id` | 稳定服务标识，对应工具组 mcp-<id> |
| `name` / `description` | 展示名称与可选描述 |
| `enabled` | 是否启用 |
| `timeout` | 初始化超时，如 30s |
| `transport` | http、sse 或 stdio |
| `url` | HTTP/SSE 服务地址 |
| `command` / `args` | stdio 的启动命令与参数 |
| `env` | 仅传给当前服务的环境变量 |

## 在智能体中使用

```toml
[[agents.items]]
id = "data"
name = "数据助手"
description = "使用外部工具处理数据"
prompt = "根据任务调用已连接的工具，并检查结果。"
model_id = "main"
tools = ["file", "mcp-local-tools"]
enabled = true
```

`main` 需预先存在于[模型池](configuration.md#模型池)。Web 智能体编辑器也可直接选择 MCP 工具组。

只接入可信服务，检查其文件、网络和账户权限。内置文件工具的工作区限制不会自动约束外部 MCP 服务，详见[安全说明](security.md#工具权限)。
