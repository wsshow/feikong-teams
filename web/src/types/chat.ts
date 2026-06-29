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
  sequence?: number;
  content?: string;
  detail?: string;
  tool_call?: ToolCallDTO;
  ask?: {
    id?: string;
    question?: string;
    options?: string[];
    multi_select?: boolean;
    selected?: string[];
    free_text?: string;
    answered?: boolean;
  };
}

export interface AgentMessage {
  agent_name?: string;
  run_path?: string;
  member_call_id?: string;
  member_tool_name?: string;
  member_name?: string;
  role?: string;
  content?: string;
  start_time?: string;
  end_time?: string;
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
