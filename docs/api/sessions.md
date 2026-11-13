# 会话管理 API

认证与响应包装见 [API 总览](README.md)。普通会话 ID 默认生成 UUID，也接受不含路径分隔符、控制字符及 `.` / `..` 的有效资源标识。

## GET /api/fkteams/sessions

返回 `{sessions: [...]}`，空列表为 `[]`。每项包括：

| 字段 | 说明 |
| --- | --- |
| `session_id` / `title` | 会话标识与标题 |
| `status` | idle、active、processing、completed、cancelled 或 error |
| `mode` / `current_agent` | 工作模式与固定智能体，可省略 |
| `favorite` | 是否收藏，可省略 |
| `active_task` | 是否有运行中的内存任务 |
| `size` / `mod_time` | 历史大小（字节）与 RFC3339 修改时间 |

## POST /api/fkteams/sessions

```json
{"title":"新的会话"}
```

`title` 可选，默认为“未命名会话”；`session_id` 可选，省略时生成 UUID。新建返回 201，已有同 ID 返回 200，响应包含 `session_id` 和说明。普通标题超长时保留前 50 个字符并追加省略号。

## GET /api/fkteams/sessions/:sessionID

返回会话展示、队列与执行状态：

```json
{
  "session_id": "example-session",
  "mode": "team",
  "current_agent": "coder",
  "favorite": false,
  "view": {"items": []},
  "next_offset": 42,
  "status": "processing",
  "queue": [],
  "active_task": true,
  "has_task": true
}
```

有内存流时，`view` 和 `next_offset` 来自同一快照；否则从已提交 transcript 恢复，`has_task=false`。`active_task` 只表示正在执行，`has_task` 还包括暂时保留的已结束任务。展示结构见[事件协议](../events.md#web-展示契约)，订阅恢复见[流式任务](stream.md)。

会话不存在返回 404；旧历史缺少规范事件信封时返回 409，保留原数据并提示新建会话；读取失败返回 500。

## PATCH /api/fkteams/sessions/:sessionID

提供以下字段中的至少一个，成功返回完整 metadata：

```json
{"title":"新的标题","favorite":true,"mode":"team","current_agent":"coder"}
```

`current_agent: ""` 清除固定角色。无效参数返回 400，不存在返回 404。

兼容接口仍可使用，新客户端优先使用 PATCH：

| 接口 | 请求体 |
| --- | --- |
| `POST /api/fkteams/sessions/rename` | `{session_id, title}` |
| `POST /api/fkteams/sessions/favorite` | `{session_id, favorite}` |
| `POST /api/fkteams/sessions/agent` | `{session_id, current_agent}` |

## DELETE /api/fkteams/sessions/:sessionID

删除会话历史与 metadata，成功返回 `{message: "session deleted"}`；不存在返回 404。运行状态与保留会话的限制由会话用例校验，冲突返回 409。

## 常驻助手

| 接口 | 行为 |
| --- | --- |
| `POST /api/fkteams/assistant` | 请求体可为 `{}`，幂等打开或创建助手 |
| `GET /api/fkteams/assistant` | 只读身份；尚未创建返回 404 |
| `POST /api/fkteams/assistant/clear` | 清空历史、上下文与队列，保留名称和长期记忆；运行中返回 409 |

打开和读取身份返回：

```json
{"session_id":"workspace-assistant","title":"助手","current_agent":"workspace_assistant","mode":"team"}
```

助手消息使用普通聊天/流式接口，传入该 `session_id` 和 `agent_name: "workspace_assistant"`。清空成功返回 `{session_id: "workspace-assistant"}`。

改名复用会话修改接口，名称为 1–40 个字符且不含控制字符。助手也可在自身会话内调用 `assistant_rename`，名称不会被首句自动标题覆盖。应用实例共用工作区和会话库，不提供多用户租户隔离。
