import type { TimelineItem } from "@/types/presentation";
import {
  lazy,
  Suspense,
  memo,
  type MouseEvent,
  type RefObject,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  ArrowDown,
  Check,
  ChevronRight,
  CircleHelp,
  Copy,
  FileText,
  GitBranch,
  Send,
  ShieldAlert,
} from "lucide-react";
import { useAppSelector } from "@/app/hooks";
import {
  assistantAgentID,
  assistantDisplayName,
  assistantSessionID,
} from "@/lib/assistant";
import { submitApproval, submitAskResponse } from "@/api/stream";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { copyText } from "@/lib/clipboard";
import { useTimedState } from "@/lib/useTimedState";
import { cn } from "@/lib/cn";
import { formatTime } from "@/lib/format";
import { ToolCallCard } from "./ToolCallCard";
import { chatBottomThreshold, chatMessageElementID } from "./dom";
import { useDisclosureState } from "./disclosureState";
import type { ContentPartDTO } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";
import type { AgentInfo } from "@/types/api";
import {
  normalizeAgentKey,
  type AskActivity,
  type ApprovalActivity,
  type MessageRenderPart,
  type MemberActivity,
  timelineItemSpacingClass,
  askResponseSummary,
  toolActivityKey,
  messagePartKey,
  nextRepeatedKey,
} from "./timeline";

import { actionSummary, decodeUIAction } from "./openui/protocol";

const AssistantUIContent = lazy(() => import("./openui/AssistantUIContent"));

export type AskAnsweredHandler = (
  ask: AskActivity,
  selected: string[],
  freeText: string,
) => void;

export type ApprovalAnsweredHandler = (
  approval: ApprovalActivity,
  decision: 0 | 1 | 2,
) => void;

export interface JumpToBottomControls {
  distanceFromBottom: number;
  jump: () => void;
}

export function MessageList({
  scrollRef,
  onJumpToBottomControlsChange,
}: {
  scrollRef: RefObject<HTMLDivElement | null>;
  onJumpToBottomControlsChange?: (controls: JumpToBottomControls) => void;
}) {
  const messages = useAppSelector((state) => state.chat.messages);
  const presentation = useAppSelector((state) => state.chat.presentation);
  const isProcessing = useAppSelector((state) =>
    Boolean(
      state.chat.activeSessionID &&
      state.chat.runningTasks[state.chat.activeSessionID],
    ),
  );
  const statusText = useAppSelector((state) => state.chat.statusText);
  const error = useAppSelector((state) => state.chat.error);
  const errorTitle = useAppSelector((state) => state.chat.errorTitle);
  const errorSuggestions = useAppSelector(
    (state) => state.chat.errorSuggestions,
  );
  const technicalError = useAppSelector((state) => state.chat.technicalError);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const agents = useAppSelector((state) => state.app.agents);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const stickToBottomRef = useRef(true);
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);
  const [scrollDistanceFromBottom, setScrollDistanceFromBottom] = useState(0);

  const canAnswerAsk = Boolean(isProcessing && activeSessionID);
  const timeline = useMemo(() => {
    const knownIDs = new Set(presentation.items.map((item) => item.id));
    const optimistic = messages.filter((message) => !knownIDs.has(message.id));
    return {
      items: [
        ...presentation.items,
        ...optimistic.map((message, index) => ({
          id: message.id,
          index: presentation.items.length + index,
          kind: "message" as const,
          order: Number.MAX_SAFE_INTEGER,
          node: {
            message,
            parts: [],
            showAgentLabel: false,
            showCopyAction: false,
            order: Number.MAX_SAFE_INTEGER,
          },
        })),
      ],
      messages,
    };
  }, [presentation, messages]);
  const hasMatchingTimelineError = timeline.items.some(
    (item) =>
      item.kind === "error" &&
      item.error.message === error &&
      item.error.title === errorTitle,
  );

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
  }, [timeline.items, isProcessing, statusText, error, scrollToBottom]);

  useEffect(() => {
    stickToBottomRef.current = true;
    setShowJumpToBottom(false);
    requestAnimationFrame(scrollToBottom);
  }, [activeSessionID, scrollToBottom]);

  useEffect(() => {
    onJumpToBottomControlsChange?.({
      distanceFromBottom: scrollDistanceFromBottom,
      jump: jumpToBottom,
    });
  }, [jumpToBottom, onJumpToBottomControlsChange, scrollDistanceFromBottom]);

  useEffect(() => {
    return () =>
      onJumpToBottomControlsChange?.({ distanceFromBottom: 0, jump: () => {} });
  }, [onJumpToBottomControlsChange]);

  if (timeline.items.length === 0 && !isProcessing && !error) {
    return <div className="min-h-0 flex-1" />;
  }

  function handleScroll() {
    const scroll = scrollRef.current;
    if (!scroll) return;
    const distance = scrollDistanceToBottom(scroll);
    const atBottom = distance < chatBottomThreshold;
    stickToBottomRef.current = atBottom;
    setScrollDistanceFromBottom(Math.round(distance));
    setShowJumpToBottom(!atBottom);
  }

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={scrollRef}
        className="chat-scroll chat-thread-scroll h-full overflow-x-hidden overflow-y-auto px-3 pb-[calc(var(--chat-dock-height,10rem)+1rem)] pt-5 sm:px-6 sm:pt-8 md:pb-8"
        onScroll={handleScroll}
      >
        <div className="mx-auto w-full max-w-4xl">
          <ConversationItems
            items={timeline.items}
            sessionID={activeSessionID}
            agents={agents}
            canAnswerAsk={canAnswerAsk}
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
          className="absolute bottom-2 left-1/2 z-20 hidden h-9 -translate-x-1/2 items-center gap-2 rounded-full border border-border bg-card px-3 text-sm font-medium text-muted-foreground shadow-popover backdrop-blur transition-colors hover:bg-accent hover:text-foreground xl:flex"
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

