# fkteams

基于 CloudWeGo Eino ADK 的多智能体协作系统，支持 CLI、Web UI、纯 API 服务和消息通道（Discord/QQ/微信）多种交互方式。

## 构建与运行

```bash
# 开发
go build ./...                          # 编译检查
go vet ./...                            # 静态检查
go run ./cmd/fkteams                    # 启动 CLI 聊天
go run ./cmd/fkteams web                # 启动 Web 服务（默认 :23456）
go run ./cmd/fkteams serve              # 启动纯 API 服务

# 构建
make native                             # 当前平台 -> release/fkteams_<goos>_<goarch>
make all                                # 预设平台（darwin/arm64, windows/amd64, linux/amd64）
make build t=linux:amd64                # 指定平台
make clean                              # 清理 release/

# 生成配置示例
go run ./cmd/fkteams generate config
```

## 项目架构

```
cmd/fkteams/main.go         # 入口，调用 internal/adapters/transport/cli/commands.Root().Run()
internal/app/               # 应用用例层，入口只调用这里
  config/                   #   TOML 配置加载、保存、热重载和示例生成
  version/                  #   应用版本和构建时间元数据
  appdata/                  #   应用数据目录、workspace/session/share/runtime 路径
  appstate/                 #   应用实例运行时状态（记忆服务 / 资源清理器）
  chat/                     #   RunTurn / 输入构建 / 入口上下文装配
    taskstream/             #   运行中任务事件流、队列、interrupt 状态管理
  agent/                    #   Runner 工厂、团队组装和 mode/agentName 解析
    catalog/                #   内置智能体定义、注册表、AgentBuilder 和成员工具元信息
  tools/                    #   工具注册、解析、策略标记和运行时无关内置工具实现
  memory/                   #   长期记忆检索、注入、提取、BM25 和 Markdown 持久化
  schedule/                 #   定时任务用例入口、后台任务结果收集，工具/HTTP/CLI 只调用这里
  skill/                    #   技能 provider、安装、移除、搜索结果和本地文件管理
  lifecycle/                #   Application 生命周期编排内核
                            #   用例层禁止依赖 agentcore 旧门面
                            #   用例层禁止依赖 pterm 等终端展示库
internal/domain/
  memory/                   #   MemoryEntry / Message / MemoryType 等长期记忆值对象
  schedule/                 #   Task / Status / HistoryEntry 等调度领域模型
  session/                  #   会话 ID 与 context 绑定
internal/runtime/           # 运行时无关内核
  turn/                     #   回合执行内核、HITL handler、hooks/context 装配
  events/                   #   事件分发、Emitter、协议校验、友好错误归一化
  registry/                 #   runtime engine 注册表和默认 runtime 选择
  model/                    #   运行时无关 ChatModel 工厂注册表
  env/                      #   FEIKONG_* 环境变量读取
  log/                      #   日志 facade 和文件轮转
  atomicfile/               #   原子文件写入
  pathguard/                #   工作区路径逃逸防护
  typeutil/                 #   运行时类型名辅助
  hooks/                    #   HookBus 实现、context 绑定和 hook 调用
  checkpoint/               #   checkpoint 存储实现
  mdiff/                    #   文件差异和补丁基础能力
  resources/                #   运行期资源清理器
  retry/                    #   模型重试和迭代限制策略
                            #   运行时内核禁止依赖 agentcore 旧门面
internal/ports/             # 运行时无关端口契约
  hooks/                    #   HookPoint、HookHandler 和明确 payload 类型
  memory/                   #   LLMClient 等长期记忆外部能力端口
  runtime/                  #   Runtime / Engine / Runner / Model / Tool 等端口
  scheduler/                #   Scheduler / TaskExecutor 调度端口
  storage/                  #   SessionMessageReader 等存储读取端口
  tools/                    #   MCPProvider 等工具外部能力端口
internal/adapters/scheduler/
  filecron/                 #   文件存储 + cron 轮询调度器
internal/adapters/tools/
  builtin/scheduler/        #   schedule_* 工具适配器，只委托 app/schedule
  mcp/                      #   MCP client、缓存和 runtime tool provider 桥接
internal/adapters/transport/
  cli/commands/             #   CLI 命令定义（urfave/cli/v3），参数解析和生命周期连接
  http/                     #   Gin HTTP 服务、Router、Handler、Middleware 和 origin 策略
  cli/runtime/              #   CLI 会话、输入、查询执行和交互运行时编排
  cli/eventview/            #   CLI 事件渲染和 JSON 输出回调
  cli/tui/                  #   CLI 终端 UI 组件、Markdown 渲染和交互控件
  cli/report/               #   CLI Markdown 报告导出 HTML 适配器
  cli/update/               #   CLI 自更新、下载、校验和替换适配器
  channel/                  #   Discord / QQ / 微信消息通道适配器和 Bridge
internal/adapters/runtime/
  eino/                     # CloudWeGo Eino ADK 适配层，唯一允许 import Eino 的目录
    runner.go               #   ADK AgentEvent -> events 协议转换，HITL resume 适配
    engine/engine.go        #   runtime.Engine 的 Eino 实现
                            #   adapter 与 middlewares 直接使用 internal/ports/runtime 与 domain 类型，禁止依赖 agentcore 旧门面
    middlewares/            #   autocontinue / summary / skills / dispatch / inject / fkfs
    middlewares/tools/      #   warperror / trimresult / patch / destructiveguard
    providers/              #   OpenAI / DeepSeek / Claude / Ollama / Ark / Gemini / Qwen / OpenRouter / Copilot
internal/adapters/model/
  providers/                #   模型 provider 注册、检测、模型列表和 Copilot 支撑
  memory/                   #   runtime ChatModel 到长期记忆 LLMClient 的适配
    providerkit/            #   provider 共用 HTTP/config 辅助
    copilot/                #   GitHub Copilot OAuth/token/HTTP 支撑
internal/adapters/storage/
  file/history/             #   HistoryRecorder、会话 metadata、历史文件读写
                            #   agentcore 旧门面已删除，禁止恢复；直接使用 internal/domain 与 internal/ports/runtime
internal/bootstrap/environment/ # init 命令运行环境初始化器（uv / bun）
internal/bootstrap/runtimes/ #  默认 runtime engine 和 provider 注册
internal/bootstrap/tools/    #  adapter 工具组与 app 工具注册表连接
internal/bootstrap/services/ #  组合层后台服务实现（memory / scheduler）
web/                        # 内嵌前端（//go:embed）
```

