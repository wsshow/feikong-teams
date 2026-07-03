import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowDown, CalendarClock, FolderOpen, ListTree, Settings, Share2, Sparkles } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { appActions, chatActions, type AppPanel } from "@/app/store";
import { loadSessionDetail } from "@/features/sessions/sessionThunks";
import { cn } from "@/lib/cn";
import { pushAppPath } from "@/lib/navigation";
import { MessageList } from "./MessageList";
import { chatMessageElementID } from "./dom";
import { ChatInput } from "./ChatInput";

export function ChatPage() {
  const dispatch = useAppDispatch();
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const runningSessionID = useAppSelector((state) => state.chat.runningSessionID);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const messages = useAppSelector((state) => state.chat.messages);
  const events = useAppSelector((state) => state.chat.events);
  const queue = useAppSelector((state) => state.chat.queue);
  const error = useAppSelector((state) => state.chat.error);
  const [loadedSessionID, setLoadedSessionID] = useState("");
  const [failedSessionID, setFailedSessionID] = useState("");
  const [showSessionLoading, setShowSessionLoading] = useState(false);
  const [referenceOpen, setReferenceOpen] = useState(false);
  const [jumpControls, setJumpControls] = useState({ distanceFromBottom: 0, jump: () => {} });
  const hasConversation = messages.length > 0 || events.length > 0 || queue.length > 0 || isProcessing || Boolean(error);
  const currentSessionIsRunning = Boolean(isProcessing && runningSessionID === activeSessionID);
  const currentViewBelongsToActiveSession = Boolean(
    activeSessionID && events.some((event) => event.session_id === activeSessionID),
  );
  const isLoadingSession = Boolean(
    activeSessionID &&
      activeSessionID !== loadedSessionID &&
      failedSessionID !== activeSessionID &&
      !(currentSessionIsRunning && currentViewBelongsToActiveSession),
  );

  useEffect(() => {
    if (!activeSessionID) {
      setLoadedSessionID("");
      setFailedSessionID("");
      return;
    }
    if (currentSessionIsRunning && currentViewBelongsToActiveSession) {
      setLoadedSessionID(activeSessionID);
      return;
    }
    if (activeSessionID === loadedSessionID) return;

    let cancelled = false;
    setFailedSessionID("");
    void dispatch(loadSessionDetail(activeSessionID))
      .unwrap()
      .then(() => {
        if (!cancelled) setLoadedSessionID(activeSessionID);
      })
      .catch((loadError) => {
        if (cancelled) return;
        setFailedSessionID(activeSessionID);
        dispatch(chatActions.setError(loadError instanceof Error ? loadError.message : String(loadError)));
      });

    return () => {
      cancelled = true;
    };
  }, [activeSessionID, loadedSessionID, currentSessionIsRunning, currentViewBelongsToActiveSession, dispatch]);

  useEffect(() => {
    dispatch(chatActions.setConnectionState("connected"));
  }, [dispatch]);

  useEffect(() => {
    if (!isLoadingSession) {
      setShowSessionLoading(false);
      return;
    }
    if (!hasConversation) {
      setShowSessionLoading(true);
      return;
    }
    const timer = window.setTimeout(() => setShowSessionLoading(true), 180);
    return () => window.clearTimeout(timer);
  }, [isLoadingSession, hasConversation]);

  const updateJumpControls = useCallback((controls: { distanceFromBottom: number; jump: () => void }) => {
    setJumpControls(controls);
  }, []);

  if (isLoadingSession && showSessionLoading) {
    return <ChatSessionLoading />;
  }

  if (!activeSessionID && !hasConversation) {
    return <ChatHome />;
  }

  return (
    <div className="relative flex h-full flex-col">
      <MessageList onJumpToBottomControlsChange={updateJumpControls} />
      <QuestionNavigator hideMobile={referenceOpen} jumpControls={jumpControls} />
      <ChatInput onReferenceOpenChange={setReferenceOpen} />
    </div>
  );
}

