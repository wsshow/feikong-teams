# 架构设计

本文档定义当前主线目标架构。项目版本仍处于 `0.x` 阶段，不承诺内部包、接口和目录结构的向后兼容；重构优先级是长期清晰、稳定、可扩展，而不是保留历史调用方式。

## 设计原则

- 内核优先：所有入口只调用应用用例层，不直接拼装 runner、history、memory、approval、hooks 或 runtime。
- 端口驱动：领域和用例只依赖接口，Eino、HTTP、CLI、文件系统、MCP、定时器等全部是 adapter。
- 单向依赖：`domain -> ports -> app -> adapters/bootstrap` 方向不可反转。
- 运行时可替换：Eino 只是默认 runtime adapter，不是核心设计的一部分。
- 状态集中治理：session、history、checkpoint、memory、task queue、schedule result 都通过明确 store/repository 接口管理。
- 事件协议稳定：运行时事件、用户通知、历史记录和传输事件分层转换，不在入口层重复拼事件。
- 可破坏性调整：旧包可以删除、重命名、移动；只需要保证功能等价、数据可迁移和验证完整。

## 目标目录

```text
cmd/fkteams/
  main.go

internal/domain/
  message/          # Message、ContentPart、ToolCall 等模型调用协议
  event/            # RunEvent、Notification、Usage、ToolSpan 等事件领域模型
  agent/            # AgentSpec、TeamSpec、PromptSpec、MemberSpec
  tool/             # ToolSpec、ToolCall、ToolPolicy、ToolCategory
  session/          # SessionID、TurnID、TurnInput、HistorySnapshot
  memory/           # MemoryEntry、MemoryQuery、MemoryInjection
  schedule/         # Task、Status、HistoryEntry、CronPolicy
  approval/         # ApprovalRequest、Decision、Policy

internal/ports/
  runtime/          # Runtime、Runner、Model、Middleware、CheckpointStore
  storage/          # SessionStore、HistoryStore、MemoryStore、TaskStore
  tools/            # ToolRegistry、ToolFactory、MCPProvider
  hooks/            # HookPoint、HookHandler、明确 payload 契约
  scheduler/        # Scheduler、TaskExecutor、Clock
  transport/        # EventPublisher、StreamHub、ChannelGateway

internal/app/
  chat/             # StartTurn、RunTurn、QueueFollowUp、Steer、Stop
  session/          # Create、Resume、Persist、Summarize
  agent/            # ResolveAgent、BuildTeam、BuildRunner
  memory/           # Search、Inject、Extract
  schedule/         # AddTask、RunDueTask、CancelTask、TaskHistory
  tools/            # ResolveToolGroups、ClassifyPolicy、CreateToolRuntime
  lifecycle/        # App lifecycle and service orchestration

internal/runtime/
  turn/             # runtime-independent turn executor
  registry/         # runtime registry and default runtime selection
  hooks/            # HookBus 实现、context 绑定和扩展点调用
  middleware/       # runtime-neutral middleware contracts
  checkpoint/       # checkpoint implementations used by runtime ports
  queue/            # steering/follow-up queue primitives

internal/adapters/
  runtime/eino/     # all CloudWeGo Eino dependencies
  model/openai/
  model/claude/
  model/deepseek/
  model/gemini/
  storage/file/
  storage/memory/
  transport/http/
  transport/cli/
  transport/channel/
  tools/builtin/
  tools/mcp/
  scheduler/filecron/

internal/bootstrap/
  app.go            # composition root
  config.go
  runtimes.go
  models.go
  tools.go
  storage.go
  services.go

web/
docs/
```

## 层级职责

