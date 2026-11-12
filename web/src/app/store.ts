import { configureStore, createSlice, type PayloadAction } from "@reduxjs/toolkit";
import type { AgentInfo, VersionInfo } from "@/types/api";
import type { ChatEvent, ContentPartDTO, QueueItem } from "@/types/events";
import type { ChatState, ChatViewMessage, SessionDetail, SessionSummary } from "@/types/chat";
import type { AppConfig, ToolInfo } from "@/types/config";
import type { FileEntry } from "@/types/files";
import type { ScheduleTask } from "@/types/schedules";
import type { SkillInfo } from "@/types/skills";
import { chatSessionIDFromPath, panelFromPath } from "@/lib/navigation";
import { assistantSessionID } from "@/lib/assistant";
import { rememberChatEvent } from "@/features/chat/eventBuffer";
import { applyPresentationUpdate, type PresentationSnapshot } from "@/types/presentation";

export type AppPanel = "chat" | "config" | "files" | "schedules" | "shares" | "skills" | "memory";

const initialChatState: ChatState = {
  activeSessionID: chatSessionIDFromPath(location.pathname),
  viewSessionID: "",
  runningTasks: {},
  currentAgent: "",
  mode: "team",
  presentation: { items: [] },
  messages: [],
  seenEventKeys: {},
  seenEventKeyOrder: [],
  queue: [],
};

