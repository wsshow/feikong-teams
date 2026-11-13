# 架构设计

fkteams 由应用用例管理会话、任务和权限，AgentKit 负责模型与工具执行。CLI、Web、消息通道和调度复用同一条对话链路；OpenAI 兼容 API 只代理模型请求。

## 分层与目录

```text
cmd → bootstrap → adapters → app / runtime → ports → domain
```

这是依赖方向，调用不必逐层经过所有目录。

| 目录 | 职责 | 允许依赖 |
| --- | --- | --- |
| `internal/domain` | 消息、事件、历史、会话等领域数据 | 标准库、无运行态的值类型库 |
| `internal/ports` | 模型、工具、存储等契约 | domain |
| `internal/runtime` | 回合执行、事件、hooks、权限与路径校验 | ports、domain |
| `internal/app` | 对话、智能体、记忆、任务、调度等用例 | runtime、ports、domain |
| `internal/adapters` | 协议、SDK、文件系统与外部服务 | app、runtime、ports、domain |
| `internal/bootstrap` | 创建应用实例并装配依赖 | 以上各层 |
| `cmd/fkteams` | 连接组合根与 CLI 命令 | bootstrap、CLI 入口 |
| `web` | React 界面；构建后嵌入 Go 二进制 | HTTP 与展示协议 |

AgentKit、Eino 及模型 SDK 集中在 `adapters/runtime/agentkit`，契约不暴露 SDK 类型。注册表和服务由应用实例持有并显式注入，不通过可变全局实例、`init()` 或空白 import 注册。`app/execution.Dependencies` 传递执行所需能力；后台任务绑定自己的上下文，避免继承 HTTP 请求的取消信号。

## 对话执行

```text
CLI / HTTP / Channel / Scheduler
    → app/chat.Service
    → runtime/turn.Executor
    → ports/runtime.Runner
    → adapters/runtime/agentkit
    → AgentKit
```

- `chat.Service.RunQueued` 统一恢复会话、执行回合、消费队列和收尾。入口只准备输入、审批与事件接收器。
- `turn.Executor` 执行回合 hooks 并调用 Runner。未配置中断处理器时固定拒绝审批；错误通过事件和返回值传递。
- Runner 每次运行创建独立 AgentKit 会话，在该会话内完成 HITL 恢复，结束后清理检查点。可以缓存声明和 Runner，不共享聊天历史。
- `chat.Service` 经 `TurnRequest.History` 统一记录事件、定稿和保存，历史提交后才更新 metadata 和记忆并发布终态。持久化或收尾失败必须报告 error；纯取消报告 cancelled。
- 同一会话只有一个执行所有者。同步 HTTP、CLI 与消息通道同样占用会话，避免其他入口并发改写历史。

轻量后台调用使用 `app/agent/standalone.Service`，继承回合 Hook、RunID 和中断默认值。文本结果由 `chat.AssistantTextCollector` 汇总，避免调用方自行拼接增量或成员输出。

## 状态所有权

| 状态 | 所有者 |
| --- | --- |
| 标题、收藏、模式、当前智能体 | session service/store |
| 对话事实与模型历史 | history transcript / recorder |
| Web 消息分段、工具、成员及交互状态 | `app/chat/presentation` |
| 当前模型执行与中断恢复 | 本次 AgentKit 会话 |
| steering、follow-up 与待消费顺序 | `chat.QueueService` / `taskstream` |
| 长期记忆 | memory service/store |
| 跨回合目标与进展 | task service/store |
| 定时安排、执行状态和结果通知 | schedule service/store |
| 本地进程与日志 | process service / local adapter |

AgentKit 接收本次运行的历史输入，不另外启用其会话持久化或内置消息队列。

`QueueService` 统一编排队列恢复、编辑、消费、保存和通知。每项有稳定 `queue_id`，保存成功后才发布可见状态；已消费项不可修改。普通运行中输入进入 follow-up，显式 steering 在下一次模型调用前消费。取消后未消费输入保留至下次运行，终端暂停时将未消费 steering 回填输入框。

## 历史与展示

```text
模型观察 / AgentKit 事件
    → domain/event.Event
    → chat 记录管线：运行身份、会话序号、历史
    → transcript：原始信封与规范消息
    → app/chat/presentation：Snapshot / Update
    → HTTP、SSE、WebSocket 与 Web
```

记录管线串行分配会话序号、保存事实并发布事件，问答与审批也经过同一管线。transcript 保留原始事件信封和规范 `Message`，`OutputParts` 标记文本、思考与工具调用的交错顺序。取消或失败时保存剩余草稿；不完整工具参数只供展示，不进入模型上下文。成员内部记录独立保存，主智能体通过父工具结果接收成员结论。

`presentation.Projector` 是 Web 展示的唯一生成者，实时事件和历史恢复复用它。客户端只应用 `view` 快照和 `view_update`，不推测顺序、成员归属或工具成败。快照与 `next_offset` 在同一锁内读取；重连先取快照，再按游标订阅。`processing_end` 携带明确终态及最终展示更新，客户端不能收到原始 error 就提前结束订阅。

