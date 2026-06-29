import { configureStore, createSlice, type PayloadAction } from "@reduxjs/toolkit";
import type { AgentInfo, VersionInfo } from "@/types/api";
import type { ChatEvent, QueueItem } from "@/types/events";
import type { ChatState, ChatViewMessage, SessionSummary } from "@/types/chat";
import type { AppConfig, ToolInfo } from "@/types/config";
import type { FileEntry } from "@/types/files";
import type { ScheduleTask } from "@/types/schedules";
import type { SkillInfo } from "@/types/skills";
import { storageKeys } from "@/lib/storage";
import { chatSessionIDFromPath, panelFromPath } from "@/lib/navigation";

export type AppPanel = "chat" | "config" | "files" | "schedules" | "shares" | "skills";

const initialChatState: ChatState = {
  activeSessionID: chatSessionIDFromPath(location.pathname),
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
      if (!action.payload) state.runningSessionID = "";
    },
    setRunningSession(state, action: PayloadAction<string>) {
      state.runningSessionID = action.payload;
      state.isProcessing = Boolean(action.payload);
    },
    setMessages(state, action: PayloadAction<ChatViewMessage[]>) {
      state.messages = action.payload;
      state.events = [];
    },
    clearMessages(state) {
      state.messages = [];
      state.events = [];
      state.queue = [];
      state.isProcessing = false;
      state.runningSessionID = "";
      state.error = undefined;
      state.statusText = undefined;
    },
    appendUserMessage(state, action: PayloadAction<{ id: string; content: string; createdAt?: string }>) {
      state.messages.push({
        id: action.payload.id,
        role: "user",
        content: action.payload.content,
        createdAt: action.payload.createdAt,
        events: [],
      });
    },
    receiveEvent(state, action: PayloadAction<ChatEvent>) {
      const event = action.payload;
      state.events.push(event);
      if (event.session_id) {
        state.runningSessionID = event.session_id;
      }
      if (event.type === "queue_updated" && Array.isArray(event.queue)) {
        state.queue = event.queue;
      }
      if (event.type === "processing_start") {
        state.isProcessing = true;
        state.statusText = String(event.message || event.content || "处理中");
      }
      if (event.type === "user_message") {
        const content = eventText(event);
        const exists = state.messages.some((item) => item.role === "user" && item.content === content);
        if (content && !exists) {
          state.messages.push({
            id: `user-${event.stream_event_id ?? Date.now()}`,
            role: "user",
            content,
            createdAt: event.created_at,
            events: [event],
          });
        }
      }
      if (isMemberActivityEvent(event)) {
        const key = event.message_id || event.member_call_id || event.member_name || event.agent_name || "member";
        const id = event.message_id || `member-${key}`;
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
        message.events.push(event);
      }
      if (event.type === "system_notice") {
        state.statusText = eventText(event) || state.statusText;
      }
      if (event.type === "error") {
        state.error = String(event.error || event.content || event.message || "request failed");
        state.isProcessing = false;
        state.runningSessionID = "";
        state.statusText = undefined;
      }
      if (event.type === "cancelled" || event.type === "processing_end") {
        state.isProcessing = false;
        state.runningSessionID = "";
        state.statusText = String(event.message || event.content || "");
      }
      if (event.type === "cancelled") {
        const content = eventText(event) || "任务已取消";
        const exists = state.messages.some((message) => message.role === "system" && message.events.some((item) => sameEventIdentity(item, event)));
        if (!exists) {
          state.messages.push({
            id: `cancelled-${event.stream_event_id ?? event.sequence ?? Date.now()}`,
            role: "system",
            content,
            createdAt: event.created_at,
            events: [event],
          });
        }
      }
    },
    setQueue(state, action: PayloadAction<QueueItem[]>) {
      state.queue = action.payload;
    },
    setError(state, action: PayloadAction<string | undefined>) {
      state.error = action.payload;
    },
  },
});

function eventText(event: ChatEvent) {
  return String(event.content || event.message || "");
}

function sameEventIdentity(left: ChatEvent, right: ChatEvent) {
  if (left.event_id && right.event_id) return left.event_id === right.event_id;
  if (left.run_id && right.run_id && left.sequence && right.sequence) return left.run_id === right.run_id && left.sequence === right.sequence;
  return false;
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
  return event.stream_id || event.agent_name || "assistant";
}

function isOutputDelta(event: ChatEvent) {
  return event.delta_kind === "output" || event.delta_kind === "" || event.delta_kind === undefined;
}

function isAssistantTextDelta(event: ChatEvent) {
  return event.type === "assistant_text_delta" && isOutputDelta(event);
}

function hasToolActivity(event: ChatEvent) {
  return Boolean(event.tool_calls?.length || event.tool_call || event.tool_name || event.tool_call_ref || event.tool_call_id);
}

function isMemberActivityEvent(event: ChatEvent) {
  return Boolean(event.is_member_event || event.member_call_id || event.member_name || event.member_tool_name || event.parent_tool_call_id);
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

const sessionsSlice = createSlice({
  name: "sessions",
  initialState: {
    items: [] as SessionSummary[],
    loading: false,
    search: "",
  },
  reducers: {
    setSessions(state, action: PayloadAction<SessionSummary[]>) {
      state.items = action.payload;
      state.loading = false;
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
