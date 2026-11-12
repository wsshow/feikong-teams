# 定时任务 API

定时任务接口用于查看调度器中的任务、取消待执行任务，以及读取任务的最新结果和历史结果。

基础路径：`/api/fkteams/schedules`

## GET /api/fkteams/schedules

获取定时任务列表。

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `status` | string | 否 | 按状态过滤，如 `pending`、`running`、`completed`、`failed`、`cancelled` |

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "tasks": [
      {
        "id": "task_001",
        "task": "每天早上 8 点发送天气报告",
        "cron_expr": "0 8 * * *",
        "one_time": false,
        "next_run_at": "2026-06-11T08:00:00+08:00",
        "status": "pending",
        "created_at": "2026-06-10T12:00:00+08:00",
        "last_run_at": null
      }
    ],
    "total": 1
  }
}
```

`tasks` 元素对应调度任务领域模型：

| 字段 | 说明 |
| ---- | ---- |
| `id` | 任务 ID |
| `task` | 要执行的任务描述 |
| `cron_expr` | cron 表达式，重复任务才有 |
| `one_time` | 是否一次性任务 |
| `next_run_at` | 下次运行时间 |
| `status` | 任务状态 |
| `created_at` | 创建时间 |
| `last_run_at` | 上次运行时间，可能为空 |

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 503 | `scheduler not initialized` | 调度器未初始化 |
| 500 | 错误详情 | 获取任务失败 |

## POST /api/fkteams/schedules/:id/cancel

取消指定任务。是否允许取消由调度器当前状态决定。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "task cancelled"
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `task ID is required` | 缺少任务 ID |
| 400 | 调度器返回的错误 | 任务不存在或当前状态不允许取消 |
| 503 | `scheduler not initialized` | 调度器未初始化 |
| 500 | 错误详情 | 取消失败 |

## GET /api/fkteams/schedules/:id/result

读取任务最新执行结果。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "task_001",
    "result": "# 执行结果\n..."
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `task ID is required` | 缺少任务 ID |
| 503 | `scheduler not initialized` | 调度器未初始化 |
| 404 | 错误详情 | 结果不存在或无法读取 |

## GET /api/fkteams/schedules/:id/history

列出任务历史结果文件。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "task_001",
    "history": [
      {
        "filename": "20260610_150405.md",
        "time": "2026-06-10 15:04:05"
      }
    ],
    "total": 1
  }
}
```

无历史结果时 `history` 返回空数组。

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `task ID is required` | 缺少任务 ID |
| 503 | `scheduler not initialized` | 调度器未初始化 |
| 500 | 错误详情 | 读取历史列表失败 |

## GET /api/fkteams/schedules/:id/history/:filename

读取指定历史结果文件内容。`filename` 只支持 `.md` 文件名，后端会使用文件名基名防止路径穿越。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "task_001",
    "filename": "20260610_150405.md",
    "content": "# 历史结果\n..."
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `task ID and filename are required` | 缺少任务 ID 或文件名 |
| 503 | `scheduler not initialized` | 调度器未初始化 |
| 404 | 错误详情 | 历史文件不存在、类型非法或无法读取 |

## GET /api/fkteams/schedules/notifications

返回 `{tasks: [...]}`，最多 20 条 `notification_pending=true` 的任务。`last_run_at` 标识待确认的运行，`last_run_status` 表示这次执行结果；周期任务的 `status` 可以已经回到 `pending`。`interrupted=true` 表示需要核对实际效果后再安排，不能直接自动重跑。调度服务未启用时返回空数组。

## POST /api/fkteams/schedules/:id/acknowledge

请求体为 `{"run_at":"2026-11-12T10:00:00Z"}`，必须使用通知中的原始 `last_run_at`。成功返回 `{acknowledged:true}`；重复确认同一次运行幂等。若已有更新结果则返回 409，不清除新通知；存储失败返回错误，保留待通知状态。确认不会执行任务或改变其运行状态。
