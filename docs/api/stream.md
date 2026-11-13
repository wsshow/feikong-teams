# 流式任务 API

任务在服务端后台运行，连接断开不取消任务。基础路径 `/api/fkteams/stream`，认证与响应包装见 [API 总览](README.md)。

## 接入流程

1. `POST /start` 提交任务，取得 `session_id`。
2. `GET /snapshot/:sessionID` 读取 `view`、`queue`、状态和 `next_offset`。
3. 若有内存任务，从 `next_offset` 调用 `/subscribe/:sessionID` 接收 SSE。
4. 应用事件中的 `view_update`；等待 `processing_end` 确认最终状态。
5. 重连时重新读取快照；缓存过期后从已提交历史恢复。

## POST /api/fkteams/stream/start

输入字段同[同步聊天](chat.md#post-apifkteamschat)：

```json
{"session_id":"example-session","message":"整理工作区资料","mode":"team"}
```

启动成功的 `data`：

```json
{
  "session_id": "example-session",
  "status": "processing",
  "message": "task started",
  "user_message_id": "run-turn:user",
  "queue": [],
  "queued_count": 0
}
```

同一会话已有运行任务时，新输入进入 follow-up，返回 `status: queued`、`queue_kind: follow_up` 和最新队列。当前任务收尾中则返回 409，消息未入队，稍后可重试。

## POST /api/fkteams/stream/steer

```json
{"session_id":"example-session","message":"先检查最新日志","contents":[]}
```

steering 在下一次模型调用前消费，不打断当前输出或工具。成功返回 `status: queued`、`queue_kind: steering`、`queue`、`queued_count`；无运行任务返回 404，正在收尾返回 409。

## 队列管理

队列项使用稳定的 `queue_id`：

```json
{
  "queue_id": "queue-1",
  "kind": "follow_up",
  "text": "补充要求",
  "parts": [],
  "display_text": "补充要求",
  "created_at": "2026-11-13T10:00:00Z",
  "updated_at": "2026-11-13T10:00:00Z"
}
```

以下路径均以 `/api/fkteams/stream` 为前缀：

| 方法与路径 | 请求 / 行为 |
| --- | --- |
| `GET /queue/:sessionID` | 返回 `{session_id, queue, queued_count}` |
| `PATCH /queue/:sessionID/:queueID` | `{message, contents}` 修改内容，两者至少一个 |
| `DELETE /queue/:sessionID/:queueID` | 删除未消费项 |
| `POST /queue/:sessionID/:queueID/kind` | `{kind: "follow_up"}` 或 `"steering"` |
| `POST /queue/:sessionID/:queueID/move` | `{direction: "up"}` 或 `"down"` |

队列按 steering、follow-up 分组返回，同类内排序。只能修改尚未消费的项，离线会话同样可管理已保存队列；不存在或已消费返回 404。保存成功后才确认修改，并通过 `queue_updated` 同步。取消后未消费输入保留至下次运行。

## GET /api/fkteams/stream/snapshot/:sessionID

一次返回完整展示快照，不分页读取原始事件：

```json
{
  "session_id": "example-session",
  "status": "processing",
  "has_task": true,
  "mode": "team",
  "queue": [],
  "view": {"items": []},
  "next_offset": 42,
  "created_at": "2026-11-13T10:00:00Z"
}
```

`view` 与 `next_offset` 在同一锁内读取。指定智能体时包含 `current_agent`，结束后可含 `finished_at`。内存任务不存在时返回[会话详情](sessions.md#get-apifkteamssessionssessionid)，包含从 transcript 恢复的展示；会话也不存在返回 404。旧历史格式不支持展示时返回 409，原数据保留。

## GET /api/fkteams/stream/subscribe/:sessionID

```http
GET /api/fkteams/stream/subscribe/example-session?offset=42
Accept: text/event-stream
```

SSE 帧：

```text
id: 42
data: {"type":"assistant_text_delta","session_id":"example-session","message_id":"msg-1","content":"正文","stream_event_id":42,"view_update":{"appends":[{"item_id":"item-1","key":"part-1","content":"正文","message_id":"msg-1","type":"text"}]}}

```

示例为已有分段的追加，实际节点和分段 ID 来自之前的快照或更新。展示更新格式见[事件协议](../events.md#web-展示契约)。

- `offset` 是下一条待读取的传输 ID。若有 `Last-Event-ID` Header，则优先从该值加一开始。
- 缓存窗口过期返回 409，重新取快照；无内存流返回 404，改读会话详情。
- 每 15 秒发送心跳注释；任务结束并发送完尾部事件后发送 `data: [DONE]`。
- `processing_end.status` 为 `completed`、`error` 或 `cancelled`，并携带最终展示更新。原始 error 事件不能代替终态。
- 认证过期或变更会关闭订阅，后台任务继续；重新登录后按快照游标恢复。

## 状态与事件拉取

| 接口 | 返回 |
| --- | --- |
| `GET /api/fkteams/stream/status/:sessionID` | 内存任务的 status、has_task、mode、event_count、created_at，及可选 agent_name、finished_at；无任务时返回会话 metadata，均不存在返回 404 |
| `GET /api/fkteams/stream/events/:sessionID` | 分页原始事件，用于程序消费或调试；无内存流返回 404 |

事件拉取支持 `offset`（默认 0）和 `limit`（默认 300、最大 1000），返回：

```json
{
  "session_id": "example-session",
  "status": "processing",
  "events": [{"id":42,"data":{"type":"processing_start","session_id":"example-session"}}],
  "event_count": 43,
  "retained_count": 43,
  "earliest_offset": 0,
  "replay_truncated": false,
  "next_offset": 43,
  "more_available": false,
  "limit": 300,
  "done": false
}
```

`more_available=true` 时用 `next_offset` 继续，`replay_truncated=true` 时重新获取展示快照。事件分页不用于拼装 Web 初始视图。

## 停止与人工输入

`POST /api/fkteams/stream/stop/:sessionID` 请求停止任务，成功返回 `{session_id, message: "task stop requested"}`；无任务返回 404，任务已结束返回 409。停止请求的成功响应不表示收尾已经结束。

`POST /api/fkteams/stream/approval` 提交审批：

```json
{"session_id":"example-session","decision":1}
```

`decision`：0 拒绝、1 允许一次、2 允许该项、3 全部允许。成功返回 `{message: "approval submitted"}`。

`POST /api/fkteams/stream/ask-response` 提交问题回答：

```json
{"session_id":"example-session","ask_id":"ask-1","selected":["方案 A"],"free_text":"补充说明"}
```

`ask_id` 应与请求事件一致，用于区分并行成员提问。成功返回 `{message: "response submitted"}`。这两个接口均在无运行任务时返回 404，无对应待处理输入时返回 409。
