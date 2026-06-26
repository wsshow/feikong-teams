import { useEffect, useRef, useState } from "react";
import anime from "animejs";
import { Check, ChevronDown, Copy, GitBranch } from "lucide-react";
import { useAppSelector } from "@/app/hooks";
import { renderMarkdown } from "@/lib/markdown";
import { cn } from "@/lib/cn";
import { ToolCallCard } from "./ToolCallCard";
import type { ChatEvent, ToolCallDTO } from "@/types/events";

export function MessageList() {
  const messages = useAppSelector((state) => state.chat.messages);
  const events = useAppSelector((state) => state.chat.events);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const statusText = useAppSelector((state) => state.chat.statusText);
  const error = useAppSelector((state) => state.chat.error);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const previousMessageCountRef = useRef(0);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [messages, isProcessing, statusText, error, toolEventsKey(events)]);

  useEffect(() => {
    const previous = previousMessageCountRef.current;
    previousMessageCountRef.current = messages.length;
    if (messages.length <= previous) return;
    anime({
      targets: ".message-row:last-of-type",
      opacity: [0, 1],
      translateY: [8, 0],
      duration: 180,
      easing: "easeOutQuad",
    });
  }, [messages.length]);

  if (messages.length === 0 && events.length === 0 && !isProcessing && !error) {
    return <div className="min-h-0 flex-1" />;
  }

  const toolEvents = collectToolActivities(events);
  const memberEvents = collectMemberActivities(events);
  const reasoningByMessage = collectReasoningBlocks(events);

  return (
    <div className="chat-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-6 py-8">
      <div className="mx-auto w-full max-w-3xl space-y-10">
        {messages.map((message) => (
          <MessageRow key={message.id} message={message} reasoning={message.reasoningContent || reasoningByMessage.get(message.id)} />
        ))}
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
        {memberEvents.map((member) => (
          <div key={member.id} className="sketch-surface rounded-md px-4 py-3 text-sm">
            <div className="mb-2 flex items-center gap-2 text-muted-foreground">
              <GitBranch className="h-4 w-4" />
              <span className="font-medium text-foreground">{member.name}</span>
              <span className="text-xs">{member.eventCount} 个事件</span>
            </div>
            {member.preview ? <div className="prose prose-sm max-w-none" dangerouslySetInnerHTML={{ __html: renderMarkdown(member.preview) }} /> : null}
          </div>
        ))}
        {toolEvents.slice(-12).map((tool, index) => (
          <ToolCallCard key={`${tool.ref || tool.id || tool.name}-${index}`} tool={tool} />
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  );
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
  };
  reasoning?: string;
}) {
  if (message.role === "user") {
    return (
      <article className="message-row flex w-full flex-col items-end gap-3">
        <div className="max-w-[78%] rounded-2xl bg-muted px-5 py-4 text-lg leading-8 text-foreground">
          <div className="whitespace-pre-wrap">{message.content}</div>
        </div>
        <MessageActions align="right" content={message.content} />
      </article>
    );
  }

  return (
    <article className="message-row w-full">
      {message.agent ? <div className="mb-3 text-sm text-muted-foreground">{message.agent}</div> : null}
      {reasoning ? <ReasoningBlock content={reasoning} /> : null}
      <div
        className="prose message-prose max-w-none text-lg leading-9"
        dangerouslySetInnerHTML={{ __html: renderMarkdown(message.content) }}
      />
      <MessageActions content={message.content} />
    </article>
  );
}

function ReasoningBlock({ content }: { content: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mb-4 rounded-xl border border-border/70 bg-muted/35">
      <button
        className="flex w-full items-center justify-between gap-3 px-4 py-2.5 text-left text-sm text-muted-foreground"
        onClick={() => setOpen(!open)}
        type="button"
      >
        <span>模型思考</span>
        <ChevronDown className={cn("h-4 w-4 transition-transform", open && "rotate-180")} />
      </button>
      {open ? (
        <div className="border-t border-border/60 px-4 py-3 text-sm leading-7 text-muted-foreground">
          <div className="whitespace-pre-wrap">{content}</div>
        </div>
      ) : null}
    </div>
  );
}

function MessageActions({ content, align = "left" }: { content: string; align?: "left" | "right" }) {
  const [copied, setCopied] = useState(false);

  async function copyContent() {
    await navigator.clipboard?.writeText(content);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <div className={cn("flex items-center gap-2", align === "right" && "justify-end")}>
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

function collectReasoningBlocks(events: ChatEvent[]) {
  const blocks = new Map<string, string>();
  for (const event of events) {
    if (event.type !== "message_delta" || event.delta_kind !== "reasoning" || event.role === "tool") continue;
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

function collectToolActivities(events: ChatEvent[]): ToolCallDTO[] {
  const result = new Map<string, ToolCallDTO>();
  const order: string[] = [];
  const upsert = (key: string, patch: Partial<ToolCallDTO>) => {
    if (!result.has(key)) {
      result.set(key, {
        id: patch.id,
        ref: patch.ref,
        name: patch.name || "tool",
        status: "pending",
      });
      order.push(key);
    }
    Object.assign(result.get(key)!, patch);
  };

  for (const event of events) {
    for (const tool of event.tool_calls || []) {
      const key = toolKey(tool, event);
      upsert(key, {
        ...tool,
        ref: tool.ref || event.tool_call_ref,
        id: tool.id || event.tool_call_id,
        status: event.type === "message_end" ? "completed" : "pending",
        member_name: event.member_name || tool.member_name,
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
  const result = new Map<string, { id: string; name: string; eventCount: number; preview: string }>();
  for (const event of events) {
    if (!event.is_member_event && !event.member_call_id && !event.member_name) continue;
    const id = event.member_call_id || event.member_name || event.agent_name || "member";
    const current = result.get(id) || {
      id,
      name: event.member_name || event.agent_name || "子智能体",
      eventCount: 0,
      preview: "",
    };
    current.eventCount += 1;
    const content = event.delta_kind === "output" || event.type === "action" ? String(event.content || event.delta || "") : "";
    if (content) current.preview = appendText(current.preview, content);
    result.set(id, current);
  }
  return Array.from(result.values()).filter((member) => member.preview || member.eventCount > 1);
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
