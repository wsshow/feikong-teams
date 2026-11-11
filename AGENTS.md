# fkteams 开发指南

## 项目概览

fkteams 是基于 AgentKit（底层为 CloudWeGo Eino ADK）的 Go 多智能体协作系统，提供 CLI/TUI、Web UI、OpenAI 兼容 API，以及 Discord、QQ、微信消息通道。前端位于 `web/`，构建产物通过 `//go:embed` 嵌入 Go 二进制。

本文件适用于整个仓库。若子目录以后增加更具体的 `AGENTS.md`，只写该子树的增量规则；冲突时以更具体的文件和用户当次指令为准。

- Go 模块：`fkteams`，版本以 `go.mod` 为准。
- 前端：React、TypeScript、Vite、Bun。
- 默认应用目录：`~/.fkteams`；可通过 `FEIKONG_APP_DIR` 覆盖。
- 完整架构说明见 `docs/architecture.md`；修改跨层依赖或运行链路前先阅读该文档。

## 常用命令

```bash
# 完整质量检查（格式、前端类型检查与构建、Go vet/test、diff check）
make check

# 开发与定向验证
make web-build
go test ./path/to/package
go test ./path/to/package -run TestName
go build ./...
go vet ./...

# 运行
go run ./cmd/fkteams
go run ./cmd/fkteams web             # 默认监听 :23456
go run ./cmd/fkteams serve

# 前端
cd web && bun run dev
cd web && bun test src

# 发布构建
make native
make all
make build t=linux:amd64
make clean

# 生成配置示例
go run ./cmd/fkteams generate config
```

`go build ./...`、`go test ./...` 和 `go vet ./...` 依赖已生成的 `web/dist`；直接运行这些命令前先执行 `make web-build`，或使用已包含该步骤的 Make 目标。`web/dist/` 与 `release/` 是生成物，不提交。

## 架构边界

### 分层与依赖方向

```text
cmd -> bootstrap -> adapters -> app/runtime -> ports -> domain
```

- `internal/domain` 只放领域模型和值对象，不依赖框架、SDK、`app`、`runtime` 或 `adapters`。
- `internal/ports` 定义运行时无关契约，不依赖 `app`、`runtime` 或具体 adapter。
- `internal/app` 实现用例，不导入具体 adapter、AgentKit、Eino 或终端展示库。
- `internal/runtime` 提供运行时无关内核，不依赖 `app` 或具体 adapter。
- `internal/adapters` 实现外部技术和传输协议；AgentKit、Eino 及其 SDK 只能出现在 `internal/adapters/runtime/agentkit`。
- `internal/bootstrap` 是组合根，负责创建并连接 runtime、模型、工具、存储和后台服务。
- `cmd/fkteams/main.go` 保持最小化，只连接组合根和 CLI 命令入口。

入口必须调用 `internal/app` 用例，不能在 CLI、HTTP 或消息通道中重新实现核心业务流程。注册表和运行态依赖必须由应用实例持有并显式注入；不要引入可变的进程级默认实例、依赖注册副作用的 `init()` 或空白 import。

不要恢复已经移除的根级 `agentcore`、`events`、`server`、`cli`、`channels` 门面。对应实现分别位于 `internal/app`、`internal/runtime/events` 和 `internal/adapters/transport`。

### 关键目录

- `cmd/fkteams/`：可执行入口。
- `internal/app/`：chat、agent、tools、memory、schedule、skill、lifecycle 等用例。
- `internal/domain/`：event、history、memory、message、schedule、session 等领域类型。
- `internal/ports/`：hooks、memory、runtime、scheduler、storage、tools 契约。
- `internal/runtime/`：turn、events、hooks、checkpoint、retry、pathguard 等内核能力。
- `internal/adapters/`：模型、runtime、工具、存储、调度器和传输实现。
- `internal/bootstrap/`：默认依赖与服务装配。
- `web/`：React 前端；`web/dist/` 为生成物。

## Go 编码约定

