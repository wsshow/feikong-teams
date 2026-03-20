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

### 中间件

#### CORS 跨域

所有请求自动处理跨域：

- `Access-Control-Allow-Origin`：动态镜像请求 `Origin`
- `Access-Control-Allow-Headers: *`
- `Access-Control-Allow-Methods: POST, GET, OPTIONS, DELETE, PUT`
- `Access-Control-Allow-Credentials: true`
- OPTIONS 预检请求直接返回 204

#### 请求体大小限制

请求体上限 **100MB**。超出时返回：

| 状态码 | message                | 说明       |
| ------ | ---------------------- | ---------- |
| 413    | request body too large | 请求体过大 |

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

- API 请求（`/api/*`、`/ws`）→ HTTP 401，`{"code": 1, "message": "未登录或登录已过期"}`
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

> 仅在认证启用时可用（`FEIKONG_LOGIN_ENABLED=true` 时注册该路由）

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

Token 格式为 `hex(payload).hex(hmac-sha256)`，payload 为 `username|expiry(RFC3339)`，有效期 7 天。Token 由客户端自行管理，服务端不设置 Cookie。

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

> SSE 和同步模式均使用 `AutoRejectHandler` 自动拒绝危险命令（无人工审批流程）。

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

排序规则：文件夹在前 → 同类型按修改时间倒序 → 时间相同按名称升序。隐藏文件（`.` 开头）被过滤。

**失败响应**：

| 状态码 | message                      | 说明           |
| ------ | ---------------------------- | -------------- |
| 400    | 无效的路径                   | 路径包含 `..`  |
| 404    | 目录不存在或无法访问         | 路径不可达     |
| 400    | 路径不是目录                 | 目标不是目录   |
| 500    | FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置 |
| 500    | 读取目录失败                 | IO 错误        |

---

### GET /api/fkteams/files/download

下载工作区中的指定文件。

**请求参数** (Query)：

| 参数   | 类型   | 必填 | 说明                   |
| ------ | ------ | ---- | ---------------------- |
| `path` | string | 是   | 文件相对于工作区的路径 |

**成功响应** (200)：

文件内容以 `Content-Disposition: attachment` 方式返回。

**失败响应**：

| 状态码 | message                      | 说明           |
| ------ | ---------------------------- | -------------- |
| 400    | 缺少 path 参数               | 未提供路径     |
| 400    | 无效的路径                   | 路径遍历       |
| 400    | 不支持下载目录               | 目标是目录     |
| 404    | 文件不存在                   | 路径不可达     |
| 500    | FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置 |

---

### POST /api/fkteams/files/upload

上传文件到工作区（支持多文件）。

**请求格式**：`multipart/form-data`

| 字段   | 类型   | 必填 | 说明                                   |
| ------ | ------ | ---- | -------------------------------------- |
| `file` | file   | 是   | 上传的文件（可多个同名字段实现多文件） |
| `path` | string | 否   | 目标子目录路径，空则为根目录           |

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

返回所有成功上传文件的 `FileInfo` 数组。

**失败响应**：

| 状态码 | message                      | 说明           |
| ------ | ---------------------------- | -------------- |
| 400    | 无效的路径                   | 路径遍历       |
| 400    | 解析表单失败                 | 表单格式错误   |
| 400    | 未找到上传文件               | 无文件字段     |
| 400    | 没有文件上传成功             | 全部文件失败   |
| 500    | 创建目录失败                 | IO 错误        |
| 500    | FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置 |

---

### POST /api/fkteams/files/upload/chunk

大文件分片上传。每个分片单独请求，所有分片到齐后自动合并。

**请求格式**：`multipart/form-data`

| 字段          | 类型   | 必填 | 说明                   |
| ------------- | ------ | ---- | ---------------------- |
| `file`        | file   | 是   | 分片内容               |
| `uploadId`    | string | 是   | 上传标识（客户端生成） |
| `chunkIndex`  | int    | 是   | 分片序号（0-based）    |
| `totalChunks` | int    | 是   | 总分片数               |
| `fileName`    | string | 是   | 最终文件名             |
| `path`        | string | 否   | 目标子目录路径         |

