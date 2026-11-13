# 聊天 API

| 入口 | 用途 |
| --- | --- |
| `POST /api/fkteams/chat` | 等待任务结束后返回 JSON |
| `POST /api/fkteams/stream/start` | 后台任务，配合 SSE；见[流式任务](stream.md) |
| `GET /ws` | WebSocket 对话、订阅与交互 |

认证和响应包装见 [API 总览](README.md)。需要审批、提问或运行中输入时使用流式任务或 WebSocket。

## POST /api/fkteams/chat

请求示例：

```json
{"session_id":"example-session","message":"分析这个问题","mode":"team"}
```

| 字段 | 说明 |
| --- | --- |
| `session_id` | 可选，不提供时生成 UUID；普通 ID 禁止路径分隔符、控制字符和 `.` / `..` |
| `message` | 文本，与 `contents` 至少提供一个 |
| `mode` | 默认 `team`；可用 `team`、`deep`、`roundtable`（CLI 对应 -m group） |
| `agent_name` | 指定单个智能体，优先于 mode |
| `contents` | 多模态数组；存在时优先作为输入 |

同步响应的 `data`：

```json
{
  "session_id": "example-session",
  "content": "完整回复文本",
  "events": [],
  "view": {"items": []}
}
```

`content` 汇总主智能体的可见回答，`events` 为执行事件，`view` 是后端生成的最终展示快照。历史与会话状态保存成功后才返回成功。同一会话执行期间不能追加同步请求或队列，冲突返回 409；`stream=true` 返回 400，请使用后台任务接口。

## 多模态输入

HTTP、WebSocket 和流式启动均接受相同的 `contents`：

```json
{
  "message": "",
  "contents": [
    {"type":"text","text":"图片中有什么？"},
    {"type":"image_url","url":"https://example.com/cat.jpg","detail":"high"}
  ]
}
```

| type | 字段 |
| --- | --- |
| `text` | `text` |
| `image_url` | `url`，可选 `detail: high/low/auto` |
| `image_base64` | `base64_data`、`mime_type`（默认 image/png） |
| `audio_url` / `video_url` / `file_url` | `url`，或上游支持的 `file_id` |

附件可带 `name`。`file_id` 是模型供应商托管引用，不能当作本地路径。实际输入能力及历史附件行为见[多模态支持](../multimodal.md)。

## WebSocket

连接 `ws://<host>/ws` 或 `wss://<host>/ws`，认证使用 Cookie 或握手 Bearer Header。连接后收到 `connected`；服务端逐条消息复核认证，关闭应用时关闭连接。单条消息上限 32 MiB。

客户端消息以 `type` 区分：

| type | 字段与行为 |
| --- | --- |
| `chat` / `follow_up` | 必须有 `session_id`；其余输入字段同 HTTP。会话运行中时追加 follow-up |
| `steer` / `steering` | `session_id`、`message` / `contents`；下次模型调用前消费 |
| `resume` | `session_id`、`offset`；从该传输游标回放并订阅 |
| `cancel` | `session_id`；请求停止任务，等待真正结束 |
| `approval` | `session_id`、`decision`；0 拒绝、1 允许一次、2 允许该项、3 全部允许 |
| `ask_answered` | `session_id`、`ask_id`、`ask_selected`、`ask_free_text` |
| `ping` | 返回 `pong` |

示例：

```json
{"type":"chat","session_id":"example-session","message":"检查工作区","agent_name":"coder"}
```

```json
{"type":"ask_answered","session_id":"example-session","ask_id":"ask-1","ask_selected":["方案 A"],"ask_free_text":""}
```

`ask_id` 应原样带回，以便并行成员的问题分别路由。未知消息类型返回 error。

服务端任务事件与 SSE 共用结构，展示使用 `view_update`，类型见[事件协议](../events.md)。WebSocket 订阅事件携带 `stream_event_id`；重连前先获取[快照](stream.md#get-apifkteamsstreamsnapshotsessionid)，再将 `next_offset` 用于 resume。缓存窗口过期时重新加载快照，不能靠旧事件推测缺失内容。

`cancel` 的即时确认不表示任务已退出。以 `processing_end` 的终态或后续快照为准；无内存任务的 resume 只返回“任务已完成或不存在”，应再读取会话快照。
