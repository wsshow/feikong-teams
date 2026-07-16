import type { ChatEvent, ContentPartDTO, QueueItem } from "./events";

export interface SessionSummary {
  session_id: string;
  title: string;
  status?: string;
  favorite?: boolean;
  active_task?: boolean;
  mod_time?: string;
  updated_at?: string;
  current_agent?: string;
}

export interface SessionDetail {
  session_id: string;
  title?: string;
  status?: string;
  favorite?: boolean;
  active_task?: boolean;
  events?: ChatEvent[];
  queue?: QueueItem[];
  message_count?: number;
  allow_tool_details?: boolean;
}

export interface ChatViewMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  agent?: string;
  content: string;
  contentParts?: ContentPartDTO[];
  reasoningContent?: string;
  createdAt?: string;
  events: ChatEvent[];
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

export interface ChatState {
  activeSessionID: string;
  viewSessionID: string;
  runningSessionID: string;
  streamInitialOffset?: number;
  currentAgent: string;
  mode: string;
  messages: ChatViewMessage[];
  events: ChatEvent[];
  queue: QueueItem[];
  isProcessing: boolean;
  connectionState: "disconnected" | "connecting" | "connected";
  error?: string;
  errorTitle?: string;
  errorSuggestions?: string[];
  technicalError?: string;
  statusText?: string;
}
