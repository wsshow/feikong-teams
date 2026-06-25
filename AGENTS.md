# fkteams

基于 CloudWeGo Eino ADK 的多智能体协作系统，支持 CLI、Web UI、纯 API 服务和消息通道（Discord/QQ/微信）多种交互方式。

## 构建与运行

```bash
# 开发
go build ./...                          # 编译检查
go vet ./...                            # 静态检查
go run .                                # 启动 CLI 聊天
go run . web                            # 启动 Web 服务（默认 :23456）
go run . serve                          # 启动纯 API 服务

# 构建
make native                             # 当前平台 -> release/fkteams_<goos>_<goarch>
make all                                # 预设平台（darwin/arm64, windows/amd64, linux/amd64）
make build t=linux:amd64                # 指定平台
make clean                              # 清理 release/

# 生成配置示例
go run . generate config
```

## 项目架构

```
main.go                     # 入口，调用 commands.Root().Run()
commands/                   # CLI 命令定义（urfave/cli/v3）
  root.go                   #   根命令，注册子命令和全局 flag
  chat.go, web.go, serve.go #   聊天 / Web 服务 / API 服务
  session.go, agent.go      #   会话和智能体管理
  skill/                    #   技能安装、移除、搜索
                            #   CLI 命令层使用 internal/ports/runtime 和 domain/message
internal/app/               # 应用用例层，入口只调用这里
  appdata/                  #   应用数据目录、workspace/session/share/runtime 路径
  appstate/                 #   应用实例运行时状态（记忆服务 / 资源清理器）
  chat/                     #   RunTurn / 输入构建 / 入口上下文装配
    taskstream/             #   运行中任务事件流、队列、interrupt 状态管理
  agent/                    #   Runner 工厂、团队组装和 mode/agentName 解析
  schedule/                 #   定时任务用例入口，工具/HTTP/CLI 只调用这里
  lifecycle/                #   Application 生命周期编排内核
                            #   用例层禁止依赖 agentcore 旧门面
internal/domain/
  schedule/                 #   Task / Status / HistoryEntry 等调度领域模型
  session/                  #   会话 ID 与 context 绑定
internal/runtime/           # 运行时无关内核
  turn/                     #   回合执行内核、HITL handler、hooks/context 装配
  registry/                 #   runtime engine 注册表和默认 runtime 选择
  hooks/                    #   HookBus 实现、context 绑定和 hook 调用
  checkpoint/               #   checkpoint 存储实现
  resources/                #   运行期资源清理器
  retry/                    #   模型重试和迭代限制策略
                            #   运行时内核禁止依赖 agentcore 旧门面
internal/ports/             # 运行时无关端口契约
  hooks/                    #   HookPoint、HookHandler 和明确 payload 类型
  runtime/                  #   Runtime / Engine / Runner / Model / Tool 等端口
  scheduler/                #   Scheduler / TaskExecutor 调度端口
internal/adapters/scheduler/
  filecron/                 #   文件存储 + cron 轮询调度器
internal/adapters/tools/
  builtin/scheduler/        #   schedule_* 工具适配器，只委托 app/schedule
internal/adapters/runtime/
  eino/                     # CloudWeGo Eino ADK 适配层，唯一允许 import Eino 的目录
    runner.go               #   ADK AgentEvent -> events 协议转换，HITL resume 适配
    engine/engine.go        #   runtime.Engine 的 Eino 实现
                            #   adapter 与 middlewares 直接使用 internal/ports/runtime 与 domain 类型，禁止依赖 agentcore 旧门面
    middlewares/            #   autocontinue / summary / skills / dispatch / inject / fkfs
    middlewares/tools/      #   warperror / trimresult / patch / destructiveguard
    providers/              #   OpenAI / DeepSeek / Claude / Ollama / Ark / Gemini / Qwen / OpenRouter / Copilot
internal/adapters/storage/
  file/history/             #   HistoryRecorder、会话 metadata、历史文件读写
                            #   agentcore 旧门面已删除，禁止恢复；直接使用 internal/domain 与 internal/ports/runtime
internal/bootstrap/services/ #  组合层后台服务实现（memory / scheduler）
agents/                     # 智能体系统
  registry.go               #   AgentInfo 注册表，延迟加载，按配置启用基础/可选/自定义智能体
  common/builder.go         #   AgentBuilder 构建器（WithTools / WithToolNames / WithSummary / WithSkills / Build）
  common/common.go          #   NewChatModel / MaxIterations
  toolmeta/                 #   成员智能体工具前缀、显示名和分类注册
                            #   智能体层使用 internal/ports/runtime，禁止再依赖 agentcore 旧门面
tools/                      # 工具系统
  registry.go               #   ToolGroupRegistry，注册和解析工具组
  tools.go                  #   GetToolsByName() — 委托注册表和 MCP fallback
  metadata.go               #   ClassifyTools() — 标记只读/破坏性工具
                            #   定时任务工具适配器位于 internal/adapters/tools/builtin/scheduler
                            #   工具层使用 internal/ports/runtime，禁止再依赖 agentcore 旧门面
server/                     # HTTP 服务（Gin）
  router/                   #   路由注册（Web 模式含内嵌前端，API 模式纯接口）
  handler/                  #   请求处理器（chat / websocket / stream / files / sessions / memory / config）
                            #   handler 使用 internal/ports/runtime、domain/message 与 app/chat，禁止依赖 agentcore 旧门面
  middleware/               #   CORS / JWT 认证 / API Key 认证 / Body Limit
channels/                   # 消息通道桥接
  channel.go                #   Channel 接口 + Manager 管理器 + Factory 工厂注册
  bridge.go                 #   Bridge — 连接通道和引擎，goroutine 串行处理会话消息
                            #   通道桥接使用 internal/ports/runtime 和 domain/message，禁止依赖 agentcore 旧门面
events/                     # 事件协议与展示/历史
  types.go                  #   domain/event 事件类型别名和常量导出
  event.go                  #   context 事件回调、NormalizeEvent、DispatchEvent
  emitter.go                #   Emitter + Agent/Turn/Message/Tool 事件构造函数
  protocol.go               #   工具调用身份协议校验与兼容辅助
  view/                     #   CLI 事件渲染、JSON 输出回调、后台 Markdown 收集
                            #   展示层使用 domain/event 和 domain/message，禁止依赖 agentcore 旧门面
config/                     # TOML 配置（atomic.Pointer 全局单例，支持热重载）
providers/                  # runtime port 模型提供者注册、检测和模型列表获取
                            #   provider 工厂禁止再依赖 agentcore 旧门面
memory/                     # 长期记忆系统（BM25 检索 + 提取 + 注入）
                            #   记忆模型适配使用 internal/ports/runtime 和 domain/message
web/                        # 内嵌前端（//go:embed）
common/                     # 明确基础包（atomicfile / pathguard / typeutil），禁止恢复 root common 杂物层
fkenv/                      # 环境变量读取
log/                        # 日志配置（lumberjack 轮转）
tui/                        # 终端 UI 组件与 Markdown 渲染
cli/                        # CLI 交互循环
                            #   CLI runtime 使用 internal/ports/runtime 和 domain/message，禁止依赖 agentcore 旧门面
mdiff/                      # 文件差异/补丁
bootstrap/                  # 应用目录初始化
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
7. **禁止事件类型的字符串字面量**：始终使用 `events/types.go`（底层为 `internal/domain/event`）中的类型常量

## 验证与交付

- 功能、重构或运行时行为改动优先执行 `go test ./...` 和 `go build ./...`；涉及静态风险时补充 `go vet ./...`。
- 小范围改动可以先跑相关 package 的测试，但最终交付前要说明实际执行过的验证。
- 文档、提示词或纯前端脚本改动至少执行 `git diff --check`；前端脚本改动优先补充 `node --check <file>`。
- 功能变更必须同步更新 `README.md`，但 README 面向用户，避免暴露不必要的内部调度细节。
- 提交信息遵循 Conventional Commits：`feat:`、`fix:`、`refactor:`、`chore:`、`docs:`、`test:` 等类型后接中文说明。
- 验证失败、未运行或被环境阻塞时必须如实说明原因和剩余风险。

## 开发约定

### 智能体

- 新智能体必须使用 `agents/common/builder.go` 的 `AgentBuilder` 创建
- 新智能体必须在 `agents/registry.go` 的 `buildRegistry()` 中注册
- 每个智能体目录包含 `agent.go`（`NewAgent()` 工厂）和 `prompt.go`（系统提示词模板）

### 工具

- 新工具组必须通过 `tools.ToolGroupRegistry` 注册，禁止在 `tools/tools.go` 中增加 switch 分支
- 工具必须通过 `tools/metadata.go` 的 `ClassifyTools()` 标记元数据（只读/破坏性）

### 配置

- 新配置项必须添加到 `config/config.go` 的 `GenerateExample()` 中生成示例
- 配置通过 `config.Get()` 获取，使用 `atomic.Pointer` 实现热重载

### 生命周期

- 新的后台服务实现 `internal/app/lifecycle.Service` 接口（`Name() / Start() / Stop()`），具体组合层服务放在 `internal/bootstrap/services`
- 服务按注册顺序启动，逆序（LIFO）停止

### 事件

- 事件处理使用 `events/types.go` / `internal/domain/event` 中的类型常量，禁止使用字符串字面量
- 新增事件类型/动作类型/通知类型必须先在 `internal/domain/event` 中定义常量，并由 `events/types.go` 导出别名
- 运行时适配器发事件优先使用 `events.Emitter` 和 `events.AgentStart` / `events.MessageDelta` / `events.ToolStart` 等构造函数
- 流式事件的规范增量载荷使用 `Content`；不要在核心事件或历史存储中重复维护 `Delta`
- 工具调用事件必须通过 `tool_call_ref` 保持 `message_delta(tool_args)`、`message_end.tool_calls[]`、`tool_start/update/end` 的稳定关联
- WebSocket `steer`、`/stream/steer` 和终端运行中 Enter 必须进入 steering 通道，由 `SteeringSource` 在下一次模型调用前消费；运行中的普通 `chat`/`follow_up` 只作为后续任务排队
- 流式任务队列项必须带稳定 `queue_id`；Web/SSE/WS 通过 `queue_updated` 同步快照。队列管理只能修改尚未消费的项，Web 运行中输入默认追加 follow-up，并支持在队列面板中转换 steering/follow-up、编辑、删除、同类排序；终端运行中只追加 steering，消费时合并当前队列，`Esc` 暂停时将未消费 steering 回填到输入框

### 通道

- 通道实现必须通过 `channels.RegisterFactory` 注册工厂
- 通道消息处理通过 `Bridge` 桥接器路由到引擎

### 模型提供者

- 新模型提供者通过 `providers/providers.go` 的工厂模式注册
- 提供商需实现模型创建和列表获取

### 其他

- `Session.OnInterrupt` 未设置时自动使用固定拒绝决策
