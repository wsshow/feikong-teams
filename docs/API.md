# 非空小队 API 文档

## 通用约定

### 统一响应格式

所有 HTTP API 使用统一的 JSON 响应结构：

```json
{
  "code": 0,
  "message": "success",
  "data": <any>
}
```

| 字段      | 说明                       |
| --------- | -------------------------- |
| `code`    | `0` 成功，`1` 失败         |
| `message` | 描述信息                   |
| `data`    | 业务数据（失败时无此字段） |

### 认证机制

通过环境变量控制是否启用：

| 环境变量                 | 说明                         |
| ------------------------ | ---------------------------- |
| `FEIKONG_LOGIN_ENABLED`  | `true` 启用认证              |
| `FEIKONG_LOGIN_SECRET`   | Token 签名密钥（启用时必填） |
| `FEIKONG_LOGIN_USERNAME` | 登录用户名                   |
| `FEIKONG_LOGIN_PASSWORD` | 登录密码                     |

**Token 传递方式**（优先级从高到低）：

1. `Authorization: Bearer <token>` 请求头
2. `?token=<token>` URL 参数
3. `fk_token` Cookie

**免认证路径**：`/login`、`/api/fkteams/login`、`/static/*`

**认证失败行为**：

- API 请求（`/api/*`、`/ws`）→ HTTP 401
- 页面请求 → 返回登录页面

---

## HTTP API

所有接口挂载在 `/api/fkteams` 路径下。

### GET /health

健康检查接口，用于容器编排和负载均衡的健康探测。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok"
  }
}
```

---

### POST /api/fkteams/login

> 仅在认证启用时可用

用户登录获取 Token。

**请求 Body**：

```json
{
  "username": "string",
  "password": "string"
}
```

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "hex_payload.hex_signature"
  }
}
```

**失败响应**：

| 状态码 | message          | 说明           |
| ------ | ---------------- | -------------- |
| 400    | 请求格式错误     | 请求体解析失败 |
| 401    | 用户名或密码错误 | 凭证不匹配     |

Token 格式为 `hex(payload).hex(hmac-sha256)`，payload 为 `username|expiry(RFC3339)`，有效期 7 天。

---

### GET /api/fkteams/version

获取服务版本信息。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "version": "0.0.1",
    "buildDate": "2025-01-01 00:00:00"
  }
}
```

---

### GET /api/fkteams/agents

获取所有可用智能体列表。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "name": "string",
      "description": "string"
    }
  ]
}
```

---

### POST /api/fkteams/chat

通过 HTTP 发送聊天消息，支持同步和 SSE 流式两种响应模式。

**请求 Body**：

```json
{
  "session_id": "string",
  "message": "string",
  "mode": "string",
  "agent_name": "string",
  "file_paths": ["string"],
  "stream": false,
  "contents": []
}
```

| 字段         | 类型     | 必填 | 说明                                                                                                                |
| ------------ | -------- | ---- | ------------------------------------------------------------------------------------------------------------------- |
| `message`    | string   | 条件 | 用户输入的文本（`message` 和 `contents` 至少提供一个）                                                              |
| `session_id` | string   | 否   | 会话标识，默认 `"default"`                                                                                          |
| `mode`       | string   | 否   | 运行模式：`supervisor`（默认）、`roundtable`、`custom`、`deep`                                                      |
| `agent_name` | string   | 否   | 指定单个智能体直接对话（优先级高于 mode）                                                                           |
| `file_paths` | string[] | 否   | 引用的文件路径列表                                                                                                  |
| `stream`     | bool     | 否   | 是否使用 SSE 流式响应，默认 `false`                                                                                 |
| `contents`   | array    | 否   | 多模态内容部分（存在时优先于 `message`），每项包含 `type`、`text`、`url`、`base64_data`、`mime_type`、`detail` 字段 |

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

**SSE 流式响应** (`stream: true`)：

返回 `text/event-stream`，每个事件的格式为：

```
data: {"type":"stream_chunk","agent_name":"小码","content":"..."}

data: {"type":"processing_end","message":"处理完成"}

```

