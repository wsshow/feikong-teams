# 架构设计

fkteams 将产品用例与智能体执行分开：应用管理会话、任务和权限，AgentKit 管理模型与工具的执行。CLI、Web 对话、消息通道和后台调度复用同一条运行链路。OpenAI 兼容 API 是模型代理入口，复用模型提供者，不执行团队用例。

## 依赖边界

```text
cmd/fkteams
    └── bootstrap               创建并连接实例
          ├── adapters          协议、SDK、文件系统与外部服务
          ├── app               产品用例
          └── runtime           与产品和 SDK 无关的运行基础能力
                └── ports       能力契约
                      └── domain 领域数据
```

这是一组依赖约束，不要求每次调用经过所有目录：

| 层 | 负责 | 允许依赖 |
| --- | --- | --- |
| `domain` | 消息、事件、历史、会话等数据 | 标准库、无运行态的值类型库（如 UUID） |
| `ports` | 模型、工具、执行器、存储等契约 | domain |
| `runtime` | 事件、hooks、权限、路径校验、回合生命周期 | ports、domain |
| `app` | 对话、智能体目录、记忆、调度等用例 | runtime、ports、domain |
| `adapters` | 外部技术与协议实现 | app、runtime、ports、domain |
| `bootstrap` | 创建依赖并显式注入入口 | 以上各层 |

AgentKit、Eino 及模型 SDK 只允许在 `internal/adapters/runtime/agentkit/` 中出现。端口不使用 SDK 类型别名，应用测试可以注入 fake runtime。入口不能直接创建智能体执行器或自行写历史。

不建立进程级默认实例，不通过 `init()`、空白 import 或包级 setter 注册依赖。需要的能力由应用实例持有并注入。只有一个默认执行实现时，组合根直接创建它，不再维护未被消费的运行时注册表。

## 一次对话的链路

```text
CLI / HTTP / Channel / Scheduler
    → app/chat.Service
    → runtime/turn.Executor
    → ports/runtime.Runner
    → adapters/runtime/agentkit
    → AgentKit
```

`chat.Service` 装配输入、steering、审批、事件接收器和会话生命周期。`turn.Executor` 执行 before/after hooks，调用 Runner，并保证收尾回调收到结果和错误。它没有额外的 core、engine 或 runLoop 转发层。

Runner 每次运行创建独立的 AgentKit 会话，将输入历史交给库，在同一会话中完成 HITL 恢复，最后清理检查点并关闭会话。检查点 ID 由库生成，不再作为应用运行参数透传。缓存的是智能体声明和 Runner，不是聊天历史；同一 Runner 可服务不同会话，不共享对话状态。

SSE 与 WebSocket 共用 `chat.Service.RunQueued`：每轮执行、消费下一条队列项、最终收尾只有一份流程。空队列会原子关闭入队；只有历史和 metadata 持久化成功后才发布 completed。同步 HTTP 请求也向同一任务注册表申请会话，禁止与流式任务同时改写同一份历史；同步请求不接受追加队列。

`TurnRequest.History` 提供记录、定稿和 `Flush()` 契约。`chat.Service` 是每回合唯一的记录与定稿所有者：记录发生在展示回调之前，steering 也经过该管线；CLI/JSON/TUI 视图只接收事件。`SessionLifecycle` 只在历史提交后更新 metadata 和记忆，不再重复保存历史。执行取消与收尾失败保留独立的错误来源：只有纯取消映射为 cancelled，任何持久化或收尾失败均映射为 error。每次调用使用独立 RunID，SessionID 只标识长期会话。

模型错误会同时通过事件与返回值传递，不能只发错误事件后返回成功。取消和审批失败也进入用例收尾路径。事件接收器返回错误时取消执行并保留原始错误。

## 智能体开发

智能体以 `app/agent/catalog/common.Definition` 声明：

```go
def := common.Definition{
    Name:        "reviewer",
    Description: "审查代码并给出有证据的改进建议",
    Instruction: reviewerPrompt,
    Profile:     common.ProfileWorkspace,
    ToolNames:   []string{"file", "git"},
}
agent, err := common.BuildAgent(ctx, def)
```

`BuildAgent` 完成模型、提示词、工具、策略和中间件解析，直接生成一个 `ports/runtime.ChatAgentConfig`。不再保留内容重复的 `ResolvedAgent`，也不为无状态步骤建立 Resolver、Assembler 空类型。

| Profile | 用途 |
| --- | --- |
| Bare | 显式模型和工具，独立后台任务 |
| Workspace | 工作区基础工具和上下文 |
| Full | 工作区、摘要和技能 |
| Team | 协调者使用的完整能力组合 |

