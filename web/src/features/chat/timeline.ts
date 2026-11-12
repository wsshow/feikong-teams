import type { ToolCallDTO } from "@/types/events";
import type { MessageRenderPart, TimelineItem } from "@/types/presentation";
export type {
  AskActivity,
  ApprovalActivity,
  MemberActivity,
  MessageRenderPart,
} from "@/types/presentation";

export function timelineItemSpacingClass(items: TimelineItem[], index: number) {
  if (index <= 0) return "";
  const previous = items[index - 1];
  const current = items[index];
  if (!previous || !current) return "chat-item-turn";
  if (current.kind === "message" && current.node.message.role === "user")
    return "chat-item-turn";
  const assistantContinuation =
    previous.kind === "message" &&
    current.kind === "message" &&
    previous.node.message.role === "assistant" &&
    current.node.message.role === "assistant" &&
    !current.node.showAgentLabel;
  if (assistantContinuation) {
    const previousPart = previous.node.parts.at(-1);
    const currentPart = current.node.parts[0];
    return previousPart?.type === "tool" && currentPart?.type === "tool"
      ? "mt-0"
      : "chat-item-continuation";
  }
  if (
    current.kind === "error" ||
    previous.kind === "error" ||
    previous.kind === "ask" ||
    previous.kind === "approval" ||
    current.kind === "member" ||
    previous.kind === "member"
  ) {
    return "chat-item-continuation";
  }
  return "chat-item-turn";
}

export function toolActivityKey(tool: ToolCallDTO) {
  return tool.ref || tool.id || tool.name;
}
export function messagePartKey(part: MessageRenderPart, ownerID: string) {
  return `${ownerID}:${part.key || (part.type === "tool" ? toolActivityKey(part.tool) : part.type === "ask" ? part.ask.id : part.type)}`;
}
export function nextRepeatedKey(seen: Map<string, number>, base: string) {
  const count = seen.get(base) || 0;
  seen.set(base, count + 1);
  return count === 0 ? base : `${base}:${count + 1}`;
}
export function normalizeAgentKey(value: string) {
  return value.trim().toLowerCase();
}
export function askResponseSummary(selected: string[], freeText: string) {
  return [...selected, freeText].filter((item) => item.trim()).join("；");
}
export function approvalDecisionLabel(decision: 0 | 1 | 2) {
  return ["已拒绝", "已允许（一次）", "已允许并记住"][decision];
}
