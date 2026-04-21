---
name: fkteams
description: >
  fkteams 多智能体协作 AI 助手的完整使用指南。当用户需要启动 fkteams、切换工作模式（团队/深度/讨论/自定义）、
  通过命令行或管道执行查询、管理单个智能体（小码/小搜/小析/小令/小访/小说/小简/小助）、管理会话历史（保存/加载/导出/恢复）、
  管理模型配置（添加/切换/删除/登录服务商）、管理本地技能（列出/搜索/安装/移除）、初始化或修改配置文件，
  以及了解 fkteams 的任意命令行用法时，请使用此 skill。
compatibility: 需要 fkteams 二进制文件，首次使用前需运行 fkteams generate config 生成配置文件。
metadata:
  author: fkteams
  version: "1.0"
---

# fkteams 命令行完整指南

## 一、快速入门

```bash
# 第一步：生成配置文件
fkteams generate config

# 第二步：编辑配置，添加模型（必填）
# 编辑 ~/.fkteams/config/config.toml，填写 api_key 和 model

# 第三步（可选）：安装运行时依赖
fkteams init --all   # 安装 uv（Python）和 bun（JavaScript）

# 第四步：启动
fkteams web          # Web 界面（推荐）
fkteams              # CLI 交互模式
```

---

## 二、启动与运行模式

### 模式对照表

| 模式 | 命令 | 适用场景 |
|------|------|---------|
| Web 界面 | `fkteams web` | 日常使用，需要历史记录和可视化界面 |
| CLI 交互 | `fkteams` | 服务器/终端环境 |
| 直接查询 | `fkteams -q "..."` | 脚本集成、自动化 |
| 纯 API 服务 | `fkteams serve` | 作为后端独立部署 |

### Web 界面模式

```bash
fkteams web
# 启动后访问 http://localhost:23456
```

### CLI 交互模式

```bash
fkteams              # 默认：团队模式（统御协调多智能体）
fkteams -m deep      # 深度分析模式
fkteams -m group     # 多智能体讨论模式（圆桌）
fkteams -m custom    # 自定义会议模式
```

### 直接查询模式（非交互，执行后退出）

```bash
fkteams -q "帮我审查 main.go"
fkteams -m deep -q "深度分析这个架构"
fkteams -q "生成一份报告" --save    # 同时保存历史
```

### 管道输入模式

管道有内容时自动进入非交互模式。

```bash
echo "解释一下 Go 的 context 包" | fkteams
cat main.go | fkteams -q "审查以下代码:"
git diff HEAD~1 | fkteams -q "审查这次提交"
curl -s https://example.com/api | fkteams -q "解析这个 API 响应"
```

**规则**：同时提供 `-q` 时，查询内容为 `-q` 文本 + 换行 + 管道内容；管道为空且无 `-q` 时报错。

### 纯 API 服务模式

```bash
fkteams serve
fkteams serve --host 0.0.0.0 --port 8080
```

与 `web` 提供相同的 API，但无前端页面。支持端点：`GET /v1/models`、`POST /v1/chat/completions`。

### 恢复历史会话

```bash
fkteams -r "20260302_091249"              # 交互模式恢复
fkteams -r "20260302_091249" -q "继续上次的分析"  # 恢复后直接查询
```

### 全局参数

| 参数 | 简写 | 说明 |
|------|------|------|
| `--mode` | `-m` | 工作模式：`team`（默认）/ `deep` / `group` / `custom` |
| `--query` | `-q` | 直接查询模式，执行后退出 |
| `--save` | | 保存聊天历史（默认不保存） |
| `--resume` | `-r` | 恢复指定会话 ID |
| `--approve` | | 自动批准工具调用：`all` / `command` / `file` / `dispatch`（逗号分隔） |

---

## 三、交互模式内置命令

