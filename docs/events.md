# 事件协议

fkteams 的 CLI、Web、HTTP Stream、WebSocket 和聊天通道共用统一事件协议。事件由 `event_id`、`sequence`、`created_at` 标识顺序和时间，由 `phase`、`is_partial`、`is_final` 表示生命周期阶段。

## 核心约定

- 流式分片事件只表示增量，不代表任务完成；消费者需要等待完整事件或会话收尾后再归档结果。
- 工具调用优先使用 `tool_call_id` 关联；流式准备阶段只有 index 时，使用 `tool_call_index` 建立临时关联，收到真实 ID 后迁移。
- AgentTool 必须在工具调用事件中带上 `kind=agent`、`display_name`、`target`，展示端不得通过工具名称前缀判断成员工具。
- 子智能体事件必须通过 `member_call_id`、`member_name`、`member_tool_name` 表示父级 AgentTool 调用归属，终端和网页不再依赖 `run_path` 猜测成员关系。
- 展示端应优先使用事件中的 `tool_name`、`member_name` 等结构化字段，`detail` 仅作为补充展示数据。

## 会话历史

会话历史使用 `history.jsonl` 记录事件日志，每行是一条 `message_event`。服务端加载历史时按 `message_id` 和 `event_index` 重建消息，前端和终端按事件顺序渲染文本、思考、工具调用、工具结果和动作。

`message_event` 行必须包含以下稳定字段：

- `type`: 固定为 `message_event`
- `message_id`: 单条消息的稳定 ID
- `event_index`: 当前消息内的事件顺序
- `agent_name` / `run_path`: 事件所属智能体和运行路径
- `member_call_id` / `member_tool_name` / `member_name`: 子智能体归属信息
- `start_time` / `end_time`: 消息时间
- `event`: 标准消息事件，类型为 `text`、`reasoning`、`tool_call`、`action` 或 `error`

工具调用历史必须通过 `tool_call.id` 或 `tool_call.index` 关联参数和结果，不做名称前缀或旧结构兼容。
