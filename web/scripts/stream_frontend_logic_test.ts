#!/usr/bin/env bun

export {};

type ToolCall = {
  id?: string;
  ref?: string;
  name?: string;
  kind?: string;
  target?: string;
};

type StreamEvent = {
  type?: string;
  stream_event_id?: number;
  sequence?: number;
  session_id?: string;
  message?: string;
  content?: string;
  error?: string;
  role?: string;
  message_id?: string;
  stream_id?: string;
  block_id?: string;
  block_type?: string;
  reasoning_content?: string;
  event_id?: string;
  is_member_event?: boolean;
  member_call_id?: string;
  member_name?: string;
  member_tool_name?: string;
  agent_name?: string;
  parent_tool_call_id?: string;
  tool_kind?: string;
  tool_name?: string;
  tool_call_id?: string;
  tool_call_ref?: string;
  tool_calls?: ToolCall[];
  tool_call?: ToolCall;
};

type FrontendState = {
  events: StreamEvent[];
  messages: unknown[];
  isProcessing: boolean;
  runningSessionID: string;
  statusText: string;
  error: string;
};

type MemberSummary = {
  id: string;
  name: string;
  count: number;
  toolRefs: string[];
  missingToolRef: number;
  completed: boolean;
  parentMatched: boolean;
  firstStreamEventID?: number;
  lastStreamEventID?: number;
};

type RenderPart = {
  type: "reasoning" | "text" | "tool" | "ignored";
  key: string;
  message_id?: string;
  start?: number;
  kind?: string;
};

type AnalysisReport = {
  totalEvents: number;
  terminalEvents: string[];
  isProcessing: boolean;
  error: string;
  parentAgentTools: ToolCall[];
  members: MemberSummary[];
  memberToolEventCount: number;
  memberPartIssues: Array<Record<string, unknown>>;
  memberTextMismatches: Array<Record<string, unknown>>;
  unscopedToolResults: Array<Record<string, unknown>>;
};

type APIResponse<T> = {
  code?: number;
  data?: T;
};

type StreamStartResponse = {
  session_id?: string;
  status?: string;
};

const baseURL = process.env.FKTEAMS_BASE_URL || "http://127.0.0.1:23456";
const timeoutMs = Number(process.env.FKTEAMS_STREAM_TEST_TIMEOUT_MS || 180000);
const prompt = process.env.FKTEAMS_STREAM_TEST_PROMPT ||
  "使用两个子智能体分别搜索 AI 和科技方面最近的重要消息。每个子智能体只返回 2 条，最终汇总要简短。";

const state: FrontendState = {
  events: [],
  messages: [],
  isProcessing: false,
  runningSessionID: "",
  statusText: "",
  error: "",
};

const controller = new AbortController();
const timeout = setTimeout(() => controller.abort(new Error(`stream test timeout after ${timeoutMs}ms`)), timeoutMs);

try {
  const start = await post<StreamStartResponse>("/api/fkteams/stream/start", {
    message: prompt,
    mode: "team",
  });
  if (!start.session_id) throw new Error(`missing session_id in start response: ${JSON.stringify(start)}`);
  if (start.status !== "processing") throw new Error(`stream did not start processing: ${JSON.stringify(start)}`);

  console.log(`session_id=${start.session_id}`);
  await subscribeUntilTerminal(start.session_id, 0, (event) => {
    receiveEvent(event);
    if (event.stream_event_id !== undefined && Number(event.stream_event_id) % 100 === 0) {
      process.stdout.write(".");
    }
  }, controller.signal);
  clearTimeout(timeout);
  console.log("");

  const report = analyzeEvents(state.events, state);
  printReport(report);
  assertReport(report);
  console.log("STREAM_FRONTEND_LOGIC_TEST_OK");
} catch (error) {
  clearTimeout(timeout);
  console.error("");
  console.error("STREAM_FRONTEND_LOGIC_TEST_FAILED");
  console.error(error instanceof Error ? error.stack || error.message : error);
  process.exitCode = 1;
}

