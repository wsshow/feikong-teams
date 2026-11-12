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

`app/execution.Dependencies` 统一声明 Runtime、中断能力、模型/工具/工具展示/智能体注册表和 HookBus。组合根、HTTP、消息通道与调度器复用它的 `Context` / `FromContext` 绑定规则；后台任务只提取能力引用，再绑定自己的上下文，不捕获请求的取消信号或会话数据。模型提供者等具体 adapter 依赖仍由组合层单独装配。

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

`TurnRequest.History` 提供记录、定稿和 `Flush()` 契约。`chat.Service` 是每回合唯一的记录与定稿所有者：`RecordAndEmit` 串行分配会话序号、记录事实并调用展示接收器，成员问答和审批同样经过该管线，避免并发来源先分配序号却后发布。steering 也经过该管线；CLI/JSON/TUI 视图只接收事件。`SessionLifecycle` 只在历史提交后更新 metadata 和记忆，不再重复保存历史。执行取消与收尾失败保留独立的错误来源：只有纯取消映射为 cancelled，任何持久化或收尾失败均映射为 error。每次调用使用独立 RunID，SessionID 只标识长期会话。

历史记录按消息身份聚合尚未完成的文本和思考；完整消息同时校正内存投影并写入 transcript。取消、失败及回合定稿会保存剩余增量，成员消息保留独立 transcript，已经完成的消息不会在收尾时重复写入。

transcript 保留运行时事件信封，包括 UUID 事件 ID、RunID、TurnID、MessageID、成员与工具归属、会话序号和消息首次出现顺序。`SessionSequence` 由会话记录器分配，重启后从主干及成员记录的最大值继续；展示顺序不依赖墙上时钟或进程级运行序号。完成与取消收尾保留相同的消息和分段身份。Web 展示只接收包含事件信封的新格式，不补造旧记录的身份和顺序。打开旧格式会话时返回带友好说明的 HTTP 409，保留原记录并建议新建会话；前端提取原生或 Redux 序列化错误中的文本消息，未知对象使用通用提示。

新 transcript 的消息正文统一保存在 `message` 中，直接使用领域 `Message`，保留同一次模型输出的文本、思考、多模态分段、整组工具调用和供应商元数据。正文和思考的交错顺序使用 `OutputParts` 记录稳定分段 ID、UTF-8 字节范围及工具调用 ID，不重复持久化正文。完整消息校正草稿时同步校正这些范围，后端展示直接取规范消息的对应片段。工具执行事件仍独立记录，长结果继续使用 artifact 引用；工具业务结果在摘要前确定明确状态，空输出可以成功。

历史内存投影保存规范消息副本，`ProjectContextMessages` 优先读取该副本，保持并行工具调用属于同一条 assistant 消息；`ToolCallRecord` 只承担工具展示和旧记录兼容。用户历史附件省略、成员输出隔离、摘要边界和取消提示仍由上下文投影策略决定。已完整生成但未执行完成的工具调用补齐空结果以保持消息配对；取消时尚未生成完整参数的调用标为 `Incomplete`，保留展示草稿但不进入模型上下文。读取历史返回深拷贝，展示方和模型调用方都不能修改 recorder 内的消息事实。

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

Definition 可重复使用：配置覆盖先复制变量表，模板只对原始占位符执行一次替换，不递归解释变量值中的大括号，也不依赖 map 遍历顺序。

轻量任务使用 `app/agent/standalone.Service` 的 `RunText`、`StreamText` 或 `Run`，内部统一调用 `chat.Service.RunTurn`，继承回合 Hook、唯一运行标识和中断默认值。文本输出统一由 `chat.AssistantTextCollector` 投影，按消息身份以完整内容替换增量，并过滤成员输出；调用方不用自行拼装历史、工具或执行循环。

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

依赖固定为 `github.com/wsshow/agentkit v1.5.1`；其要求的 Go 版本是 `1.26.11`，底层 Eino 为 `v0.9.23`。

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
| 消息分段、成员归属、工具与交互展示状态 | `app/chat/presentation` |
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
    → transcript 原始信封及规范消息
    → app/chat/presentation → Web 展示快照与更新
    → 其他 transport DTO
