import { Bot, ChevronDown, FileText, Plus, Send, Square, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import type { AgentInfo } from "@/types/api";
import type { FileEntry } from "@/types/files";

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
  agents?: AgentInfo[];
  selectedAgent?: string;
  fileSuggestions?: FileEntry[];
  referenceLoading?: boolean;
  variant?: "dock" | "hero";
  className?: string;
  onValueChange: (value: string) => void;
  onModeChange: (mode: string) => void;
  onReferenceQuery?: (query: string) => void;
  onAgentChange?: (agent: string) => void;
  onSubmit: () => void;
  onStop: () => void;
}

export function ChatComposer({
  value,
  mode,
  processing,
  agents = [],
  selectedAgent,
  fileSuggestions = [],
  referenceLoading = false,
  variant = "dock",
  className,
  onValueChange,
  onModeChange,
  onReferenceQuery,
  onAgentChange,
  onSubmit,
  onStop,
}: ChatComposerProps) {
  const [composing, setComposing] = useState(false);
  const [modeMenuOpen, setModeMenuOpen] = useState(false);
  const editorRef = useRef<HTMLDivElement | null>(null);
  const isHero = variant === "hero";
  const maxTextareaHeight = isHero ? 440 : 280;
  const [trigger, setTrigger] = useState<ReferenceTrigger | undefined>();
  const [activeReferenceIndex, setActiveReferenceIndex] = useState(0);
  const filteredAgents = useMemo(() => {
    if (trigger?.kind !== "agent") return [];
    const query = trigger.query.toLowerCase();
    return agents
      .filter((agent) => {
        const text = `${agent.name} ${agent.description || ""} ${agent.role || ""}`.toLowerCase();
        return text.includes(query);
      })
      .slice(0, 8);
  }, [agents, trigger]);
  const referenceOptions = useMemo<ReferenceOption[]>(() => {
    if (!trigger) return [];
    if (trigger.kind === "file") {
      return fileSuggestions.slice(0, 10).map((file) => ({
        kind: "file" as const,
        key: file.path,
        label: file.path,
        file,
      }));
    }
    return filteredAgents.map((agent) => ({
      kind: "agent" as const,
      key: agent.name,
      label: agent.name,
      agent,
    }));
  }, [fileSuggestions, filteredAgents, trigger]);

  useEffect(() => {
    if (trigger?.kind === "file") onReferenceQuery?.(trigger.query);
  }, [onReferenceQuery, trigger]);

  useEffect(() => {
    setActiveReferenceIndex(0);
  }, [trigger?.kind, trigger?.query, referenceOptions.length]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    if (!value && editorText(editor)) {
      editor.replaceChildren();
      setTrigger(undefined);
    }
  }, [value]);

  function syncFromEditor() {
    const editor = editorRef.current;
    if (!editor) return;
    const nextValue = editorText(editor);
    onValueChange(nextValue);
    updateTriggerFromEditor();
  }

  function updateTriggerFromEditor() {
    const editor = editorRef.current;
    if (!editor) return;
    const text = editorText(editor);
    const cursor = caretTextOffset(editor);
    setTrigger(cursor === undefined ? undefined : resolveTriggerAt(text, cursor));
  }

  function insertFileToken(path: string) {
    if (!trigger || !editorRef.current) return;
    replaceTextRangeWithFileToken(editorRef.current, trigger.start, trigger.end, `#${path}`);
    const nextValue = editorText(editorRef.current);
    onValueChange(nextValue);
    setTrigger(undefined);
    requestAnimationFrame(() => editorRef.current?.focus());
  }

  function selectAgent(agent: string) {
    if (!trigger || !editorRef.current) return;
    replaceTextRange(editorRef.current, trigger.start, trigger.end, "");
    const nextValue = editorText(editorRef.current);
    onValueChange(nextValue);
    onAgentChange?.(agent);
    setTrigger(undefined);
    requestAnimationFrame(() => editorRef.current?.focus());
  }

  function selectReferenceOption(option: ReferenceOption) {
    if (option.kind === "agent") {
      selectAgent(option.agent.name);
      return;
    }
    insertFileToken(option.file.path);
  }

  function handleReferenceNavigation(event: React.KeyboardEvent<HTMLDivElement>) {
    if (!trigger) return false;
    if (event.key === "Escape") {
      event.preventDefault();
      setTrigger(undefined);
      return true;
    }
    if (!referenceOptions.length) return false;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveReferenceIndex((index) => (index + 1) % referenceOptions.length);
      return true;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveReferenceIndex((index) => (index - 1 + referenceOptions.length) % referenceOptions.length);
      return true;
    }
    if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      selectReferenceOption(referenceOptions[Math.min(activeReferenceIndex, referenceOptions.length - 1)]);
      return true;
    }
    return false;
  }

  function deleteToken(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "Backspace" && event.key !== "Delete") return false;
    const editor = event.currentTarget;
    const selection = selectionTextRange(editor);
    if (!selection) return false;
    const currentValue = editorText(editor);
    const deleted = deleteInlineReferenceToken(currentValue, selection.start, selection.end, event.key);
    if (!deleted) return false;
    event.preventDefault();
    replaceTextRange(editor, deleted.start, deleted.end, "");
    const nextValue = editorText(editor);
    onValueChange(nextValue);
    requestAnimationFrame(() => {
      editor.focus();
      setCaretTextOffset(editor, deleted.cursor);
      updateTriggerFromEditor();
    });
    return true;
  }

  return (
    <div
      className={cn(
        "sketch-surface relative w-full rounded-2xl bg-card/95 p-4",
        isHero ? "p-5" : "p-3",
        className,
      )}
    >
      <div className="relative">
        {!value ? (
          <div className="pointer-events-none absolute left-1 top-0 text-base leading-7 text-muted-foreground">
            {isHero ? "今天要推进什么？" : "输入任务，使用 # 引用文件，@ 指定智能体。"}
          </div>
        ) : null}
        <div
          ref={editorRef}
          contentEditable
          suppressContentEditableWarning
          role="textbox"
          aria-multiline="true"
          onCompositionStart={() => setComposing(true)}
          onCompositionEnd={() => setComposing(false)}
          onInput={syncFromEditor}
          onClick={updateTriggerFromEditor}
          onKeyUp={updateTriggerFromEditor}
          onKeyDown={(event) => {
            if (handleReferenceNavigation(event)) return;
            if (deleteToken(event)) return;
            if (event.key === "Enter" && !event.shiftKey && !composing) {
              event.preventDefault();
              onSubmit();
            }
          }}
          className={cn(
            "composer-textarea min-w-0 overflow-y-auto whitespace-pre-wrap break-words border-0 bg-transparent px-1 py-0 text-base leading-7 text-foreground outline-none focus-visible:ring-0",
            isHero ? "min-h-[92px]" : "min-h-[56px]",
          )}
          style={{ maxHeight: maxTextareaHeight }}
        />
      </div>
      {trigger ? (
        <ReferenceMenu
          trigger={trigger}
          agents={filteredAgents}
          files={fileSuggestions}
          loading={referenceLoading}
          activeIndex={activeReferenceIndex}
          onSelectAgent={selectAgent}
          onSelectFile={insertFileToken}
          onActiveIndexChange={setActiveReferenceIndex}
        />
      ) : null}
      <div className="mt-3 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <Button variant="ghost" size="icon" aria-label="添加附件">
            <Plus className="h-4 w-4" />
          </Button>
          {selectedAgent ? (
            <AgentTargetChip agent={selectedAgent} onClear={() => onAgentChange?.("")} />
          ) : null}
        </div>
        <div className="flex min-w-0 items-center gap-3">
          <ModePicker
            mode={mode}
            selectedAgent={selectedAgent}
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

function ReferenceMenu({
  trigger,
  agents,
  files,
  loading,
  activeIndex,
  onSelectAgent,
  onSelectFile,
  onActiveIndexChange,
}: {
  trigger: ReferenceTrigger;
  agents: AgentInfo[];
  files: FileEntry[];
  loading: boolean;
  activeIndex: number;
  onSelectAgent: (agent: string) => void;
  onSelectFile: (path: string) => void;
  onActiveIndexChange: (index: number) => void;
}) {
  const isFile = trigger.kind === "file";
  const emptyText = isFile ? "没有匹配文件" : "没有匹配智能体";

  return (
    <div className="sketch-surface absolute bottom-[calc(100%+0.75rem)] left-0 z-40 max-h-64 w-[min(32rem,100%)] overflow-y-auto rounded-xl bg-card p-1.5 text-sm shadow-[0_14px_32px_hsl(218_30%_25%/0.16)]">
      {isFile ? (
        <>
          {loading ? <div className="px-3 py-2 text-muted-foreground">搜索文件中...</div> : null}
          {!loading && files.length === 0 ? <div className="px-3 py-2 text-muted-foreground">{emptyText}</div> : null}
          {files.slice(0, 10).map((file, index) => (
            <button
              key={file.path}
              type="button"
              className={cn(
                "flex w-full min-w-0 items-center gap-2 rounded-lg px-3 py-2 text-left hover:bg-accent/65",
                index === activeIndex && "bg-accent/70 text-foreground",
              )}
              onMouseEnter={() => onActiveIndexChange(index)}
              onClick={() => onSelectFile(file.path)}
            >
              <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">{file.path}</span>
            </button>
          ))}
        </>
      ) : (
        <>
          {agents.length === 0 ? <div className="px-3 py-2 text-muted-foreground">{emptyText}</div> : null}
          {agents.map((agent, index) => (
            <button
              key={agent.name}
              type="button"
              className={cn(
                "flex w-full min-w-0 items-center gap-2 rounded-lg px-3 py-2 text-left hover:bg-accent/65",
                index === activeIndex && "bg-accent/70 text-foreground",
              )}
              onMouseEnter={() => onActiveIndexChange(index)}
              onClick={() => onSelectAgent(agent.name)}
            >
              <Bot className="h-4 w-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{agent.name}</span>
                {agent.description ? <span className="block truncate text-xs text-muted-foreground">{agent.description}</span> : null}
              </span>
            </button>
          ))}
        </>
      )}
    </div>
  );
}

function AgentTargetChip({ agent, onClear }: { agent: string; onClear: () => void }) {
  return (
    <span className="group inline-flex h-9 max-w-[15rem] items-center gap-1.5 rounded-lg px-2 text-sm text-muted-foreground transition-colors hover:bg-muted/65 hover:text-foreground">
      <span className="relative h-4 w-4 shrink-0">
        <Bot className="absolute inset-0 h-4 w-4 text-muted-foreground/80 transition-opacity group-hover:opacity-0" />
        <button
          type="button"
          className="absolute inset-0 rounded-full text-muted-foreground/70 opacity-0 transition-opacity hover:bg-background/70 hover:text-foreground group-hover:opacity-100"
          aria-label="清空指定智能体"
          onClick={onClear}
        >
          <X className="h-4 w-4" />
        </button>
      </span>
      <span className="min-w-0 truncate">{agent}</span>
    </span>
  );
}

function ModePicker({
  mode,
  selectedAgent,
  open,
  onOpenChange,
  onModeChange,
}: {
  mode: string;
  selectedAgent?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: string) => void;
}) {
  const selected = modeOptions.find((option) => option.value === mode) || modeOptions[0];
  const label = selectedAgent ? `智能体 · ${selectedAgent}` : `${selected.label} · ${selected.value}`;

  return (
    <div className="relative">
      <button
        className="inline-flex h-9 items-center gap-1.5 rounded-lg px-2 text-sm text-muted-foreground transition-colors hover:bg-accent/60 focus-visible:outline-none focus-visible:ring-0"
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
      >
        <span>{label}</span>
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

interface InlineReferenceToken {
  start: number;
  end: number;
  value: string;
  kind: "file" | "agent";
}

function deleteInlineReferenceToken(
  value: string,
  selectionStart: number,
  selectionEnd: number,
  key: "Backspace" | "Delete",
) {
  const tokens = inlineReferenceTokens(value);
  if (!tokens.length) return undefined;

  const range = selectionStart === selectionEnd
    ? tokenNearCursor(tokens, selectionStart, key)
    : tokenIntersectingSelection(tokens, selectionStart, selectionEnd);
  if (!range) return undefined;

  const start = Math.min(range.start, selectionStart, selectionEnd);
  const end = Math.max(range.end, selectionStart, selectionEnd);
  return {
    start,
    end,
    value: `${value.slice(0, start)}${value.slice(end)}`.replace(/[ \t]{2,}/g, " "),
    cursor: start,
  };
}

function inlineReferenceTokens(value: string) {
  const tokens: InlineReferenceToken[] = [];
  const pattern = /(#[^\s#@]+)/g;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(value)) !== null) {
    const token = match[0];
    tokens.push({
      start: match.index,
      end: match.index + token.length,
      value: token,
      kind: "file",
    });
  }

  return tokens;
}

function tokenNearCursor(tokens: InlineReferenceToken[], cursor: number, key: "Backspace" | "Delete") {
  return tokens.find((token) => {
    if (cursor > token.start && cursor < token.end) return true;
    if (key === "Backspace") return cursor === token.end || cursor === token.end + 1;
    return cursor === token.start;
  });
}

function tokenIntersectingSelection(tokens: InlineReferenceToken[], selectionStart: number, selectionEnd: number) {
  return tokens
    .filter((token) => token.end > selectionStart && token.start < selectionEnd)
    .reduce<InlineReferenceToken | undefined>((merged, token) => {
      if (!merged) return token;
      return {
        start: Math.min(merged.start, token.start),
        end: Math.max(merged.end, token.end),
        value: "",
        kind: token.kind,
      };
    }, undefined);
}

function editorText(root: HTMLElement) {
  let text = "";
  const visit = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      text += node.textContent || "";
      return;
    }
    if (node instanceof HTMLElement && node.dataset.referenceToken) {
      text += node.textContent || "";
      return;
    }
    if (node.nodeName === "BR") {
      text += "\n";
      return;
    }
    node.childNodes.forEach(visit);
  };
  root.childNodes.forEach(visit);
  return text.replace(/\u00a0/g, " ");
}