事件结构与 WebSocket 的 Agent 事件消息相同。

**失败响应**：

| 状态码 | message         | 说明               |
| ------ | --------------- | ------------------ |
| 400    | invalid request | 请求体解析失败     |
| 400    | agent not found | 指定的智能体不存在 |

---

### GET /api/fkteams/files

获取工作区目录下的文件和文件夹列表。

**请求参数** (Query)：

| 参数   | 类型   | 必填 | 说明                                     |
| ------ | ------ | ---- | ---------------------------------------- |
| `path` | string | 否   | 相对于工作区根目录的子路径，空则为根目录 |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "name": "file.txt",
      "path": "subdir/file.txt",
      "is_dir": false,
      "size": 1234,
      "mod_time": 1700000000
    }
  ]
}
```

| 字段       | 说明                    |
| ---------- | ----------------------- |
| `name`     | 文件/目录名             |
| `path`     | 相对路径                |
| `is_dir`   | 是否为目录              |
| `size`     | 文件大小（字节）        |
| `mod_time` | 修改时间（Unix 时间戳） |

排序规则：文件夹在前，同类型按修改时间倒序。隐藏文件（`.` 开头）被过滤。

**失败响应**：

| 状态码 | message                      | 说明           |
| ------ | ---------------------------- | -------------- |
| 400    | 无效的路径                   | 路径包含 `..`  |
| 404    | 目录不存在或无法访问         | 路径不可达     |
| 400    | 路径不是目录                 | 目标不是目录   |
| 500    | FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置 |
| 500    | 读取目录失败                 | IO 错误        |

---

### GET /api/fkteams/sessions

列出所有聊天历史会话。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "sessions": [
      {
        "session_id": "550e8400-e29b-41d4-a716-446655440000",
        "title": "2025-01-01 12:00:00",
        "status": "active",
        "size": 2048,
        "mod_time": "2025-01-01T12:00:00Z"
      }
    ]
  }
}
```

| 字段         | 说明                 |
| ------------ | -------------------- |
| `session_id` | 会话 ID（UUID）      |
| `title`      | 会话标题             |
| `status`     | 会话状态             |
| `size`       | 历史文件大小（字节） |
| `mod_time`   | 修改时间（RFC3339）  |

---

### POST /api/fkteams/sessions

创建新的会话（生成 metadata）。

**请求 Body**：

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "session created",
    "session_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**失败响应**：

| 状态码 | message                  |
| ------ | ------------------------ |
| 400    | invalid request body     |
| 400    | invalid session ID       |
| 500    | failed to create session |

---

### GET /api/fkteams/sessions/:sessionID

加载指定会话的历史记录。

**路径参数**：

| 参数        | 类型   | 说明            |
| ----------- | ------ | --------------- |
| `sessionID` | string | 会话 ID（UUID） |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "messages": []
  }
}
```

**失败响应**：

| 状态码 | message                      | 说明                   |
| ------ | ---------------------------- | ---------------------- |
| 400    | invalid session ID           | 会话 ID 含 `..` 或 `/` |
| 404    | session not found            | 会话不存在             |
| 500    | failed to read/parse history | 读取或解析失败         |

---

### DELETE /api/fkteams/sessions/:sessionID

删除指定的会话目录（包括历史记录和元数据）。

**路径参数**：

| 参数        | 类型   | 说明            |
| ----------- | ------ | --------------- |
| `sessionID` | string | 会话 ID（UUID） |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "session deleted"
  }
}
```

**失败响应**：

| 状态码 | message                  |
| ------ | ------------------------ |
| 400    | invalid session ID       |
| 404    | session not found        |
| 500    | failed to delete session |

---

### POST /api/fkteams/sessions/rename

更新会话的标题。

**请求 Body**：

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "新的会话标题"
}
```

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "session renamed",
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "title": "新的会话标题"
  }
}
```

**失败响应**：

