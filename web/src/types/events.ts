export const chatEventTypes = [
  "agent_started",
  "agent_completed",
  "turn_started",
  "turn_completed",
  "user_message",
  "assistant_started",
  "assistant_reasoning_delta",
  "assistant_text_delta",
  "assistant_completed",
  "tool_call_started",
  "tool_call_arguments_delta",
  "tool_call_result_delta",
  "tool_call_completed",
  "tool_call_failed",
  "ask_requested",
  "ask_answered",
  "approval_requested",
  "approval_answered",
  "system_notice",
  "usage_reported",
  "error",
  "processing_start",
  "processing_end",
  "queue_updated",
  "cancelled",
  "turn_failed",
  "turn_cancelled",
  "member_started",
  "member_completed",
  "connected",
  "pong",
  "invalid_api_key",
] as const;

export type ChatEventType = typeof chatEventTypes[number];

export interface ToolCallDTO {
  id?: string;
  ref?: string;
  index?: number;
  name: string;
  display_name?: string;
  kind?: "tool" | "agent" | string;
  target?: string;
  arguments?: string;
  result?: string;
  status?: "pending" | "running" | "completed" | "error" | string;
  agent_name?: string;
  member_name?: string;
  content?: string;
}

export interface QueueItem {
  queue_id: string;
  kind: "steering" | "follow_up" | string;
  text?: string;
  display_text?: string;
  content?: string;
  message?: string;
  created_at?: string;
}

export interface ContentPartDTO {
  type: "text" | "image_url" | "image_base64" | "audio_url" | "video_url" | "file_url" | string;
  name?: string;
  text?: string;
  url?: string;
  base64_data?: string;
  mime_type?: string;
  detail?: string;
}

export interface ChatEvent {
  type: ChatEventType;
  session_id?: string;
  stream_event_id?: number;
  run_id?: string;
  turn_id?: string;
  event_id?: string;
  sequence?: number;
  created_at?: string;
  agent_name?: string;
  role?: string;
  delta_kind?: string;
  content?: string;
  content_parts?: ContentPartDTO[];
  reasoning_content?: string;
  message_id?: string;
  block_id?: string;
  block_type?: string;
  stream_id?: string;
  chunk_index?: number;
  tool_name?: string;
  tool_display_name?: string;
  tool_kind?: string;
  tool_target?: string;
  tool_args?: string;
  tool_result?: string;
  tool_call_id?: string;
  tool_call_ref?: string;
  tool_call_index?: number;
  tool_calls?: ToolCallDTO[];
  tool_call?: ToolCallDTO;
  detail?: string;
  ask_id?: string;
  question?: string;
  options?: string[];
  multi_select?: boolean;
  error?: string;
  error_code?: string;
  error_title?: string;
  display_error?: string;
  technical_error?: string;
  error_suggestions?: string[];
  message?: string;
  queue?: QueueItem[];
  is_member_event?: boolean;
  member_call_id?: string;
  member_tool_name?: string;
  member_name?: string;
  member_order?: number;
  parent_tool_call_id?: string;
  parent_tool_name?: string;
  [key: string]: unknown;
}

const knownChatEventTypes = new Set<string>(chatEventTypes);

// 未识别的协议扩展停留在传输边界，不扩宽应用事件类型。
export function decodeChatEvent(value: unknown): ChatEvent | undefined {
  if (!value || typeof value !== "object") return undefined;
  const event = value as Record<string, unknown>;
  if (typeof event.type !== "string" || !knownChatEventTypes.has(event.type)) return undefined;
  for (const key of ["content", "session_id", "event_id", "message_id", "run_id", "turn_id"] as const) {
    if (event[key] !== undefined && typeof event[key] !== "string") return undefined;
  }
  return event as unknown as ChatEvent;
}

// 实时流、快照与历史回放共用事件入口。
export function decodeChatEvents(values: unknown): ChatEvent[] {
  if (!Array.isArray(values)) return [];
  return values.map(decodeChatEvent).filter((event): event is ChatEvent => event !== undefined);
}