### 数据目录

默认应用目录为 `~/.fkteams`，可用 `FEIKONG_APP_DIR` 覆盖。常用子目录：

`{workspace,scheduler,sessions,history,config,log,share,runtime}`

## 代码风格

1. **错误信息英文，注释中文**（只在必要位置写精简注释）
2. **禁止 emoji 图形字符**（文字符号如 ✓✗ 允许）
3. **向 `strings.Builder` 写格式化内容用 `fmt.Fprintf(&sb, ...)`**，不用 `sb.WriteString(fmt.Sprintf(...))`
4. **用 `any` 替代 `interface{}`**
5. **工具函数不返回 error**：将错误信息放入响应的 `ErrorMessage` 字段并返回 nil
6. **初始化函数必须返回 error**，不使用 `log.Fatal`
7. **禁止事件类型的字符串字面量**：始终使用 `internal/domain/event` 中的类型常量

## 验证与交付

- 功能、重构或运行时行为改动优先执行 `go test ./...` 和 `go build ./...`；涉及静态风险时补充 `go vet ./...`。
- 小范围改动可以先跑相关 package 的测试，但最终交付前要说明实际执行过的验证。
- 文档、提示词或纯前端脚本改动至少执行 `git diff --check`；前端脚本改动优先补充 `node --check <file>`。
- 功能变更必须同步更新 `README.md`，但 README 面向用户，避免暴露不必要的内部调度细节。
- 提交信息遵循 Conventional Commits：`feat:`、`fix:`、`refactor:`、`chore:`、`docs:`、`test:` 等类型后接中文说明。
- 验证失败、未运行或被环境阻塞时必须如实说明原因和剩余风险。

## 开发约定

### 智能体

