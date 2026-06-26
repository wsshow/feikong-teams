import { Paperclip, Plus, Send, Square } from "lucide-react";
import { useState } from "react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions } from "@/app/store";
import { startStream, stopStream } from "@/api/chat";
import { subscribeStream } from "@/api/stream";
import { readJSON, storageKeys, writeJSON } from "@/lib/storage";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/cn";

export function ChatInput({ variant = "dock", className }: { variant?: "dock" | "hero"; className?: string }) {
  const dispatch = useAppDispatch();
  const sessionID = useAppSelector((state) => state.chat.activeSessionID);
  const runningSessionID = useAppSelector((state) => state.chat.runningSessionID);
  const mode = useAppSelector((state) => state.chat.mode);
  const currentAgent = useAppSelector((state) => state.chat.currentAgent);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const [value, setValue] = useState("");
  const [composing, setComposing] = useState(false);

  async function submit() {
    const message = value.trim();
    if (!message || isProcessing) return;
    setValue("");
    dispatch(chatActions.setError(undefined));
    dispatch(chatActions.appendUserMessage({ id: `user-${Date.now()}`, content: message }));
    dispatch(chatActions.setProcessing(true));
    try {
      const result = await startStream({
        session_id: sessionID || undefined,
        message,
        mode,
        agent_name: currentAgent || undefined,
      });
      dispatch(chatActions.setActiveSession(result.session_id));
      dispatch(chatActions.setRunningSession(result.session_id));
      resetOffset(result.session_id);
      void subscribe(result.session_id, 0);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
      dispatch(chatActions.setProcessing(false));
    }
  }

  async function subscribe(id: string, initialOffset?: number) {
    const offsets = readJSON<Record<string, number>>(storageKeys.streamOffsets, {});
    const offset = initialOffset ?? offsets[id] ?? 0;
    await subscribeStream(id, offset, (event) => {
      dispatch(chatActions.receiveEvent(event));
      if (event.stream_event_id !== undefined) {
        offsets[id] = Number(event.stream_event_id) + 1;
        writeJSON(storageKeys.streamOffsets, offsets);
      }
    }).catch((error) => {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
      dispatch(chatActions.setProcessing(false));
    });
  }

  async function stop() {
    const id = runningSessionID || sessionID;
    if (!id) return;
    try {
      await stopStream(id);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
    } finally {
      dispatch(chatActions.setProcessing(false));
    }
  }

  const textarea = (
    <Textarea
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onCompositionStart={() => setComposing(true)}
      onCompositionEnd={() => setComposing(false)}
      onKeyDown={(event) => {
        if (event.key === "Enter" && !event.shiftKey && !composing) {
          event.preventDefault();
          void submit();
        }
      }}
      className={cn(
        "resize-none text-base leading-7",
        variant === "hero"
          ? "min-h-[92px] border-0 bg-transparent px-1 py-0 shadow-none focus-visible:ring-0"
          : "min-h-12 flex-1",
      )}
      placeholder={variant === "hero" ? "今天要推进什么？" : "输入任务，使用 # 引用文件，@ 指定智能体。"}
    />
  );

  const actionButton = isProcessing ? (
    <Button variant="destructive" size={variant === "hero" ? "icon" : "md"} onClick={stop} aria-label="取消">
      <Square className="h-4 w-4" />
      {variant === "dock" ? "取消" : null}
    </Button>
  ) : (
    <Button size={variant === "hero" ? "icon" : "md"} onClick={submit} aria-label="发送">
      <Send className="h-4 w-4" />
      {variant === "dock" ? "发送" : null}
    </Button>
  );

  if (variant === "hero") {
    return (
      <div className={cn("sketch-surface w-full rounded-2xl bg-card/95 p-5", className)}>
        {textarea}
        <div className="mt-3 flex items-center justify-between gap-3">
          <Button variant="ghost" size="icon" aria-label="添加附件">
            <Plus className="h-4 w-4" />
          </Button>
          <div className="flex min-w-0 items-center gap-3">
            <div className="hidden truncate text-sm text-muted-foreground sm:block">
              {currentAgent || "团队"} · {mode}
            </div>
            <Button variant="ghost" size="icon" aria-label="添加附件">
              <Paperclip className="h-4 w-4" />
            </Button>
            {actionButton}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={cn("sketch-rule border-t bg-background/72 p-4 backdrop-blur", className)}>
      <div className="mx-auto flex max-w-5xl gap-3 rounded-2xl bg-card/75 p-2 shadow-[0_1px_0_hsl(218_30%_76%/0.36)]">
        <Button variant="outline" size="icon" aria-label="添加附件">
          <Paperclip className="h-4 w-4" />
        </Button>
        {textarea}
        {actionButton}
      </div>
    </div>
  );
}

function resetOffset(sessionID: string) {
  const offsets = readJSON<Record<string, number>>(storageKeys.streamOffsets, {});
  delete offsets[sessionID];
  writeJSON(storageKeys.streamOffsets, offsets);
}
