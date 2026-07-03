import { Menu, Share2 } from "lucide-react";
import { useEffect, useState } from "react";
import { appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { shortID } from "@/lib/format";
import { Sidebar } from "./Sidebar";
import { SessionShareDialog } from "./SessionShareDialog";

export function AppShell({ children }: { children: React.ReactNode }) {
  const dispatch = useAppDispatch();
  const toast = useAppSelector((state) => state.app.toast);
  const [shareTarget, setShareTarget] = useState<{ session_id: string; title?: string } | null>(null);
  const activePanel = useAppSelector((state) => state.app.activePanel);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const sessions = useAppSelector((state) => state.sessions.items);
  const title = resolveTitle(activePanel, activeSessionID, sessions);
  const activeSession = sessions.find((item) => item.session_id === activeSessionID);
  const canShareSession = activePanel === "chat" && Boolean(activeSessionID);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => dispatch(appActions.showToast(undefined)), 2000);
    return () => window.clearTimeout(timer);
  }, [dispatch, toast]);

  useEffect(() => {
    if (!canShareSession) setShareTarget(null);
  }, [canShareSession]);

  return (
    <div className="flex h-[var(--app-viewport-height,100dvh)] overflow-hidden bg-background/95 text-foreground">
      <Sidebar />
      <main className="relative flex min-w-0 flex-1 flex-col">
        <header className="sketch-rule flex h-14 shrink-0 items-center justify-between border-b bg-background/82 px-3 backdrop-blur sm:px-5">
          <div className="flex min-w-0 items-center gap-3">
            <Button
              className="md:hidden"
              size="icon"
              variant="ghost"
              aria-label="打开导航"
              onClick={() => dispatch(appActions.setSidebarOpen(true))}
            >
              <Menu className="h-4 w-4" />
            </Button>
            <div className="min-w-0 truncate text-base font-semibold">
              {title}
            </div>
          </div>
          {canShareSession ? (
            <button
              className="flex h-9 w-9 shrink-0 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
              aria-label="分享会话"
              title="分享会话"
              type="button"
              onClick={() => setShareTarget({ session_id: activeSessionID, title: activeSession?.title })}
            >
              <Share2 className="h-4 w-4" />
            </button>
          ) : null}
        </header>
        <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
      </main>
      <SessionShareDialog
        session={shareTarget}
        onClose={() => setShareTarget(null)}
      />
      {toast ? (
        <div className="sketch-surface fixed bottom-4 left-4 right-4 z-50 rounded-md px-4 py-3 text-sm sm:left-auto sm:w-auto">
          {toast}
        </div>
      ) : null}
    </div>
  );
}

function resolveTitle(
  activePanel: "chat" | "config" | "files" | "schedules" | "shares" | "skills",
  activeSessionID: string,
  sessions: Array<{ session_id: string; title?: string }>,
) {
  if (activePanel !== "chat") {
    return {
      files: "文件",
      schedules: "任务",
      shares: "分享",
      skills: "技能",
      config: "配置",
    }[activePanel];
  }
  if (!activeSessionID) return "新会话";
  const session = sessions.find((item) => item.session_id === activeSessionID);
  return session?.title || shortID(activeSessionID) || "新会话";
}
