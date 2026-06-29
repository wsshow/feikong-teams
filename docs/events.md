# 事件协议

fkteams 的 CLI、Web、HTTP Stream、WebSocket 和聊天通道共用统一事件协议。事件由 `event_id`、`sequence`、`created_at` 标识顺序和时间，由 `type` 表示生命周期节点：`agent_started/completed`、`turn_started/completed`、`assistant_*`、`tool_call_*`、`ask_*`、`approval_*`、`member_*`、`system_notice`、`usage_reported`、`error`。

## 核心约定

- 事件核心实现位于 `internal/runtime/events`；根 `events` 包只保留外层入口使用的导出门面和类型别名，`internal/**` 不得导入根门面。
- 运行时适配器通过 `internal/runtime/events.Emitter` 和事件构造函数发出生命周期事件；适配器负责把底层框架事件翻译为协议事件，不直接把结构体字段拼装逻辑扩散到入口层。
- 流式分片事件只表示增量，不代表任务完成；消费者需要等待 `assistant_completed`、`tool_call_completed`、`turn_completed` 等完整事件或会话收尾后再归档结果。
- 助手输出拆为 `assistant_reasoning_delta` 和 `assistant_text_delta`；增量载荷只使用 `content`，核心事件、HTTP 事件和历史存储不重复保存同一份文本。
- 工具调用优先使用 `tool_call_ref` 关联；流式 `tool_call_arguments_delta`、`assistant_completed.tool_calls[]`、`tool_call_started`、`tool_call_result_delta`、`tool_call_completed` 必须保持同一个 ref，`tool_call_id` 和 `tool_call_index` 仅作为辅助身份。
- 用户提问/回答使用 `ask_requested` 和 `ask_answered`，问题、选项、选择结果进入 `ask` 载荷并同步展开为 HTTP 字段。
- 展示端必须遍历 `tool_calls[]`；单个工具调用事件可以携带 `tool_call` 作为当前调用对象，但协议入口仍以事件类型和 `tool_call_ref` 为准。
- AgentTool 必须在工具调用事件中带上 `kind=agent`、`display_name`、`target`，展示端不得通过工具名称前缀判断成员工具。
- 子智能体事件必须通过 `member_call_id`、`member_name`、`member_tool_name` 表示父级 AgentTool 调用归属，终端和网页不再依赖 `run_path` 猜测成员关系。
- 展示端应优先使用事件中的 `tool_name`、`member_name` 等结构化字段，`detail` 仅作为补充展示数据。

## 会话历史

会话历史使用 `history.jsonl` 记录事件日志，每行是一条 `message_event`。服务端加载历史时按 `message_id` 和 `event_index` 重建消息，前端和终端按事件顺序渲染文本、思考、工具调用、工具结果、ask 卡片和系统提示。

`message_event` 行必须包含以下稳定字段：

- `type`: 固定为 `message_event`
- `message_id`: 单条消息的稳定 ID
- `event_index`: 当前消息内的事件顺序
- `agent_name` / `run_path`: 事件所属智能体和运行路径
- `member_call_id` / `member_tool_name` / `member_name`: 子智能体归属信息
- `start_time` / `end_time`: 消息时间
- `event`: 标准消息事件，类型为 `text`、`reasoning`、`tool_call`、`ask`、`notice`、`usage`、`error` 或 `cancelled`

工具调用历史必须通过 `tool_call.id` 或 `tool_call.index` 关联参数和结果，不做名称前缀推断。
