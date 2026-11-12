import * as React from "react";
import { cn } from "@/lib/cn";

export function Badge({ className, ...props }: React.HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md border border-border bg-card/80 px-2 py-0.5 text-xs font-semibold text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}
