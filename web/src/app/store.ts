import { configureStore, createSlice, type PayloadAction } from "@reduxjs/toolkit";
import type { AgentInfo, VersionInfo } from "@/types/api";
import type { ChatEvent, ContentPartDTO, QueueItem } from "@/types/events";
import type { ChatState, ChatViewMessage, SessionDetail, SessionSummary } from "@/types/chat";
import type { AppConfig, ToolInfo } from "@/types/config";
import type { FileEntry } from "@/types/files";
import type { ScheduleTask } from "@/types/schedules";
import type { SkillInfo } from "@/types/skills";
import { storageKeys } from "@/lib/storage";
import { chatSessionIDFromPath, panelFromPath } from "@/lib/navigation";

export type AppPanel = "chat" | "config" | "files" | "schedules" | "shares" | "skills";

const initialChatState: ChatState = {
  activeSessionID: chatSessionIDFromPath(location.pathname),
  viewSessionID: "",
  runningSessionID: "",
  currentAgent: "",
  mode: "team",
  messages: [],
  events: [],
  queue: [],
  isProcessing: false,
  connectionState: "disconnected",
};

const chatSlice = createSlice({
  name: "chat",
  initialState: initialChatState,
  reducers: {
    setActiveSession(state, action: PayloadAction<string>) {
      state.activeSessionID = action.payload;
      if (action.payload) localStorage.setItem(storageKeys.sessionID, action.payload);
      else localStorage.removeItem(storageKeys.sessionID);
    },
    setMode(state, action: PayloadAction<string>) {
      state.mode = action.payload;
    },
    setCurrentAgent(state, action: PayloadAction<string>) {
      state.currentAgent = action.payload;
    },
    setConnectionState(state, action: PayloadAction<ChatState["connectionState"]>) {
      state.connectionState = action.payload;
    },
    setProcessing(state, action: PayloadAction<boolean>) {
      state.isProcessing = action.payload;
      if (!action.payload) {
        state.runningSessionID = "";
        state.streamInitialOffset = undefined;
      }
    },
    activateRunningSession(state, action: PayloadAction<{ sessionID: string; initialOffset?: number }>) {
      const { sessionID, initialOffset } = action.payload;
      state.activeSessionID = sessionID;
      state.runningSessionID = sessionID;
      state.streamInitialOffset = initialOffset;
      state.isProcessing = true;
      localStorage.setItem(storageKeys.sessionID, sessionID);
    },
    consumeStreamInitialOffset(state) {
      state.streamInitialOffset = undefined;
    },
    finishRunningSession(state, action: PayloadAction<string>) {
      if (state.runningSessionID !== action.payload) return;
      state.runningSessionID = "";
      state.streamInitialOffset = undefined;
      state.isProcessing = false;
    },
    clearMessages(state) {
      state.viewSessionID = "";
      state.messages = [];
      state.events = [];
      state.queue = [];
      state.isProcessing = false;
      state.runningSessionID = "";
      state.streamInitialOffset = undefined;
      state.error = undefined;
      state.errorTitle = undefined;
      state.errorSuggestions = undefined;
      state.technicalError = undefined;
      state.statusText = undefined;
    },
    setSessionDetail(state, action: PayloadAction<SessionDetail>) {
      const detail = action.payload;
      state.activeSessionID = detail.session_id;
      state.viewSessionID = detail.session_id;
      if (detail.session_id) localStorage.setItem(storageKeys.sessionID, detail.session_id);
      else localStorage.removeItem(storageKeys.sessionID);
      state.messages = [];
      state.events = [];
      state.queue = [];
      state.error = undefined;
      state.errorTitle = undefined;
      state.errorSuggestions = undefined;
      state.technicalError = undefined;
      state.statusText = undefined;
      for (const event of detail.events || []) {
        applyChatEvent(state, event);
      }
      state.queue = detail.queue || [];
      if (detail.active_task) {
        state.runningSessionID = detail.session_id;
        state.streamInitialOffset = undefined;
        state.isProcessing = true;
      } else {
        state.runningSessionID = "";
        state.streamInitialOffset = undefined;
        state.isProcessing = false;
      }
    },
    appendUserMessage(state, action: PayloadAction<{ id: string; content: string; sessionID?: string; contentParts?: ContentPartDTO[]; createdAt?: string }>) {
      const sessionID = action.payload.sessionID || state.activeSessionID;
      state.viewSessionID = sessionID;
      const event: ChatEvent = {
        type: "user_message",
        session_id: sessionID,
        event_id: `local:${action.payload.id}`,
        content: action.payload.content,
        content_parts: action.payload.contentParts,
        created_at: action.payload.createdAt,
      };
      state.events.push(event);
      state.messages.push({
        id: action.payload.id,
        role: "user",
        content: action.payload.content,
        contentParts: action.payload.contentParts,
        createdAt: action.payload.createdAt,
        events: [event],
      });
    },
    receiveEvent(state, action: PayloadAction<ChatEvent>) {
      applyChatEvent(state, action.payload);
    },
    setQueue(state, action: PayloadAction<QueueItem[]>) {
      state.queue = action.payload;
    },
    setError(state, action: PayloadAction<string | undefined>) {
      state.error = action.payload;
      state.errorTitle = undefined;
      state.errorSuggestions = undefined;
      state.technicalError = undefined;
    },
  },
});

