import { useEffect, useMemo, useRef, useState } from "react";
import anime from "animejs";
import { ArrowDown, Check, ChevronRight, CircleHelp, Copy, GitBranch, Send } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions } from "@/app/store";
import { submitAskResponse } from "@/api/stream";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/cn";
import { formatTime } from "@/lib/format";
import { ToolCallCard } from "./ToolCallCard";
import type { ChatEvent, ToolCallDTO } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";
import type { AgentInfo } from "@/types/api";

type ToolActivity = ToolCallDTO & { message_id?: string };
interface AskActivity {
  id: string;
  sessionID?: string;
  messageID?: string;
  sequence?: number;
  question: string;
  options: string[];
  multiSelect: boolean;
  selected: string[];
  freeText: string;
  answered: boolean;
  memberName?: string;
  toolName?: string;
}

type MessageRenderPart =
  | { type: "reasoning"; content: string }
  | { type: "text"; content: string }
  | { type: "ask"; ask: AskActivity }
  | { type: "tool"; tool: ToolActivity; member?: MemberActivity };

interface TimelineMessageNode {
  message: ChatViewMessage;
  parts: MessageRenderPart[];
  showAgentLabel: boolean;
  order: number;
}

type TimelineItem =
  | { kind: "message"; node: TimelineMessageNode; order: number }
  | { kind: "member"; member: MemberActivity; order: number };

interface TimelineModel {
  items: TimelineItem[];
  messages: ChatViewMessage[];
  trailingAsks: AskActivity[];
  trailingTools: MessageRenderPart[];
}

export function MessageList() {
  const dispatch = useAppDispatch();
  const messages = useAppSelector((state) => state.chat.messages);
  const events = useAppSelector((state) => state.chat.events);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const statusText = useAppSelector((state) => state.chat.statusText);
  const error = useAppSelector((state) => state.chat.error);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const agents = useAppSelector((state) => state.app.agents);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const stickToBottomRef = useRef(true);
  const previousMessageCountRef = useRef(0);
  const [submittedAskIDs, setSubmittedAskIDs] = useState<Set<string>>(() => new Set());
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);
  const displayEvents = useMemo(() => eventsForDisplay(events), [events]);
  const timeline = useMemo(
    () => buildTimelineModel(messages, displayEvents, submittedAskIDs),
    [messages, displayEvents, submittedAskIDs],
  );

  useEffect(() => {
    if (!stickToBottomRef.current) return;
    scrollToBottom();
  }, [timeline.items, timeline.trailingAsks.length, isProcessing, statusText, error, toolEventsKey(displayEvents)]);

  useEffect(() => {
    stickToBottomRef.current = true;
    setShowJumpToBottom(false);
    requestAnimationFrame(scrollToBottom);
  }, [activeSessionID]);

  useEffect(() => {
    const previous = previousMessageCountRef.current;
    previousMessageCountRef.current = timeline.messages.length;
    if (timeline.messages.length <= previous) return;
    anime({
      targets: ".message-row:last-of-type",
      opacity: [0, 1],
      translateY: [8, 0],
      duration: 180,
      easing: "easeOutQuad",
    });
  }, [timeline.messages.length]);

  if (timeline.messages.length === 0 && displayEvents.length === 0 && !isProcessing && !error) {
    return <div className="min-h-0 flex-1" />;
  }

  function scrollToBottom() {
    const scroll = scrollRef.current;
    if (!scroll) return;
    scroll.scrollTop = scroll.scrollHeight;
  }

  function handleScroll() {
    const scroll = scrollRef.current;
    if (!scroll) return;
    const atBottom = isNearScrollBottom(scroll);
    stickToBottomRef.current = atBottom;
    setShowJumpToBottom(!atBottom);
  }

  function jumpToBottom() {
    stickToBottomRef.current = true;
    setShowJumpToBottom(false);
    scrollToBottom();
  }

  return (
    <div className="relative min-h-0 flex-1">
      <div ref={scrollRef} className="chat-scroll chat-thread-scroll h-full overflow-x-hidden overflow-y-auto px-6 py-8" onScroll={handleScroll}>
        <div className="mx-auto w-full max-w-4xl space-y-6">
          {timeline.items.map((item) => {
            if (item.kind === "member") {
              const member = item.member;
              return <MemberActivityBlock key={`member-${member.id}`} member={member} agents={agents} />;
            }
            const { message, parts, showAgentLabel } = item.node;
            if (message.hidden) return null;
            return (
              <div key={message.id} className="space-y-3">
                <MessageRow
                  message={message}
                  parts={parts}
                  sessionID={activeSessionID}
                  agents={agents}
                  showAgentLabel={showAgentLabel}
                  onAskAnswered={(ask, selected, freeText) => {
                    setSubmittedAskIDs((previous) => new Set(previous).add(ask.id));
                    dispatch(chatActions.receiveEvent({
                      type: "ask_answered",
                      session_id: ask.sessionID || activeSessionID,
                      ask_id: ask.id,
                      detail: ask.id,
                      selected,
                      free_text: freeText,
                      content: askResponseSummary(selected, freeText),
                    }));
                  }}
                />
              </div>
            );
          })}
          {timeline.trailingAsks.map((ask) => (
            <AskTimelineItem
              key={ask.id}
              ask={ask}
              sessionID={ask.sessionID || activeSessionID}
              onAnswered={(selected, freeText) => {
                setSubmittedAskIDs((previous) => new Set(previous).add(ask.id));
                dispatch(chatActions.receiveEvent({
                  type: "ask_answered",
                  session_id: ask.sessionID || activeSessionID,
                  ask_id: ask.id,
                  detail: ask.id,
                  selected,
                  free_text: freeText,
                  content: askResponseSummary(selected, freeText),
                }));
              }}
            />
          ))}
          <ActivityList tools={timeline.trailingTools} agents={agents} />
          {isProcessing ? (
            <div className="message-row text-lg text-muted-foreground">
              <div>
                {loadingStatusText(statusText)}
                <span className="ml-1 inline-flex w-8 justify-between align-middle">
                  <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60" />
                  <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/45" />
                  <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30" />
                </span>
              </div>
            </div>
          ) : null}
          {error ? <div className="sketch-surface rounded-md border-destructive/50 px-4 py-3 text-sm text-destructive">{error}</div> : null}
          <div ref={bottomRef} />
        </div>
      </div>
      {showJumpToBottom ? (
        <button
          className="absolute bottom-4 left-1/2 z-20 flex h-9 -translate-x-1/2 items-center gap-2 rounded-full border border-border bg-card/95 px-3 text-sm font-medium text-muted-foreground shadow-[0_8px_24px_hsl(218_30%_25%/0.14)] backdrop-blur transition-colors hover:bg-accent hover:text-foreground"
          type="button"
          onClick={jumpToBottom}
        >
          <ArrowDown className="h-4 w-4" />
          回到底部
        </button>
      ) : null}
    </div>
  );
}

