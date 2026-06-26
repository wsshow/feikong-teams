import { ChevronDown, Menu } from "lucide-react";
import { appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { createSessionShare } from "@/api/shares";
import { shortID } from "@/lib/format";
import { Sidebar } from "./Sidebar";

export function AppShell({ children }: { children: React.ReactNode }) {
  const dispatch = useAppDispatch();
  const toast = useAppSelector((state) => state.app.toast);
  const activePanel = useAppSelector((state) => state.app.activePanel);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const sessions = useAppSelector((state) => state.sessions.items);
  const title = resolveTitle(activePanel, activeSessionID, sessions);

  async function shareSession() {
    if (!activeSessionID) return;
    try {
      const share = await createSessionShare(activeSessionID);
      const url = `${location.origin}/s/${encodeURIComponent(share.share_id)}`;
      await navigator.clipboard?.writeText(url);
      dispatch(appActions.showToast("分享链接已复制"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    }
  }

  return (
    <div className="flex h-screen overflow-hidden bg-background/95 text-foreground">
      <Sidebar />
      <main className="relative flex min-w-0 flex-1 flex-col">
        <header className="sketch-rule flex h-14 shrink-0 items-center justify-between border-b bg-background/82 px-5 backdrop-blur">
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
            <button className="flex min-w-0 items-center gap-2 text-left text-base font-semibold" type="button">
              <span className="truncate">{title}</span>
              <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
            </button>
          </div>
          <Button variant="outline" onClick={shareSession} disabled={activePanel !== "chat" || !activeSessionID}>
            分享
          </Button>
        </header>
        <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
      </main>
      {toast ? (
        <div className="sketch-surface fixed bottom-4 right-4 z-50 rounded-md px-4 py-3 text-sm">
          {toast}
        </div>
      ) : null}
    </div>
  );
}

function resolveTitle(
  activePanel: "chat" | "config" | "files" | "schedules" | "skills",
  activeSessionID: string,
  sessions: Array<{ session_id: string; title?: string }>,
) {
  if (activePanel !== "chat") {
    return {
      files: "文件",
      schedules: "任务",
      skills: "技能",
      config: "配置",
    }[activePanel];
  }
  if (!activeSessionID) return "新会话";
  const session = sessions.find((item) => item.session_id === activeSessionID);
  return session?.title || shortID(activeSessionID) || "新会话";
}