function applyChatEvent(state: ChatState, payload: ChatEvent) {
  const event = { ...payload };
  if (event.session_id && state.activeSessionID && event.session_id !== state.activeSessionID) {
    if ((event.type === "processing_end" || event.type === "cancelled" || event.type === "error") && state.runningSessionID === event.session_id) {
      state.isProcessing = false;
      state.runningSessionID = "";
      state.streamInitialOffset = undefined;
    }
    return;
  }
  if (state.events.some((item) => sameEventIdentity(item, event))) return;
  state.events.push(event);
  if (event.type === "queue_updated" && Array.isArray(event.queue)) {
    state.queue = event.queue;
  }
  if (event.type === "processing_start") {
    state.isProcessing = true;
    if (event.session_id) state.runningSessionID = event.session_id;
    state.statusText = String(event.message || event.content || "处理中");
  }
  if (isModelResponseEvent(event)) {
    state.statusText = undefined;
  }
  if (event.type === "user_message") {
    const content = eventText(event);
    const contentParts = Array.isArray(event.content_parts) ? event.content_parts : [];
    if (!content && contentParts.length === 0) return;
    const eventExists = state.messages.some((item) => item.role === "user" && item.events.some((messageEvent) => sameEventIdentity(messageEvent, event)));
    if (eventExists) return;
    const mergeTarget = content
      ? findMergeableLocalUserMessage(state.messages, content)
      : findMergeableLocalUserAttachmentMessage(state.messages, contentParts);
    if (mergeTarget) {
      mergeTarget.createdAt = mergeTarget.createdAt || event.created_at;
      mergeTarget.contentParts = mergeTarget.contentParts?.length ? mergeTarget.contentParts : contentParts;
      mergeTarget.events.push(event);
      return;
    }
    state.messages.push({
      id: `user-${eventIdentityKey(event)}`,
      role: "user",
      content,
      contentParts,
      createdAt: event.created_at,
      events: [event],
    });
  }
  if (isMemberActivityEvent(event)) {
    const key = memberActivityKey(event);
    const id = `member-${key}`;
    let message = state.messages.find((item) => item.id === id);
    if (!message) {
      message = {
        id,
        role: "assistant",
        agent: event.member_name || event.agent_name,
        content: "",
        events: [],
        hidden: true,
      };
      const parentIndex = findParentToolMessageIndex(state.messages, event);
      if (parentIndex >= 0) state.messages.splice(parentIndex + 1, 0, message);
      else state.messages.push(message);
    }
    message.events.push(event);
  }
  if (shouldAttachAssistantMessage(event)) {
    const key = assistantMessageKey(event);
    let message = state.messages.find((item) => item.id === key);
    if (!message) {
      message = {
        id: key,
        role: "assistant",
        agent: event.agent_name,
        content: "",
        events: [],
      };
      state.messages.push(message);
    }
    const content = eventText(event);
    if (isAssistantTextDelta(event) && content) {
      message.content += content;
    }
    if (event.type === "assistant_completed") {
      if (!message.content && content) message.content = content;
      if (event.reasoning_content) message.reasoningContent = String(event.reasoning_content);
    }
    message.events.push(event);
  }
  if (event.type === "system_notice") {
    state.statusText = eventText(event) || state.statusText;
  }
  if (event.type === "error") {
    state.error = friendlyEventMessage(event);
    state.errorTitle = event.error_title;
    state.errorSuggestions = Array.isArray(event.error_suggestions) ? event.error_suggestions : undefined;
    state.technicalError = event.technical_error || event.error || event.content || event.message;
    state.isProcessing = false;
    state.runningSessionID = "";
    state.streamInitialOffset = undefined;
    state.statusText = undefined;
  }
  if (event.type === "cancelled" || event.type === "processing_end") {
    state.isProcessing = false;
    state.runningSessionID = "";
    state.streamInitialOffset = undefined;
    state.statusText = String(event.message || event.content || "");
  }
  if (event.type === "cancelled") {
    const content = eventText(event) || "任务已取消";
    const exists = state.messages.some((message) => message.role === "system" && message.events.some((item) => sameEventIdentity(item, event)));
    if (!exists) {
      state.messages.push({
        id: `cancelled-${event.event_id ?? event.sequence ?? Date.now()}`,
        role: "system",
        content,
        createdAt: event.created_at,
        events: [event],
      });
    }
  }
}

