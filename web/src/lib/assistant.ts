import type { SessionSummary } from "@/types/chat";

export const assistantSessionID = "workspace-assistant";
export const assistantAgentID = "workspace_assistant";

export function assistantDisplayName(sessions: SessionSummary[]) {
  return sessions.find((session) => session.session_id === assistantSessionID)?.title || "助手";
}
