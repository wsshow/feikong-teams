# fkteams 开发规范

## 项目简介

fkteams 是基于 CloudWeGo Eino ADK 框架的多智能体协作系统，支持多种协作模式（Supervisor、圆桌讨论、自定义组合、深度探索、后台任务），提供 CLI 和 Web 双界面。

## AI 开发行为规范

### 禁止行为

1. **禁止随意创建文件**: 不要自行创建 markdown 文档、测试文件或其他辅助文件，除非用户明确要求
2. **禁止冗余代码**: 不添加未使用的 import、变量或函数
3. **禁止过度注释**: 只在复杂逻辑处添加必要注释

### 必须执行

1. **功能变更后更新 README.md**: 任何功能新增、修改或删除，必须同步更新 `README.md` 相关章节
2. **新增配置项更新配置文件**: 添加新配置项时，必须在 `config/config.go` 的 `GenerateExample` 中添加对应条目和注释
3. **代码审查**: 编码完成后必须检查代码是否有语法错误、逻辑问题，确保无编译报错
4. **保持简洁**: 代码实现以最简洁有效的方式完成，避免过度设计

## 项目结构

```
fkteams/
├── main.go                 # 程序入口
├── agents/                 # 智能体实现
│   ├── registry.go         # 智能体注册表（延迟加载 + 按需创建）
│   ├── common/             # 公共模块
│   │   ├── common.go       # NewChatModel() 返回 (model, error), MaxIterations, IsRetryAble
│   │   └── builder.go      # AgentBuilder 流式构建器
│   ├── middlewares/         # 智能体中间件
│   │   ├── dispatch/       # 子任务并行分发中间件
│   │   ├── fkfs/           # 文件系统后端（内存/本地）
│   │   ├── skills/         # 技能学习中间件（读取 workspace/skills/ 目录）
│   │   ├── summary/        # 自动摘要中间件（80K token 阈值）
│   │   └── tools/warperror/# 工具错误包装中间件
│   ├── coder/              # 代码专家 (小码)
│   ├── searcher/           # 搜索专家 (小搜)
│   ├── cmder/              # 命令行专家 (小令)
│   ├── visitor/            # SSH 专家 (小访)
│   ├── storyteller/        # 讲故事专家 (小说)
│   ├── analyst/            # 数据分析师 (小析)
│   ├── summarizer/         # 总结专家 (小简)
│   ├── assistant/          # 个人全能助手 (小助)
│   ├── leader/             # 团队协调者 (统御)
│   ├── tasker/             # 后台任务执行 (任务官)
│   ├── moderator/          # 自定义模式主持人 (小议)
│   ├── deep/               # 深度探索协调者
│   ├── discussant/         # 圆桌讨论者（多模型支持）
│   └── custom/             # 自定义智能体（配置驱动）
├── tools/                  # 工具实现
│   ├── tools.go            # GetToolsByName 统一工具注册入口
│   ├── approval/           # 审批机制 (HITL / Reject)
│   ├── file/               # 文件操作 (afero 沙箱隔离)
│   ├── git/                # Git 仓库操作
│   ├── excel/              # Excel 文件处理
│   ├── command/            # 命令执行
│   ├── ssh/                # SSH 连接
│   ├── search/             # DuckDuckGo 搜索
│   ├── fetch/              # 网页抓取
│   ├── doc/                # 文档工具
│   ├── todo/               # 待办事项管理
│   ├── scheduler/          # 定时任务调度
│   ├── script/             # 脚本工具
│   │   ├── uv/             # Python uv 工具
│   │   └── bun/            # JavaScript bun 工具
│   └── mcp/                # MCP 协议动态工具
├── runner/                 # Runner 工厂（创建各模式的 adk.Runner）
├── config/                 # 配置管理（TOML 配置文件解析）
├── common/                 # 全局共用（常量、WorkspaceDir、ResourceCleaner）
├── g/                      # 全局变量（Cleaner、MemoryManager）
├── cli/                    # CLI 交互（命令、会话、自动补全）
├── commands/               # Cobra 命令定义（chat/serve/init/update/generate...）
├── fkevent/                # 事件系统（EventPrinter、SessionManager）
├── engine/                 # 引擎（统一 CLI/Web 的运行入口）
├── lifecycle/              # 生命周期管理（Service 接口、优雅启停）
├── memory/                 # 长期记忆（BM25 检索、Markdown 存储）
├── chatutil/               # 消息构建工具
├── bootstrap/              # 运行时引导（bun/uv 安装检查）
├── mdiff/                  # Markdown diff 工具
├── report/                 # 报告生成
├── log/                    # 日志配置
├── tui/                    # TUI 组件（输入、选择、文本域）
├── update/                 # 自动更新
├── utils/                  # 路径工具
├── version/                # 版本信息
├── server/                 # Web 服务
│   ├── server.go           # HTTP 服务（lifecycle.Service 实现，返回 error）
│   ├── handler/            # 请求处理
│   │   ├── resp.go         # 统一响应 OK()/Fail()
│   │   ├── conn.go         # WS 连接池管理
│   │   ├── chat.go         # RunnerCache 类型 + 聊天处理
│   │   ├── websocket.go    # WS handler + 消息路由
│   │   ├── history.go      # 会话 CRUD
│   │   ├── agents.go       # 智能体列表
│   │   ├── auth.go         # 认证（AuthEnabled 返回 error）
│   │   ├── files.go        # 文件列表
│   │   └── version.go      # 版本信息
│   ├── router/             # 路由注册（Init/InitAPI 返回 error）
│   └── middleware/         # 中间件 (CORS)
└── web/                    # 前端静态资源（embed.go 嵌入）
```

