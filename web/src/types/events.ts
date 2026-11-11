export interface ToolCallDTO {
  id?: string;
  ref?: string;
  index?: number;
  name: string;
  display_name?: string;
  kind?: string;
  target?: string;
  arguments?: string;
  result?: string;
  status?: string;
  agent_name?: string;
  member_name?: string;
  content?: string;
}

export interface QueueItem {
  queue_id: string;
  kind: "steering" | "follow_up";
  text?: string;
  display_text?: string;
  content?: string;
  message?: string;
  created_at?: string;
  updated_at?: string;
  parts?: ContentPartDTO[];
}

export interface ContentPartDTO {
  type: "text" | "image_url" | "image_base64" | "audio_url" | "video_url" | "file_url";
  name?: string;
  text?: string;
  url?: string;
  base64_data?: string;
  mime_type?: string;
  detail?: string;
}

type Validator = (value: unknown) => boolean;
const isString: Validator = (value) => typeof value === "string";
const isBoolean: Validator = (value) => typeof value === "boolean";
const isCount: Validator = (value) => typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const isStrings: Validator = (value) => Array.isArray(value) && value.every(isString);
const isQueueKind: Validator = (value) => value === "steering" || value === "follow_up";
function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
function fieldsMatch(value: Record<string, unknown>, fields: Record<string, Validator>) {
  return Object.entries(fields).every(([key, validate]) => value[key] === undefined || validate(value[key]));
}
function hasString(value: Record<string, unknown>, key: string) { return typeof value[key] === "string" && value[key] !== ""; }
function isContentPart(value: unknown): value is ContentPartDTO {
  if (!isRecord(value) || !fieldsMatch(value, {
    type: isString, name: isString, text: isString, url: isString,
    base64_data: isString, mime_type: isString, detail: isString,
  })) return false;
  switch (value.type) {
    case "text": return typeof value.text === "string";
    case "image_base64": return hasString(value, "base64_data");
    case "image_url": return hasString(value, "url") || hasString(value, "base64_data");
    case "audio_url": case "video_url": case "file_url": return hasString(value, "url");
    default: return false;
  }
}
const isContentParts: Validator = (value) => Array.isArray(value) && value.every(isContentPart);
function isToolCall(value: unknown): value is ToolCallDTO {
  return isRecord(value) && typeof value.name === "string" && fieldsMatch(value, {
    id: isString, ref: isString, index: isCount, name: isString, display_name: isString,
    kind: isString, target: isString, arguments: isString, result: isString,
    status: isString, agent_name: isString, member_name: isString, content: isString,
  });
}
function isQueueItem(value: unknown): value is QueueItem {
  return isRecord(value) && hasString(value, "queue_id") && isQueueKind(value.kind) && fieldsMatch(value, {
    text: isString, display_text: isString, content: isString, message: isString,
    created_at: isString, updated_at: isString, parts: isContentParts,
  });
}
const isQueue: Validator = (value) => Array.isArray(value) && value.every(isQueueItem);
const isUsage: Validator = (value) => isRecord(value) && fieldsMatch(value, { prompt_tokens: isCount, completion_tokens: isCount, total_tokens: isCount });
const isNotice: Validator = (value) => isRecord(value) && fieldsMatch(value, { level: isString, message: isString, code: isString });
const isApproval: Validator = (value) => isRecord(value) && fieldsMatch(value, { id: isString, message: isString, decision: isString });


