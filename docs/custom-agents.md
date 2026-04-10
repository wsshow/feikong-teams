# 自定义智能体使用指南

## 概述

自定义智能体通过配置文件 `~/.fkteams/config/config.toml` 中的 `[[custom.agents]]` 定义。配置后，自定义智能体会自动注册到全局智能体注册表中，有以下三种使用方式：

1. **任意模式下通过 `@` 直接对话**：在 CLI 交互中输入 `@智能体名` 即可切换到该智能体
2. **通过 `agent` 子命令指定运行**：`fkteams agent -n 智能体名` 直接使用
3. **在自定义会议模式中协作**：`fkteams -m custom` 启动自定义模式，由主持人协调自定义智能体协作

## 工作模式说明

自定义智能体与各工作模式的关系：

- **团队模式** (`team`)：使用内置智能体（小码、小令、小说、小简始终可用；小搜、小析、小访、小助需在配置文件中启用）。自定义智能体同样加入注册表，可通过 `@` 或 `agent` 子命令单独使用
- **深度模式** (`deep`)：使用深度探索协调者，能深入分析问题并协调多个智能体解决复杂任务。自定义智能体同样可通过 `@` 单独使用
- **讨论模式** (`group`)：使用 `[[roundtable.members]]` 配置的圆桌会议成员。自定义智能体可通过 `@` 单独使用
- **自定义模式** (`custom`)：使用 `[custom.moderator]` 主持人 + `[[custom.agents]]` 自定义智能体组成 Supervisor 协作

## 使用自定义智能体

### 方式一：`@` 直接对话（任意模式下可用）

在 CLI 交互模式中，输入 `@` 符号后自动提示可用的智能体列表（包含内置和自定义智能体），选择后即切换到该智能体：

```bash
# 先启动任意模式
fkteams

# 交互中使用
@前端开发专家 帮我创建一个 React 项目
@前端开发专家                 # 仅切换，不立即提问
```

### 方式二：`agent` 子命令

```bash
# 直接查询
fkteams agent -n 前端开发专家 -q "帮我创建一个 React 项目"

# 交互模式
fkteams agent -n 前端开发专家

# JSON 格式输出
fkteams agent -n 前端开发专家 -q "问题" --format json

# 管道输入
cat src/App.tsx | fkteams agent -n 前端开发专家 -q "审查这段代码"

# 查看所有可用 Agent（包含自定义智能体）
fkteams agent list
```

### 方式三：自定义会议模式

当需要多个自定义智能体协作解决复杂问题时，使用自定义模式：

```bash
# 命令行模式
fkteams -m custom

# 直接查询模式
fkteams -m custom -q "你的问题"

# Web 模式（在侧边栏选择自定义模式）
fkteams web
```

自定义模式下，主持人智能体会根据任务需求协调调度各自定义智能体协作。

## 创建自定义智能体

编辑配置文件 `~/.fkteams/config/config.toml`：

```toml
# 先定义模型（如果自定义智能体需要使用不同于 default 的模型）
[[models]]
name = "default"
provider = "openai"
base_url = "https://api.openai.com/v1"
api_key = "your_api_key"
model = "gpt-4"

[[models]]
name = "deepseek"
provider = "deepseek"
base_url = "https://api.deepseek.com/v1"
api_key = "your_deepseek_key"
model = "deepseek-chat"

# 自定义智能体通过 model 字段引用上面定义的模型名称
[[custom.agents]]
name = "前端开发专家"
desc = "专注于前端开发的智能体"
model = "default"  # 引用 [[models]] 中 name = "default" 的模型
system_prompt = """你是一个专业的前端开发工程师。
你擅长：
- React、Vue、Angular 等现代前端框架
- HTML、CSS、JavaScript/TypeScript
- 响应式设计和移动端适配
- 性能优化和最佳实践

你需要：
1. 理解用户的前端开发需求
2. 使用合适的工具创建和修改代码
3. 确保代码质量和最佳实践
4. 提供清晰的技术建议
"""
tools = ["command", "file", "search"]
```

## 配置参数说明

### 自定义智能体 `[[custom.agents]]`

