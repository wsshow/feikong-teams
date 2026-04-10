# 流式任务 API

流式任务 API 将任务执行与前端连接解耦。任务在服务端后台独立运行，所有输出事件缓存在内存中。前端通过 SSE（Server-Sent Events）订阅事件流，断线重连后可从断点继续接收，实现无损续接。

## 核心流程

```
1. POST /stream/start          → 启动后台任务，返回 session_id
2. GET  /stream/subscribe/:id  → SSE 订阅事件流（支持断线重连）
3. POST /stream/stop/:id       → 停止正在运行的任务
4. GET  /stream/status/:id     → 查询任务状态
5. GET  /stream/events/:id     → 一次性拉取已缓冲事件
6. POST /stream/approval       → 提交 HITL 审批决定
7. POST /stream/ask-response   → 提交交互式提问的回答
```

## 接口详情

### 启动任务

```
POST /api/fkteams/stream/start
```

**请求体**：

```json
{
  "session_id": "可选，不提供则自动生成 UUID",
  "message": "用户消息",
  "mode": "supervisor",
  "agent_name": "可选，指定单个智能体",
  "contents": []
}
```

| 字段         | 类型   | 必填 | 说明                                                |
| ------------ | ------ | ---- | --------------------------------------------------- |
| `session_id` | string | 否   | 会话 ID，不提供则自动生成                           |
| `message`    | string | 条件 | 文本消息（与 `contents` 二选一）                    |
| `mode`       | string | 否   | 工作模式：`supervisor`/`roundtable`/`custom`/`deep` |
| `agent_name` | string | 否   | 指定单个智能体名称                                  |
| `contents`   | array  | 条件 | 多模态内容（与 `message` 二选一），格式同聊天 API   |

**成功响应**：

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

**错误码**：

| 状态码 | 说明                   |
| ------ | ---------------------- |
| 400    | 参数错误               |
| 409    | 该会话已有运行中的任务 |
| 500    | Runner 创建失败        |

---

### 订阅事件流

```
GET /api/fkteams/stream/subscribe/:sessionID
```

SSE 长连接，持续推送事件直到任务完成或客户端断开。

**断线重连**：

- **方式一**：浏览器 `EventSource` 自动携带 `Last-Event-ID` 请求头
- **方式二**：手动指定 `?offset=N` query 参数

每个 SSE 事件格式：

```
id: 42
data: {"type":"stream_chunk","agent_name":"小码","content":"...","session_id":"abc-123"}
```

**事件类型**：

| type                | 说明         |
| ------------------- | ------------ |
| `processing_start`  | 任务开始     |
| `stream_chunk`      | 文本片段     |
| `reasoning_chunk`   | 推理内容片段 |
| `tool_calls`        | 工具调用     |
| `tool_result`       | 工具结果     |
| `action`            | 动作事件     |
| `approval_required` | 需要审批     |
| `error`             | 错误         |
| `cancelled`         | 任务已取消   |
| `processing_end`    | 任务完成     |

**前端示例**：

```javascript
const es = new EventSource("/api/fkteams/stream/subscribe/abc-123");

es.onmessage = (e) => {
  const event = JSON.parse(e.data);
  switch (event.type) {
    case "stream_chunk":
      appendText(event.content);
      break;
    case "processing_end":
      es.close();
      break;
  }
};

// 断线后浏览器会自动重连并携带 Last-Event-ID，
// 服务端从断点继续推送，无需额外处理。
```

---

### 停止任务

```
POST /api/fkteams/stream/stop/:sessionID
```

无请求体。

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

| 状态码 | 说明               |
| ------ | ------------------ |
| 404    | 未找到该会话的任务 |
| 409    | 任务未在运行状态   |

---

### 查询任务状态

```
GET /api/fkteams/stream/status/:sessionID
```

**有活跃任务时**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "processing",
    "has_task": true,
    "mode": "supervisor",
    "agent_name": "",
    "event_count": 42,
    "created_at": "2026-04-05T10:00:00Z"
  }
}
```

**无活跃任务但有会话记录时**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "completed",
    "has_task": false,
    "title": "用户问题标题",
    "created_at": "2026-04-05T10:00:00Z",
    "updated_at": "2026-04-05T10:05:00Z"
  }
}
```

