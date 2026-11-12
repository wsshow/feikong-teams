import { z } from "zod";
import type { ChatViewMessage } from "./chat";
import type { ToolCallDTO } from "./events";

export interface AskActivity {
  active: boolean;
  run_id?: string;
  id: string;
  sessionID?: string;
  messageID?: string;
  question: string;
  options: string[];
  multiSelect: boolean;
  selected: string[];
  freeText: string;
  answered: boolean;
  memberID?: string;
  memberName?: string;
}
export interface ApprovalActivity {
  active: boolean;
  run_id?: string;
  id: string;
  order: number;
  message: string;
  answered: boolean;
  decision?: string;
}
export interface MemberActivity {
  id: string;
  name: string;
  completed: boolean;
  toolCount: number;
  parts: MessageRenderPart[];
}
export type MessageRenderPart =
  | { type: "reasoning"; content: string; key?: string; streaming?: boolean }
  | { type: "text"; content: string; key?: string; streaming?: boolean }
  | { type: "ask"; key?: string; ask: AskActivity }
  | {
      type: "tool";
      key?: string;
      tool: ToolCallDTO & { message_id?: string; order?: number };
      member?: MemberActivity;
    };
export interface TimelineMessageNode {
  message: ChatViewMessage;
  parts: MessageRenderPart[];
  showAgentLabel: boolean;
  showCopyAction: boolean;
  order: number;
}
export type TimelineItem = { id: string; index: number; order: number } & (
  | { kind: "message"; node: TimelineMessageNode }
  | { kind: "member"; member: MemberActivity }
  | { kind: "ask"; ask: AskActivity }
  | { kind: "approval"; approval: ApprovalActivity }
  | {
      kind: "error";
      error: {
        id: string;
        order: number;
        title?: string;
        message: string;
        suggestions?: string[];
        technicalDetail?: string;
      };
    }
);
export interface PresentationSnapshot {
  items: TimelineItem[];
}
export interface PresentationUpdate {
  reset?: PresentationSnapshot;
  remove?: string[];
  items?: TimelineItem[];
  appends?: {
    item_id: string;
    key: string;
    content: string;
    message_id?: string;
    type?: "text" | "reasoning";
  }[];
}

