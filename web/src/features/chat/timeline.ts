import type { ChatEvent, ToolCallDTO } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";

type ToolActivity = ToolCallDTO & { message_id?: string; order?: number };

export interface AskActivity {
  id: string;
  sessionID?: string;
  messageID?: string;
  sequence?: number;
  order?: number;
  question: string;
  options: string[];
  multiSelect: boolean;
  selected: string[];
  freeText: string;
  answered: boolean;
  anchored?: boolean;
  memberID?: string;
  memberName?: string;
  toolName?: string;
}

interface AskToolAnchor {
  question: string;
  order: number;
  messageID?: string;
  memberID?: string;
  memberName?: string;
  toolName?: string;
}

export interface ApprovalActivity {
  id: string;
  order: number;
  message: string;
  answered: boolean;
  decision?: string;
}

interface ErrorActivity {
  id: string;
  order: number;
  title?: string;
  message: string;
  suggestions?: string[];
  technicalDetail?: string;
}

export type MessageRenderPart =
  | { type: "reasoning"; content: string; key?: string; streaming?: boolean }
  | { type: "text"; content: string; key?: string; streaming?: boolean }
  | { type: "ask"; ask: AskActivity }
  | { type: "tool"; tool: ToolActivity; member?: MemberActivity };

interface TimelineMessageNode {
  message: ChatViewMessage;
  parts: MessageRenderPart[];
  showAgentLabel: boolean;
  showCopyAction: boolean;
  order: number;
}

type TimelineItem =
  | { kind: "message"; node: TimelineMessageNode; order: number }
  | { kind: "member"; member: MemberActivity; order: number }
  | { kind: "tool"; part: Extract<MessageRenderPart, { type: "tool" }>; order: number }
  | { kind: "ask"; ask: AskActivity; order: number }
  | { kind: "approval"; approval: ApprovalActivity; order: number }
  | { kind: "error"; error: ErrorActivity; order: number };

interface TimelineModel {
  items: TimelineItem[];
  messages: ChatViewMessage[];
  trailingAsks: AskActivity[];
  trailingTools: MessageRenderPart[];
}

function assistantMessageParts(
  message: ChatViewMessage,
  asks: AskActivity[],
  tools: ToolActivity[],
  fallbackReasoningBlocks?: string[],
  isProcessing = false,
) {
  const parts: MessageRenderPart[] = [];
  const renderedTools = new Set<string>();
  for (const event of message.events || []) {
    if (isReasoningDelta(event) && event.role !== "tool") {
      appendMessagePart(parts, "reasoning", String(event.reasoning_content || event.content || ""), textPartKey(event, "reasoning"), isProcessing && isStreamEvent(event));
      continue;
    }
    if (isTextDelta(event) && event.role !== "tool") {
      appendMessagePart(parts, "text", String(event.content || ""), textPartKey(event, "text"), isProcessing && isStreamEvent(event));
      continue;
    }
    const ask = askFromToolEvent(event, asks);
    if (ask) {
      parts.push({ type: "ask", ask });
      continue;
    }
    const tool = toolFromEvent(event, tools);
    if (tool) {
      const key = toolActivityKey(tool);
      if (!renderedTools.has(key)) {
        parts.push({ type: "tool", tool });
        renderedTools.add(key);
      }
    }
  }

  if (!parts.length) {
    for (const content of fallbackReasoningBlocks || []) {
      appendMessagePart(parts, "reasoning", content);
    }
  }

  const hasTextPart = parts.some((part) => part.type === "text" && part.content.trim());
  if (!hasTextPart && message.content.trim()) {
    // 历史只包含完成事件，必须与实时增量使用相同的分段标识来恢复交互状态。
    const completed = message.events.find(isAssistantCompleted);
    parts.push({ type: "text", content: message.content, key: completed ? textPartKey(completed, "text") : `text:${message.id}` });
  }
  return parts.filter((part) => part.type === "ask" || part.type === "tool" || part.content.trim());
}

function appendMessagePart(parts: MessageRenderPart[], type: "reasoning" | "text", content: string, key?: string, streaming = false) {
  if (!content) return;
  const previous = parts[parts.length - 1];
  if (previous?.type === type && (!key || !previous.key || previous.key === key)) {
    previous.content += content;
    previous.key = previous.key || key;
    previous.streaming = previous.streaming || streaming;
    return;
  }
  parts.push({ type, content, key, streaming });
}

export interface MemberActivity {
  id: string;
  name: string;
  completed: boolean;
  eventCount: number;
  toolCount: number;
  firstOrder: number;
  parts: MessageRenderPart[];
  tools: ToolActivity[];
  messageIDs: string[];
}

function memberForTool(tool: ToolActivity, memberByCallID: Map<string, MemberActivity>) {
  const keys = uniqueStrings([
    ...toolIdentityKeys(tool.ref),
    ...toolIdentityKeys(tool.id),
  ].filter(Boolean));
  for (const key of keys) {
    const member = memberByCallID.get(key);
    if (member) return member;
  }
  return undefined;
}

function buildMemberLookup(members: MemberActivity[]) {
  const result = new Map<string, MemberActivity>();
  for (const member of members) {
    for (const key of memberLookupKeys(member.id)) {
      result.set(key, member);
    }
  }
  return result;
}

function memberIDsWithParentTools(members: MemberActivity[], tools: ToolActivity[]) {
  const lookup = buildMemberLookup(members);
  const result = new Set<string>();
  for (const tool of tools) {
    const member = memberForTool(tool, lookup);
    if (member) result.add(member.id);
  }
  return result;
}

function memberLookupKeys(value: string) {
  if (!value) return [];
  return uniqueStrings(toolIdentityKeys(value));
}

