import * as React from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";
import { Button } from "./button";

export function Dialog({
  open,
  title,
  children,
  closeDisabled = false,
  overlay,
  footer,
  hideClose = false,
  className,
  onOpenChange,
}: {
  open: boolean;
  title: string;
  children: React.ReactNode;
  closeDisabled?: boolean;
  overlay?: React.ReactNode;
  footer?: React.ReactNode;
  hideClose?: boolean;
  className?: string;
  onOpenChange: (open: boolean) => void;
}) {
  const dialogRef = React.useRef<HTMLDialogElement>(null);
  const titleID = React.useId();
  React.useEffect(() => {
    const dialog = dialogRef.current;
    if (!open || !dialog) return;
    const previousFocus = document.activeElement as HTMLElement | null;
    dialog.showModal();
    return () => {
      dialog.close();
      if (previousFocus?.isConnected) previousFocus.focus();
    };
  }, [open]);

  if (!open) return null;
  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={titleID}
      className="fixed inset-0 !m-0 h-[var(--app-viewport-height,100dvh)] max-h-none w-screen max-w-none border-0 bg-transparent p-0 text-foreground backdrop:bg-foreground/15 backdrop:backdrop-blur-[1px] sm:p-6"
      onCancel={(event) => {
        if (event.target !== event.currentTarget) return;
        event.preventDefault();
        if (!closeDisabled) onOpenChange(false);
      }}
    >
      <div
        className="flex h-full items-end justify-center sm:items-center"
        onMouseDown={(event) => {
          if (!closeDisabled && event.target === event.currentTarget) onOpenChange(false);
        }}
      >
        <div
          className={cn(
            "sketch-surface flex max-h-[92%] w-full max-w-3xl flex-col overflow-hidden rounded-t-2xl border-b-0 bg-background shadow-xl sm:max-h-full sm:rounded-xl sm:border-b-[1.5px]",
            className,
          )}
        >
          <div className="flex items-center justify-between border-b px-4 py-3">
            <h2 id={titleID} className="text-base font-semibold">
              {title}
            </h2>
            {!hideClose ? (
              <Button variant="ghost" size="icon" disabled={closeDisabled} onClick={() => onOpenChange(false)} aria-label="关闭">
                <X className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
          <div className={cn("relative min-h-0 flex-1 p-4", !footer && "pb-[max(1rem,env(safe-area-inset-bottom))] sm:pb-4", overlay ? "overflow-hidden" : "overflow-auto")}>
            {children}
            {overlay ? (
              <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/75 backdrop-blur-[1px]">{overlay}</div>
            ) : null}
          </div>
          {footer ? <div className="shrink-0 border-t border-border/70 px-4 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:pb-3">{footer}</div> : null}
        </div>
      </div>
    </dialog>
  );
}
