import { createAsyncThunk } from "@reduxjs/toolkit";
import { listSessions, getSession } from "@/api/sessions";
import { chatActions, sessionsActions } from "@/app/store";
import type { AgentMessage, ChatViewMessage } from "@/types/chat";
import type { ChatEvent } from "@/types/events";

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
  const viewMessages: ChatViewMessage[] = messages.map((message, index) => {
    const id = `history-${index}`;
    const events = historyEventsToChatEvents(id, message);
    const isMember = isMemberMessage(message);
    const cancelledContent = cancellationContentFromEvents(message.events || []);
    const isSystem = isSystemMessage(message);
    const content = (message.events || [])
      .map((event) => {
        if (event.type === "text") return event.content || "";
        if (event.type === "reasoning") return "";
        return "";
      })
      .join("");
    const role: ChatViewMessage["role"] = isHistoryUserMessage(message) ? "user" : isSystem ? "system" : "assistant";
    return {
      id,
      role,
      agent: message.agent_name,
      content: isSystem ? cancelledContent || message.content || "" : content || message.content || "",
      reasoningContent: isMember ? "" : reasoningContentFromEvents(message.events || []),
      createdAt: message.start_time || message.end_time,
      events,
      hidden: isMember,
    };
  });
  return assignHistoryDisplayOrder(placeMemberMessagesAfterParent(viewMessages));
}

function isHistoryUserMessage(message: AgentMessage) {
  const agentName = (message.agent_name || "").trim().toLowerCase();
  return message.role === "user" || agentName === "用户" || agentName === "user";
}

function isMemberMessage(message: AgentMessage) {
  return Boolean(message.member_call_id || message.member_name || message.member_tool_name);
}

function isSystemMessage(message: AgentMessage) {
  const agentName = (message.agent_name || "").trim().toLowerCase();
  return agentName === "system" || agentName === "系统";
}

function reasoningContentFromEvents(events: AgentMessage["events"]) {
  return (events || [])
    .filter((event) => event.type === "reasoning")
    .map((event) => event.content || "")
    .join("");
}

function cancellationContentFromEvents(events: AgentMessage["events"]) {
  return (events || []).find((event) => event.type === "cancelled")?.content || "";
}

function placeMemberMessagesAfterParent(messages: ChatViewMessage[]) {
  const result: ChatViewMessage[] = [];
  for (const message of messages) {
    if (!message.hidden) {
      result.push(message);
      continue;
    }
    const parentIndex = findParentToolMessageIndex(result, message.events[0]);
    if (parentIndex >= 0) result.splice(parentIndex + 1, 0, message);
    else result.push(message);
  }
  return result;
}

function assignHistoryDisplayOrder(messages: ChatViewMessage[]) {
  let order = 0;
  return messages.map((message) => ({
    ...message,
    events: message.events.map((event) => ({
      ...event,
      display_order: order++,
    })),
  }));
}

function findParentToolMessageIndex(messages: ChatViewMessage[], event?: ChatEvent) {
  if (!event) return -1;
  const refs = new Set(
    [event.parent_tool_call_id, event.member_call_id, event.tool_call_id, event.tool_call_ref]
      .filter(Boolean)
      .flatMap((value) => {
        const ref = String(value);
        return ref.startsWith("tool_call:") ? [ref, ref.slice("tool_call:".length)] : [ref, `tool_call:${ref}`];
      }),
  );
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messageHasToolRef(messages[i], refs, event.member_tool_name)) return i;
  }
  return -1;
}

function messageHasToolRef(message: ChatViewMessage, refs: Set<string>, memberToolName?: string) {
  for (const event of message.events || []) {
    for (const tool of event.tool_calls || []) {
      if (toolMatchesParent(tool, refs, memberToolName)) return true;
    }
    if (event.tool_call && toolMatchesParent(event.tool_call, refs, memberToolName)) return true;
    if (event.tool_call_id && refs.has(event.tool_call_id)) return true;
    if (event.tool_call_ref && refs.has(event.tool_call_ref)) return true;
    if (memberToolName && event.tool_name === memberToolName) return true;
  }
  return false;
}

function toolMatchesParent(tool: { id?: string; ref?: string; name?: string }, refs: Set<string>, memberToolName?: string) {
  return Boolean((tool.id && refs.has(tool.id)) || (tool.ref && refs.has(tool.ref)) || (memberToolName && tool.name === memberToolName));
}

function historyEventsToChatEvents(messageID: string, message: AgentMessage): ChatEvent[] {
  const isUser = isHistoryUserMessage(message);
  const common = {
    message_id: messageID,
    agent_name: message.agent_name,
    run_path: message.run_path,
    is_member_event: isMemberMessage(message) || undefined,
    member_call_id: message.member_call_id,
    member_tool_name: message.member_tool_name,
    member_name: message.member_name,
  };

  return (message.events || []).flatMap((event, index): ChatEvent | ChatEvent[] => {
    const base = {
      ...common,
      sequence: event.sequence,
      created_at: message.start_time || message.end_time,
    };
    if (event.type === "text") {
      if (isUser) {
        return [{
          ...base,
          type: "user_message",
          role: "user",
          content: event.content || "",
        }];
      }
      return {
        ...base,
        type: "assistant_text_delta",
        role: "assistant",
        delta_kind: "output",
        content: event.content || "",
      };
    }
    if (event.type === "reasoning") {
      return {
        ...base,
        type: "assistant_reasoning_delta",
        role: "assistant",
        delta_kind: "reasoning",
        content: event.content || "",
        reasoning_content: event.content || "",
      };
    }
    if (event.type === "tool_call" && event.tool_call) {
      const ref = event.tool_call.ref || (event.tool_call.id ? `tool_call:${event.tool_call.id}` : undefined);
      return [
        {
          ...base,
          type: "tool_call_started",
          tool_name: event.tool_call.name,
          tool_display_name: event.tool_call.display_name,
          tool_kind: event.tool_call.kind,
          tool_target: event.tool_call.target,
          tool_args: event.tool_call.arguments,
          tool_call_id: event.tool_call.id,
          tool_call_ref: ref,
          tool_call_index: event.tool_call.index,
          tool_call: event.tool_call,
          tool_calls: [event.tool_call],
        },
        {
          ...base,
          sequence: event.sequence === undefined ? undefined : event.sequence + 0.1,
          type: "tool_call_completed",
          tool_name: event.tool_call.name,
          tool_display_name: event.tool_call.display_name,
          tool_kind: event.tool_call.kind,
          tool_target: event.tool_call.target,
          tool_args: event.tool_call.arguments,
          tool_result: event.tool_call.result,
          content: event.tool_call.result,
          tool_call_id: event.tool_call.id,
          tool_call_ref: ref,
          tool_call_index: event.tool_call.index,
          tool_call: event.tool_call,
          tool_calls: [event.tool_call],
        },
      ];
    }
    if (event.type === "ask" && event.ask) {
      return {
        ...base,
        type: event.ask.answered ? "ask_answered" : "ask_requested",
        ask_id: event.ask.id,
        question: event.ask.question,
        options: event.ask.options,
        multi_select: event.ask.multi_select,
        selected: event.ask.selected,
        free_text: event.ask.free_text,
        content: event.ask.answered ? askResponseSummary(event.ask.selected || [], event.ask.free_text || "") : event.ask.question || "",
        detail: event.ask.id,
      };
    }
    return {
      ...base,
      type: event.type,
      content: event.content || "",
    };
  }) as ChatEvent[];
}

function askResponseSummary(selected: string[], freeText: string) {
  return [...selected, freeText].filter((item) => item && item.trim()).join("；");
}
