import { type MouseEvent, useEffect, useRef, useState } from "react";
import { Check, ChevronRight, Copy, FileText, ListTree } from "lucide-react";
import { accessPublicShare, getPublicShareInfo, type SessionShare } from "@/api/shares";
import { APIError } from "@/api/client";
import type { SessionDetail } from "@/types/chat";
import type { ChatEvent, ContentPartDTO, ToolCallDTO } from "@/types/events";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";
import { ToolCallCard } from "@/features/chat/ToolCallCard";
import { useDisclosureState } from "@/features/chat/disclosureState";

export function SharePage() {
  const shareID = decodeURIComponent(location.pathname.split("/").filter(Boolean).pop() || "");
  const [info, setInfo] = useState<SessionShare | null>(null);
  const [title, setTitle] = useState("");
  const [password, setPassword] = useState("");
  const [detail, setDetail] = useState<SessionDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!shareID) return;
    let cancelled = false;
    async function init() {
      setLoading(true);
      setError("");
      try {
        const nextInfo = await getPublicShareInfo(shareID);
        if (cancelled) return;
        setInfo(nextInfo);
        setTitle(nextInfo.title || nextInfo.id || nextInfo.share_id || shareID);
        if (!nextInfo.has_password) {
          const nextDetail = await accessPublicShare(shareID, "");
          if (!cancelled) setDetail(nextDetail);
        }
      } catch (err) {
        if (!cancelled) setError(publicShareErrorMessage(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void init();
    return () => {
      cancelled = true;
    };
  }, [shareID]);

  async function load() {
    setError("");
    setLoading(true);
    try {
      setDetail(await accessPublicShare(shareID, password));
    } catch (err) {
      setError(publicShareErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="relative h-[var(--app-viewport-height,100dvh)] overflow-y-auto bg-muted/30 px-4 py-10">
      {!detail ? (
        <div className="flex min-h-[calc(var(--app-viewport-height,100dvh)-5rem)] items-center justify-center">
          <Panel className="w-full max-w-xl">
            <PanelHeader className="text-center">
              <div className="text-lg font-semibold">{title || "共享会话"}</div>
              <div className="mt-1 text-sm text-muted-foreground">
                {info?.message_count ? `${info.message_count} 条消息 · ` : ""}
                {info?.expires_at ? `有效期至 ${formatTime(info.expires_at)}` : "公开只读视图"}
              </div>
            </PanelHeader>
            <PanelBody className="space-y-4">
              {info?.has_password ? (
                <div className="mx-auto flex w-full max-w-md flex-col gap-3 sm:flex-row">
                  <Input
                    className="min-w-0 flex-1"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") void load();
                    }}
                    placeholder="输入访问密码"
                    type="password"
                  />
                  <Button className="min-w-24 whitespace-nowrap" onClick={() => void load()} disabled={loading}>
                    {loading ? "查看中" : "查看分享"}
                  </Button>
                </div>
              ) : error ? (
                <div className="mx-auto max-w-md py-8 text-center">
                  <div className="text-base font-semibold text-foreground">{error}</div>
                  <div className="mt-2 text-sm leading-6 text-muted-foreground">
                    可以回到原会话重新创建分享，或联系分享创建者确认链接是否仍然有效。
                  </div>
                </div>
              ) : (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  {loading ? "正在打开分享..." : "正在准备分享内容..."}
                </div>
              )}
              {error && info?.has_password ? <div className="text-center text-sm text-destructive">{error}</div> : null}
            </PanelBody>
          </Panel>
        </div>
      ) : (
        <ShareThread
          detail={detail}
          entries={shareEntriesFromEvents(detail.events || [])}
          expiresAt={info?.expires_at}
          title={title}
        />
      )}
    </div>
  );
}

function ShareThread({
  detail,
  entries,
  expiresAt,
  title,
}: {
  detail: SessionDetail;
  entries: ShareEntry[];
  expiresAt?: string | number;
  title: string;
}) {
  return (
    <>
      <div className="mx-auto flex min-h-full w-full max-w-5xl flex-col">
        <header className="mb-6 rounded-2xl border border-border/70 bg-card/75 px-4 py-4 shadow-[0_10px_28px_hsl(218_30%_25%/0.08)] sm:px-5">
          <div className="text-lg font-semibold text-foreground">{detail.title || title || "共享会话"}</div>
          <div className="mt-1 text-sm text-muted-foreground">
            公开只读视图
            {detail.message_count ? ` · ${detail.message_count} 条消息` : ""}
            {expiresAt ? ` · 有效期至 ${formatTime(expiresAt)}` : ""}
          </div>
        </header>
        <main className="space-y-3 pb-8 sm:space-y-4">
          {entries.map((entry) => (
            <ShareMessageRow key={entry.id} entry={entry} />
          ))}
        </main>
      </div>
      <ShareQuestionNavigator entries={entries} />
    </>
  );
}

interface ShareEntry {
  id: string;
  role: "user" | "assistant" | "system" | "tool" | "error";
  agent?: string;
  content: string;
  reasoning?: string;
  contentParts?: ContentPartDTO[];
  order: number;
  tool?: ToolCallDTO;
  showAgentLabel?: boolean;
}

interface ShareAgentInfo {
  displayName: string;
  builtin: boolean;
}

function shareEntriesFromEvents(events: ChatEvent[]) {
  const entries: ShareEntry[] = [];
  const byID = new Map<string, ShareEntry>();
  const toolEntries = new Map<string, ShareEntry>();
  const seenUserKeys = new Set<string>();
  for (const event of orderedShareEvents(events)) {
    if (event.type === "user_message") {
      const content = eventText(event);
      const parts = Array.isArray(event.content_parts) ? event.content_parts : [];
      const key = event.event_id || `${event.sequence || entries.length}:${content}:${parts.length}`;
      if (seenUserKeys.has(key)) continue;
      seenUserKeys.add(key);
      if (!content.trim() && parts.length === 0) continue;
      entries.push({
        id: `user-${key}`,
        role: "user",
        content,
        contentParts: parts,
        order: shareEventOrder(event),
      });
      continue;
    }
    if (isShareSystemEvent(event)) {
      const content = shareEventContent(event);
      if (!content.trim()) continue;
      entries.push({
        id: `${event.type}-${event.event_id || event.sequence || entries.length}`,
        role: event.type === "error" ? "error" : "system",
        content,
        order: shareEventOrder(event),
      });
      continue;
    }
    if (event.type === "tool_call_started" || event.type === "tool_call_completed" || event.type === "tool_call_failed") {
      const tool = shareToolFromEvent(event);
      const id = `tool-${tool.ref || tool.id || event.event_id || event.sequence || entries.length}`;
      const current = toolEntries.get(id);
      if (current) {
        current.tool = mergeShareTool(current.tool, tool);
        current.content = current.tool.result || current.content;
        current.order = Math.min(current.order, shareEventOrder(event));
      } else {
        const entry: ShareEntry = {
          id,
          role: "tool",
          agent: tool.display_name || tool.name,
          content: tool.result || "",
          order: shareEventOrder(event),
          tool,
        };
        toolEntries.set(id, entry);
        entries.push(entry);
      }
      continue;
    }
    if (!isShareAssistantEvent(event)) continue;
    const id = shareEventMessageID(event);
    let entry = byID.get(id);
    if (!entry) {
      entry = {
        id,
        role: "assistant",
        agent: String(event.member_name || event.agent_name || ""),
        content: "",
        reasoning: "",
        order: shareEventOrder(event),
      };
      byID.set(id, entry);
      entries.push(entry);
    }
    entry.order = Math.min(entry.order, shareEventOrder(event));
    const reasoning = String(event.reasoning_content || (event.type === "assistant_reasoning_delta" ? event.content || "" : ""));
    if (reasoning) entry.reasoning = appendShareContent(entry.reasoning || "", reasoning);
    const content = event.type === "assistant_reasoning_delta" ? "" : shareEventContent(event);
    if (content) entry.content = appendShareContent(entry.content, content);
  }
  return markShareAgentLabels(entries
    .filter((entry) => entry.role === "tool" || entry.content.trim() || entry.reasoning?.trim() || (entry.contentParts?.length ?? 0) > 0)
    .sort((left, right) => left.order - right.order));
}

function ShareMessageRow({ entry }: { entry: ShareEntry }) {
  if (entry.role === "user") {
    const attachments = shareAttachmentParts(entry.contentParts);
    return (
      <article id={shareQuestionElementID(entry.id)} className="flex w-full scroll-mt-8 flex-col items-end gap-2">
        <div className="flex max-w-[92%] flex-col items-end gap-2 sm:max-w-[78%]">
          {attachments.length ? <ShareAttachmentPills attachments={attachments} /> : null}
          {entry.content.trim() ? (
            <div className="rounded-2xl bg-muted px-4 py-3 text-base leading-7 text-foreground sm:px-5 sm:py-4 sm:text-lg sm:leading-8">
              <div className="whitespace-pre-wrap">{entry.content}</div>
            </div>
          ) : null}
        </div>
      </article>
    );
  }

  if (entry.role === "system" || entry.role === "error") {
    return (
      <article className={cn("rounded-xl px-4 py-3 text-sm leading-7", entry.role === "error" ? "border border-destructive/35 bg-destructive/5 text-destructive" : "bg-muted/45 text-muted-foreground")}>
        <div className="mb-1 font-medium">{entry.role === "error" ? "错误" : "系统"}</div>
        <div className="whitespace-pre-wrap text-base">{entry.content}</div>
      </article>
    );
  }

  if (entry.role === "tool") {
    if (!entry.tool) return null;
    return <ToolCallCard disclosureID={`share:${entry.id}`} tool={entry.tool} title={shareToolTitle(entry.tool)} />;
  }

  return (
    <article className="group w-full">
      {entry.agent && entry.showAgentLabel ? <ShareAgentLabel name={entry.agent} /> : null}
      <div className="space-y-2">
        {entry.reasoning?.trim() ? (
          <ShareReasoningBlock content={entry.reasoning} disclosureID={`share:reasoning:${entry.id}`} />
        ) : null}
        {entry.content.trim() ? (
          <MarkdownContent className="text-base leading-8 sm:text-lg sm:leading-9" content={entry.content} />
        ) : null}
      </div>
      {entry.content.trim() ? <ShareMessageActions content={entry.content} /> : null}
    </article>
  );
}

function markShareAgentLabels(entries: ShareEntry[]) {
  let previousAgent = "";
  return entries.map((entry) => {
    if (entry.role === "user" || entry.role === "system" || entry.role === "error") {
      previousAgent = "";
      return { ...entry, showAgentLabel: false };
    }
    if (entry.role !== "assistant" || !entry.agent) return { ...entry, showAgentLabel: false };
    const currentAgent = normalizeShareAgentKey(entry.agent);
    const showAgentLabel = currentAgent !== previousAgent;
    previousAgent = currentAgent;
    return { ...entry, showAgentLabel };
  });
}

function ShareAgentLabel({ name }: { name: string }) {
  const agent = shareAgentInfo(name);
  return (
    <div className="mb-2 text-sm text-muted-foreground">
      <span className="inline-flex min-w-0 items-center gap-2">
        <span className="truncate">{agent.displayName}</span>
        {agent.builtin ? <ShareBuiltinAgentBadge /> : null}
      </span>
    </div>
  );
}

function ShareBuiltinAgentBadge() {
  return (
    <span className="shrink-0 rounded border border-primary/25 bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary">
      内置
    </span>
  );
}

function shareAgentInfo(name: string): ShareAgentInfo {
  const normalized = normalizeShareAgentKey(name);
  const runtime = shareRuntimeAgents[normalized];
  if (runtime) return runtime;
  return { displayName: name, builtin: false };
}

function normalizeShareAgentKey(value: string) {
  return value.trim().toLowerCase();
}

const shareRuntimeAgents: Record<string, ShareAgentInfo> = {
  coordinator: { displayName: "协调者", builtin: true },
  deep_researcher: { displayName: "深度研究员", builtin: true },
};

function ShareMessageActions({ content }: { content: string }) {
  const [copied, setCopied] = useState(false);

  async function copyContent() {
    await navigator.clipboard?.writeText(content);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <div className={cn("mt-1 h-0 overflow-visible opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100", copied && "opacity-100")}>
      <button
        className={cn(
          "flex h-8 items-center gap-1.5 rounded-lg px-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
          copied && "bg-muted text-foreground",
        )}
        aria-label={copied ? "已复制" : "复制"}
        onClick={() => void copyContent()}
        type="button"
      >
        {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
        {copied ? <span className="text-xs">已复制</span> : null}
      </button>
    </div>
  );
}

function ShareReasoningBlock({ content, disclosureID }: { content: string; disclosureID: string }) {
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

function ShareQuestionNavigator({ entries }: { entries: ShareEntry[] }) {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [activeQuestionID, setActiveQuestionID] = useState("");
  const [mobileOpen, setMobileOpen] = useState(false);
  const questions = entries.filter((entry) => entry.role === "user" && entry.content.trim());
  const orderedQuestions = questions.map((question, index) => ({ question, index: index + 1 }));
  const visibleQuestions = orderedQuestions.slice(-8);
  const latestQuestionID = orderedQuestions[orderedQuestions.length - 1]?.question.id || "";

  useEffect(() => {
    if (!latestQuestionID) return;
    setActiveQuestionID(latestQuestionID);
    window.requestAnimationFrame(() => panelRef.current?.scrollTo({ top: panelRef.current.scrollHeight }));
  }, [latestQuestionID]);

  function jumpTo(messageID: string) {
    setActiveQuestionID(messageID);
    setMobileOpen(false);
    document.getElementById(shareQuestionElementID(messageID))?.scrollIntoView({
      behavior: "smooth",
      block: "center",
    });
  }

  if (questions.length === 0) return null;

  return (
    <>
      <div className="fixed bottom-4 left-1/2 z-30 -translate-x-1/2 xl:hidden">
        {mobileOpen ? (
          <div className="chat-scroll sketch-surface mb-2 max-h-[42vh] w-[min(calc(100vw-1.5rem),19.5rem)] overflow-y-auto rounded-xl bg-card/95 p-2 shadow-[0_18px_48px_hsl(218_30%_20%/0.16)] backdrop-blur">
            <div className="space-y-0.5">
              {orderedQuestions.map(({ question, index }) => (
                <ShareQuestionButton
                  key={question.id}
                  active={activeQuestionID === question.id}
                  index={index}
                  content={question.content}
                  onClick={() => jumpTo(question.id)}
                />
              ))}
            </div>
          </div>
        ) : null}
        <button
          className={cn(
            "flex h-10 items-center gap-2 rounded-full border border-border bg-card/95 px-4 text-sm font-semibold text-muted-foreground shadow-[0_8px_24px_hsl(218_30%_25%/0.14)] backdrop-blur transition-colors hover:bg-accent hover:text-foreground",
            mobileOpen && "bg-accent text-foreground",
          )}
          type="button"
          onClick={() => setMobileOpen((open) => !open)}
          aria-expanded={mobileOpen}
          aria-label="问题导航"
        >
          <ListTree className="h-4 w-4" />
          问题 {orderedQuestions.length}
        </button>
      </div>
      <aside className="group fixed right-3 top-[42%] z-20 hidden -translate-y-1/2 xl:block">
        <div className="flex min-h-20 w-7 items-center justify-center">
          <div className="space-y-3 rounded-full bg-background/55 px-2 py-2.5 backdrop-blur-sm">
            {visibleQuestions.map(({ question, index }) => (
              <button
                key={question.id}
                className={cn(
                  "block h-[2px] rounded-full transition-all hover:w-5 hover:bg-primary",
                  activeQuestionID === question.id ? "w-5 bg-primary" : "w-3.5 bg-muted-foreground/35",
                )}
                onClick={() => jumpTo(question.id)}
                aria-label={`跳转到问题 ${index}`}
                title={question.content}
              />
            ))}
          </div>
        </div>
        <div
          ref={panelRef}
          className="chat-scroll sketch-surface pointer-events-none absolute right-0 top-1/2 max-h-72 w-64 -translate-y-1/2 overflow-y-auto rounded-xl bg-card/95 p-2.5 opacity-0 shadow-[0_18px_48px_hsl(218_30%_20%/0.16)] backdrop-blur transition-opacity duration-150 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100"
        >
          <div className="space-y-0.5">
            {orderedQuestions.map(({ question, index }) => (
              <ShareQuestionButton
                key={question.id}
                active={activeQuestionID === question.id}
                index={index}
                content={question.content}
                onClick={() => jumpTo(question.id)}
              />
            ))}
          </div>
        </div>
      </aside>
    </>
  );
}

function ShareQuestionButton({ active, index, content, onClick }: { active: boolean; index: number; content: string; onClick: () => void }) {
  return (
    <button
      className={cn(
        "flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-[13px] leading-5 transition-colors hover:bg-muted",
        active && "bg-muted text-primary",
      )}
      onClick={onClick}
      title={content}
      type="button"
    >
      <span
        className={cn(
          "flex h-4 min-w-4 shrink-0 items-center justify-center rounded-full border text-[10px] leading-none",
          active
            ? "border-primary/60 bg-primary/10 text-primary"
            : "border-border/70 bg-muted/55 text-muted-foreground",
        )}
      >
        {index}
      </span>
      <span className="min-w-0 flex-1 truncate text-foreground/90">{content}</span>
      <span
        className={cn(
          "h-[2px] shrink-0 rounded-full",
          active ? "w-4 bg-primary" : "w-2.5 bg-muted-foreground/35",
        )}
        aria-hidden="true"
      />
    </button>
  );
}

function ShareAttachmentPills({ attachments }: { attachments: ContentPartDTO[] }) {
  return (
    <div className="flex max-w-full flex-wrap justify-end gap-2">
      {attachments.map((part, index) => {
        if (isShareImagePart(part)) return <ShareImagePill key={shareAttachmentKey(part, index)} part={part} index={index} />;
        return <ShareFilePill key={shareAttachmentKey(part, index)} part={part} index={index} />;
      })}
    </div>
  );
}

function ShareImagePill({ part, index }: { part: ContentPartDTO; index: number }) {
  const src = part.base64_data
    ? `data:${part.mime_type || "image/png"};base64,${part.base64_data}`
    : part.url || "";
  const label = shareAttachmentLabel(part, index);
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

function ShareFilePill({ part, index }: { part: ContentPartDTO; index: number }) {
  return (
    <div className="flex h-12 max-w-[10.5rem] items-center gap-2 rounded-xl border border-border/70 bg-muted px-3 py-1.5 text-sm text-foreground sm:max-w-[13rem]">
      <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate font-medium">{shareAttachmentLabel(part, index)}</span>
    </div>
  );
}

function shareAttachmentParts(parts?: ContentPartDTO[]) {
  return (parts || []).filter((part) => part.type !== "text");
}

function shareAttachmentKey(part: ContentPartDTO, index: number) {
  return `${part.type}:${part.name || ""}:${part.url || part.mime_type || ""}:${index}`;
}

function shareAttachmentLabel(part: ContentPartDTO, index: number) {
  if (part.name) return part.name;
  if (part.url) {
    const segments = part.url.split(/[\\/]/).filter(Boolean);
    return segments[segments.length - 1] || `附件 ${index + 1}`;
  }
  if (part.mime_type) return part.mime_type;
  return `附件 ${index + 1}`;
}

function isShareImagePart(part: ContentPartDTO) {
  return part.type === "image_url" || part.type === "image_base64" || Boolean(part.base64_data);
}

function appendShareContent(current: string, next: string) {
  if (!next) return current;
  if (!current) return next;
  return current.endsWith("\n") || next.startsWith("\n") ? `${current}${next}` : `${current}\n\n${next}`;
}

function orderedShareEvents(events: ChatEvent[]) {
  return [...events].sort((left, right) => shareEventOrder(left) - shareEventOrder(right));
}

function shareEventOrder(event: ChatEvent) {
  const order = event.sequence;
  if (typeof order === "number") return order;
  const transcriptIndex = event.transcript_index;
  if (typeof transcriptIndex === "number") return transcriptIndex + 0.5;
  return Number.MAX_SAFE_INTEGER;
}

function shareEventMessageID(event: ChatEvent) {
  return String(event.message_id || event.member_call_id || event.stream_id || event.event_id || event.sequence || "assistant");
}

function shareQuestionElementID(id: string) {
  return `share-question-${id.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
}

function isShareAssistantEvent(event: ChatEvent) {
  return (
    event.type === "assistant_completed" ||
    event.type === "assistant_text_delta" ||
    event.type === "assistant_reasoning_delta"
  );
}

function shareEventContent(event: ChatEvent) {
  return eventText(event);
}

function shareToolFromEvent(event: ChatEvent): ToolCallDTO {
  const direct = typeof event.tool_call === "object" && event.tool_call ? event.tool_call as ToolCallDTO : undefined;
  const nested = Array.isArray(event.tool_calls) ? event.tool_calls[0] : undefined;
  const source = direct || nested;
  const functionName = toolFunctionName(source);
  const name = firstNonEmpty(source?.name, functionName, event.tool_name);
  const displayName = firstNonEmpty(source?.display_name, event.tool_display_name, name, "工具调用");
  return {
    id: source?.id || event.tool_call_id || event.tool_call_ref || event.event_id,
    ref: source?.ref || event.tool_call_ref || event.tool_call_id || event.event_id,
    index: source?.index ?? event.tool_call_index,
    name: name || "tool",
    display_name: displayName,
    kind: source?.kind || event.tool_kind || "tool",
    target: source?.target || event.tool_target,
    arguments: source?.arguments || toolFunctionArguments(source) || event.tool_args || "",
    result: source?.result || event.tool_result || event.content || "",
    status: event.type === "tool_call_failed" ? "error" : event.type === "tool_call_started" ? "completed" : "completed",
    agent_name: source?.agent_name || event.agent_name,
    member_name: source?.member_name || event.member_name,
    content: source?.content || event.content,
  };
}

function mergeShareTool(current: ToolCallDTO | undefined, next: ToolCallDTO): ToolCallDTO {
  if (!current) return next;
  return {
    ...current,
    ...next,
    name: meaningfulToolName(next.name) ? next.name : current.name,
    display_name: meaningfulToolDisplay(next.display_name) ? next.display_name : current.display_name,
    kind: next.kind || current.kind,
    target: next.target || current.target,
    arguments: next.arguments || current.arguments,
    result: next.result || current.result,
    status: next.status || current.status,
  };
}

function shareToolTitle(tool: ToolCallDTO) {
  const displayName = String(tool.display_name || "").trim();
  const name = String(tool.name || "").trim();
  if (displayName && displayName !== "工具调用") return displayName;
  if (name && name !== "tool") return name;
  return "工具调用";
}

function meaningfulToolName(value?: string) {
  const normalized = String(value || "").trim();
  return normalized !== "" && normalized !== "tool";
}

function meaningfulToolDisplay(value?: string) {
  const normalized = String(value || "").trim();
  return normalized !== "" && normalized !== "工具调用";
}

function toolFunctionName(source?: ToolCallDTO) {
  const fn = toolFunctionPayload(source);
  return typeof fn?.name === "string" ? fn.name : "";
}

function toolFunctionArguments(source?: ToolCallDTO) {
  const fn = toolFunctionPayload(source);
  return typeof fn?.arguments === "string" ? fn.arguments : "";
}

function toolFunctionPayload(source?: ToolCallDTO) {
  if (!source || typeof source !== "object") return undefined;
  const value = (source as { function?: unknown }).function;
  return value && typeof value === "object" ? value as { name?: unknown; arguments?: unknown } : undefined;
}

function firstNonEmpty(...values: Array<unknown>) {
  for (const value of values) {
    const text = String(value || "").trim();
    if (text) return text;
  }
  return "";
}

function isShareSystemEvent(event: ChatEvent) {
  return event.type === "system_notice" || event.type === "error" || event.type === "cancelled";
}

function eventText(event: ChatEvent) {
  return String(event.content || event.message || "");
}

function publicShareErrorMessage(error: unknown) {
  if (error instanceof APIError) {
    if (error.status === 404) return "这个分享已失效或已被取消";
    if (error.status === 410) return "这个分享已过期";
    if (error.status === 401) return "访问密码不正确";
  }
  const message = error instanceof Error ? error.message : String(error);
  if (/not found/i.test(message)) return "这个分享已失效或已被取消";
  if (/expired|gone/i.test(message)) return "这个分享已过期";
  if (/password|unauthorized/i.test(message)) return "访问密码不正确";
  return "暂时无法打开这个分享";
}
