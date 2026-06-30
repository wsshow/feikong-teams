export type ChatEventType =
  | "agent_started"
  | "agent_completed"
  | "turn_started"
  | "turn_completed"
  | "user_message"
  | "assistant_started"
  | "assistant_reasoning_delta"
  | "assistant_text_delta"
  | "assistant_completed"
  | "tool_call_started"
  | "tool_call_arguments_delta"
  | "tool_call_result_delta"
  | "tool_call_completed"
  | "tool_call_failed"
  | "ask_requested"
  | "ask_answered"
  | "approval_requested"
  | "approval_answered"
  | "system_notice"
  | "usage_reported"
  | "error"
  | "processing_start"
  | "processing_end"
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
  text?: string;
  display_text?: string;
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
  display_order?: number;
  created_at?: string;
  agent_name?: string;
  role?: string;
  delta_kind?: string;
  content?: string;
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
