export interface ScheduleTask {
  notification_pending?: boolean;
  last_run_status?: string;
  interrupted?: boolean;
  id: string;
  task: string;
  status: string;
  cron_expr?: string;
  next_run_at?: string;
  last_run_at?: string;
  created_at?: string;
}

export interface ScheduleTaskPayload {
  task: string;
  cron_expr?: string;
  execute_at?: string;
}

export interface ScheduleHistoryEntry {
	filename?: string;
	time?: string;
	created_at?: string;
	status?: string;
	content?: string;
}
