# 会话分享 API

把已保存的对话发布为 `/s/:shareID` 链接。管理操作需要普通登录认证，公开读取使用分享自身的密码和期限。响应包装见 [API 总览](README.md)。

## POST /api/fkteams/session-shares

```json
{"session_id":"example-session","password":"example-password","expires_in":604800,"allow_tool_details":false}
```

| 字段 | 说明 |
| --- | --- |
| `session_id` | 必填，会话标识 |
| `password` | 可选访问密码 |
| `expires_in` | 秒，默认 7 天、最长 90 天，负数表示永不过期 |
| `allow_tool_details` | 是否公开工具参数和结果，默认 false |

成功返回：

```json
{"id":"share-1","session_id":"example-session","title":"会话标题","has_password":true,"allow_tool_details":false,"message_count":8,"expires_at":1760007200,"created_at":1760000000}
```

没有可分享消息返回 400，历史不存在返回 404，读取或创建失败返回 500。关闭工具详情会清除工具载荷及外置结果引用，但不会自动清理对话正文中的敏感信息。

## 链接管理

| 接口 | 行为 |
| --- | --- |
| `GET /api/fkteams/session-shares` | 返回有效分享数组，按创建时间倒序；字段同创建响应，可含 last_accessed_at |
| `DELETE /api/fkteams/session-shares/:shareID` | 撤销，返回 `{message: "share deleted"}`；不存在返回 404 |

## 公开读取

`GET /api/fkteams/public/session-shares/:shareID/info` 返回 id、title、has_password、message_count、expires_at、created_at、allow_tool_details，不包含正文。

`POST /api/fkteams/public/session-shares/:shareID/access`：

```json
{"password":"example-password"}
```

无密码分享可提交 `{}`。成功返回：

```json
{
  "id": "share-1",
  "title": "会话标题",
  "view": {"items": []},
  "message_count": 8,
  "expires_at": 1760007200,
  "created_at": 1760000000,
  "allow_tool_details": false
}
```

`view` 使用与会话详情相同的[展示契约](../events.md#web-展示契约)。密码只通过请求体提交。

| 状态码 | 含义 |
| --- | --- |
| 401 | 缺少或错误密码；缺少时 data 包含 require_password: true |
| 404 | 分享不存在 |
| 410 | 分享过期或原会话不可用 |
| 429 | 密码尝试过多，按 Retry-After 等待 |
