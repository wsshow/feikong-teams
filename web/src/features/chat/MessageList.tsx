import { useEffect, useRef, useState } from "react";
import anime from "animejs";
import { Check, ChevronRight, Copy, GitBranch } from "lucide-react";
import { useAppSelector } from "@/app/hooks";
import { renderMarkdown } from "@/lib/markdown";
import { cn } from "@/lib/cn";
import { formatTime } from "@/lib/format";
import { ToolCallCard } from "./ToolCallCard";
import type { ChatEvent, ToolCallDTO } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";

type ToolActivity = ToolCallDTO & { message_id?: string };

export function MessageList() {
  const messages = useAppSelector((state) => state.chat.messages);
  const events = useAppSelector((state) => state.chat.events);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const statusText = useAppSelector((state) => state.chat.statusText);
  const error = useAppSelector((state) => state.chat.error);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const previousMessageCountRef = useRef(0);
  const displayEvents = eventsForDisplay(messages, events);
  const reasoningByMessage = collectReasoningBlocks(displayEvents);
  const toolEvents = collectToolActivities(displayEvents, { includeMemberEvents: false });
  const memberEvents = collectMemberActivities(displayEvents);
  const memberByCallID = new Map(memberEvents.map((member) => [member.id, member]));
  const memberByMessageID = mapMembersByMessageID(memberEvents);
  const timelineMessages = dedupeAdjacentSystemMessages(
    messages.filter((message) => shouldShowTimelineItem(message, reasoningByMessage, memberByMessageID)),
  );
  const nestedMemberIDs = new Set<string>();
  const renderedToolKeys = new Set<string>();
  const toolEventsByMessageID = groupToolsByMessageID(toolEvents);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [timelineMessages, isProcessing, statusText, error, toolEventsKey(displayEvents)]);

  useEffect(() => {
    const previous = previousMessageCountRef.current;
    previousMessageCountRef.current = timelineMessages.length;
    if (timelineMessages.length <= previous) return;
    anime({
      targets: ".message-row:last-of-type",
      opacity: [0, 1],
      translateY: [8, 0],
      duration: 180,
      easing: "easeOutQuad",
    });
  }, [timelineMessages.length]);

  if (timelineMessages.length === 0 && displayEvents.length === 0 && !isProcessing && !error) {
    return <div className="min-h-0 flex-1" />;
  }

  return (
    <div className="chat-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-6 py-8">
      <div className="mx-auto w-full max-w-4xl space-y-6">
        {timelineMessages.map((message) => {
          if (message.hidden) {
            const member = memberByMessageID.get(message.id);
            if (!member || nestedMemberIDs.has(member.id)) return null;
            nestedMemberIDs.add(member.id);
            return <MemberActivityBlock key={message.id} member={member} />;
          }
          const messageTools = toolEventsByMessageID.get(message.id) || [];
          return (
            <div key={message.id} className="space-y-3">
              <MessageRow message={message} reasoning={message.reasoningContent || reasoningByMessage.get(message.id)} />
              {messageTools.length ? (
                <ActivityList
                  tools={messageTools}
                  memberByCallID={memberByCallID}
                  nestedMemberIDs={nestedMemberIDs}
                  renderedToolKeys={renderedToolKeys}
                />
              ) : null}
            </div>
          );
        })}
        {isProcessing ? (
          <div className="message-row text-lg text-muted-foreground">
            <div>
              {statusText || "处理中"}
              <span className="ml-1 inline-flex w-8 justify-between align-middle">
                <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60" />
                <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/45" />
                <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30" />
              </span>
            </div>
          </div>
        ) : null}
        {error ? <div className="sketch-surface rounded-md border-destructive/50 px-4 py-3 text-sm text-destructive">{error}</div> : null}
        <ActivityList
          tools={toolEvents.filter((tool) => !renderedToolKeys.has(toolActivityKey(tool)))}
          memberByCallID={memberByCallID}
          nestedMemberIDs={nestedMemberIDs}
          renderedToolKeys={renderedToolKeys}
        />
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

export function chatMessageElementID(messageID: string) {
  return `chat-message-${messageID.replace(/[^a-zA-Z0-9_-]/g, "_")}`;
}

function MessageRow({
  message,
  reasoning,
}: {
  message: {
    id: string;
    role: "user" | "assistant" | "system" | "tool";
    agent?: string;
    content: string;
    reasoningContent?: string;
    createdAt?: string;
  };
  reasoning?: string;
}) {
  const hasContent = Boolean(message.content.trim());
  if (message.role === "system") {
    return (
      <article id={chatMessageElementID(message.id)} className="message-row w-full scroll-mt-8">
        <div className="mb-2 text-sm text-muted-foreground">系统</div>
        <div className="text-lg leading-9 text-muted-foreground">
          {message.content}
        </div>
      </article>
    );
  }

  if (message.role === "user") {
    return (
      <article id={chatMessageElementID(message.id)} className="message-row group flex w-full scroll-mt-8 flex-col items-end gap-2">
        <div className="max-w-[78%] rounded-2xl bg-muted px-5 py-4 text-lg leading-8 text-foreground">
          <div className="whitespace-pre-wrap">{message.content}</div>
        </div>
        <MessageActions align="right" content={message.content} time={message.createdAt} />
      </article>
    );
  }

  return (
    <article id={chatMessageElementID(message.id)} className="message-row group w-full scroll-mt-8">
      {message.agent ? <div className="mb-2 text-sm text-muted-foreground">{message.agent}</div> : null}
      {reasoning ? <ReasoningBlock content={reasoning} /> : null}
      {hasContent ? (
        <>
          <div
            className="prose message-prose w-full max-w-none text-lg leading-9"
            dangerouslySetInnerHTML={{ __html: renderMarkdown(message.content) }}
          />
          <MessageActions content={message.content} />
        </>
      ) : null}
    </article>
  );
}

function ReasoningBlock({ content }: { content: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mb-4 text-sm">
      <button
        className="flex items-center gap-3 rounded-lg px-2 py-2 text-left text-amber-600 transition-colors hover:bg-amber-50/70"
        onClick={() => setOpen(!open)}
        type="button"
      >
        <span className="h-2 w-2 rounded-full bg-amber-400" />
        <span className="font-semibold">已思考</span>
        <ChevronRight className={cn("h-4 w-4 transition-transform", open && "rotate-90")} />
      </button>
      {open ? (
        <div className="ml-7 border-l border-amber-200/70 pl-4 pt-2 text-sm leading-7 text-muted-foreground">
          <div className="whitespace-pre-wrap">{content}</div>
        </div>
      ) : null}
    </div>
  );
}

interface MemberActivity {
  id: string;
  name: string;
  eventCount: number;
  preview: string;
  reasoning: string;
  tools: ToolActivity[];
  messageIDs: string[];
}

function ActivityList({
  tools,
  memberByCallID,
  nestedMemberIDs,
  renderedToolKeys,
}: {
  tools: ToolActivity[];
  memberByCallID: Map<string, MemberActivity>;
  nestedMemberIDs: Set<string>;
  renderedToolKeys: Set<string>;
}) {
        const visibleTools = tools.slice(-12).filter((tool) => !renderedToolKeys.has(toolActivityKey(tool)));
  if (!visibleTools.length) return null;
  return (
    <div className="space-y-2">
      {visibleTools.map((tool, index) => {
        renderedToolKeys.add(toolActivityKey(tool));
        const member = memberByCallID.get(tool.id || "") || memberByCallID.get(stripToolRef(tool.ref || ""));
        if (member && nestedMemberIDs.has(member.id)) {
          return <ToolCallCard key={`${tool.ref || tool.id || tool.name}-${index}`} tool={tool} />;
        }
        if (member) nestedMemberIDs.add(member.id);
        return (
          <ToolCallCard key={`${tool.ref || tool.id || tool.name}-${index}`} tool={tool}>
            {member ? <MemberActivityDetails member={member} /> : null}
          </ToolCallCard>
        );
      })}
    </div>
  );
}

function MemberActivityBlock({ member }: { member: MemberActivity }) {
  const [open, setOpen] = useState(false);
  const title = member.name.toUpperCase();
  return (
    <div className="text-sm">
      <button
        className="flex items-center gap-3 rounded-lg px-2 py-2 text-left tracking-[0.12em] text-muted-foreground transition-colors hover:bg-muted/70"
        onClick={() => setOpen(!open)}
        type="button"
      >
        <span className="h-2 w-2 rounded-full bg-muted-foreground/35" />
        <span className="font-semibold">{title}</span>
        <ChevronRight className={cn("h-4 w-4 transition-transform", open && "rotate-90")} />
      </button>
      {open ? (
        <div className="ml-7 space-y-3 border-l border-border/60 pl-4 pt-2">
          <MemberActivityDetails member={member} />
        </div>
      ) : null}
    </div>
  );
}

function MemberActivityDetails({ member }: { member: MemberActivity }) {
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <GitBranch className="h-3.5 w-3.5" />
        <span>{member.name}</span>
        <span>{member.eventCount} 个事件</span>
      </div>
      {member.reasoning ? <ReasoningBlock content={member.reasoning} /> : null}
      {member.tools.map((tool, index) => (
        <ToolCallCard key={`${tool.ref || tool.id || tool.name}-${index}`} tool={tool} />
      ))}
      {member.preview ? (
        <div className="prose message-prose w-full max-w-none text-base leading-8" dangerouslySetInnerHTML={{ __html: renderMarkdown(member.preview) }} />
      ) : null}
    </div>
  );
}

