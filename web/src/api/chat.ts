import { post } from "./client";
import type { ContentPartDTO, QueueItem } from "@/types/events";

export interface StartStreamRequest {
  session_id?: string;
  message?: string;
  mode?: string;
  agent_name?: string;
  contents?: ContentPartDTO[];
}

export interface StartStreamResponse {
  session_id: string;
  run_id?: string;
  user_message_id?: string;
  status?: "processing" | "queued" | string;
  queue_kind?: string;
  queue?: QueueItem[];
  queued_count?: number;
}

export function startStream(req: StartStreamRequest) {
  return post<StartStreamResponse>("/api/fkteams/stream/start", req);
}

export function stopStream(sessionID: string) {
  return post<{ session_id: string }>(`/api/fkteams/stream/stop/${encodeURIComponent(sessionID)}`);
}

export function sendSteering(sessionID: string, message: string, contents?: ContentPartDTO[]) {
  return post<StartStreamResponse>("/api/fkteams/stream/steer", { session_id: sessionID, message, contents });
}
