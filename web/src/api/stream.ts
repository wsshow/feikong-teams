import type { ChatEvent, QueueItem } from "@/types/events";
import { expireAuthentication } from "@/lib/auth-session";
import { authToken } from "@/lib/storage";
import { APIError, del, get, patch, post } from "./client";

export function streamStatus(sessionID: string) {
  return get<{
    status: string;
    has_task?: boolean;
    event_count?: number;
    session_id?: string;
    mode?: string;
    agent_name?: string;
    created_at?: string;
    finished_at?: string;
  }>(`/api/fkteams/stream/status/${encodeURIComponent(sessionID)}`);
}

export interface StreamSnapshot {
  session_id: string;
  status: string;
  has_task?: boolean;
  mode?: string;
  agent_name?: string;
  event_count: number;
  next_offset: number;
  snapshot_offset?: number;
  more_available?: boolean;
  limit?: number;
  queue?: QueueItem[];
  events?: ChatEvent[];
  created_at?: string;
  finished_at?: string;
}

export function streamSnapshot(sessionID: string, options?: { offset?: number; limit?: number }) {
  const params = new URLSearchParams();
  if (options?.offset !== undefined) params.set("offset", String(options.offset));
  if (options?.limit !== undefined) params.set("limit", String(options.limit));
  const query = params.toString();
  return get<StreamSnapshot>(
    `/api/fkteams/stream/snapshot/${encodeURIComponent(sessionID)}${query ? `?${query}` : ""}`,
  );
}

export function streamQueue(sessionID: string) {
  return get<{ queue: QueueItem[] }>(`/api/fkteams/stream/queue/${encodeURIComponent(sessionID)}`);
}

export function updateQueueItem(sessionID: string, queueID: string, content: string) {
  return patch<{ queue: QueueItem[] }>(
    `/api/fkteams/stream/queue/${encodeURIComponent(sessionID)}/${encodeURIComponent(queueID)}`,
    { content },
  );
}

export function deleteQueueItem(sessionID: string, queueID: string) {
  return del<{ queue: QueueItem[] }>(
    `/api/fkteams/stream/queue/${encodeURIComponent(sessionID)}/${encodeURIComponent(queueID)}`,
  );
}

export function changeQueueKind(sessionID: string, queueID: string, kind: string) {
  return post<{ queue: QueueItem[] }>(
    `/api/fkteams/stream/queue/${encodeURIComponent(sessionID)}/${encodeURIComponent(queueID)}/kind`,
    { kind },
  );
}

export function moveQueueItem(sessionID: string, queueID: string, direction: "up" | "down") {
  return post<{ queue: QueueItem[] }>(
    `/api/fkteams/stream/queue/${encodeURIComponent(sessionID)}/${encodeURIComponent(queueID)}/move`,
    { direction },
  );
}

export function submitAskResponse(
  sessionID: string,
  askID: string,
  payload: { selected?: string[]; free_text?: string },
) {
  return post<{ message: string }>("/api/fkteams/stream/ask-response", {
    session_id: sessionID,
    ask_id: askID,
    selected: payload.selected || [],
    free_text: payload.free_text || "",
  });
}

export function submitApproval(sessionID: string, decision: 0 | 1 | 2) {
  return post<{ message: string }>("/api/fkteams/stream/approval", {
    session_id: sessionID,
    decision,
  });
}

export async function subscribeStream(
  sessionID: string,
  offset: number,
  onEvent: (event: ChatEvent) => void,
  signal?: AbortSignal,
) {
  const headers = new Headers();
  const token = authToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(
    `/api/fkteams/stream/subscribe/${encodeURIComponent(sessionID)}?offset=${encodeURIComponent(String(offset))}`,
    { headers, signal },
  );
  if (response.status === 401) {
    expireAuthentication();
    throw new APIError("未登录或登录已过期", response.status);
  }
  if (!response.ok) throw new APIError(response.statusText || "stream subscribe failed", response.status);
  if (!response.body) throw new Error("stream response body is empty");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const chunks = buffer.split("\n\n");
    buffer = chunks.pop() || "";
    for (const chunk of chunks) {
      const lines = chunk.split("\n");
      const idLine = lines.find((part) => part.startsWith("id:"));
      const dataLines = lines.filter((part) => part.startsWith("data:"));
      if (dataLines.length === 0) continue;
      const raw = dataLines.map((line) => line.replace(/^data:\s*/, "")).join("\n");
      if (!raw) continue;
      if (raw === "[DONE]") return;
      const event = JSON.parse(raw) as ChatEvent;
      if (idLine && event.stream_event_id === undefined) {
        const id = Number(idLine.replace(/^id:\s*/, ""));
        if (Number.isFinite(id)) event.stream_event_id = id;
      }
      onEvent(event);
    }
  }
}
