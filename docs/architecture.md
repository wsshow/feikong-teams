# 架构设计

本文档记录项目的稳定边界和扩展契约。目标是让入口、核心协议、运行时适配器、工具、存储和 hooks 保持高内聚低耦合。

## 分层边界

| 层级 | 目录 | 职责 |
| ---- | ---- | ---- |
| 入口层 | `commands/`、`server/`、`channels/`、`cli/` | 处理 CLI、HTTP、WebSocket 和消息通道输入，只编排会话，不直接依赖具体运行时框架 |
| 会话执行层 | `engine/` | 装配 context、历史、HITL、事件分发和运行级 hooks，并把本轮输入交给 `agentcore.Runner` |
| 核心协议层 | `agentcore/` | 定义 Agent、Runner、Engine、Model、Tool、Event、Checkpoint、Runtime capability 等稳定接口 |
| 运行时注册层 | `agentcore/runtime/` | 管理运行时注册表、默认运行时选择和 MCP 工具提供者桥接，不 import 任何具体运行时适配器 |
| 应用装配层 | `bootstrap/runtimes/` | 安装默认运行时实现，是应用二进制的 composition root |
| 运行时适配层 | `agentcore/eino/` | 把 CloudWeGo Eino ADK 的 Agent、Runner、Tool、Model、Middleware 和事件转换为 `agentcore` 协议 |
| 扩展层 | `hooks/`、`tools/`、`agents/`、`memory/` | 通过核心接口接入，不越过 `agentcore` 直接依赖运行时实现 |

## 运行时隔离规则

- `agentcore/runtime` 只保存 `agentcore.Engine`，不得 import `agentcore/eino` 或任何 `github.com/cloudwego/eino*` 包。
- 默认运行时由 `bootstrap/runtimes` 注册。应用入口通过空导入安装默认实现，测试可以选择显式安装 bootstrap 或注册 mock engine。
- 运行时适配器必须实现 `agentcore.Engine`。如需暴露能力信息和健康状态，应额外实现 `agentcore.RuntimeInspector`。
- MCP、middlewares、model decorator 等框架特定能力必须通过 `agentcore` 中的 capability interface 暴露，调用方只做接口断言。
- `agentcore/eino/boundary_test.go` 负责阻止 Eino import 泄漏到适配器和 bootstrap 之外。

## 新运行时接入流程

1. 新建运行时适配目录，实现 `agentcore.Engine`。
2. 在适配器内部完成底层框架对象和 `agentcore` 协议对象的双向转换。
3. 按需实现 `ModelDecorator`、`AgentPipelineProvider`、`ToolPipelineProvider`、`MCPToolProvider` 和 `RuntimeInspector`。
4. 在独立 bootstrap 包中调用 `agentcore/runtime.Register(name, engine)`。
5. 增加边界测试，确保底层框架依赖只出现在适配器和 bootstrap 中。

## 稳定性约束

- 会话运行必须经过 `engine.Session`，由 `runConfig` 统一装配 context、历史记录、HITL 处理器和 hooks。
- `Session.OnInterrupt` 未设置时使用固定拒绝决策，避免无处理器时卡住。
- hooks 默认带超时、panic recover 和错误策略；前置 hooks 默认失败即停止，后置和事件 hooks 默认告警后继续。
- 业务代码优先使用 `hooks.Bus` 的类型化方法触发扩展点，例如 `InvokeBeforeRun`、`InvokeEvent`、`InvokeBeforeToolCall` 和 `InvokeBeforeModelRequest`；只有新增扩展点时才直接构造底层 `Invocation`。
- 所有运行时事件必须转换为 `agentcore.Event`，事件类型使用 `agentcore/types.go` 和 `events/types.go` 中的常量。
- checkpoint、history、memory 等状态能力通过接口传入，不允许运行时适配层直接耦合入口层存储实现。
