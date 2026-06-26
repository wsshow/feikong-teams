import type { ChatEvent, QueueItem, ToolCallDTO } from "./events";

export interface SessionSummary {
  session_id: string;
  title: string;
  status?: string;
  favorite?: boolean;
  mod_time?: string;
  updated_at?: string;
  current_agent?: string;
}

export interface MessageEvent {
  type: string;
  content?: string;
  tool_call?: ToolCallDTO;
  action?: {
    action_type?: string;
    content?: string;
  };
}

export interface AgentMessage {
  agent_name?: string;
  role?: string;
  content?: string;
  events?: MessageEvent[];
}

export interface SessionDetail {
  session_id: string;
  title?: string;
  status?: string;
  favorite?: boolean;
  messages?: AgentMessage[];
}

export interface ChatViewMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  agent?: string;
  content: string;
  reasoningContent?: string;
  events: ChatEvent[];
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
