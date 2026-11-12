import { startStream } from "@/api/chat";
import { chatActions, sessionsActions, type AppDispatch, type RootState } from "@/app/store";
import { assistantAgentID, assistantSessionID } from "@/lib/assistant";
import { chatPath, pushAppPath } from "@/lib/navigation";
import type { ContentPartDTO } from "@/types/events";
import { clearStreamOffset } from "./streamOffsets";

interface SendMessageRequest {
  sessionID: string;
  message: string;
  displayText?: string;
  contents?: ContentPartDTO[];
  mode: string;
  agent: string;
  newSession?: boolean;
}

// 输入框和生成界面共用提交、队列接纳及失败回滚，避免维护第二条对话链路。
export function sendChatMessage(request: SendMessageRequest) {
  return async (dispatch: AppDispatch, getState: () => RootState) => {
    const { sessionID, message, contents, newSession = false } = request;
    const task = getState().chat.runningTasks[sessionID];
    if (task?.phase === "starting") throw new Error("上一条消息正在提交，请稍后重试");
    const mode = sessionID === assistantSessionID ? "team" : request.mode;
    const agent = sessionID === assistantSessionID ? assistantAgentID : request.agent;
    const queueing = task?.phase === "processing";
    const startedAt = Date.now();
    const messageID = `user-${newChatID()}`;
    const displayText = request.displayText ?? message;
    dispatch(chatActions.setError(undefined));
    if (!queueing) {
      dispatch(chatActions.appendUserMessage({ id: messageID, content: displayText, sessionID, contentParts: contents, createdAt: new Date(startedAt).toISOString() }));
      dispatch(chatActions.beginRunningSession({ sessionID, startedAt }));
    }
    try {
      const result = await startStream({ session_id: sessionID, message, contents, mode, agent_name: agent || undefined });
      const visible = getState().chat.activeSessionID === sessionID;
      if (result.status === "queued") {
        if (visible && Array.isArray(result.queue)) dispatch(chatActions.setQueue(result.queue));
        if (!queueing) dispatch(chatActions.rollbackUserMessage({ id: messageID, sessionID, resetSession: false }));
      }
      const now = new Date().toISOString();
      if (newSession) {
        const runes = Array.from(displayText);
        dispatch(sessionsActions.upsertSession({ session_id: result.session_id, title: runes.length <= 50 ? displayText : `${runes.slice(0, 50).join("")}...`, status: "processing", active_task: true, mode, current_agent: agent, mod_time: now, updated_at: now }));
      } else {
        dispatch(sessionsActions.updateSessionRuntime({ sessionID: result.session_id, status: "processing", activeTask: true, updatedAt: now, mode, currentAgent: agent }));
      }
      const fresh = result.status !== "queued";
      if (fresh) clearStreamOffset(result.session_id);
      if (!queueing || fresh) dispatch(chatActions.activateRunningSession({ sessionID: result.session_id, initialOffset: fresh ? 0 : undefined, startedAt, activate: visible }));
      if (newSession && visible) pushAppPath(chatPath(result.session_id));
      return result;
    } catch (error) {
      if (!queueing) {
        dispatch(chatActions.finishRunningSession(sessionID));
        dispatch(chatActions.rollbackUserMessage({ id: messageID, sessionID, resetSession: newSession }));
      }
      throw error;
    }
  };
}

export function newChatID() {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
