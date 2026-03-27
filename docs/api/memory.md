# 长期记忆

## GET /api/fkteams/memory

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

## DELETE /api/fkteams/memory

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

## POST /api/fkteams/memory/clear

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
