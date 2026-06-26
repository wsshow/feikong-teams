# 架构设计

本文档定义当前主线架构。项目仍处于 `0.x` 阶段，内部包、接口和目录结构可以继续调整；调整目标不是“迁移越多越好”，而是让核心边界稳定、职责清晰、运行可靠，并为替换 runtime、模型 provider、存储、调度和工具实现留下明确入口。

## 架构原则

- 核心用例优先：HTTP、CLI、消息通道、调度器等入口只做协议转换和用户交互，不直接拼装 runner、history、memory、approval、hooks 或 runtime。
- 端口驱动：核心用例依赖接口和领域模型；Eino、HTTP、CLI、文件存储、MCP、Git、SSH、定时器等具体技术放在 adapter。
- 单向依赖：`domain`、`ports`、`app`、`runtime` 不反向依赖 `adapters`；`bootstrap` 是组合根，负责把配置、adapter 和 service 接起来。
- Runtime 可替换：Eino 是默认 runtime adapter，不是核心架构的一部分。其他核心只面向 `internal/ports/runtime`。
- 稳定性优先：session、history、checkpoint、memory、task queue、schedule result 等状态必须通过明确 store/repository/service 管理，避免入口层散落写入。
- 事件协议稳定：运行事实事件、用例事件管线和传输展示 DTO 分层转换，不在入口层重复拼装核心事件。
- 不做形式主义隔离：成熟、无状态、值对象级别的小依赖可以直接使用，例如 `github.com/google/uuid`。需要隔离的是框架、协议、外部系统、IO、存储、网络客户端、有生命周期或需要替换的技术实现。

## 目录结构

```text
cmd/fkteams/
  main.go                    # 命令入口

internal/domain/             # 领域模型和值对象
  event/
  history/
  memory/
  message/
  schedule/
  session/

internal/ports/              # 外部能力接口和核心契约
  hooks/
  memory/
  runtime/
  scheduler/
  storage/
  tools/

internal/app/                # 应用用例层
  agent/
  appdata/
  appstate/
  chat/
  config/
  lifecycle/
  memory/
  schedule/
  skill/
  tools/
  version/

internal/runtime/            # 运行时无关的执行内核和基础能力
  atomicfile/
  checkpoint/
  env/
  events/
  hooks/
  log/
  mdiff/
  model/
  pathguard/
  registry/
  resources/
  retry/
  turn/
  typeutil/

internal/adapters/           # 技术实现和协议转换
  model/
  runtime/eino/
  scheduler/filecron/
  storage/file/
  storage/memory/
  tools/builtin/command/
  tools/builtin/doc/
  tools/builtin/excel/
  tools/builtin/fetch/
  tools/builtin/file/
  tools/builtin/git/
  tools/builtin/search/
  tools/builtin/scheduler/
  tools/builtin/script/
  tools/builtin/ssh/
  tools/builtin/todo/
  tools/mcp/
  transport/channel/
  transport/cli/
  transport/http/

internal/bootstrap/          # 组合根
  environment/
  runtimes/
  services/
  tools/

web/
docs/
```

## 层级职责

| 层级 | 职责 | 边界 |
| ---- | ---- | ---- |
| `domain` | 业务模型、值对象、上下文中的领域标识 | 不依赖 app、adapter、配置、框架 SDK、传输 SDK；允许稳定无状态的小工具库 |
| `ports` | runtime、storage、scheduler、tools、hooks 等接口契约 | 不依赖 app、adapter 和 runtime 实现；不暴露具体 SDK 类型 |
| `app` | 用例编排、事务边界、状态流转、策略选择 | 不 import adapter、Eino、Gin、pterm 等具体实现 |
| `runtime` | 与 Eino 无关的执行内核、事件、hooks、checkpoint、模型注册表等 | 不依赖 adapter、HTTP/CLI handler 或 Eino |
| `adapters` | Eino、模型 provider、HTTP、CLI、channel、文件存储、MCP、Git、SSH、调度器等实现 | 可以依赖 domain、ports 和第三方 SDK；不得被 app/domain 反向依赖 |
| `bootstrap` | 初始化环境、注册 runtime/provider/tool/service | 唯一主动连接各层的 composition root；注册必须由命令入口或服务启动显式调用，禁止用 `init()` 自动装配 |

## 核心用例中轴

所有入口统一调用 `internal/app/chat.Service` 或对应应用服务。Web、CLI、消息通道、定时任务不直接调用 engine 或具体 runtime；runner 创建由 `internal/app/agent` 完成，并通过 context 注入的 `runtime.Engine` 与组合根连接。

```go
type Service interface {
    RunTurn(ctx context.Context, req TurnRequest, opts ...TurnOption) (*runtime.RunResult, error)
}
```

`chat.Service` 负责：

