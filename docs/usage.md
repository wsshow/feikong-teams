# 使用指南

## 运行模式

### Web 界面模式（推荐）

启动 Web 服务器，通过浏览器访问：

```bash
fkteams web
```

打开 <http://localhost:23456>，选择工作模式后输入任务。侧栏可新建、搜索、重命名、导出和删除会话。

任务运行中继续发送的内容默认作为续问排队；队列面板可将其转为转向，也可编辑、删除或调整顺序。转向在下一次模型调用前生效，续问在当前回合结束后执行。登录认证见[配置指南](configuration.md#服务与认证)。

#### 常驻助手

侧栏“助手”会恢复同一个常驻会话，可读取工作区、检索历史、记忆偏好和跟进任务。可以直接说“以后叫你小空”为它改名。使用方式见[个人助手](personal-assistant.md)，表单、图表和输入框命令见[助手交互界面](assistant-ui.md)。

### 纯 API 服务模式

启动不带 Web 界面的纯 API 服务，适合作为后端接口独立部署：

```bash
fkteams serve

# 指定监听地址和端口
fkteams serve --host 0.0.0.0 --port 8080
```

提供与 Web 模式相同的 API 接口和 WebSocket 服务，但不包含前端页面。详细接口文档请参考 [API 文档](./api/README.md)。

### 命令行模式

适合开发者和高级用户：

```bash
# 默认启动团队模式
fkteams

# 启动深度分析模式
fkteams -m deep

# 启动多智能体讨论模式
fkteams -m group
```

源码构建和开发运行见[部署指南](deployment.md#从源码构建)。

## 模型登录与管理

推荐通过交互式向导配置模型：

```bash
fkteams login
```

也可以直接指定服务商：

```bash
fkteams login openai
fkteams login deepseek
fkteams login copilot
```

GitHub Copilot 用户需要有效订阅，可以使用设备码登录，也可以导入 VS Code 已保存的 token：

```bash
fkteams login copilot --import
```

常用管理命令：

```bash
fkteams model ls
fkteams model lr --name main       # 查询该配置的上游模型
fkteams model sw --name main       # 切换默认对话模型
fkteams model rm --name old-model
fkteams logout openai
```

如果需要直接编辑 TOML、配置自定义 OpenAI 兼容服务或指定不同用途的默认模型，请参考[配置指南](./configuration.md)。

## 工作区项目约定

在应用工作区中放置 `AGENTS.md`，可以为所有智能体提供统一的项目说明和开发约定；系统也兼容 `Agents.md`。

默认文件位置为 `~/.fkteams/workspace/AGENTS.md`。通过 `FEIKONG_APP_DIR` 修改应用目录后，文件位置相应变为 `{FEIKONG_APP_DIR}/workspace/AGENTS.md`。

规则文件支持通过 `@import` 拆分。内容会在每次模型调用前临时注入，不会写入会话历史，因此适合保存项目架构、代码规范和验证命令等长期上下文。

## 终端交互

启动 `fkteams` 后直接输入任务，或使用以下命令。

### 常用命令

| 命令 | 说明 |
| ------------------------------- | ----------------------------------------------------- |
| `quit` / `q` | 退出程序 |
| `help` | 显示帮助信息 |
| `list_agents` | 列出所有可用的智能体 |
| `@智能体名 [查询内容]` | 切换到指定智能体并可选执行查询 |
| `switch_work_mode` | 切换工作模式（团队模式/深度模式/圆桌讨论模式） |
| `save_chat_history` | 保存聊天历史到当前会话文件 |
| `list_chat_history` | 列出所有可用的聊天历史会话 |
| `load_chat_history` | 选择并加载聊天历史会话 |
| `clear_chat_history` | 清空当前聊天历史 |
| `save_chat_history_to_markdown` | 导出聊天历史为 Markdown 文件 |
| `save_chat_history_to_html` | 导出聊天历史为 HTML 文件 |
| `list_schedule` | 列出所有定时任务 |
| `cancel_schedule` | 选择并取消定时任务 |
| `delete_schedule` | 选择并删除定时任务 |
| `list_memory` | 列出所有长期记忆条目 |
| `delete_memory` | 选择并删除记忆条目 |
| `clear_memory` | 清空所有长期记忆 |

任务运行中直接输入内容并按 Enter 会加入转向队列，并立即显示在终端消息流里；下一次模型调用前会合并消费当前所有未执行转向。运行中按 `Esc` 会暂停当前任务，并将尚未执行的转向消息回填到输入框，便于继续修改后重新提交。

## 智能体切换

从交互模式下，你可以直接与单个智能体对话，无需启动整个团队：

```
# 列出所有可用的智能体
list_agents

# 切换到 analyst（数据分析师）
@analyst

# 切换并直接提问
@coder 帮我创建一个 Python 脚本

# 切换到 researcher 并搜索信息
@researcher 查找最新的 Go 语言教程

# 向非空小助手咨询项目配置
@fkteams_helper 如何配置一个 stdio MCP 服务？
```

**可用智能体列表**：

内置智能体和用户新增智能体统一由 `[[agents.items]]` 管理：

- `@coordinator` - 协调者，直接处理常规任务并按需调度成员
- `@coder` - 软件工程师，代码实现、调试、重构和验证
- `@researcher` - 网络研究员，擅长检索、抓取和交叉验证时效信息
- `@fkteams_helper` - 非空小助手，解答 fkteams 安装、配置、使用、扩展和故障排查问题
- `@analyst` - 数据分析师，擅长使用 Excel、Python 脚本和文档处理工具
- `@remote` - 远程运维专家，擅长通过 SSH 连接远程服务器
- `@generalist` - 通用执行助手，综合命令、文件、搜索等工具完成任务

配置页可以查看内置智能体完整提示词、工具和描述，并可新增全局智能体。

**提示**：输入 `@` 符号后会自动显示可用的智能体列表供选择。

## 定时任务

可通过对话安排一次性或周期任务，例如“5 分钟后检查构建结果”“每个工作日早上 9 点整理新闻”。时间、频率或内容不明确时，助手会先追问。也可在 Web 定时任务页创建、修改、取消或删除安排。

应用需要保持运行才能执行调度。任务定义与状态保存在 SQLite，报告位于应用目录的 `scheduler/results/`。Web 会通知未读结果；CLI 使用 `list_schedule` 查看任务。重启后中断的执行标为失败，需要核对实际效果后再安排。通知与恢复行为见[个人助手](personal-assistant.md#后台安排与结果)，集成方式见[定时任务 API](api/schedule.md)。

## 命令行用法

```bash
# 默认交互模式
fkteams

# 直接查询模式
fkteams -q "你的问题"

# 管道输入模式
echo "解释一下 Go 的 context 包" | fkteams

# 管道传入文件内容
cat main.go | fkteams -q "审查这段代码"

# 管道与 agent 子命令配合
cat error.log | fkteams agent -n coder -q "分析这个错误日志"

# 指定工作模式
fkteams -m deep

# 恢复历史会话（交互模式）
fkteams -r "<session-id>"

# 恢复历史会话并直接查询
fkteams -r "<session-id>" -q "继续上次的问题"
```

### 子命令

| 命令 | 说明 |
| ------------------ | ------------------------------------- |
| `web` | 启动 Web 服务器模式（推荐） |
| `serve` | 启动纯 API 服务（无 Web 界面） |
| `session list` | 列出所有可用的聊天历史会话 |
| `update` | 检查并更新到最新版本 |
| `init` | 初始化运行环境（安装/升级 uv 等依赖） |
| `generate config` | 生成示例配置文件 |
| `generate apikey` | 生成 OpenAI 兼容 API 密钥 |
| `agent` | 指定单个 Agent 执行任务 |
| `agent list` | 列出所有可用的 Agent |
| `login <provider>` | 登录模型服务商（openai, deepseek 等） |
| `auth` | 管理 Web 登录认证，见 `fkteams auth --help` |
| `logout` | 退出登录 |
| `model ls` | 列出已配置的模型 |
| `model lr` | 查询服务商的可用模型 |
| `model sw` | 切换默认对话模型 |
| `model rm` | 移除指定的模型配置 |
| `tool list` | 列出所有可用的工具 |
| `skill list` | 列出本地已安装的技能 |
| `skill search` | 搜索技能市场 |
| `skill install` | 安装技能 |
| `skill remove` | 移除本地技能 |

### 全局参数

| 参数 | 简写 | 说明 |
| ----------- | ---- | ------------------------------------------------------------------------------ |
| `--mode` | `-m` | 工作模式: `team`（团队）、`deep`（深度）或 `group`（讨论） |
| `--query` | `-q` | 直接查询模式，执行完查询后退出 |
| `--resume` | `-r` | 恢复指定的聊天历史会话，可与 `-q` 组合使用 |
| `--temporary` | `--temp` | 开启临时会话，不保存聊天历史且不显示恢复命令 |
| `--approve` |  | 自动批准指定操作类别（`all`/`command`/`file`/`git`/`dispatch`，逗号分隔） |
| `--version` | `-v` | 显示版本信息 |

### 管道输入

管道有内容时自动进入非交互查询模式。同时提供 `-q` 时，查询内容为 `-q` 文本、换行和管道内容；空管道且没有 `-q` 时返回错误。

### agent 子命令参数

| 参数 | 简写 | 说明 |
| ----------- | ---- | ------------------------------------------------------------------- |
| `list` |  | 列出所有可用的 Agent |
| `--name` | `-n` | Agent 名称（必填，与 list 互斥） |
| `--query` | `-q` | 直接查询模式，执行完查询后退出 |
| `--temporary` | `--temp` | 开启临时会话，不保存聊天历史且不显示恢复命令 |
| `--format` |  | 输出格式: `default`（格式化）或 `json`（原始JSON） |
| `--approve` |  | 自动批准指定操作类别（`all`/`command`/`file`/`git`/`dispatch`，逗号分隔） |

会话默认保存，可通过全局 `--resume` 恢复；如需额外导出 HTML，请在交互模式内执行 `save_chat_history_to_html`。

```bash
fkteams agent -n coder                         # 指定角色交互
fkteams agent -n researcher -q "检索资料" --format json
fkteams agent -n researcher -q "临时问答" --temporary
```