export function buildTimelineModel(
  messages: ChatViewMessage[],
  displayEvents: ChatEvent[],
  submittedAskIDs: Set<string>,
  submittedApprovalIDs: Set<string>,
  isProcessing: boolean,
): TimelineModel {
  const eventOrders = eventOrderMap(displayEvents);
  const reasoningByMessage = collectReasoningBlocks(displayEvents);
  const toolEvents = collectToolActivities(displayEvents, { includeMemberEvents: false });
  const askToolAnchors = collectAskToolAnchors(displayEvents);
  const askActivities = collectAskActivities(displayEvents, submittedAskIDs, askToolAnchors);
  const approvalActivities = collectApprovalActivities(displayEvents, submittedApprovalIDs, eventOrders);
  const errorActivities = collectErrorActivities(displayEvents, eventOrders);
  const memberActivities = collectMemberActivities(displayEvents, isProcessing, askActivities);
  const memberLookup = buildMemberLookup(memberActivities);
  const attachedMemberIDs = new Set<string>();
  const renderedToolKeys = new Set<string>();
  const memberByMessageID = mapMembersByMessageID(memberActivities);
  const toolEventsByMessageID = groupToolsByMessageID(toolEvents);
  const baseMessages = dedupeAdjacentSystemMessages(
    messages.filter((message) => !message.hidden && shouldShowTimelineItem(message, reasoningByMessage, memberByMessageID)),
  );
  const messageNodes = baseMessages.map((message, index) => {
    const messageTools = toolEventsByMessageID.get(message.id) || [];
    const parts = message.role === "assistant"
      ? attachMembersToParts(
          assistantMessageParts(
            message,
            askActivities,
            messageTools,
            reasoningByMessage.get(message.id) || reasoningContentBlocks(message.reasoningContent),
            isProcessing,
          ),
          memberLookup,
          attachedMemberIDs,
        )
      : [];
    for (const part of parts) {
      if (part.type === "tool") renderedToolKeys.add(toolActivityKey(part.tool));
    }
    return {
      message,
      parts,
      showAgentLabel: false,
      showCopyAction: false,
      order: messageOrder(message, index, eventOrders),
    };
  });
  const trailingTools = attachMembersToParts(
    toolEvents
      .filter((tool) => !renderedToolKeys.has(toolActivityKey(tool)))
      .map((tool): MessageRenderPart => ({ type: "tool", tool })),
    memberLookup,
    attachedMemberIDs,
  );
  for (const part of trailingTools) {
    if (part.type === "tool") renderedToolKeys.add(toolActivityKey(part.tool));
  }
  const fallbackMembers = memberActivities.filter((member) => !attachedMemberIDs.has(member.id));
  const timelineTools = trailingTools.filter((part): part is Extract<MessageRenderPart, { type: "tool" }> => part.type === "tool");
  const inlineAskIDs = unionSets(collectInlineAskIDsFromMessageNodes(messageNodes), collectInlineAskIDsFromMembers(memberActivities));
  const timelineAsks = askActivities.filter((ask) => !ask.memberID && !inlineAskIDs.has(ask.id));
  const items = orderedTimelineItems(messageNodes, fallbackMembers, timelineTools, timelineAsks, approvalActivities, errorActivities);
  markFinalAssistantCopyActions(items, isProcessing);
  const timelineMessages = items.filter((item) => item.kind === "message").map((item) => item.node.message);
  const agentLabelMessageIDs = visibleAgentLabelMessageIDs(timelineMessages);
  for (const item of items) {
    if (item.kind === "message") item.node.showAgentLabel = agentLabelMessageIDs.has(item.node.message.id);
  }
  const renderedAskIDs = unionSets(collectInlineAskIDsFromItems(items), collectInlineAskIDsFromMembers(memberActivities));
  return {
    items,
    messages: timelineMessages,
    trailingAsks: askActivities.filter((ask) => !renderedAskIDs.has(ask.id)),
    trailingTools: [],
  };
}

function attachMembersToParts(
  parts: MessageRenderPart[],
  memberLookup: Map<string, MemberActivity>,
  attachedMemberIDs: Set<string>,
) {
  return parts.map((part) => {
    if (part.type !== "tool") return part;
    const member = memberForTool(part.tool, memberLookup);
    if (!member || attachedMemberIDs.has(member.id)) return part;
    attachedMemberIDs.add(member.id);
    const status = member.completed && (part.tool.status === "pending" || part.tool.status === "running")
      ? "completed"
      : part.tool.status;
    return { ...part, tool: { ...part.tool, status }, member };
  });
}

export function toolEventsKey(events: Array<{ tool_calls?: unknown[]; tool_call?: unknown; tool_call_ref?: string; type?: string }>) {
  return events
    .map((event) => `${event.type}:${event.tool_call_ref || ""}:${event.tool_calls?.length || 0}:${event.tool_call ? 1 : 0}`)
    .join(":");
}

export function eventsForDisplay(liveEvents: ChatEvent[]) {
  const seen = new Set<string>();
  const result: ChatEvent[] = [];
  for (const event of liveEvents) {
    const key = eventDisplayKey(event);
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(event);
  }
  return result;
}

function eventDisplayKey(event: ChatEvent) {
  if (event.event_id) return `event:${event.event_id}`;
  if (event.run_id && event.sequence !== undefined) return `run:${event.run_id}:${event.sequence}`;
  return [
    event.type,
    event.message_id || "",
    event.stream_id || "",
    event.sequence || "",
    event.delta_kind || "",
    event.tool_call_ref || "",
    event.tool_call_id || "",
    event.ask_id || event.detail || "",
    event.content || "",
  ].join(":");
}

function shouldShowTimelineItem(
  message: ChatViewMessage,
  reasoningByMessage: Map<string, string[]>,
  memberByMessageID: Map<string, MemberActivity>,
) {
  if (message.hidden) return memberByMessageID.has(message.id);
  if (message.role === "user") return Boolean(message.content.trim());
  if (message.content.trim()) return true;
  if (message.reasoningContent?.trim()) return true;
  if ((reasoningByMessage.get(message.id) || []).some((content) => content.trim())) return true;
  return false;
}