| 命令 | 说明 |
|------|------|
| `quit` / `q` | 退出程序 |
| `help` | 显示帮助 |
| `list_agents` | 列出所有可用智能体 |
| `@智能体名 [查询]` | 切换到指定智能体并可选执行查询 |
| `switch_work_mode` | 切换工作模式 |
| `save_chat_history` | 保存当前会话 |
| `list_chat_history` | 列出所有历史会话 |
| `load_chat_history` | 选择并加载历史会话 |
| `clear_chat_history` | 清空当前会话（不删除文件） |
| `save_chat_history_to_markdown` | 导出为 Markdown |
| `save_chat_history_to_html` | 导出为 HTML |
| `list_schedule` | 列出所有定时任务 |
| `cancel_schedule` | 取消定时任务 |
| `delete_schedule` | 删除定时任务 |
| `list_memory` | 列出长期记忆条目 |
| `delete_memory` | 删除记忆条目 |
| `clear_memory` | 清空所有长期记忆 |

---

## 四、单智能体模式（`agent` 子命令）

### 内置智能体（始终可用）

| 名称 | 角色 |
|------|------|
| `小码` | 资深软件工程师，代码实现、调试、重构 |
| `小令` | 命令行专家，根据 OS 执行合适的 shell 命令 |
| `小说` | 故事创作专家，创意写作、小说、叙事 |
| `小简` | 总结专家，将冗长内容提炼为精简要点 |

### 可选智能体（需在配置中启用）

| 名称 | 配置项 | 角色 |
|------|--------|------|
| `小搜` | `[agents] searcher = true` | DuckDuckGo 网络搜索 |
| `小析` | `[agents] analyst = true` | 数据分析（Excel、Python、文档） |
| `小访` | `[agents.ssh_visitor] enabled = true` | SSH 远程服务器访问 |
| `小助` | `[agents] assistant = true` | 全能助手，支持并行子任务分发 |

### agent 命令用法

```bash
# 列出所有可用智能体
fkteams agent list

# 交互模式（进入对话）
fkteams agent -n 小码
fkteams agent --name 小析

# 直接查询（一次性，执行后退出）
fkteams agent -n 小搜 -q "搜索最新的 Go 语言新闻"
fkteams agent -n 小码 -q "解释这个函数的作用"

# 配合管道
cat error.log | fkteams agent -n 小码 -q "分析这个错误日志"
git diff HEAD~1 | fkteams agent -n 小码 -q "审查这次提交"
cat data.csv | fkteams agent -n 小析 -q "计算基本统计数据"

# JSON 原始事件输出（用于程序化处理）
fkteams agent -n 小搜 -q "搜索 AI 新闻" --format json

# 自动批准工具调用
fkteams agent -n 小令 -q "清理临时文件" --approve all
fkteams agent -n 小码 -q "重构 main.go" --approve file,command

# 保存历史
fkteams agent -n 小搜 -q "搜索 AI 新闻" --save
```

### `agent` 子命令参数

| 参数 | 简写 | 说明 |
|------|------|------|
| `list` | | 列出所有可用智能体 |
| `--name` | `-n` | 智能体名称（必填，与 list 互斥） |
| `--query` | `-q` | 直接查询模式 |
| `--save` | | 保存历史（默认不保存） |
| `--format` | | 输出格式：`default`（格式化）或 `json`（原始事件） |
| `--approve` | | 自动批准：`all` / `command` / `file` / `dispatch` |

交互模式下，输入 `@` 符号后自动显示智能体列表供选择。

---

## 五、会话管理（`session` 子命令）

会话文件保存在 `~/.fkteams/sessions/`，以时间戳命名（如 `20260302_091249`）。

```bash
# 列出所有历史会话
fkteams session list

# 启动时自动保存（退出时写入文件）
fkteams --save
fkteams -q "你的问题" --save

# 恢复历史会话
fkteams -r "20260302_091249"
fkteams -r "20260302_091249" -q "继续上次的问题"
```

交互模式内：`save_chat_history` / `load_chat_history` / `clear_chat_history` / `save_chat_history_to_markdown` / `save_chat_history_to_html`

---

## 六、模型管理（`model` 子命令）

```bash
# 列出已配置的模型
fkteams model ls       # 或 fkteams model list

# 查询服务商的可用模型
fkteams model lr --name deepseek
fkteams model lr --provider openai

# 切换默认模型（交互式选择）
fkteams model sw
# 指定配置名
fkteams model sw --name deepseek
# 切换到指定模型
fkteams model sw --name deepseek --model deepseek-reasoner

# 移除模型配置（交互式选择）
fkteams model rm
fkteams model rm --name old-config
```

### 登录服务商（写入 config.toml，无需手动编辑）

