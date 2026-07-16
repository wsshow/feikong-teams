# 聊天 API

聊天能力有三种入口：

| 入口 | 适用场景 | 特点 |
| ---- | -------- | ---- |
| `POST /api/fkteams/chat` | 普通 HTTP 调用 | 同步 JSON 响应 |
| `GET /ws` | Web 前端交互 | WebSocket 推送、HITL、运行中队列、断线后可 `resume` |
| `POST /api/fkteams/stream/start` | 推荐的后台任务入口 | 任务与连接解耦，详见 [流式任务 API](stream.md) |

## POST /api/fkteams/chat

通过 HTTP 发送聊天消息，返回同步 JSON 响应。实时流式输出统一使用 [流式任务 API](stream.md)。

> HTTP 聊天默认使用非交互式中断处理。需要完整 HITL 审批或运行中排队管理时，使用 WebSocket 或后台流式任务接口。

**请求 Body**：

```json
{
  "session_id": "可选，不提供则自动生成 UUID",
  "message": "string",
  "mode": "team",
  "agent_name": "string",
  "contents": []
}
```

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `session_id` | string | 否 | 会话 ID；不提供时自动生成 UUID |
| `message` | string | 条件 | 用户文本，和 `contents` 至少提供一个 |
| `mode` | string | 否 | 运行模式，默认 `team`；具体值由 Runner 缓存解析 |
| `agent_name` | string | 否 | 指定单个智能体，优先于 `mode` |
| `contents` | array | 条件 | 多模态内容；存在时优先用于构建输入 |

`contents` 元素结构：

```json
{
  "type": "text",
  "text": "文本内容",
  "url": "https://example.com/image.png",
  "base64_data": "...",
  "mime_type": "image/png",
  "detail": "auto"
}
```

| `type` | 使用字段 |
| ------ | -------- |
| `text` | `text` |
| `image_url` | `url`、`detail`，`detail` 支持 `high`、`low`、`auto` |
| `image_base64` | `base64_data`、`mime_type`，`mime_type` 默认 `image/png` |
| `audio_url` | `url` |
| `video_url` | `url` |
| `file_url` | `url` |

**同步响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "content": "完整回复文本",
    "events": []
  }
}
```

`events` 是原始 Agent 事件数组，按执行顺序返回。

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid request: ...` | 请求体解析失败 |
| 400 | `message or contents is required` | 消息为空 |
| 400 | `stream=true is not supported on /api/fkteams/chat; use /api/fkteams/stream/start` | 请求内 SSE 已移除 |
| 400 | Runner 错误详情 | `agent_name` 指定的智能体不可用 |
| 500 | Runner 错误详情 | Runner 创建或执行失败 |

## WebSocket

### 连接

```text
ws://<host>/ws
wss://<host>/ws
```

启用登录认证时，Token 可以通过 `?token=<token>` 或 `fk_token` Cookie 传递。服务端会对连接中的每条消息再次校验 Token，以响应登录过期和认证配置热更新。连接建立后服务端发送：

```json
{
  "type": "connected",
  "message": "欢迎连接到非空小队"
}
```

服务关闭时，后端会主动关闭所有连接。

### 客户端消息结构

```json
{
  "type": "chat",
  "session_id": "会话 ID",
  "offset": 0,
  "message": "用户消息",
  "mode": "team",
  "agent_name": "coder",
  "decision": 1,
  "contents": [],
  "ask_selected": ["选项 A"],
  "ask_free_text": "补充文本"
}
```

字段按消息类型选择性填写。

### chat / follow_up

发送用户消息。WebSocket 的 `chat` 和 `follow_up` 必须显式携带 `session_id`。

```json
{
  "type": "chat",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "message": "帮我分析这个问题",
  "mode": "team"
}
```

如果会话已有运行中任务，消息会追加为 `follow_up` 队列项，服务端推送：

- `user_message`，包含 `queued=true`、`queue_id`、`queue_kind`、`queued_count`
- `queue_updated`，包含最新 `queue` 快照

### steer / steering

向运行中任务追加 steering。

```json
{
  "type": "steer",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "message": "先检查最新日志，再继续原任务"
}
```

失败时返回 `type=error`，例如 `no running task to steer`。

### resume

断线后重新绑定当前会话的内存流，并从指定事件 offset 回放。

```json
{
  "type": "resume",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "offset": 42
}
```

如果任务不存在或已结束，返回：

