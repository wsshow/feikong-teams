import * as React from "react";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "确认",
  cancelLabel = "取消",
  destructive = false,
  busy = false,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  description: React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  destructive?: boolean;
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      onMouseDown={(event) => {
        if (!busy && event.target === event.currentTarget) onCancel();
      }}
    >
      <div className="sketch-surface w-full max-w-md rounded-2xl bg-card/95 p-5 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        <div className="flex items-start gap-3">
          <div
            className={cn(
              "mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border",
              destructive ? "border-destructive/30 bg-destructive/10 text-destructive" : "border-primary/30 bg-primary/10 text-primary",
            )}
          >
            <AlertTriangle className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="confirm-dialog-title" className="text-lg font-semibold text-foreground">
              {title}
            </h2>
            <div className="mt-2 text-sm leading-6 text-muted-foreground">{description}</div>
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={busy}>
            {cancelLabel}
          </Button>
          <Button variant={destructive ? "destructive" : "default"} onClick={onConfirm} disabled={busy}>
            {busy ? "处理中" : confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function TextInputDialog({
  open,
  title,
  label,
  description,
  value,
  placeholder,
  confirmLabel = "创建",
  busy = false,
  onValueChange,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  label: string;
  description?: React.ReactNode;
  value: string;
  placeholder?: string;
  confirmLabel?: string;
  busy?: boolean;
  onValueChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!open) return null;
  const trimmed = value.trim();
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="text-input-dialog-title"
      onMouseDown={(event) => {
        if (!busy && event.target === event.currentTarget) onCancel();
      }}
    >
      <div className="sketch-surface w-full max-w-md rounded-2xl bg-card/95 p-5 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        <h2 id="text-input-dialog-title" className="text-lg font-semibold text-foreground">
          {title}
        </h2>
        {description ? <div className="mt-2 text-sm leading-6 text-muted-foreground">{description}</div> : null}
        <label className="mt-4 block space-y-1 text-sm font-medium">
          <span>{label}</span>
          <Input
            autoFocus
            disabled={busy}
            value={value}
            placeholder={placeholder}
            onChange={(event) => onValueChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape" && !busy) onCancel();
              if (event.key === "Enter" && trimmed && !busy) onConfirm();
            }}
          />
        </label>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={busy}>
            取消
          </Button>
          <Button onClick={onConfirm} disabled={!trimmed || busy}>
            {busy ? "处理中" : confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