```bash
fkteams login openai     --api-key sk-...
fkteams login deepseek   --api-key sk-...
fkteams login claude     --api-key sk-ant-...
fkteams login gemini     --api-key AIza...
fkteams login qwen       --api-key sk-...
fkteams login ollama                          # 无需 API Key
fkteams login ark        --api-key ...
fkteams login openrouter --api-key sk-or-...
fkteams login copilot                         # OAuth 设备码流程
fkteams login copilot --import                # 从 VS Code 已保存的 token 导入
fkteams login custom --base-url https://my-proxy.example.com/v1 --api-key sk-...

# 通用可选参数
fkteams login openai --api-key sk-... --model gpt-4o --name my-openai
# --name default 会覆盖当前默认模型

# 退出登录
fkteams logout
```

---

## 七、技能管理（`skill` 子命令）

技能目录：`~/.fkteams/skills/<技能名>/`，每个技能必须包含 `SKILL.md`。

```bash
# 列出本地已安装的技能
fkteams skill list    # 或 fkteams skill ls

# 搜索技能市场（默认后端：SkillHub）
fkteams skill search <关键词>
fkteams skill search ffmpeg --page 2 --size 20
fkteams skill search ffmpeg --provider SkillHub

# 安装技能
fkteams skill install <技能slug>
fkteams skill install video-frames --version 1.0.0
fkteams skill install video-frames --provider SkillHub

# 移除技能
fkteams skill remove <技能slug>
```

---

## 八、配置与初始化（`generate` / `init` 子命令）

```bash
# 生成示例配置文件（首次使用必须执行）
fkteams generate config
# 生成路径：~/.fkteams/config/config.toml

# 生成 OpenAI 兼容 API 密钥
fkteams generate apikey

# 初始化运行时依赖
fkteams init           # 交互式选择
fkteams init --all     # 安装全部（uv + bun）
fkteams init --env uv  # 仅安装 uv（Python 脚本工具）
fkteams init --env bun # 仅安装 bun（JS 脚本工具）
fkteams init --mirror  # 生成镜像源配置（国内加速）
```

### 关键配置项

```toml
# 模型（必填）
[[models]]
name     = "default"
provider = "openai"
base_url = "https://api.openai.com/v1"
api_key  = "sk-..."
model    = "gpt-4o"

# 智能体开关
[agents]
searcher  = true
assistant = true
analyst   = false

[agents.ssh_visitor]
enabled  = false
host     = "ip:port"
username = "user"
password = "pass"

# 长期记忆
[memory]
enabled = true

# Web 服务器
[server]
port = 23456

# OpenAI 兼容 API（需要先 fkteams generate apikey）
[openai_api]
api_keys = ["sk-fkteams-your-secret"]
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `FEIKONG_APP_DIR` | `~/.fkteams` | 应用数据目录 |
| `FEIKONG_PROXY_URL` | — | 代理地址（唯一的代理配置方式） |
| `FEIKONG_MAX_ITERATIONS` | `60` | 智能体最大迭代次数（0/-1 不限制） |

---

## 九、其他子命令

```bash
# 检查并更新 fkteams 到最新版本
fkteams update

# 列出所有可用工具
fkteams tool list

# 显示版本
fkteams --version
```

---

## 十、常见问题（Gotchas）

- **`--save` 必须显式设置**：历史默认不保存，需在启动时加 `--save`，或在交互中执行 `save_chat_history`。
- **管道触发非交互模式**：哪怕管道为空也会触发，空管道 + 无 `-q` 会报错。
- **`fkteams web` vs `fkteams serve`**：`web` 包含前端页面；`serve` 仅提供 API，无界面。
- **首次使用需先生成配置**：运行 `fkteams generate config` 后再启动，否则报错。
- **可选智能体需先在配置中启用**：调用未启用的智能体会返回错误。
- **代理只能通过环境变量设置**：`config.toml` 中没有代理配置项，只用 `FEIKONG_PROXY_URL`。
- **`model sw` 只切换默认，不删除配置**：要删除用 `model rm`。
- **技能安装后需重启 fkteams 才生效**：技能在启动时加载，热安装不生效。
- **`[openai_api]` 不配置 `api_keys` 时所有 API 请求返回 401**。
