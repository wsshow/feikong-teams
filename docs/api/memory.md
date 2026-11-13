# 长期记忆 API

基础路径 `/api/fkteams/memory`，认证与响应包装见 [API 总览](README.md)。关闭模型记忆读写后仍可手动管理，行为和容量见[长期记忆](../memory.md)。

## GET /api/fkteams/memory

返回条目数组：

```json
[
  {
    "id": "memory-example",
    "version": 1,
    "type": "preference",
    "scope": "",
    "summary": "回答风格",
    "detail": "优先使用简洁的中文说明",
    "tags": ["沟通"]
  }
]
```

条目还包含创建、更新时间和来源等信息。未装配服务时返回空数组。

## PUT /api/fkteams/memory

新增或更新完整条目：

```json
{"type":"preference","scope":"","summary":"回答风格","detail":"优先使用简洁的中文说明","tags":["沟通"]}
```

新增省略 `id`；更新必须带读取到的 `id` 与 `version`。`type` 支持 preference、fact、feedback、lesson、decision、insight、experience。`scope` 为空表示全局。

成功返回完整条目。字段无效或容量已满返回 400，不存在返回 404，版本冲突返回 409，存储失败返回 500，服务不可用返回 503。保存失败保留原值。

## 删除与清空

| 接口 | 请求与响应 |
| --- | --- |
| `DELETE /api/fkteams/memory/:id` | 按稳定 ID 删除，返回 `{deleted: 1}`；不存在返回 404 |
| `POST /api/fkteams/memory/clear` | 清空，返回 `{cleared: N}` |
| `DELETE /api/fkteams/memory` | 兼容接口，Body `{summary}`，返回 `{deleted: N}` |

新客户端使用 ID 删除，避免同摘要条目一起被删。删除、清空失败返回错误并保留原数据。
