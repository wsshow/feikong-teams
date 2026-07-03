import { ArrowDown, ArrowUp, CornerDownRight, MessageSquare, MoreHorizontal, Pencil, Route, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions } from "@/app/store";
import { changeQueueKind, deleteQueueItem, moveQueueItem } from "@/api/stream";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import type { QueueItem } from "@/types/events";

export function QueuePanel({ onEditMessage }: { onEditMessage: (message: string) => void }) {
  const dispatch = useAppDispatch();
  const sessionID = useAppSelector((state) => state.chat.activeSessionID);
  const queue = useAppSelector((state) => state.chat.queue);
  const [openMenuID, setOpenMenuID] = useState("");
  const panelRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!openMenuID) return;
    const closeOnOutsidePointer = (event: PointerEvent) => {
      const panel = panelRef.current;
      if (panel?.contains(event.target as Node)) return;
      setOpenMenuID("");
    };

    document.addEventListener("pointerdown", closeOnOutsidePointer);
    return () => document.removeEventListener("pointerdown", closeOnOutsidePointer);
  }, [openMenuID]);

  if (!queue.length) return null;

  async function refresh(action: Promise<{ queue: QueueItem[] }>) {
    const result = await action;
    setOpenMenuID("");
    dispatch(chatActions.setQueue(result.queue || []));
  }

  return (
    <div ref={panelRef} className="relative z-0 mx-auto -mb-5 max-h-36 w-[calc(100%-2rem)] overflow-visible rounded-t-[1.35rem] border border-b-0 border-border/55 bg-card/80 px-4 pb-7 pt-3 shadow-[0_8px_24px_hsl(218_30%_25%/0.06)] backdrop-blur sm:w-[calc(100%-4rem)] sm:px-5">
      <div className="space-y-1">
        {queue.map((item) => (
          <QueueRow
            key={item.queue_id}
            item={item}
            sessionID={sessionID}
            menuOpen={openMenuID === item.queue_id}
            onToggleMenu={() => setOpenMenuID(openMenuID === item.queue_id ? "" : item.queue_id)}
            onEditMessage={onEditMessage}
            onRefresh={refresh}
          />
        ))}
      </div>
    </div>
  );
}

function QueueRow({
  item,
  sessionID,
  menuOpen,
  onToggleMenu,
  onEditMessage,
  onRefresh,
}: {
  item: QueueItem;
  sessionID: string;
  menuOpen: boolean;
  onToggleMenu: () => void;
  onEditMessage: (message: string) => void;
  onRefresh: (action: Promise<{ queue: QueueItem[] }>) => void;
}) {
  const isSteering = item.kind === "steering";
  const text = queueItemText(item);

  async function editMessage() {
    const result = await deleteQueueItem(sessionID, item.queue_id);
    onEditMessage(text);
    onRefresh(Promise.resolve(result));
  }

  return (
    <div className="relative flex min-h-8 items-center gap-2 rounded-md px-1 sm:px-2">
      <MessageSquare className="h-3.5 w-3.5 shrink-0 text-muted-foreground/55" />
      <div
        className="min-w-0 flex-1 truncate text-sm font-semibold leading-7 text-foreground/76"
        title={text}
      >
        {text || "空消息"}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Button
          className={cn(
            "group/button h-7 px-2 text-sm font-semibold text-muted-foreground hover:bg-accent/65 hover:text-foreground",
            isSteering && "text-foreground",
          )}
          size="sm"
          variant="ghost"
          disabled={isSteering}
          onClick={() => onRefresh(changeQueueKind(sessionID, item.queue_id, "steering"))}
          title={isSteering ? "已设为转向" : "设为转向"}
        >
          <Route className="h-3.5 w-3.5 transition-transform group-hover/button:translate-x-0.5" />
          转向
        </Button>
        <Button
          className="h-7 w-7 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
          size="icon"
          variant="ghost"
          onClick={() => onRefresh(deleteQueueItem(sessionID, item.queue_id))}
          aria-label="删除"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
        <Button
          className="h-7 w-7 text-muted-foreground hover:bg-accent/65 hover:text-foreground"
          size="icon"
          variant="ghost"
          onClick={onToggleMenu}
          aria-label="更多"
          aria-expanded={menuOpen}
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </Button>
      </div>
      {menuOpen ? (
        <div className="absolute bottom-[calc(100%+0.25rem)] right-2 z-50 w-32 overflow-hidden rounded-md border border-border bg-card py-1 text-sm shadow-[0_10px_24px_hsl(218_30%_25%/0.14)]">
          <button className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted" type="button" onClick={() => void editMessage()}>
            <Pencil className="h-3.5 w-3.5" />
            编辑
          </button>
          <button className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted" type="button" onClick={() => onRefresh(moveQueueItem(sessionID, item.queue_id, "up"))}>
            <ArrowUp className="h-3.5 w-3.5" />
            上移
          </button>
          <button className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted" type="button" onClick={() => onRefresh(moveQueueItem(sessionID, item.queue_id, "down"))}>
            <ArrowDown className="h-3.5 w-3.5" />
            下移
          </button>
          <button className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted" type="button" onClick={() => onRefresh(changeQueueKind(sessionID, item.queue_id, "follow_up"))}>
            <CornerDownRight className="h-3.5 w-3.5" />
            后续
          </button>
        </div>
      ) : null}
    </div>
  );
}

function queueItemText(item: { display_text?: string; text?: string; content?: string; message?: string }) {
  return item.display_text || item.text || item.content || item.message || "";
}
