import type { ScheduleHistoryEntry, ScheduleTask, ScheduleTaskPayload } from "@/types/schedules";
import { del, get, post, put } from "./client";

export function listSchedules(status = "") {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return get<{ tasks: ScheduleTask[] }>(`/api/fkteams/schedules${query}`);
}

export function createSchedule(payload: ScheduleTaskPayload) {
  return post<{ task: ScheduleTask }>("/api/fkteams/schedules", payload);
}

export function updateSchedule(id: string, payload: ScheduleTaskPayload) {
  return put<{ task: ScheduleTask }>(`/api/fkteams/schedules/${encodeURIComponent(id)}`, payload);
}

export function cancelSchedule(id: string) {
  return post<{ id: string }>(`/api/fkteams/schedules/${encodeURIComponent(id)}/cancel`);
}

export function deleteSchedule(id: string) {
  return del<{ id: string }>(`/api/fkteams/schedules/${encodeURIComponent(id)}`);
}

export function getScheduleResult(id: string) {
  return get<{ result?: string; content?: string }>(`/api/fkteams/schedules/${encodeURIComponent(id)}/result`);
}

export function getScheduleHistory(id: string) {
  return get<{ entries?: ScheduleHistoryEntry[]; history?: ScheduleHistoryEntry[]; count?: number; total?: number }>(
    `/api/fkteams/schedules/${encodeURIComponent(id)}/history`,
  );
}

export function listScheduleNotifications(signal?: AbortSignal) {
  return get<{ tasks: ScheduleTask[] }>("/api/fkteams/schedules/notifications", { signal });
}

export function acknowledgeScheduleResult(id: string, runAt: string, signal?: AbortSignal) {
  return post<{ acknowledged: boolean }>(`/api/fkteams/schedules/${encodeURIComponent(id)}/acknowledge`, { run_at: runAt }, { signal });
}