| 层级 | 职责 | 禁止 |
| ---- | ---- | ---- |
| `domain` | 纯业务模型和值对象 | 禁止 import adapters、app、config、Eino、Gin、TUI、文件系统实现 |
| `ports` | 外部能力接口和契约 | 禁止 import adapters、app、具体第三方 SDK |
| `app` | 用例编排、事务边界、状态流转 | 禁止 import `agentcore` 和具体 adapter；只能依赖 `domain` 和 `ports` |
| `runtime` | 与具体框架无关的执行内核组件 | 禁止 import `agentcore`、Eino、HTTP、CLI、server handler |
| `adapters` | 技术实现和协议转换 | 可以依赖 `domain`、`ports`、第三方 SDK；不得被 domain/app 反向依赖 |
| `bootstrap` | 组装配置、adapter、service | 是唯一允许主动连接所有层的 composition root |

## 核心用例中轴

所有入口统一调用 `internal/app/chat.Service`。Web、CLI、消息通道、定时任务都不再直接调用 runner 或 engine。

```go
type Service interface {
    StartTurn(ctx context.Context, req StartTurnRequest) (*TurnHandle, error)
    SubmitFollowUp(ctx context.Context, req SubmitMessageRequest) (*QueueItem, error)
    SubmitSteering(ctx context.Context, req SubmitMessageRequest) (*QueueItem, error)
    StopTurn(ctx context.Context, sessionID string) error
}
```

`chat.Service` 统一负责：

- 加载和保存 session/history。
- 注入长期记忆。
- 创建或复用 runner。
- 装配 approval、ask、steering、hooks。
- 分发事件到 history、stream、CLI view、channel reply。
- 处理 follow-up 队列和 steering 队列。
- 运行结束后提取记忆、更新 session metadata。

入口层只负责 DTO 转换和用户交互：

- HTTP handler：JSON/SSE/WebSocket 转换。
- CLI：终端输入、展示、快捷键。
- Channel：平台消息转换、回复发送。
- Scheduler：时间触发和结果存档。

## Runtime 端口

运行时接口必须足够小，不能暴露 Eino 概念。

```go
type Runtime interface {
    BuildAgent(ctx context.Context, spec AgentSpec) (Agent, error)
    BuildRunner(ctx context.Context, spec RunnerSpec) (Runner, error)
    Capabilities() RuntimeCapabilities
}

type Runner interface {
    Run(ctx context.Context, input TurnInput, opts RunOptions) (*RunResult, error)
}
```

Runtime adapter 负责把目标协议转换到底层框架：

- Eino adapter 内部使用 ADK Agent、Runner、Middleware。
- 其他 runtime 只要实现 ports/runtime 即可接入。
- `internal/adapters/runtime/eino` 是唯一允许 import `github.com/cloudwego/eino*` 的目录。

## Agent 与 Tool 设计

智能体目录不再创建实际 runtime agent，只声明规格：

```go
type AgentSpec struct {
    Name        string
    Description string
    Prompt      PromptSpec
    Model       ModelRef
    Tools       []ToolGroupRef
    Policies    []PolicyRef
    Features    AgentFeatures
}
```

工具系统改为注册表：

```go
type ToolRegistry interface {
    Register(group ToolGroupSpec, factory ToolFactory) error
    Resolve(ctx context.Context, refs []ToolGroupRef) ([]Tool, []ToolPolicy, error)
    Catalog(ctx context.Context) ([]ToolGroupInfo, error)
}
```

工具不得反向调用应用执行层。定时任务工具只提交 schedule use case；真正执行任务由 `app/schedule` 调用 `app/chat`。

## 状态与存储

状态能力按用途拆分：

- `SessionStore`：session metadata、title、status、timestamps。
- `HistoryStore`：结构化消息、事件、摘要。
- `CheckpointStore`：runtime checkpoint。
- `MemoryStore`：长期记忆。
- `TaskStore`：schedule task、result、history。
- `StreamStore` 或 `StreamHub`：运行中事件流和队列快照。

文件系统只是 adapter；核心不直接拼路径。

## Hooks

Hooks 属于用例和运行时之间的稳定扩展边界：

- before/after turn
- before/after model request
- before/after tool call
- on event
- before/after memory injection
- before/after schedule execution

hook payload 使用 `internal/ports/hooks` 中的明确结构体，不在业务代码里散落 `any` 和字符串 hook point。`internal/runtime/hooks` 只负责总线实现、超时/错误策略、context 绑定和便捷调用；旧顶层 `hooks` 包不再保留。

