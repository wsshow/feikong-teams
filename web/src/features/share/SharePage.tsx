import { ConversationItems } from "@/features/chat/MessageList";
import { useEffect, useRef, useState } from "react";
import { ListTree } from "lucide-react";
import { accessPublicShare, getPublicShareInfo, type SessionShare } from "@/api/shares";
import { APIError } from "@/api/client";
import type { SessionDetail } from "@/types/chat";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";

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
          entries={(detail.view.items).flatMap(item => item.kind === "message" ? [item.node.message] : [])}
          expiresAt={info?.expires_at}
          title={title}
        />
      )}
    </div>
  );
}

interface ShareEntry { id: string; role: string; content: string }
function ShareThread({ detail, entries, expiresAt, title }: { detail: SessionDetail; entries: ShareEntry[]; expiresAt?: string | number; title: string }) {
  return <>
    <div className="mx-auto flex min-h-full w-full max-w-5xl flex-col">
      <header className="mb-6 rounded-2xl border border-border/70 bg-card px-4 py-4 sm:px-5">
        <div className="text-lg font-semibold">{detail.title || title || "共享会话"}</div>
        <div className="mt-1 text-sm text-muted-foreground">公开只读视图{expiresAt ? ` · 有效期至 ${formatTime(expiresAt)}` : ""}</div>
      </header>
      <main className="pb-8"><ConversationItems items={detail.view.items} /></main>
    </div>
    <ShareQuestionNavigator entries={entries} />
  </>;
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
          <div className="chat-scroll sketch-surface mb-2 max-h-[42vh] w-[min(calc(100vw-1.5rem),19.5rem)] overflow-y-auto rounded-xl bg-card p-2 shadow-popover backdrop-blur">
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
            "flex h-10 items-center gap-2 rounded-full border border-border bg-card px-4 text-sm font-semibold text-muted-foreground shadow-popover backdrop-blur transition-colors hover:bg-accent hover:text-foreground",
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
          className="chat-scroll sketch-surface pointer-events-none absolute right-0 top-1/2 max-h-72 w-64 -translate-y-1/2 overflow-y-auto rounded-xl bg-card p-2.5 opacity-0 shadow-popover backdrop-blur transition-opacity duration-150 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100"
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
            ? "border-primary/25 bg-accent text-primary"
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

function shareQuestionElementID(id: string) { return `chat-message-${id.replace(/[^a-zA-Z0-9_-]/g, "-")}`; }

function publicShareErrorMessage(error: unknown) {
  if (error instanceof APIError) {
    if (error.status === 404) return "这个分享已失效或已被取消";
    if (error.status === 410) return "这个分享已过期";
    if (error.status === 401) return "访问密码不正确";
    if (error.status === 409) return error.message;
  }
  const message = error instanceof Error ? error.message : String(error);
  if (/not found/i.test(message)) return "这个分享已失效或已被取消";
  if (/expired|gone/i.test(message)) return "这个分享已过期";
  if (/password|unauthorized/i.test(message)) return "访问密码不正确";
  return "暂时无法打开这个分享";
}
