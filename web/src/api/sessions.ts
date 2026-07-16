import type { SessionDetail, SessionSummary } from "@/types/chat";
import { del, get, post } from "./client";

export function listSessions() {
  return get<{ sessions: SessionSummary[] }>("/api/fkteams/sessions");
}

export function createSession(title = "") {
  return post<{ session_id: string }>("/api/fkteams/sessions", { title });
}

export function getSession(sessionID: string, signal?: AbortSignal) {
  return get<SessionDetail>(`/api/fkteams/sessions/${encodeURIComponent(sessionID)}`, { signal });
}

export function deleteSession(sessionID: string) {
  return del<{ session_id: string }>(`/api/fkteams/sessions/${encodeURIComponent(sessionID)}`);
}

export function renameSession(sessionID: string, title: string) {
  return post<{ session_id: string }>("/api/fkteams/sessions/rename", { session_id: sessionID, title });
}

export function favoriteSession(sessionID: string, favorite: boolean) {
  return post<{ session_id: string; favorite: boolean }>("/api/fkteams/sessions/favorite", { session_id: sessionID, favorite });
}

export function updateSessionAgent(sessionID: string, agent: string) {
  return post<{ session_id: string }>("/api/fkteams/sessions/agent", { session_id: sessionID, agent });
}
