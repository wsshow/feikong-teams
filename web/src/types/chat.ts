import type { ChatEvent, QueueItem } from "./events";

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
  message_count?: number;
  allow_tool_details?: boolean;
}

export interface ChatViewMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  agent?: string;
  content: string;
  reasoningContent?: string;
  createdAt?: string;
  events: ChatEvent[];
  hidden?: boolean;
}

export interface ChatState {
  activeSessionID: string;
  runningSessionID: string;
  currentAgent: string;
  mode: string;
  messages: ChatViewMessage[];
  events: ChatEvent[];
  queue: QueueItem[];
  isProcessing: boolean;
  connectionState: "disconnected" | "connecting" | "connected";
  error?: string;
  statusText?: string;
}
