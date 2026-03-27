# 聊天接口

## POST /api/fkteams/chat

通过 HTTP 发送聊天消息，支持同步和 SSE 流式两种响应模式。

> SSE 和同步模式使用 `AutoRejectHandler` 自动拒绝危险命令（无人工审批流程）。WebSocket 模式支持完整的人工审批（HITL）流程，详见下方 WebSocket 协议说明。

**请求 Body**：

```json
{
  "session_id": "string",
  "message": "string",
  "mode": "string",
  "agent_name": "string",
  "stream": false,
  "contents": []
}
```

| 字段         | 类型   | 必填 | 说明                                                                                                                |
| ------------ | ------ | ---- | ------------------------------------------------------------------------------------------------------------------- |
| `message`    | string | 条件 | 用户输入的文本（`message` 和 `contents` 至少提供一个）                                                              |
| `session_id` | string | 否   | 会话标识，默认 `"default"`                                                                                          |
| `mode`       | string | 否   | 运行模式：`supervisor`（默认）、`roundtable`、`custom`、`deep`                                                      |
| `agent_name` | string | 否   | 指定单个智能体直接对话（优先级高于 mode）                                                                           |
| `stream`     | bool   | 否   | 是否使用 SSE 流式响应，默认 `false`                                                                                 |
| `contents`   | array  | 否   | 多模态内容部分（存在时优先于 `message`），每项包含 `type`、`text`、`url`、`base64_data`、`mime_type`、`detail` 字段 |

**同步响应** (`stream: false`)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "default",
    "content": "完整的回复文本",
    "events": []
  }
}
```

`events` 数组中每个元素为 Agent 事件对象（结构见 [Agent 事件消息](#agent-事件消息)），按执行顺序排列。

**SSE 流式响应** (`stream: true`)：

返回 `text/event-stream`（`Cache-Control: no-cache`，`Connection: keep-alive`），每个事件的格式为：

```
data: {"type":"stream_chunk","agent_name":"小码","content":"..."}

data: {"type":"processing_end","message":"处理完成"}

```

事件结构与 WebSocket 的 Agent 事件消息相同。

**失败响应**：

| 状态码 | message                         | 说明                       |
| ------ | ------------------------------- | -------------------------- |
| 400    | invalid request: \<详情\>       | 请求体解析失败             |
| 400    | message or contents is required | message 和 contents 均为空 |
| 400    | agent not found: \<agent_name\> | 指定的智能体不存在         |
| 500    | \<错误详情\>                    | Runner 创建或其他内部错误  |

---

## WebSocket

### 连接

- **URL**：`ws://<host>/ws` 或 `wss://<host>/ws`
- **认证**：启用认证时通过 `?token=<token>` 参数传递
- **连接建立后**：服务器自动发送 `connected` 消息
- **服务关闭时**：服务端主动关闭所有连接（`CloseGoingAway`，消息 `"server shutting down"`）

### 客户端 → 服务器

所有消息使用统一 JSON 结构：

```json
{
  "type": "string",
  "session_id": "string",
  "message": "string",
  "mode": "string",
  "agent_name": "string",
  "decision": 0,
  "contents": [
    {
      "type": "text|image_url|image_base64|audio_url|video_url|file_url",
      "text": "string",
      "url": "string",
      "base64_data": "string",
      "mime_type": "string",
      "detail": "high|low|auto"
    }
  ]
}
```

> 所有字段均为 `omitempty`，按消息类型选择性填写。

#### chat — 发送聊天消息

| 字段         | 类型   | 说明                                                           |
| ------------ | ------ | -------------------------------------------------------------- |
| `session_id` | string | 会话标识，默认 `"default"`                                     |
| `message`    | string | 用户输入的文本                                                 |
| `mode`       | string | 运行模式：`supervisor`（默认）、`roundtable`、`custom`、`deep` |
| `agent_name` | string | 指定单个智能体直接对话（优先级高于 mode）                      |
| `contents`   | array  | 多模态内容部分（可选，存在时优先于 `message` 字段）            |

**处理流程**：

1. 创建独立可取消 context（`context.WithCancel(connCtx)`）
2. 获取或创建会话的 HistoryRecorder（自动加载文件历史）
3. 构建输入消息（如有历史记录则注入 SystemMessage 作为上下文摘要）
4. 根据 `agent_name` 或 `mode` 获取/创建 Runner（带缓存）
5. 更新会话 title（首次提交时从默认标题更新为用户输入）和 status 为 `"processing"`
6. 发送 `processing_start` → 执行 Runner（支持 HITL 审批中断）→ 发送 `processing_end`
7. 保存聊天历史到文件，更新 status 为 `"completed"`

**Runner 创建规则**：

| 条件              | Runner 类型         |
| ----------------- | ------------------- |
| `agent_name` 非空 | 单智能体 Runner     |
| `mode=roundtable` | 圆桌会议 Runner     |
| `mode=custom`     | 自定义监督者 Runner |
| `mode=deep`       | 深度分析 Runner     |
| 其他              | 默认监督者 Runner   |

#### cancel — 取消当前任务

取消该连接正在执行的任务（取消任务 context）。

**服务器响应**：

```json
{
  "type": "cancelled",
  "session_id": "550e8400-...",
  "message": "任务已取消"
}
```

#### approval — 审批决定（HITL）

当 Agent 执行危险操作（如命令执行、文件修改、子任务分发）时，服务端会发送 `approval_required` 事件等待用户审批，客户端通过此消息回复审批决定。

| 字段       | 类型 | 说明                                                         |
| ---------- | ---- | ------------------------------------------------------------ |
| `decision` | int  | 审批决定：`0` 拒绝，`1` 允许一次，`2` 允许该项，`3` 全部允许 |