```

库的公共事件没有工具参数分片、产品消息 ID 和完整的多模态输出字段，所以适配器观察同一次模型响应以补齐这些信息，完整消息在产品中间件修复完成后发布；它不负责执行工具或推动模型循环。工具参数第一次出现就确定调用 ID，SDK 执行、`message_delta(tool_args)`、`message_end.tool_calls[]` 和 `tool_start/update/end` 使用相同的 `tool_call_ref`。

工具调用的 `Extra` 作为供应商元数据保留在领域消息、工具历史和 transcript 中，恢复上下文时原样传回模型；跨 SDK 边界复制嵌套映射和切片，避免复用流分片修改已记录的元数据。多模态内容的 `file_id` 是供应商托管引用，通过消息、队列和 Web 协议传递，不能当作本地文件路径解析；实际支持取决于模型提供者。`Usage` 和历史用量记录保留 `cached_tokens`、`cache_write_tokens`，缓存明细已经包含在 prompt token 总数内，不额外累加到总用量。

消息与多模态分段也保留各自的 `Extra`；历史用量与事件共用同一份领域类型，读取时直接复制完整用量载荷。会话分享关闭工具详情时，同时清除规范消息里的工具参数、结果、供应商元数据以及外置结果的摘要和引用，避免通过新载荷绕过分享策略。

增量进入领域协议后统一使用 `Content`，不增加第二个 Delta 字段。UI DTO、历史投影和运行事实保持分层；展示内容由运行事实派生。

`app/chat/presentation.Projector` 是 Web 会话展示的唯一生成者。实时事件与 transcript 恢复都经过同一投影器，产生有明确 ID、位置、分段顺序和状态的 `Snapshot` 与 `Update`；SSE、WebSocket、同步 HTTP、会话历史和分享复用该契约。问答与审批的 `active` 由所属运行决定，运行结束后不能因开启下一轮而重新激活。成员内容嵌入所属工具节点，前端无需推测归属。

任务流在同一锁内应用运行事实、发布展示更新并推进传输游标，快照在同一锁内读取 `view` 与 `next_offset`。订阅从该游标继续，不再分页拼接展示历史；缓存窗口过期时重新读取完整快照，整个任务缓存过期后从已提交 transcript 恢复。结束帧 `processing_end` 包含明确的 `completed/error/cancelled` 状态，并以已提交 transcript 生成的展示快照定稿。原始错误事件用于报告错误，客户端只在结束帧或权威终态快照到达后结束订阅，避免漏掉最后一段正文。

## 组合、配置与持久化

`bootstrap.ExecutionDependencies` 持有一次应用的运行时、注册表和配置执行边界。`Command()` 注入 CLI 的服务装配与服务端启动函数；`RunServer()` 组合记忆、调度、HTTP 和消息通道。HTTP 服务只实现传输生命周期，不反向导入 bootstrap。关闭与重启由实例级 `lifecycle.Control` 管理；共享 MCP 连接在所有服务停止后由组合根清理。

`config.Get()` 继续通过 `atomic.Pointer` 发布快照。HTTP、消息通道、调度以及辅助工具执行持有同一个 `ExecutionGate` 读取租约，覆盖依赖构建、执行和收尾。配置更新只在空闲时取得写租约，有运行任务时返回 HTTP 409。先完成校验和可失败的记忆模型构建，再保存配置并重载智能体目录、清理 Runner/MCP 缓存、重置通道 Runner 和记忆模型，最后释放租约。这避免执行中混用新配置与旧依赖；监听地址、后台服务启停和连接配置仍需重启应用。

业务数据的存储位置与契约如下：

| 数据 | 应用契约 | 默认持久化 |
| --- | --- | --- |
| 对话历史与会话信息 | `chat.SessionHistory.Flush` / session store | SQLite 文档及追加片段 |
| 长期记忆 | `ports/memory.Store` | SQLite `memories` |
| 调度定义与执行状态 | scheduler store | SQLite 文档 |
| 调度结果与报告 | `ports/scheduler.ResultStore` | 文件目录 |
| 待执行队列 | `ports/storage.QueueStore` | SQLite 快照，附件留在文件目录 |
| 分享授权 | 具体分享存储 API | SQLite 文档 |
| 会话待办 | todo tools | SQLite 文档 |
| JavaScript 扩展状态 | JavaScript StateStore | SQLite 文档 |

组合根通过 `ports/storage.Documents` 为历史、队列、分享、调度和脚本适配器注入 SQLite 文档存储。文档使用应用根目录下的相对逻辑路径作为键；追加历史单独存为片段，避免每条消息重写完整会话。适配器继续负责领域格式与权限校验，未注入数据库的旧文件实现只用于兼容读取和测试。`modernc.org/sqlite` 通过实例 connector 使用，支持 `CGO_ENABLED=0`。

`bootstrap.prepareStorage` 在任何业务写入、后台调度或记忆提取启动前取得应用目录锁。`app/migration.Service` 编排业务目录备份、格式及附件校验、数据库导入和逐条回读验证；事务同时提交完成标记。Web 旧数据先进入维护服务器和一次性升级弹窗，CLI/API 自动等待同一用例。失败时不启动业务，原文件与备份保留，重试从未提交状态重新执行。数据库连接与目录锁在后台任务排空后的 Cleanup 释放。备份与恢复说明见 [存储升级](storage.md)。

分享存储复用同一个条目存储实现，统一并发、写入回滚与副本读取，具体条目负责自己的编码和校验。队列存储使用固定根目录读写附件。记忆加载失败必须返回错误，写入失败保留脏状态以供重试。服务停止和 cleanup 错误也从应用入口返回，不能只记录日志后报告成功。

## 前端职责

常驻助手通过 `app/session.Service.OpenAssistant` 幂等创建固定会话，名称保存在会话 metadata 的标题中。`workspace_assistant` 以标准内置 Definition 运行，复用聊天、历史、队列和审批链路；身份工具仅允许助手会话改名。`app/session.QueryService` 经会话存储和消息读取端口执行跨会话查询，`conversations` 工具组只输出有界正文投影。历史只读查询不修复 transcript 尾部，文件存储读取校验符号链接边界。

Web 的 `/assistant` 路由映射至固定会话；入口与“新建会话”位于侧栏滚动容器外。前端在改名工具完成及任务收尾后重新读取助手身份，显示服务端保存的名称。

助手输入框的 `/clear` 和 `/rename` 直接调用会话用例，不经过模型。清空操作持有会话操作锁并检查 recorder 租约，拒绝活跃会话；存储层在同一数据库事务内移除历史和队列、保留身份元数据，清空时淘汰内存 recorder 与流快照。长期记忆独立保留。

常驻助手消息中的 `openui-lang` 围栏由按需加载的 OpenUI Renderer 渲染，组件定义同时生成 Go 嵌入的提示词。UI 源码沿用消息 `Content` 和 transcript，表单操作作为普通用户消息通过共用提交函数进入现有聊天或 follow-up 队列；不建立第二套运行时、历史或审批通道。已提交字段从用户消息恢复，未提交草稿仅存于当前界面。组件和操作边界见 [助手交互界面](assistant-ui.md)。

聊天的 `timeline.ts` 只保留展示辅助函数，`MessageList.tsx` 与分享页共用 `ConversationItems` 渲染后端节点。前端只根据服务端的 ID、位置和分段键应用替换或追加，不重建事件时序、不解析工具响应推断成败，也不从工具参数恢复问答。配置的 `configModel.ts` 负责默认值、合并和保存格式；智能体编辑、集成编辑和共用字段按职责放在少量组件文件中。SSE 事件使用 `decodeChatEvent(s)`，所有展示快照与更新由 `types/presentation.ts` 统一校验；未知事件类型在 API 边界过滤，应用事件类型不再用 `| string` 扩宽。

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

当前 Claude 模型 adapter 依赖 `v0.1.26`，其 `Stream` 在协程合并前导空分片时写入了外层返回值 `err`，与函数返回路径存在数据竞争。`TestClaudeReportsCacheUsageThroughRunner` 使用本地 SSE 响应保留了复现路径，普通测试通过，`-race` 会报告该上游问题；升级到上游修复版本后应再次执行该测试确认。

### 关闭时的资源存活期

应用 context 取消会立即传递给 HTTP 任务、消息通道和调度器，阻止继续调度。服务按 LIFO 等待真实退出，之后才运行 Cleanup 并关闭共享 MCP 连接。`ShutdownTimeout` 只限制 `Application.Run` 调用方的等待时间；超时后收尾仍继续，嵌入方通过 `WaitShutdown(ctx)` 等待最终结果，排空期间不能重新运行同一应用。CLI 信号监听由当前直接查询持有，退出时注销 `signal.Notify` 并等待监听协程结束。

### 前端协议边界

`web/src/types/events.ts` 用同一份字段声明生成按 `type` 区分的类型与运行时校验。工具调用、队列项、附件及数值序号在进入 store 之前验证，队列接口共用队列校验。`types/presentation.ts` 校验展示快照、节点更新及分段追加；缺少展示快照时拒绝响应，不回退到前端推导。前台每次重连都先取后端快照及游标，既恢复内容，也确认任务是否已经结束。切换选中会话时在同一次状态更新中清空旧视图、队列和错误，保持后台任务独立；详情、加载错误和流式内容必须匹配当前视图所属会话。加载失败展示独立错误页，不渲染旧正文、问题导航或输入框。

Go handler 的原始事件协议样本保存于 `web/src/types/testdata/chat-events.json`；展示协议样本 `chat-view.json` 同时保存每一帧的更新和后端快照。Go 测试防止协议悄然变化，Bun 测试逐帧应用更新并与对应后端快照比较，覆盖思考与正文交错、最终正文改写、并行工具、成员问答和审批。后端另验证实时、落盘重放、进程重启、取消草稿、快照游标边界和并发发布的一致性。