工具链通过稳定 `tool_call_ref` 关联参数增量、完整消息和执行事件。领域增量统一使用 `Content`；消息与工具调用的供应商元数据跨 SDK 边界深拷贝，恢复模型上下文时保留。旧历史缺少规范事件信封时，Web 返回 HTTP 409 并建议新建会话，保留原记录。协议细节见[事件协议](events.md)和[流式任务 API](api/stream.md)。

## 智能体与扩展

内置智能体以 `app/agent/catalog/common.Definition` 声明，经 `common.BuildAgent()` 生成运行契约；元信息与默认定义在 `builtinAgentSpecs()` 注册。配置覆盖与自定义角色走同一构建流程，提示词模板独立保存。

| Profile | 用途 |
| --- | --- |
| Bare | 显式模型和工具的独立任务 |
| Workspace | 工作区工具和上下文 |
| Full | 工作区、摘要和技能 |
| Team | 协调者所需完整能力 |

普通智能体、团队、Deep 和 dispatch 子任务均使用 AgentKit。成员通过库的独立上下文和 HITL 执行；默认最多委派 8 次，4 个不同成员并行，单次 10 分钟，同一成员不能同时接受多个委派。团队目录为单层。圆桌按配置顺序传递讨论内容，`max_iterations = 0` 时持续运行至取消。

| 扩展 | 接入位置与约束 |
| --- | --- |
| 工具组 | `app/tools.ToolGroupRegistry` 注册，使用 `ports/runtime.NewTools` 声明明确的请求/响应类型；IO 实现在 `adapters/tools`，由 bootstrap 注入 |
| 工具策略 | `runtime/toolpolicy.ClassifyTools` 标记只读、破坏性和审批元数据 |
| MCP | 经 `ports/tools.MCPProvider` 注入，不让应用依赖 MCP SDK |
| 模型 | `adapters/model/providers` 的实例工厂注册创建与列表能力，SDK 实现留在运行适配器 |
| Hooks | `ports/hooks` 定义结构化 payload，`runtime/hooks` 提供总线；可移植业务扩展优先使用 hooks |
| 消息通道 | 平台 adapter 显式 `Register(FactoryRegistry)`，经 Bridge 调用应用用例 |
| 后台服务 | 实现 `app/lifecycle.Service` 的 `Name`、`Start`、`Stop` |

个人助手是固定会话中的标准 `workspace_assistant`，复用同一套工具、队列、历史和审批；跨回合目标独立于回合状态。交互界面源码随消息保存，表单提交作为普通用户消息入队，详见[助手交互界面](assistant-ui.md)。

## 工具与进程边界

组合根注入共享 `resources.KeyedLocks`，文件、Git、Excel 按工作区协调读写，TODO 按会话快照协调完整事务。文件操作经 `os.Root` 约束；文档解析使用大小受限的私有快照；覆盖写与下载采用原子替换。具体限制见[安全说明](security.md)。

`app/process.Service` 持有本地命令的任务归属、期限、配额和完成投递，adapter 负责执行和有界日志。任务按工作区与会话隔离，通过稳定任务 ID 操作；不按用户提供的 PID 发信号。等待预算耗尽或显式后台返回后，进程归应用管理，后续回合取消不会终止它。

完成结果先持久化，再投递到 `QueueService`，在回合结束后串行消费；空闲或取消的会话留待下次运行，不自行启动模型。队列与投递回执原子保存，失败重试；重启后未完成进程标为 interrupted，不接管旧 PID 或重跑命令。

## 配置、存储与关闭

`config.Get()` 通过 `atomic.Pointer` 发布快照。HTTP、通道和调度共享 `ExecutionGate`，读取租约覆盖依赖构建、执行和收尾；配置写入只在空闲时取得写租约，否则返回冲突。校验与保存后统一重载目录、Runner/MCP 缓存、通道 Runner 和记忆开关。监听地址、服务启停和连接配置仍需重启。

组合根在业务启动前取得应用目录锁，通过 `ports/storage.Documents` 注入 SQLite 文档存储。历史追加片段、metadata、队列、记忆、任务、分享、调度和脚本状态存入数据库，附件、报告与工作区文件保留在文件目录。旧文件实现用于兼容读取和测试；首次升级由 `app/migration.Service` 备份、校验并事务导入，详见[存储与升级](storage.md)。

服务按注册顺序启动、按 LIFO 停止。应用取消传递给所属任务，`Stop` 等待真实退出，再执行 Cleanup 并释放 MCP、数据库和目录锁。`ShutdownTimeout` 只限制调用方等待，后台继续排空；嵌入方使用 `WaitShutdown(ctx)` 确认资源释放。停止与清理错误必须返回调用方。

## 开发与验证

编码约定和命令见[开发指南](../AGENTS.md)。`internal/architecture` 测试约束分层和 SDK 位置。跨包或运行行为变更运行 `make check`，并发变更加跑相关包 `go test -race`。

当前 Claude adapter 的 `Stream` 存在上游数据竞争，`TestClaudeReportsCacheUsageThroughRunner` 可在 `-race` 下复现；升级依赖后应复测。依赖版本以 `go.mod` 为准。
