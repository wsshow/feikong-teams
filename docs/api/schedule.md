# 定时任务

## GET /api/fkteams/schedules

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

## POST /api/fkteams/schedules/:id/cancel

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