## 开发约定

### 智能体开发

每个智能体包含两个文件:
- `{name}.go`: 实现 `NewAgent(ctx context.Context) (adk.Agent, error)` 函数
- `prompt.go`: 定义小写未导出的提示词模板变量（如 `searcherPromptTemplate`）

**所有智能体通过 `AgentBuilder` 流式构建器创建**（定义在 `agents/common/builder.go`）:

```go
// 简单智能体（无工具 / 少量工具）
func NewAgent(ctx context.Context) (adk.Agent, error) {
    return common.NewAgentBuilder("小搜", "搜索专家...").
        WithTemplate(searcherPromptTemplate).
        WithToolNames("search", "fetch").
        Build(ctx)
}

// 带中间件的复杂智能体
func NewAgent(ctx context.Context) (adk.Agent, error) {
    return common.NewAgentBuilder("小助", "个人全能助手...").
        WithTemplate(assistantPromptTemplate).
        WithTemplateVar("os_type", runtime.GOOS).
        WithTemplateVar("workspace_dir", common.WorkspaceDir()).
        WithToolNames("command", "file", "todo", "scheduler", "search", "fetch").
        WithSummary().     // 自动摘要
        WithSkills().      // 技能学习
        Build(ctx)
}
```

**AgentBuilder API**:
| 方法 | 说明 |
|------|------|
| `NewAgentBuilder(name, desc)` | 创建构建器（自动设置 `current_time` 模板变量） |
| `WithTools(tools...)` | 添加工具实例 |
| `WithToolNames(names...)` | 通过名称添加工具（Build 时通过 `GetToolsByName` 解析） |
| `WithTemplate(t)` | 设置提示词模板 |
| `WithTemplateVar(k, v)` | 添加模板变量 |
| `WithModel(m)` | 使用自定义模型（不设置则用配置文件默认模型） |
| `WithMiddleware(m...)` | 添加 AgentMiddleware |
| `WithHandler(h...)` | 添加 ChatModelAgentMiddleware |
| `WithSummary()` | 启用 summary 中间件（80K token 阈值） |
| `WithSkills()` | 启用 skills 中间件 |
| `WithDispatch(cfg)` | 启用子任务并行分发中间件（`*dispatch.Config`） |
| `Build(ctx)` | 构建并返回 `(adk.Agent, error)` |

**智能体签名约定**:
- 基本: `func NewAgent(ctx context.Context) (adk.Agent, error)`
- 带参数: `func NewAgent(ctx context.Context, cfg Config) (adk.Agent, error)`（custom）
- 带参数: `func NewAgent(ctx context.Context, member config.TeamMember) (adk.Agent, error)`（discussant）
- 带参数: `func NewAgent(ctx context.Context, subAgents []adk.Agent) (adk.Agent, error)`（deep）

### 智能体注册

智能体通过 `agents/registry.go` 注册，采用延迟加载模式:
- `agentCreator` 类型: `func(ctx context.Context) (adk.Agent, error)`
- 内置智能体在 `init()` 中注册，自定义智能体从配置文件动态加载
- 通过 `agents.ListAgents(ctx)` 获取所有可用智能体

### 工作模式

系统支持 5 种工作模式，在 `runner/runner.go` 中创建对应的 `adk.Runner`:

| 模式 | 函数 | 说明 |
|------|------|------|
| Supervisor | `CreateTaskRunner` | 默认模式，Leader 协调多个专家智能体 |
| Roundtable | `CreateLoopAgentRunner` | 圆桌讨论，多个 Discussant 轮流发言 |
| Custom | `CreateCustomSupervisorRunner` | 自定义主持人 + 自定义智能体组合 |
| DeepAgents | `CreateDeepRunner` | 深度探索模式 |
| Background | `CreateBackgroundRunner` | 后台定时任务（Tasker 执行） |

