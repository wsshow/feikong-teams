# 流式任务 API

流式任务 API 用于把任务执行与前端连接解耦。任务在服务端后台运行，事件缓存在内存中；客户端可以通过 SSE 订阅实时事件，也可以用 HTTP 拉取当前缓冲。

基础路径：`/api/fkteams/stream`

## 核心流程

1. `POST /start` 启动后台任务，返回 `session_id`。
2. `GET /subscribe/:sessionID` 订阅 SSE 实时事件。
3. 页面刷新或重连时先 `GET /status/:sessionID` 判断任务是否仍在内存中。
4. 运行中可用 `/steer` 注入 steering，或再次 `/start` 追加 follow-up。
5. 可用 `/queue/*` 管理尚未消费的队列项。
6. HITL 场景通过 `/approval` 或 `/ask-response` 提交用户输入。

## POST /api/fkteams/stream/start

启动后台流式任务。如果同一 `session_id` 已有运行中任务，请求会被追加为 `follow_up` 队列项，不会取消当前任务。

**请求 Body**：

```json
{
  "session_id": "可选，不提供则自动生成 UUID",
  "message": "用户消息",
  "mode": "team",
  "agent_name": "可选，指定单个智能体",
  "contents": []
}
```

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `session_id` | string | 否 | 会话 ID；提供时必须是合法会话 ID |
| `message` | string | 条件 | 文本消息，和 `contents` 至少提供一个 |
| `mode` | string | 否 | 运行模式，默认 `team`；支持值由 Runner 缓存解析 |
| `agent_name` | string | 否 | 指定单个智能体，优先于 `mode` |
| `contents` | array | 条件 | 多模态内容，结构同 [聊天接口](chat.md) |

**启动成功**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "processing",
    "message": "task started"
  }
}
```

**运行中追加 follow-up**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "queued",
    "message": "message queued",
    "queue_kind": "follow_up",
    "queued_count": 1
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid request: ...` | 请求体解析失败 |
| 400 | `message or contents is required` | 消息为空 |
| 400 | `invalid session ID` | 会话 ID 不合法 |
| 400 | Runner 错误详情 | `agent_name` 指定的智能体不可用 |
| 500 | Runner 错误详情 | Runner 创建失败 |

## POST /api/fkteams/stream/steer

向运行中的任务追加 steering。steering 会在下一次模型调用前注入上下文，不会打断正在输出的 token 或正在执行的工具。

**请求 Body**：

```json
{
  "session_id": "abc-123",
  "message": "先处理这个新约束",
  "contents": []
}
```

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "queued",
    "message": "steering queued",
    "queue_kind": "steering",
    "queued_count": 1
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `session_id is required` | 缺少会话 ID |
| 400 | `message or contents is required` | 消息为空 |
| 400 | `invalid session ID` | 会话 ID 不合法 |
| 404 | `no running task for this session` | 会话没有运行中任务 |

## 队列管理

运行中追加的 `follow_up` 和 `steering` 都会进入未消费队列。每个队列项包含：

```json
{
  "id": "queue-id",
  "kind": "follow_up",
  "text": "原始文本",
  "parts": [],
  "display_text": "展示文本",
  "created_at": "2026-06-10T10:00:00Z",
  "updated_at": "2026-06-10T10:01:00Z"
}
```

队列快照按 `steering` 在前、`follow_up` 在后返回。移动排序只在同类队列内生效。

### GET /api/fkteams/stream/queue/:sessionID

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "queue": [],
    "queued_count": 0
  }
}
```

### PATCH /api/fkteams/stream/queue/:sessionID/:queueID

修改尚未消费的队列项内容。

```json
{
  "message": "更新后的内容",
  "contents": []
}
```

### DELETE /api/fkteams/stream/queue/:sessionID/:queueID

删除尚未消费的队列项。

### POST /api/fkteams/stream/queue/:sessionID/:queueID/kind

切换队列项类型。

```json
{
  "kind": "steering"
}
```

`kind` 只允许 `follow_up` 或 `steering`。

### POST /api/fkteams/stream/queue/:sessionID/:queueID/move

调整队列项在同类队列内的顺序。

```json
{
  "direction": "up"
}
```

`direction` 只允许 `up` 或 `down`。

**队列修改成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "queue_item": {},
    "queue": []
  }
}
```

队列发生变化时，服务端会同时推送 `queue_updated` 事件。

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid session ID` | 会话 ID 不合法 |
| 400 | `message or contents is required` | PATCH 内容为空 |
| 400 | `kind must be follow_up or steering` | 队列类型非法 |
| 400 | `direction must be up or down` | 移动方向非法 |
| 404 | `no running task for this session` | 无运行中任务 |
| 404 | `queued message not found` | 队列项不存在或已被消费 |

## GET /api/fkteams/stream/subscribe/:sessionID

SSE 订阅后台任务事件。支持通过 `Last-Event-ID` 或 `?offset=N` 从指定事件继续接收。

```http
GET /api/fkteams/stream/subscribe/abc-123?offset=0
Accept: text/event-stream
```

SSE 事件格式：

```text
id: 42
data: {"type":"message_delta","session_id":"abc-123","content":"...","stream_event_id":42}
```

服务端优先使用 `Last-Event-ID`，并从 `Last-Event-ID + 1` 开始回放；没有该 header 时使用 `offset` query 参数。

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid session ID` | 会话 ID 不合法 |
| 404 | `no active task for this session` | 内存中没有活跃或保留中的任务 |