function ChatSessionLoading() {
  return (
    <div className="flex h-full items-center justify-center px-3 sm:px-6">
      <div className="sketch-surface flex w-full max-w-sm flex-col items-center gap-4 rounded-2xl px-6 py-8 text-center sm:px-8 sm:py-9">
        <div className="flex items-center gap-2">
          <span className="h-2 w-2 animate-pulse rounded-full bg-primary" />
          <span className="h-2 w-2 animate-pulse rounded-full bg-primary/70 [animation-delay:120ms]" />
          <span className="h-2 w-2 animate-pulse rounded-full bg-primary/45 [animation-delay:240ms]" />
        </div>
        <div className="text-base text-muted-foreground">正在打开会话</div>
      </div>
    </div>
  );
}

function QuestionNavigator({
  hideMobile,
  jumpControls,
}: {
  hideMobile: boolean;
  jumpControls: { distanceFromBottom: number; jump: () => void };
}) {
  const messages = useAppSelector((state) => state.chat.messages);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const mobileRef = useRef<HTMLDivElement | null>(null);
  const [activeQuestionID, setActiveQuestionID] = useState("");
  const [mobileOpen, setMobileOpen] = useState(false);
  const [mobileQuestionVisible, setMobileQuestionVisible] = useState(false);
  const [mobileBottomVisible, setMobileBottomVisible] = useState(false);
  const questions = messages.filter((message) => message.role === "user" && message.content.trim());
  const orderedQuestions = questions.map((question, index) => ({ question, index: index + 1 }));
  const latestQuestionID = orderedQuestions[orderedQuestions.length - 1]?.question.id || "";
  const visibleQuestions = orderedQuestions.slice(-8);
  const showMobileQuestion = !hideMobile && questions.length > 0 && mobileQuestionVisible;
  const showMobileBottom = !hideMobile && mobileBottomVisible;
  const showMobileControls = showMobileQuestion || showMobileBottom;

  useEffect(() => {
    if (!latestQuestionID) return;
    setActiveQuestionID(latestQuestionID);
    window.requestAnimationFrame(() => {
      const panel = panelRef.current;
      if (!panel) return;
      panel.scrollTo({ top: panel.scrollHeight });
    });
  }, [latestQuestionID]);

  useEffect(() => {
    if (!mobileOpen) return;
    const closeOnOutsidePointer = (event: PointerEvent) => {
      const root = mobileRef.current;
      if (root?.contains(event.target as Node)) return;
      setMobileOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutsidePointer);
    return () => document.removeEventListener("pointerdown", closeOnOutsidePointer);
  }, [mobileOpen]);

  useEffect(() => {
    if (hideMobile) setMobileOpen(false);
  }, [hideMobile]);

  useEffect(() => {
    if (hideMobile || questions.length === 0) {
      setMobileQuestionVisible(false);
      setMobileBottomVisible(false);
      setMobileOpen(false);
      return;
    }

    const distance = jumpControls.distanceFromBottom;
    setMobileQuestionVisible((visible) => (visible ? distance > 40 : distance > 72));
    setMobileBottomVisible((visible) => (visible ? distance > 160 : distance > 220));
  }, [hideMobile, jumpControls.distanceFromBottom, questions.length]);

  function jumpTo(messageID: string) {
    setActiveQuestionID(messageID);
    setMobileOpen(false);
    document.getElementById(chatMessageElementID(messageID))?.scrollIntoView({
      behavior: "smooth",
      block: "center",
    });
  }

  if (questions.length === 0 && !showMobileBottom) return null;

  return (
    <>
      {showMobileControls ? (
        <div
          ref={mobileRef}
          className="absolute left-1/2 z-[35] -translate-x-1/2 xl:hidden"
          style={{ bottom: "calc(var(--app-keyboard-inset-bottom,0px) + var(--chat-dock-height,10rem) + 1rem)" }}
        >
          {mobileOpen && showMobileQuestion ? (
            <div className="chat-scroll sketch-surface mb-2 max-h-[42vh] w-[min(calc(100vw-1.5rem),19.5rem)] animate-in fade-in slide-in-from-bottom-1 overflow-y-auto rounded-xl bg-card/95 p-2 shadow-[0_18px_48px_hsl(218_30%_20%/0.16)] backdrop-blur duration-150">
              <div className="space-y-0.5">
                {orderedQuestions.map(({ question, index }) => (
                  <QuestionButton
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
          <div className="flex justify-center">
            <div className="inline-flex w-fit animate-in items-center gap-1 rounded-full border border-border bg-card/95 p-1 shadow-[0_8px_24px_hsl(218_30%_25%/0.14)] backdrop-blur duration-150 fade-in slide-in-from-bottom-1">
              {showMobileBottom ? (
                <button
                  className="flex h-9 items-center gap-2 rounded-full px-3 text-sm font-semibold text-muted-foreground transition-[background,color,transform] hover:bg-accent hover:text-foreground active:translate-y-[1px]"
                  type="button"
                  onClick={jumpControls.jump}
                  aria-label="回到底部"
                >
                  <ArrowDown className="h-4 w-4" />
                  底部
                </button>
              ) : null}
              {showMobileQuestion ? (
                <button
                  className={cn(
                    "flex h-9 items-center gap-2 rounded-full px-3 text-sm font-semibold text-muted-foreground transition-[background,color,transform] hover:bg-accent hover:text-foreground active:translate-y-[1px]",
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
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
      {questions.length > 0 ? (
        <aside
          className="group absolute top-[42%] z-20 hidden -translate-y-1/2 xl:block"
          style={{ right: "0.75rem" }}
        >
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
                <QuestionButton
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
      ) : null}
    </>
  );
}

function QuestionButton({ active, index, content, onClick }: { active: boolean; index: number; content: string; onClick: () => void }) {
  return (
    <button
      className={cn(
        "flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-[13px] leading-5 transition-colors hover:bg-muted",
        active && "bg-muted text-primary",
      )}
      onClick={onClick}
      title={content}
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

function ChatHome() {
  const dispatch = useAppDispatch();
  const shortcuts: Array<{ label: string; icon: typeof FolderOpen; panel: AppPanel; path: string }> = [
    { label: "文件", icon: FolderOpen, panel: "files", path: "/files" },
    { label: "任务", icon: CalendarClock, panel: "schedules", path: "/schedules" },
    { label: "技能", icon: Sparkles, panel: "skills", path: "/skills" },
    { label: "分享", icon: Share2, panel: "shares", path: "/shares" },
    { label: "配置", icon: Settings, panel: "config", path: "/config" },
  ];

  function openPanel(panel: AppPanel, path: string) {
    dispatch(appActions.setActivePanel(panel));
    pushAppPath(path);
  }

  return (
    <div className="flex h-full items-center justify-center px-3 pb-6 pt-8 sm:px-6 sm:pb-10 sm:pt-20">
      <div className="w-full max-w-4xl">
        <div className="mb-6 flex flex-col items-center justify-center gap-3 text-center sm:mb-9 sm:flex-row sm:gap-4">
          <img className="h-10 w-10 shrink-0 drop-shadow-sm sm:h-12 sm:w-12 md:h-14 md:w-14" src="/assets/fkteams-logo.svg" alt="" />
          <h1 className="text-2xl font-semibold tracking-normal text-foreground sm:text-4xl md:text-5xl">
            {greeting()}，非空小队
          </h1>
        </div>
        <ChatInput variant="hero" className="mx-auto max-w-3xl" />
        <div className="mt-5 flex flex-wrap items-center justify-center gap-2">
          {shortcuts.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.panel}
                className="inline-flex h-9 items-center gap-2 rounded-lg border border-border bg-card/80 px-3 text-sm shadow-[1px_2px_0_hsl(218_32%_30%/0.08)] transition-colors hover:bg-accent/70"
                onClick={() => openPanel(item.panel, item.path)}
              >
                <Icon className="h-4 w-4" />
                {item.label}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function greeting() {
  const hour = new Date().getHours();
  if (hour < 6) return "夜深了";
  if (hour < 12) return "上午好";
  if (hour < 18) return "下午好";
  return "晚上好";
}