### 中间件栈

```
┌──────────────────────────────┐
│  Warperror Middleware        │  ← 工具错误 → 成功输出（不中断 Agent 流程）
├──────────────────────────────┤
│  Summary Middleware          │  ← 80K token 阈值自动摘要历史
├──────────────────────────────┤
│  Skills Handler              │  ← 读取 workspace/skills/ 注入上下文
└──────────────────────────────┘
```

### 审批机制

通过 `tools/approval/` 包实现，支持两种模式:
- `ApprovalModeHITL` (默认值 0): 人工审批，工具调用前请求用户确认
- `ApprovalModeReject` (值 1): 自动拒绝危险操作（仅 tasker 使用）

### 工具开发

工具通过 `tools/tools.go` 的 `GetToolsByName(name string) ([]tool.BaseTool, error)` 注册:

**已注册工具名**:
| 名称 | 说明 |
|------|------|
| `file` | 文件操作（沙箱隔离） |
| `git` | Git 仓库操作 |
| `excel` | Excel 文件处理 |
| `todo` | 待办事项管理 |
| `ssh` | SSH 远程连接 |
| `command` | 命令执行 |
| `scheduler` | 定时任务调度 |
| `search` | DuckDuckGo 搜索 |
| `fetch` | 网页抓取 |
| `doc` | 文档工具 |
| `uv` | Python uv 工具 |
| `bun` | JavaScript bun 工具 |
| `mcp-{name}` | MCP 协议动态工具（前缀匹配） |

#### 工具函数错误处理规范

**关键原则**: 工具函数不应该抛出 Go error，而应该将错误信息放在响应结构体的 `ErrorMessage` 字段中。

```go
// 错误做法 - 会导致 Agent 流程中断，用户看到系统级错误
func (t *Tool) DoSomething(ctx context.Context, req *Request) (*Response, error) {
    if err := someOperation(); err != nil {
        return &Response{
            ErrorMessage: fmt.Sprintf("操作失败: %v", err),
        }, err  // 不要返回 error
    }
    return &Response{Success: true}, nil
}

// 正确做法 - Agent 可以看到错误信息并继续执行
func (t *Tool) DoSomething(ctx context.Context, req *Request) (*Response, error) {
    if err := someOperation(); err != nil {
        return &Response{
            Success:      false,
            ErrorMessage: fmt.Sprintf("操作失败: %v，请检查...", err),
        }, nil  // 返回 nil
    }
    return &Response{Success: true, Message: "操作成功"}, nil
}
```

**何时可以返回 error？** 仅在工具初始化阶段（如 `NewXXXTools`）可以返回 error，因为初始化失败应该阻止程序启动。

### 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 智能体名称 | 中文 | 小码、小搜 |
| 包/文件名 | 小写英文 | coder, searcher |
| 工具函数 | 下划线分隔 | file_read, ssh_execute |
| 环境变量 | FEIKONG_ 前缀 | FEIKONG_APP_DIR |
| 提示词模板变量 | 小写未导出 | searcherPromptTemplate |
| 全局变量 | 完整语义命名 | MemoryManager（非缩写） |

### 代码风格规范

1. **错误信息使用英文**: `fmt.Errorf`、`errors.New` 等错误信息一律使用英文
   ```go
   // 正确
   return fmt.Errorf("failed to read file: %w", err)
   // 错误
   return fmt.Errorf("读取文件失败: %w", err)
   ```

2. **注释使用中文**: 代码注释、文档注释使用中文
   ```go
   // 初始化文件工具，限制在指定目录内
   func NewFileTools(dir string) (*FileTools, error) {
       // ...
   }
   ```

3. **日志输出**: 面向用户的日志可使用中文，内部调试日志使用英文

4. **错误处理**: 所有初始化函数返回 `error`（不使用 `log.Fatal`），运行时错误通过事件系统传递

### 全局变量

定义在 `g/g.go`:
- `g.MemoryManager`: 全局长期记忆管理器
- `g.ProcessCleaner`: 进程级资源清理器（终止后台子进程、关闭 SSH 连接等）
- `g.RunProcessCleanup()`: 执行所有进程级清理函数

公共函数定义在 `common/common.go`:
- `AppDir()`: 应用数据目录（默认 `~/.fkteams`，支持 `FEIKONG_APP_DIR` 环境变量覆盖）
- `WorkspaceDir()`: 工作目录（固定为 `~/.fkteams/workspace`）
- `GenerateSessionID()`: 生成 UUID v4 会话 ID