const text = z.string();
const count = z.number().int().nonnegative();
const tool = z.object({
  id: text,
  ref: text,
  name: text,
  status: z.enum([
    "pending",
    "running",
    "completed",
    "error",
    "cancelled",
    "interrupted",
  ]),
  message_id: text,
  display_name: text.optional(),
  kind: text.optional(),
  target: text.optional(),
  arguments: text.optional(),
  result: text.optional(),
  member_name: text.optional(),
});
const ask = z.object({
  active: z.boolean(),
  run_id: text.optional(),
  id: text,
  sessionID: text.optional(),
  messageID: text.optional(),
  question: text,
  options: z.array(text),
  multiSelect: z.boolean(),
  selected: z.array(text),
  freeText: text,
  answered: z.boolean(),
  memberID: text.optional(),
});
const approval = z.object({
  active: z.boolean(),
  run_id: text.optional(),
  id: text,
  order: count,
  message: text,
  answered: z.boolean(),
  decision: text.optional(),
});
const error = z.object({
  id: text,
  order: count,
  message: text,
  title: text.optional(),
  suggestions: z.array(text).optional(),
  technicalDetail: text.optional(),
});
const member: z.ZodTypeAny = z.lazy(() =>
  z.object({
    id: text,
    name: text,
    completed: z.boolean(),
    toolCount: count,
    parts: z.array(part),
  }),
);
const part: z.ZodTypeAny = z.lazy(() =>
  z.discriminatedUnion("type", [
    z.object({
      type: z.literal("text"),
      key: text,
      content: text,
      streaming: z.boolean().optional(),
    }),
    z.object({
      type: z.literal("reasoning"),
      key: text,
      content: text,
      streaming: z.boolean().optional(),
    }),
    z.object({
      type: z.literal("tool"),
      key: text,
      tool,
      member: member.optional(),
    }),
    z.object({ type: z.literal("ask"), key: text, ask }),
  ]),
);
const attachment = z.object({
  type: z.enum([
    "text",
    "image_url",
    "image_base64",
    "audio_url",
    "video_url",
    "file_url",
  ]),
  name: text.optional(),
  text: text.optional(),
  url: text.optional(),
  base64_data: text.optional(),
  file_id: text.optional(),
  mime_type: text.optional(),
  detail: text.optional(),
});
const message = z.object({
  id: text,
  role: z.enum(["user", "assistant", "system", "tool"]),
  agent: text.optional(),
  content: text,
  contentParts: z.array(attachment).optional(),
  createdAt: text,
  run_id: text.optional(),
});
const base = { id: text, index: count, order: count };
const item = z.discriminatedUnion("kind", [
  z.object({
    ...base,
    kind: z.literal("message"),
    node: z.object({
      message,
      parts: z.array(part),
      showAgentLabel: z.boolean(),
      showCopyAction: z.boolean(),
      order: count,
    }),
  }),
  z.object({ ...base, kind: z.literal("member"), member }),
  z.object({ ...base, kind: z.literal("ask"), ask }),
  z.object({ ...base, kind: z.literal("approval"), approval }),
  z.object({ ...base, kind: z.literal("error"), error }),
]);
const snapshotSchema = z.object({ items: z.array(item) });
const updateSchema = z.object({
  reset: snapshotSchema.optional(),
  remove: z.array(text).optional(),
  items: z.array(item).optional(),
  appends: z
    .array(
      z.object({
        item_id: text,
        key: text,
        content: text,
        message_id: text.optional(),
        type: z.enum(["text", "reasoning"]).optional(),
      }),
    )
    .optional(),
});
export function decodePresentationSnapshot(
  value: unknown,
): PresentationSnapshot {
  return snapshotSchema.parse(value) as PresentationSnapshot;
}
export function isPresentationUpdate(
  value: unknown,
): value is PresentationUpdate {
  return updateSchema.safeParse(value).success;
}

// 更新操作只按服务端给出的身份和位置执行，不推导消息归属、状态或展示顺序。
export function applyPresentationUpdate(
  snapshot: PresentationSnapshot,
  update: PresentationUpdate,
) {
  if (update.reset) snapshot.items = structuredClone(update.reset.items);
  if (update.remove?.length) {
    const removed = new Set(update.remove);
    snapshot.items = snapshot.items.filter((item) => !removed.has(item.id));
  }
  for (const item of update.items || []) {
    const previous = snapshot.items.findIndex(
      (candidate) => candidate.id === item.id,
    );
    if (previous >= 0) snapshot.items.splice(previous, 1);
    snapshot.items.splice(item.index, 0, structuredClone(item));
  }
  if (update.remove?.length || update.items?.length) {
    snapshot.items.forEach((item, index) => {
      item.index = index;
    });
  }
  for (const append of update.appends || []) {
    const item = snapshot.items.find(
      (candidate) => candidate.id === append.item_id,
    );
    if (!item) throw new Error("presentation update requires a fresh snapshot");
    const parts =
      item.kind === "message"
        ? item.node.parts
        : item.kind === "member"
          ? item.member.parts
          : [];
    if (!appendPart(parts, append.key, append.content))
      throw new Error("presentation block requires a fresh snapshot");
    if (
      append.type === "text" &&
      item.kind === "message" &&
      item.node.message.id === append.message_id
    )
      item.node.message.content += append.content;
  }
}
function appendPart(
  parts: MessageRenderPart[],
  key: string,
  content: string,
): boolean {
  for (const part of parts) {
    if (
      (part.type === "text" || part.type === "reasoning") &&
      part.key === key
    ) {
      part.content += content;
      return true;
    }
    if (
      part.type === "tool" &&
      part.member &&
      appendPart(part.member.parts, key, content)
    )
      return true;
  }
  return false;
}
