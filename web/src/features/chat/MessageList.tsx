import { type MouseEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import anime from "animejs";
import { ArrowDown, Check, ChevronRight, CircleHelp, Copy, FileText, GitBranch, Send } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions } from "@/app/store";
import { submitAskResponse } from "@/api/stream";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/cn";
import { formatTime } from "@/lib/format";
import { ToolCallCard } from "./ToolCallCard";
import { chatMessageElementID } from "./dom";
import { useDisclosureState } from "./disclosureState";
import type { ChatEvent, ContentPartDTO, ToolCallDTO } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";
import type { AgentInfo } from "@/types/api";

type ToolActivity = ToolCallDTO & { message_id?: string; order?: number };
interface AskActivity {
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

type AskAnsweredHandler = (ask: AskActivity, selected: string[], freeText: string) => void;

interface ErrorActivity {
  id: string;
  order: number;
  title?: string;
  message: string;
  suggestions?: string[];
  technicalDetail?: string;
}

type MessageRenderPart =
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
  | { kind: "error"; error: ErrorActivity; order: number };

interface TimelineModel {
  items: TimelineItem[];
  messages: ChatViewMessage[];
  trailingAsks: AskActivity[];
  trailingTools: MessageRenderPart[];
}

interface JumpToBottomControls {
  distanceFromBottom: number;
  jump: () => void;
}

export function MessageList({ onJumpToBottomControlsChange }: { onJumpToBottomControlsChange?: (controls: JumpToBottomControls) => void }) {
  const dispatch = useAppDispatch();
  const messages = useAppSelector((state) => state.chat.messages);
  const events = useAppSelector((state) => state.chat.events);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const statusText = useAppSelector((state) => state.chat.statusText);
  const error = useAppSelector((state) => state.chat.error);
  const errorTitle = useAppSelector((state) => state.chat.errorTitle);
  const errorSuggestions = useAppSelector((state) => state.chat.errorSuggestions);
  const technicalError = useAppSelector((state) => state.chat.technicalError);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const runningSessionID = useAppSelector((state) => state.chat.runningSessionID);
  const agents = useAppSelector((state) => state.app.agents);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const stickToBottomRef = useRef(true);
  const previousMessageCountRef = useRef(0);
  const [submittedAskIDs, setSubmittedAskIDs] = useState<Set<string>>(() => new Set());
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);
  const [scrollDistanceFromBottom, setScrollDistanceFromBottom] = useState(0);
  const displayEvents = useMemo(() => eventsForDisplay(events), [events]);
  const canAnswerAsk = Boolean(isProcessing && activeSessionID && (!runningSessionID || runningSessionID === activeSessionID));
  const timeline = useMemo(
    () => buildTimelineModel(messages, displayEvents, submittedAskIDs, isProcessing),
    [messages, displayEvents, submittedAskIDs, isProcessing],
  );
  const hasMatchingTimelineError = timeline.items.some((item) => (
    item.kind === "error"
    && item.error.message === error
    && item.error.title === errorTitle
  ));

  const scrollToBottom = useCallback(() => {
    const scroll = scrollRef.current;
    if (!scroll) return;
    scroll.scrollTop = scroll.scrollHeight;
    setScrollDistanceFromBottom(0);
    setShowJumpToBottom(false);
  }, []);

  const jumpToBottom = useCallback(() => {
    stickToBottomRef.current = true;
    scrollToBottom();
  }, [scrollToBottom]);

  useEffect(() => {
    if (!stickToBottomRef.current) return;
    scrollToBottom();
  }, [timeline.items, timeline.trailingAsks.length, isProcessing, statusText, error, scrollToBottom, toolEventsKey(displayEvents)]);

  useEffect(() => {
    stickToBottomRef.current = true;
    setShowJumpToBottom(false);
    requestAnimationFrame(scrollToBottom);
  }, [activeSessionID, scrollToBottom]);

  useEffect(() => {
    onJumpToBottomControlsChange?.({ distanceFromBottom: scrollDistanceFromBottom, jump: jumpToBottom });
  }, [jumpToBottom, onJumpToBottomControlsChange, scrollDistanceFromBottom]);