`status` 取值：`processing` / `completed` / `error` / `cancelled` / `idle`

---

### 拉取已缓冲事件

```
GET /api/fkteams/stream/events/:sessionID?offset=0
```

一次性返回已缓冲的事件（非 SSE），适用于页面加载时快速获取历史。

| 参数     | 说明               |
| -------- | ------------------ |
| `offset` | 起始位置（默认 0） |

**响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "abc-123",
    "status": "processing",
    "events": [
      { "id": 0, "data": { "type": "processing_start", "...": "..." } },
      { "id": 1, "data": { "type": "stream_chunk", "...": "..." } }
    ],
    "event_count": 42,
    "done": false
  }
}
```

---

### 提交审批

```
POST /api/fkteams/stream/approval
```

**请求体**：

```json
{
  "session_id": "abc-123",
  "decision": 1
}
```

| decision | 含义         |
| -------- | ------------ |
| 0        | 拒绝         |
| 1        | 允许（一次） |
| 2        | 允许（该项） |
| 3        | 全部允许     |

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

| 状态码 | 说明                 |
| ------ | -------------------- |
| 404    | 无运行中的任务       |
| 409    | 当前没有待审批的请求 |

---

### 提交交互式提问回答

```
POST /api/fkteams/stream/ask-response
```

当智能体通过 `ask_questions` 工具向用户提问时，前端通过此接口提交用户的回答。

**请求体**：

```json
{
  "session_id": "abc-123",
  "selected": ["选项1", "选项2"],
  "free_text": "用户的自由输入文本"
}
```

| 字段         | 类型     | 必填 | 说明               |
| ------------ | -------- | ---- | ------------------ |
| `session_id` | string   | ✓    | 会话 ID            |
| `selected`   | string[] | 否   | 用户选择的选项列表 |
| `free_text`  | string   | 否   | 用户的自由文本输入 |

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

| 状态码 | 说明                     |
| ------ | ------------------------ |
| 404    | 无运行中的任务           |
| 409    | 当前没有待回答的提问请求 |

---

## 缓存机制

流式事件缓存**仅服务于运行中及刚完成的任务**：

| 状态                      | 数据来源 | 接口                           |
| ------------------------- | -------- | ------------------------------ |
| 任务运行中                | 内存缓存 | `/stream/subscribe` SSE 实时流 |
| 任务刚完成（5 分钟内）    | 内存缓存 | `/stream/events` 拉取缓冲      |
| 任务已完成（超过 5 分钟） | 历史文件 | `/sessions/:id` 加载历史       |

- 缓存在任务完成 5 分钟后自动释放内存
- 已完成任务的完整数据已通过历史系统持久化，前端应使用会话接口加载
- 同一 session 启动新任务时，旧缓存自动替换

## 典型使用场景

### 页面首次打开（多轮会话场景）

1. 调用 `GET /sessions/:id` 加载完整历史（所有已完成的轮次）并渲染
2. 调用 `GET /stream/status/:id` 检查是否有运行中的任务
3. 如果 `has_task=true && status=processing`：
   - 调用 `GET /stream/events/:id?offset=0` 获取当前轮次的已缓冲事件并渲染
   - 然后用 `GET /stream/subscribe/:id?offset=<last_id+1>` 接入实时流
4. 如果 `has_task=false`，仅展示历史，等待用户输入

### 断线重连

浏览器 `EventSource` 自动处理：断线时浏览器自动重连并发送 `Last-Event-ID`，服务端从下一个事件开始推送。前端无需额外代码。

### 退出页面后重新进入

1. 调用 `GET /sessions/:id` 加载完整会话历史 → 渲染已完成的所有轮次
2. 调用 `GET /stream/status/:id` → 若 `has_task=true, status=processing`：
   - 调用 `GET /stream/events/:id` 拉取当前轮次缓冲事件 → 渲染
   - 调用 `GET /stream/subscribe/:id?offset=<last_event_id+1>` → 续接实时流
3. 若 `has_task=false`，仅展示历史