内置智能体目录保留定义与独立提示词。新内置角色在 `builtinAgentSpecs()` 注册一次元信息和默认定义。自定义角色和内置角色的配置覆盖都由 `buildConfiguredDefinition` 生成相同的 Definition，再调用 `BuildAgent`。模型引用保留完整模型配置（包括额外请求头）；未知 Profile 在构建时返回错误。

轻量任务使用 `app/agent/standalone.Service` 的 `RunText`、`StreamText` 或 `Run`，不用自行拼装历史、工具或执行循环。

工具组中的工具使用同一份声明列表，处理函数保留明确的请求与响应类型：

```go
func GetTools() ([]runtimeport.Tool, error) {
    return runtimeport.NewTools([]runtimeport.ToolDefinition{
        {Name: "read_report", Description: "读取报告", Handler: readReport},
        {Name: "save_report", Description: "保存报告", Handler: saveReport},
    })
}
```

`NewTools` 校验名称与重复项，并从处理函数推导 schema；构建失败不会返回部分列表。`ToolResolveContext.Config` 和 `SSH` 使用统一的配置类型，避免隐式类型断言。工具组只需在组合根注册一次，已有工具组的成员不需要扩充中央 switch。

SDK 中间件使用显式绑定运行时的句柄，适配器校验 backend 与原生类型；它不是仅凭 `Name()` 就能实现的通用扩展接口。可移植的业务扩展应实现带结构化 payload 的 hooks。

## AgentKit 适配器

依赖固定为 `github.com/wsshow/agentkit v1.5.0`；其要求的 Go 版本是 `1.25.14`，底层 Eino 为 `v0.9.19`。

适配器按职责组织：

- `agent.go`、`factory.go`：可复用声明、能力转换、Runner 创建。
- `agent_tool.go`：把成员工具声明转换为 AgentKit `SubAgentConfig`。
- `runner.go`：会话生命周期、审批恢复和库事件转换。
- `model_events.go`：补齐产品协议所需的消息 ID、工具参数增量和多模态输出。
- `deep.go`：组合规划、工作区、命令和成员能力。
- `model.go`、`message.go`、`tool.go`、`middleware.go`：领域契约与 SDK 的转换。
- `engine/`：组合项目中间件与上述适配能力，避免中间件和核心适配互相导入。
- `providers/`、`middlewares/`：具体模型提供者和产品扩展。

普通智能体、团队成员、Deep 和 dispatch 后台子任务均由 AgentKit 执行。项目不再直接创建 ADK Runner、ChatModelAgent、LoopAgent 或 DeepAgent。

团队成员使用库提供的独立上下文和 HITL 能力，成员输出带稳定的父工具调用 ID。项目不再维护 ADK 事件指针的全局关联表、成员事件转发协程或错误包装智能体。

采用 AgentKit 的默认成员策略：最多 8 次委派、4 个不同成员并行、单次委派 10 分钟；同一个成员不能同时接受多个委派。当前产品目录是单层团队；不通过成员工具递归嵌套团队。

圆桌讨论按配置顺序逐个执行成员，把已有讨论传给下一位；`max_iterations = 0` 保留持续讨论直到取消的行为。Deep 以同一套执行模型组合规划、文件和命令能力，直接暴露成员工具，不再使用另一个预构建框架的 `task` 路由。工作区路径必须显式传入；`shell.streaming` 通过工具进度事件报告输出。

## 状态与事件

每种状态只有明确的所有者：

| 状态 | 所有者 |
| --- | --- |
| 会话标题、收藏、当前角色等 metadata | session store/service |
| 可恢复的聊天记录 | history transcript |
| 当前模型与工具执行、中断恢复 | 当前运行的 AgentKit 会话 |
| 待处理 steering、follow-up 与顺序 | `app/chat/taskstream` |
| 长期记忆 | memory service/store |
| 定时任务和执行记录 | schedule service/store |

AgentKit 的会话持久化和内置消息队列不与现有产品状态并行启用。传给库的历史是本次运行输入；应用仍以 transcript 为持久化事实来源。

`chat.QueueService` 串行编排同一会话的队列修改、消费、持久化和通知。`taskstream.Queue` 作为事务草稿，只有保存成功才替换可见状态；离线编辑同样使用该用例，加载失败阻止启动，保存失败返回错误并保留原输入。附件按内容寻址，未提交的附件写入不会覆盖已提交版本。

产品队列需要稳定的 `queue_id`、编辑、删除、排序和断线恢复，因此保留在 taskstream。运行中的普通消息进入 follow-up；显式 steer 由 `SteeringSource` 在下一次模型调用前消费。已经消费的条目不可修改，队列变更通过 `queue_updated` 同步。终端暂停时将未消费 steering 回填输入框。

事件转换链路：

```text
模型观察 / AgentKit 工具事件
    → domain/event.Event
    → app 事件管线：session、turn、sequence、history
    → transcript / transport DTO
```