| 参数            | 说明                                             | 必填 |
| --------------- | ------------------------------------------------ | ---- |
| `name`          | 智能体名称（用于 `@` 引用和显示）                | ✓    |
| `desc`          | 智能体描述（帮助用户和主持人了解其能力）         | ✓    |
| `system_prompt` | 系统提示词，定义智能体的行为和能力               | ✓    |
| `model`         | 引用 `[[models]]` 中的模型名称（默认 `default`） | ✗    |
| `tools`         | 工具列表（内置工具和 MCP 工具）                  | ✗    |

> **注意**：模型配置采用引用机制。自定义智能体不直接配置 `base_url`、`api_key` 等参数，而是通过 `model` 字段引用 `[[models]]` 中定义的具名模型。不设置 `model` 时使用 `name = "default"` 的模型。

### 自定义主持人 `[custom.moderator]`

自定义模式下可配置自定义主持人（可选，不配置时使用内置主持人）：

```toml
[custom.moderator]
name = "项目经理"
desc = "负责协调团队成员完成项目任务"
model = "default"
system_prompt = "你是一个经验丰富的项目经理，负责根据任务需求合理分配给团队成员。"
tools = []
```

| 参数            | 说明                           | 必填 |
| --------------- | ------------------------------ | ---- |
| `name`          | 主持人名称                     | ✓    |
| `desc`          | 主持人描述                     | ✓    |
| `system_prompt` | 系统提示词                     | ✓    |
| `model`         | 引用 `[[models]]` 中的模型名称 | ✗    |
| `tools`         | 工具列表                       | ✗    |

### 可用的内置工具

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

## 系统提示词编写技巧

1. **明确角色定位**：清楚说明智能体的专业领域
2. **定义能力范围**：列出智能体擅长的具体技能
3. **设定工作流程**：指导智能体如何处理任务
4. **强调约束条件**：说明需要遵守的规则和限制

## 工具配置最佳实践

1. **按需选择**：只配置智能体真正需要的工具
2. **组合使用**：合理搭配内置工具和 MCP 工具
3. **权限控制**：注意工具的安全性和访问权限

## 使用场景示例

**场景 1：代码审查助手**

```toml
[[custom.agents]]
name = "代码审查专家"
desc = "专业的代码审查和质量分析"
model = "default"
system_prompt = """你是一个严格的代码审查专家...
重点关注：代码质量、安全漏洞、性能问题、最佳实践"""
tools = ["command", "file", "mcp-github"]
```

**场景 2：DevOps 助手**

```toml
[[custom.agents]]
name = "DevOps 工程师"
desc = "自动化运维和部署专家"
model = "default"
system_prompt = """你是一个经验丰富的 DevOps 工程师...
擅长：CI/CD、容器化、监控告警、自动化脚本"""
tools = ["command", "mcp-github"]
```

**场景 3：数据分析师**

```toml
[[custom.agents]]
name = "数据分析师"
desc = "数据处理和可视化专家"
model = "deepseek"
system_prompt = """你是一个数据分析专家...
能力：数据清洗、统计分析、数据可视化、报告生成"""
tools = ["command", "uv", "mcp-postgres", "mcp-filesystem"]
```

**场景 4：完整的自定义会议团队**

```toml
# 自定义主持人（可选）
[custom.moderator]
name = "技术总监"
desc = "负责协调技术团队完成复杂项目"
model = "default"
system_prompt = "你是一个技术总监，根据任务类型将工作分配给最合适的团队成员。"

# 自定义智能体成员
[[custom.agents]]
name = "后端工程师"
desc = "专注于后端 API 和服务开发"
model = "default"
system_prompt = "你是一个后端工程师，擅长 Go、Python 等后端开发..."
tools = ["command", "file"]

[[custom.agents]]
name = "前端工程师"
desc = "专注于前端界面开发"
model = "default"
system_prompt = "你是一个前端工程师，擅长 React、TypeScript 等前端开发..."
tools = ["command", "file"]

[[custom.agents]]
name = "测试工程师"
desc = "专注于质量保证和测试"
model = "deepseek"
system_prompt = "你是一个测试工程师，擅长编写测试用例和自动化测试..."
tools = ["command", "file"]
```