function isNearScrollBottom(element: HTMLElement) {
  return element.scrollHeight - element.scrollTop - element.clientHeight < 48;
}

function loadingStatusText(statusText?: string) {
  if (!statusText || statusText === "处理中" || statusText === "开始处理您的请求...") return "思考中...";
  return statusText;
}

export function chatMessageElementID(messageID: string) {
  return `chat-message-${messageID.replace(/[^a-zA-Z0-9_-]/g, "_")}`;
}

function MessageRow({
  message,
  parts,
  sessionID,
  agents,
  showAgentLabel,
  onAskAnswered,
}: {
  message: ChatViewMessage;
  parts: MessageRenderPart[];
  sessionID?: string;
  agents: AgentInfo[];
  showAgentLabel: boolean;
  onAskAnswered: (ask: AskActivity, selected: string[], freeText: string) => void;
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

  const textParts = parts.filter((part): part is { type: "text"; content: string } => part.type === "text");
  const copyContent = textParts.map((part) => part.content).join("");

  return (
    <article id={chatMessageElementID(message.id)} className="message-row group w-full scroll-mt-8">
      {message.agent && showAgentLabel ? (
        <div className="mb-2 text-sm text-muted-foreground">
          <AgentNameLabel name={message.agent} agent={resolveAgentInfo(message.agent, agents)} />
        </div>
      ) : null}
      <div className="space-y-3">
        {parts.map((part, index) => (
          <MessagePart
            key={`${message.id}-part-${index}`}
            part={part}
            sessionID={sessionID}
            agents={agents}
            onAskAnswered={(selected, freeText) => {
              if (part.type === "ask") onAskAnswered(part.ask, selected, freeText);
            }}
          />
        ))}
      </div>
      {hasContent || copyContent ? <MessageActions content={copyContent || message.content} /> : null}
    </article>
  );
}

function MessagePart({
  part,
  sessionID,
  agents,
  onAskAnswered,
}: {
  part: MessageRenderPart;
  sessionID?: string;
  agents: AgentInfo[];
  onAskAnswered: (selected: string[], freeText: string) => void;
}) {
  if (part.type === "reasoning") return <ReasoningBlock content={part.content} />;
  if (part.type === "tool") {
    return (
      <ToolCallCard tool={part.tool}>
        {part.member ? <MemberActivityDetails member={part.member} agents={agents} /> : null}
      </ToolCallCard>
    );
  }
  if (part.type === "ask") {
    return (
      <div>
        <AskTimelineItem ask={part.ask} sessionID={part.ask.sessionID || sessionID} onAnswered={onAskAnswered} />
      </div>
    );
  }
  return <MarkdownContent className="text-lg leading-9" content={part.content} />;
}

function ReasoningBlock({ content }: { content: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="-ml-2 text-sm">
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

function assistantMessageParts(
  message: ChatViewMessage,
  asks: AskActivity[],
  tools: ToolActivity[],
  fallbackReasoningBlocks?: string[],
) {
  const parts: MessageRenderPart[] = [];
  const renderedTools = new Set<string>();
  for (const event of message.events || []) {
    if (isReasoningDelta(event) && event.role !== "tool") {
      appendMessagePart(parts, "reasoning", String(event.reasoning_content || event.content || ""));
      continue;
    }
    if (isTextDelta(event) && event.role !== "tool") {
      appendMessagePart(parts, "text", String(event.content || ""));
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
    parts.push({ type: "text", content: message.content });
  }
  return parts.filter((part) => part.type === "ask" || part.type === "tool" || part.content.trim());
}

function appendMessagePart(parts: MessageRenderPart[], type: "reasoning" | "text", content: string) {
  if (!content) return;
  const previous = parts[parts.length - 1];
  if (previous?.type === type) {
    previous.content += content;
    return;
  }
  parts.push({ type, content });
}

interface MemberActivity {
  id: string;
  name: string;
  eventCount: number;
  toolCount: number;
  firstOrder: number;
  parts: MessageRenderPart[];
  tools: ToolActivity[];
  messageIDs: string[];
}

function ActivityList({
  tools,
  agents,
}: {
  tools: MessageRenderPart[];
  agents: AgentInfo[];
}) {
  const visibleTools = tools.slice(-12);
  if (!visibleTools.length) return null;
  return (
    <div className="space-y-2">
      {visibleTools.map((part, index) => {
        if (part.type !== "tool") return null;
        return (
          <ToolCallCard key={`${part.tool.ref || part.tool.id || part.tool.name}-${index}`} tool={part.tool}>
            {part.member ? <MemberActivityDetails member={part.member} agents={agents} /> : null}
          </ToolCallCard>
        );
      })}
    </div>
  );
}

function memberForTool(tool: ToolActivity, memberByCallID: Map<string, MemberActivity>) {
  const keys = uniqueStrings([
    ...toolIdentityKeys(tool.ref),
    ...toolIdentityKeys(tool.id),
    tool.name || "",
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

function MemberActivityBlock({ member, agents }: { member: MemberActivity; agents: AgentInfo[] }) {
  const [open, setOpen] = useState(false);
  const agent = resolveAgentInfo(member.name, agents);
  return (
    <div className="-ml-2 text-sm">
      <button
        className="flex items-center gap-3 rounded-lg px-2 py-2 text-left text-muted-foreground transition-colors hover:bg-muted/70"
        onClick={() => setOpen(!open)}
        type="button"
      >
        <span className="h-2 w-2 rounded-full bg-muted-foreground/35" />
        <AgentNameLabel name={member.name} agent={agent} loud />
        <ChevronRight className={cn("h-4 w-4 transition-transform", open && "rotate-90")} />
      </button>
      {open ? (
        <div className="ml-7 space-y-3 border-l border-border/60 pl-4 pt-2">
          <MemberActivityDetails member={member} agents={agents} />
        </div>
      ) : null}
    </div>
  );
}

function MemberActivityDetails({ member, agents }: { member: MemberActivity; agents: AgentInfo[] }) {
  const agent = resolveAgentInfo(member.name, agents);
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <GitBranch className="h-3.5 w-3.5" />
        <AgentNameLabel name={member.name} agent={agent} />
        <span>{member.toolCount ? `${member.toolCount} 个工具调用` : "暂无工具调用"}</span>
      </div>
      {member.parts.map((part, index) => (
        <MemberActivityPart key={`${member.id}-part-${index}`} part={part} />
      ))}
    </div>
  );
}

function MemberActivityPart({ part }: { part: MessageRenderPart }) {
  if (part.type === "reasoning") return <ReasoningBlock content={part.content} />;
  if (part.type === "tool") return <ToolCallCard tool={part.tool} />;
  if (part.type === "text") return <MarkdownContent className="text-base leading-8" content={part.content} />;
  return null;
}

function AgentNameLabel({ name, agent, loud = false }: { name: string; agent?: AgentInfo; loud?: boolean }) {
  const label = agentDisplayName(agent, name);
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-2", loud && "font-semibold tracking-normal")}>
      <span className="truncate">{label}</span>
      {agent?.builtin ? <BuiltinAgentBadge /> : null}
    </span>
  );
}

function BuiltinAgentBadge() {
  return (
    <span className="shrink-0 rounded border border-primary/25 bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary">
      内置
    </span>
  );
}

function resolveAgentInfo(name: string, agents: AgentInfo[]) {
  const key = normalizeAgentKey(name);
  if (!key) return undefined;
  return agents.find((agent) => {
    if (normalizeAgentKey(agent.name) === key) return true;
    if (normalizeAgentKey(agent.display_name || "") === key) return true;
    return (agent.aliases || []).some((alias) => normalizeAgentKey(alias) === key);
  });
}

function agentDisplayName(agent: AgentInfo | undefined, fallback: string) {
  return agent?.display_name || agent?.name || fallback;
}

function normalizeAgentKey(value: string) {
  return value.trim().toLowerCase();
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

function AskPanel({
  ask,
  sessionID,
  onAnswered,
}: {
  ask: AskActivity;
  sessionID?: string;
  onAnswered: (selected: string[], freeText: string) => void;
}) {
  const [selected, setSelected] = useState<string[]>(ask.selected);
  const [freeText, setFreeText] = useState(ask.freeText);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const hasOptions = ask.options.length > 0;
  const canSubmit = Boolean(sessionID && ask.id && !ask.answered && !submitting && (hasOptions ? selected.length > 0 || freeText.trim() : freeText.trim()));

  function toggleOption(option: string) {
    setSelected((current) => {
      if (!ask.multiSelect) return current.includes(option) ? [] : [option];
      return current.includes(option) ? current.filter((item) => item !== option) : [...current, option];
    });
  }

  async function submit() {
    if (!canSubmit || !sessionID) return;
    setSubmitting(true);
    setError("");
    try {
      const text = freeText.trim();
      await submitAskResponse(sessionID, ask.id, { selected, free_text: text });
      onAnswered(selected, text);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : String(submitError));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="sketch-surface rounded-xl bg-card/95 px-4 py-4 shadow-[0_10px_24px_hsl(218_30%_25%/0.1)]">
      <div className="mb-3 flex items-start gap-3">
        <CircleHelp className="mt-1 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">
            {ask.memberName || ask.toolName ? `${ask.memberName || ask.toolName} · ask_questions` : "ask_questions"}
          </div>
          <div className="mt-1 whitespace-pre-wrap text-base leading-7 text-foreground">{ask.question}</div>
        </div>
      </div>
      {hasOptions ? (
        <div className="mb-3 grid gap-2 sm:grid-cols-2">
          {ask.options.map((option) => {
            const checked = selected.includes(option);
            return (
              <label
                key={option}
                className={cn(
                  "flex min-h-10 cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors",
                  checked ? "border-primary/55 bg-primary/10 text-foreground" : "border-border bg-background/60 hover:bg-muted/65",
                )}
              >
                <input
                  className="h-4 w-4 shrink-0 accent-primary"
                  type={ask.multiSelect ? "checkbox" : "radio"}
                  name={`ask-${ask.id}`}
                  checked={checked}
                  onChange={() => toggleOption(option)}
                />
                <span className="min-w-0 break-words">{option}</span>
              </label>
            );
          })}
        </div>
      ) : null}
      <Textarea
        className="min-h-20 resize-y"
        value={freeText}
        disabled={ask.answered || submitting}
        placeholder={hasOptions ? "补充说明" : "输入回答"}
        onChange={(event) => setFreeText(event.target.value)}
        onKeyDown={(event) => {
          if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
            event.preventDefault();
            void submit();
          }
        }}
      />
      <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">
          {ask.answered ? `已回答：${askResponseSummary(ask.selected, ask.freeText) || "空回答"}` : error}
        </div>
        <Button type="button" size="sm" onClick={() => void submit()} disabled={!canSubmit}>
          <Send className="h-4 w-4" />
          {submitting ? "提交中" : "提交回答"}
        </Button>
      </div>
    </section>
  );
}

function AskRecord({ ask }: { ask: AskActivity }) {
  const answer = askResponseSummary(ask.selected, ask.freeText);
  return (
    <section className="rounded-xl border border-primary/20 bg-card/70 px-4 py-4 shadow-[1px_2px_0_hsl(218_32%_30%/0.06)]">
      <div className="flex items-start gap-3">
        <CircleHelp className="mt-1 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">
            {ask.memberName || ask.toolName ? `${ask.memberName || ask.toolName} · ask_questions` : "ask_questions"}
          </div>
          <div className="mt-1 whitespace-pre-wrap text-base leading-7 text-foreground">{ask.question}</div>
          {ask.options.length ? (
            <div className="mt-3 flex flex-wrap gap-2">
              {ask.options.map((option) => {
                const selected = ask.selected.includes(option);
                return (
                  <span
                    key={option}
                    className={cn(
                      "rounded-md border px-2.5 py-1 text-sm",
                      selected ? "border-primary/45 bg-primary/10 text-primary" : "border-border bg-background/55 text-muted-foreground",
                    )}
                  >
                    {option}
                  </span>
                );
              })}
            </div>
          ) : null}
          <div className="mt-3 rounded-md border border-border bg-background/55 px-3 py-2 text-sm">
            <span className="text-muted-foreground">你的回答：</span>
            <span className="whitespace-pre-wrap text-foreground">{answer || "空回答"}</span>
          </div>
        </div>
      </div>
    </section>
  );
}

function AskTimelineItem({
  ask,
  sessionID,
  onAnswered,
}: {
  ask: AskActivity;
  sessionID?: string;
  onAnswered: (selected: string[], freeText: string) => void;
}) {
  if (ask.answered) return <AskRecord ask={ask} />;
  return <AskPanel ask={ask} sessionID={sessionID} onAnswered={onAnswered} />;
}

function buildTimelineModel(
  messages: ChatViewMessage[],
  displayEvents: ChatEvent[],
  submittedAskIDs: Set<string>,
): TimelineModel {
  const reasoningByMessage = collectReasoningBlocks(displayEvents);
  const toolEvents = collectToolActivities(displayEvents, { includeMemberEvents: false });
  const memberActivities = collectMemberActivities(displayEvents);
  const askActivities = collectAskActivities(displayEvents, submittedAskIDs);
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
      order: messageOrder(message, index),
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
  const items = orderedTimelineItems(messageNodes, fallbackMembers);
  const timelineMessages = items.filter((item) => item.kind === "message").map((item) => item.node.message);
  const agentLabelMessageIDs = visibleAgentLabelMessageIDs(timelineMessages);
  for (const item of items) {
    if (item.kind === "message") item.node.showAgentLabel = agentLabelMessageIDs.has(item.node.message.id);
  }
  const inlineAskIDs = collectInlineAskIDsFromItems(items);
  return {
    items,
    messages: timelineMessages,
    trailingAsks: askActivities.filter((ask) => !inlineAskIDs.has(ask.id)),
    trailingTools,
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
    return { ...part, member };
  });
}

function toolEventsKey(events: Array<{ tool_calls?: unknown[]; tool_call?: unknown; tool_call_ref?: string; type?: string }>) {
  return events
    .map((event) => `${event.type}:${event.tool_call_ref || ""}:${event.tool_calls?.length || 0}:${event.tool_call ? 1 : 0}`)
    .join(":");
}

function eventsForDisplay(liveEvents: ChatEvent[]) {
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

function orderedTimelineItems(messages: TimelineMessageNode[], members: MemberActivity[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  for (const node of messages) {
    items.push({ kind: "message", node, order: node.order });
  }
  for (const member of members) {
    items.push({ kind: "member", member, order: member.firstOrder });
  }
  return items.sort((left, right) => {
    if (left.order !== right.order) return left.order - right.order;
    if (left.kind === right.kind) return 0;
    return left.kind === "message" ? -1 : 1;
  });
}

function messageOrder(message: ChatViewMessage, fallback: number) {
  const sequences = (message.events || [])
    .map((event, index) => eventOrder(event, index))
    .filter((sequence) => Number.isFinite(sequence));
  if (sequences.length) return Math.min(...sequences);
  return fallback + 0.5;
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

function collectInlineAskIDsFromItems(items: TimelineItem[]) {
  const ids = new Set<string>();
  for (const item of items) {
    if (item.kind !== "message") continue;
    for (const part of item.node.parts) {
      if (part.type === "ask") ids.add(part.ask.id);
    }
  }
  return ids;
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

function collectAskActivities(events: ChatEvent[], submittedAskIDs: Set<string>) {
  const asks = new Map<string, AskActivity>();
  const answered = new Map<string, { selected: string[]; freeText: string }>();
  let latestAskID = "";

  for (const event of events) {
    if (isAskQuestionEvent(event)) {
      const id = askEventID(event) || `ask-${asks.size + 1}`;
      latestAskID = id;
      const existing = asks.get(id);
      asks.set(id, {
        id,
        sessionID: event.session_id || existing?.sessionID,
        messageID: event.message_id || existing?.messageID,
        sequence: event.sequence ?? existing?.sequence,
        question: String(event.question || event.content || existing?.question || ""),
        options: askOptions(event.options) || existing?.options || [],
        multiSelect: Boolean(event.multi_select ?? existing?.multiSelect),
        selected: existing?.selected || [],
        freeText: existing?.freeText || "",
        answered: submittedAskIDs.has(id) || existing?.answered || false,
        memberName: event.member_name || existing?.memberName,
        toolName: event.tool_name || existing?.toolName,
      });
    }
    if (isAskResponseEvent(event)) {
      const id = askEventID(event) || latestAskID;
      if (!id) continue;
      answered.set(id, parseAskResponse(event));
    }
  }

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
  const selected = askOptions(event.selected) || askOptions(event.ask_selected) || [];
  const freeText = String(event.free_text || event.ask_free_text || event.content || "");
  return { selected, freeText };
}

function askResponseSummary(selected: string[], freeText: string) {
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
    const matched = findMatchingAsk(question, asks, event.sequence);
    const selected = askOptions(result.selected) || matched?.selected || [];
    const freeText = String(result.free_text || result.answer || matched?.freeText || "");
    return {
      id: matched?.id || tool.id || tool.ref || `ask-${event.message_id || ""}-${event.sequence ?? ""}`,
      sessionID: matched?.sessionID || event.session_id,
      messageID: event.message_id,
      sequence: event.sequence,
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
  return undefined;
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

function isToolCompleted(event: ChatEvent) {
  return event.type === "tool_call_completed";
}

function isToolResultEvent(event: ChatEvent) {
  return event.type === "tool_call_result_delta" || event.type === "tool_call_completed" || event.delta_kind === "tool_result";
}

function eventToolResultContent(event: ChatEvent) {
  return String(event.tool_result || ((isToolResultEvent(event) || event.role === "tool") ? event.content : "") || "");
}

function findMatchingAsk(question: string, asks: AskActivity[], sequence?: number) {
  const sameQuestion = asks.filter((ask) => ask.question.trim() === question.trim());
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
    const current = result.get(key)!;
    for (const [field, value] of Object.entries(patch)) {
      if (value === undefined || value === "") continue;
      (current as unknown as Record<string, unknown>)[field] = value;
    }
  };

  for (const event of events) {
    if (!options.includeMemberEvents && isMemberActivityEvent(event)) continue;
    for (const tool of event.tool_calls || []) {
      if (tool.name === "ask_questions") continue;
      const key = toolKey(tool, event);
      upsert(key, {
        ...tool,
        ref: tool.ref || event.tool_call_ref,
        id: tool.id || event.tool_call_id,
        status: isAssistantCompleted(event) ? "completed" : "pending",
        member_name: event.member_name || tool.member_name,
        result: tool.result || eventToolResultContent(event) || undefined,
        message_id: event.message_id,
      });
    }
    if (event.tool_call) {
      if (event.tool_call.name === "ask_questions") continue;
      const key = toolKey(event.tool_call, event);
      upsert(key, {
        ...event.tool_call,
        ref: event.tool_call.ref || event.tool_call_ref,
        id: event.tool_call.id || event.tool_call_id,
        status: isAssistantCompleted(event) ? "completed" : "pending",
        member_name: event.member_name || event.tool_call.member_name,
        result: event.tool_call.result || eventToolResultContent(event) || undefined,
        message_id: event.message_id,
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
        status: isToolCompleted(event) ? "completed" : event.type === "tool_call_failed" || event.type === "error" ? "error" : current?.status || "running",
        message_id: event.message_id || current?.message_id,
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
  }
  return order.map((key) => result.get(key)!).filter((tool) => tool.name && tool.name !== "tool");
}

type OrderedChatEvent = { event: ChatEvent; order: number };

function collectMemberActivities(events: ChatEvent[]) {
  const grouped = new Map<string, OrderedChatEvent[]>();
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
      const parts = memberMessageParts(memberEvents, tools);
      return {
        id,
        name: memberEvents.find((event) => event.member_name)?.member_name || memberEvents[0]?.agent_name || "子智能体",
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

function memberActivityID(event: ChatEvent) {
  if (event.member_call_id) return event.member_call_id;
  if (event.parent_tool_call_id) return event.parent_tool_call_id;
  if (event.message_id) return event.message_id;
  if (event.stream_id) return event.stream_id;
  return eventDisplayKey(event);
}

function memberMessageParts(events: ChatEvent[], tools: ToolActivity[]) {
  const parts: MessageRenderPart[] = [];
  const renderedTools = new Set<string>();
  let seenReasoning = "";
  let seenText = "";
  let reasoningOpen = false;
  let textOpen = false;

  for (const event of events) {
    if (isReasoningDelta(event) && event.role !== "tool") {
      const content = String(event.reasoning_content || event.content || "");
      appendSequencedTextPart(parts, "reasoning", content, reasoningOpen);
      seenReasoning = appendText(seenReasoning, content);
      reasoningOpen = true;
      textOpen = false;
      continue;
    }
    reasoningOpen = false;

    if (isTextDelta(event) && event.role !== "tool") {
      const content = String(event.content || "");
      appendSequencedTextPart(parts, "text", content, textOpen);
      seenText = appendText(seenText, content);
      textOpen = true;
      continue;
    }
    textOpen = false;

    const tool = toolFromEvent(event, tools);
    if (tool) {
      const key = toolActivityKey(tool);
      if (!renderedTools.has(key)) {
        parts.push({ type: "tool", tool });
        renderedTools.add(key);
      }
    }

    if (isAssistantCompleted(event)) {
      const reasoning = completedTextDelta(String(event.reasoning_content || ""), seenReasoning);
      if (reasoning && !seenReasoning.includes(reasoning)) {
        appendSequencedTextPart(parts, "reasoning", reasoning, false);
        seenReasoning = appendText(seenReasoning, reasoning);
      }
      const content = completedTextDelta(String(event.content || ""), seenText);
      if (content && !seenText.includes(content)) {
        appendSequencedTextPart(parts, "text", content, false);
        seenText = appendText(seenText, content);
      }
    }
  }

  return parts.filter((part) => part.type === "ask" || part.type === "tool" || part.content.trim());
}

function appendSequencedTextPart(parts: MessageRenderPart[], type: "reasoning" | "text", content: string, mergeWithPrevious: boolean) {
  if (!content) return;
  const previous = parts[parts.length - 1];
  if (mergeWithPrevious && previous?.type === type) {
    previous.content = appendText(previous.content, content);
    return;
  }
  parts.push({ type, content });
}

function eventOrder(event: ChatEvent, fallback: number) {
  const sequence = Number(event.sequence);
  if (Number.isFinite(sequence)) return sequence;
  return fallback + 0.5;
}

function completedTextDelta(content: string, seenText: string) {
  if (!content || !seenText) return content;
  if (seenText.includes(content)) return "";
  if (content.startsWith(seenText)) return content.slice(seenText.length);
  return content;
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
  return canonicalToolRef(tool.ref || tool.id) || `${tool.message_id || "tool"}:${tool.index ?? ""}`;
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
  if (left.includes(right)) return left;
  return left + right;
}