function dedupeAdjacentSystemMessages(messages: ChatViewMessage[]) {
  let lastSystemContent = "";
  return messages.filter((message) => {
    if (message.role !== "system") return true;
    const key = message.content.trim();
    if (!key) return false;
    if (lastSystemContent === key) return false;
    lastSystemContent = key;
    return true;
  });
}

function visibleAgentLabelMessageIDs(messages: ChatViewMessage[]) {
  const result = new Set<string>();
  let previousAgent = "";
  for (const message of messages) {
    if (message.hidden) continue;
    if (message.role === "user" || message.role === "system") {
      previousAgent = "";
      continue;
    }
    if (message.role !== "assistant" || !message.agent) continue;
    const currentAgent = normalizeAgentKey(message.agent);
    if (currentAgent !== previousAgent) {
      result.add(message.id);
      previousAgent = currentAgent;
    }
  }
  return result;
}

function orderedTimelineItems(
  messages: TimelineMessageNode[],
  members: MemberActivity[],
  tools: Array<Extract<MessageRenderPart, { type: "tool" }>>,
  asks: AskActivity[],
  approvals: ApprovalActivity[],
  errors: ErrorActivity[],
): TimelineItem[] {
  const items: TimelineItem[] = [];
  for (const node of messages) {
    items.push({ kind: "message", node, order: node.order });
  }
  for (const part of tools) {
    items.push({ kind: "tool", part, order: part.tool.order ?? Number.MAX_SAFE_INTEGER });
  }
  for (const member of members) {
    items.push({ kind: "member", member, order: member.firstOrder });
  }
  for (const ask of asks) {
    items.push({ kind: "ask", ask, order: ask.order ?? Number.MAX_SAFE_INTEGER });
  }
  for (const approval of approvals) {
    items.push({ kind: "approval", approval, order: approval.order });
  }
  for (const error of errors) {
    items.push({ kind: "error", error, order: error.order });
  }
  return items.sort((left, right) => {
    if (left.order !== right.order) return left.order - right.order;
    if (left.kind === right.kind) return 0;
    return timelineKindPriority(left.kind) - timelineKindPriority(right.kind);
  });
}

function timelineKindPriority(kind: TimelineItem["kind"]) {
  switch (kind) {
    case "message":
      return 0;
    case "tool":
      return 1;
    case "ask":
      return 2;
    case "approval":
      return 3;
    case "member":
      return 4;
    case "error":
      return 5;
    default:
      return 6;
  }
}

export function timelineItemSpacingClass(items: TimelineItem[], index: number) {
  if (index <= 0) return "";
  const previous = items[index - 1];
  const current = items[index];
  if (!previous || !current) return "mt-6";
  const previousEndsWithTool = previous.kind === "tool"
    || (previous.kind === "message" && previous.node.parts[previous.node.parts.length - 1]?.type === "tool");
  const currentStartsWithTool = current.kind === "tool"
    || (current.kind === "message" && !current.node.showAgentLabel && current.node.parts[0]?.type === "tool");
  if (previousEndsWithTool && currentStartsWithTool) return "mt-0";
  if (current.kind === "tool") return "mt-2";
  if (current.kind === "ask") return previous.kind === "tool" ? "mt-2" : "mt-3";
  if (current.kind === "approval") return previous.kind === "tool" ? "mt-2" : "mt-3";
  if (current.kind === "error") return previous.kind === "message" ? "mt-4" : "mt-3";
  if (previous.kind === "tool" && current.kind === "message") return "mt-5";
  if (previous.kind === "ask" && current.kind === "message") return "mt-5";
  if (previous.kind === "approval" && current.kind === "message") return "mt-5";
  if (previous.kind === "error" && current.kind === "message") return "mt-6";
  if (current.kind === "member" || previous.kind === "member") return "mt-3";
  return "mt-6";
}

function markFinalAssistantCopyActions(items: TimelineItem[], suppressCurrentTurn: boolean) {
  let candidate: TimelineMessageNode | undefined;
  const flush = (suppress = false) => {
    if (candidate && !suppress) candidate.showCopyAction = true;
    candidate = undefined;
  };
  for (const item of items) {
    if (item.kind !== "message") continue;
    const message = item.node.message;
    if (message.role === "user") {
      flush();
      continue;
    }
    if (message.role === "assistant" && hasCopyableAssistantText(item.node)) {
      candidate = item.node;
    }
  }
  flush(suppressCurrentTurn);
}

function hasCopyableAssistantText(node: TimelineMessageNode) {
  if (node.message.content.trim()) return true;
  return node.parts.some((part) => part.type === "text" && part.content.trim());
}

function messageOrder(message: ChatViewMessage, fallback: number, eventOrders: Map<string, number>) {
  const sequences = (message.events || [])
    .map((event) => eventOrders.get(eventDisplayKey(event)))
    .filter((sequence): sequence is number => sequence !== undefined && Number.isFinite(sequence));
  if (sequences.length) return Math.min(...sequences);
  return fallback + 0.5;
}

function eventOrderMap(events: ChatEvent[]) {
  const result = new Map<string, number>();
  events.forEach((event, index) => {
    const key = eventDisplayKey(event);
    if (!result.has(key)) result.set(key, index);
  });
  return result;
}

function collectErrorActivities(events: ChatEvent[], eventOrders: Map<string, number>) {
  const result: ErrorActivity[] = [];
  for (const event of events) {
    if (event.type !== "error") continue;
    const message = errorEventMessage(event);
    if (!message.trim()) continue;
    const id = event.event_id || eventDisplayKey(event);
    result.push({
      id,
      order: eventOrders.get(eventDisplayKey(event)) ?? Number.MAX_SAFE_INTEGER,
      title: stringValue(event.error_title),
      message,
      suggestions: Array.isArray(event.error_suggestions) ? event.error_suggestions : undefined,
      technicalDetail: stringValue(event.technical_error || event.error || event.content || event.message),
    });
  }
  return result;
}

