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
      "id": "memory-example",
      "type": "fact",
      "summary": "记忆摘要",
      "detail": "记忆内容",
      "tags": [],
      "hit_count": 0,
      "created_at": "2025-01-01T12:00:00Z"
    }
  ]
}
```

> 默认应用即使关闭自动记忆提取，仍提供手动管理。嵌入模式未装配记忆服务时返回空数组 `[]`。

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
| 400    | 长期记忆未启用             | 未装配记忆服务   |
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
| 400    | 长期记忆未启用 | 未装配记忆服务 |

## PUT /api/fkteams/memory

新增或编辑记忆。请求字段为 `id`（新增时省略）、`type`、`summary`、`detail` 和字符串数组 `tags`。类型支持 preference、fact、feedback、lesson、decision、insight、experience。

成功返回完整条目，编辑保留原 ID、创建时间、来源会话和命中统计。无效字段返回 400，不存在的 ID 返回 404，存储失败返回 500，服务不可用返回 503。只有持久化成功后才发布修改。

## DELETE /api/fkteams/memory/:id

按稳定 ID 删除单条记忆。成功返回 `{"deleted":1}`；不存在返回 404，存储失败返回 500，且保留原条目。管理页面使用此接口，避免相同摘要的记忆被一起删除。