## 事件分层

事件分三层：

1. `domain/event`：运行时和用例的事实事件。
2. `app` event pipeline：补齐 session、turn、sequence、history metadata。
3. `transport` event DTO：SSE、WebSocket、CLI、channel 展示格式。

根 `events` 包只负责事件构造、分发和协议校验，必须直接依赖 `internal/domain/event`、`internal/domain/message` 和运行时端口，不得再通过 `agentcore` 旧门面取类型。

历史记录只保存 domain/app 事件，不保存入口层展示 DTO。

## 当前历史包袱处理

以下旧包不作为长期结构保留：

- `agentcore`：拆到 `internal/domain/*` 和 `internal/ports/runtime`；`internal/app` 与 `internal/runtime` 已禁止依赖该旧门面。
- `engine`：已移除，实际执行内核迁入 `internal/runtime/turn`。
- `runner`：已移除，Runner 工厂和缓存实现并入 `internal/app/agent`。
- `events/chat`：已移除，turn input builder 实现并入 `internal/app/chat`。
- `events/log`：已迁移为 `internal/adapters/storage/file/history`，历史记录实现归属文件存储适配器。
- `tools/tools.go`：已由 `ToolGroupRegistry` 接管解析，禁止重新引入 switch 分发。
- `tools/*`：工具层已直接使用 `internal/ports/runtime` 与领域类型，禁止再通过 `agentcore` 旧门面获取 Tool/Interrupt/ContentPart。
- `agents/*`：智能体构建层已直接使用 `internal/ports/runtime` 与领域类型，禁止再通过 `agentcore` 旧门面获取 Agent/Tool/Model/Middleware。
- `tools/scheduler`：已拆成 `internal/domain/schedule`、`internal/ports/scheduler`、`internal/app/schedule`、`internal/adapters/scheduler/filecron` 和 `internal/adapters/tools/builtin/scheduler`；顶层旧包不再保留。
- `common`：root 包已拆除；应用目录在 `internal/app/appdata`，会话上下文在 `internal/domain/session`，模型重试在 `internal/runtime/retry`，运行期资源清理在 `internal/runtime/resources`，输入历史在 `internal/adapters/storage/file/inputhistory`。`common/atomicfile`、`common/pathguard`、`common/typeutil` 作为明确基础包暂时保留。
- `server/handler/taskstream`：已上移为 `internal/app/chat/taskstream`，server handler 只持有和转发用例层 stream。

## 迁移顺序

1. 建立 `internal/domain` 和 `internal/ports`，移动纯类型和接口。
2. 建立 `internal/app/chat`，把 Web/CLI/channel/scheduler 重复执行循环合并。
3. 建立 `internal/app/agent`，接管 agent spec、team spec、runner resolution。
4. 建立 `internal/adapters/runtime/eino`，移动所有 Eino 依赖。
5. 建立 storage adapters，替换 history/session/checkpoint/memory 的直接文件访问。
6. 建立 tool registry，替换 `tools/tools.go` switch。
7. 重构 scheduler：工具只创建计划，执行由 schedule use case 触发 chat service。（已完成）
8. 收缩入口层：HTTP、CLI、channel 只保留协议转换。
9. 删除旧 facade、旧全局变量和旧目录。

## 验证门禁

每完成一个大模块必须执行：

```bash
make check
make native
```

同时补充架构边界测试：

- `internal/domain` 不得 import `internal/app`、`internal/adapters`、第三方 runtime SDK。
- `internal/ports` 不得 import `internal/app`、`internal/adapters`。
- `internal/app` 不得 import `internal/adapters`。
- 只有 `internal/adapters/runtime/eino` 可以 import `github.com/cloudwego/eino*`。
- `server`、`cli`、`channels` 不得 import runtime adapter。
- `tools` 不得 import `engine` 或 `app/chat`；工具只能通过端口提交用例请求。

提交粒度按大模块切分，提交信息使用 Conventional Commits 中文说明。
