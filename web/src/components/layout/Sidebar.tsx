import {
  CalendarClock,
  FileText,
  FolderOpen,
  MessageSquare,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Search,
  Settings,
  SlidersHorizontal,
  Sparkles,
  UserRound,
} from "lucide-react";
import { useRef } from "react";
import type { LucideIcon } from "lucide-react";
import { appActions, chatActions, sessionsActions, type AppPanel } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { shortID, formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";
import { createSession } from "@/api/sessions";
import { loadSessions } from "@/features/sessions/sessionThunks";

const panels: Array<{ key: AppPanel; label: string; path: string; icon: LucideIcon }> = [
  { key: "chat", label: "对话", path: "/chat", icon: MessageSquare },
  { key: "files", label: "文件", path: "/files", icon: FolderOpen },
  { key: "schedules", label: "任务", path: "/schedules", icon: CalendarClock },
  { key: "skills", label: "技能", path: "/skills", icon: Sparkles },
  { key: "config", label: "配置", path: "/config", icon: Settings },
] as const;

export function Sidebar() {
  const dispatch = useAppDispatch();
  const searchInputRef = useRef<HTMLInputElement | null>(null);
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
  const groups = groupSessions(filtered);

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
        "sketch-rule flex h-screen shrink-0 flex-col border-r bg-sidebar/95 text-sidebar-foreground transition-[width]",
        sidebarOpen ? "w-[312px]" : "w-16",
      )}
    >
      <div className="flex h-16 items-center gap-3 px-4">
        <img className="h-9 w-9 shrink-0 drop-shadow-sm" src="/assets/fkteams-logo.svg" alt="" />
        {sidebarOpen ? <div className="min-w-0 flex-1 text-xl font-semibold tracking-normal">非空小队</div> : null}
        {sidebarOpen ? (
          <Button size="icon" variant="ghost" aria-label="搜索会话" onClick={() => searchInputRef.current?.focus()}>
            <Search className="h-4 w-4" />
          </Button>
        ) : null}
        <Button
          size="icon"
          variant="ghost"
          aria-label={sidebarOpen ? "收起侧栏" : "展开侧栏"}
          onClick={() => dispatch(appActions.setSidebarOpen(!sidebarOpen))}
        >
          {sidebarOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeftOpen className="h-4 w-4" />}
        </Button>
      </div>

      <nav className="space-y-1 px-3 pb-4">
        <SidebarNavItem
          icon={Plus}
          label="新建会话"
          open={sidebarOpen}
          onClick={handleNewSession}
        />
        {panels.map((panel) => (
          <SidebarNavItem
            key={panel.key}
            icon={panel.icon}
            label={panel.label}
            open={sidebarOpen}
            active={activePanel === panel.key}
            onClick={() => switchPanel(panel)}
          />
        ))}
        <div
          className={cn(
            "flex h-9 items-center gap-3 rounded-md px-2.5 text-sm text-muted-foreground/60",
            !sidebarOpen && "justify-center px-0",
          )}
          title="文档"
        >
          <FileText className="h-4 w-4" />
          {sidebarOpen ? (
            <>
              <span className="flex-1">文档</span>
              <span className="rounded-full border border-border/70 px-2 py-0.5 text-xs text-muted-foreground">后续</span>
            </>
          ) : null}
        </div>
      </nav>

      {sidebarOpen ? (
        <>
          <div className="px-4 pb-3">
            <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
              <span>最近会话</span>
              <SlidersHorizontal className="h-3.5 w-3.5" />
            </div>
            <Input
              ref={searchInputRef}
              value={search}
              onChange={(event) => dispatch(sessionsActions.setSessionSearch(event.target.value))}
              placeholder="搜索会话"
              className="h-8 rounded-lg bg-background/70"
            />
          </div>
          <div className="min-h-0 flex-1 overflow-auto px-3 pb-3">
            {filtered.length === 0 ? (
              <div className="px-2 py-8 text-sm text-muted-foreground">暂无会话</div>
            ) : (
              groups.map((group) => (
                <section key={group.label} className="mb-4">
                  <div className="px-2 pb-1 pt-2 text-xs text-muted-foreground">{group.label}</div>
                  {group.sessions.map((session) => (
                    <button
                      key={session.session_id}
                      className={cn(
                        "group mb-1 w-full rounded-lg px-2.5 py-2 text-left text-sm transition-colors hover:bg-card/75",
                        activeSessionID === session.session_id && "bg-card text-accent-foreground shadow-[2px_2px_0_hsl(218_32%_30%/0.08)]",
                      )}
                      onClick={() => dispatch(chatActions.setActiveSession(session.session_id))}
                    >
                      <div className="truncate font-medium">{session.title || shortID(session.session_id)}</div>
                      <div className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                        <span className="truncate">{shortID(session.session_id)}</span>
                        <span>·</span>
                        <span className="truncate">{formatTime(session.mod_time || session.updated_at)}</span>
                        {session.status ? (
                          <>
                            <span>·</span>
                            <span className="truncate">{session.status}</span>
                          </>
                        ) : null}
                      </div>
                    </button>
                  ))}
                </section>
              ))
            )}
          </div>
          <div className="sketch-rule border-t p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-foreground text-background">
                <UserRound className="h-4 w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">本地工作区</div>
                <div className="truncate text-xs text-muted-foreground">{version?.version || "dev"}</div>
              </div>
            </div>
          </div>
        </>
      ) : null}
    </aside>
  );
}

function SidebarNavItem({
  icon: Icon,
  label,
  active,
  open,
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  active?: boolean;
  open: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "flex h-9 w-full items-center gap-3 rounded-lg px-2.5 text-sm transition-colors hover:bg-card/70",
        active && "bg-card shadow-[2px_2px_0_hsl(218_32%_30%/0.08)]",
        !open && "justify-center px-0",
      )}
      onClick={onClick}
      title={label}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {open ? <span className="truncate">{label}</span> : null}
    </button>
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

function groupSessions<T extends { mod_time?: string; updated_at?: string }>(sessions: T[]) {
  const today = startOfDay(new Date());
  const yesterday = today - 24 * 60 * 60 * 1000;
  const groups = new Map<string, T[]>();
  for (const session of sessions) {
    const time = sessionTime(session);
    const day = Number.isFinite(time) ? startOfDay(new Date(time)) : 0;
    const label = day === today ? "今天" : day === yesterday ? "昨天" : "更早";
    const bucket = groups.get(label) || [];
    bucket.push(session);
    groups.set(label, bucket);
  }
  return ["今天", "昨天", "更早"]
    .map((label) => ({ label, sessions: groups.get(label) || [] }))
    .filter((group) => group.sessions.length > 0);
}

function startOfDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}