function selectionTextRange(root: HTMLElement) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) return undefined;
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return undefined;

  const beforeStart = document.createRange();
  beforeStart.selectNodeContents(root);
  beforeStart.setEnd(range.startContainer, range.startOffset);
  const beforeEnd = document.createRange();
  beforeEnd.selectNodeContents(root);
  beforeEnd.setEnd(range.endContainer, range.endOffset);

  return {
    start: beforeStart.toString().replace(/\u00a0/g, " ").length,
    end: beforeEnd.toString().replace(/\u00a0/g, " ").length,
  };
}

function caretTextOffset(root: HTMLElement) {
  const range = selectionTextRange(root);
  if (!range || range.start !== range.end) return undefined;
  return range.start;
}

function replaceTextRange(root: HTMLElement, start: number, end: number, text: string) {
  const range = rangeFromTextOffsets(root, start, end);
  if (!range) return;
  range.deleteContents();
  const inserted = text ? document.createTextNode(text) : undefined;
  if (inserted) range.insertNode(inserted);
  const cursor = start + text.length;
  setCaretTextOffset(root, cursor);
}

function replaceTextRangeWithFileToken(root: HTMLElement, start: number, end: number, token: string) {
  const range = rangeFromTextOffsets(root, start, end);
  if (!range) return;
  range.deleteContents();
  const tokenNode = document.createElement("span");
  tokenNode.dataset.referenceToken = "true";
  tokenNode.dataset.kind = "file";
  tokenNode.contentEditable = "false";
  tokenNode.textContent = token;
  tokenNode.className = cn(
    "mx-0.5 inline-flex select-none items-center rounded-md border border-primary/25 bg-primary/10 px-1.5 py-0.5 text-sm leading-5 text-primary align-baseline",
  );
  const spaceNode = document.createTextNode(" ");
  range.insertNode(spaceNode);
  range.insertNode(tokenNode);
  setCaretAfterNode(root, spaceNode);
}