- 内部 `error` 文本使用英文；面向用户的 UI 文案可以使用中文。
- 注释使用中文。新增导出的类型、函数和方法应提供符合 Go Doc 的简洁注释；私有代码只在需要解释意图、约束或非显然行为时添加注释。
- 使用 `any`，不要新增 `interface{}`。
- 禁止 emoji 图形字符；`✓`、`✗` 等文字符号可以使用。
- 向 `strings.Builder` 写格式化内容时使用 `fmt.Fprintf(&builder, ...)`，不要组合 `WriteString(fmt.Sprintf(...))`。
- 可能失败的初始化必须返回 `error`，不得通过 `panic` 或 `log.Fatal` 终止进程；简单且不会失败的构造函数可以只返回实例。
- 能编码进工具响应的业务错误或校验错误应写入响应的 `ErrorMessage` 并返回 `nil` error；框架初始化、工具构建和无法形成有效响应的故障仍正常返回 `error`。
- 事件类型、动作类型和通知类型必须使用 `internal/domain/event` 中的常量，禁止散落字符串字面量。
- 修改 Go 文件后运行 `gofmt`；不要手工维护与 `gofmt` 冲突的格式。

## 子系统变更清单

### 智能体

- 新内置智能体以 `common.Definition` 声明，通过 `common.BuildAgent()` 创建；参考 `internal/app/agent/catalog/common/definition.go`。
- 在 `internal/app/agent/catalog/registry.go` 的 `builtinAgentSpecs()` 中声明元信息和默认 definition，最终由 `buildRegistry()` 生成目录。
- 每个内置智能体目录保留 agent 定义/工厂与独立的系统提示词模板。
- 普通、团队、Deep 与后台子任务统一使用 AgentKit 执行；不直接创建 ADK Runner 或预构建智能体。
- 智能体声明可缓存，运行会话和检查点由每次 Run 持有；不要把会话历史放进共享 Runner。

### 工具

- 工具列表统一使用 `ports/runtime.ToolDefinition` 与 `runtimeport.NewTools()` 声明，保持明确的请求、响应类型；复用流程，不为每个声明创建小文件。
- 新工具组通过 `internal/app/tools.ToolGroupRegistry` 注册，不要在 `internal/app/tools/tools.go` 增加 switch 分支。
- 依赖存储、调度器或第三方 SDK 的实现放在 `internal/adapters/tools`，通过 `internal/bootstrap/tools` 接入；`internal/app/tools` 不得反向导入 adapter。
- go-git 实现只放在 `internal/adapters/tools/builtin/git`；SSH/SFTP 实现只放在 `internal/adapters/tools/builtin/ssh`。
- MCP 动态工具通过 `internal/ports/tools.MCPProvider` 注入；`internal/app/tools` 不直接导入 `github.com/mark3labs/mcp-go`。
- 工具列表使用 `internal/runtime/toolpolicy.ClassifyTools()` 标记只读、破坏性、审批等策略元数据。

### 配置与模型

- 新配置项同步加入 `internal/app/config/config.go` 的 `GenerateExample()`；运行时配置通过 `config.Get()` 读取，保留 `atomic.Pointer` 热重载机制。
- HTTP、消息通道与调度执行共用组合根注入的 `config.ExecutionGate`；配置写入必须在空闲边界更新配置及派生依赖，运行任务存在时返回冲突。
- 新模型提供者通过 `internal/adapters/model/providers` 的实例级工厂注册，并实现模型创建与列表查询。

### 生命周期与后台服务

- 新后台服务实现 `internal/app/lifecycle.Service` 的 `Name`、`Start`、`Stop`；具体组合层服务放在 `internal/bootstrap/services`。
- 服务按注册顺序启动，按逆序（LIFO）停止；停止和收尾错误必须向调用方返回。关闭与重启使用实例级 `lifecycle.Control`。
- 服务的 `Start(ctx)` 必须将应用取消传递给所属任务，`Stop(ctx)` 等待任务真实退出。应用关闭超时只结束调用方等待，后台继续排空后再执行 Cleanup；嵌入方通过 `WaitShutdown(ctx)` 确认资源释放，不得在任务仍运行时清理共享依赖。
- `turn.Request.OnInterrupt` 未设置时必须保持固定拒绝的安全默认值。

### 事件、Hooks 与流式任务

