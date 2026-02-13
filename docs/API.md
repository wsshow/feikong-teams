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

| message                      | 说明           |
| ---------------------------- | -------------- |
| FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置 |
| 无效的路径                   | 路径包含 `..`  |
| 目录不存在或无法访问         | 路径不可达     |
| 路径不是目录                 | 目标不是目录   |
| 读取目录失败                 | IO 错误        |

---

### GET /api/fkteams/history/files

列出所有聊天历史文件。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "files": [
      {
        "filename": "fkteams_chat_history_20250101_120000",
        "display_name": "2025-01-01 12:00:00",
        "session_id": "20250101_120000",
        "size": 2048,
        "mod_time": "2025-01-01T12:00:00Z"
      }
    ]
  }
}
```

| 字段           | 说明                |
| -------------- | ------------------- |
| `filename`     | 原始文件名          |
| `display_name` | 格式化的显示名称    |
| `session_id`   | 提取的会话 ID       |
| `size`         | 文件大小（字节）    |
| `mod_time`     | 修改时间（RFC3339） |

---

### GET /api/fkteams/history/files/:filename

加载指定的历史文件内容。

**路径参数**：

| 参数       | 类型   | 说明       |
| ---------- | ------ | ---------- |
| `filename` | string | 历史文件名 |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "filename": "fkteams_chat_history_20250101_120000",
    "session_id": "20250101_120000",
    "messages": []
  }
}
```

**失败响应**：

| 状态码 | message                   | 说明                 |
| ------ | ------------------------- | -------------------- |
| 400    | invalid filename          | 文件名含 `..` 或 `/` |
| 404    | file not found            | 文件不存在           |
| 500    | failed to read/parse file | 读取或解析失败       |

---

### DELETE /api/fkteams/history/files/:filename

删除指定的历史文件。

**路径参数**：

| 参数       | 类型   | 说明       |
| ---------- | ------ | ---------- |
| `filename` | string | 历史文件名 |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "file deleted"
  }
}
```

**失败响应**：

| 状态码 | message               |
| ------ | --------------------- |
| 400    | invalid filename      |
| 404    | file not found        |
| 500    | failed to delete file |

---

### POST /api/fkteams/history/files/rename

重命名历史文件。

**请求 Body**：

```json
{
  "old_filename": "string",
  "new_filename": "string"
}
```

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "file renamed",
    "new_filename": "new_name"
  }
}
```

**失败响应**：

| 状态码 | message                                 |
| ------ | --------------------------------------- |
| 400    | invalid request body / invalid filename |
| 404    | source file not found                   |
| 409    | target filename already exists          |
| 500    | failed to rename file                   |

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
  "file_paths": ["string"]
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

**特殊值**：`session_id = "__memory_only__"` — 仅清空所有会话的内存历史（不删除文件），用于新建会话。

**服务器响应**：`{"type": "history_cleared"}` 或 `{"type": "error"}`

#### load_history — 加载历史文件

| 字段      | 说明                           |
| --------- | ------------------------------ |
| `message` | 要加载的历史文件名（不含路径） |

**服务器响应**：

```json
{
  "type": "history_loaded",
  "message": "历史记录已加载",
  "filename": "fkteams_chat_history_xxx",
  "session_id": "xxx",
  "messages": []
}
```

#### ping — 心跳检测

**服务器响应**：`{"type": "pong"}`

---

### 服务器 → 客户端

#### 连接与控制消息

| type               | 说明       | 附加字段                                        |
| ------------------ | ---------- | ----------------------------------------------- |
| `connected`        | 连接建立   | `message`                                       |
| `error`            | 错误通知   | `error`                                         |
| `pong`             | 心跳响应   | —                                               |
| `cancelled`        | 任务已取消 | `message`                                       |
| `history_cleared`  | 历史已清除 | `message`                                       |
| `history_loaded`   | 历史已加载 | `message`、`filename`、`session_id`、`messages` |
| `processing_start` | 开始处理   | `message`                                       |
| `processing_end`   | 处理完成   | `message`                                       |

#### Agent 事件消息

所有事件的基础结构：

```json
{
  "type": "<事件类型>",
  "agent_name": "string",
  "run_path": "string",
  "content": "string",
  "tool_calls": [],
  "action_type": "string",
  "error": "string"
}
```

| type                   | 触发场景           | 关键字段                                      |
| ---------------------- | ------------------ | --------------------------------------------- |
| `message`              | Agent 输出完整消息 | `content`，可能含 `tool_calls`                |
| `tool_result`          | Tool 返回完整结果  | `content`                                     |
| `stream_chunk`         | 流式输出文本块     | `content`                                     |
| `tool_result_chunk`    | Tool 流式输出块    | `content`                                     |
| `tool_calls_preparing` | 识别到工具调用开始 | `tool_calls[].name`                           |
| `tool_calls`           | 完整的工具调用信息 | `tool_calls[].name`、`tool_calls[].arguments` |
| `action`               | Agent 执行动作     | `action_type`、`content`                      |
| `error`                | Agent 执行错误     | `error`                                       |

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
