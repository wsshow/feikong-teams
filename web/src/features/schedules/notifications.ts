import { acknowledgeScheduleResult, listScheduleNotifications } from "@/api/schedules";
import type { ScheduleTask } from "@/types/schedules";

export function notificationMessage(tasks: ScheduleTask[]) {
  if (tasks.length !== 1) return `${tasks.length} 项后台任务有新结果，可在定时任务中查看。`;
  const task = tasks[0];
  const status = task.interrupted ? "执行中断，请核对结果后再重试" : task.last_run_status === "failed" ? "执行失败" : task.last_run_status === "cancelled" ? "已取消" : "已完成";
  return `${task.task.slice(0, 80)}：${status}。可在定时任务中查看。`;
}

// 通知确认与业务执行分离；网络失败只重试确认，不重跑任务。
export async function deliverScheduleNotifications(
  shown: Set<string>,
  show: (message: string) => void,
  signal: AbortSignal,
) {
  const result = await listScheduleNotifications(signal);
  if (signal.aborted) return;
  const tasks = (result.tasks || []).filter((task) => task.notification_pending && task.last_run_at);
  const key = (task: ScheduleTask) => `${task.id}:${task.last_run_at}`;
  const fresh = tasks.filter((task) => !shown.has(key(task)));
  if (fresh.length) {
    show(notificationMessage(fresh));
    for (const task of fresh) shown.add(key(task));
  }
  await Promise.allSettled(tasks.map((task) => acknowledgeScheduleResult(task.id, task.last_run_at!, signal)));
  const pending = new Set(tasks.map(key));
  for (const item of shown) if (!pending.has(item)) shown.delete(item);
}