export function ConversationItems({
  items,
  sessionID = "",
  agents = [],
  canAnswerAsk = false,
  onAskAnswered = () => {},
  onApprovalAnswered = () => {},
}: {
  items: TimelineItem[];
  sessionID?: string;
  agents?: AgentInfo[];
  canAnswerAsk?: boolean;
  onAskAnswered?: AskAnsweredHandler;
  onApprovalAnswered?: ApprovalAnsweredHandler;
}) {
  return (
    <>
      {items.map((item, index) => {
        const spacing = timelineItemSpacingClass(items, index);
        if (item.kind === "member") {
          const member = item.member;
          return (
            <div key={`member-${member.id}`} className={spacing}>
              <MemberActivityBlock
                member={member}
                agents={agents}
                sessionID={sessionID}
                canAnswerAsk={canAnswerAsk}
                onAskAnswered={onAskAnswered}
              />
            </div>
          );
        }
        if (item.kind === "ask") {
          return (
            <div key={`ask-${item.ask.id}`} className={spacing}>
              <AskTimelineItem
                ask={item.ask}
                sessionID={item.ask.sessionID || sessionID}
                canAnswer={canAnswerAsk && item.ask.active}
                onAnswered={(selected, freeText) =>
                  onAskAnswered(item.ask, selected, freeText)
                }
              />
            </div>
          );
        }
        if (item.kind === "approval") {
          return (
            <div key={`approval-${item.approval.id}`} className={spacing}>
              <ApprovalTimelineItem
                approval={item.approval}
                sessionID={sessionID}
                canAnswer={canAnswerAsk && item.approval.active}
                onAnswered={onApprovalAnswered}
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

        return (
          <div key={message.id} className={spacing}>
            <MessageRow
              message={message}
              parts={parts}
              sessionID={sessionID}
              agents={agents}
              showAgentLabel={showAgentLabel}
              showCopyAction={item.node.showCopyAction}
              canAnswerAsk={canAnswerAsk}
              onAskAnswered={onAskAnswered}
            />
          </div>
        );
      })}
    </>
  );
}

export function ErrorNotice({
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
    <div
      className={cn(
        "sketch-surface rounded-md border-destructive/45 bg-destructive/5 px-4 py-3 text-sm text-destructive",
        className ?? "mt-4",
      )}
    >
      {title ? <div className="font-semibold">{title}</div> : null}
      <div
        className={title ? "mt-1 text-destructive/90" : "text-destructive/90"}
      >
        {message}
      </div>
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

export function scrollDistanceToBottom(element: HTMLElement) {
  return Math.max(
    0,
    element.scrollHeight - element.scrollTop - element.clientHeight,
  );
}

export function loadingStatusText(statusText?: string) {
  if (
    !statusText ||
    statusText === "处理中" ||
    statusText === "开始处理您的请求..."
  )
    return "执行中";
  return statusText;
}

export function ThinkingDots() {
  return (
    <span className="ml-2 inline-flex items-center gap-1" aria-hidden="true">
      <i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/60 [animation-delay:-0.24s]" />
      <i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/45 [animation-delay:-0.12s]" />
      <i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/30" />
    </span>
  );
}

export interface MessageRowProps {
  message: ChatViewMessage;
  parts: MessageRenderPart[];
  sessionID?: string;
  agents: AgentInfo[];
  showAgentLabel: boolean;
  showCopyAction: boolean;
  canAnswerAsk: boolean;
  onAskAnswered: AskAnsweredHandler;
}

export const MessageRow = memo(function MessageRow({
  message,
  parts,
  sessionID,
  agents,
  showAgentLabel,
  showCopyAction,
  canAnswerAsk,
  onAskAnswered,
}: MessageRowProps) {
  const hasContent = Boolean(message.content.trim());
  if (message.role === "system") {
    return (
      <article
        id={chatMessageElementID(message.id)}
        className="message-row w-full scroll-mt-8"
      >
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
      <article
        id={chatMessageElementID(message.id)}
        className="message-row group flex w-full scroll-mt-8 flex-col items-end gap-2"
      >
        <div className="flex max-w-[92%] flex-col items-end gap-2 sm:max-w-[78%]">
          {attachments.length ? (
            <UserAttachmentPills attachments={attachments} />
          ) : null}
          {message.content.trim() ? (
            <div className="rounded-2xl bg-accent px-4 py-3 text-base leading-7 text-foreground sm:px-5 sm:py-4 sm:text-lg sm:leading-8">
              <div className="whitespace-pre-wrap">
                {decodeUIAction(message.content)
                  ? actionSummary(decodeUIAction(message.content)!)
                  : message.content}
              </div>
            </div>
          ) : null}
        </div>
        <MessageActions
          align="right"
          content={message.content}
          time={message.createdAt}
        />
      </article>
    );
  }

  const textParts = parts.filter(
    (part): part is { type: "text"; content: string } => part.type === "text",
  );
  const copyContent = textParts.map((part) => part.content).join("");

  return (
    <article
      id={chatMessageElementID(message.id)}
      className="message-row group w-full scroll-mt-8"
    >
      {message.agent && showAgentLabel ? (
        <div className="mb-2 text-sm text-muted-foreground">
          <AgentNameLabel
            name={message.agent}
            agent={resolveAgentInfo(message.agent, agents)}
          />
        </div>
      ) : null}
      <div className="message-parts">
        {(() => {
          const partKeys = new Map<string, number>();
          return parts.map((part) => {
            const key = nextRepeatedKey(
              partKeys,
              messagePartKey(part, message.id),
            );
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
      {showCopyAction && (hasContent || copyContent) ? (
        <MessageActions content={copyContent || message.content} compact />
      ) : null}
    </article>
  );
}, messageRowPropsEqual);

export function messageRowPropsEqual(
  previous: MessageRowProps,
  next: MessageRowProps,
) {
  return (
    previous.message === next.message &&
    previous.sessionID === next.sessionID &&
    previous.agents === next.agents &&
    previous.showAgentLabel === next.showAgentLabel &&
    previous.showCopyAction === next.showCopyAction &&
    previous.canAnswerAsk === next.canAnswerAsk &&
    previous.onAskAnswered === next.onAskAnswered &&
    messageRenderPartsEqual(previous.parts, next.parts)
  );
}

export function messageRenderPartsEqual(
  previous: MessageRenderPart[],
  next: MessageRenderPart[],
) {
  if (previous === next) return true;
  if (previous.length !== next.length) return false;
  return previous.every((part, index) => {
    const candidate = next[index];
    if (part.type !== candidate.type) return false;
    if (part.type === "text" || part.type === "reasoning") {
      return (
        candidate.type === part.type &&
        part.content === candidate.content &&
        part.key === candidate.key &&
        part.streaming === candidate.streaming
      );
    }
    if (part.type === "ask") {
      return (
        candidate.type === "ask" &&
        JSON.stringify(part.ask) === JSON.stringify(candidate.ask)
      );
    }
    return (
      candidate.type === "tool" &&
      JSON.stringify([part.tool, part.member]) ===
        JSON.stringify([candidate.tool, candidate.member])
    );
  });
}

export function UserAttachmentPills({
  attachments,
}: {
  attachments: ContentPartDTO[];
}) {
  return (
    <div className="flex max-w-full flex-wrap justify-end gap-2">
      {attachments.map((part, index) => {
        if (isImagePart(part))
          return (
            <UserImagePill
              key={attachmentPartKey(part, index)}
              part={part}
              index={index}
            />
          );
        return (
          <UserFilePill key={attachmentPartKey(part, index)} part={part} />
        );
      })}
    </div>
  );
}

export function UserImagePill({
  part,
  index,
}: {
  part: ContentPartDTO;
  index: number;
}) {
  const src = part.base64_data
    ? `data:${part.mime_type || "image/png"};base64,${part.base64_data}`
    : part.url || "";
  const label = imagePartLabel(part, index);
  return (
    <div className="flex h-12 max-w-[10.5rem] items-center gap-2 rounded-xl border border-border/70 bg-muted px-2 py-1.5 text-sm text-foreground sm:max-w-[13rem]">
      <div className="h-9 w-9 shrink-0 overflow-hidden rounded-lg bg-background/60">
        {src ? (
          <img className="h-full w-full object-cover" src={src} alt={label} />
        ) : null}
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

export function UserFilePill({ part }: { part: ContentPartDTO }) {
  const label = part.name || part.url || part.text || "附件";
  return (
    <div className="flex h-12 max-w-[10.5rem] items-center gap-2 rounded-xl border border-border/70 bg-muted px-3 py-1.5 text-sm text-foreground sm:max-w-[13rem]">
      <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate font-medium">{label}</span>
    </div>
  );
}

export function messageAttachmentParts(parts?: ContentPartDTO[]) {
  return (parts || []).filter((part) => part.type !== "text");
}

export function isImagePart(part: ContentPartDTO) {
  return (
    part.type === "image_url" ||
    part.type === "image_base64" ||
    Boolean(part.base64_data)
  );
}

export function attachmentPartKey(part: ContentPartDTO, index: number) {
  return `${part.type}:${part.name || ""}:${part.url || part.mime_type || ""}:${index}`;
}

export function imagePartLabel(part: ContentPartDTO, index: number) {
  if (part.name) return part.name;
  if (part.url) {
    const segments = part.url.split(/[\\/]/).filter(Boolean);
    return segments[segments.length - 1] || `图片 ${index + 1}`;
  }
  return `图片 ${index + 1}`;
}

export function MessagePart({
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
  if (part.type === "reasoning")
    return (
      <ReasoningBlock content={part.content} disclosureID={disclosureID} />
    );
  if (part.type === "tool") {
    const memberAgent = part.member
      ? resolveAgentInfo(part.member.name, agents)
      : undefined;
    return (
      <ToolCallCard
        disclosureID={disclosureID}
        tool={part.tool}
        title={
          part.member ? (
            <AgentNameLabel name={part.member.name} agent={memberAgent} loud />
          ) : undefined
        }
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
          canAnswer={canAnswerAsk && part.ask.active}
          onAnswered={(selected, freeText) =>
            onAskAnswered(part.ask, selected, freeText)
          }
        />
      </div>
    );
  }
  if (
    sessionID === assistantSessionID &&
    /(?:^|\n) {0,3}(?:`{3,}|~{3,})openui-lang(?:\s|$)/.test(part.content)
  ) {
    return (
      <Suspense
        fallback={
          <p className="text-sm text-muted-foreground">正在加载交互内容…</p>
        }
      >
        <AssistantUIContent
          content={part.content}
          sourceID={disclosureID}
          streaming={part.streaming}
        />
      </Suspense>
    );
  }
  return (
    <MarkdownContent
      className="text-base leading-8 sm:text-lg sm:leading-9"
      content={part.content}
    />
  );
}

export function ReasoningBlock({
  content,
  disclosureID,
}: {
  content: string;
  disclosureID: string;
}) {
  const [open, toggleOpen] = useDisclosureState(disclosureID);

  function handleToggle(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    event.stopPropagation();
    toggleOpen();
  }

  return (
    <div className="-ml-2 text-sm">
      <button
        className="flex min-h-11 items-center gap-3 rounded-lg px-2 py-2 text-left text-warning transition-colors hover:bg-warning-muted sm:min-h-10"
        onClick={handleToggle}
        type="button"
      >
        <span className="h-2 w-2 rounded-full bg-warning" />
        <span className="font-semibold">已思考</span>
        <ChevronRight
          className={cn("h-4 w-4 transition-transform", open && "rotate-90")}
        />
      </button>
      {open ? (
        <div className="ml-7 border-l border-warning/20 pl-4 pt-2 text-sm leading-7 text-muted-foreground">
          <div className="whitespace-pre-wrap">{content}</div>
        </div>
      ) : null}
    </div>
  );
}

export function ActivityList({
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
    <div>
      {visibleTools.map((part) => {
        if (part.type !== "tool") return null;
        const memberAgent = part.member
          ? resolveAgentInfo(part.member.name, agents)
          : undefined;
        const key = `activity:${toolActivityKey(part.tool)}`;
        return (
          <ToolCallCard
            key={key}
            disclosureID={key}
            tool={part.tool}
            title={
              part.member ? (
                <AgentNameLabel
                  name={part.member.name}
                  agent={memberAgent}
                  loud
                />
              ) : undefined
            }
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

export function MemberActivityBlock({
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
        className="flex min-h-11 items-center gap-3 rounded-lg px-2 py-2 text-left text-muted-foreground transition-colors hover:bg-muted/70 sm:min-h-10"
        onClick={handleToggle}
        type="button"
      >
        <span className="h-2 w-2 rounded-full bg-muted-foreground/35" />
        <AgentNameLabel name={member.name} agent={agent} loud />
        <ChevronRight
          className={cn("h-4 w-4 transition-transform", open && "rotate-90")}
        />
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

export function MemberActivityDetails({
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
    <div className="message-parts">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <GitBranch className="h-3.5 w-3.5" />
        <AgentNameLabel name={member.name} agent={agent} />
        <span>
          {member.toolCount ? `${member.toolCount} 个工具调用` : "暂无工具调用"}
        </span>
      </div>
      {(() => {
        const partKeys = new Map<string, number>();
        return member.parts.map((part) => {
          const key = nextRepeatedKey(
            partKeys,
            messagePartKey(part, member.id),
          );
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

export function MemberActivityPart({
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
  if (part.type === "reasoning")
    return (
      <ReasoningBlock content={part.content} disclosureID={disclosureID} />
    );
  if (part.type === "tool")
    return <ToolCallCard tool={part.tool} disclosureID={disclosureID} />;
  if (part.type === "ask") {
    return (
      <AskTimelineItem
        ask={part.ask}
        sessionID={part.ask.sessionID || sessionID}
        canAnswer={canAnswerAsk}
        onAnswered={(selected, freeText) =>
          onAskAnswered(part.ask, selected, freeText)
        }
      />
    );
  }
  if (part.type === "text")
    return (
      <MarkdownContent className="text-base leading-8" content={part.content} />
    );
  return null;
}

export function AgentNameLabel({
  name,
  agent,
  loud = false,
}: {
  name: string;
  agent?: AgentInfo;
  loud?: boolean;
}) {
  if (name === assistantAgentID) return <AssistantNameLabel loud={loud} />;
  const label = agentDisplayName(agent, name);
  return (
    <span
      className={cn(
        "inline-flex min-w-0 items-center gap-2",
        loud && "font-semibold tracking-normal",
      )}
    >
      <span className="truncate">{label}</span>
      {agent?.builtin ? <BuiltinAgentBadge /> : null}
    </span>
  );
}

function AssistantNameLabel({ loud }: { loud: boolean }) {
  const name = useAppSelector((state) =>
    assistantDisplayName(state.sessions.items),
  );
  return (
    <span className={cn("truncate", loud && "font-semibold")}>{name}</span>
  );
}

export function BuiltinAgentBadge() {
  return (
    <span className="shrink-0 rounded border border-primary/25 bg-accent px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary">
      内置
    </span>
  );
}

export function resolveAgentInfo(name: string, agents: AgentInfo[]) {
  const key = normalizeAgentKey(name);
  if (!key) return undefined;
  return agents.find((agent) => {
    if (normalizeAgentKey(agent.name) === key) return true;
    if (normalizeAgentKey(agent.display_name || "") === key) return true;
    return (agent.aliases || []).some(
      (alias) => normalizeAgentKey(alias) === key,
    );
  });
}

export function agentDisplayName(
  agent: AgentInfo | undefined,
  fallback: string,
) {
  return (
    agent?.display_name ||
    agent?.name ||
    runtimeAgentDisplayName(fallback) ||
    fallback
  );
}

export function runtimeAgentDisplayName(name: string) {
  return runtimeAgentDisplayNames[normalizeAgentKey(name)];
}

export const runtimeAgentDisplayNames: Record<string, string> = {
  deep_researcher: "深度研究员",
};

export function MessageActions({
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
  const [copied, showCopied] = useTimedState(false);

  async function copyContent() {
    try {
      await copyText(content);
    } catch {
      return;
    }
    showCopied(true);
  }

  return (
    <div
      className={cn(
        "flex items-center gap-2 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100",
        compact && "mt-2",
        copied && "opacity-100",
        align === "right" && "justify-end",
      )}
    >
      {time ? (
        <span className="px-1 text-sm text-muted-foreground">
          {formatTime(time)}
        </span>
      ) : null}
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

export function AskPanel({
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
  const [submitted, setSubmitted] = useState(false);
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
    !submitted &&
    (hasOptions ? selected.length > 0 || freeText.trim() : freeText.trim()),
  );

  function toggleOption(option: string) {
    setSelected((current) => {
      if (!ask.multiSelect) return current.includes(option) ? [] : [option];
      return current.includes(option)
        ? current.filter((item) => item !== option)
        : [...current, option];
    });
  }

  async function submit() {
    if (!canSubmit || !sessionID) return;
    setSubmitting(true);
    setError("");
    try {
      const text = freeText.trim();
      await submitAskResponse(sessionID, ask.id, { selected, free_text: text });
      setSubmitted(true);
      onAnswered(selected, text);
    } catch (submitError) {
      setError(
        submitError instanceof Error
          ? submitError.message
          : String(submitError),
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="sketch-surface rounded-xl bg-card px-3 py-3 shadow-surface sm:px-4 sm:py-4">
      <div className="mb-3 flex items-start gap-2 sm:gap-3">
        <CircleHelp className="mt-1 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">{askTitle(ask)}</div>
          <div className="mt-1 whitespace-pre-wrap text-base leading-7 text-foreground">
            {ask.question}
          </div>
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
                  canAnswer && !ask.answered
                    ? "cursor-pointer"
                    : "cursor-not-allowed opacity-65",
                  checked
                    ? "border-primary/25 bg-accent text-foreground"
                    : "border-border bg-background/60 hover:bg-muted/65",
                )}
              >
                <input
                  className="h-4 w-4 shrink-0 accent-primary"
                  type={ask.multiSelect ? "checkbox" : "radio"}
                  name={`ask-${ask.id}`}
                  checked={checked}
                  disabled={
                    !canAnswer || ask.answered || submitting || submitted
                  }
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
        disabled={!canAnswer || ask.answered || submitting || submitted}
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
          {ask.answered
            ? `已回答：${askResponseSummary(ask.selected, ask.freeText) || "空回答"}`
            : disabledReason || error}
        </div>
        <Button
          type="button"
          size="sm"
          onClick={() => void submit()}
          disabled={!canSubmit}
        >
          <Send className="h-4 w-4" />
          {submitting ? "提交中" : "提交回答"}
        </Button>
      </div>
    </section>
  );
}

export function AskRecord({ ask }: { ask: AskActivity }) {
  const answer = askResponseSummary(ask.selected, ask.freeText);
  return (
    <section className="rounded-xl border border-primary/20 bg-card/70 px-3 py-3 sm:px-4 sm:py-4">
      <div className="flex items-start gap-2 sm:gap-3">
        <CircleHelp className="mt-1 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">{askTitle(ask)}</div>
          <div className="mt-1 whitespace-pre-wrap text-base leading-7 text-foreground">
            {ask.question}
          </div>
          {ask.options.length ? (
            <div className="mt-3 flex flex-wrap gap-2">
              {ask.options.map((option) => {
                const selected = ask.selected.includes(option);
                return (
                  <span
                    key={option}
                    className={cn(
                      "rounded-md border px-2.5 py-1 text-sm",
                      selected
                        ? "border-primary/25 bg-accent text-primary"
                        : "border-border bg-background/55 text-muted-foreground",
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
            <span className="whitespace-pre-wrap text-foreground">
              {answer || "空回答"}
            </span>
          </div>
        </div>
      </div>
    </section>
  );
}

export function AskTimelineItem({
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
  return (
    <AskPanel
      ask={ask}
      sessionID={sessionID}
      canAnswer={canAnswer && ask.active}
      onAnswered={onAnswered}
    />
  );
}

export function ApprovalTimelineItem({
  approval,
  sessionID,
  canAnswer,
  onAnswered,
}: {
  approval: ApprovalActivity;
  sessionID?: string;
  canAnswer: boolean;
  onAnswered: ApprovalAnsweredHandler;
}) {
  if (approval.answered) return <ApprovalRecord approval={approval} />;
  return (
    <ApprovalPanel
      approval={approval}
      sessionID={sessionID}
      canAnswer={canAnswer && approval.active}
      onAnswered={onAnswered}
    />
  );
}

export function ApprovalPanel({
  approval,
  sessionID,
  canAnswer,
  onAnswered,
}: {
  approval: ApprovalActivity;
  sessionID?: string;
  canAnswer: boolean;
  onAnswered: ApprovalAnsweredHandler;
}) {
  const [submitted, setSubmitted] = useState(false);
  const [submitting, setSubmitting] = useState<0 | 1 | 2 | undefined>();
  const [error, setError] = useState("");
  const disabledReason = !canAnswer ? "当前任务已结束，无法提交审批。" : "";

  async function answer(decision: 0 | 1 | 2) {
    if (!canAnswer || !sessionID || submitting !== undefined || submitted)
      return;
    setSubmitting(decision);
    setError("");
    try {
      await submitApproval(sessionID, decision);
      setSubmitted(true);
      onAnswered(approval, decision);
    } catch (approvalError) {
      setError(
        approvalError instanceof Error
          ? approvalError.message
          : String(approvalError),
      );
    } finally {
      setSubmitting(undefined);
    }
  }

  return (
    <section className="sketch-surface rounded-xl border-warning/25 bg-warning-muted px-3 py-3 shadow-surface sm:px-4 sm:py-4">
      <div className="flex items-start gap-2 sm:gap-3">
        <ShieldAlert className="mt-1 h-4 w-4 shrink-0 text-warning" />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-semibold text-warning">需要权限审批</div>
          <div className="mt-1 whitespace-pre-wrap text-sm leading-7 text-foreground">
            {approval.message}
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
            <div className="text-sm text-muted-foreground">
              {error || disabledReason}
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void answer(0)}
                disabled={!canAnswer || submitting !== undefined || submitted}
              >
                {submitting === 0 ? "提交中" : "拒绝"}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => void answer(1)}
                disabled={!canAnswer || submitting !== undefined || submitted}
              >
                {submitting === 1 ? "提交中" : "允许一次"}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => void answer(2)}
                disabled={!canAnswer || submitting !== undefined || submitted}
              >
                {submitting === 2 ? "提交中" : "记住允许"}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

export function ApprovalRecord({ approval }: { approval: ApprovalActivity }) {
  return (
    <section className="rounded-xl border border-warning/20 bg-card/70 px-3 py-3 sm:px-4 sm:py-4">
      <div className="flex items-start gap-2 sm:gap-3">
        <ShieldAlert className="mt-1 h-4 w-4 shrink-0 text-warning" />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-semibold text-warning">权限审批</div>
          <div className="mt-1 whitespace-pre-wrap text-sm leading-7 text-muted-foreground">
            {approval.message}
          </div>
          <div className="mt-3 rounded-md border border-border bg-background/55 px-3 py-2 text-sm">
            <span className="text-muted-foreground">审批结果：</span>
            <span className="text-foreground">
              {approval.decision || "已处理"}
            </span>
          </div>
        </div>
      </div>
    </section>
  );
}

export function askTitle(ask: AskActivity) {
  if (ask.memberName) return `${ask.memberName} · ask_questions`;
  return "ask_questions";
}
