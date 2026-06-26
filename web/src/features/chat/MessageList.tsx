import { useEffect, useRef } from "react";
import anime from "animejs";
import { Bot, User } from "lucide-react";
import { useAppSelector } from "@/app/hooks";
import { renderMarkdown } from "@/lib/markdown";
import { cn } from "@/lib/cn";
import { ActivityCanvas } from "@/components/layout/ActivityCanvas";
import { ToolCallCard } from "./ToolCallCard";

export function MessageList() {
  const messages = useAppSelector((state) => state.chat.messages);
  const events = useAppSelector((state) => state.chat.events);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const statusText = useAppSelector((state) => state.chat.statusText);
  const error = useAppSelector((state) => state.chat.error);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const previousMessageCountRef = useRef(0);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [messages, isProcessing, statusText, error, toolEventsKey(events)]);

  useEffect(() => {
    const previous = previousMessageCountRef.current;
    previousMessageCountRef.current = messages.length;
    if (messages.length <= previous) return;
    anime({
      targets: ".message-row:last-of-type",
      opacity: [0, 1],
      translateY: [8, 0],
      duration: 180,
      easing: "easeOutQuad",
    });
  }, [messages.length]);

  if (messages.length === 0 && events.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-10 text-center">
        <div className="sketch-surface max-w-md rounded-md px-8 py-7">
          <ActivityCanvas />
          <div className="mb-3 text-lg font-semibold">准备接收任务</div>
          <p className="text-sm text-muted-foreground">当前会话尚无消息。</p>
        </div>
      </div>
    );
  }

  const toolEvents = events.flatMap((event) => event.tool_calls || (event.tool_call ? [event.tool_call] : []));

  return (
    <div className="h-full overflow-auto px-6 py-5">
      <div className="mx-auto max-w-5xl space-y-4">
        {messages.map((message) => (
          <article key={message.id} className={cn("message-row flex gap-3", message.role === "user" && "justify-end")}>
            {message.role !== "user" ? (
              <div className="sketch-surface mt-1 flex h-9 w-9 items-center justify-center rounded-md bg-secondary">
                <Bot className="h-4 w-4" />
              </div>
            ) : null}
            <div
              className={cn(
                "max-w-[78%] rounded-md px-4 py-3 text-sm",
                message.role === "user"
                  ? "border border-primary/65 bg-primary text-primary-foreground shadow-[3px_4px_0_hsl(214_45%_30%/0.14)]"
                  : "sketch-surface bg-card",
              )}
            >
              {message.agent ? <div className="mb-2 text-xs text-muted-foreground">{message.agent}</div> : null}
              <div
                className="prose prose-sm max-w-none dark:prose-invert"
                dangerouslySetInnerHTML={{ __html: renderMarkdown(message.content) }}
              />
            </div>
            {message.role === "user" ? (
              <div className="mt-1 flex h-9 w-9 items-center justify-center rounded-md border border-primary/65 bg-primary text-primary-foreground shadow-[2px_3px_0_hsl(214_45%_30%/0.14)]">
                <User className="h-4 w-4" />
              </div>
            ) : null}
          </article>
        ))}
        {isProcessing ? (
          <div className="message-row flex gap-3">
            <div className="sketch-surface mt-1 flex h-9 w-9 items-center justify-center rounded-md bg-secondary">
              <Bot className="h-4 w-4" />
            </div>
            <div className="sketch-surface rounded-md px-4 py-3 text-sm text-muted-foreground">
              {statusText || "处理中"}
              <span className="ml-1 inline-flex w-8 justify-between align-middle">
                <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60" />
                <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/45" />
                <i className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30" />
              </span>
            </div>
          </div>
        ) : null}
        {error ? <div className="sketch-surface rounded-md border-destructive/50 px-4 py-3 text-sm text-destructive">{error}</div> : null}
        {toolEvents.slice(-8).map((tool, index) => (
          <ToolCallCard key={`${tool.ref || tool.id || tool.name}-${index}`} tool={tool} />
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

function toolEventsKey(events: Array<{ tool_calls?: unknown[]; tool_call?: unknown }>) {
  return events.map((event) => (event.tool_calls?.length || 0) + (event.tool_call ? 1 : 0)).join(":");
}