function eventText(event: ChatEvent) {
  return String(event.content || event.message || "");
}

function friendlyEventMessage(event: ChatEvent) {
  return String(event.display_error || event.error_title || event.error || event.content || event.message || "请求失败");
}

function findMergeableLocalUserMessage(messages: ChatViewMessage[], content: string) {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (message.role !== "user") return undefined;
    if (!hasLocalUserEvent(message)) return undefined;
    if (message.content === content) return message;
  }
  return undefined;
}

function findMergeableLocalUserAttachmentMessage(messages: ChatViewMessage[], contentParts: ContentPartDTO[]) {
  if (!contentParts.length) return undefined;
  const signature = contentPartsSignature(contentParts);
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (message.role !== "user") return undefined;
    if (!hasLocalUserEvent(message)) return undefined;
    if (contentPartsSignature(message.contentParts || []) === signature) return message;
  }
  return undefined;
}

function hasLocalUserEvent(message: ChatViewMessage) {
  return (message.events || []).some((event) => event.type === "user_message" && event.event_id?.startsWith("local:"));
}

function contentPartsSignature(parts: ContentPartDTO[]) {
  return parts
    .filter((part) => part.type !== "text")
    .map((part) => `${part.type}:${part.url || ""}:${part.mime_type || ""}:${part.base64_data?.slice(0, 64) || ""}`)
    .join("|");
}

function sameEventIdentity(left: ChatEvent, right: ChatEvent) {
  if (left.event_id && right.event_id) return left.event_id === right.event_id;
  if (left.run_id && right.run_id && left.sequence !== undefined && right.sequence !== undefined) {
    return left.run_id === right.run_id && left.sequence === right.sequence;
  }
  return false;
}

function eventIdentityKey(event: ChatEvent) {
  if (event.event_id) return event.event_id;
  if (event.run_id && event.sequence !== undefined) return `${event.run_id}:${event.sequence}`;
  if (event.turn_id && event.sequence !== undefined) return `${event.turn_id}:${event.sequence}`;
  if (event.sequence !== undefined) return String(event.sequence);
  return `${event.type}:${Date.now()}`;
}

function shouldAttachAssistantMessage(event: ChatEvent) {
  if (isMemberActivityEvent(event)) {
    return false;
  }
  if (event.type === "assistant_started" || event.type === "assistant_reasoning_delta" || event.type === "assistant_text_delta" || event.type === "assistant_completed") {
    return true;
  }
  if (hasToolActivity(event)) {
    return true;
  }
  return false;
}

function assistantMessageKey(event: ChatEvent) {
  if (event.message_id) return event.message_id;
  if (event.stream_id && event.delta_kind) {
    const suffix = `:${event.delta_kind}`;
    if (event.stream_id.endsWith(suffix)) return event.stream_id.slice(0, -suffix.length);
  }
  if (event.turn_id) return `${event.turn_id}:${event.agent_name || "assistant"}`;
  if (event.run_id) return `${event.run_id}:${event.agent_name || "assistant"}`;
  return event.stream_id || event.agent_name || "assistant";
}