### 配置管理

所有配置统一由 `config/config.go` 管理，使用 TOML 格式配置文件 (`~/.fkteams/config/config.toml`):

- `config.Init()`: 初始化配置（应在启动时调用一次）
- `config.Get()`: 返回全局配置单例 `*Config`（无 error）
- `config.ResolveModel(name)`: 通过名称查找模型配置，空名称返回 "default" 模型
- `config.ProxyURL()`: 返回代理 URL（配置文件优先，环境变量回退）
- `config.GenerateExample()`: 生成示例配置文件

模型池设计: 模型定义为具名实体（`[[models]]`），其他配置通过名称引用。

```toml
[[models]]
name = "default"
provider = "openai"
base_url = "https://api.openai.com/v1"
api_key = "xxxxx"
model = "GPT-5"

[agents]
coder = true
cmder = true
assistant = true

[server.auth]
enabled = false
username = "admin"
password = "admin"
secret = "your_jwt_secret"
```

### 环境变量

仅保留用于 Docker 等场景的环境变量回退:
```bash
FEIKONG_APP_DIR            # 应用数据目录 (默认 ~/.fkteams)
FEIKONG_PROXY_URL          # 代理地址 (配置文件优先)
# 以下为 NewChatModelFromEnv 回退（未配置 config.toml 时使用）
FEIKONG_API_KEY            # 模型 API Key
FEIKONG_BASE_URL           # 模型 Base URL
FEIKONG_MODEL              # 模型名称
FEIKONG_PROVIDER           # 模型提供者类型 (可选，自动检测)
FEIKONG_EXTRA_HEADERS      # 额外 HTTP 请求头 (格式: Key1:Value1,Key2:Value2)
```

## Web 服务

服务通过 `lifecycle.Service` 接口管理，支持优雅启停:
- `server.go` 的 `run()` 返回 `error`（不使用 `log.Fatal`）
- 认证: `handler/auth.go` 的 `AuthEnabled()` 返回 `(bool, error)`
- 路由: `router/` 的 `Init()` / `InitAPI()` 返回 `(http.Handler, error)`
- Runner 缓存: `handler/chat.go` 的 `RunnerCache` 类型管理 Web 模式下的 Runner 实例

**API 端点**:
```
GET    /agents      # 获取智能体列表
GET    /files       # 获取文件列表
WS     /ws          # WebSocket 聊天
GET    /history     # 历史管理
GET    /version     # 版本信息
POST   /login       # 登录（若启用认证）
```

## CLI 命令

| 类别 | 命令 | 说明 |
|------|------|------|
| 基本 | `help` `q` `quit` | 帮助、退出 |
| 智能体 | `list_agents` `@agent_name` | 列表、直接对话 |
| 文件引用 | `#path` | 快速引用文件内容 |
| 聊天历史 | `list_chat_history` `load_chat_history` | 列表、加载 |
| | `save_chat_history` `clear_chat_history` | 保存、清空 |
| | `save_chat_history_to_markdown` | 导出为 Markdown |
| | `save_chat_history_to_html` | 导出为 HTML |
| 定时任务 | `list_schedule` `cancel_schedule` `delete_schedule` | 查看、取消、删除 |
| 模式 | `switch_work_mode` | 切换工作模式 |
| 记忆 | `list_memory` `delete_memory` `clear_memory` | 长期记忆管理 |

## 构建与运行

```bash
make build                                         # 构建到 release/
./release/fkteams_darwin_arm64 -m team -q "问题"    # CLI 模式
./release/fkteams_darwin_arm64 -m team -w           # Web 模式
./release/fkteams_darwin_arm64 serve --port 8080    # API 服务模式
```

## 代码风格

1. 错误处理: 初始化函数返回 `error`，运行时错误通过事件系统传递
2. 并发安全: WebSocket 连接池使用 `sync.Mutex` 保护，RunnerCache 使用双重检查锁
3. 文件操作: 所有路径通过 `afero.BasePathFs` 沙箱隔离
4. 日志格式: `log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)`
5. 模型创建: `NewChatModel()` 返回 `(model, error)`，配合 `MaxIterations=60`、`MaxRetries=3`

## 检查清单

完成开发后确认:

- [ ] 代码无编译错误（`go build ./...` + `go vet ./...`）
- [ ] 无未使用的 import/变量
- [ ] 功能变更已更新 README.md
- [ ] 新配置项已添加到 `config/config.go` 的 `GenerateExample`
- [ ] 新智能体使用 `AgentBuilder` 创建并接受 `ctx` 参数
- [ ] 新工具已在 `tools/tools.go` 的 `GetToolsByName` 中注册
- [ ] 遵循现有代码风格
