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
  onOpenChange,
}: {
  open: boolean;
  title: string;
  children: React.ReactNode;
  closeDisabled?: boolean;
  overlay?: React.ReactNode;
  onOpenChange: (open: boolean) => void;
}) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true">
      <div className={cn("max-h-[88vh] w-full max-w-3xl overflow-hidden rounded-md border bg-background shadow-xl")}>
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 className="text-sm font-semibold">{title}</h2>
          <Button variant="ghost" size="icon" disabled={closeDisabled} onClick={() => onOpenChange(false)} aria-label="关闭">
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className={cn("relative max-h-[75vh] p-4", overlay ? "overflow-hidden" : "overflow-auto")}>
          {children}
          {overlay ? <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/75 backdrop-blur-[1px]">{overlay}</div> : null}
        </div>
      </div>
    </div>
  );
}
