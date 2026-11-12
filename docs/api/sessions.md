# 会话管理

## 常驻助手

`POST /api/fkteams/assistant` 幂等打开助手，首次创建固定会话 `workspace-assistant`，默认名称“助手”，智能体为 `workspace_assistant`。请求体可为 `{}`。`GET /api/fkteams/assistant` 只读取当前身份，尚未创建时返回 404。

两个接口均返回：

```json
{"code":0,"message":"success","data":{"session_id":"workspace-assistant","title":"助手","current_agent":"workspace_assistant","mode":"team"}}
```

助手使用现有会话历史与流式任务接口；发送时传入该 `session_id` 和 `agent_name: "workspace_assistant"`。改名复用会话重命名接口，名称限 1～40 个字符且不能包含控制字符。模型也可在助手会话内通过 `assistant_rename` 改名；该工具拒绝其他会话调用。

助手会话包含在会话列表中，前端将其显示为固定入口。它的标题是助手名称，不会被聊天首句自动覆盖。应用目前使用同一实例的工作区和会话库，不提供多用户租户隔离。

## GET /api/fkteams/sessions

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
        "title": "帮我查一下天气",
        "status": "completed",
        "current_agent": "coder",
        "active_task": false,
        "size": 2048,
        "mod_time": "2025-01-01T12:00:00Z"
      }
    ]
  }
}
```

| 字段         | 说明                                                         |
| ------------ | ------------------------------------------------------------ |
| `session_id` | 会话 ID（普通新会话默认为 UUID，常驻助手使用固定标识）       |
| `title`      | 会话标题（首次提交时从用户输入截取，未提交时为"未命名会话"） |
| `status`     | 会话状态：`idle`、`active`、`processing`、`completed`、`cancelled`、`error` |
| `current_agent` | 当前会话固定智能体，可能为空或省略 |
| `active_task` | 内存中是否有可订阅的运行中/刚完成流式任务 |
| `size`       | 历史文件大小（字节，无历史文件时为 0）                       |
| `mod_time`   | 修改时间（RFC3339，无历史文件时取 metadata 更新时间）        |

**失败响应**：

| 状态码 | message                  | 说明         |
| ------ | ------------------------ | ------------ |
| 503    | session storage unavailable | 会话存储不可用 |

> 会话目录不存在时返回空数组，不报错。

---

## POST /api/fkteams/sessions

创建新的会话（生成 metadata 目录）。

**请求 Body**：

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "新的会话"
}
```

| 字段         | 类型   | 必填 | 说明            |
| ------------ | ------ | ---- | --------------- |
| `session_id` | string | 否   | 会话 ID；不提供时后端生成 UUID |
| `title`      | string | 否   | 初始标题，默认 `"未命名会话"`；超长时保留前 50 个 rune 并追加省略号 |

**成功响应**：新建时返回 201；指定 ID 已存在时返回 200。

新建会话：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "message": "session created"
  }
}
```

会话已存在时直接返回成功：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "current_agent": "",
    "message": "session already exists"
  }
}
```

> 新建会话的初始 title 为 `"未命名会话"`，status 为 `"idle"`。

**失败响应**：

| 状态码 | message                  | 说明                 |
| ------ | ------------------------ | -------------------- |
| 400    | invalid request body     | 请求体解析失败       |
| 400    | invalid session ID       | ID 为空、为 `.`/`..`、含路径分隔符或控制字符 |
| 503    | session storage unavailable | 会话存储不可用 |

---

## PATCH /api/fkteams/sessions/:sessionID

按资源路径更新会话元数据。`title`、`favorite`、`current_agent` 至少提供一个；标题会去除首尾空白，超长时保留前 50 个 rune 并追加省略号。

```json
{
  "title": "新的标题",
  "favorite": true,
  "current_agent": "coder"
}
```

成功时返回更新后的完整 metadata。失败状态包括 400（参数无效）、404（会话不存在）和 503（会话存储不可用）。

旧的 `/sessions/rename`、`/sessions/favorite`、`/sessions/agent` 继续保留兼容，但新客户端应优先使用本接口。

---

## GET /api/fkteams/sessions/:sessionID

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
    "current_agent": "coder",
    "active_task": false,
    "messages": []
  }
}
```

**失败响应**：

| 状态码 | message                 | 说明                   |
| ------ | ----------------------- | ---------------------- |
| 400    | invalid session ID      | 会话 ID 不符合资源标识规则 |
| 404    | session not found       | 历史和元数据均不存在，且没有活跃任务 |
| 500    | failed to read history  | 读取文件失败           |

---

## DELETE /api/fkteams/sessions/:sessionID

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

| 状态码 | message                  | 说明           |
| ------ | ------------------------ | -------------- |
| 400    | invalid session ID       | ID 不合法      |
| 404    | session not found        | 会话目录不存在 |
| 500    | failed to delete session | 删除操作失败   |

---

## POST /api/fkteams/sessions/rename

更新会话的标题。

**请求 Body**：

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "新的会话标题"
}
```

| 字段         | 类型   | 必填 | 说明    |
| ------------ | ------ | ---- | ------- |
| `session_id` | string | 是   | 会话 ID |
| `title`      | string | 是   | 新标题  |

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

| 状态码 | message                 | 说明             |
| ------ | ----------------------- | ---------------- |
| 400    | invalid request body    | 请求体解析失败   |
| 400    | invalid session ID      | ID 不合法        |
| 404    | session not found       | 元数据文件不存在 |
| 500    | failed to read metadata | 读取元数据失败   |
| 500    | failed to save metadata | 保存元数据失败   |

---

## POST /api/fkteams/sessions/agent

更新会话当前固定智能体。前端用于记住某个会话当前直接对话的智能体；传空字符串表示回到团队模式。

**请求 Body**：

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "current_agent": "coder"
}
```

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `session_id` | string | 是 | 会话 ID |
| `current_agent` | string | 否 | 智能体名称，空字符串表示不固定 |

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "agent updated",
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "current_agent": "coder"
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid request body` | 请求体解析失败 |
| 400 | `invalid session ID` | ID 不合法 |
| 404 | `session not found` | 元数据不存在 |
| 500 | `failed to read metadata` | 读取元数据失败 |
| 500 | `failed to save metadata` | 保存元数据失败 |