// 字段声明同时驱动 TypeScript 类型和边界校验，新增协议字段只声明一次。
interface Field<T, Required extends boolean = false> { validate: Validator; required: Required; readonly valueType?: T }
function field<T>(validate: Validator): Field<T> { return { validate, required: false }; }
function required<T>(base: Field<T>): Field<T, true> { return { ...base, required: true }; }
type Fields = Record<string, Field<unknown, boolean>>;
type FieldValue<F> = F extends Field<infer T, boolean> ? T : never;
type Shape<S extends Fields> = {
  [K in keyof S as S[K]["required"] extends true ? K : never]: FieldValue<S[K]>
} & {
  [K in keyof S as S[K]["required"] extends true ? never : K]?: FieldValue<S[K]>
};
const str = field<string>(isString);
const identity = field<string>((value) => typeof value === "string" && value.length > 0);
const count = field<number>(isCount);
const flag = field<boolean>(isBoolean);
const strings = field<string[]>(isStrings);
const contentParts = field<ContentPartDTO[]>(isContentParts);
const toolCall = field<ToolCallDTO>(isToolCall);
const toolCalls = field<ToolCallDTO[]>((value) => Array.isArray(value) && value.every(isToolCall));
interface ApprovalDTO { id?: string; message?: string; decision?: string }
interface UsageDTO { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number }
interface NoticeDTO { level?: string; message?: string; code?: string }

const envelope = {
  session_id: str, stream_event_id: count, run_id: str, turn_id: str,
  event_id: str, sequence: count, created_at: str, agent_name: str,
  run_path: str, message_id: str, block_id: str, block_type: str,
  stream_id: str, chunk_index: count, is_member_event: flag,
  member_call_id: str, member_tool_name: str, member_name: str,
  member_order: count, parent_tool_call_id: str, parent_tool_name: str, transcript_index: count,
};
const content = { content: str, message: str, detail: str };
const usage = { prompt_tokens: count, completion_tokens: count, total_tokens: count, usage: field<UsageDTO>(isUsage) };
const toolMetadata = {
  tool_name: str, tool_call_ref: str, tool_display_name: str, tool_kind: str,
  tool_target: str, tool_args: str, tool_result: str, tool_call_id: str, tool_call_index: count,
  result_ref: str, summary: str, truncated: flag, original_chars: count,
};
const message = { ...content, ...usage, ...toolMetadata, role: str, delta_kind: str, content_parts: contentParts, reasoning_content: str, tool_calls: toolCalls, tool_call: toolCall };
const assistant = { ...message, message_id: required(identity) };
const delta = { ...assistant, content: required(str) };

const tool = { ...message, ...toolMetadata, tool_name: required(identity), tool_call_ref: required(identity) };
const ask = { ...content, ask_id: required(identity), question: str, options: strings, multi_select: flag, selected: strings, free_text: str };
const approval = { ...content, detail: required(identity), approval: field<ApprovalDTO>(isApproval) };
const error = { ...content, ...toolMetadata, error: required(identity), error_code: str, error_title: str, display_error: str, technical_error: str, error_suggestions: strings };
const processing = { ...content, queue_id: str, queue_kind: field<QueueItem["kind"]>(isQueueKind), queued_executing: flag };
const queue = { queue: required(field<QueueItem[]>(isQueue)), queued_count: count };
const notice = { ...content, content: required(identity), notice: field<NoticeDTO>(isNotice) };
const member = { ...message, member_call_id: required(identity) };
const turn = { ...content, run_id: required(identity), turn_id: required(identity) };

const eventSchemas = {
  agent_started: content, agent_completed: { ...error, error: str },
  turn_started: turn, turn_completed: turn,
  user_message: { ...message, content: required(str) },
  assistant_started: assistant, assistant_completed: assistant,
  assistant_reasoning_delta: delta, assistant_text_delta: delta,
  tool_call_started: tool, tool_call_arguments_delta: tool, tool_call_result_delta: tool,
  tool_call_completed: tool, tool_call_failed: { ...error, ...tool, error: str },
  ask_requested: { ...ask, question: required(identity) }, ask_answered: ask,
  approval_requested: approval, approval_answered: approval,
  system_notice: notice, usage_reported: { ...usage, total_tokens: required(count) }, error,
  processing_start: processing, processing_end: content, queue_updated: queue, cancelled: content,
  turn_failed: error, turn_cancelled: content,
  member_started: member, member_completed: member,
  connected: content, pong: content, invalid_api_key: error,
};