## GET /api/fkteams/stream/events/:sessionID

一次性拉取当前内存流中的已缓冲事件。

**Query 参数**：

| 参数 | 类型 | 默认 | 说明 |
| ---- | ---- | ---- | ---- |
| `offset` | uint | `0` | 起始事件 ID |

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "processing",
    "events": [
      {
        "id": 0,
        "data": {
          "type": "processing_start",
          "session_id": "abc-123",
          "stream_event_id": 0
        }
      }
    ],
    "event_count": 1,
    "done": false
  }
}
```

## GET /api/fkteams/stream/status/:sessionID

查询任务状态。若内存中有任务，返回任务状态；若没有任务但会话历史存在，返回会话元数据；都不存在则返回 404。

**有任务时**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "processing",
    "has_task": true,
    "mode": "team",
    "agent_name": "coder",
    "event_count": 42,
    "created_at": "2026-06-10T10:00:00Z",
    "finished_at": "2026-06-10T10:05:00Z"
  }
}
```

`agent_name` 仅在启动任务时指定了智能体时返回；`finished_at` 仅任务已结束且仍在内存保留期内返回。

**无任务但会话存在时**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "completed",
    "has_task": false,
    "title": "会话标题",
    "created_at": "2026-06-10T10:00:00Z",
    "updated_at": "2026-06-10T10:05:00Z"
  }
}
```

## POST /api/fkteams/stream/stop/:sessionID

请求停止运行中的后台任务。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "message": "task stop requested"
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid session ID` | 会话 ID 不合法 |
| 404 | `no task found for this session` | 没有任务 |
| 409 | `task is not running, current status: ...` | 任务已结束或不在运行中 |

## POST /api/fkteams/stream/approval

提交 HITL 审批决定。

**请求 Body**：

```json
{
  "session_id": "abc-123",
  "decision": 1
}
```

| decision | 含义 |
| -------- | ---- |
| `0` | 拒绝 |
| `1` | 允许一次 |
| `2` | 允许该项 |
| `3` | 全部允许 |

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "approval submitted"
  }
}
```

## POST /api/fkteams/stream/ask-response

提交 `ask_questions` 的回答。

**请求 Body**：

```json
{
  "session_id": "abc-123",
  "ask_id": "ask-id-from-event",
  "selected": ["选项 A"],
  "free_text": "补充说明"
}
```

当 `ask_questions` 事件包含 `ask_id` 时，提交回答应带回同一个 `ask_id`。并行子智能体同时提问时，服务端会按 `ask_id` 将回答投递给对应成员。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "response submitted"
  }
}
```

`/approval` 和 `/ask-response` 失败响应：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid request body` | 请求体解析失败或缺少 `session_id` |
| 400 | `invalid session ID` | 会话 ID 不合法 |
| 404 | `no running task for this session` | 没有运行中任务 |
| 409 | `no pending approval request` | 当前没有待审批请求 |
| 409 | `no pending ask request` | 当前没有待回答请求 |

## 事件结构

后台任务事件由内部 Agent 事件转换而来，常见字段如下：

| 字段 | 说明 |
| ---- | ---- |
| `type` | 事件类型 |
| `session_id` | 会话 ID |
| `stream_event_id` | 当前内存流事件 ID，和 SSE `id` 一致 |
| `run_id` | 当前用户轮次或队列项运行 ID |
| `turn_id` | 当前运行中的轮次 ID |
| `message_id` | 模型消息 ID，用于合并增量 |
| `agent_name` | 事件所属智能体 |
| `content` | 文本内容 |
| `delta` | `message_delta` 的文本增量 |
| `delta_kind` | 增量类型，如正文或工具参数 |
| `tool_calls` | 工具调用列表 |
| `tool_call_ref` | 工具调用稳定引用 |
| `queue_id` | 队列项 ID |
| `queue_kind` | `follow_up` 或 `steering` |
| `queued` | 用户消息已排队 |
| `queued_executing` | 队列项开始执行 |

常见 `type`：

| type | 说明 |
| ---- | ---- |
| `user_message` | 用户消息；排队时包含 `queued` 和队列字段 |
| `processing_start` | 某个用户轮次或队列项开始处理 |
| `processing_end` | 任务全部完成 |
| `queue_updated` | 队列快照更新 |
| `message_delta` | 模型输出增量 |
| `message_end` | 模型消息结束 |
| `tool_start` / `tool_update` / `tool_end` | 工具执行事件 |
| `ask_questions` | 需要用户回答问题；并行成员提问会携带 `ask_id` 和成员归属字段 |
| `approval_required` | 需要用户审批 |
| `error` | 任务错误 |
| `cancelled` | 任务被取消 |

## 页面恢复建议

刷新页面或重新打开会话时：

1. `GET /api/fkteams/sessions/:sessionID` 加载已持久化历史。
2. `GET /api/fkteams/stream/status/:sessionID` 判断是否存在内存任务。
3. 若 `has_task=true`，用 `GET /api/fkteams/stream/events/:sessionID?offset=0` 拉取当前轮次缓冲。
4. 再用 `GET /api/fkteams/stream/subscribe/:sessionID?offset=<last_id+1>` 接入实时流。
5. 若状态接口返回 404，则该会话不存在或未保存，客户端应切回主页。

任务完成后内存流会短暂保留，完整历史以会话 API 为准。