function collectApprovalActivities(
  events: ChatEvent[],
  submittedApprovalIDs: Set<string>,
  eventOrders: Map<string, number>,
) {
  const approvals: ApprovalActivity[] = [];
  const byID = new Map<string, ApprovalActivity>();
  for (const event of events) {
    if (event.type === "approval_requested") {
      const id = approvalEventID(event) || eventDisplayKey(event) || `approval-${approvals.length + 1}`;
      const existing = byID.get(id);
      if (existing) {
        existing.message = approvalRequestMessage(event) || existing.message;
        existing.answered = existing.answered || submittedApprovalIDs.has(id);
        continue;
      }
      const activity: ApprovalActivity = {
        id,
        order: eventOrders.get(eventDisplayKey(event)) ?? Number.MAX_SAFE_INTEGER,
        message: approvalRequestMessage(event) || "需要审批",
        answered: submittedApprovalIDs.has(id),
      };
      approvals.push(activity);
      byID.set(id, activity);
      continue;
    }

    if (event.type !== "approval_answered") continue;
    const explicitID = approvalEventID(event);
    const target = explicitID ? byID.get(explicitID) : approvals.slice().reverse().find((approval) => !approval.answered);
    if (!target) continue;
    target.answered = true;
    target.decision = approvalDecisionFromEvent(event);
  }
  for (const id of submittedApprovalIDs) {
    const approval = byID.get(id);
    if (approval) approval.answered = true;
  }
  return approvals.filter((approval) => approval.message.trim());
}

function errorEventMessage(event: ChatEvent) {
  return stringValue(event.display_error || event.error_title || event.error || event.content || event.message) || "请求失败";
}

function approvalEventID(event: ChatEvent) {
  return String(event.detail || "").trim();
}

function approvalRequestMessage(event: ChatEvent) {
  const approval = approvalPayload(event);
  return stringValue(approval.message) || stringValue(event.content || event.message);
}

function approvalDecisionFromEvent(event: ChatEvent) {
  const approval = approvalPayload(event);
  return stringValue(approval.decision) || stringValue(event.content || event.message) || "已处理";
}

export function approvalDecisionLabel(decision: 0 | 1 | 2) {
  switch (decision) {
    case 0:
      return "已拒绝";
    case 1:
      return "已允许（一次）";
    case 2:
      return "已允许并记住";
    default:
      return "已处理";
  }
}

function approvalPayload(event: ChatEvent) {
  const value = event.approval;
  return value || {};
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
}

function mapMembersByMessageID(members: MemberActivity[]) {
  const result = new Map<string, MemberActivity>();
  for (const member of members) {
    for (const messageID of member.messageIDs) {
      result.set(messageID, member);
    }
  }
  return result;
}

function groupToolsByMessageID(tools: ToolActivity[]) {
  const result = new Map<string, ToolActivity[]>();
  for (const tool of tools) {
    if (!tool.message_id) continue;
    result.set(tool.message_id, [...(result.get(tool.message_id) || []), tool]);
  }
  return result;
}

function collectInlineAskIDsFromMessageNodes(nodes: TimelineMessageNode[]) {
  const ids = new Set<string>();
  for (const node of nodes) {
    for (const part of node.parts) {
      if (part.type === "ask") ids.add(part.ask.id);
    }
  }
  return ids;
}

function collectInlineAskIDsFromMembers(members: MemberActivity[]) {
  const ids = new Set<string>();
  for (const member of members) {
    for (const part of member.parts) {
      if (part.type === "ask") ids.add(part.ask.id);
    }
  }
  return ids;
}

function collectInlineAskIDsFromItems(items: TimelineItem[]) {
  const ids = new Set<string>();
  for (const item of items) {
    if (item.kind === "ask") {
      ids.add(item.ask.id);
      continue;
    }
    if (item.kind !== "message") continue;
    for (const part of item.node.parts) {
      if (part.type === "ask") ids.add(part.ask.id);
    }
  }
  return ids;
}

function unionSets<T>(...sets: Array<Set<T>>) {
  const result = new Set<T>();
  for (const set of sets) {
    for (const item of set) result.add(item);
  }
  return result;
}

function collectReasoningBlocks(events: ChatEvent[]) {
  const blocks = new Map<string, string[]>();
  const closed = new Set<string>();
  for (const event of events) {
    if (isMemberActivityEvent(event)) continue;
    const keys = reasoningKeys(event);
    if (!isReasoningDelta(event) || event.role === "tool") {
      for (const key of keys) {
        if ((blocks.get(key) || []).length > 0) closed.add(key);
      }
      continue;
    }
    const content = String(event.reasoning_content || event.content || "");
    if (!content) continue;
    for (const key of keys) {
      const current = blocks.get(key) || [];
      if (!current.length || closed.has(key)) {
        current.push(content);
      } else {
        current[current.length - 1] = appendText(current[current.length - 1], content);
      }
      blocks.set(key, current);
      closed.delete(key);
    }
  }
  return blocks;
}

function reasoningContentBlocks(content?: string) {
  return content?.trim() ? [content] : [];
}

function reasoningKeys(event: ChatEvent) {
  const keys = new Set<string>();
  if (event.message_id) keys.add(event.message_id);
  if (event.stream_id) {
    keys.add(event.stream_id);
    const suffix = event.delta_kind ? `:${event.delta_kind}` : "";
    if (suffix && event.stream_id.endsWith(suffix)) {
      keys.add(event.stream_id.slice(0, -suffix.length));
    }
  }
  if (keys.size === 0) keys.add(`${event.agent_name || "assistant"}`);
  return keys;
}

