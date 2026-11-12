import { decodePresentationSnapshot } from "@/types/presentation";
import { decodeQueueItems } from "@/types/events";
import type { SessionDetail } from "@/types/chat";
import { del, get, post } from "./client";

export interface SessionShare {
  id?: string;
  share_id?: string;
  session_id: string;
  title?: string;
  has_password?: boolean;
  allow_tool_details?: boolean;
  message_count?: number;
  created_at?: string | number;
  expires_at?: string | number;
  last_accessed_at?: string | number;
}

export interface CreateSessionShareOptions {
  password?: string;
  expires_in?: number;
  allow_tool_details?: boolean;
}

export function createSessionShare(sessionID: string, options: CreateSessionShareOptions | string = {}) {
  const payload = typeof options === "string" ? { password: options } : options;
  return post<SessionShare>("/api/fkteams/session-shares", { session_id: sessionID, ...payload });
}

export function listSessionShares() {
  return get<{ shares?: SessionShare[] } | SessionShare[]>("/api/fkteams/session-shares");
}

export function deleteSessionShare(shareID: string) {
  return del<{ share_id: string }>(`/api/fkteams/session-shares/${encodeURIComponent(shareID)}`);
}

export function getPublicShareInfo(shareID: string) {
  return get<SessionShare>(`/api/fkteams/public/session-shares/${encodeURIComponent(shareID)}/info`, {
    authFailure: "ignore",
  });
}

export async function accessPublicShare(shareID: string, password = "") {
  const session = await post<Omit<SessionDetail, "queue"> & { queue?: unknown }>(
    `/api/fkteams/public/session-shares/${encodeURIComponent(shareID)}/access`,
    { password },
    { authFailure: "ignore" },
  );
  return { ...session, view: decodePresentationSnapshot(session.view), queue: decodeQueueItems(session.queue) };
}
