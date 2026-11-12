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
  if (!previous || !current) return "mt-6";
  const previousEndsWithTool =
    previous.kind === "message" &&
    previous.node.parts[previous.node.parts.length - 1]?.type === "tool";
  const currentStartsWithTool =
    current.kind === "message" &&
    !current.node.showAgentLabel &&
    current.node.parts[0]?.type === "tool";
  if (previousEndsWithTool && currentStartsWithTool) return "mt-0";
  if (current.kind === "error")
    return previous.kind === "message" ? "mt-4" : "mt-3";
  if (previous.kind === "ask" && current.kind === "message") return "mt-5";
  if (previous.kind === "approval" && current.kind === "message") return "mt-5";
  if (previous.kind === "error" && current.kind === "message") return "mt-6";
  if (current.kind === "member" || previous.kind === "member") return "mt-3";
  return "mt-6";
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