- 新智能体必须使用 `internal/app/agent/catalog/common/builder.go` 的 `AgentBuilder` 创建
- 新智能体必须在 `internal/app/agent/catalog/registry.go` 的 `buildRegistry()` 中注册
- 每个智能体目录包含 `agent.go`（`NewAgent()` 工厂）和 `prompt.go`（系统提示词模板）

### 工具

- 新工具组必须通过 `internal/app/tools.ToolGroupRegistry` 注册，禁止在 `internal/app/tools/tools.go` 中增加 switch 分支
- 依赖具体存储、调度器或第三方 SDK 的工具实现属于 `internal/adapters/tools`，通过 `internal/bootstrap/tools` 连接到应用工具注册表；`internal/app/tools` 禁止反向 import adapter
- MCP 动态工具只能通过 `internal/ports/tools.MCPProvider` 注入，禁止在 `internal/app/tools` 中直接 import `github.com/mark3labs/mcp-go`
- 工具必须通过 `internal/app/tools/metadata.go` 的 `ClassifyTools()` 标记元数据（只读/破坏性）

### 配置

- 新配置项必须添加到 `internal/app/config/config.go` 的 `GenerateExample()` 中生成示例
- 配置通过 `config.Get()` 获取，使用 `atomic.Pointer` 实现热重载

### 生命周期

- 新的后台服务实现 `internal/app/lifecycle.Service` 接口（`Name() / Start() / Stop()`），具体组合层服务放在 `internal/bootstrap/services`
- 服务按注册顺序启动，逆序（LIFO）停止

### 事件

- 事件处理使用 `internal/domain/event` 中的类型常量，禁止使用字符串字面量
- 新增事件类型/动作类型/通知类型必须先在 `internal/domain/event` 中定义常量
- 发事件必须使用 `internal/runtime/events`；禁止恢复根 `events` 门面
- HTTP handler、middleware、router 和 origin 策略位于 `internal/adapters/transport/http`，禁止恢复根 `server` 包
- 运行时适配器发事件优先使用 `internal/runtime/events.Emitter` 和 `AgentStart` / `MessageDelta` / `ToolStart` 等构造函数
- CLI 会话、查询执行和交互运行时位于 `internal/adapters/transport/cli/runtime`，禁止恢复根 `cli` 包
- 流式事件的规范增量载荷使用 `Content`；不要在核心事件或历史存储中重复维护 `Delta`
- 工具调用事件必须通过 `tool_call_ref` 保持 `message_delta(tool_args)`、`message_end.tool_calls[]`、`tool_start/update/end` 的稳定关联
- WebSocket `steer`、`/stream/steer` 和终端运行中 Enter 必须进入 steering 通道，由 `SteeringSource` 在下一次模型调用前消费；运行中的普通 `chat`/`follow_up` 只作为后续任务排队
- 流式任务队列项必须带稳定 `queue_id`；Web/SSE/WS 通过 `queue_updated` 同步快照。队列管理只能修改尚未消费的项，Web 运行中输入默认追加 follow-up，并支持在队列面板中转换 steering/follow-up、编辑、删除、同类排序；终端运行中只追加 steering，消费时合并当前队列，`Esc` 暂停时将未消费 steering 回填到输入框

### Hooks

- Hook payload 必须在 `internal/ports/hooks` 中定义为明确结构体并实现 `hooks.Payload`，禁止在 `Invocation` / `Result` 中使用 `any` 作为 payload 契约
- 新增 hook point 必须补充 payload 结构、便捷调用函数和架构边界测试

### 通道

- 通道实现必须通过 `internal/adapters/transport/channel.RegisterFactory` 注册工厂
- 通道消息处理通过 `internal/adapters/transport/channel.Bridge` 桥接器路由到应用用例
- 禁止恢复根 `channels` 包；Discord/QQ/微信实现属于 `internal/adapters/transport/channel`

### 模型提供者

- 新模型提供者通过 `internal/adapters/model/providers/providers.go` 的工厂模式注册
- 提供商需实现模型创建和列表获取

### 其他

- `Session.OnInterrupt` 未设置时自动使用固定拒绝决策