**进行中响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "uploadId": "abc123",
    "chunkIndex": 0,
    "received": 1,
    "total": 3,
    "completed": false
  }
}
```

**完成响应** (200)（最后一个分片到达时）：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "uploadId": "abc123",
    "completed": true,
    "file": {
      "name": "large.zip",
      "path": "subdir/large.zip",
      "is_dir": false,
      "size": 104857600,
      "mod_time": 1700000000
    }
  }
}
```

**失败响应**：

| 状态码 | message                      | 说明             |
| ------ | ---------------------------- | ---------------- |
| 400    | 缺少必要参数                 | 参数不完整       |
| 400    | 无效的分片序号               | chunkIndex 非法  |
| 400    | 无效的总分片数               | totalChunks 非法 |
| 400    | 分片序号超出范围             | 超出总片数       |
| 400    | 无效的文件名                 | 文件名不安全     |
| 400    | 无效的路径                   | 路径遍历         |
| 500    | FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置   |

---

### DELETE /api/fkteams/files

删除工作区中的文件或目录。

**请求 Body**：

```json
{
  "path": "subdir/file.txt",
  "force": false
}
```

| 字段    | 类型   | 必填 | 说明                                      |
| ------- | ------ | ---- | ----------------------------------------- |
| `path`  | string | 是   | 文件或目录的相对路径                      |
| `force` | bool   | 否   | 删除非空目录时须设为 `true`，默认 `false` |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

**失败响应**：

| 状态码 | message                              | 说明           |
| ------ | ------------------------------------ | -------------- |
| 400    | 缺少 path 参数                       | 未提供路径     |
| 400    | 无效的路径                           | 路径遍历       |
| 400    | 无效的文件路径                       | 尝试删除根目录 |
| 400    | 目录非空，请设置 force:true 确认删除 | 非空目录未确认 |
| 404    | 文件或目录不存在                     | 路径不可达     |
| 500    | FEIKONG_WORKSPACE_DIR 未配置         | 环境变量未设置 |

---

### POST /api/fkteams/preview

创建文件预览链接（支持设置过期时间和访问密码）。

**请求 Body**：

```json
{
  "file_path": "docs/report.pdf",
  "password": "abc123",
  "expires_in": 7200
}
```

| 字段         | 类型   | 必填 | 说明                                          |
| ------------ | ------ | ---- | --------------------------------------------- |
| `file_path`  | string | 是   | 文件相对路径                                  |
| `password`   | string | 否   | 访问密码，不设则无需密码                      |
| `expires_in` | int    | 否   | 过期时间（秒），默认 86400（1天），最长 30 天 |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "a1b2c3d4e5f6...",
    "file_path": "docs/report.pdf",
    "expires_at": 1700007200,
    "created_at": 1700000000
  }
}
```

**失败响应**：

| 状态码 | message                      | 说明           |
| ------ | ---------------------------- | -------------- |
| 400    | 参数错误                     | JSON 解析失败  |
| 400    | 无效的文件路径               | 路径遍历       |
| 404    | 文件不存在                   | 路径不可达     |
| 500    | 生成链接失败                 | 随机数生成错误 |
| 500    | FEIKONG_WORKSPACE_DIR 未配置 | 环境变量未设置 |

---

### GET /api/fkteams/preview/:linkId

通过预览链接访问/下载文件。

**路径参数**：

| 参数     | 类型   | 说明    |
| -------- | ------ | ------- |
| `linkId` | string | 链接 ID |

**请求参数** (Query/Header)：

| 参数                 | 位置   | 说明                       |
| -------------------- | ------ | -------------------------- |
| `password`           | Query  | 访问密码（设置密码时必填） |
| `X-Preview-Password` | Header | 访问密码（备选方式）       |

**成功响应** (200)：

文件内容直接返回。可浏览器预览的类型（图片/PDF/文本/音视频等）以 `Content-Disposition: inline` 返回，其他类型以 `attachment` 方式下载。

**失败响应**：

| 状态码 | message              | 说明           |
| ------ | -------------------- | -------------- |
| 400    | 缺少链接 ID          | 路径参数为空   |
| 401    | 需要输入访问密码     | 未提供密码     |
| 403    | 密码错误             | 密码不匹配     |
| 404    | 链接不存在或已失效   | 链接未找到     |
| 404    | 文件不存在或已被删除 | 原文件已被移除 |
| 410    | 链接已过期           | 超过过期时间   |

---

### GET /api/fkteams/preview

列出所有有效的预览链接。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": "a1b2c3d4e5f6...",
      "file_path": "docs/report.pdf",
      "expires_at": 1700007200,
      "created_at": 1700000000
    }
  ]
}
```

