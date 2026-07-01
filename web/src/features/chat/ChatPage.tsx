import { useEffect, useRef, useState } from "react";
import { CalendarClock, FolderOpen, Settings, Share2, Sparkles } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { appActions, chatActions, type AppPanel } from "@/app/store";
import { loadSessionDetail } from "@/features/sessions/sessionThunks";
import { cn } from "@/lib/cn";
import { pushAppPath } from "@/lib/navigation";
import { MessageList } from "./MessageList";
import { chatMessageElementID } from "./dom";
import { QueuePanel } from "./QueuePanel";
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
  const hasConversation = messages.length > 0 || events.length > 0 || queue.length > 0 || isProcessing || Boolean(error);
  const currentSessionIsRunning = Boolean(isProcessing && runningSessionID === activeSessionID);
  const isLoadingSession = Boolean(
    activeSessionID &&
      activeSessionID !== loadedSessionID &&
      failedSessionID !== activeSessionID &&
      !currentSessionIsRunning,
  );

  useEffect(() => {
    if (!activeSessionID) {
      setLoadedSessionID("");
      setFailedSessionID("");
      return;
    }
    if (currentSessionIsRunning) {
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
  }, [activeSessionID, loadedSessionID, currentSessionIsRunning, dispatch]);

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

  if (isLoadingSession && showSessionLoading) {
    return <ChatSessionLoading />;
  }

  if (!activeSessionID && !hasConversation) {
    return <ChatHome />;
  }

  return (
    <div className="relative flex h-full flex-col">
      <MessageList />
      <QuestionNavigator />
      <QueuePanel />
      <ChatInput />
    </div>
  );
}

function ChatSessionLoading() {
  return (
    <div className="flex h-full items-center justify-center px-6">
      <div className="sketch-surface flex w-full max-w-sm flex-col items-center gap-4 rounded-2xl px-8 py-9 text-center">
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

function QuestionNavigator() {
  const messages = useAppSelector((state) => state.chat.messages);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [activeQuestionID, setActiveQuestionID] = useState("");
  const questions = messages.filter((message) => message.role === "user" && message.content.trim());
  const orderedQuestions = questions.map((question, index) => ({ question, index: index + 1 }));
  const latestQuestionID = orderedQuestions[orderedQuestions.length - 1]?.question.id || "";
  const visibleQuestions = orderedQuestions.slice(-8);

  useEffect(() => {
    if (!latestQuestionID) return;
    setActiveQuestionID(latestQuestionID);
    window.requestAnimationFrame(() => {
      const panel = panelRef.current;
      if (!panel) return;
      panel.scrollTo({ top: panel.scrollHeight });
    });
  }, [latestQuestionID]);

  function jumpTo(messageID: string) {
    setActiveQuestionID(messageID);
    document.getElementById(chatMessageElementID(messageID))?.scrollIntoView({
      behavior: "smooth",
      block: "center",
    });
  }

  if (questions.length === 0) return null;

  return (
    <aside
      className="group absolute top-[42%] z-20 hidden -translate-y-1/2 xl:block"
      style={{ right: "0.75rem" }}
    >
      <div className="flex min-h-24 w-7 items-center justify-center">
        <div className="space-y-4 rounded-full bg-background/55 px-2 py-3 backdrop-blur-sm">
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
        className="chat-scroll sketch-surface pointer-events-none absolute right-0 top-1/2 max-h-80 w-72 -translate-y-1/2 overflow-y-auto rounded-2xl bg-card/95 p-4 opacity-0 shadow-[0_18px_48px_hsl(218_30%_20%/0.16)] backdrop-blur transition-opacity duration-150 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100"
      >
        <div className="space-y-1.5">
          {orderedQuestions.map(({ question, index }) => (
            <button
              key={question.id}
              className={cn(
                "flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm transition-colors hover:bg-muted",
                activeQuestionID === question.id && "bg-muted text-primary",
              )}
              onClick={() => jumpTo(question.id)}
              title={question.content}
            >
              <span
                className={cn(
                  "flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full border text-[11px]",
                  activeQuestionID === question.id
                    ? "border-primary/60 bg-primary/10 text-primary"
                    : "border-border/70 bg-muted/55 text-muted-foreground",
                )}
              >
                {index}
              </span>
              <span className="min-w-0 flex-1 truncate text-foreground/90">{question.content}</span>
              <span
                className={cn(
                  "h-[2px] shrink-0 rounded-full",
                  activeQuestionID === question.id ? "w-5 bg-primary" : "w-3.5 bg-muted-foreground/35",
                )}
                aria-hidden="true"
              />
            </button>
          ))}
        </div>
      </div>
    </aside>
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
    <div className="flex h-full items-center justify-center px-6 pb-10 pt-20">
      <div className="w-full max-w-4xl">
        <div className="mb-9 flex items-center justify-center gap-4 text-center">
          <img className="h-12 w-12 shrink-0 drop-shadow-sm md:h-14 md:w-14" src="/assets/fkteams-logo.svg" alt="" />
          <h1 className="text-4xl font-semibold tracking-normal text-foreground md:text-5xl">
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