function collectAskActivities(events: ChatEvent[], submittedAskIDs: Set<string>, anchors: AskToolAnchor[]) {
  const asks = new Map<string, AskActivity>();
  const answered = new Map<string, { selected: string[]; freeText: string }>();
  let latestAskID = "";

  events.forEach((event, index) => {
    if (isAskQuestionEvent(event)) {
      const id = askEventID(event) || `ask-${asks.size + 1}`;
      latestAskID = id;
      const existing = asks.get(id);
      const question = String(event.question || event.content || existing?.question || "");
      const memberID = memberActivityIDIfPresent(event);
      const anchor = matchingAskToolAnchor(question, anchors, eventOrder(event, index), memberID);
      asks.set(id, {
        id,
        sessionID: event.session_id || existing?.sessionID,
        messageID: anchor?.messageID || event.message_id || existing?.messageID,
        sequence: event.sequence ?? existing?.sequence,
        order: anchor?.order ?? existing?.order ?? eventOrder(event, index),
        question,
        options: askOptions(event.options) || existing?.options || [],
        multiSelect: Boolean(event.multi_select ?? existing?.multiSelect),
        selected: existing?.selected || [],
        freeText: existing?.freeText || "",
        answered: submittedAskIDs.has(id) || existing?.answered || false,
        anchored: Boolean(anchor) || existing?.anchored,
        memberID: anchor?.memberID || memberID || existing?.memberID,
        memberName: anchor?.memberName || event.member_name || existing?.memberName,
        toolName: anchor?.toolName || event.tool_name || existing?.toolName,
      });
    }
    if (isAskResponseEvent(event)) {
      const id = askEventID(event) || latestAskID;
      if (!id) return;
      answered.set(id, parseAskResponse(event));
    }
  });

  for (const [id, response] of answered) {
    const ask = asks.get(id);
    if (!ask) continue;
    ask.selected = response.selected;
    ask.freeText = response.freeText;
    ask.answered = true;
  }
  for (const id of submittedAskIDs) {
    const ask = asks.get(id);
    if (ask) ask.answered = true;
  }
  return Array.from(asks.values()).filter((ask) => ask.question.trim());
}

function collectAskToolAnchors(events: ChatEvent[]) {
  const anchors: AskToolAnchor[] = [];
  events.forEach((event, index) => {
    const order = eventOrder(event, index) + 0.1;
    const seen = new Set<string>();
    const addAnchor = (question: string, toolName?: string) => {
      if (!question) return;
      const key = `${question}:${toolName || ""}`;
      if (seen.has(key)) return;
      seen.add(key);
      anchors.push({
        question,
        order,
        messageID: event.message_id,
        memberID: memberActivityIDIfPresent(event),
        memberName: event.member_name,
        toolName,
      });
    };
    const tools = [...(event.tool_calls || []), event.tool_call].filter(Boolean) as ToolCallDTO[];
    for (const tool of tools) {
      if (tool.name !== "ask_questions") continue;
      addAnchor(askQuestionFromArgs(tool.arguments), tool.display_name || tool.name);
    }
    if (event.tool_name === "ask_questions") {
      addAnchor(askQuestionFromArgs(event.tool_args), event.tool_display_name || event.tool_name);
    }
  });
  return anchors;
}

function askQuestionFromArgs(value: unknown) {
  const args = parseToolJSON(value);
  return String(args.question || "").trim();
}

function matchingAskToolAnchor(question: string, anchors: AskToolAnchor[], order: number, memberID?: string) {
  const sameQuestion = anchors.filter((anchor) => {
    if (anchor.question.trim() !== question.trim()) return false;
    return memberID ? anchor.memberID === memberID : !anchor.memberID;
  });
  if (!sameQuestion.length) return undefined;
  return sameQuestion
    .slice()
    .sort((left, right) => Math.abs(left.order - order) - Math.abs(right.order - order))[0];
}

function isAskQuestionEvent(event: ChatEvent) {
  return event.type === "ask_requested";
}

function isAskResponseEvent(event: ChatEvent) {
  return event.type === "ask_answered";
}

function askEventID(event: ChatEvent) {
  return String(event.ask_id || event.detail || "");
}

function askOptions(value: unknown) {
  if (!Array.isArray(value)) return undefined;
  return value.map((item) => String(item)).filter(Boolean);
}

function parseAskResponse(event: ChatEvent) {
  const selected = askOptions(event.selected) || [];
  const structuredFreeText = event.free_text;
  const freeText = structuredFreeText !== undefined
    ? String(structuredFreeText)
    : selected.length
      ? ""
      : String(event.content || "");
  return { selected, freeText };
}

export function askResponseSummary(selected: string[], freeText: string) {
  return [...selected, freeText].filter((item) => item && item.trim()).join("；");
}

function askFromToolEvent(event: ChatEvent, asks: AskActivity[]) {
  const tools = [...(event.tool_calls || []), event.tool_call].filter(Boolean) as ToolCallDTO[];
  for (const tool of tools) {
    if (tool.name !== "ask_questions") continue;
    const args = parseToolJSON(tool.arguments);
    const result = parseToolJSON(tool.result);
    const question = String(args.question || "");
    if (!question.trim()) continue;
    const matched = findMatchingAsk(question, asks, event.sequence, memberActivityIDIfPresent(event));
    const selected = askOptions(result.selected) || matched?.selected || [];
    const freeText = String(result.free_text || result.answer || matched?.freeText || "");
    return {
      id: matched?.id || tool.id || tool.ref || `ask-${event.message_id || ""}-${event.sequence ?? ""}`,
      sessionID: matched?.sessionID || event.session_id,
      messageID: event.message_id,
      sequence: event.sequence,
      order: matched?.order,
      question,
      options: askOptions(args.options) || matched?.options || [],
      multiSelect: Boolean(args.multi_select ?? matched?.multiSelect),
      selected,
      freeText,
      answered: Boolean(matched?.answered || selected.length || freeText.trim() || tool.result),
      memberName: matched?.memberName || event.member_name,
      toolName: matched?.toolName || tool.display_name || tool.name,
    };
  }
  if (event.tool_name === "ask_questions") {
    const args = parseToolJSON(event.tool_args);
    const result = parseToolJSON(event.tool_result);
    const question = String(args.question || "");
    if (!question.trim()) return undefined;
    const matched = findMatchingAsk(question, asks, event.sequence, memberActivityIDIfPresent(event));
    const selected = askOptions(result.selected) || matched?.selected || [];
    const freeText = String(result.free_text || result.answer || matched?.freeText || "");
    return {
      id: matched?.id || event.tool_call_id || event.tool_call_ref || `ask-${event.message_id || ""}-${event.sequence ?? ""}`,
      sessionID: matched?.sessionID || event.session_id,
      messageID: event.message_id,
      sequence: event.sequence,
      order: matched?.order,
      question,
      options: askOptions(args.options) || matched?.options || [],
      multiSelect: Boolean(args.multi_select ?? matched?.multiSelect),
      selected,
      freeText,
      answered: Boolean(matched?.answered || selected.length || freeText.trim() || event.tool_result),
      memberID: matched?.memberID || memberActivityIDIfPresent(event),
      memberName: matched?.memberName || event.member_name,
      toolName: matched?.toolName || event.tool_display_name || event.tool_name,
    };
  }
  return undefined;
}

