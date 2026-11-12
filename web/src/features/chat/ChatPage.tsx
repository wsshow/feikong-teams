import { memo, useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { shallowEqual } from "react-redux";
import { ArrowDown, Bot, CalendarClock, FolderOpen, ListTree, Settings, Share2, Sparkles } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { appActions, chatActions, sessionsActions, type AppPanel } from "@/app/store";
import { LoadingSurface } from "@/components/ui/loading-surface";
import { Button } from "@/components/ui/button";
import { assistantDisplayName, assistantSessionID } from "@/lib/assistant";
import { loadSessionDetail } from "@/features/sessions/sessionThunks";
import { cn } from "@/lib/cn";
import { errorMessage } from "@/lib/errors";
import { pushAppPath } from "@/lib/navigation";
import { MessageList } from "./MessageList";
import { chatMessageElementID } from "./dom";
import { ChatInput } from "./ChatInput";
import { decodeUIAction } from "./openui/protocol";
import { useActiveQuestion } from "./useActiveQuestion";

export function ChatPage() {
  const dispatch = useAppDispatch();
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const viewSessionID = useAppSelector((state) => state.chat.viewSessionID);
  const isProcessing = useAppSelector((state) => Boolean(
    state.chat.activeSessionID && state.chat.runningTasks[state.chat.activeSessionID],
  ));
  const messageCount = useAppSelector((state) => state.chat.messages.length);
  const itemCount = useAppSelector((state) => state.chat.presentation.items.length);
  const queueCount = useAppSelector((state) => state.chat.queue.length);
  const error = useAppSelector((state) => state.chat.error);
  const [failedSessionID, setFailedSessionID] = useState("");
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [showSessionLoading, setShowSessionLoading] = useState(false);
  const [referenceOpen, setReferenceOpen] = useState(false);
  const [jumpControls, setJumpControls] = useState({ distanceFromBottom: 0, jump: () => {} });
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const hasConversation = messageCount > 0 || itemCount > 0 || queueCount > 0 || isProcessing || Boolean(error);
  const isLoadingSession = Boolean(
    activeSessionID &&
      activeSessionID !== viewSessionID &&
      failedSessionID !== activeSessionID,
  );

  useEffect(() => {
    if (!activeSessionID) {
      setFailedSessionID("");
      return;
    }
    if (activeSessionID === viewSessionID) return;

    const loadingSessionID = activeSessionID;
    let cancelled = false;
    setFailedSessionID("");
    void dispatch(loadSessionDetail(loadingSessionID))
      .unwrap()
      .then((detail) => {
        if (cancelled) return;
        dispatch(chatActions.setSessionDetail(detail));
        dispatch(sessionsActions.updateSessionRuntime({
          sessionID: detail.session_id,
          status: detail.status,
          activeTask: Boolean(detail.active_task),
        }));
      })
      .catch((loadError) => {
        if (cancelled) return;
        setFailedSessionID(loadingSessionID);
        dispatch(chatActions.setError(errorMessage(loadError, "暂时无法打开这个会话，请稍后重试。")));
      });
    return () => {
      cancelled = true;
    };
  }, [activeSessionID, viewSessionID, dispatch, loadAttempt]);

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

  if (isLoadingSession && (showSessionLoading || activeSessionID === assistantSessionID)) {
    return <ChatSessionLoading />;
  }

  if (!activeSessionID && !hasConversation) {
    return <ChatHome />;
  }

  if (activeSessionID === assistantSessionID && failedSessionID === activeSessionID) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 px-6 text-center">
        <p className="text-muted-foreground">{error || "暂时无法打开助手"}</p>
        <Button variant="outline" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>重新打开</Button>
      </div>
    );
  }

  if (activeSessionID === assistantSessionID && !isLoadingSession && !hasConversation) {
    return <AssistantHome />;
  }

  return (
    <div className="relative flex h-full flex-col">
      <MessageList scrollRef={scrollRef} onJumpToBottomControlsChange={updateJumpControls} />
      <QuestionNavigator scrollRef={scrollRef} hideMobile={referenceOpen} jumpControls={jumpControls} />
      <ChatInput key={activeSessionID === assistantSessionID ? "assistant" : "chat"} onReferenceOpenChange={setReferenceOpen} />
    </div>
  );
}

