# 自定义智能体使用指南

自定义智能体在 `~/.fkteams/config/config.toml` 的 `[[agents.items]]` 中定义。保存配置后会注册到全局智能体目录，可用于：

- 在聊天中通过 `@智能体ID` 指定单个智能体。
- 通过 `fkteams agent -n <智能体ID>` 单独运行。
- 在团队模式中由协调者按需调度。

## 配置示例

以下示例假定[模型池](configuration.md#模型池)中已配置 `id = "main"`。

```toml
[[agents.items]]
id = "frontend"
name = "前端开发专家"
description = "专注于前端开发的智能体"
model_id = "main"
prompt = """你是一个专业的前端开发工程师。
你擅长 React、TypeScript、CSS 和前端工程化。
"""
tools = ["command", "file", "search"]
enabled = true
```

## 字段说明

| 参数 | 说明 |
| ---- | ---- |
| `id` | 稳定智能体 ID，用于 `@` 引用、命令参数和工具标识 |
| `name` | 展示名称 |
| `description` | 能力描述 |
| `prompt` | 系统提示词 |
| `model_id` | 引用 `[[models]].id` |
| `tools` | 工具列表，可包含内置工具组、`javascript` 和 `mcp-<server_id>` |
| `enabled` | 是否启用该智能体 |

`id` 是引用标识，应保持稳定；`name` 只是展示名称，可以按需要调整。

## 运行方式

```bash
# 直接查询
fkteams agent -n frontend -q "帮我创建一个 React 项目"

# 交互模式
fkteams agent -n frontend
```

## 工具选择

常用内置工具：

| 名称 | 说明 |
| ---- | ---- |
| `file` | 文件读写 |
| `git` | Git 仓库操作 |
| `command` | 命令执行 |
| `search` | 网络搜索 |
| `fetch` | 网页抓取 |
| `ask` | 向用户提问 |
| `uv` | Python uv 脚本 |
| `bun` | JavaScript bun 脚本 |

MCP 工具通过 `mcp-<server_id>` 引用，例如 `id = "filesystem"` 的 MCP 服务对应 `mcp-filesystem`。

Web 配置页支持直接选择工具，不需要手动记忆全部工具名。