**审批流程**：

1. Agent 调用需审批的工具 → 触发中断
2. 服务端发送 `approval_required` 事件（含审批描述）
3. 客户端展示审批 UI 并等待用户操作
4. 客户端发送 `{"type": "approval", "decision": <int>}`
5. Agent 根据决定继续或中止执行

#### clear_history — 清除会话历史

> 此操作会**删除整个会话目录**（包括 history.json 和 metadata.json），并从内存中移除会话记录。

| 字段         | 类型   | 说明                              |
| ------------ | ------ | --------------------------------- |
| `session_id` | string | 要清除的会话 ID，默认 `"default"` |

**服务器响应**：

成功：`{"type": "history_cleared", "message": "历史记录已清除"}`

失败：`{"type": "error", "error": "清除历史失败"}`

#### load_history — 加载历史会话

| 字段      | 类型   | 说明                         |
| --------- | ------ | ---------------------------- |
| `message` | string | 要加载的会话 ID（UUID 格式） |

**服务器响应**：

```json
{
  "type": "history_loaded",
  "message": "历史记录已加载",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "messages": []
}
```

> 历史文件不存在时返回空 messages 数组（新建会话尚无历史），不报错。ID 不合法时返回 `{"type": "error", "error": "无效的会话 ID"}`。

#### ping — 心跳检测

**服务器响应**：`{"type": "pong"}`

#### 未知类型

发送未注册的 `type` 时，服务器返回：`{"type": "error", "error": "unknown message type"}`

---

### 服务器 → 客户端

#### 连接与控制消息

> 所有与聊天任务相关的消息（`processing_start`、`processing_end`、`cancelled`、`error`、Agent 事件）均携带 `session_id` 字段，用于多会话隔离。

| type               | 说明       | 附加字段                                           |
| ------------------ | ---------- | -------------------------------------------------- |
| `connected`        | 连接建立   | `message`（`"欢迎连接到非空小队"`）                |
| `error`            | 错误通知   | `error`，可能含 `session_id`                       |
| `pong`             | 心跳响应   | —                                                  |
| `cancelled`        | 任务已取消 | `session_id`、`message`                            |
| `history_cleared`  | 历史已清除 | `message`                                          |
| `history_loaded`   | 历史已加载 | `session_id`、`message`、`messages`                |
| `processing_start` | 开始处理   | `session_id`、`message`（`"开始处理您的请求..."`） |
| `processing_end`   | 处理完成   | `session_id`、`message`（`"处理完成"`）            |

#### Agent 事件消息

所有 Agent 事件均携带 `session_id`，完整基础结构：

```json
{
  "type": "<事件类型>",
  "session_id": "string",
  "agent_name": "string",
  "run_path": "string",
  "content": "string",
  "detail": "string",
  "reasoning_content": "string",
  "tool_calls": [],
  "action_type": "string",
  "error": "string"
}
```

> 所有字段均为可选（`omitempty`），仅在有值时出现。

| type                   | 触发场景                  | 关键字段                                            |
| ---------------------- | ------------------------- | --------------------------------------------------- |
| `message`              | Agent 输出完整消息        | `content`，可能含 `tool_calls`、`reasoning_content` |
| `tool_result`          | Tool 返回完整结果         | `content`                                           |
| `stream_chunk`         | 流式输出文本块            | `content`                                           |
| `reasoning_chunk`      | 推理/思考过程流式增量     | `content`（仅推理模型）                             |
| `tool_result_chunk`    | Tool 流式输出块           | `content`                                           |
| `tool_calls_preparing` | 识别到工具调用开始        | `tool_calls[].name`                                 |
| `tool_calls`           | 完整的工具调用信息        | `tool_calls[].name`、`tool_calls[].arguments`       |
| `action`               | Agent 执行动作            | `action_type`、`content`                            |
| `dispatch_progress`    | 子任务并行执行进度        | `action_type`、`detail`（见下方说明）               |
| `approval_required`    | 需要用户审批（HITL 中断） | `session_id`、`message`（审批描述）                 |
| `error`                | Agent 执行错误            | `error`                                             |

**action_type 子类型**：

| action_type         | 说明                     | content / 备注                                                  |
| ------------------- | ------------------------ | --------------------------------------------------------------- |
| `transfer`          | 转交到其他 Agent         | `"Transfer to agent: <name>"`                                   |
| `exit`              | Agent 执行完成           | `"Agent execution completed"`                                   |
| `approval_required` | 需要审批（记录在历史中） | 审批描述文本                                                    |
| `approval_decision` | 审批决定（记录在历史中） | `"已拒绝"`/`"已允许（一次）"`/`"已允许（该项）"`/`"已全部允许"` |

> `interrupted` 类型在 WebSocket 模式下被过滤不发送（由 `approval_required` 消息替代），仅记录在历史中。

**tool_calls 结构**：

```json
{
  "name": "function_name",
  "arguments": "{\"key\": \"value\"}"
}
```

**dispatch_progress 的 detail 结构**：

子任务并行分发中间件通过 `dispatch_progress` 事件实时推送各子任务的执行进度。`detail` 字段为 JSON 字符串：

```json
{
  "task_index": 0,
  "description": "子任务描述",
  "event_type": "start|op|content|done|error|timeout",
  "event_detail": "具体信息"
}
```

| event_type | 说明           | event_detail |
| ---------- | -------------- | ------------ |
| `start`    | 子任务开始执行 | 空           |
| `op`       | 工具操作       | 操作描述     |
| `content`  | 子任务输出内容 | 文本内容     |
| `done`     | 子任务完成     | 空           |
| `error`    | 子任务出错     | 错误信息     |
| `timeout`  | 子任务超时     | 空           |