const chatSlice = createSlice({
  name: "chat",
  initialState: initialChatState,
  reducers: {
    setActiveSession(state, action: PayloadAction<string>) {
      if (state.activeSessionID === action.payload) return;
      if (state.activeSessionID === assistantSessionID && action.payload !== assistantSessionID) {
        state.currentAgent = "";
        state.mode = "team";
      }
      state.activeSessionID = action.payload;
      resetConversationView(state);
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
    },
    activateRunningSession(state, action: PayloadAction<{ sessionID: string; initialOffset?: number; startedAt?: number; activate?: boolean }>) {
      const { sessionID, initialOffset, startedAt } = action.payload;
      const current = state.runningTasks[sessionID];
      if (action.payload.activate !== false) state.activeSessionID = sessionID;
      state.runningTasks[sessionID] = {
        phase: "processing",
        initialOffset,
        startedAt: current?.startedAt ?? startedAt ?? 0,
        connectionState: current?.connectionState ?? "disconnected",
      };
      if (initialOffset === 0 && state.viewSessionID === sessionID) state.seenStreamEventID = undefined;
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
      resetConversationView(state);
    },
    setSessionDetail(state, action: PayloadAction<SessionDetail>) {
      const detail = action.payload;
      if (state.activeSessionID !== detail.session_id) return;
      resetConversationView(state);
      state.viewSessionID = detail.session_id;
      state.mode = detail.mode || "team";
      state.currentAgent = detail.current_agent || "";
      state.presentation = detail.view;
      syncPresentationMessages(state);
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
    setPresentationSnapshot(state, action: PayloadAction<{ sessionID: string; view: PresentationSnapshot; nextOffset: number }>) {
      if (state.activeSessionID !== action.payload.sessionID) return;
      state.presentation = action.payload.view;
      state.seenStreamEventID = action.payload.nextOffset > 0 ? action.payload.nextOffset - 1 : undefined;
      syncPresentationMessages(state);
    },
    confirmUserMessage(state, action: PayloadAction<{ id: string; serverID: string }>) {
      const message = state.messages.find(item => item.id === action.payload.id);
      if (message) message.id = action.payload.serverID;
    },
    appendUserMessage(state, action: PayloadAction<{ id: string; content: string; sessionID?: string; contentParts?: ContentPartDTO[]; createdAt?: string }>) {
      const sessionID = action.payload.sessionID || state.activeSessionID;
      if (state.viewSessionID !== sessionID) resetConversationView(state);
      state.viewSessionID = sessionID;
      state.messages.push({
        id: action.payload.id,
        role: "user",
        content: action.payload.content,
        contentParts: action.payload.contentParts,
        createdAt: action.payload.createdAt,
        optimistic: true,
      });
    },
    rollbackUserMessage(state, action: PayloadAction<{ id: string; sessionID: string; resetSession: boolean }>) {
      state.messages = state.messages.filter(message => message.id !== action.payload.id || !message.optimistic);
      if (action.payload.resetSession && state.activeSessionID === action.payload.sessionID) {
        state.activeSessionID = "";
        if (state.viewSessionID === action.payload.sessionID) state.viewSessionID = "";
        state.queue = [];
      }
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
    sessionLoadFailed(state, action: PayloadAction<{ sessionID: string; message: string }>) {
      if (state.activeSessionID !== action.payload.sessionID || state.viewSessionID === action.payload.sessionID) return;
      state.error = action.payload.message;
    },
  },
});

// 会话切换在一次状态更新内撤下旧视图，后台任务继续由 runningTasks 独立管理。
function resetConversationView(state: ChatState) {
  state.viewSessionID = "";
  state.presentation = { items: [] };
  state.messages = [];
  state.seenEventKeys = {};
  state.seenEventKeyOrder = [];
  state.seenStreamEventID = undefined;
  state.queue = [];
  state.error = undefined;
  state.errorTitle = undefined;
  state.errorSuggestions = undefined;
  state.technicalError = undefined;
  state.statusText = undefined;
}
function applyChatEvent(
  state: ChatState,
  payload: ChatEvent,
) {
  const event = { ...payload };
  if (!state.activeSessionID || state.viewSessionID !== state.activeSessionID || (event.session_id && event.session_id !== state.activeSessionID)) {
    applyTaskLifecycleEvent(state, event);
    return;
  }
  if (!rememberChatEvent(state, event)) return;
  applyTaskLifecycleEvent(state, event);
  if (event.view_update) {
    applyPresentationUpdate(state.presentation, event.view_update);
    syncPresentationMessages(state);
  }
  if (event.type === "queue_updated" && Array.isArray(event.queue)) {
    state.queue = event.queue;
  }
  if (event.type === "processing_start") {
    state.statusText = String(event.message || event.content || "处理中");
  }
  if (event.view_update) {
    state.statusText = undefined;
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
  if (event.type === "processing_end" && eventSessionID) {
    delete state.runningTasks[eventSessionID];
  }
}

function eventText(event: ChatEvent) {
  return String(event.content || event.message || "");
}

function friendlyEventMessage(event: ChatEvent) {
  return String(event.display_error || event.error_title || event.error || event.content || event.message || "请求失败");
}

function syncPresentationMessages(state: ChatState) {
  const messages = state.presentation.items.flatMap(item => item.kind === "message" ? [item.node.message] : []);
  const ids = new Set(messages.map(message => message.id));
  const optimistic = state.messages.filter(message => message.optimistic && !ids.has(message.id));
  state.messages = [...messages, ...optimistic];
}

const sessionsSlice = createSlice({
  name: "sessions",
  initialState: {
    items: [] as SessionSummary[],
    loading: false,
    search: "",
    activeRequestStartedAt: undefined as number | undefined,
    localPatches: {} as Record<string, Partial<SessionSummary>>,
  },
  reducers: {
    beginSessionsRequest(state, action: PayloadAction<number>) {
      state.loading = true;
      state.activeRequestStartedAt = action.payload;
      state.localPatches = {};
    },
    setSessions(state, action: PayloadAction<{ items: SessionSummary[]; requestStartedAt: number }>) {
      const currentSessions = new Map(state.items.map((session) => [session.session_id, session]));
      const received = new Set(action.payload.items.map((session) => session.session_id));
      if (state.activeRequestStartedAt !== action.payload.requestStartedAt) return;
      const hasLocalPatch = (sessionID: string) => Boolean(state.localPatches[sessionID]);
      const missingLocalSessions = state.items.filter(
        (session) => !received.has(session.session_id) && hasLocalPatch(session.session_id),
      );
      state.items = [
        ...missingLocalSessions,
        ...action.payload.items.map((session) => {
          const current = currentSessions.get(session.session_id);
          const patch = state.localPatches[session.session_id];
          if (!current || !patch) return session;
          return { ...session, ...patch };
        }),
      ];
      state.localPatches = {};
      state.activeRequestStartedAt = undefined;
      state.loading = false;
    },
    upsertSession(state, action: PayloadAction<SessionSummary>) {
      const next = action.payload;
      if (state.activeRequestStartedAt !== undefined) {
        state.localPatches[next.session_id] = { ...state.localPatches[next.session_id], ...next };
      }
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
    updateSessionRuntime(state, action: PayloadAction<{ sessionID: string; status?: string; activeTask: boolean; updatedAt?: string; mode?: string; currentAgent?: string }>) {
      const { sessionID, status, activeTask, updatedAt, mode, currentAgent } = action.payload;
      const session = state.items.find((item) => item.session_id === sessionID);
      if (!session) return;
      if (status) session.status = status;
      session.active_task = activeTask;
      if (mode !== undefined) session.mode = mode;
      if (currentAgent !== undefined) session.current_agent = currentAgent;
      if (updatedAt) {
        session.mod_time = updatedAt;
        session.updated_at = updatedAt;
      }
      if (state.activeRequestStartedAt !== undefined) {
        state.localPatches[sessionID] = {
          ...state.localPatches[sessionID],
          ...(status ? { status } : {}),
          active_task: activeTask,
          ...(mode !== undefined ? { mode } : {}),
          ...(currentAgent !== undefined ? { current_agent: currentAgent } : {}),
          ...(updatedAt ? { mod_time: updatedAt, updated_at: updatedAt } : {}),
        };
      }
    },
    renameSessionLocal(state, action: PayloadAction<{ sessionID: string; title: string }>) {
      const session = state.items.find((item) => item.session_id === action.payload.sessionID);
      if (!session) return;
      session.title = action.payload.title;
      if (state.activeRequestStartedAt !== undefined) {
        state.localPatches[action.payload.sessionID] = {
          ...state.localPatches[action.payload.sessionID],
          title: action.payload.title,
        };
      }
    },
    setSessionFavorite(state, action: PayloadAction<{ sessionID: string; favorite: boolean }>) {
      const session = state.items.find((item) => item.session_id === action.payload.sessionID);
      if (!session) return;
      session.favorite = action.payload.favorite;
      if (state.activeRequestStartedAt !== undefined) {
        state.localPatches[action.payload.sessionID] = {
          ...state.localPatches[action.payload.sessionID],
          favorite: action.payload.favorite,
        };
      }
    },
    removeSession(state, action: PayloadAction<string>) {
      state.items = state.items.filter((item) => item.session_id !== action.payload);
      delete state.localPatches[action.payload];
    },
    setSessionsLoading(state, action: PayloadAction<boolean>) {
      state.loading = action.payload;
      if (!action.payload) {
        state.activeRequestStartedAt = undefined;
        state.localPatches = {};
      }
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