async function post<T = unknown>(path: string, body: Record<string, unknown>) {
  const response = await fetch(`${baseURL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const payload = await response.json() as APIResponse<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(`POST ${path} failed: HTTP ${response.status} ${JSON.stringify(payload)}`);
  }
  if (payload.data === undefined) {
    throw new Error(`POST ${path} returned empty data: ${JSON.stringify(payload)}`);
  }
  return payload.data;
}

async function subscribeUntilTerminal(
  sessionID: string,
  initialOffset: number,
  onEvent: (event: StreamEvent) => void,
  signal: AbortSignal,
) {
  let offset = initialOffset;
  for (;;) {
    await subscribeStream(sessionID, offset, (event) => {
      if (event.stream_event_id !== undefined) {
        offset = Math.max(offset, Number(event.stream_event_id) + 1);
      }
      onEvent(event);
    }, signal);
    if (state.events.some((event) => event.type === "processing_end" || event.type === "cancelled" || event.type === "error")) {
      return;
    }
    await sleep(250);
  }
}

async function subscribeStream(
  sessionID: string,
  offset: number,
  onEvent: (event: StreamEvent) => void,
  signal: AbortSignal,
) {
  const response = await fetch(
    `${baseURL}/api/fkteams/stream/subscribe/${encodeURIComponent(sessionID)}?offset=${encodeURIComponent(String(offset))}`,
    { signal },
  );
  if (!response.ok || !response.body) throw new Error(`stream subscribe failed: HTTP ${response.status}`);
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const chunks = buffer.split("\n\n");
      buffer = chunks.pop() || "";
      for (const chunk of chunks) {
        const lines = chunk.split("\n");
        const idLine = lines.find((part) => part.startsWith("id:"));
        const dataLines = lines.filter((part) => part.startsWith("data:"));
        if (dataLines.length === 0) continue;
        const raw = dataLines.map((line) => line.replace(/^data:\s*/, "")).join("\n");
        if (!raw || raw === "[DONE]") continue;
        const event = JSON.parse(raw) as StreamEvent;
        if (idLine && event.stream_event_id === undefined) {
          const id = Number(idLine.replace(/^id:\s*/, ""));
          if (Number.isFinite(id)) event.stream_event_id = id;
        }
        onEvent(event);
      }
    }
  } catch (error) {
    if (!isSSECloseError(error)) throw error;
  }
}

function isSSECloseError(error: unknown) {
  return error instanceof TypeError && String(error.message || "").includes("terminated");
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function receiveEvent(event: StreamEvent) {
  state.events.push(event);
  if (event.type === "processing_start") {
    state.isProcessing = true;
    if (event.session_id) state.runningSessionID = event.session_id;
    state.statusText = String(event.message || event.content || "处理中");
  }
  if (event.type === "error") {
    state.error = String(event.error || event.content || event.message || "request failed");
    state.isProcessing = false;
    state.runningSessionID = "";
    state.statusText = "";
  }
  if (event.type === "cancelled" || event.type === "processing_end") {
    state.isProcessing = false;
    state.runningSessionID = "";
    state.statusText = String(event.message || event.content || "");
  }
}

function analyzeEvents(events: StreamEvent[], state: FrontendState): AnalysisReport {
  const parentAgentTools: ToolCall[] = [];
  const memberEvents = new Map<string, {
    id: string;
    name: string;
    count: number;
    toolRefs: Set<string>;
    missingToolRef: number;
    completed: boolean;
    firstStreamEventID?: number;
    lastStreamEventID?: number;
  }>();
  const memberToolEvents: StreamEvent[] = [];
  const unscopedToolResults: StreamEvent[] = [];
  const terminalEvents: StreamEvent[] = [];
  const completedMemberIDs = completedMemberCallIDs(events);
  const memberPartIssues = analyzeMemberRenderParts(events);
  const memberTextMismatches = analyzeMemberCompletedText(events);

  for (const event of events) {
    if (event.type === "processing_end" || event.type === "cancelled" || event.type === "error") terminalEvents.push(event);
    for (const tool of event.tool_calls || []) {
      if (tool.kind === "agent" || tool.name?.startsWith("ask_fkagent_")) {
        parentAgentTools.push({
          id: tool.id || event.tool_call_id,
          ref: tool.ref || event.tool_call_ref,
          name: tool.name,
          target: tool.target,
        });
      }
    }
    if (event.tool_call && (event.tool_call.kind === "agent" || event.tool_call.name?.startsWith("ask_fkagent_"))) {
      parentAgentTools.push({
        id: event.tool_call.id || event.tool_call_id,
        ref: event.tool_call.ref || event.tool_call_ref,
        name: event.tool_call.name,
        target: event.tool_call.target,
      });
    }
    if (isMemberActivityEvent(event)) {
      const id = memberActivityID(event);
      if (!memberEvents.has(id)) {
        memberEvents.set(id, {
          id,
          name: event.member_name || event.agent_name || "子智能体",
          count: 0,
          toolRefs: new Set(),
          missingToolRef: 0,
          completed: false,
          firstStreamEventID: event.stream_event_id,
          lastStreamEventID: event.stream_event_id,
        });
      }
      const member = memberEvents.get(id);
      if (!member) throw new Error(`missing member state for ${id}`);
      member.count += 1;
      member.lastStreamEventID = event.stream_event_id;
      if (event.type === "agent_completed") member.completed = true;
      if (hasToolActivity(event)) {
        memberToolEvents.push(event);
        const refs = toolRefsFromEvent(event);
        if (refs.length) {
          for (const ref of refs) member.toolRefs.add(ref);
        } else {
          member.missingToolRef += 1;
        }
      }
    }
    if ((event.type === "tool_call_completed" || event.type === "tool_call_result_delta") && !event.member_call_id) {
      const name = event.tool_name || event.tool_call?.name || "";
      const isParentAgentResult = name.startsWith("ask_fkagent_") || event.tool_kind === "agent";
      if (!isParentAgentResult) unscopedToolResults.push(event);
    }
  }

  const parentIDs = unique(parentAgentTools.flatMap((tool) => toolIdentityKeys(tool.id || tool.ref)));
  const members = [...memberEvents.values()].map((member) => ({
    ...member,
    completed: member.completed || completedMemberIDs.has(stripToolRef(member.id)),
    parentMatched: parentIDs.includes(stripToolRef(member.id)),
    toolRefs: [...member.toolRefs],
  }));

  return {
    totalEvents: events.length,
    terminalEvents: terminalEvents.map((event) => event.type).filter((type): type is string => Boolean(type)),
    isProcessing: state.isProcessing,
    error: state.error,
    parentAgentTools: dedupeTools(parentAgentTools),
    members,
    memberToolEventCount: memberToolEvents.length,
    memberPartIssues,
    memberTextMismatches,
    unscopedToolResults: unscopedToolResults.map((event) => ({
      type: event.type,
      stream_event_id: event.stream_event_id,
      sequence: event.sequence,
      tool_name: event.tool_name || event.tool_call?.name,
      tool_call_id: event.tool_call_id || event.tool_call?.id,
      tool_call_ref: event.tool_call_ref || event.tool_call?.ref,
    })),
  };
}

function completedMemberCallIDs(events: StreamEvent[]) {
  const result = new Set<string>();
  for (const event of events) {
    if (event.type !== "tool_call_completed") continue;
    if (isAgentDispatchEvent(event)) {
      for (const key of toolIdentityKeys(event.tool_call_ref || event.tool_call_id)) result.add(stripToolRef(key));
    }
    for (const tool of event.tool_calls || []) {
      if (!isAgentDispatchTool(tool)) continue;
      for (const key of toolIdentityKeys(tool.ref || tool.id)) result.add(stripToolRef(key));
    }
    if (event.tool_call && isAgentDispatchTool(event.tool_call)) {
      for (const key of toolIdentityKeys(event.tool_call.ref || event.tool_call.id)) result.add(stripToolRef(key));
    }
  }
  return result;
}

function toolRefsFromEvent(event: StreamEvent) {
  const refs: string[] = [];
  for (const tool of event.tool_calls || []) {
    const ref = canonicalToolRef(tool.ref || tool.id);
    if (ref) refs.push(ref);
  }
  if (event.tool_call) {
    const ref = canonicalToolRef(event.tool_call.ref || event.tool_call.id);
    if (ref) refs.push(ref);
  }
  const topLevelRef = canonicalToolRef(event.tool_call_ref || event.tool_call_id);
  if (topLevelRef) refs.push(topLevelRef);
  return unique(refs);
}

function printReport(report: AnalysisReport) {
  console.log(JSON.stringify({
    totalEvents: report.totalEvents,
    terminalEvents: report.terminalEvents,
    isProcessing: report.isProcessing,
    parentAgentToolCount: report.parentAgentTools.length,
    memberCount: report.members.length,
    memberToolEventCount: report.memberToolEventCount,
    memberPartIssueCount: report.memberPartIssues.length,
    memberTextMismatchCount: report.memberTextMismatches.length,
    unscopedToolResultCount: report.unscopedToolResults.length,
    members: report.members.map((member) => ({
      id: member.id,
      name: member.name,
      count: member.count,
      completed: member.completed,
      parentMatched: member.parentMatched,
      toolRefCount: member.toolRefs.length,
      missingToolRef: member.missingToolRef,
      firstStreamEventID: member.firstStreamEventID,
      lastStreamEventID: member.lastStreamEventID,
    })),
    unscopedToolResults: report.unscopedToolResults.slice(0, 10),
    memberPartIssues: report.memberPartIssues.slice(0, 10),
    memberTextMismatches: report.memberTextMismatches.slice(0, 10),
  }, null, 2));
}

function assertReport(report: AnalysisReport) {
  const failures: string[] = [];
  if (!report.terminalEvents.includes("processing_end")) failures.push("missing processing_end");
  if (report.isProcessing) failures.push("frontend reducer still thinks stream is processing");
  if (report.error) failures.push(`stream reported error: ${report.error}`);
  if (report.parentAgentTools.length < 2) failures.push(`expected at least 2 parent agent tools, got ${report.parentAgentTools.length}`);
  if (report.members.length < 2) failures.push(`expected at least 2 member groups, got ${report.members.length}`);
  for (const member of report.members) {
    if (!member.parentMatched) failures.push(`member ${member.id} does not match a parent agent tool call`);
    if (!member.completed) failures.push(`member ${member.id} did not receive agent_completed`);
    if (member.missingToolRef > 0) failures.push(`member ${member.id} has ${member.missingToolRef} tool events missing tool_call_ref/tool_call_id`);
  }
  if (report.unscopedToolResults.length > 0) failures.push(`found ${report.unscopedToolResults.length} unscoped non-agent tool result events`);
  if (report.memberPartIssues.length > 0) failures.push(`found ${report.memberPartIssues.length} member render part issues`);
  if (report.memberTextMismatches.length > 0) failures.push(`found ${report.memberTextMismatches.length} member text delta/completed mismatches`);
  if (failures.length) throw new Error(failures.join("; "));
}

function analyzeMemberCompletedText(events: StreamEvent[]) {
  const byKey = new Map<string, {
    member_id: string;
    message_id: string;
    text_delta: string;
    reasoning_delta: string;
    completed_text: string;
    completed_reasoning: string;
  }>();
  for (const event of events) {
    if (!isMemberActivityEvent(event) || !event.message_id || event.role === "tool") continue;
    const memberID = memberActivityID(event);
    const key = `${memberID}:${event.message_id}`;
    if (!byKey.has(key)) {
      byKey.set(key, {
        member_id: memberID,
        message_id: event.message_id,
        text_delta: "",
        reasoning_delta: "",
        completed_text: "",
        completed_reasoning: "",
      });
    }
    const item = byKey.get(key);
    if (!item) throw new Error(`missing member text state for ${key}`);
    if (event.type === "assistant_text_delta") item.text_delta += String(event.content || "");
    if (event.type === "assistant_reasoning_delta") item.reasoning_delta += String(event.reasoning_content || event.content || "");
    if (event.type === "assistant_completed") {
      item.completed_text = String(event.content || "");
      item.completed_reasoning = String(event.reasoning_content || "");
    }
  }
  const mismatches: Array<Record<string, unknown>> = [];
  for (const item of byKey.values()) {
    if (item.text_delta && item.completed_text && item.text_delta !== item.completed_text) {
      mismatches.push({
        kind: "text",
        member_id: item.member_id,
        message_id: item.message_id,
        delta_len: item.text_delta.length,
        completed_len: item.completed_text.length,
        delta_sample: item.text_delta.slice(0, 120),
        completed_sample: item.completed_text.slice(0, 120),
      });
    }
    if (item.reasoning_delta && item.completed_reasoning && item.reasoning_delta !== item.completed_reasoning) {
      mismatches.push({
        kind: "reasoning",
        member_id: item.member_id,
        message_id: item.message_id,
        delta_len: item.reasoning_delta.length,
        completed_len: item.completed_reasoning.length,
        delta_sample: item.reasoning_delta.slice(0, 120),
        completed_sample: item.completed_reasoning.slice(0, 120),
      });
    }
  }
  return mismatches;
}

function analyzeMemberRenderParts(events: StreamEvent[]) {
  const groups = new Map<string, StreamEvent[]>();
  for (const event of events) {
    if (!isMemberActivityEvent(event)) continue;
    const id = memberActivityID(event);
    if (!groups.has(id)) groups.set(id, []);
    groups.get(id)?.push(event);
  }
  const issues: Array<Record<string, unknown>> = [];
  for (const [memberID, memberEvents] of groups) {
    const parts = memberRenderParts(memberEvents);
    for (let index = 1; index < parts.length; index += 1) {
      const previous = parts[index - 1];
      const current = parts[index];
      if (previous?.type === "reasoning" && current?.type === "reasoning") {
        issues.push({
          member_id: memberID,
          kind: "adjacent_reasoning",
          previous,
          current,
        });
      }
    }
    for (const part of parts) {
      if (part.kind === "completed_text_after_delta" || part.kind === "completed_reasoning_after_delta") {
        issues.push({ member_id: memberID, ...part });
      }
    }
  }
  return issues;
}

function memberRenderParts(events: StreamEvent[]) {
  const sorted = events.slice().sort((left, right) => eventOrder(left) - eventOrder(right));
  const parts: RenderPart[] = [];
  const renderedTools = new Set<string>();
  const seenReasoningDeltaKeys = new Set<string>();
  const seenTextDeltaKeys = new Set<string>();
  let reasoningOpen = false;
  let textOpen = false;
  for (const event of sorted) {
    if (event.type === "assistant_reasoning_delta" && event.role !== "tool") {
      const key = textPartKey(event, "reasoning");
      appendRenderPart(parts, { type: "reasoning", key, message_id: event.message_id, start: event.stream_event_id }, reasoningOpen);
      seenReasoningDeltaKeys.add(key);
      reasoningOpen = true;
      textOpen = false;
      continue;
    }
    reasoningOpen = false;
    if (event.type === "assistant_text_delta" && event.role !== "tool") {
      const key = textPartKey(event, "text");
      appendRenderPart(parts, { type: "text", key, message_id: event.message_id, start: event.stream_event_id }, textOpen);
      seenTextDeltaKeys.add(key);
      textOpen = true;
      continue;
    }
    textOpen = false;
    if (hasToolActivity(event)) {
      const keys = toolRefsFromEvent(event);
      const key = keys[0] || event.tool_name || event.tool_call?.name || event.tool_calls?.[0]?.name || "";
      if (key && !renderedTools.has(key)) {
        renderedTools.add(key);
        parts.push({ type: "tool", key, message_id: event.message_id, start: event.stream_event_id });
      }
    }
    if (event.type === "assistant_completed") {
      const reasoningKey = textPartKey(event, "reasoning");
      if (String(event.reasoning_content || "").trim()) {
        if (seenReasoningDeltaKeys.has(reasoningKey)) {
          parts.push({ type: "ignored", kind: "completed_reasoning_after_delta", key: reasoningKey, message_id: event.message_id, start: event.stream_event_id });
        } else {
          parts.push({ type: "reasoning", key: reasoningKey, message_id: event.message_id, start: event.stream_event_id });
        }
      }
      const textKey = textPartKey(event, "text");
      if (String(event.content || "").trim()) {
        if (seenTextDeltaKeys.has(textKey)) {
          parts.push({ type: "ignored", kind: "completed_text_after_delta", key: textKey, message_id: event.message_id, start: event.stream_event_id });
        } else {
          parts.push({ type: "text", key: textKey, message_id: event.message_id, start: event.stream_event_id });
        }
      }
    }
  }
  return parts.filter((part) => part.type !== "ignored");
}

function appendRenderPart(parts: RenderPart[], part: RenderPart, mergeWithPrevious: boolean) {
  const previous = parts[parts.length - 1];
  if (mergeWithPrevious && previous?.type === part.type && previous.key === part.key) {
    return;
  }
  parts.push(part);
}

function eventOrder(event: StreamEvent) {
  const streamEventID = Number(event.stream_event_id);
  if (Number.isFinite(streamEventID)) return streamEventID;
  const sequence = Number(event.sequence);
  if (Number.isFinite(sequence)) return sequence;
  return Number.MAX_SAFE_INTEGER;
}

function textPartKey(event: StreamEvent, type: "reasoning" | "text") {
  if (event.message_id) {
    return [type, event.member_call_id || event.parent_tool_call_id || "", event.message_id].filter(Boolean).join(":");
  }
  return [
    type,
    event.member_call_id || event.parent_tool_call_id || "",
    event.message_id || "",
    event.stream_id || "",
    event.block_id || "",
    event.block_type || "",
  ].filter(Boolean).join(":") || `${type}:${event.type}:${event.sequence || event.stream_event_id || ""}`;
}

function isMemberActivityEvent(event: StreamEvent) {
  return Boolean(event.is_member_event || event.member_call_id || event.member_name || event.member_tool_name || event.parent_tool_call_id);
}

function memberActivityID(event: StreamEvent) {
  if (event.member_call_id) return event.member_call_id;
  if (event.parent_tool_call_id) return event.parent_tool_call_id;
  if (event.message_id) return event.message_id;
  if (event.stream_id) return event.stream_id;
  return `${event.type}:${event.event_id || event.sequence || event.stream_event_id || ""}`;
}

function hasToolActivity(event: StreamEvent) {
  return Boolean(event.tool_calls?.length || event.tool_call || event.tool_name || event.tool_call_ref || event.tool_call_id);
}

function isAgentDispatchEvent(event: StreamEvent) {
  return event.tool_kind === "agent" || Boolean(event.tool_name?.startsWith("ask_fkagent_"));
}

function isAgentDispatchTool(tool: ToolCall) {
  return tool.kind === "agent" || Boolean(tool.name?.startsWith("ask_fkagent_"));
}

function stripToolRef(ref = "") {
  return String(ref).startsWith("tool_call:") ? String(ref).slice("tool_call:".length) : String(ref);
}

function canonicalToolRef(value?: string) {
  if (!value) return "";
  return `tool_call:${stripToolRef(value)}`;
}

function toolIdentityKeys(value?: string) {
  if (!value) return [];
  const stripped = stripToolRef(value);
  return [String(value), stripped, `tool_call:${stripped}`];
}

function unique<T>(values: T[]) {
  return [...new Set(values.filter(Boolean))];
}

function dedupeTools(tools: ToolCall[]) {
  const seen = new Set();
  const result = [];
  for (const tool of tools) {
    const key = canonicalToolRef(tool.ref || tool.id) || tool.name;
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(tool);
  }
  return result;
}