function askFromAskEvent(event: ChatEvent, asks: AskActivity[]) {
  if (!isAskQuestionEvent(event)) return undefined;
  const id = askEventID(event);
  if (id) {
    const byID = asks.find((ask) => ask.id === id);
    if (byID) return byID.anchored ? undefined : byID;
  }
  const question = String(event.question || event.content || "").trim();
  if (!question) return undefined;
  const order = typeof event.sequence === "number" ? event.sequence : undefined;
  const matched = findMatchingAsk(question, asks, order, memberActivityIDIfPresent(event));
  return matched?.anchored ? undefined : matched;
}

function toolFromEvent(event: ChatEvent, tools: ToolActivity[]) {
  if (!hasToolEvent(event)) return undefined;
  const directTools = [...(event.tool_calls || []), event.tool_call].filter(Boolean) as ToolCallDTO[];
  for (const directTool of directTools) {
    if (directTool.name === "ask_questions") continue;
    const directKey = toolKey(directTool, event);
    const matched = tools.find((tool) => toolActivityKey(tool) === directKey || tool.ref === directTool.ref || tool.id === directTool.id);
    if (matched) return matched;
    const result = eventToolResultContent(event);
    return {
      ...directTool,
      ref: directTool.ref || event.tool_call_ref,
      id: directTool.id || event.tool_call_id,
      status: isAssistantCompleted(event) || isToolCompleted(event) ? "completed" : "running",
      result: directTool.result || result || undefined,
      message_id: event.message_id,
    };
  }
  const key = event.tool_call_ref || event.tool_call_id || event.tool_name;
  if (!key) return undefined;
  return tools.find((tool) => tool.ref === key || tool.id === key || tool.name === key);
}

function hasToolEvent(event: ChatEvent) {
  return Boolean(event.tool_calls?.length || event.tool_call || event.tool_name || event.tool_call_ref || event.tool_call_id);
}

function isReasoningDelta(event: ChatEvent) {
  return event.type === "assistant_reasoning_delta";
}

function isTextDelta(event: ChatEvent) {
  return event.type === "assistant_text_delta";
}

function isAssistantCompleted(event: ChatEvent) {
  return event.type === "assistant_completed";
}

function isAgentCompleted(event: ChatEvent) {
  return event.type === "agent_completed";
}

function isToolCompleted(event: ChatEvent) {
  return event.type === "tool_call_completed";
}

function isToolResultEvent(event: ChatEvent) {
  return event.type === "tool_call_result_delta" || event.type === "tool_call_completed" || event.delta_kind === "tool_result";
}

function eventToolResultContent(event: ChatEvent) {
  return String(event.tool_result || ((isToolResultEvent(event) || event.role === "tool") ? event.content : "") || "");
}

function findMatchingAsk(question: string, asks: AskActivity[], sequence?: number, memberID?: string) {
  const sameQuestion = asks.filter((ask) => {
    if (ask.question.trim() !== question.trim()) return false;
    return memberID ? ask.memberID === memberID : !ask.memberID;
  });
  if (!sameQuestion.length) return undefined;
  if (sequence === undefined) return sameQuestion[0];
  return sameQuestion
    .slice()
    .sort((left, right) => Math.abs((left.sequence ?? sequence) - sequence) - Math.abs((right.sequence ?? sequence) - sequence))[0];
}

function parseToolJSON(value: unknown) {
  if (!value || typeof value !== "string") return {};
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === "object" ? parsed as Record<string, unknown> : {};
  } catch {
    return {};
  }
}