- 将入口层能力转换为稳定的 turn 执行选项：approval、ask、steering、事件记录、history sink、hooks、scheduler service 和 context hook。
- 统一调用 `internal/runtime/turn.Session`，入口层不得直接散落这些 runtime context key。
- 通过 `SessionLifecycle` 统一保存 history、更新 metadata、记录取消/错误、提取或 flush 长期记忆。
- 通过 `internal/app/chat/taskstream` 提供运行中事件流、follow-up 队列、steering 队列、interrupt/ask 响应和断线续接能力。

入口层只负责：

- HTTP handler：JSON、SSE、WebSocket DTO 转换。
- CLI：终端输入、展示、快捷键和当前 CLI `Session` 实例生命周期。
- Channel：平台消息转换、回复发送和当前 Bridge 实例生命周期。
- Scheduler：时间触发和结果归档。

智能体目录由 `internal/app/agent/catalog.Registry` 实例持有，命令入口创建后通过 context 注入 HTTP、CLI、channel 和 scheduler。配置更新只 reload 当前入口持有的 registry；catalog 包不提供进程级 `Registry`、`GetRegistry()` 或 `ReloadRegistry()` 默认实例。

## Runtime 边界

Runtime 端口必须足够小，不能暴露 Eino 概念。

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

`internal/adapters/runtime/eino` 是唯一允许 import `github.com/cloudwego/eino*` 的目录。其他 runtime 只要实现 `internal/ports/runtime` 即可接入。

## Tool 边界

工具系统通过注册表管理目录、工厂和策略：

```go
type ToolRegistry interface {
    Register(group ToolGroupSpec, factory ToolFactory) error
    Resolve(ctx context.Context, refs []ToolGroupRef) ([]Tool, []ToolPolicy, error)
    Catalog(ctx context.Context) ([]ToolGroupInfo, error)
}
```

值得迁入 adapter 的工具实现：

- 依赖外部协议或 SDK，例如 MCP、Git、SSH/SFTP、HTTP 抓取、搜索 provider。
- 依赖文件格式引擎或客户端生命周期，例如 Excel、文档读取、远程连接。
- 需要配置、资源清理、连接池、缓存、替换 provider 或外部 IO。

不值得迁移的内容：

- 稳定、无状态、值对象级别的小依赖，例如 UUID 生成。
- 迁移后只是换目录，没有减少耦合、没有清晰接口、没有替换收益的代码。
- 为了“纯净”重写成熟库。

当前已经明确隔离的工具实现：

- `internal/app/tools` 只保留工具组注册表、目录查询、MCP provider 门面、审批策略和无需外部 IO 生命周期的应用级能力；默认工具组不在 app 层内建，也不保留进程级默认注册表或 MCP provider。
- `internal/bootstrap/runtimes` 显式注册 runtime engine、MCP tool provider 桥接和模型 provider；运行时模型工厂注册表和 adapter 模型 provider 注册表都由组合根创建并注入入口上下文，`internal/runtime/model` 与 `internal/adapters/model/providers` 不提供可变的进程级默认注册表；禁止用空白 import 或 `init()` 完成 provider 装配。
- `internal/bootstrap/tools` 是唯一默认工具组组合入口，负责创建工具注册表实例，注册 file、todo、ask、command、uv、bun、excel、doc、fetch、search、git、ssh、scheduler 等工具组，并由命令入口注入 HTTP、CLI、channel 和 scheduler 上下文。
- `tools/mcp`：MCP client、缓存和工具组 provider 位于 `internal/adapters/tools/mcp`；app 工具层只依赖 `internal/ports/tools.MCPProvider`。MCP client 转 runtime tool 的桥接由 `internal/bootstrap/runtimes` 连接具体 runtime adapter，不进入 ports 契约。
- `tools/command`、`tools/script/uv`、`tools/script/bun`：进程执行和脚本运行时实现位于 `internal/adapters/tools/builtin/*`；后台 tasker 通过隐藏工具组 `command_reject` 使用自动拒绝危险操作的命令策略。
- `tools/file`、`tools/todo`：工作区文件 IO 和会话 todo 持久化位于 `internal/adapters/tools/builtin/*`，由 bootstrap 注入工作区和会话目录。
- `tools/scheduler`：`schedule_*` 工具位于 `internal/adapters/tools/builtin/scheduler`，只委托 `internal/app/schedule`；工具执行时从 context 读取调度用例服务，不依赖进程级默认实例。调度器实现位于 `internal/adapters/scheduler/filecron`。
- `tools/git`：go-git 实现位于 `internal/adapters/tools/builtin/git`，由 `internal/bootstrap/tools` 注册。
- `tools/ssh`：SSH/SFTP 实现位于 `internal/adapters/tools/builtin/ssh`，配置读取、连接创建和资源清理由 `internal/bootstrap/tools` 组合。
- `tools/excel`、`tools/doc`、`tools/fetch`、`tools/search`：文件格式引擎、HTTP 抓取和搜索实现位于 `internal/adapters/tools/builtin/*`，由 `internal/bootstrap/tools` 注册。

## 状态与存储