| 状态码 | message                                   |
| ------ | ----------------------------------------- |
| 400    | invalid request body / invalid session ID |
| 404    | session not found                         |
| 500    | failed to read/save metadata              |

---

### GET /api/fkteams/schedules

获取定时任务列表。

**请求参数** (Query)：

| 参数     | 类型   | 必填 | 说明                                          |
| -------- | ------ | ---- | --------------------------------------------- |
| `status` | string | 否   | 按状态过滤（如 `pending`、`running`、`done`） |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "tasks": [
      {
        "id": "task_001",
        "status": "pending",
        "schedule": "2025-01-01T12:00:00Z",
        "message": "任务描述"
      }
    ],
    "total": 1
  }
}
```

**失败响应**：

| 状态码 | message        | 说明             |
| ------ | -------------- | ---------------- |
| 503    | 调度器未初始化 | 调度功能未启用   |
| 500    | (错误详情)     | 获取任务列表失败 |

---

### POST /api/fkteams/schedules/:id/cancel

取消指定的定时任务。

**路径参数**：

| 参数 | 类型   | 说明    |
| ---- | ------ | ------- |
| `id` | string | 任务 ID |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "任务已取消"
  }
}
```

**失败响应**：

| 状态码 | message          | 说明           |
| ------ | ---------------- | -------------- |
| 400    | 任务 ID 不能为空 | 缺少任务 ID    |
| 400    | (错误详情)       | 取消失败       |
| 503    | 调度器未初始化   | 调度功能未启用 |
| 500    | (错误详情)       | 内部错误       |

---

### GET /api/fkteams/memory

获取所有长期记忆条目列表。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "summary": "记忆摘要",
      "content": "记忆内容",
      "created_at": "2025-01-01T12:00:00Z"
    }
  ]
}
```

> 长期记忆未启用时返回空数组 `[]`。

---

### DELETE /api/fkteams/memory

删除匹配指定摘要的记忆条目。

**请求 Body**：

```json
{
  "summary": "string"
}
```

| 字段      | 类型   | 必填 | 说明             |
| --------- | ------ | ---- | ---------------- |
| `summary` | string | 是   | 要删除的记忆摘要 |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "deleted": 1
  }
}
```

**失败响应**：

| 状态码 | message              | 说明         |
| ------ | -------------------- | ------------ |
| 400    | 长期记忆未启用       | 功能未启用   |
| 400    | 参数错误             | 缺少 summary |
| 404    | 未找到匹配的记忆条目 | 无匹配条目   |

---

### POST /api/fkteams/memory/clear