function collectToolActivities(events: ChatEvent[], options: { includeMemberEvents?: boolean } = {}): ToolActivity[] {
  const result = new Map<string, ToolActivity>();
  const order: string[] = [];
  const unfinished = new Set<string>();
  const upsert = (key: string, patch: Partial<ToolActivity>) => {
    if (!result.has(key)) {
      result.set(key, {
        id: patch.id,
        ref: patch.ref,
        name: patch.name || "tool",
        status: "pending",
        message_id: patch.message_id,
        order: patch.order,
      });
      order.push(key);
    }
    const current = result.get(key)!;
    for (const [field, value] of Object.entries(patch)) {
      if (value === undefined || value === "") continue;
      if (field === "order" && Number.isFinite(current.order)) continue;
      (current as unknown as Record<string, unknown>)[field] = value;
    }
    if (current.status === "pending" || current.status === "running") unfinished.add(key);
    else unfinished.delete(key);
  };

  events.forEach((event, index) => {
    if (!options.includeMemberEvents && isMemberActivityEvent(event)) return;
    const terminalStatus = event.type === "error" ? "error"
      : event.type === "cancelled" ? "cancelled"
      : event.type === "processing_end" ? "completed" : undefined;
    if (terminalStatus) {
      // 只收尾此事件之前的调用，避免历史回合的结束状态覆盖当前运行。
      for (const key of unfinished) result.get(key)!.status = terminalStatus;
      unfinished.clear();
    }
    const orderValue = eventOrder(event, index);
    for (const tool of event.tool_calls || []) {
      if (tool.name === "ask_questions") continue;
      const key = toolKey(tool, event);
      upsert(key, {
        ...tool,
        ref: tool.ref || event.tool_call_ref,
        id: tool.id || event.tool_call_id,
        status: tool.status || result.get(key)?.status || (tool.result ? "completed" : "pending"),
        member_name: event.member_name || tool.member_name,
        result: tool.result || eventToolResultContent(event) || undefined,
        message_id: event.message_id,
        order: orderValue,
      });
    }
    if (event.tool_call && event.tool_call.name !== "ask_questions") {
      const key = toolKey(event.tool_call, event);
      upsert(key, {
        ...event.tool_call,
        ref: event.tool_call.ref || event.tool_call_ref,
        id: event.tool_call.id || event.tool_call_id,
        status: event.tool_call.status || result.get(key)?.status || (event.tool_call.result ? "completed" : "pending"),
        member_name: event.member_name || event.tool_call.member_name,
        result: event.tool_call.result || eventToolResultContent(event) || undefined,
        message_id: event.message_id,
        order: orderValue,
      });
    }
    if (event.tool_name || event.tool_call_ref || event.tool_call_id) {
      const key = canonicalToolRef(event.tool_call_ref || event.tool_call_id) || eventDisplayKey(event);
      const current = result.get(key);
      upsert(key, {
        ref: event.tool_call_ref,
        id: event.tool_call_id,
        name: event.tool_name || current?.name || "tool",
        display_name: event.tool_display_name || current?.display_name,
        kind: event.tool_kind || current?.kind,
        target: event.tool_target || current?.target,
        member_name: event.member_name || current?.member_name,
        status: isToolCompleted(event) ? "completed"
          : event.type === "tool_call_failed" || event.type === "error" ? "error"
          : event.type === "tool_call_started" || event.type === "tool_call_result_delta" ? "running"
          : current?.status || "pending",
        message_id: event.message_id || current?.message_id,
        order: orderValue,
      });
      const next = result.get(key)!;
      const argsContent = String(event.tool_args || (event.delta_kind === "tool_args" ? event.content : "") || "");
      const resultContent = eventToolResultContent(event);
      if (event.delta_kind === "tool_args" && argsContent) {
        next.arguments = appendText(next.arguments, argsContent);
      }
      if ((event.type === "tool_call_started" || event.delta_kind === "tool_args") && argsContent && !next.arguments) {
        next.arguments = argsContent;
      }
      if ((isToolResultEvent(event) || event.role === "tool") && resultContent) {
        next.result = event.type === "tool_call_completed" ? resultContent : appendText(next.result, resultContent);
      }
    }
  });
  return order
    .map((key) => result.get(key)!)
    .filter((tool) => tool.name && tool.name !== "tool");
}

type OrderedChatEvent = { event: ChatEvent; order: number };

function collectMemberActivities(events: ChatEvent[], isProcessing: boolean, asks: AskActivity[]) {
  const grouped = new Map<string, OrderedChatEvent[]>();
  const completedMemberIDs = completedMemberCallIDs(events);
  events.forEach((event, index) => {
    if (!isMemberActivityEvent(event)) return;
    const id = memberActivityID(event);
    grouped.set(id, [...(grouped.get(id) || []), { event, order: eventOrder(event, index) }]);
  });
  return Array.from(grouped.entries())
    .map(([id, orderedEvents]) => {
      const memberEvents = orderedEvents
        .slice()
        .sort((left, right) => left.order - right.order)
        .map((item) => item.event);
      const tools = collectToolActivities(memberEvents, { includeMemberEvents: true });
      const parts = memberMessageParts(memberEvents, tools, asks, isProcessing);
      return {
        id,
        name: memberEvents.find((event) => event.member_name)?.member_name || memberEvents[0]?.agent_name || "子智能体",
        completed: completedMemberIDs.has(stripToolRef(id)) || memberEvents.some(isAgentCompleted) || memberEvents.some((event) => event.type === "processing_end"),
        eventCount: memberEvents.length,
        toolCount: tools.length,
        firstOrder: Math.min(...orderedEvents.map((item) => item.order)),
        parts,
        tools,
        messageIDs: uniqueStrings(memberEvents.map((event) => event.message_id).filter(Boolean) as string[]),
      };
    })
    .filter((member) => member.parts.length);
}

function completedMemberCallIDs(events: ChatEvent[]) {
  const result = new Set<string>();
  for (const event of events) {
    if (!isToolCompleted(event)) continue;
    if (isAgentDispatchToolEvent(event)) {
      for (const key of toolIdentityKeys(event.tool_call_ref || event.tool_call_id)) {
        result.add(stripToolRef(key));
      }
    }
    for (const tool of event.tool_calls || []) {
      if (!isAgentDispatchToolDTO(tool)) continue;
      for (const key of toolIdentityKeys(tool.ref || tool.id)) {
        result.add(stripToolRef(key));
      }
    }
    if (event.tool_call && isAgentDispatchToolDTO(event.tool_call)) {
      for (const key of toolIdentityKeys(event.tool_call.ref || event.tool_call.id)) {
        result.add(stripToolRef(key));
      }
    }
  }
  return result;
}

function isAgentDispatchToolEvent(event: ChatEvent) {
  return event.tool_kind === "agent" || Boolean(event.tool_name?.startsWith("ask_fkagent_"));
}

function isAgentDispatchToolDTO(tool: ToolCallDTO) {
  return tool.kind === "agent" || Boolean(tool.name?.startsWith("ask_fkagent_"));
}

function memberActivityID(event: ChatEvent) {
  if (event.member_call_id) return event.member_call_id;
  if (event.parent_tool_call_id) return event.parent_tool_call_id;
  if (event.message_id) return event.message_id;
  if (event.stream_id) return event.stream_id;
  return eventDisplayKey(event);
}

function memberActivityIDIfPresent(event: ChatEvent) {
  if (!isMemberActivityEvent(event)) return undefined;
  return memberActivityID(event);
}

