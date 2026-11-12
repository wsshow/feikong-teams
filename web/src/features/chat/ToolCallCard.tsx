import { Check, ChevronRight, Circle, CircleAlert, CircleSlash, Clock3, LoaderCircle } from "lucide-react";
import { type MouseEvent, type ReactNode, useId } from "react";
import type { ToolCallDTO } from "@/types/events";
import { cn } from "@/lib/cn";
import { useDisclosureState } from "./disclosureState";

export function ToolCallCard({
  tool,
  title,
  children,
  disclosureID,
}: {
  tool: ToolCallDTO;
  title?: ReactNode;
  children?: ReactNode;
  disclosureID?: string;
}) {
  const [open, toggleOpen] = useDisclosureState(disclosureID || tool.ref || tool.id || tool.name || "tool");
  const detailsID = useId();
  const state = toolState(tool);
  const StatusIcon = state.icon;
  const summary = toolSummary(tool);
  const label = title || tool.display_name || tool.name || "tool";
  const isAgentDispatch = isAgentDispatchTool(tool);
  const showArguments = Boolean(tool.arguments);
  const showResult = Boolean(tool.result && !(isAgentDispatch && children));

  function handleToggle(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    event.stopPropagation();
    toggleOpen();
  }

  return (
    <div className="min-w-0 text-sm" data-tool-call>
      <button
        className={cn(
          "flex min-h-11 w-full min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left transition-colors hover:bg-muted/70 active:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:min-h-10",
          state.failed ? "text-destructive" : "text-muted-foreground",
        )}
        onClick={handleToggle}
        type="button"
        aria-expanded={open}
        aria-controls={open ? detailsID : undefined}
      >
        <StatusIcon
          aria-hidden="true"
          className={cn("h-3.5 w-3.5 shrink-0", state.tone, state.spinning && "animate-spin motion-reduce:animate-none")}
        />
        <span className="max-w-[55%] truncate font-medium" title={typeof label === "string" ? label : undefined}>{label}</span>
        {summary ? <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground" title={summary}>{summary}</span> : <span className="flex-1" />}
        <span className={cn("shrink-0 text-xs", state.tone, !state.showLabel && "sr-only")}>{state.label}</span>
        <ChevronRight aria-hidden="true" className={cn("h-3.5 w-3.5 shrink-0 transition-transform motion-reduce:transition-none", open && "rotate-90")} />
      </button>
      {open ? (
        <div id={detailsID} className="ml-4 space-y-2 border-l border-border/60 pb-2 pl-4 pt-1">
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

function toolSummary(tool: ToolCallDTO) {
  const args = parseObject(tool.arguments);
  // 只提取能说明操作目标的字段，不把整个参数对象带到折叠行。
  for (const key of ["command", "cmd", "pattern", "query", "filepath", "file_path", "path", "url", "script_path", "title", "task", "description", "reason", "task_action", "attachment_id", "file_id"]) {
    const value = args?.[key];
    if (typeof value === "string" && value.trim()) return value.replace(/\s+/g, " ").trim();
  }
  return (tool.target || "").replace(/\s+/g, " ").trim();
}

function toolState(tool: ToolCallDTO) {
  const failed = tool.status === "error";
  if (failed) return { icon: CircleAlert, label: "失败", tone: "text-destructive", showLabel: true, failed: true };
  if (tool.status === "running") return { icon: LoaderCircle, label: "运行中", tone: "text-primary", showLabel: true, spinning: true };
  if (tool.status === "pending") return { icon: Clock3, label: "等待中", tone: "text-muted-foreground", showLabel: true };
  if (tool.status === "completed") return { icon: Check, label: "已完成", tone: "text-success/75", showLabel: false };
  if (tool.status === "interrupted") return { icon: CircleSlash, label: "执行中断", tone: "text-muted-foreground", showLabel: true };
  if (tool.status === "cancelled") return { icon: CircleSlash, label: "已取消", tone: "text-muted-foreground", showLabel: true };
  return { icon: Circle, label: "状态未知", tone: "text-muted-foreground/60", showLabel: false };
}

function parseObject(value?: string): Record<string, unknown> | undefined {
  if (!value) return undefined;
  try {
    const parsed: unknown = JSON.parse(value);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed as Record<string, unknown> : undefined;
  } catch {
    return undefined;
  }
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