function isOutputDelta(event: ChatEvent) {
  return event.delta_kind === "output" || event.delta_kind === "" || event.delta_kind === undefined;
}

function isAssistantTextDelta(event: ChatEvent) {
  return event.type === "assistant_text_delta" && isOutputDelta(event);
}

function isModelResponseEvent(event: ChatEvent) {
  return event.type === "assistant_started" ||
    event.type === "assistant_reasoning_delta" ||
    event.type === "assistant_text_delta" ||
    event.type === "assistant_completed";
}

function hasToolActivity(event: ChatEvent) {
  return Boolean(event.tool_calls?.length || event.tool_call || event.tool_name || event.tool_call_ref || event.tool_call_id);
}

function isMemberActivityEvent(event: ChatEvent) {
  return Boolean(event.is_member_event || event.member_call_id || event.member_name || event.member_tool_name || event.parent_tool_call_id);
}

function memberActivityKey(event: ChatEvent) {
  if (event.member_call_id) return event.member_call_id;
  if (event.parent_tool_call_id) return event.parent_tool_call_id;
  if (event.message_id) return event.message_id;
  if (event.stream_id) return event.stream_id;
  return eventIdentityKey(event);
}

function findParentToolMessageIndex(messages: ChatViewMessage[], event: ChatEvent) {
  const refs = new Set(
    [event.parent_tool_call_id, event.member_call_id, event.tool_call_id, event.tool_call_ref]
      .filter(Boolean)
      .flatMap((value) => {
        const ref = String(value);
        return ref.startsWith("tool_call:") ? [ref, ref.slice("tool_call:".length)] : [ref, `tool_call:${ref}`];
      }),
  );
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messageHasToolRef(messages[i], refs)) return i;
  }
  return -1;
}

function messageHasToolRef(message: ChatViewMessage, refs: Set<string>) {
  for (const event of message.events || []) {
    for (const tool of event.tool_calls || []) {
      if (toolMatchesParent(tool, refs)) return true;
    }
    if (event.tool_call && toolMatchesParent(event.tool_call, refs)) return true;
    if (event.tool_call_id && refs.has(event.tool_call_id)) return true;
    if (event.tool_call_ref && refs.has(event.tool_call_ref)) return true;
  }
  return false;
}

function toolMatchesParent(tool: { id?: string; ref?: string }, refs: Set<string>) {
  return Boolean((tool.id && refs.has(tool.id)) || (tool.ref && refs.has(tool.ref)));
}

const sessionsSlice = createSlice({
  name: "sessions",
  initialState: {
    items: [] as SessionSummary[],
    loading: false,
    search: "",
  },
  reducers: {
    setSessions(state, action: PayloadAction<SessionSummary[]>) {
      const activeSessions = new Map(
        state.items.filter((session) => session.active_task).map((session) => [session.session_id, session]),
      );
      const received = new Set(action.payload.map((session) => session.session_id));
      const missingActiveSessions = [...activeSessions.values()].filter((session) => !received.has(session.session_id));
      state.items = [
        ...missingActiveSessions,
        ...action.payload.map((session) => {
          const active = activeSessions.get(session.session_id);
          if (!active) return session;
          return {
            ...session,
            status: active.status || session.status,
            active_task: true,
            mod_time: active.mod_time || session.mod_time,
            updated_at: active.updated_at || session.updated_at,
          };
        }),
      ];
      state.loading = false;
    },
    upsertSession(state, action: PayloadAction<SessionSummary>) {
      const next = action.payload;
      const index = state.items.findIndex((item) => item.session_id === next.session_id);
      if (index < 0) {
        state.items.unshift(next);
        return;
      }
      const current = state.items[index];
      state.items[index] = {
        ...current,
        ...next,
        title: next.title || current.title,
        status: next.status || current.status,
        mod_time: next.mod_time || current.mod_time,
        updated_at: next.updated_at || current.updated_at,
      };
    },
    updateSessionRuntime(state, action: PayloadAction<{ sessionID: string; status?: string; activeTask: boolean; updatedAt?: string }>) {
      const { sessionID, status, activeTask, updatedAt } = action.payload;
      const session = state.items.find((item) => item.session_id === sessionID);
      if (!session) return;
      if (status) session.status = status;
      session.active_task = activeTask;
      if (updatedAt) {
        session.mod_time = updatedAt;
        session.updated_at = updatedAt;
      }
    },
    renameSessionLocal(state, action: PayloadAction<{ sessionID: string; title: string }>) {
      const session = state.items.find((item) => item.session_id === action.payload.sessionID);
      if (session) session.title = action.payload.title;
    },
    setSessionFavorite(state, action: PayloadAction<{ sessionID: string; favorite: boolean }>) {
      const session = state.items.find((item) => item.session_id === action.payload.sessionID);
      if (session) session.favorite = action.payload.favorite;
    },
    removeSession(state, action: PayloadAction<string>) {
      state.items = state.items.filter((item) => item.session_id !== action.payload);
    },
    setSessionsLoading(state, action: PayloadAction<boolean>) {
      state.loading = action.payload;
    },
    setSessionSearch(state, action: PayloadAction<string>) {
      state.search = action.payload;
    },
  },
});