  useEffect(() => {
    return () => onJumpToBottomControlsChange?.({ distanceFromBottom: 0, jump: () => {} });
  }, [onJumpToBottomControlsChange]);

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

  function handleScroll() {
    const scroll = scrollRef.current;
    if (!scroll) return;
    const distance = scrollDistanceToBottom(scroll);
    const atBottom = distance < 48;
    stickToBottomRef.current = atBottom;
    setScrollDistanceFromBottom(Math.round(distance));
    setShowJumpToBottom(!atBottom);
  }

  function handleAskAnswered(ask: AskActivity, selected: string[], freeText: string) {
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
  }

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={scrollRef}
        className="chat-scroll chat-thread-scroll h-full overflow-x-hidden overflow-y-auto px-3 pb-[calc(var(--chat-dock-height,10rem)+1rem)] pt-5 sm:px-6 sm:pt-8 md:pb-8"
        onScroll={handleScroll}
      >
        <div className="mx-auto w-full max-w-4xl">
          {timeline.items.map((item, index) => {
            const spacing = timelineItemSpacingClass(timeline.items, index);
            if (item.kind === "member") {
              const member = item.member;
              return (
                <div key={`member-${member.id}`} className={spacing}>
                  <MemberActivityBlock
                    member={member}
                    agents={agents}
                    sessionID={activeSessionID}
                    canAnswerAsk={canAnswerAsk}
                    onAskAnswered={handleAskAnswered}
                  />
                </div>
              );
            }
            if (item.kind === "tool") {
              return (
                <div key={`tool-${toolActivityKey(item.part.tool)}`} className={cn(spacing, "space-y-3")}>
                  <MessagePart
                    part={item.part}
                    disclosureID={`timeline:${toolActivityKey(item.part.tool)}`}
                    sessionID={activeSessionID}
                    agents={agents}
                    canAnswerAsk={canAnswerAsk}
                    onAskAnswered={handleAskAnswered}
                  />
                </div>
              );
            }
            if (item.kind === "ask") {
              return (
                <div key={`ask-${item.ask.id}`} className={spacing}>
                  <AskTimelineItem
                    ask={item.ask}
                    sessionID={item.ask.sessionID || activeSessionID}
                    canAnswer={canAnswerAsk}
                    onAnswered={(selected, freeText) => handleAskAnswered(item.ask, selected, freeText)}
                  />
                </div>
              );
            }
            if (item.kind === "error") {
              return (
                <div key={`error-${item.error.id}`} className={spacing}>
                  <ErrorNotice
                    title={item.error.title}
                    message={item.error.message}
                    suggestions={item.error.suggestions}
                    technicalDetail={item.error.technicalDetail}
                    className="mt-0"
                  />
                </div>
              );
            }
            const { message, parts, showAgentLabel } = item.node;
            if (message.hidden) return null;
            return (
              <div key={message.id} className={cn(spacing, "space-y-3")}>
                <MessageRow
                  message={message}
                  parts={parts}
                  sessionID={activeSessionID}
                  agents={agents}
                  showAgentLabel={showAgentLabel}
                  showCopyAction={item.node.showCopyAction}
                  canAnswerAsk={canAnswerAsk}
                  onAskAnswered={handleAskAnswered}
                />
              </div>
            );
          })}
          {timeline.trailingAsks.map((ask, index) => (
            <div key={ask.id} className={index === 0 && timeline.items.length === 0 ? "" : "mt-6"}>
              <AskTimelineItem
                ask={ask}
                sessionID={ask.sessionID || activeSessionID}
                canAnswer={canAnswerAsk}
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
            </div>
          ))}
          <ActivityList
            tools={timeline.trailingTools}
            agents={agents}
            sessionID={activeSessionID}
            canAnswerAsk={canAnswerAsk}
            onAskAnswered={handleAskAnswered}
          />
          {isProcessing ? (
            <div className="message-row mt-6 text-lg text-muted-foreground">
              <div className="flex items-center">
                {loadingStatusText(statusText)}
                <ThinkingDots />
              </div>
            </div>
          ) : null}
          {error && !hasMatchingTimelineError ? (
            <ErrorNotice
              title={errorTitle}
              message={error}
              suggestions={errorSuggestions}
              technicalDetail={technicalError}
            />
          ) : null}
          <div ref={bottomRef} />
        </div>
      </div>
      {showJumpToBottom ? (
        <button
          className="absolute bottom-4 left-1/2 z-20 hidden h-9 -translate-x-1/2 items-center gap-2 rounded-full border border-border bg-card/95 px-3 text-sm font-medium text-muted-foreground shadow-[0_8px_24px_hsl(218_30%_25%/0.14)] backdrop-blur transition-colors hover:bg-accent hover:text-foreground xl:flex"
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

function ErrorNotice({
  title,
  message,
  suggestions,
  technicalDetail,
  className,
}: {
  title?: string;
  message: string;
  suggestions?: string[];
  technicalDetail?: string;
  className?: string;
}) {
  const showTechnical = Boolean(technicalDetail && technicalDetail !== message);
  return (
    <div className={cn("sketch-surface rounded-md border-destructive/45 bg-destructive/5 px-4 py-3 text-sm text-destructive", className ?? "mt-4")}>
      {title ? <div className="font-semibold">{title}</div> : null}
      <div className={title ? "mt-1 text-destructive/90" : "text-destructive/90"}>{message}</div>
      {suggestions?.length ? (
        <ul className="mt-2 list-disc space-y-1 pl-5 text-destructive/80">
          {suggestions.map((suggestion) => (
            <li key={suggestion}>{suggestion}</li>
          ))}
        </ul>
      ) : null}
      {showTechnical ? (
        <details className="mt-2 text-destructive/70">
          <summary className="cursor-pointer select-none">技术详情</summary>
          <pre className="mt-2 max-h-32 overflow-auto whitespace-pre-wrap break-words rounded-md bg-background/70 p-2 text-xs leading-5">
            {technicalDetail}
          </pre>
        </details>
      ) : null}
    </div>
  );
}

function scrollDistanceToBottom(element: HTMLElement) {
  return Math.max(0, element.scrollHeight - element.scrollTop - element.clientHeight);
}

function loadingStatusText(statusText?: string) {
  if (!statusText || statusText === "处理中" || statusText === "开始处理您的请求...") return "执行中";
  return statusText;
}

function ThinkingDots() {
  return (
    <span className="ml-2 inline-flex items-center gap-1" aria-hidden="true">
      <i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/60 [animation-delay:-0.24s]" />
      <i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/45 [animation-delay:-0.12s]" />
      <i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/30" />
    </span>
  );
}

function MessageRow({
  message,
  parts,
  sessionID,
  agents,
  showAgentLabel,
  showCopyAction,
  canAnswerAsk,
  onAskAnswered,
}: {
  message: ChatViewMessage;
  parts: MessageRenderPart[];
  sessionID?: string;
  agents: AgentInfo[];
  showAgentLabel: boolean;
  showCopyAction: boolean;
  canAnswerAsk: boolean;
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
    const attachments = messageAttachmentParts(message.contentParts);
    return (
      <article id={chatMessageElementID(message.id)} className="message-row group flex w-full scroll-mt-8 flex-col items-end gap-2">
        <div className="flex max-w-[92%] flex-col items-end gap-2 sm:max-w-[78%]">
          {attachments.length ? <UserAttachmentPills attachments={attachments} /> : null}
          {message.content.trim() ? (
            <div className="rounded-2xl bg-muted px-4 py-3 text-base leading-7 text-foreground sm:px-5 sm:py-4 sm:text-lg sm:leading-8">
              <div className="whitespace-pre-wrap">{message.content}</div>
            </div>
          ) : null}
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
        {(() => {
          const partKeys = new Map<string, number>();
          return parts.map((part) => {
            const key = nextRepeatedKey(partKeys, messagePartKey(part, message.id));
            return (
              <MessagePart
                key={key}
                part={part}
                disclosureID={key}
                sessionID={sessionID}
                agents={agents}
                canAnswerAsk={canAnswerAsk}
                onAskAnswered={onAskAnswered}
              />
            );
          });
        })()}
      </div>
      {showCopyAction && (hasContent || copyContent) ? <MessageActions content={copyContent || message.content} compact /> : null}
    </article>
  );
}

function UserAttachmentPills({ attachments }: { attachments: ContentPartDTO[] }) {
  return (
    <div className="flex max-w-full flex-wrap justify-end gap-2">
      {attachments.map((part, index) => {
        if (isImagePart(part)) return <UserImagePill key={attachmentPartKey(part, index)} part={part} index={index} />;
        return <UserFilePill key={attachmentPartKey(part, index)} part={part} />;
      })}
    </div>
  );
}

function UserImagePill({ part, index }: { part: ContentPartDTO; index: number }) {
  const src = part.base64_data
    ? `data:${part.mime_type || "image/png"};base64,${part.base64_data}`
    : part.url || "";
  const label = imagePartLabel(part, index);
  return (
    <div className="flex h-12 max-w-[10.5rem] items-center gap-2 rounded-xl border border-border/70 bg-muted px-2 py-1.5 text-sm text-foreground sm:max-w-[13rem]">
      <div className="h-9 w-9 shrink-0 overflow-hidden rounded-lg bg-background/60">
        {src ? <img className="h-full w-full object-cover" src={src} alt={label} /> : null}
      </div>
      <div className="min-w-0">
        <div className="truncate font-medium leading-5">{label}</div>
        <div className="flex items-center gap-1 text-xs leading-4 text-muted-foreground">
          <FileText className="h-3.5 w-3.5" />
          <span>{part.mime_type || "图片"}</span>
        </div>
      </div>
    </div>
  );
}

function UserFilePill({ part }: { part: ContentPartDTO }) {
  const label = part.name || part.url || part.text || "附件";
  return (
    <div className="flex h-12 max-w-[10.5rem] items-center gap-2 rounded-xl border border-border/70 bg-muted px-3 py-1.5 text-sm text-foreground sm:max-w-[13rem]">
      <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate font-medium">{label}</span>
    </div>
  );
}

function messageAttachmentParts(parts?: ContentPartDTO[]) {
  return (parts || []).filter((part) => part.type !== "text");
}

function isImagePart(part: ContentPartDTO) {
  return part.type === "image_url" || part.type === "image_base64" || Boolean(part.base64_data);
}

function attachmentPartKey(part: ContentPartDTO, index: number) {
  return `${part.type}:${part.name || ""}:${part.url || part.mime_type || ""}:${index}`;
}

function imagePartLabel(part: ContentPartDTO, index: number) {
  if (part.name) return part.name;
  if (part.url) {
    const segments = part.url.split(/[\\/]/).filter(Boolean);
    return segments[segments.length - 1] || `图片 ${index + 1}`;
  }
  return `图片 ${index + 1}`;
}

function MessagePart({
  part,
  disclosureID,
  sessionID,
  agents,
  canAnswerAsk,
  onAskAnswered,
}: {
  part: MessageRenderPart;
  disclosureID: string;
  sessionID?: string;
  agents: AgentInfo[];
  canAnswerAsk: boolean;
  onAskAnswered: AskAnsweredHandler;
}) {
  if (part.type === "reasoning") return <ReasoningBlock content={part.content} disclosureID={disclosureID} />;
  if (part.type === "tool") {
    const memberAgent = part.member ? resolveAgentInfo(part.member.name, agents) : undefined;
    return (
      <ToolCallCard
        disclosureID={disclosureID}
        tool={part.tool}
        title={part.member ? <AgentNameLabel name={part.member.name} agent={memberAgent} loud /> : undefined}
      >
        {part.member ? (
          <MemberActivityDetails
            member={part.member}
            agents={agents}
            sessionID={sessionID}
            canAnswerAsk={canAnswerAsk}
            onAskAnswered={onAskAnswered}
          />
        ) : null}
      </ToolCallCard>
    );
  }
  if (part.type === "ask") {
    return (
      <div>
        <AskTimelineItem
          ask={part.ask}
          sessionID={part.ask.sessionID || sessionID}
          canAnswer={canAnswerAsk}
          onAnswered={(selected, freeText) => onAskAnswered(part.ask, selected, freeText)}
        />
      </div>
    );
  }
  return <MarkdownContent className="text-base leading-8 sm:text-lg sm:leading-9" content={part.content} />;
}

function ReasoningBlock({ content, disclosureID }: { content: string; disclosureID: string }) {
  const [open, toggleOpen] = useDisclosureState(disclosureID);

  function handleToggle(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    event.stopPropagation();
    toggleOpen();
  }

  return (
    <div className="-ml-2 text-sm">
      <button
        className="flex items-center gap-3 rounded-lg px-2 py-2 text-left text-amber-600 transition-colors hover:bg-amber-50/70"
        onClick={handleToggle}
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
    parts.push({ type: "text", content: message.content, key: `${message.id}:text:fallback` });
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

interface MemberActivity {
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

function ActivityList({
  tools,
  agents,
  sessionID,
  canAnswerAsk,
  onAskAnswered,
}: {
  tools: MessageRenderPart[];
  agents: AgentInfo[];
  sessionID?: string;
  canAnswerAsk: boolean;
  onAskAnswered: AskAnsweredHandler;
}) {
  const visibleTools = tools.slice(-12);
  if (!visibleTools.length) return null;
  return (
    <div className="space-y-2">
      {visibleTools.map((part) => {
        if (part.type !== "tool") return null;
        const memberAgent = part.member ? resolveAgentInfo(part.member.name, agents) : undefined;
        const key = `activity:${toolActivityKey(part.tool)}`;
        return (
          <ToolCallCard
            key={key}
            disclosureID={key}
            tool={part.tool}
            title={part.member ? <AgentNameLabel name={part.member.name} agent={memberAgent} loud /> : undefined}
          >
            {part.member ? (
              <MemberActivityDetails
                member={part.member}
                agents={agents}
                sessionID={sessionID}
                canAnswerAsk={canAnswerAsk}
                onAskAnswered={onAskAnswered}
              />
            ) : null}
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

function MemberActivityBlock({
  member,
  agents,
  sessionID,
  canAnswerAsk,
  onAskAnswered,
}: {
  member: MemberActivity;
  agents: AgentInfo[];
  sessionID?: string;
  canAnswerAsk: boolean;
  onAskAnswered: AskAnsweredHandler;
}) {
  const [open, toggleOpen] = useDisclosureState(`member:${member.id}`);
  const agent = resolveAgentInfo(member.name, agents);

  function handleToggle(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    event.stopPropagation();
    toggleOpen();
  }

  return (
    <div className="-ml-2 text-sm">
      <button
        className="flex items-center gap-3 rounded-lg px-2 py-2 text-left text-muted-foreground transition-colors hover:bg-muted/70"
        onClick={handleToggle}
        type="button"
      >
        <span className="h-2 w-2 rounded-full bg-muted-foreground/35" />
        <AgentNameLabel name={member.name} agent={agent} loud />
        <ChevronRight className={cn("h-4 w-4 transition-transform", open && "rotate-90")} />
      </button>
      {open ? (
        <div className="ml-7 space-y-3 border-l border-border/60 pl-4 pt-2">
          <MemberActivityDetails
            member={member}
            agents={agents}
            sessionID={sessionID}
            canAnswerAsk={canAnswerAsk}
            onAskAnswered={onAskAnswered}
          />
        </div>
      ) : null}
    </div>
  );
}

function MemberActivityDetails({
  member,
  agents,
  sessionID,
  canAnswerAsk,
  onAskAnswered,
}: {
  member: MemberActivity;
  agents: AgentInfo[];
  sessionID?: string;
  canAnswerAsk: boolean;
  onAskAnswered: AskAnsweredHandler;
}) {
  const agent = resolveAgentInfo(member.name, agents);
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <GitBranch className="h-3.5 w-3.5" />
        <AgentNameLabel name={member.name} agent={agent} />
        <span>{member.toolCount ? `${member.toolCount} 个工具调用` : "暂无工具调用"}</span>
      </div>
      {(() => {
        const partKeys = new Map<string, number>();
        return member.parts.map((part) => {
          const key = nextRepeatedKey(partKeys, messagePartKey(part, member.id));
          return (
            <MemberActivityPart
              key={key}
              part={part}
              disclosureID={key}
              sessionID={sessionID}
              canAnswerAsk={canAnswerAsk}
              onAskAnswered={onAskAnswered}
            />
          );
        });
      })()}
    </div>
  );
}

function MemberActivityPart({
  part,
  disclosureID,
  sessionID,
  canAnswerAsk,
  onAskAnswered,
}: {
  part: MessageRenderPart;
  disclosureID: string;
  sessionID?: string;
  canAnswerAsk: boolean;
  onAskAnswered: AskAnsweredHandler;
}) {
  if (part.type === "reasoning") return <ReasoningBlock content={part.content} disclosureID={disclosureID} />;
  if (part.type === "tool") return <ToolCallCard tool={part.tool} disclosureID={disclosureID} />;
  if (part.type === "ask") {
    return (
      <AskTimelineItem
        ask={part.ask}
        sessionID={part.ask.sessionID || sessionID}
        canAnswer={canAnswerAsk}
        onAnswered={(selected, freeText) => onAskAnswered(part.ask, selected, freeText)}
      />
    );
  }
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
  return agent?.display_name || agent?.name || runtimeAgentDisplayName(fallback) || fallback;
}

function normalizeAgentKey(value: string) {
  return value.trim().toLowerCase();
}

function runtimeAgentDisplayName(name: string) {
  return runtimeAgentDisplayNames[normalizeAgentKey(name)];
}

const runtimeAgentDisplayNames: Record<string, string> = {
  deep_researcher: "深度研究员",
};

function MessageActions({
  content,
  align = "left",
  time,
  compact = false,
}: {
  content: string;
  align?: "left" | "right";
  time?: string;
  compact?: boolean;
}) {
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
        compact && "mt-1 h-0 overflow-visible",
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
  canAnswer,
  onAnswered,
}: {
  ask: AskActivity;
  sessionID?: string;
  canAnswer: boolean;
  onAnswered: (selected: string[], freeText: string) => void;
}) {
  const [selected, setSelected] = useState<string[]>(ask.selected);
  const [freeText, setFreeText] = useState(ask.freeText);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const hasOptions = ask.options.length > 0;
  const disabledReason = !canAnswer ? "当前任务已结束，无法继续提交回答。" : "";
  const canSubmit = Boolean(
    canAnswer &&
      sessionID &&
      ask.id &&
      !ask.answered &&
      !submitting &&
      (hasOptions ? selected.length > 0 || freeText.trim() : freeText.trim()),
  );

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
    <section className="sketch-surface rounded-xl bg-card/95 px-3 py-3 shadow-[0_10px_24px_hsl(218_30%_25%/0.1)] sm:px-4 sm:py-4">
      <div className="mb-3 flex items-start gap-2 sm:gap-3">
        <CircleHelp className="mt-1 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">{askTitle(ask)}</div>
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
                  "flex min-h-10 items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors",
                  canAnswer && !ask.answered ? "cursor-pointer" : "cursor-not-allowed opacity-65",
                  checked ? "border-primary/55 bg-primary/10 text-foreground" : "border-border bg-background/60 hover:bg-muted/65",
                )}
              >
                <input
                  className="h-4 w-4 shrink-0 accent-primary"
                  type={ask.multiSelect ? "checkbox" : "radio"}
                  name={`ask-${ask.id}`}
                  checked={checked}
                  disabled={!canAnswer || ask.answered || submitting}
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
        disabled={!canAnswer || ask.answered || submitting}
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
          {ask.answered ? `已回答：${askResponseSummary(ask.selected, ask.freeText) || "空回答"}` : disabledReason || error}
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
    <section className="rounded-xl border border-primary/20 bg-card/70 px-3 py-3 shadow-[1px_2px_0_hsl(218_32%_30%/0.06)] sm:px-4 sm:py-4">
      <div className="flex items-start gap-2 sm:gap-3">
        <CircleHelp className="mt-1 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">{askTitle(ask)}</div>
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
  canAnswer,
  onAnswered,
}: {
  ask: AskActivity;
  sessionID?: string;
  canAnswer: boolean;
  onAnswered: (selected: string[], freeText: string) => void;
}) {
  if (ask.answered) return <AskRecord ask={ask} />;
  return <AskPanel ask={ask} sessionID={sessionID} canAnswer={canAnswer} onAnswered={onAnswered} />;
}

function askTitle(ask: AskActivity) {
  if (ask.memberName) return `${ask.memberName} · ask_questions`;
  return "ask_questions";
}

function buildTimelineModel(
  messages: ChatViewMessage[],
  displayEvents: ChatEvent[],
  submittedAskIDs: Set<string>,
  isProcessing: boolean,
): TimelineModel {
  const eventOrders = eventOrderMap(displayEvents);
  const reasoningByMessage = collectReasoningBlocks(displayEvents);
  const toolEvents = collectToolActivities(displayEvents, { includeMemberEvents: false });
  const askToolAnchors = collectAskToolAnchors(displayEvents);
  const askActivities = collectAskActivities(displayEvents, submittedAskIDs, askToolAnchors);
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
  const items = orderedTimelineItems(messageNodes, fallbackMembers, timelineTools, timelineAsks, errorActivities);
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

function orderedTimelineItems(
  messages: TimelineMessageNode[],
  members: MemberActivity[],
  tools: Array<Extract<MessageRenderPart, { type: "tool" }>>,
  asks: AskActivity[],
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
    case "member":
      return 3;
    case "error":
      return 4;
    default:
      return 5;
  }
}

function timelineItemSpacingClass(items: TimelineItem[], index: number) {
  if (index <= 0) return "";
  const previous = items[index - 1];
  const current = items[index];
  if (!previous || !current) return "mt-6";
  if (current.kind === "tool") return previous.kind === "message" ? "mt-2" : "mt-2";
  if (current.kind === "ask") return previous.kind === "tool" ? "mt-2" : "mt-3";
  if (current.kind === "error") return previous.kind === "message" ? "mt-4" : "mt-3";
  if (previous.kind === "tool" && current.kind === "message") return "mt-5";
  if (previous.kind === "ask" && current.kind === "message") return "mt-5";
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

function errorEventMessage(event: ChatEvent) {
  return stringValue(event.display_error || event.error_title || event.error || event.content || event.message) || "请求失败";
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
  const streamTerminalStatus = terminalToolStatus(events);
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
  };

  events.forEach((event, index) => {
    if (!options.includeMemberEvents && isMemberActivityEvent(event)) return;
    const orderValue = eventOrder(event, index);
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
        order: orderValue,
      });
    }
    if (event.tool_call && event.tool_call.name !== "ask_questions") {
      const key = toolKey(event.tool_call, event);
      upsert(key, {
        ...event.tool_call,
        ref: event.tool_call.ref || event.tool_call_ref,
        id: event.tool_call.id || event.tool_call_id,
        status: isAssistantCompleted(event) ? "completed" : "pending",
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
        status: isToolCompleted(event) ? "completed" : event.type === "tool_call_failed" || event.type === "error" ? "error" : current?.status || "running",
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
    .map((key) => {
      const tool = result.get(key)!;
      if (streamTerminalStatus && (tool.status === "pending" || tool.status === "running")) {
        return { ...tool, status: streamTerminalStatus };
      }
      return tool;
    })
    .filter((tool) => tool.name && tool.name !== "tool");
}

function terminalToolStatus(events: ChatEvent[]) {
  if (events.some((event) => event.type === "error")) return "error";
  if (events.some((event) => event.type === "processing_end" || event.type === "cancelled")) return "completed";
  return "";
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

function toolActivityKey(tool: ToolActivity) {
  return canonicalToolRef(tool.ref || tool.id) || `${tool.message_id || "tool"}:${tool.index ?? ""}`;
}

function messagePartKey(part: MessageRenderPart, ownerID: string) {
  if (part.type === "tool") return `${ownerID}:tool:${toolActivityKey(part.tool)}`;
  if (part.type === "ask") return `${ownerID}:ask:${part.ask.id}`;
  if (part.key) return `${ownerID}:${part.key}`;
  return `${ownerID}:${part.type}:${stableTextHash(part.content)}`;
}

function nextRepeatedKey(seen: Map<string, number>, base: string) {
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
