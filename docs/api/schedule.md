# 定时任务 API

基础路径 `/api/fkteams/schedules`，认证与响应包装见 [API 总览](README.md)。调度器未启用时普通操作返回 503，通知查询返回空任务列表。

## 创建与修改

`POST /api/fkteams/schedules` 创建，`PUT /api/fkteams/schedules/:id` 修改；均返回 `{task: {...}}`。请求填写 `task`，并在 `execute_at` 和 `cron_expr` 中选择一种安排：

```json
{"task":"检查发布状态并整理报告","execute_at":"2026-12-01T09:00:00+08:00"}
```

```json
{"task":"整理工作日简报","cron_expr":"CRON_TZ=Asia/Shanghai 0 9 * * 1-5"}
```

一次执行使用带时区的 ISO 8601 时间；周期任务使用五段 cron，可加 `CRON_TZ` 前缀。更新需提交完整任务与时间安排；执行中的任务不能修改，冲突返回 409。无效安排返回 400，不存在返回 404。

## GET /api/fkteams/schedules

可用 `?status=pending` 筛选，返回 `{tasks: [...], total: N}`。

| 任务字段 | 说明 |
| --- | --- |
| `id` / `task` | 任务标识与执行内容 |
| `cron_expr` / `one_time` | 周期表达式与是否一次执行 |
| `next_run_at` | 下一次运行时间 |
| `status` | pending、running、completed、failed 或 cancelled |
| `created_at` / `last_run_at` | 创建与上次运行时间，后者可省略 |
| `notification_pending` | 是否有未确认结果 |
| `last_run_status` | 最近一次执行状态；周期任务的整体 status 可能已恢复 pending |
| `interrupted` | 上次执行被应用退出或崩溃中断，需先核对实际效果 |

## 取消与删除

| 接口 | 行为与成功响应 |
| --- | --- |
| `POST /api/fkteams/schedules/:id/cancel` | 取消安排，返回 `{message: "task cancelled"}` |
| `DELETE /api/fkteams/schedules/:id` | 删除任务，返回 `{message: "task deleted"}` |

当前状态不允许操作时返回冲突。运行中断不会自动重跑可能已经产生副作用的工作。

## 结果与历史

| 接口 | 响应的 data |
| --- | --- |
| `GET /api/fkteams/schedules/:id/result` | `{task_id, result}`，result 为最新 Markdown 报告 |
| `GET /api/fkteams/schedules/:id/history` | `{task_id, history: [{filename, time}], total}` |
| `GET /api/fkteams/schedules/:id/history/:filename` | `{task_id, filename, content}` |

无历史时返回空列表。指定历史只能读取安全的 `.md` 文件名，不接受目录路径；结果不存在返回 404。

## 后台结果通知

`GET /api/fkteams/schedules/notifications` 返回 `{tasks: [...]}`，最多 20 条待通知任务。周期任务的未读通知合并为最新一次，完整报告仍在历史中。

`POST /api/fkteams/schedules/:id/acknowledge` 确认对应运行的通知：

```json
{"run_at":"2026-11-13T10:00:00Z"}
```

必须原样使用通知的 `last_run_at`。成功返回 `{acknowledged: true}`，同一次确认可重试；已有更新结果返回 409，保留新通知。确认只清除通知，不执行任务。用户端行为见[个人助手](../personal-assistant.md#后台安排与结果)。