function memberMessageParts(events: ChatEvent[], tools: ToolActivity[], asks: AskActivity[], isProcessing = false) {
  const parts: MessageRenderPart[] = [];
  const renderedTools = new Set<string>();
  const renderedAsks = new Set<string>();
  const seenReasoningDeltaKeys = new Set<string>();
  const seenTextDeltaKeys = new Set<string>();
  let reasoningOpen = false;
  let textOpen = false;

  for (const event of events) {
    if (isReasoningDelta(event) && event.role !== "tool") {
      const content = String(event.reasoning_content || event.content || "");
      const key = textPartKey(event, "reasoning");
      appendSequencedTextPart(parts, "reasoning", content, reasoningOpen, key, isProcessing && isStreamEvent(event));
      seenReasoningDeltaKeys.add(key);
      reasoningOpen = true;
      textOpen = false;
      continue;
    }
    reasoningOpen = false;

    if (isTextDelta(event) && event.role !== "tool") {
      const content = String(event.content || "");
      const key = textPartKey(event, "text");
      appendSequencedTextPart(parts, "text", content, textOpen, key, isProcessing && isStreamEvent(event));
      seenTextDeltaKeys.add(key);
      textOpen = true;
      continue;
    }
    textOpen = false;

    const ask = askFromToolEvent(event, asks) || askFromAskEvent(event, asks);
    if (ask && !renderedAsks.has(ask.id)) {
      parts.push({ type: "ask", ask });
      renderedAsks.add(ask.id);
      continue;
    }

    const tool = toolFromEvent(event, tools);
    if (tool) {
      const key = toolActivityKey(tool);
      if (!renderedTools.has(key)) {
        parts.push({ type: "tool", tool });
        renderedTools.add(key);
      }
    }

    if (isAssistantCompleted(event)) {
      const reasoningKey = textPartKey(event, "reasoning");
      const reasoning = seenReasoningDeltaKeys.has(reasoningKey) ? "" : String(event.reasoning_content || "");
      if (reasoning) {
        appendSequencedTextPart(parts, "reasoning", reasoning, false, reasoningKey);
      }
      const textKey = textPartKey(event, "text");
      const content = seenTextDeltaKeys.has(textKey) ? "" : String(event.content || "");
      if (content) {
        appendSequencedTextPart(parts, "text", content, false, textKey);
      }
    }
  }

  return parts.filter((part) => part.type === "ask" || part.type === "tool" || part.content.trim());
}

function appendSequencedTextPart(
  parts: MessageRenderPart[],
  type: "reasoning" | "text",
  content: string,
  mergeWithPrevious: boolean,
  key?: string,
  streaming = false,
) {
  if (!content) return;
  const previous = parts[parts.length - 1];
  if (mergeWithPrevious && previous?.type === type && (!key || !previous.key || previous.key === key)) {
    previous.content = appendText(previous.content, content);
    previous.key = previous.key || key;
    previous.streaming = previous.streaming || streaming;
    return;
  }
  parts.push({ type, content, key, streaming });
}

function eventOrder(_event: ChatEvent, fallback: number) {
  return fallback;
}

function uniqueStrings(values: string[]) {
  return Array.from(new Set(values));
}

function isMemberActivityEvent(event: ChatEvent) {
  return Boolean(event.is_member_event || event.member_call_id || event.member_name || event.member_tool_name || event.parent_tool_call_id);
}

function stripToolRef(ref: string) {
  return ref.startsWith("tool_call:") ? ref.slice("tool_call:".length) : ref;
}

export function toolActivityKey(tool: ToolActivity) {
  return canonicalToolRef(tool.ref || tool.id) || `${tool.message_id || "tool"}:${tool.index ?? ""}`;
}

export function messagePartKey(part: MessageRenderPart, ownerID: string) {
  if (part.type === "tool") return `${ownerID}:tool:${toolActivityKey(part.tool)}`;
  if (part.type === "ask") return `${ownerID}:ask:${part.ask.id}`;
  if (part.key) return `${ownerID}:${part.key}`;
  return `${ownerID}:${part.type}:${stableTextHash(part.content)}`;
}

export function nextRepeatedKey(seen: Map<string, number>, base: string) {
  const count = seen.get(base) || 0;
  seen.set(base, count + 1);
  return count === 0 ? base : `${base}:${count + 1}`;
}

function stableTextHash(value: string) {
  let hash = 5381;
  for (let index = 0; index < value.length; index += 1) {
    hash = ((hash << 5) + hash) ^ value.charCodeAt(index);
  }
  return (hash >>> 0).toString(36);
}

function textPartKey(event: ChatEvent, type: "reasoning" | "text") {
  if (event.message_id) {
    return [
      type,
      event.member_call_id || event.parent_tool_call_id || "",
      event.message_id,
    ].filter(Boolean).join(":");
  }
  return [
    type,
    event.member_call_id || event.parent_tool_call_id || "",
    event.message_id || "",
    event.stream_id || "",
    event.block_id || "",
    event.block_type || "",
  ].filter(Boolean).join(":") || `${type}:${eventDisplayKey(event)}`;
}

function isStreamEvent(event: ChatEvent) {
  return event.stream_event_id !== undefined;
}

function toolKey(tool: ToolCallDTO, event: ChatEvent) {
  return canonicalToolRef(tool.ref || event.tool_call_ref || tool.id || event.tool_call_id) || eventDisplayKey(event);
}

function toolIdentityKeys(value?: string) {
  if (!value) return [];
  const stripped = stripToolRef(value);
  return [value, stripped, `tool_call:${stripped}`];
}

function canonicalToolRef(value?: string) {
  if (!value) return "";
  const stripped = stripToolRef(value);
  return `tool_call:${stripped}`;
}

function appendText(left = "", right = "") {
  if (!right) return left;
  if (!left) return right;
  return left + right;
}

export function normalizeAgentKey(value: string) {
  return value.trim().toLowerCase();
}
