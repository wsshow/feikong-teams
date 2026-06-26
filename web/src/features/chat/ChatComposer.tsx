import { ChevronDown, Plus, Send, Square } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/cn";

const modeOptions = [
  { value: "team", label: "团队" },
  { value: "deep", label: "深度" },
  { value: "roundtable", label: "圆桌" },
  { value: "supervisor", label: "主管" },
  { value: "custom", label: "自定义" },
] as const;

export interface ChatComposerProps {
  value: string;
  mode: string;
  processing: boolean;
  variant?: "dock" | "hero";
  className?: string;
  onValueChange: (value: string) => void;
  onModeChange: (mode: string) => void;
  onSubmit: () => void;
  onStop: () => void;
}

export function ChatComposer({
  value,
  mode,
  processing,
  variant = "dock",
  className,
  onValueChange,
  onModeChange,
  onSubmit,
  onStop,
}: ChatComposerProps) {
  const [composing, setComposing] = useState(false);
  const [modeMenuOpen, setModeMenuOpen] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const isHero = variant === "hero";
  const maxTextareaHeight = isHero ? 440 : 280;

  useLayoutEffect(() => {
    resizeTextarea(textareaRef.current, maxTextareaHeight);
  }, [value, maxTextareaHeight]);

  return (
    <div
      className={cn(
        "sketch-surface w-full rounded-2xl bg-card/95 p-4",
        isHero ? "p-5" : "p-3",
        className,
      )}
    >
      <Textarea
        ref={textareaRef}
        value={value}
        onChange={(event) => {
          onValueChange(event.target.value);
          resizeTextarea(event.currentTarget, maxTextareaHeight);
        }}
        onCompositionStart={() => setComposing(true)}
        onCompositionEnd={() => setComposing(false)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey && !composing) {
            event.preventDefault();
            onSubmit();
          }
        }}
        className={cn(
          "composer-textarea resize-none border-0 bg-transparent px-1 py-0 text-base leading-7 shadow-none focus-visible:ring-0",
          isHero ? "min-h-[92px]" : "min-h-[56px]",
        )}
        placeholder={isHero ? "今天要推进什么？" : "输入任务，使用 # 引用文件，@ 指定智能体。"}
      />
      <div className="mt-3 flex items-center justify-between gap-3">
        <Button variant="ghost" size="icon" aria-label="添加附件">
          <Plus className="h-4 w-4" />
        </Button>
        <div className="flex min-w-0 items-center gap-3">
          <ModePicker
            mode={mode}
            open={modeMenuOpen}
            onOpenChange={setModeMenuOpen}
            onModeChange={onModeChange}
          />
          {processing ? (
            <Button variant="destructive" size="icon" onClick={onStop} aria-label="取消">
              <Square className="h-4 w-4" />
            </Button>
          ) : (
            <Button size="icon" onClick={onSubmit} aria-label="发送">
              <Send className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

function ModePicker({
  mode,
  open,
  onOpenChange,
  onModeChange,
}: {
  mode: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: string) => void;
}) {
  const selected = modeOptions.find((option) => option.value === mode) || modeOptions[0];

  return (
    <div className="relative">
      <button
        className="inline-flex h-9 items-center gap-1.5 rounded-lg px-2 text-sm text-muted-foreground transition-colors hover:bg-accent/60 focus-visible:outline-none focus-visible:ring-0"
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
      >
        <span>{selected.label} · {selected.value}</span>
        <ChevronDown className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")} />
      </button>
      {open ? (
        <div className="sketch-surface absolute bottom-10 right-0 z-40 w-36 rounded-xl bg-card p-1.5 text-sm shadow-[0_12px_28px_hsl(218_30%_25%/0.16)]">
          {modeOptions.map((option) => (
            <button
              key={option.value}
              className={cn(
                "flex h-9 w-full items-center rounded-lg px-3 text-left hover:bg-accent/65",
                option.value === mode && "bg-muted text-foreground",
              )}
              type="button"
              onClick={() => {
                onModeChange(option.value);
                onOpenChange(false);
              }}
            >
              {option.label} · {option.value}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function resizeTextarea(textarea: HTMLTextAreaElement | null, maxHeight: number) {
  if (!textarea) return;
  textarea.style.height = "auto";
  const nextHeight = Math.min(textarea.scrollHeight, maxHeight);
  textarea.style.height = `${nextHeight}px`;
  textarea.style.overflowY = textarea.scrollHeight > maxHeight ? "auto" : "hidden";
}
