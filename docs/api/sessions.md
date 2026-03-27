# 会话管理

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
        "size": 2048,
        "mod_time": "2025-01-01T12:00:00Z"
      }
    ]
  }
}
```

| 字段         | 说明                                                         |
| ------------ | ------------------------------------------------------------ |
| `session_id` | 会话 ID（UUID）                                              |
| `title`      | 会话标题（首次提交时从用户输入截取，未提交时为"未命名会话"） |
| `status`     | 会话状态：`idle`、`processing`、`completed`                  |
| `size`       | 历史文件大小（字节，无历史文件时为 0）                       |
| `mod_time`   | 修改时间（RFC3339，无历史文件时取 metadata 更新时间）        |

**失败响应**：

| 状态码 | message                  | 说明         |
| ------ | ------------------------ | ------------ |
| 500    | failed to read directory | 读取目录失败 |

> 会话目录不存在时返回空数组，不报错。

---

## POST /api/fkteams/sessions

创建新的会话（生成 metadata 目录）。

**请求 Body**：

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

| 字段         | 类型   | 必填 | 说明            |
| ------------ | ------ | ---- | --------------- |
| `session_id` | string | 是   | 会话 ID（UUID） |

**成功响应** (200)：

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
    "message": "session already exists"
  }
}
```

> 新建会话的初始 title 为 `"未命名会话"`，status 为 `"idle"`。

**失败响应**：

| 状态码 | message                  | 说明                 |
| ------ | ------------------------ | -------------------- |
| 400    | invalid request body     | 请求体解析失败       |
| 400    | invalid session ID       | ID 含 `..`、`/`、`\` |
| 500    | failed to create session | 创建目录或写文件失败 |

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
    "messages": []
  }
}
```

**失败响应**：

| 状态码 | message                 | 说明                   |
| ------ | ----------------------- | ---------------------- |
| 400    | invalid session ID      | 会话 ID 含 `..` 或 `/` |
| 404    | session not found       | 历史文件不存在         |
| 500    | failed to read history  | 读取文件失败           |
| 500    | failed to parse history | JSON 解析失败          |

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