function MessageActions({ content, align = "left", time }: { content: string; align?: "left" | "right"; time?: string }) {
  const [copied, setCopied] = useState(false);

  async function copyContent() {
    await navigator.clipboard?.writeText(content);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <div
      className={cn(
        "flex items-center gap-2 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100",
        copied && "opacity-100",
        align === "right" && "justify-end",
      )}
    >
      {time ? <span className="px-1 text-sm text-muted-foreground">{formatTime(time)}</span> : null}
      <button
        className={cn(
          "flex h-8 items-center gap-1.5 rounded-lg px-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
          copied && "bg-muted text-foreground",
        )}
        aria-label={copied ? "已复制" : "复制"}
        onClick={() => void copyContent()}
      >
        {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
        {copied ? <span className="text-xs">已复制</span> : null}
      </button>
    </div>
  );
}

function toolEventsKey(events: Array<{ tool_calls?: unknown[]; tool_call?: unknown; tool_call_ref?: string; type?: string }>) {
  return events
    .map((event) => `${event.type}:${event.tool_call_ref || ""}:${event.tool_calls?.length || 0}:${event.tool_call ? 1 : 0}`)
    .join(":");
}

function hasEventToolActivity(event: ChatEvent) {
  return Boolean(event.tool_calls?.length || event.tool_call || event.tool_name || event.tool_call_ref || event.tool_call_id);
}

function eventsForDisplay(messages: ChatViewMessage[], liveEvents: ChatEvent[]) {
  const seen = new Set<string>();
  const result: ChatEvent[] = [];
  const messageEvents = messages.flatMap((message) =>
    (message.events || []).map((event) => ({
      ...event,
      message_id: event.message_id || message.id,
    })),
  );
  for (const event of [...messageEvents, ...liveEvents]) {
    const key = `${event.event_id || ""}:${event.sequence || ""}:${event.type}:${event.tool_call_ref || ""}:${event.tool_call_id || ""}:${event.content || event.delta || ""}`;
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(event);
  }
  return result;
}

function shouldShowTimelineItem(
  message: ChatViewMessage,
  reasoningByMessage: Map<string, string>,
  memberByMessageID: Map<string, MemberActivity>,
) {
  if (message.hidden) return memberByMessageID.has(message.id);
  if (message.role === "user") return Boolean(message.content.trim());
  if (message.content.trim()) return true;
  if (message.reasoningContent?.trim()) return true;
  if (reasoningByMessage.get(message.id)?.trim()) return true;
  if (message.events.some((event) => hasEventToolActivity(event))) return true;
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

function collectReasoningBlocks(events: ChatEvent[]) {
  const blocks = new Map<string, string>();
  for (const event of events) {
    if (isMemberActivityEvent(event) || event.type !== "message_delta" || event.delta_kind !== "reasoning" || event.role === "tool") continue;
    const content = String(event.reasoning_content || event.content || event.delta || "");
    if (!content) continue;
    for (const key of reasoningKeys(event)) {
      blocks.set(key, appendText(blocks.get(key), content));
    }
  }
  return blocks;
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
  keys.add(`${event.agent_name || "assistant"}`);
  return keys;
}

function collectToolActivities(events: ChatEvent[], options: { includeMemberEvents?: boolean } = {}): ToolActivity[] {
  const result = new Map<string, ToolActivity>();
  const order: string[] = [];
  const upsert = (key: string, patch: Partial<ToolActivity>) => {
    if (!result.has(key)) {
      result.set(key, {
        id: patch.id,
        ref: patch.ref,
        name: patch.name || "tool",
        status: "pending",
        message_id: patch.message_id,
      });
      order.push(key);
    }
    Object.assign(result.get(key)!, patch);
  };

  for (const event of events) {
    if (!options.includeMemberEvents && isMemberActivityEvent(event)) continue;
    for (const tool of event.tool_calls || []) {
      const key = toolKey(tool, event);
      upsert(key, {
        ...tool,
        ref: tool.ref || event.tool_call_ref,
        id: tool.id || event.tool_call_id,
        status: event.type === "message_end" ? "completed" : "pending",
        member_name: event.member_name || tool.member_name,
        message_id: event.message_id,
      });
    }
    if (event.tool_call) {
      const key = toolKey(event.tool_call, event);
      upsert(key, {
        ...event.tool_call,
        ref: event.tool_call.ref || event.tool_call_ref,
        id: event.tool_call.id || event.tool_call_id,
        status: event.type === "message_end" ? "completed" : "pending",
        member_name: event.member_name || event.tool_call.member_name,
        message_id: event.message_id,
      });
    }
    if (event.tool_name || event.tool_call_ref || event.tool_call_id) {
      const key = event.tool_call_ref || event.tool_call_id || event.tool_name || "tool";
      const current = result.get(key);
      upsert(key, {
        ref: event.tool_call_ref,
        id: event.tool_call_id,
        name: event.tool_name || current?.name || "tool",
        display_name: event.tool_display_name || current?.display_name,
        kind: event.tool_kind || current?.kind,
        target: event.tool_target || current?.target,
        member_name: event.member_name || current?.member_name,
        status: event.type === "tool_end" ? "completed" : event.type === "error" ? "error" : current?.status || "running",
        message_id: event.message_id || current?.message_id,
      });
      const next = result.get(key)!;
      const content = String(event.tool_args || event.content || event.delta || "");
      if (event.delta_kind === "tool_args" && content) {
        next.arguments = appendText(next.arguments, content);
      }
      if ((event.type === "tool_start" || event.delta_kind === "tool_args") && content && !next.arguments) {
        next.arguments = content;
      }
      if ((event.type === "tool_update" || event.type === "tool_end" || event.delta_kind === "tool_result" || event.role === "tool") && content) {
        next.result = appendText(next.result, content);
      }
    }
  }
  return order.map((key) => result.get(key)!).filter((tool) => tool.name && tool.name !== "tool");
}

function collectMemberActivities(events: ChatEvent[]) {
  const grouped = new Map<string, ChatEvent[]>();
  for (const event of events) {
    if (!isMemberActivityEvent(event)) continue;
    const id = event.member_call_id || event.member_name || event.agent_name || "member";
    grouped.set(id, [...(grouped.get(id) || []), event]);
  }
  return Array.from(grouped.entries())
    .map(([id, memberEvents]) => {
      let preview = "";
      let reasoning = "";
      for (const event of memberEvents) {
        const content = String(event.content || event.delta || "");
        if (!content) continue;
        if (event.type === "message_delta" && event.delta_kind === "reasoning") reasoning = appendText(reasoning, content);
        if (event.type === "message_delta" && event.delta_kind === "output") preview = appendText(preview, content);
        if (event.type === "action" && event.action_type !== "ask_response") preview = appendText(preview, content);
      }
      return {
        id,
        name: memberEvents.find((event) => event.member_name)?.member_name || memberEvents[0]?.agent_name || "子智能体",
        eventCount: memberEvents.length,
        preview,
        reasoning,
        tools: collectToolActivities(memberEvents, { includeMemberEvents: true }),
        messageIDs: uniqueStrings(memberEvents.map((event) => event.message_id).filter(Boolean) as string[]),
      };
    })
    .filter((member) => member.preview || member.reasoning || member.tools.length || member.eventCount > 1);
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

function toolActivityKey(tool: ToolActivity) {
  return tool.ref || tool.id || `${tool.message_id || ""}:${tool.name}:${tool.index ?? ""}`;
}

function toolKey(tool: ToolCallDTO, event: ChatEvent) {
  return tool.ref || event.tool_call_ref || tool.id || event.tool_call_id || `${tool.name}:${tool.index ?? ""}`;
}

function appendText(left = "", right = "") {
  if (!right) return left;
  if (!left) return right;
  if (left.includes(right)) return left;
  return left + right;
}