- 发事件使用 `internal/runtime/events` 的 `Emitter` 和事件构造函数；新增事件、动作或通知前先在 `internal/domain/event` 定义常量。
- 规范流式增量只使用 `Content`，不要在核心事件或历史存储中重新维护 `Delta`。
- 工具调用链通过稳定的 `tool_call_ref` 关联 `message_delta(tool_args)`、`message_end.tool_calls[]` 与 `tool_start/update/end`。
- Hook payload 在 `internal/ports/hooks` 定义为实现 `hooks.Payload` 的明确结构体；不要以 `any` 作为 `Invocation` 或 `Result` 的 payload 契约。新增 hook point 时同步增加便捷调用函数和架构边界测试。
- WebSocket `steer`、`/stream/steer` 和终端运行中 Enter 进入 steering 通道，由 `SteeringSource` 在下一次模型调用前消费；运行中的普通 `chat`/`follow_up` 只追加后续任务。
- 流式队列项必须有稳定 `queue_id`，并通过 `queue_updated` 向 Web/SSE/WS 同步。只能编辑、删除或排序尚未消费的项；终端暂停时将未消费 steering 回填输入框。
- 持久化队列的恢复、编辑与消费统一通过 `chat.QueueService`；入口不得自行组合 Stream 修改、文件保存和通知。保存失败不能确认修改成功，取消后的 steering 保留给下一次运行。
- 回合历史通过 `TurnRequest.History` 注入，由 `chat.Service` 统一记录、定稿与保存；视图只接收事件，`SessionLifecycle` 只负责历史提交后的 metadata 和记忆更新。
- 前端事件字段在 `web/src/types/events.ts` 的声明中同时定义类型和校验，历史兼容字段只在 decoder 归一化。协议变化同步更新 Go/Bun 共用的 `chat-events.json` 样本及相关测试。

### 传输与消息通道

- HTTP handler、middleware、router 和 origin 策略位于 `internal/adapters/transport/http`；CLI 会话和查询执行位于 `internal/adapters/transport/cli/runtime`。
- 消息通道 adapter 导出接收 `*channel.FactoryRegistry` 的 `Register()`，由 `internal/bootstrap/channels.RegisterDefaults()` 显式装配，并通过 `channel.Bridge` 路由到应用用例。

## 安全约束

- 不得绕过或弱化 workspace 路径限制、符号链接检查、危险命令检测、工具权限元数据和 HITL 审批。
- 不在日志、错误、测试夹具或文档中写入 API key、token、密码、会话凭据等秘密。
- 修改文件、Git、SSH、命令执行、MCP 或上传下载路径时，补充路径逃逸、拒绝策略或权限边界测试。
- 消息通道明确默认自动审批，由 Bridge 注入自动审批注册表；其他后台及未配置审批策略的非交互执行默认拒绝高风险操作。自动审批不豁免路径限制、危险命令检测和工具权限元数据。

## 验证与交付

按改动风险选择验证，不要为了机械满足清单而运行无关命令：

- 文档、注释或提示词：至少运行 `make diff-check`。
- 单一 Go package 的局部改动：先运行该 package 测试；交付前补充 `go build ./...`，必要时运行 `go vet ./...`。
- 功能、跨包重构、架构边界或运行时行为改动：运行 `make check`。
- 前端改动：运行相关 `bun test`、`make web-typecheck` 和 `make web-build`；涉及 Go 嵌入或端到端行为时再运行 `make check`。
- 安全边界、并发、生命周期、持久化或协议改动必须增加或更新对应测试。

如果完整验证受环境限制，运行仍可执行的最小相关检查，并在交付说明中列出未运行项、阻塞原因和剩余风险。不要掩盖失败，也不要把既有无关失败描述成当前改动已通过。

`README.md` 只维护面向新用户的稳定概览。新增或移除主要入口、核心能力，或安装与快速开始方式发生变化时才更新 README；局部功能、兼容性修复、内部重构和实现细节无需修改。面向用户的详细行为变化按需更新相应 `docs/` 专题文档，架构边界变化更新 `docs/architecture.md`。

提交信息遵循 Conventional Commits：`<type>(<scope>): <summary>`，scope 可省略；type/scope 用小写英文，摘要可用中文。常用类型为 `feat`、`fix`、`refactor`、`perf`、`docs`、`test`、`build`、`ci`、`style`、`chore`、`revert`。除非用户明确要求，不要自行创建提交、推送或 PR。
