import {
  CalendarClock,
  FolderOpen,
  MoreVertical,
  MessageSquare,
  PanelLeftClose,
  PanelLeftOpen,
  Pencil,
  Plus,
  Search,
  Settings,
  Share2,
  Sparkles,
  Star,
  Trash2,
  X,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { LucideIcon } from "lucide-react";
import type { RefObject } from "react";
import { appActions, chatActions, type AppPanel } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { shortID, formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";
import { chatPath, panelPath, pushAppPath } from "@/lib/navigation";
import { deleteSession, favoriteSession, renameSession } from "@/api/sessions";
import { loadSessions } from "@/features/sessions/sessionThunks";
import { SessionShareDialog } from "./SessionShareDialog";

const panels: Array<{ key: AppPanel; label: string; icon: LucideIcon }> = [
  { key: "files", label: "文件", icon: FolderOpen },
  { key: "schedules", label: "任务", icon: CalendarClock },
  { key: "shares", label: "分享", icon: Share2 },
  { key: "skills", label: "技能", icon: Sparkles },
  { key: "config", label: "配置", icon: Settings },
] as const;

const sessionStatusLabels: Record<string, string> = {
  active: "活跃",
  processing: "进行中",
  completed: "已完成",
  cancelled: "已取消",
  canceled: "已取消",
  error: "失败",
};

const sessionMenuWidth = 176;
const sessionMenuHeight = 190;

export function Sidebar() {
  const dispatch = useAppDispatch();
  const [openMenuID, setOpenMenuID] = useState("");
  const [sessionMenuPosition, setSessionMenuPosition] = useState<{ top: number; left: number } | undefined>();
  const [deleteTarget, setDeleteTarget] = useState<{ session_id: string; title?: string } | null>(null);
  const [shareTarget, setShareTarget] = useState<{ session_id: string; title?: string } | null>(null);
  const [deletingSessionID, setDeletingSessionID] = useState("");
  const sessionMenuRef = useRef<HTMLDivElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const sidebarOpen = useAppSelector((state) => state.app.sidebarOpen);
  const activePanel = useAppSelector((state) => state.app.activePanel);
  const sessions = useAppSelector((state) => state.sessions.items);
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const version = useAppSelector((state) => state.app.version);

  const sortedSessions = [...sessions].sort((left, right) => sessionTime(right) - sessionTime(left));
  const searchResults = sortedSessions.filter((session) => {
    const text = `${session.title || ""} ${session.session_id}`.toLowerCase();
    return text.includes(searchQuery.toLowerCase());
  });
  const groups = groupSessions(sortedSessions);
  const openMenuSession = sortedSessions.find((session) => session.session_id === openMenuID);

  useEffect(() => {
    if (!openMenuID) return;
    function closeMenuOnOutsidePointer(event: PointerEvent) {
      if (sessionMenuRef.current?.contains(event.target as Node)) return;
      if (event.target instanceof Element && event.target.closest(`[data-session-menu-trigger="${openMenuID}"]`)) return;
      closeSessionMenu();
    }
    document.addEventListener("pointerdown", closeMenuOnOutsidePointer);
    return () => document.removeEventListener("pointerdown", closeMenuOnOutsidePointer);
  }, [openMenuID]);

  function closeSessionMenu() {
    setOpenMenuID("");
    setSessionMenuPosition(undefined);
  }

  function toggleSessionMenu(sessionID: string, trigger: HTMLElement) {
    if (openMenuID === sessionID) {
      closeSessionMenu();
      return;
    }
    const rect = trigger.getBoundingClientRect();
    const left = Math.min(
      Math.max(8, rect.right - sessionMenuWidth),
      Math.max(8, window.innerWidth - sessionMenuWidth - 8),
    );
    const top = Math.min(
      Math.max(8, rect.bottom + 4),
      Math.max(8, window.innerHeight - sessionMenuHeight - 8),
    );
    setSessionMenuPosition({ top, left });
    setOpenMenuID(sessionID);
  }

  function closeMobileSidebar() {
    if (window.matchMedia("(max-width: 767px)").matches) {
      dispatch(appActions.setSidebarOpen(false));
    }
  }

  function handleNewSession() {
    closeSessionMenu();
    dispatch(appActions.setActivePanel("chat"));
    dispatch(chatActions.setActiveSession(""));
    dispatch(chatActions.clearMessages());
    pushAppPath(chatPath());
    closeMobileSidebar();
  }

  function switchPanel(panel: (typeof panels)[number]) {
    closeSessionMenu();
    dispatch(appActions.setActivePanel(panel.key));
    pushAppPath(panelPath(panel.key));
    closeMobileSidebar();
  }

  function openSession(sessionID: string) {
    closeSessionMenu();
    dispatch(appActions.setActivePanel("chat"));
    dispatch(chatActions.setActiveSession(sessionID));
    pushAppPath(chatPath(sessionID));
    closeMobileSidebar();
  }

  async function toggleFavorite(session: { session_id: string; favorite?: boolean }) {
    await favoriteSession(session.session_id, !session.favorite);
    closeSessionMenu();
    dispatch(loadSessions());
  }

  async function handleRename(session: { session_id: string; title?: string }) {
    const title = window.prompt("重命名会话", session.title || "");
    if (title === null) return;
    const nextTitle = title.trim();
    if (!nextTitle) return;
    await renameSession(session.session_id, nextTitle);
    closeSessionMenu();
    dispatch(loadSessions());
  }

  function requestDelete(session: { session_id: string; title?: string }) {
    closeSessionMenu();
    setDeleteTarget(session);
  }

  function requestShare(session: { session_id: string; title?: string }) {
    closeSessionMenu();
    setShareTarget(session);
  }

  async function confirmDelete() {
    if (!deleteTarget || deletingSessionID) return;
    const sessionID = deleteTarget.session_id;
    setDeletingSessionID(sessionID);
    try {
      await deleteSession(sessionID);
      setDeleteTarget(null);
      if (activeSessionID === sessionID) {
        dispatch(chatActions.setActiveSession(""));
        dispatch(chatActions.clearMessages());
        pushAppPath(chatPath());
      }
      dispatch(loadSessions());
    } finally {
      setDeletingSessionID("");
    }
  }

  return (
    <>
      {sidebarOpen ? (
        <button
          className="fixed inset-0 z-30 bg-foreground/15 backdrop-blur-[1px] md:hidden"
          type="button"
          aria-label="关闭导航"
          onClick={() => dispatch(appActions.setSidebarOpen(false))}
        />
      ) : null}
      <aside
        className={cn(
          "sketch-rule fixed inset-y-0 left-0 z-40 flex h-[100dvh] shrink-0 flex-col border-r bg-sidebar/95 text-sidebar-foreground transition-[transform,width] md:relative md:z-auto md:h-screen md:translate-x-0",
          sidebarOpen
            ? "w-[min(292px,calc(100vw-3rem))] translate-x-0 shadow-[12px_0_32px_hsl(218_30%_20%/0.16)] md:w-[292px] md:shadow-none"
            : "w-[min(292px,calc(100vw-3rem))] -translate-x-full md:w-16 md:translate-x-0",
        )}
      >
      <div
        className={cn(
          "flex items-center",
          sidebarOpen ? "h-14 gap-2.5 px-3" : "h-20 flex-col justify-center gap-1.5 px-0",
        )}
      >
        <img className="h-8 w-8 shrink-0 drop-shadow-sm" src="/assets/fkteams-logo.svg" alt="" />
        {sidebarOpen ? (
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <div className="truncate text-lg font-semibold tracking-normal">非空小队</div>
            <span className="shrink-0 rounded-full border border-border/75 bg-card/70 px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
              {version?.version || "dev"}
            </span>
          </div>
        ) : null}
        {sidebarOpen ? (
          <Button size="icon" variant="ghost" aria-label="搜索会话" onClick={() => setSearchOpen(true)}>
            <Search className="h-4 w-4" />
          </Button>
        ) : null}
        <Button
          size="icon"
          variant="ghost"
          className={cn(!sidebarOpen && "h-8 w-8")}
          aria-label={sidebarOpen ? "收起侧栏" : "展开侧栏"}
          onClick={() => dispatch(appActions.setSidebarOpen(!sidebarOpen))}
        >
          {sidebarOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeftOpen className="h-4 w-4" />}
        </Button>
      </div>

      <nav className="space-y-0.5 px-2 pb-3">
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
      </nav>

      {sidebarOpen ? (
        <>
          <div className="px-3 pb-1">
            <div className="text-xs text-muted-foreground">最近会话</div>
          </div>
          <div className="min-h-0 flex-1 overflow-auto px-2 pb-2" onScroll={closeSessionMenu}>
            {sortedSessions.length === 0 ? (
              <div className="px-2 py-8 text-base text-muted-foreground">暂无会话</div>
            ) : (
              groups.map((group) => (
                <section key={group.label} className="mb-3">
                  <div className="px-2 pb-1 pt-2 text-xs text-muted-foreground">{group.label}</div>
                  {group.sessions.map((session) => (
                    <div
                      key={session.session_id}
                      className={cn(
                        "group relative mb-1 flex w-full items-start gap-1 rounded-xl py-2 pl-2.5 pr-1.5 text-base transition-colors hover:bg-card/70",
                        activeSessionID === session.session_id && "bg-card/80 text-accent-foreground",
                      )}
                    >
                      <button
                        className="min-w-0 flex-1 text-left"
                        onClick={() => openSession(session.session_id)}
                      >
                        <div className="flex min-w-0 items-center gap-1.5">
                          {session.favorite ? (
                            <Star className="h-4 w-4 shrink-0 fill-foreground" />
                          ) : null}
                          <span className="truncate font-medium">{session.title || shortID(session.session_id)}</span>
                        </div>
                        <div className="mt-1 flex items-center gap-1.5 text-xs leading-5 text-muted-foreground">
                          <span className="truncate">{formatTime(session.mod_time || session.updated_at)}</span>
                          {session.status ? (
                            <>
                              <span>·</span>
                              <span className="truncate">{sessionStatusLabel(session.status)}</span>
                            </>
                          ) : null}
                        </div>
                      </button>
                      <button
                        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-opacity hover:bg-muted hover:text-foreground md:opacity-0 md:group-hover:opacity-100"
                        aria-label="会话操作"
                        data-session-menu-trigger={session.session_id}
                        onClick={(event) => {
                          event.stopPropagation();
                          toggleSessionMenu(session.session_id, event.currentTarget);
                        }}
                      >
                        <MoreVertical className="h-4 w-4" />
                      </button>
                    </div>
                  ))}
                </section>
              ))
            )}
          </div>
        </>
      ) : null}
      </aside>
      {searchOpen ? (
        <SessionSearchDialog
          activeSessionID={activeSessionID}
          query={searchQuery}
          sessions={searchResults}
          onQueryChange={setSearchQuery}
          onClose={() => setSearchOpen(false)}
          onSelect={(sessionID) => {
            openSession(sessionID);
            setSearchOpen(false);
          }}
        />
      ) : null}
      <SessionDeleteDialog
        session={deleteTarget}
        deleting={Boolean(deleteTarget && deletingSessionID === deleteTarget.session_id)}
        onCancel={() => {
          if (!deletingSessionID) setDeleteTarget(null);
        }}
        onConfirm={() => void confirmDelete()}
      />
      <SessionShareDialog
        session={shareTarget}
        onClose={() => setShareTarget(null)}
        onCreated={() => dispatch(loadSessions())}
      />
      {openMenuSession && sessionMenuPosition ? (
        <SessionMenu
          menuRef={sessionMenuRef}
          position={sessionMenuPosition}
          favorite={Boolean(openMenuSession.favorite)}
          onToggleFavorite={() => void toggleFavorite(openMenuSession)}
          onRename={() => void handleRename(openMenuSession)}
          onShare={() => requestShare(openMenuSession)}
          onDelete={() => requestDelete(openMenuSession)}
        />
      ) : null}
    </>
  );
}

function sessionStatusLabel(status: string) {
  return sessionStatusLabels[status.toLowerCase()] || status;
}

function SessionDeleteDialog({
  session,
  deleting,
  onCancel,
  onConfirm,
}: {
  session: { session_id: string; title?: string } | null;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!session) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="session-delete-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onCancel();
      }}
    >
      <div className="sketch-surface w-full max-w-md rounded-2xl bg-card/95 p-5 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        <div className="flex items-start gap-3">
          <div className="mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-destructive/30 bg-destructive/10 text-destructive">
            <Trash2 className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="session-delete-title" className="text-lg font-semibold text-foreground">
              删除会话
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              会话「{session.title || shortID(session.session_id)}」会从列表中移除，相关聊天记录也将不可恢复。
            </p>
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={deleting}>
            取消
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={deleting}>
            {deleting ? "删除中" : "确认删除"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function SessionSearchDialog({
  activeSessionID,
  query,
  sessions,
  onQueryChange,
  onClose,
  onSelect,
}: {
  activeSessionID: string;
  query: string;
  sessions: Array<{ session_id: string; title?: string; mod_time?: string; updated_at?: string }>;
  onQueryChange: (value: string) => void;
  onClose: () => void;
  onSelect: (sessionID: string) => void;
}) {
  return (
    <div
      className="fixed inset-0 z-50 bg-foreground/10 px-3 pt-16 backdrop-blur-[1px] sm:px-4 sm:pt-20"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="mx-auto flex max-h-[78vh] w-full max-w-[720px] flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-[0_18px_50px_hsl(218_30%_20%/0.18)] sm:max-h-[68vh]">
        <div className="flex h-14 items-center gap-3 border-b border-border/70 px-4 sm:h-16 sm:px-5">
          <Search className="h-5 w-5 shrink-0 text-muted-foreground" />
          <Input
            autoFocus
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") onClose();
            }}
            placeholder="搜索会话"
            className="h-10 border-0 bg-transparent px-0 text-base shadow-none focus-visible:ring-0 sm:text-lg"
          />
          <button className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground" onClick={onClose} aria-label="关闭搜索">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-auto p-3">
          {sessions.length === 0 ? (
            <div className="px-4 py-10 text-sm text-muted-foreground">没有匹配的会话</div>
          ) : (
            <div className="space-y-2">
              {sessions.map((session) => (
                <button
                  key={session.session_id}
                  className={cn(
                    "flex min-h-12 w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-base transition-colors hover:bg-muted/60 sm:px-4",
                    activeSessionID === session.session_id && "bg-muted",
                  )}
                  onClick={() => onSelect(session.session_id)}
                >
                  <MessageSquare className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate font-medium">{session.title || shortID(session.session_id)}</span>
                  <span className="shrink-0 text-sm text-muted-foreground">{relativeSessionTime(session)}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function SessionMenu({
  menuRef,
  position,
  favorite,
  onToggleFavorite,
  onRename,
  onShare,
  onDelete,
}: {
  menuRef: RefObject<HTMLDivElement | null>;
  position: { top: number; left: number };
  favorite: boolean;
  onToggleFavorite: () => void;
  onRename: () => void;
  onShare: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      ref={menuRef}
      className="sketch-surface fixed z-50 w-44 rounded-xl bg-card p-1.5 text-sm shadow-[0_12px_28px_hsl(218_30%_25%/0.16)]"
      style={{ top: position.top, left: position.left }}
    >
      <button className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left hover:bg-accent/65" onClick={onToggleFavorite}>
        <Star className={cn("h-4 w-4", favorite && "fill-foreground")} />
        {favorite ? "取消收藏" : "收藏"}
      </button>
      <button className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left hover:bg-accent/65" onClick={onRename}>
        <Pencil className="h-4 w-4" />
        重命名
      </button>
      <button className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left hover:bg-accent/65" onClick={onShare}>
        <Share2 className="h-4 w-4" />
        分享
      </button>
      <div className="my-1 border-t border-border/70" />
      <button className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left text-destructive hover:bg-destructive/10" onClick={onDelete}>
        <Trash2 className="h-4 w-4" />
        删除
      </button>
    </div>
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

function relativeSessionTime(session: { mod_time?: string; updated_at?: string }) {
  const time = sessionTime(session);
  if (!time) return "";
  const diff = Date.now() - time;
  const day = 24 * 60 * 60 * 1000;
  if (diff < day) return "今天";
  if (diff < day * 2) return "昨天";
  if (diff < day * 30) return "近 30 天";
  return "更早";
}

function parseTime(value: string) {
  const normalized = value.trim().replace(/\//g, "-");
  const match = normalized.match(/^(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T](\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/);
  if (!match) return Date.parse(value);
  const [, year, month, day, hour = "0", minute = "0", second = "0"] = match;
  return new Date(Number(year), Number(month) - 1, Number(day), Number(hour), Number(minute), Number(second)).getTime();
}

function groupSessions<T extends { favorite?: boolean; mod_time?: string; updated_at?: string }>(sessions: T[]) {
  const today = startOfDay(new Date());
  const yesterday = today - 24 * 60 * 60 * 1000;
  const groups = new Map<string, T[]>();
  for (const session of sessions) {
    const time = sessionTime(session);
    const day = Number.isFinite(time) ? startOfDay(new Date(time)) : 0;
    const label = session.favorite ? "收藏" : day === today ? "今天" : day === yesterday ? "昨天" : "更早";
    const bucket = groups.get(label) || [];
    bucket.push(session);
    groups.set(label, bucket);
  }
  return ["收藏", "今天", "昨天", "更早"]
    .map((label) => ({ label, sessions: groups.get(label) || [] }))
    .filter((group) => group.sessions.length > 0);
}

function startOfDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}
