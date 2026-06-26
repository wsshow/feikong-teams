import { useEffect } from "react";
import { CalendarClock, FolderOpen, Settings, Sparkles } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { appActions, chatActions, type AppPanel } from "@/app/store";
import { loadSessionDetail } from "@/features/sessions/sessionThunks";
import { MessageList } from "./MessageList";
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
  const hasConversation = messages.length > 0 || events.length > 0 || queue.length > 0 || isProcessing || Boolean(error);

  useEffect(() => {
    if (activeSessionID && !(isProcessing && runningSessionID === activeSessionID)) {
      void dispatch(loadSessionDetail(activeSessionID));
    }
  }, [activeSessionID, runningSessionID, isProcessing, dispatch]);

  useEffect(() => {
    dispatch(chatActions.setConnectionState("connected"));
  }, [dispatch]);

  if (!hasConversation) {
    return <ChatHome />;
  }

  return (
    <div className="flex h-full flex-col pt-12">
      <MessageList />
      <QueuePanel />
      <ChatInput />
    </div>
  );
}

function ChatHome() {
  const dispatch = useAppDispatch();
  const shortcuts: Array<{ label: string; icon: typeof FolderOpen; panel: AppPanel; path: string }> = [
    { label: "文件", icon: FolderOpen, panel: "files", path: "/files" },
    { label: "任务", icon: CalendarClock, panel: "schedules", path: "/schedules" },
    { label: "技能", icon: Sparkles, panel: "skills", path: "/skills" },
    { label: "配置", icon: Settings, panel: "config", path: "/config" },
  ];

  function openPanel(panel: AppPanel, path: string) {
    dispatch(appActions.setActivePanel(panel));
    if (location.pathname !== path) history.pushState(null, "", path);
  }

  return (
    <div className="flex h-full items-center justify-center px-6 pb-10 pt-20">
      <div className="w-full max-w-4xl">
        <div className="mb-9 flex items-center justify-center gap-4 text-center">
          <span className="text-4xl text-destructive" aria-hidden>
            ※
          </span>
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
