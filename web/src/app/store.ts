import { configureStore, createSlice, type PayloadAction } from "@reduxjs/toolkit";
import type { AgentInfo, VersionInfo } from "@/types/api";
import type { ChatEvent, QueueItem } from "@/types/events";
import type { ChatState, ChatViewMessage, SessionSummary } from "@/types/chat";
import type { AppConfig, ToolInfo } from "@/types/config";
import type { FileEntry } from "@/types/files";
import type { ScheduleTask } from "@/types/schedules";
import type { SkillInfo } from "@/types/skills";
import { storageKeys } from "@/lib/storage";

export type AppPanel = "chat" | "config" | "files" | "schedules" | "skills";

function panelFromPath(path: string): AppPanel {
  switch (path) {
    case "/config":
      return "config";
    case "/files":
      return "files";
    case "/schedules":
      return "schedules";
    case "/skills":
      return "skills";
    default:
      return "chat";
  }
}

const initialChatState: ChatState = {
  activeSessionID: localStorage.getItem(storageKeys.sessionID) || "",
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
      if (isRenderableMessageEvent(event)) {
        const key = `${event.message_id || event.stream_id || event.agent_name || "assistant"}`;
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
        if (event.type === "message_delta" && content) {
          message.content += content;
        }
        message.events.push(event);
      }
      if (event.type === "action") {
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
  return String(event.content || event.delta || event.message || "");
}

function isRenderableMessageEvent(event: ChatEvent) {
  if (event.type === "message_start") {
    return event.role !== "tool";
  }
  if (event.type !== "message_delta") {
    return false;
  }
  if (event.role === "tool") {
    return false;
  }
  return event.delta_kind !== "reasoning" && event.delta_kind !== "tool_args" && event.delta_kind !== "tool_result";
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