库的公共事件没有工具参数分片、产品消息 ID 和完整的多模态输出字段，所以适配器观察同一次模型响应以补齐这些信息，完整消息在产品中间件修复完成后发布；它不负责执行工具或推动模型循环。工具参数第一次出现就确定调用 ID，SDK 执行、`message_delta(tool_args)`、`message_end.tool_calls[]` 和 `tool_start/update/end` 使用相同的 `tool_call_ref`。

增量进入领域协议后统一使用 `Content`，不增加第二个 Delta 字段。UI DTO、历史投影和运行事实保持分层；显示需求不能改变持久化事实模型。

## 组合、配置与持久化

`bootstrap.ExecutionDependencies` 持有一次应用的运行时、注册表和配置执行边界。`Command()` 注入 CLI 的服务装配与服务端启动函数；`RunServer()` 组合记忆、调度、HTTP 和消息通道。HTTP 服务只实现传输生命周期，不反向导入 bootstrap。关闭与重启由实例级 `lifecycle.Control` 管理；共享 MCP 连接在所有服务停止后由组合根清理。

`config.Get()` 继续通过 `atomic.Pointer` 发布快照。HTTP、消息通道、调度以及辅助工具执行持有同一个 `ExecutionGate` 读取租约，覆盖依赖构建、执行和收尾。配置更新只在空闲时取得写租约，有运行任务时返回 HTTP 409。先完成校验和可失败的记忆模型构建，再保存配置并重载智能体目录、清理 Runner/MCP 缓存、重置通道 Runner 和记忆模型，最后释放租约。这避免执行中混用新配置与旧依赖；监听地址、后台服务启停和连接配置仍需重启应用。

业务数据的存储位置与契约如下：

| 数据 | 应用契约 | 文件实现 |
| --- | --- | --- |
| 对话历史 | `chat.SessionHistory.Flush` | `adapters/storage/file/history` |
| 长期记忆 | `ports/memory.Store` | `adapters/storage/file/memory` |
| 调度结果 | `ports/scheduler.ResultStore` | `adapters/scheduler/filecron` |
| 待执行队列 | `domain/message.QueuedMessage` | `adapters/storage/file/queue` |
| 分享授权 | 具体分享存储 API | `adapters/storage/file/share` |

分享存储复用同一个条目存储实现，统一并发、写入回滚与副本读取，具体条目负责自己的编码和校验。队列存储使用固定根目录读写附件。记忆加载失败必须返回错误，写入失败保留脏状态以供重试。服务停止和 cleanup 错误也从应用入口返回，不能只记录日志后报告成功。

## 前端职责

聊天的 `timeline.ts` 负责事件去重与展示投影，`MessageList.tsx` 负责组件渲染。配置的 `configModel.ts` 负责默认值、合并和保存格式；智能体编辑、集成编辑和共用字段按职责放在少量组件文件中。SSE、快照和历史回放共用 `decodeChatEvent(s)`，未知事件类型在 API 边界过滤，应用事件类型不再用 `| string` 扩宽。

## 扩展点

- **工具组**：通过 `app/tools.ToolGroupRegistry` 注册。IO 和 SDK 实现放在 `adapters/tools`，依赖经 `ToolResolveContext` 注入。策略由 `toolpolicy.ClassifyTools` 标记。
- **模型**：在 `adapters/model/providers` 的实例工厂注册创建和列表能力，SDK 实现放在运行适配器中。
- **Hooks**：契约在 `ports/hooks`，实现总线在 `runtime/hooks`。payload 必须是明确结构体；未注入总线时不执行 hook。
- **消息通道**：adapter 导出接收 FactoryRegistry 的 Register，由 bootstrap 显式装配，通过 Bridge 调用应用用例。
- **后台服务**：实现 `app/lifecycle.Service` 的 Name、Start、Stop。按注册顺序启动、按 LIFO 停止。
- **配置**：通过 `config.Get()` 获取热重载快照；新增配置同步维护示例。

## 安全与验证

工作区路径与符号链接检查、危险命令检测、工具权限元数据和 HITL 审批不因迁移而绕过。消息通道明确采用自动审批注册表，这是该入口的产品策略。没有显式审批策略的回合以及后台调度保留默认拒绝；自动审批不改变工作区路径限制和工具元数据。运行结束或取消后清理该运行的检查点。

`internal/architecture` 用层级白名单矩阵约束内部依赖方向（测试仅额外允许 testmodel）；需要组合根的集成测试放在 bootstrap。适配器边界测试限制 SDK 所属位置。运行回归测试覆盖流式工具关联、成员并行问答和恢复、工具 schema 传递、会话隔离、取消、事件接收器失败、默认拒绝、检查点清理、圆桌顺序及 Deep 工作区逃逸。

跨包或运行行为变更运行 `make check`；并发变更另运行相关包的 `go test -race`。生成的 `web/dist` 与 `release` 不提交。