function setCaretAfterNode(root: HTMLElement, node: Node) {
  const range = document.createRange();
  range.setStartAfter(node);
  range.collapse(true);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
  root.focus();
}

function setCaretTextOffset(root: HTMLElement, offset: number) {
  const range = rangeFromTextOffsets(root, offset, offset);
  if (!range) return;
  range.collapse(true);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

function rangeFromTextOffsets(root: HTMLElement, start: number, end: number) {
  const range = document.createRange();
  const startPoint = domPointFromTextOffset(root, start);
  const endPoint = domPointFromTextOffset(root, end);
  if (!startPoint || !endPoint) return undefined;
  range.setStart(startPoint.node, startPoint.offset);
  range.setEnd(endPoint.node, endPoint.offset);
  return range;
}

function domPointFromTextOffset(root: HTMLElement, target: number): { node: Node; offset: number } | undefined {
  let offset = 0;

  const scan = (node: Node): { node: Node; offset: number } | undefined => {
    if (node.nodeType === Node.TEXT_NODE) {
      const length = node.textContent?.length || 0;
      if (target <= offset + length) return { node, offset: Math.max(0, target - offset) };
      offset += length;
      return undefined;
    }
    if (node instanceof HTMLElement && node.dataset.referenceToken) {
      const length = node.textContent?.length || 0;
      if (target <= offset) return { node: node.parentNode || root, offset: childIndex(node) };
      if (target <= offset + length) return { node: node.parentNode || root, offset: childIndex(node) + 1 };
      offset += length;
      return undefined;
    }
    if (node.nodeName === "BR") {
      if (target <= offset + 1) return { node: node.parentNode || root, offset: childIndex(node) + 1 };
      offset += 1;
      return undefined;
    }
    for (const child of Array.from(node.childNodes)) {
      const point = scan(child);
      if (point) return point;
    }
    return undefined;
  };

  for (const child of Array.from(root.childNodes)) {
    const point = scan(child);
    if (point) return point;
  }

  return { node: root, offset: root.childNodes.length };
}

function childIndex(node: Node) {
  let index = 0;
  let current = node.previousSibling;
  while (current) {
    index += 1;
    current = current.previousSibling;
  }
  return index;
}

function resolveTriggerAt(value: string, cursor: number): ReferenceTrigger | undefined {
  const beforeCursor = value.slice(0, cursor);
  const match = /(^|\s)([#@])([^\s#@]*)$/.exec(beforeCursor);
  if (!match) return undefined;
  const markerIndex = cursor - match[0].length + match[1].length;
  return {
    kind: match[2] === "#" ? "file" : "agent",
    query: match[3] || "",
    start: markerIndex,
    end: cursor,
  };
}

interface ReferenceTrigger {
  kind: "file" | "agent";
  query: string;
  start: number;
  end: number;
}

type ReferenceOption =
  | {
      kind: "file";
      key: string;
      label: string;
      file: FileEntry;
    }
  | {
      kind: "agent";
      key: string;
      label: string;
      agent: AgentInfo;
    };
