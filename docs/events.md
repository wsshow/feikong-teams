# 事件与展示协议

CLI、Web、HTTP、WebSocket 和消息通道共享领域事件。Web 使用后端生成的展示快照与更新，详细传输方式见[流式任务 API](api/stream.md)。

## 事件身份与关联

领域类型定义在 `internal/domain/event`，构造与发射在 `internal/runtime/events`。`chat.Service` 统一记录、分配会话序号并发布；HTTP 边界再转换为传输 DTO。

| 字段 | 用途 |
| --- | --- |
| `event_id` | 稳定事件身份 |
| `run_id` / `turn_id` | 当前运行与回合 |
| `message_id` | 同一模型消息的开始、增量和完整内容 |
| `tool_call_ref` | 工具参数增量、完整消息中的工具调用与工具执行 |
| `parent_tool_call_id` | 成员执行所属的父工具调用 |
| `session_sequence` | 领域事件在长期会话中的提交顺序，持久化后保留 |
| `sequence` | 事件序号；客户端无需用它重建 Web 展示 |
| `stream_event_id` | 传输游标，等于 SSE `id`；仅用于续接 |

连接确认、心跳等传输控制消息不一定有完整运行身份。不要用时间、工具名前缀或相邻事件推测成员归属。

## 主要事件

| 类型 | 含义 |
| --- | --- |
| `agent_started` / `agent_completed` | 智能体运行开始、结束 |
| `turn_started` / `turn_completed` / `turn_failed` / `turn_cancelled` | 回合生命周期 |
| `assistant_started` / `assistant_completed` | 模型消息开始、完整消息 |
| `assistant_text_delta` / `assistant_reasoning_delta` | 正文与思考增量，载荷统一使用 `content` |
| `tool_call_started` / `tool_call_arguments_delta` / `tool_call_result_delta` / `tool_call_completed` / `tool_call_failed` | 工具生命周期 |
| `ask_requested` / `ask_answered` | 提问及用户回答，以 `ask_id` 关联 |
| `approval_requested` / `approval_answered` | 审批请求与决定 |
| `member_started` / `member_completed` | 成员执行状态 |
| `queue_updated` | 已提交的待消费队列快照 |
| `system_notice` / `usage_reported` / `error` | 提示、用量与错误 |
| `processing_start` / `processing_end` | 任务处理开始、最终收尾 |

`assistant_completed` 可以包含一组 `tool_calls[]`，不能只读取第一项。成员工具携带 `kind=agent`、展示名和目标。`usage` 的缓存计数已经包含在输入 token 总数内，不另加到总用量。

## Web 展示契约

会话、同步聊天、流式快照与分享接口返回 `view: {items: [...]}`；实时事件通过 `view_update` 更新它。

- `reset`：替换完整视图。
- `remove`：按 ID 删除节点。
- `items`：按服务端 ID 替换节点，并放到指定 `index`。
- `appends`：按 `item_id` 和分段 `key` 追加正文或思考。

节点类型为 message、member、ask、approval 或 error。顺序、分段、工具成败和成员归属由 `app/chat/presentation.Projector` 统一确定，前端不从原始事件重新推导。更新找不到目标时重新获取快照。

重连先取快照及 `next_offset`，再从该游标订阅。只有 `processing_end` 的 `completed/error/cancelled` 或权威终态快照确认任务结束；单独的 error 事件不表示收尾完成。

## 历史与兼容

transcript 保留规范 `message` 和原始事件 `envelope`，实时与重载经同一投影器生成展示。主线与成员记录分开，长工具结果使用外置引用；默认存储位置见[存储说明](storage.md)。

旧历史缺少规范事件信封时，Web 返回 409 并提示新建会话，原数据保留。客户端不补造消息身份或排序。

协议类型与校验见 `web/src/types/events.ts`、`web/src/types/presentation.ts`。Go/Bun 共用样本位于 `web/src/types/testdata/chat-events.json` 和 `chat-view.json`，修改协议时同步更新样本与测试。