const appSlice = createSlice({
  name: "app",
  initialState: {
    version: undefined as VersionInfo | undefined,
    agents: [] as AgentInfo[],
    activePanel: panelFromPath(location.pathname),
    sidebarOpen: true,
    authExpired: false,
    toast: undefined as string | undefined,
  },
  reducers: {
    setVersion(state, action: PayloadAction<VersionInfo | undefined>) {
      state.version = action.payload;
    },
    setAgents(state, action: PayloadAction<AgentInfo[]>) {
      state.agents = action.payload;
    },
    setActivePanel(state, action: PayloadAction<AppPanel>) {
      state.activePanel = action.payload;
    },
    setSidebarOpen(state, action: PayloadAction<boolean>) {
      state.sidebarOpen = action.payload;
    },
    setAuthExpired(state, action: PayloadAction<boolean>) {
      state.authExpired = action.payload;
    },
    showToast(state, action: PayloadAction<string | undefined>) {
      state.toast = action.payload;
    },
  },
});

const configSlice = createSlice({
  name: "config",
  initialState: {
    value: undefined as AppConfig | undefined,
    tools: [] as ToolInfo[],
  },
  reducers: {
    setConfig(state, action: PayloadAction<AppConfig | undefined>) {
      state.value = action.payload;
    },
    setTools(state, action: PayloadAction<ToolInfo[]>) {
      state.tools = action.payload;
    },
  },
});

const filesSlice = createSlice({
  name: "files",
  initialState: {
    path: "",
    entries: [] as FileEntry[],
  },
  reducers: {
    setPath(state, action: PayloadAction<string>) {
      state.path = action.payload;
    },
    setFiles(state, action: PayloadAction<FileEntry[]>) {
      state.entries = action.payload;
    },
  },
});

const schedulesSlice = createSlice({
  name: "schedules",
  initialState: {
    items: [] as ScheduleTask[],
    filter: "",
  },
  reducers: {
    setSchedules(state, action: PayloadAction<ScheduleTask[]>) {
      state.items = action.payload;
    },
    setScheduleFilter(state, action: PayloadAction<string>) {
      state.filter = action.payload;
    },
  },
});

const skillsSlice = createSlice({
  name: "skills",
  initialState: {
    local: [] as SkillInfo[],
    results: [] as SkillInfo[],
  },
  reducers: {
    setLocalSkills(state, action: PayloadAction<SkillInfo[]>) {
      state.local = action.payload;
    },
    setSkillResults(state, action: PayloadAction<SkillInfo[]>) {
      state.results = action.payload;
    },
  },
});

export const chatActions = chatSlice.actions;
export const sessionsActions = sessionsSlice.actions;
export const appActions = appSlice.actions;
export const configActions = configSlice.actions;
export const filesActions = filesSlice.actions;
export const schedulesActions = schedulesSlice.actions;
export const skillsActions = skillsSlice.actions;

export const store = configureStore({
  reducer: {
    app: appSlice.reducer,
    chat: chatSlice.reducer,
    sessions: sessionsSlice.reducer,
    config: configSlice.reducer,
    files: filesSlice.reducer,
    schedules: schedulesSlice.reducer,
    skills: skillsSlice.reducer,
  },
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;