```json
{
  "type": "processing_end",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "message": "任务已完成或不存在"
}
```

### cancel

请求取消运行中的任务。

```json
{
  "type": "cancel",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

服务端立即确认取消请求：

```json
{
  "type": "cancellation_requested",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "message": "取消请求已发送"
}
```

任务真正取消后还会推送 `cancelled` 事件。

### approval

提交 HITL 审批决定。

```json
{
  "type": "approval",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "decision": 1
}
```

| decision | 含义 |
| -------- | ---- |
| `0` | 拒绝 |
| `1` | 允许一次 |
| `2` | 允许该项 |
| `3` | 全部允许 |

### ask_answered

提交 `ask_requested` 的回答。

```json
{
  "type": "ask_answered",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "ask_id": "ask-id-from-event",
  "ask_selected": ["选项 A"],
  "ask_free_text": "补充说明"
}
```

当 `ask_requested` 事件包含 `ask_id` 时，提交回答应带回同一个 `ask_id`。并行子智能体同时提问时，服务端会按 `ask_id` 将回答投递给对应成员。

### ping

```json
{
  "type": "ping"
}
```

服务端返回：

```json
{
  "type": "pong"
}
```

未知 `type` 返回：

```json
{
  "type": "error",
  "error": "unknown message type"
}
```

## 服务端事件

WebSocket 和后台流式任务共享同一套任务事件结构。常见控制事件：

| type | 说明 |
| ---- | ---- |
| `connected` | WebSocket 连接建立 |
| `user_message` | 用户消息；排队时带 `queued`、`queue_id`、`queue_kind` |
| `processing_start` | 某个轮次开始处理 |
| `processing_end` | 当前任务全部完成 |
| `queue_updated` | 未消费队列快照 |
| `ask_requested` | 需要用户回答问题；并行成员提问会携带 `ask_id` 和成员归属字段 |
| `approval_required` | 需要用户审批 |
| `cancelled` | 任务被取消 |
| `error` | 错误 |
| `pong` | 心跳响应 |

Agent 事件基础字段：

```json
{
  "type": "assistant_text_delta",
  "session_id": "string",
  "run_id": "string",
  "turn_id": "string",
  "message_id": "string",
  "agent_name": "string",
  "run_path": "string",
  "role": "assistant",
  "delta_kind": "output",
  "content": "string",
  "reasoning_content": "string",
  "tool_calls": [],
  "tool_call_ref": "string",
  "tool_name": "string",
  "tool_display_name": "string",
  "tool_kind": "string",
  "detail": "string",
  "error": "string",
  "event_id": "evt_1",
  "sequence": 1,
  "created_at": "2026-06-30T00:00:00Z"
}
```

字段均为按需返回。业务展示顺序只使用 `sequence`；WebSocket/SSE 续接使用对应传输层 offset。

常见 Agent `type`：

| type | 说明 |
| ---- | ---- |
| `agent_started` / `agent_completed` | 智能体执行开始/结束 |
| `turn_started` / `turn_completed` | 轮次开始/结束 |
| `assistant_started` / `assistant_reasoning_delta` / `assistant_text_delta` / `assistant_completed` | 模型消息生命周期 |
| `tool_call_started` / `tool_call_arguments_delta` / `tool_call_result_delta` / `tool_call_completed` | 工具调用生命周期 |
| `ask_requested` / `ask_answered` | 用户提问与回答记录 |
| `approval_requested` / `approval_answered` | 审批请求与审批结果 |
| `system_notice` | 系统状态提示，例如上下文压缩或转交提示 |
| `usage_reported` | Token 用量 |
| `error` | 执行错误 |
| `member_started` / `member_completed` | 成员智能体执行状态 |
| `context_compress_start` / `context_compress` | 上下文压缩 |

## 队列顺序语义

运行中产生的队列项必须按用户交互顺序渲染：

1. 当前用户问题产生 `user_message` 和 `processing_start`。
2. 当前智能体持续推送回复事件。
3. 运行中追加的问题先以 `queued=true` 展示在队列区。
4. 当队列项开始执行时，服务端推送带 `queued_executing=true` 的 `user_message` 或 `processing_start`，客户端再把它提升为正式轮次。

前端不应仅按接收时间把排队问题直接追加到结果流后面；应优先使用 `run_id`、`turn_id`、`queue_id` 和 `queued_executing` 维护轮次归属。
