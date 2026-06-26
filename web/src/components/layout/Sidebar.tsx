import {
  CalendarClock,
  FolderOpen,
  History,
  MessageSquarePlus,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Sparkles,
  Wrench,
} from "lucide-react";
import { appActions, chatActions, sessionsActions, type AppPanel } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { shortID, formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";
import { createSession } from "@/api/sessions";
import { loadSessions } from "@/features/sessions/sessionThunks";

const panels: Array<{ key: AppPanel; label: string; path: string; icon: typeof MessageSquarePlus }> = [
  { key: "chat", label: "对话", path: "/chat", icon: MessageSquarePlus },
  { key: "files", label: "文件", path: "/files", icon: FolderOpen },
  { key: "schedules", label: "任务", path: "/schedules", icon: CalendarClock },
  { key: "skills", label: "技能", path: "/skills", icon: Sparkles },
  { key: "config", label: "配置", path: "/config", icon: Settings },
] as const;

export function Sidebar() {
  const dispatch = useAppDispatch();
  const sidebarOpen = useAppSelector((state) => state.app.sidebarOpen);
  const activePanel = useAppSelector((state) => state.app.activePanel);
  const sessions = useAppSelector((state) => state.sessions.items);
  const search = useAppSelector((state) => state.sessions.search);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const version = useAppSelector((state) => state.app.version);

  const filtered = sessions
    .filter((session) => {
      const text = `${session.title || ""} ${session.session_id}`.toLowerCase();
      return text.includes(search.toLowerCase());
    })
    .sort((left, right) => sessionTime(right) - sessionTime(left));

  async function handleNewSession() {
    const result = await createSession("");
    dispatch(chatActions.setActiveSession(result.session_id));
    dispatch(chatActions.clearMessages());
    dispatch(loadSessions());
  }

  function switchPanel(panel: (typeof panels)[number]) {
    dispatch(appActions.setActivePanel(panel.key));
    if (location.pathname !== panel.path) history.pushState(null, "", panel.path);
  }

  return (
    <aside
      className={cn(
        "sketch-rule flex h-screen shrink-0 flex-col border-r bg-sidebar/92 text-sidebar-foreground transition-[width]",
        sidebarOpen ? "w-[300px]" : "w-16",
      )}
    >
      <div className="sketch-rule flex h-14 items-center gap-3 border-b px-3">
        <img className="h-9 w-9 drop-shadow-sm" src="/assets/fkteams-logo.svg" alt="" />
        {sidebarOpen ? <div className="min-w-0 flex-1 text-lg font-semibold">非空小队</div> : null}
        <Button
          size="icon"
          variant="ghost"
          aria-label={sidebarOpen ? "收起侧栏" : "展开侧栏"}
          onClick={() => dispatch(appActions.setSidebarOpen(!sidebarOpen))}
        >
          {sidebarOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeftOpen className="h-4 w-4" />}
        </Button>
      </div>

      <div className="sketch-rule space-y-2 border-b p-2">
        {panels.map((panel) => {
          const Icon = panel.icon;
          return (
            <Button
              key={panel.key}
              variant={activePanel === panel.key ? "secondary" : "ghost"}
              className={cn("h-10 w-full justify-start", activePanel === panel.key && "bg-accent", !sidebarOpen && "justify-center px-0")}
              onClick={() => switchPanel(panel)}
              title={panel.label}
            >
              <Icon className="h-4 w-4" />
              {sidebarOpen ? <span>{panel.label}</span> : null}
            </Button>
          );
        })}
      </div>

      {sidebarOpen ? (
        <>
          <div className="sketch-rule flex items-center gap-2 border-b p-3">
            <Button className="flex-1" onClick={handleNewSession}>
              <MessageSquarePlus className="h-4 w-4" />
              新建会话
            </Button>
          </div>
          <div className="sketch-rule border-b p-3">
            <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
              <History className="h-3.5 w-3.5" />
              会话历史
            </div>
            <Input
              value={search}
              onChange={(event) => dispatch(sessionsActions.setSessionSearch(event.target.value))}
              placeholder="搜索会话"
            />
          </div>
          <div className="min-h-0 flex-1 overflow-auto p-2">
            {filtered.length === 0 ? (
              <div className="p-4 text-sm text-muted-foreground">暂无会话</div>
            ) : (
              filtered.map((session) => (
                <button
                  key={session.session_id}
                  className={cn(
                    "mb-2 w-full rounded-md border border-transparent px-3 py-2 text-left text-sm hover:border-border hover:bg-card/70",
                    activeSessionID === session.session_id && "border-border bg-card text-accent-foreground shadow-[2px_2px_0_hsl(218_32%_30%/0.08)]",
                  )}
                  onClick={() => dispatch(chatActions.setActiveSession(session.session_id))}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate font-medium">{session.title || shortID(session.session_id)}</span>
                    {session.status ? <Badge>{session.status}</Badge> : null}
                  </div>
                  <div className="mt-1 truncate text-xs text-muted-foreground">
                    {shortID(session.session_id)} · {formatTime(session.mod_time || session.updated_at)}
                  </div>
                </button>
              ))
            )}
          </div>
          <div className="sketch-rule border-t p-3 text-xs text-muted-foreground">
            <div className="flex items-center gap-2">
              <Wrench className="h-3.5 w-3.5" />
              {version?.version || "dev"}
            </div>
          </div>
        </>
      ) : null}
    </aside>
  );
}

function sessionTime(session: { mod_time?: string; updated_at?: string }) {
  const value = session.updated_at || session.mod_time || "";
  const time = parseTime(value);
  return Number.isFinite(time) ? time : 0;
}

function parseTime(value: string) {
  const normalized = value.trim().replace(/\//g, "-");
  const match = normalized.match(/^(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T](\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/);
  if (!match) return Date.parse(value);
  const [, year, month, day, hour = "0", minute = "0", second = "0"] = match;
  return new Date(Number(year), Number(month) - 1, Number(day), Number(hour), Number(minute), Number(second)).getTime();
}
