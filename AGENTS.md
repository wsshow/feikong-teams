# fkteams

基于 CloudWeGo Eino ADK 的多智能体协作系统，支持 CLI、Web UI 和消息通道（Discord/QQ/微信）三种交互方式。

## 构建与运行

```bash
# 开发
go build ./...                          # 编译检查
go vet ./...                            # 静态检查
go run .                                # 启动 CLI 聊天
go run . web                            # 启动 Web 服务（默认 :23456）
go run . serve                          # 启动纯 API 服务

# 构建
make native                             # 当前平台 -> release/fkteams_os_arch
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
engine/                     # 统一执行引擎
  config.go                 #   RunConfig — 集中管理 context 装配和回调
                            #   （OnStart → OnInterrupt → OnFinish），各入口不再手动装配 context
  run.go                    #   Engine.Run() — 装配 context 后调用 runLoop
  loop.go                   #   runLoop() — Runner 事件循环，处理迭代和 HITL 中断/恢复
  interrupt.go              #   HITL 中断处理器（AutoRejectHandler / ChannelHandler / CallbackHandler）
agents/                     # 智能体系统
  registry.go               #   AgentInfo 注册表，延迟加载，按配置启用基础/可选/自定义智能体
  common/builder.go         #   AgentBuilder 流式构建器（WithTools / WithTemplate / WithSummary / Build）
  common/common.go          #   NewChatModel / MaxIterations
  middlewares/              #   中间件（autocontinue / summary / skills / dispatch / inject）
  middlewares/tools/        #   工具中间件（warperror / trimresult / patch / destructiveguard）
  retry/                    #   模型调用自动重试
runner/                     # Runner 工厂 — 根据 mode 创建不同 Runner
  runner.go                 #   CreateSupervisorRunner / CreateDeepAgentsRunner /
                            #   CreateLoopAgentRunner / CreateCustomSupervisorRunner
tools/                      # 工具系统
  tools.go                  #   GetToolsByName() — 按名称返回工具列表
  metadata.go               #   ClassifyTools() — 标记只读/破坏性工具
lifecycle/                  # 应用生命周期管理
  lifecycle.go              #   Application — Init → Setup → Start → Ready → [wait] → Stop → Cleanup
                            #   Service 接口，服务按序启动、逆序停止（LIFO）
server/                     # HTTP 服务（Gin）
  router/                   #   路由注册（Web 模式含内嵌前端，API 模式纯接口）
  handler/                  #   请求处理器（chat / websocket / stream / files / sessions / memory / config）
  middleware/               #   CORS / JWT 认证 / API Key 认证 / Body Limit
channels/                   # 消息通道桥接
  channel.go                #   Channel 接口 + Manager 管理器 + Factory 工厂注册
  bridge.go                 #   Bridge — 连接通道和引擎，goroutine 串行处理会话消息
fkevent/                    # 事件系统
  types.go                  #   类型常量：EventType / ActionType / NotifyType（禁止字符串字面量）
  event.go                  #   ProcessAgentEvent() — 事件分发和流式消息处理
  history.go                #   HistoryRecorder — 会话历史记录和摘要持久化
config/                     # TOML 配置（atomic.Pointer 全局单例，支持热重载）
providers/                  # 模型提供者（OpenAI / DeepSeek / Claude / Ollama / Ark / Gemini / Qwen / OpenRouter / Copilot）
memory/                     # 长期记忆系统（BM25 检索 + 提取 + 注入）
web/                        # 内嵌前端（//go:embed）
g/                          # 全局变量（MemoryManager / ProcessCleaner）
common/                     # 跨模块共享（会话 ID / 目录路径 / 重试判断）
fkenv/                      # 环境变量读取
log/                        # 日志配置（lumberjack 轮转）
tui/                        # 终端 UI 组件
cli/                        # CLI 交互循环
mdiff/                      # 文件差异/补丁
bootstrap/                  # 应用目录初始化
```

### 数据目录

`~/.fkteams/{workspace,scheduler,sessions,history,config,log}`

## 代码风格

1. **错误信息英文，注释中文**（只在必要位置写精简注释）
2. **禁止 emoji 图形字符**（文字符号如 ✓✗ 允许）
3. **向 `strings.Builder` 写格式化内容用 `fmt.Fprintf(&sb, ...)`**，不用 `sb.WriteString(fmt.Sprintf(...))`
4. **用 `any` 替代 `interface{}`**
5. **工具函数不返回 error**：将错误信息放入响应的 `ErrorMessage` 字段并返回 nil
6. **初始化函数必须返回 error**，不使用 `log.Fatal`
7. **禁止事件类型的字符串字面量**：始终使用 `fkevent/types.go` 中的类型常量

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

- 事件处理使用 `fkevent/types.go` 中的类型常量，禁止使用字符串字面量
- 新增事件类型/动作类型/通知类型必须先在 `types.go` 中定义常量

### 通道

- 通道实现必须通过 `channels.RegisterFactory` 注册工厂
- 通道消息处理通过 `Bridge` 桥接器路由到引擎

### 模型提供者

- 新模型提供者通过 `providers/providers.go` 的工厂模式注册
- 提供商需实现模型创建和列表获取

### 其他

- `RunConfig.OnInterrupt` 为 nil 时自动使用 `AutoRejectHandler`
- 功能变更必须同步更新 `README.md`
