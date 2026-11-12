import { decodeChatEvents, decodeQueueItems } from "@/types/events";
import type { SessionDetail, SessionSummary } from "@/types/chat";
import { del, get, post } from "./client";

export function listSessions() {
  return get<{ sessions: SessionSummary[] }>("/api/fkteams/sessions");
}

export function createSession(title = "") {
  return post<{ session_id: string }>("/api/fkteams/sessions", { title });
}

export function openAssistant() {
  return post<SessionSummary>("/api/fkteams/assistant", {});
}

export function getAssistant() {
  return get<SessionSummary>("/api/fkteams/assistant");
}

export async function getSession(sessionID: string, signal?: AbortSignal) {
  const session = await get<Omit<SessionDetail, "events" | "queue"> & { events?: unknown; queue?: unknown }>(`/api/fkteams/sessions/${encodeURIComponent(sessionID)}`, { signal });
  return { ...session, events: decodeChatEvents(session.events), queue: decodeQueueItems(session.queue) };
}

export function deleteSession(sessionID: string) {
  return del<{ session_id: string }>(`/api/fkteams/sessions/${encodeURIComponent(sessionID)}`);
}

export function renameSession(sessionID: string, title: string) {
  return post<{ session_id: string; title: string }>("/api/fkteams/sessions/rename", { session_id: sessionID, title });
}

export function favoriteSession(sessionID: string, favorite: boolean) {
  return post<{ session_id: string; favorite: boolean }>("/api/fkteams/sessions/favorite", { session_id: sessionID, favorite });
}

export function updateSessionAgent(sessionID: string, agent: string) {
  return post<{ session_id: string }>("/api/fkteams/sessions/agent", { session_id: sessionID, agent });
}