清空所有长期记忆。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "cleared": 10
  }
}
```

**失败响应**：

| 状态码 | message        | 说明       |
| ------ | -------------- | ---------- |
| 400    | 长期记忆未启用 | 功能未启用 |

---

## 页面路由

| 方法 | 路径        | 说明                     |
| ---- | ----------- | ------------------------ |
| GET  | `/`         | 首页                     |
| GET  | `/chat`     | 聊天页面（同首页）       |
| GET  | `/login`    | 登录页面（仅认证启用时） |
| GET  | `/static/*` | 静态资源                 |

---

## WebSocket

### 连接

- **URL**：`ws://<host>/ws` 或 `wss://<host>/ws`
- **认证**：启用认证时通过 `?token=<token>` 参数传递
- **连接建立后**：服务器自动发送 `connected` 消息

### 客户端 → 服务器

所有消息使用统一结构：

```json
{
  "type": "string",
  "session_id": "string",
  "message": "string",
  "mode": "string",
  "agent_name": "string",
  "file_paths": ["string"],
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

#### chat — 发送聊天消息

| 字段         | 说明                                                           |
| ------------ | -------------------------------------------------------------- |
| `session_id` | 会话标识，默认 `"default"`                                     |
| `message`    | 用户输入的文本                                                 |
| `mode`       | 运行模式：`supervisor`（默认）、`roundtable`、`custom`、`deep` |
| `agent_name` | 指定单个智能体直接对话（优先级高于 mode）                      |
| `file_paths` | 引用的文件路径列表                                             |
| `contents`   | 多模态内容部分（可选，存在时优先于 `message` 字段）            |

**处理流程**：

1. 创建独立可取消 context
2. 获取或创建会话的 HistoryRecorder（自动加载文件历史）
3. 构建输入消息（如有历史记录则注入 SystemMessage 作为上下文）
4. 根据 `agent_name` 或 `mode` 获取/创建 Runner（带缓存）
5. 发送 `processing_start` → 执行 Runner → 发送 `processing_end`
6. 保存聊天历史到文件

**Runner 创建规则**：

| 条件              | Runner 类型         |
| ----------------- | ------------------- |
| `agent_name` 非空 | 单智能体 Runner     |
| `mode=roundtable` | 圆桌会议 Runner     |
| `mode=custom`     | 自定义监督者 Runner |
| `mode=deep`       | 深度分析 Runner     |
| 其他              | 默认监督者 Runner   |

#### cancel — 取消当前任务

取消该连接正在执行的任务。

**服务器响应**：`{"type": "cancelled", "message": "任务已取消"}`

#### clear_history — 清除会话历史

| 字段         | 说明                              |
| ------------ | --------------------------------- |
| `session_id` | 要清除的会话 ID，默认 `"default"` |

**服务器响应**：`{"type": "history_cleared"}` 或 `{"type": "error"}`

#### load_history — 加载历史会话

| 字段      | 说明                         |
| --------- | ---------------------------- |
| `message` | 要加载的会话 ID（UUID 格式） |

**服务器响应**：

```json
{
  "type": "history_loaded",
  "message": "历史记录已加载",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "messages": []
}
```

#### ping — 心跳检测

**服务器响应**：`{"type": "pong"}`

---

### 服务器 → 客户端

#### 连接与控制消息

| type               | 说明       | 附加字段                            |
| ------------------ | ---------- | ----------------------------------- |
| `connected`        | 连接建立   | `message`                           |
| `error`            | 错误通知   | `error`                             |
| `pong`             | 心跳响应   | —                                   |
| `cancelled`        | 任务已取消 | `message`                           |
| `history_cleared`  | 历史已清除 | `message`                           |
| `history_loaded`   | 历史已加载 | `message`、`session_id`、`messages` |
| `processing_start` | 开始处理   | `message`                           |
| `processing_end`   | 处理完成   | `message`                           |

#### Agent 事件消息

所有事件的基础结构：

```json
{
  "type": "<事件类型>",
  "agent_name": "string",
  "run_path": "string",
  "content": "string",
  "reasoning_content": "string",
  "tool_calls": [],
  "action_type": "string",
  "error": "string"
}
```

| type                   | 触发场景              | 关键字段                                            |
| ---------------------- | --------------------- | --------------------------------------------------- |
| `message`              | Agent 输出完整消息    | `content`，可能含 `tool_calls`、`reasoning_content` |
| `tool_result`          | Tool 返回完整结果     | `content`                                           |
| `stream_chunk`         | 流式输出文本块        | `content`                                           |
| `reasoning_chunk`      | 推理/思考过程流式增量 | `content`（仅推理模型）                             |
| `tool_result_chunk`    | Tool 流式输出块       | `content`                                           |
| `tool_calls_preparing` | 识别到工具调用开始    | `tool_calls[].name`                                 |
| `tool_calls`           | 完整的工具调用信息    | `tool_calls[].name`、`tool_calls[].arguments`       |
| `action`               | Agent 执行动作        | `action_type`、`content`                            |
| `error`                | Agent 执行错误        | `error`                                             |

**action_type 子类型**：

| action_type   | 说明             | content                       |
| ------------- | ---------------- | ----------------------------- |
| `transfer`    | 转交到其他 Agent | `"Transfer to agent: <name>"` |
| `interrupted` | Agent 被中断     | 中断上下文信息                |
| `exit`        | Agent 执行完成   | `"Agent execution completed"` |

**tool_calls 结构**：

```json
{
  "name": "function_name",
  "arguments": "{\"key\": \"value\"}"
}
```
