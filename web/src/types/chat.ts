import type { PresentationSnapshot } from "./presentation";
import type { ContentPartDTO, QueueItem } from "./events";

export interface SessionSummary {
  session_id: string;
  title: string;
  status?: string;
  favorite?: boolean;
  active_task?: boolean;
  mod_time?: string;
  updated_at?: string;
  mode?: string;
  current_agent?: string;
}

export interface SessionDetail {
  session_id: string;
  title?: string;
  status?: string;
  favorite?: boolean;
  active_task?: boolean;
  view: PresentationSnapshot;
  next_offset?: number;
  queue?: QueueItem[];
  message_count?: number;
  allow_tool_details?: boolean;
  mode?: string;
  current_agent?: string;
}

export interface ChatViewMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  agent?: string;
  content: string;
  contentParts?: ContentPartDTO[];
  reasoningContent?: string;
  createdAt?: string;
  optimistic?: boolean;
  hidden?: boolean;
}

export interface ChatAttachmentDraft {
  id: string;
  kind: "image" | "file";
  name: string;
  size: number;
  mimeType: string;
  status: "uploading" | "ready" | "error";
  previewURL?: string;
  base64Data?: string;
  path?: string;
  error?: string;
}

export interface RunningChatTask {
  phase: "starting" | "processing";
  initialOffset?: number;
  startedAt: number;
  connectionState: "disconnected" | "connecting" | "connected";
}

export interface ChatState {
  presentation: PresentationSnapshot;
  activeSessionID: string;
  viewSessionID: string;
  runningTasks: Record<string, RunningChatTask>;
  currentAgent: string;
  mode: string;
  messages: ChatViewMessage[];
  seenEventKeys: Record<string, true>;
  seenEventKeyOrder: string[];
  seenStreamEventID?: number;
  queue: QueueItem[];
  error?: string;
  errorTitle?: string;
  errorSuggestions?: string[];
  technicalError?: string;
  statusText?: string;
}
