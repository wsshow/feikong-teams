import { createAsyncThunk } from "@reduxjs/toolkit";
import { listSessions, getSession } from "@/api/sessions";
import { chatActions, sessionsActions } from "@/app/store";
import type { AgentMessage, ChatViewMessage } from "@/types/chat";

export const loadSessions = createAsyncThunk("sessions/load", async (_, { dispatch }) => {
  dispatch(sessionsActions.setSessionsLoading(true));
  const result = await listSessions();
  dispatch(sessionsActions.setSessions(result.sessions || []));
});

export const loadSessionDetail = createAsyncThunk("sessions/detail", async (sessionID: string, { dispatch }) => {
  const detail = await getSession(sessionID);
  const messages = historyToMessages(detail.messages || []);
  dispatch(chatActions.setMessages(messages));
  dispatch(chatActions.setActiveSession(sessionID));
});

function historyToMessages(messages: AgentMessage[]): ChatViewMessage[] {
  return messages.map((message, index) => {
    const reasoningContent = (message.events || [])
      .filter((event) => event.type === "reasoning")
      .map((event) => event.content || "")
      .join("");
    const content = (message.events || [])
      .map((event) => {
        if (event.type === "text") return event.content || "";
        if (event.type === "reasoning") return "";
        if (event.type === "tool_call" && event.tool_call) {
          return `\n[${event.tool_call.display_name || event.tool_call.name}] ${event.tool_call.result || ""}\n`;
        }
        if (event.type === "action" && event.action) return `\n${event.action.content || event.action.action_type || ""}\n`;
        return event.content || "";
      })
      .join("");
    return {
      id: `history-${index}`,
      role: isHistoryUserMessage(message) ? "user" : "assistant",
      agent: message.agent_name,
      content: content || message.content || "",
      reasoningContent,
      createdAt: message.start_time || message.end_time,
      events: [],
    };
  });
}

function isHistoryUserMessage(message: AgentMessage) {
  const agentName = (message.agent_name || "").trim().toLowerCase();
  return message.role === "user" || agentName === "用户" || agentName === "user";
}
