export type ChatEventType =
  | "message_start"
  | "message_delta"
  | "message_end"
  | "tool_start"
  | "tool_update"
  | "tool_end"
  | "action"
  | "usage"
  | "error"
  | "processing_start"
  | "processing_end"
  | "user_message"
  | "queue_updated"
  | "cancelled"
  | string;

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
  content?: string;
  message?: string;
  created_at?: string;
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
  delta?: string;
  content?: string;
  reasoning_content?: string;
  message_id?: string;
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
  action_type?: string;
  error?: string;
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