已过期的链接会自动清理，不在列表中返回。

---

### DELETE /api/fkteams/preview/:linkId

手动删除（撤销）预览链接。

**路径参数**：

| 参数     | 类型   | 说明    |
| -------- | ------ | ------- |
| `linkId` | string | 链接 ID |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

**失败响应**：

| 状态码 | message     | 说明       |
| ------ | ----------- | ---------- |
| 400    | 缺少链接 ID | 参数为空   |
| 404    | 链接不存在  | 未找到链接 |

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

### POST /api/fkteams/sessions

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

| 状态码 | message                 | 说明                   |
| ------ | ----------------------- | ---------------------- |
| 400    | invalid session ID      | 会话 ID 含 `..` 或 `/` |
| 404    | session not found       | 历史文件不存在         |
| 500    | failed to read history  | 读取文件失败           |
| 500    | failed to parse history | JSON 解析失败          |

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

| 状态码 | message                  | 说明           |
| ------ | ------------------------ | -------------- |
| 400    | invalid session ID       | ID 不合法      |
| 404    | session not found        | 会话目录不存在 |
| 500    | failed to delete session | 删除操作失败   |

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

### GET /api/fkteams/schedules

获取定时任务列表。

**请求参数** (Query)：

| 参数     | 类型   | 必填 | 说明                                                                 |
| -------- | ------ | ---- | -------------------------------------------------------------------- |
| `status` | string | 否   | 按状态过滤：`pending`、`running`、`completed`、`failed`、`cancelled` |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "tasks": [
      {
        "id": "task_001",
        "task": "每天早上8点发送天气报告",
        "cron_expr": "0 8 * * *",
        "one_time": false,
        "next_run_at": "2025-01-02T08:00:00Z",
        "status": "pending",
        "created_at": "2025-01-01T12:00:00Z",
        "last_run_at": null,
        "result": ""
      }
    ],
    "total": 1
  }
}
```

| 字段          | 类型    | 说明                                                           |
| ------------- | ------- | -------------------------------------------------------------- |
| `id`          | string  | 任务 ID                                                        |
| `task`        | string  | 任务描述（发送给团队执行的查询）                               |
| `cron_expr`   | string  | cron 表达式（重复任务）                                        |
| `one_time`    | bool    | 是否一次性任务                                                 |
| `next_run_at` | string  | 下次执行时间（RFC3339）                                        |
| `status`      | string  | 任务状态：`pending`/`running`/`completed`/`failed`/`cancelled` |
| `created_at`  | string  | 创建时间（RFC3339）                                            |
| `last_run_at` | string? | 上次执行时间（可为 null）                                      |
| `result`      | string  | 执行结果（可为空）                                             |

**失败响应**：

| 状态码 | message        | 说明             |
| ------ | -------------- | ---------------- |
| 503    | 调度器未初始化 | 调度功能未启用   |
| 500    | (错误详情)     | 获取任务列表失败 |

---

### POST /api/fkteams/schedules/:id/cancel

取消指定的定时任务（仅 `pending` 状态可取消）。

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
    "message": "任务 task_001 已取消"
  }
}
```

**失败响应**：

| 状态码 | message          | 说明                   |
| ------ | ---------------- | ---------------------- |
| 400    | 任务 ID 不能为空 | 缺少路径参数           |
| 400    | (错误详情)       | 任务不存在/状态不允许  |
| 503    | 调度器未初始化   | 调度功能未启用         |
| 500    | (错误详情)       | 加载或保存任务列表失败 |

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

| 状态码 | message                    | 说明         |
| ------ | -------------------------- | ------------ |
| 400    | 长期记忆未启用             | 功能未启用   |
| 400    | 参数错误: summary 不能为空 | 缺少 summary |
| 404    | 未找到匹配的记忆条目       | 无匹配条目   |

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