function AssistantHome() {
  const name = useAppSelector((state) => assistantDisplayName(state.sessions.items));
  return (
    <div className="flex h-full items-center justify-center overflow-y-auto px-4 py-8 sm:px-6">
      <div className="w-full max-w-3xl space-y-6">
        <div className="space-y-3 text-center">
          <Bot className="mx-auto h-8 w-8 text-primary" aria-hidden="true" />
          <h1 className="break-words text-2xl font-semibold tracking-tight sm:text-3xl">今天和{name}一起做什么？</h1>
          <p className="text-sm leading-7 text-muted-foreground sm:text-base">整理工作区文件，查找过去的讨论，或直接交给我一项任务。</p>
        </div>
        <ChatInput variant="hero" />
        <p className="text-center text-xs leading-6 text-muted-foreground">输入 / 清空上下文或重命名助手，也可以直接告诉我「以后叫你小空」。</p>
      </div>
    </div>
  );
}

function ChatSessionLoading() {
  return (
    <div className="flex h-full items-center justify-center px-4">
      <LoadingSurface label="正在打开会话" />
    </div>
  );
}

const QuestionNavigator = memo(function QuestionNavigator({
  scrollRef,
  hideMobile,
  jumpControls,
}: {
  scrollRef: RefObject<HTMLDivElement | null>;
  hideMobile: boolean;
  jumpControls: { distanceFromBottom: number; jump: () => void };
}) {
  const questions = useAppSelector(
    (state) => state.chat.messages.filter((message) => message.role === "user" && !message.hidden && Boolean(message.content.trim())),
    shallowEqual,
  );
  const panelRef = useRef<HTMLDivElement | null>(null);
  const mobilePanelRef = useRef<HTMLDivElement | null>(null);
  const mobileRef = useRef<HTMLDivElement | null>(null);
  const activeQuestionID = useActiveQuestion(scrollRef, questions);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [mobileQuestionVisible, setMobileQuestionVisible] = useState(false);
  const [mobileBottomVisible, setMobileBottomVisible] = useState(false);
  const orderedQuestions = questions.map((question, index) => ({ question: { ...question, content: decodeUIAction(question.content)?.message ?? question.content }, index: index + 1 }));
  const activeQuestionIndex = questions.findIndex((question) => question.id === activeQuestionID);
  const visibleStart = Math.max(0, Math.min(activeQuestionIndex - 3, orderedQuestions.length - 8));
  const visibleQuestions = orderedQuestions.slice(visibleStart, visibleStart + 8);
  const showMobileQuestion = !hideMobile && questions.length > 0 && mobileQuestionVisible;
  const showMobileBottom = !hideMobile && mobileBottomVisible;
  const showMobileControls = showMobileQuestion || showMobileBottom;

  useEffect(() => {
    for (const panel of [panelRef.current, mobilePanelRef.current]) {
      const activeButton = panel?.querySelector<HTMLButtonElement>('[aria-current="location"]');
      if (!panel || !activeButton) continue;
      if (activeButton.offsetTop < panel.scrollTop || activeButton.offsetTop + activeButton.offsetHeight > panel.scrollTop + panel.clientHeight) {
        panel.scrollTo({ top: activeButton.offsetTop - (panel.clientHeight - activeButton.offsetHeight) / 2 });
      }
    }
  }, [activeQuestionID, mobileOpen]);

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
          style={{ bottom: "calc(var(--app-keyboard-inset-bottom,0px) + var(--chat-dock-height,10rem) + 0.35rem)" }}
        >
          {mobileOpen && showMobileQuestion ? (
            <div ref={mobilePanelRef} className="chat-scroll sketch-surface mb-2 max-h-[42vh] w-[min(calc(100vw-1.5rem),19.5rem)] animate-in fade-in slide-in-from-bottom-1 overflow-y-auto rounded-xl bg-card p-2 shadow-popover backdrop-blur duration-150">
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
            <div className="inline-flex w-fit animate-in items-center gap-1 rounded-full border border-border bg-card p-1 shadow-popover backdrop-blur duration-150 fade-in slide-in-from-bottom-1">
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
                  aria-current={activeQuestionID === question.id ? "location" : undefined}
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
});

function QuestionButton({ active, index, content, onClick }: { active: boolean; index: number; content: string; onClick: () => void }) {
  return (
    <button
      className={cn(
        "flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-[13px] leading-5 transition-colors hover:bg-muted",
        active && "bg-muted text-primary",
      )}
      onClick={onClick}
      aria-current={active ? "location" : undefined}
      title={content}
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
                className="inline-flex h-9 items-center gap-2 rounded-lg border border-border bg-card/80 px-3 text-sm transition-colors hover:bg-accent/70"
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
