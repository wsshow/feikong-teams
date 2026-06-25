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
internal/app/               # 应用用例层，入口只调用这里
  chat/                     #   RunTurn / 输入构建 / 入口上下文装配
  agent/                    #   Runner 工厂、团队组装和 mode/agentName 解析
  schedule/                 #   定时任务执行用例，调度任务通过 chat service 执行
internal/runtime/           # 运行时无关内核
  turn/                     #   回合执行内核、HITL handler、hooks/context 装配
  checkpoint/               #   checkpoint 存储实现
internal/adapters/runtime/
  eino/                     # CloudWeGo Eino ADK 适配层，唯一允许 import Eino 的目录
    runner.go               #   ADK AgentEvent -> events 协议转换，HITL resume 适配
    engine/engine.go        #   runtime.Engine 的 Eino 实现
    middlewares/            #   autocontinue / summary / skills / dispatch / inject / fkfs
    middlewares/tools/      #   warperror / trimresult / patch / destructiveguard
    providers/              #   OpenAI / DeepSeek / Claude / Ollama / Ark / Gemini / Qwen / OpenRouter / Copilot
agentcore/                  # 过渡期核心类型导出层，最终收缩到 internal/domain 与 internal/ports
  types.go                  #   Message / ToolCall / Event / RunOptions / Runner 等协议类型别名
  agent.go                  #   Agent / Engine 抽象和 ChatAgentConfig / RunnerConfig 别名
  steering.go               #   SteeringSource context 能力，供运行时在模型调用边界消费转向消息
  model.go, tool.go         #   ChatModel / Tool 抽象别名
  runtime/runtime.go        #   默认 runtime engine 注册和获取
agents/                     # 智能体系统
  registry.go               #   AgentInfo 注册表，延迟加载，按配置启用基础/可选/自定义智能体
  common/builder.go         #   AgentBuilder 构建器（WithTools / WithToolNames / WithSummary / WithSkills / Build）
  common/common.go          #   NewChatModel / MaxIterations
  toolmeta/                 #   成员智能体工具前缀、显示名和分类注册
tools/                      # 工具系统
  tools.go                  #   GetToolsByName() — 按名称返回工具列表
  metadata.go               #   ClassifyTools() — 标记只读/破坏性工具
lifecycle/                  # 应用生命周期管理
  lifecycle.go              #   Application — Init → Setup → Start → Ready → [wait] → Stop → Cleanup
                            #   Service 接口，服务按序启动、逆序停止（LIFO）
server/                     # HTTP 服务（Gin）
  router/                   #   路由注册（Web 模式含内嵌前端，API 模式纯接口）
  handler/                  #   请求处理器（chat / websocket / stream / files / sessions / memory / config）
  handler/taskstream/       #   运行中任务事件流、HITL 输入、可管理 steering/follow-up 队列
  middleware/               #   CORS / JWT 认证 / API Key 认证 / Body Limit
channels/                   # 消息通道桥接
  channel.go                #   Channel 接口 + Manager 管理器 + Factory 工厂注册
  bridge.go                 #   Bridge — 连接通道和引擎，goroutine 串行处理会话消息
events/                     # 事件协议与展示/历史
  types.go                  #   agentcore 事件类型别名和常量导出
  event.go                  #   context 事件回调、NormalizeEvent、DispatchEvent
  emitter.go                #   Emitter + Agent/Turn/Message/Tool 事件构造函数
  protocol.go               #   工具调用身份协议校验与兼容辅助
  log/                      #   HistoryRecorder、会话 metadata、全局历史记录器管理
  view/                     #   CLI 事件渲染、JSON 输出回调、后台 Markdown 收集
  chat/                     #   历史消息构建器
config/                     # TOML 配置（atomic.Pointer 全局单例，支持热重载）
providers/                  # agentcore 外层模型提供者注册、检测和模型列表获取
memory/                     # 长期记忆系统（BM25 检索 + 提取 + 注入）
web/                        # 内嵌前端（//go:embed）
appstate/                   # 应用实例运行时状态（记忆管理器 / 资源清理器）
common/                     # 跨模块共享（会话 ID / 目录路径 / 重试判断）
fkenv/                      # 环境变量读取
log/                        # 日志配置（lumberjack 轮转）
tui/                        # 终端 UI 组件与 Markdown 渲染
cli/                        # CLI 交互循环
mdiff/                      # 文件差异/补丁
bootstrap/                  # 应用目录初始化
```

### 数据目录

默认应用目录为 `~/.fkteams`，可用 `FEIKONG_APP_DIR` 覆盖。常用子目录：

`{workspace,scheduler,sessions,history,config,log,share}`

## 代码风格

1. **错误信息英文，注释中文**（只在必要位置写精简注释）
2. **禁止 emoji 图形字符**（文字符号如 ✓✗ 允许）
3. **向 `strings.Builder` 写格式化内容用 `fmt.Fprintf(&sb, ...)`**，不用 `sb.WriteString(fmt.Sprintf(...))`
4. **用 `any` 替代 `interface{}`**
5. **工具函数不返回 error**：将错误信息放入响应的 `ErrorMessage` 字段并返回 nil
6. **初始化函数必须返回 error**，不使用 `log.Fatal`
7. **禁止事件类型的字符串字面量**：始终使用 `events/types.go`（底层为 `agentcore/types.go`）中的类型常量

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

- 新工具组必须在 `tools/tools.go` 的 `GetToolsByName()` 中注册
- 工具必须通过 `tools/metadata.go` 的 `ClassifyTools()` 标记元数据（只读/破坏性）

### 配置

- 新配置项必须添加到 `config/config.go` 的 `GenerateExample()` 中生成示例
- 配置通过 `config.Get()` 获取，使用 `atomic.Pointer` 实现热重载

### 生命周期

- 新的后台服务实现 `lifecycle.Service` 接口（`Name() / Start() / Stop()`）
- 服务按注册顺序启动，逆序（LIFO）停止

### 事件

- 事件处理使用 `events/types.go` / `agentcore/types.go` 中的类型常量，禁止使用字符串字面量
- 新增事件类型/动作类型/通知类型必须先在 `agentcore/types.go` 中定义常量，并由 `events/types.go` 导出别名
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
