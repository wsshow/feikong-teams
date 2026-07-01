import { ChevronRight } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { ToolCallDTO } from "@/types/events";
import { cn } from "@/lib/cn";

export function ToolCallCard({ tool, title, children }: { tool: ToolCallDTO; title?: ReactNode; children?: ReactNode }) {
  const [open, setOpen] = useState(false);
  const isRunning = tool.status === "running" || tool.status === "pending";
  const isError = tool.status === "error";
  const label = title || tool.display_name || tool.name || "tool";
  const isAgentDispatch = isAgentDispatchTool(tool);
  const runningTone = "text-amber-600";
  const runningDotTone = "bg-amber-400";
  const showArguments = Boolean(tool.arguments);
  const showResult = Boolean(tool.result && !(isAgentDispatch && children));
  return (
    <div className="-ml-2 text-sm">
      <button
        className={cn(
          "flex items-center gap-3 rounded-lg px-2 py-2 text-left transition-colors hover:bg-muted/70",
          isError ? "text-destructive" : isRunning ? runningTone : "text-muted-foreground",
        )}
        onClick={() => setOpen(!open)}
        type="button"
      >
        <span
          className={cn("h-2 w-2 rounded-full", isError ? "bg-destructive" : isRunning ? runningDotTone : "bg-muted-foreground/35")}
        />
        <span className="font-semibold">{label}</span>
        <ChevronRight className={cn("h-4 w-4 transition-transform", open && "rotate-90")} />
      </button>
      {open ? (
        <div className="ml-7 space-y-2 border-l border-border/60 pl-4 pt-2">
          {tool.member_name || tool.target ? <div className="text-xs text-muted-foreground">{tool.member_name || tool.target}</div> : null}
          {showArguments ? (
            <pre className="max-h-40 overflow-y-auto overflow-x-hidden whitespace-pre-wrap break-words rounded-lg bg-muted/45 p-3 text-xs leading-5 text-muted-foreground">
              {formatArgs(tool.arguments || "")}
            </pre>
          ) : null}
          {showResult ? (
            <pre className="max-h-56 overflow-y-auto overflow-x-hidden whitespace-pre-wrap break-words rounded-lg bg-muted/45 p-3 text-xs leading-5 text-foreground">
              {formatResult(tool.result || "")}
            </pre>
          ) : null}
          {children}
        </div>
      ) : null}
    </div>
  );
}

function isAgentDispatchTool(tool: ToolCallDTO) {
  return tool.kind === "agent" || tool.name?.startsWith("ask_fkagent_");
}

function formatArgs(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

function formatResult(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return "";
  const formatted = formatJSON(trimmed);
  if (formatted.length > 5000) return `${formatted.slice(0, 5000)}\n...`;
  return formatted;
}

function formatJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}