export type ChatEventType = keyof typeof eventSchemas;
export const chatEventTypes = Object.keys(eventSchemas) as ChatEventType[];
type Payload<K extends ChatEventType> = Shape<typeof eventSchemas[K]>;
type UnionKeys<T> = T extends unknown ? keyof T : never;
type PayloadKeys = UnionKeys<{ [K in ChatEventType]: Payload<K> }[ChatEventType]>;
// 不适用字段只能为 undefined；按 type 缩窄后，必需字段无需再次断言。
export type ChatEvent = {
  [K in ChatEventType]: Shape<typeof envelope> & { type: K } & Payload<K>
    & { [P in Exclude<PayloadKeys, keyof Payload<K> | keyof typeof envelope>]?: never }
}[ChatEventType];

const knownChatEventTypes = new Set<string>(chatEventTypes);
const allFields: Fields = Object.assign({}, envelope, ...Object.values(eventSchemas));

// 兼容字段只在入口归一化，投影与展示使用同一份标准载荷。
export function decodeChatEvent(value: unknown): ChatEvent | undefined {
  if (!isRecord(value) || typeof value.type !== "string" || !knownChatEventTypes.has(value.type)) return undefined;
  // 即使字段不适用于当前事件，也拒绝已知字段的错误类型。
  for (const [key, field] of Object.entries(allFields)) {
    if (value[key] !== undefined && !field.validate(value[key])) return undefined;
  }
  const type = value.type as ChatEventType;
  const event: { type: ChatEventType } & Record<string, unknown> = { ...value, type };
  if (type.startsWith("tool_call_")) {
    const tool = isToolCall(event.tool_call) ? event.tool_call : undefined;
    event.tool_name ||= tool?.name;
    event.tool_call_id ||= tool?.id;
    event.tool_call_ref ||= tool?.ref || (event.tool_call_id ? `tool_call:${event.tool_call_id}` : undefined);
  }
  if (type === "ask_requested" || type === "ask_answered") {
    event.ask_id ||= event.detail;
    if (type === "ask_requested") event.question ||= event.content;
    for (const [legacy, canonical, validate] of [["ask_selected", "selected", isStrings], ["ask_free_text", "free_text", isString]] as const) {
      if (value[legacy] !== undefined && !validate(value[legacy])) return undefined;
      event[canonical] ??= value[legacy];
    }
  }
  if (type === "approval_requested" || type === "approval_answered") event.detail ||= isRecord(event.approval) ? event.approval.id : undefined;
  if (type === "user_message") {
    if (!hasString(event, "content") && !(Array.isArray(event.content_parts) && event.content_parts.length)) return undefined;
    event.content ??= "";
  }
  if (type === "assistant_reasoning_delta") event.content ??= event.reasoning_content;
  if (type === "system_notice") event.content ||= event.message || (isRecord(event.notice) ? event.notice.message : undefined);
  if (type === "error" || type === "turn_failed" || type === "tool_call_failed" || type === "invalid_api_key") event.error ||= event.content || event.message;
  if (type === "usage_reported") {
    const counts = isRecord(event.usage) ? event.usage : event;
    if ([counts.prompt_tokens, counts.completion_tokens, counts.total_tokens, event.total_tokens].every((value) => value === undefined)) return undefined;
    event.total_tokens ??= counts.total_tokens ?? (Number(counts.prompt_tokens || 0) + Number(counts.completion_tokens || 0));
  }
  const decoded: { type: ChatEventType } & Record<string, unknown> = { type };
  for (const [key, field] of Object.entries({ ...envelope, ...eventSchemas[type] })) {
    if (event[key] === undefined && !field.required) continue;
    if (!field.validate(event[key])) return undefined;
    decoded[key] = event[key];
  }
  // 以上按对应 schema 校验全部必需字段，并只保留该事件声明的字段。
  return decoded as ChatEvent;
}

// 实时流、快照与历史回放共用事件入口。
export function decodeChatEvents(values: unknown): ChatEvent[] {
  if (!Array.isArray(values)) return [];
  return values.map(decodeChatEvent).filter((event): event is ChatEvent => event !== undefined);
}

// decodeQueueItems 在队列接口、会话详情和流快照的边界统一校验队列。
export function decodeQueueItems(value: unknown): QueueItem[] {
  return Array.isArray(value) ? value.filter(isQueueItem) : [];
}