状态能力按用途拆分，不在入口层散落读写：

- `SessionStore`：session metadata、title、status、timestamps。
- `HistoryStore`：结构化消息、事件、摘要。
- `CheckpointStore`：runtime checkpoint，端口契约位于 `internal/ports/storage`。
- `MemoryStore`：长期记忆。
- `TaskStore`：schedule task、result、history。
- `StreamHub`：运行中事件流和队列快照。

HTTP、CLI、channel 等入口必须由各自的 server/session/bridge 实例持有 `SessionHistoryManager`、history dir、stream manager、runner cache、HTTP runtime cache/store 和 scheduler service；禁止在文件历史 adapter、HTTP handler 或 app service 中恢复跨入口共享的全局运行态。`internal/runtime/checkpoint` 只提供内存存储、命名空间等 runtime 无关实现。文件系统只是 adapter；核心不直接拼路径，也不依赖具体文件历史实现。

## Hooks

Hooks 是用例和运行时之间的稳定扩展边界：

- before/after turn
- before/after model request
- before/after tool call
- on event
- before/after memory injection
- before/after schedule execution

hook payload 使用 `internal/ports/hooks` 中的明确结构体，并统一实现 `hooks.Payload` 契约；`Invocation` 和 `Result` 只能携带 `hooks.Payload`，不能退回裸 `any`。`internal/runtime/hooks` 负责总线实现、payload 与 hook point 匹配校验、超时/错误策略、context 绑定和便捷调用。HookBus 必须由用例或组合根显式传入；未传入时不执行 hook，不提供可注册的全局默认实例。

中断 runtime 和模型工厂注册表必须通过 context 或运行服务依赖显式传入；`internal/ports/runtime` 不提供进程级默认实例、全局注册函数或兜底注册表，`internal/runtime/model` 只提供实例化注册表和 context 绑定。HTTP、CLI、channel 和 scheduler 等入口由组合根注入默认 Eino interrupt runtime 与模型注册表，测试通过 `runtime.WithInterruptRuntime` / `model.WithRegistry` 注入假实现，避免跨用例污染。

调度用例服务必须由 `internal/bootstrap/services.SchedulerService` 创建并显式注入 HTTP runtime、CLI session、channel bridge 和后台 tasker context；`internal/app/schedule` 只提供用例服务和 context 绑定函数，不提供 `Default` / `SetDefault` 形式的进程级服务实例。

## 事件分层

事件分三层：

1. `internal/domain/event`：运行时和用例的事实事件。
2. `app` event pipeline：补齐 session、turn、sequence、history metadata。
3. `transport` event DTO：SSE、WebSocket、CLI、channel 展示格式。

`internal/runtime/events` 负责事件构造、分发、协议校验和友好错误归一化，只依赖领域事件、领域消息、运行时端口和 hooks。历史记录保存 domain/app 事件，不保存入口层展示 DTO。

事件模型仍保留单一 `domain/event.Event` 结构体，但 runtime 事件出口必须通过 `runtime/events.ValidateEventContract` 维护最低语义字段和工具调用身份约束；后续只有在字段继续膨胀并影响调用方时，才进一步拆分 typed payload。

## 当前重点

后续不再以“继续搬目录”为目标，而是围绕下面几条主线推进：

- 继续收紧 Eino 隔离，确保核心 runtime、app、domain、ports 不出现 Eino 类型。
- 完善 `app/chat` 中轴，让 HTTP、CLI、channel、scheduler 的执行路径持续收敛。
- 梳理状态和存储接口，减少全局变量、散落文件访问和入口层状态拼装。
- 完善 hooks payload、事件协议和 mock runtime 测试，提升可验证性。
- `app/chat` 必须保留不依赖 Eino 的 fake runtime 端到端测试，覆盖 hooks、interrupt、tool call 和事件记录/发布。
- 对工具迁移保持克制，只迁移真正带外部协议、IO、生命周期或替换价值的实现；成熟外部标准库不做无收益重写。
- runtime registry 和 bootstrap 注册路径必须显式返回 error，不允许以 panic 作为配置或注册错误的常规控制流。

## 验证门禁

每完成一个大模块必须执行：

```bash
make check
make native
```

同时维护架构边界测试：

- `internal/app` 不得 import `internal/adapters`、Eino、Gin、pterm。
- `internal/ports` 不得 import `internal/app` 或 `internal/adapters`，也不得暴露具体 SDK 类型。
- `internal/ports` 不得 import `internal/runtime`；端口契约放在 `internal/ports/*`，实现放在 `internal/runtime` 或 `internal/adapters`。
- 只有 `internal/adapters/runtime/eino` 可以 import `github.com/cloudwego/eino*`。
- HTTP、CLI、channel 入口不得绕过 app use case 直接调用 runtime adapter。
- 工具不得反向调用 `app/chat` 或入口展示层；需要调用用例时通过明确 service/port。

提交粒度按大模块切分，提交信息使用 Conventional Commits 中文说明。
