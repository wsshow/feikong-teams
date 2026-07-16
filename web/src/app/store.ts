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
import { appendBufferedChatEvent, rememberChatEvent } from "@/features/chat/eventBuffer";

export type AppPanel = "chat" | "config" | "files" | "schedules" | "shares" | "skills";

const initialChatState: ChatState = {
  activeSessionID: chatSessionIDFromPath(location.pathname),
  viewSessionID: "",
  runningTasks: {},
  currentAgent: "",
  mode: "team",
  messages: [],
  events: [],
  seenEventKeys: {},
  seenEventKeyOrder: [],
  queue: [],
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
    setTaskConnectionState(state, action: PayloadAction<{ sessionID: string; connectionState: "disconnected" | "connecting" | "connected" }>) {
      const task = state.runningTasks[action.payload.sessionID];
      if (task) task.connectionState = action.payload.connectionState;
    },
    beginRunningSession(state, action: PayloadAction<{ sessionID: string; startedAt: number }>) {
      const { sessionID, startedAt } = action.payload;
      state.activeSessionID = sessionID;
      state.runningTasks[sessionID] = {
        phase: "starting",
        startedAt,
        connectionState: "disconnected",
      };
      localStorage.setItem(storageKeys.sessionID, sessionID);
    },
    activateRunningSession(state, action: PayloadAction<{ sessionID: string; initialOffset?: number; startedAt?: number }>) {
      const { sessionID, initialOffset, startedAt } = action.payload;
      const current = state.runningTasks[sessionID];
      state.activeSessionID = sessionID;
      state.runningTasks[sessionID] = {
        phase: "processing",
        initialOffset,
        startedAt: current?.startedAt ?? startedAt ?? 0,
        connectionState: current?.connectionState ?? "disconnected",
      };
      if (initialOffset === 0 && state.viewSessionID === sessionID) state.seenStreamEventID = undefined;
      localStorage.setItem(storageKeys.sessionID, sessionID);
    },
    syncRunningSessions(state, action: PayloadAction<{ sessionIDs: string[]; requestStartedAt: number }>) {
      const active = new Set(action.payload.sessionIDs);
      for (const sessionID of action.payload.sessionIDs) {
        if (state.runningTasks[sessionID]) continue;
        state.runningTasks[sessionID] = {
          phase: "processing",
          startedAt: 0,
          connectionState: "disconnected",
        };
      }
      for (const [sessionID, task] of Object.entries(state.runningTasks)) {
        if (active.has(sessionID) || task.phase === "starting" || task.startedAt >= action.payload.requestStartedAt) continue;
        delete state.runningTasks[sessionID];
      }
    },
    consumeStreamInitialOffset(state, action: PayloadAction<string>) {
      const task = state.runningTasks[action.payload];
      if (task) task.initialOffset = undefined;
    },
    finishRunningSession(state, action: PayloadAction<string>) {
      delete state.runningTasks[action.payload];
    },
    clearMessages(state) {
      state.viewSessionID = "";
      state.messages = [];
      state.events = [];
      state.seenEventKeys = {};
      state.seenEventKeyOrder = [];
      state.seenStreamEventID = undefined;
      state.queue = [];
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
      state.seenEventKeys = {};
      state.seenEventKeyOrder = [];
      state.seenStreamEventID = undefined;
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
        const current = state.runningTasks[detail.session_id];
        state.runningTasks[detail.session_id] = {
          phase: "processing",
          startedAt: current?.startedAt ?? 0,
          connectionState: current?.connectionState ?? "disconnected",
        };
      } else if (state.runningTasks[detail.session_id]?.phase !== "starting") {
        delete state.runningTasks[detail.session_id];
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
      rememberChatEvent(state, event);
      state.events.push({ ...event });
      state.messages.push({
        id: action.payload.id,
        role: "user",
        content: action.payload.content,
        contentParts: action.payload.contentParts,
        createdAt: action.payload.createdAt,
        events: [{ ...event }],
      });
    },
    receiveEvent(state, action: PayloadAction<ChatEvent>) {
      applyChatEvent(state, action.payload);
    },
    receiveEvents(state, action: PayloadAction<ChatEvent[]>) {
      for (const event of action.payload) applyChatEvent(state, event);
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
    applyTaskLifecycleEvent(state, event);
    return;
  }
  if (!rememberChatEvent(state, event)) return;
  applyTaskLifecycleEvent(state, event);
  appendBufferedChatEvent(state.events, event, assistantMessageKey(event));
  if (event.type === "queue_updated" && Array.isArray(event.queue)) {
    state.queue = event.queue;
  }
  if (event.type === "processing_start") {
    state.statusText = String(event.message || event.content || "处理中");
  }
  if (isModelResponseEvent(event)) {
    state.statusText = undefined;
  }
  if (event.type === "user_message") {
    const content = eventText(event);
    const contentParts = Array.isArray(event.content_parts) ? event.content_parts : [];
    if (!content && contentParts.length === 0) return;
    const mergeTarget = content
      ? findMergeableLocalUserMessage(state.messages, content)
      : findMergeableLocalUserAttachmentMessage(state.messages, contentParts);
    if (mergeTarget) {
      mergeTarget.createdAt = mergeTarget.createdAt || event.created_at;
      mergeTarget.contentParts = mergeTarget.contentParts?.length ? mergeTarget.contentParts : contentParts;
      mergeTarget.events.push({ ...event });
      return;
    }
    state.messages.push({
      id: `user-${eventIdentityKey(event)}`,
      role: "user",
      content,
      contentParts,
      createdAt: event.created_at,
      events: [{ ...event }],
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
    appendBufferedChatEvent(message.events, event, assistantMessageKey(event));
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
    appendBufferedChatEvent(message.events, event, key);
  }
  if (event.type === "system_notice") {
    state.statusText = eventText(event) || state.statusText;
  }
  if (event.type === "error") {
    state.error = friendlyEventMessage(event);
    state.errorTitle = event.error_title;
    state.errorSuggestions = Array.isArray(event.error_suggestions) ? event.error_suggestions : undefined;
    state.technicalError = event.technical_error || event.error || event.content || event.message;
    state.statusText = undefined;
  }
  if (event.type === "cancelled" || event.type === "processing_end") {
    state.statusText = String(event.message || event.content || "");
  }
  if (event.type === "cancelled") {
    const content = eventText(event) || "任务已取消";
    state.messages.push({
      id: `cancelled-${event.event_id ?? event.sequence ?? Date.now()}`,
      role: "system",
      content,
      createdAt: event.created_at,
      events: [{ ...event }],
    });
  }
}

function applyTaskLifecycleEvent(state: ChatState, event: ChatEvent) {
  const eventSessionID = event.session_id || state.activeSessionID;
  if (event.type === "processing_start" && eventSessionID) {
    const current = state.runningTasks[eventSessionID];
    state.runningTasks[eventSessionID] = {
      phase: "processing",
      startedAt: current?.startedAt ?? 0,
      connectionState: current?.connectionState ?? "connected",
    };
  }
  if ((event.type === "processing_end" || event.type === "cancelled" || event.type === "error") && eventSessionID) {
    delete state.runningTasks[eventSessionID];
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
    localPatches: {} as Record<string, { observedAt: number; values: Partial<SessionSummary> }>,
  },
  reducers: {
    setSessions(state, action: PayloadAction<{ items: SessionSummary[]; requestStartedAt: number }>) {
      const currentSessions = new Map(state.items.map((session) => [session.session_id, session]));
      const received = new Set(action.payload.items.map((session) => session.session_id));
      const hasNewerPatch = (sessionID: string) => {
        const patch = state.localPatches[sessionID];
        return Boolean(patch && patch.observedAt >= action.payload.requestStartedAt);
      };
      const missingLocalSessions = state.items.filter(
        (session) => !received.has(session.session_id) && hasNewerPatch(session.session_id),
      );
      state.items = [
        ...missingLocalSessions,
        ...action.payload.items.map((session) => {
          const current = currentSessions.get(session.session_id);
          const patch = state.localPatches[session.session_id];
          if (!current || !hasNewerPatch(session.session_id) || !patch) return session;
          return { ...session, ...patch.values };
        }),
      ];
      for (const [sessionID, patch] of Object.entries(state.localPatches)) {
        if (patch.observedAt < action.payload.requestStartedAt) delete state.localPatches[sessionID];
      }
      state.loading = false;
    },
    upsertSession(state, action: PayloadAction<SessionSummary>) {
      const next = action.payload;
      state.localPatches[next.session_id] = {
        observedAt: Date.now(),
        values: { ...state.localPatches[next.session_id]?.values, ...next },
      };
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
      state.localPatches[sessionID] = {
        observedAt: Date.now(),
        values: {
          ...state.localPatches[sessionID]?.values,
          ...(status ? { status } : {}),
          active_task: activeTask,
          ...(updatedAt ? { mod_time: updatedAt, updated_at: updatedAt } : {}),
        },
      };
    },
    renameSessionLocal(state, action: PayloadAction<{ sessionID: string; title: string }>) {
      const session = state.items.find((item) => item.session_id === action.payload.sessionID);
      if (!session) return;
      session.title = action.payload.title;
      state.localPatches[action.payload.sessionID] = {
        observedAt: Date.now(),
        values: { ...state.localPatches[action.payload.sessionID]?.values, title: action.payload.title },
      };
    },
    setSessionFavorite(state, action: PayloadAction<{ sessionID: string; favorite: boolean }>) {
      const session = state.items.find((item) => item.session_id === action.payload.sessionID);
      if (!session) return;
      session.favorite = action.payload.favorite;
      state.localPatches[action.payload.sessionID] = {
        observedAt: Date.now(),
        values: { ...state.localPatches[action.payload.sessionID]?.values, favorite: action.payload.favorite },
      };
    },
    removeSession(state, action: PayloadAction<string>) {
      state.items = state.items.filter((item) => item.session_id !== action.payload);
      delete state.localPatches[action.payload];
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
