import { Bot, Check, ChevronDown, MessageSquare, Plus, Search, Sparkles, Trash2, Wrench, X } from "lucide-react";
import { useEffect, useState } from "react";
import { rewriteText } from "@/api/ai";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import type { ModelConfig } from "@/types/config";
import { type ToolSelectOption, uniqueToolNames, modelUseOptions } from "./configModel";

export function AITextField({
  label,
  scenario,
  value,
  context,
  onChange,
  disabled,
}: {
  label: string;
  scenario: string;
  value?: string;
  context?: Record<string, unknown>;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <Field label={label} action={disabled ? undefined : <AIRewriteButton scenario={scenario} value={value || ""} context={context} onApply={onChange} />}>
      <Input value={value || ""} disabled={disabled} onChange={(event) => onChange(event.target.value)} />
    </Field>
  );
}

export function AIRewriteButton({
  scenario,
  value,
  context,
  onApply,
}: {
  scenario: string;
  value: string;
  context?: Record<string, unknown>;
  onApply: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
        <Sparkles className="h-4 w-4" />
        AI 修改
      </Button>
      {open ? <AIRewriteDialog scenario={scenario} value={value} context={context} onApply={onApply} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function AIRewriteDialog({
  scenario,
  value,
  context,
  onApply,
  onClose,
}: {
  scenario: string;
  value: string;
  context?: Record<string, unknown>;
  onApply: (value: string) => void;
  onClose: () => void;
}) {
  const [instruction, setInstruction] = useState("");
  const [result, setResult] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function runRewrite() {
    const trimmed = instruction.trim();
    if (!trimmed || loading) return;
    setLoading(true);
    setError("");
    try {
      const resp = await rewriteText({ scenario, instruction: trimmed, text: value, context });
      setResult(resp.text || "");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  function apply() {
    if (!result.trim()) return;
    onApply(result);
    onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6" role="dialog" aria-modal="true">
      <div className="sketch-surface flex max-h-[calc(100dvh-1.5rem)] w-full max-w-2xl flex-col overflow-hidden rounded-2xl bg-card/95 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        <div className="flex items-start justify-between gap-3 border-b border-border/70 p-4">
          <div>
            <div className="flex items-center gap-2 font-semibold">
              <Sparkles className="h-4 w-4 text-primary" />
              AI 修改
            </div>
            <div className="mt-1 text-sm leading-6 text-muted-foreground">输入修改要求，生成结果确认后再应用到字段。</div>
          </div>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label="关闭">
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className="chat-scroll min-h-0 flex-1 space-y-4 overflow-auto p-4">
          <Field label="修改要求">
            <Textarea className="min-h-24 text-sm" value={instruction} placeholder="例如：更专业、更简洁，并增加输出边界。" onChange={(event) => setInstruction(event.target.value)} />
          </Field>
          <Field label="当前内容">
            <Textarea className="min-h-28 text-sm" value={value} readOnly />
          </Field>
          {error ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{error}</div> : null}
          {result ? (
            <Field label="生成结果">
              <Textarea className="min-h-40 text-sm" value={result} onChange={(event) => setResult(event.target.value)} />
            </Field>
          ) : null}
        </div>
        <div className="flex flex-col gap-2 border-t border-border/70 p-4 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button variant="outline" disabled={!instruction.trim() || loading} onClick={() => void runRewrite()}>
            <Sparkles className="h-4 w-4" />
            {loading ? "生成中" : "生成修改"}
          </Button>
          <Button disabled={!result.trim() || loading} onClick={apply}>
            应用
          </Button>
        </div>
      </div>
    </div>
  );
}

export function ToolSelectField({
  tools,
  options,
  onChange,
  disabled,
}: {
  tools: string[];
  options: ToolSelectOption[];
  onChange: (tools: string[]) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selectedTools = uniqueToolNames(tools);
  const selectedSet = new Set(selectedTools);
  const optionByName = new Map(options.map((option) => [option.name, option]));
  const normalizedQuery = query.trim().toLowerCase();
  const visibleOptions = options.filter((option) => {
    if (!normalizedQuery) return true;
    return `${option.name} ${option.label} ${option.description || ""} ${option.category || ""}`.toLowerCase().includes(normalizedQuery);
  });
  const canAddCustom = Boolean(query.trim() && !selectedSet.has(query.trim()) && !optionByName.has(query.trim()));

  function addTool(name: string) {
    const trimmed = name.trim();
    if (!trimmed || selectedSet.has(trimmed)) return;
    onChange([...selectedTools, trimmed]);
    setQuery("");
  }

  function removeTool(name: string) {
    onChange(selectedTools.filter((tool) => tool !== name));
  }

  function toggleTool(name: string) {
    if (selectedSet.has(name)) {
      removeTool(name);
      return;
    }
    addTool(name);
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">工具</div>
        <Button variant="outline" size="sm" disabled={disabled} onClick={() => setOpen((value) => !value)}>
          <Plus className="h-4 w-4" />
          添加工具
        </Button>
      </div>

      <div className="min-h-12 rounded-xl border border-border/75 bg-background/45 p-2">
        {selectedTools.length ? (
          <div className="flex flex-wrap gap-2">
            {selectedTools.map((tool) => {
              const option = optionByName.get(tool);
              return (
                <span
                  key={tool}
                  className="inline-flex max-w-full items-center gap-1.5 rounded-lg border border-border/80 bg-card/85 px-2 py-1 text-sm"
                >
                  <Wrench className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{option?.label || tool}</span>
                  {option?.source === "mcp" ? <Badge>MCP</Badge> : null}
                  {option?.source === "custom" ? <Badge>自定义</Badge> : null}
                  {!option ? <Badge>手动</Badge> : null}
                  {disabled ? null : (
                    <button
                      type="button"
                      className="ml-0.5 rounded-full text-muted-foreground transition-colors hover:text-foreground"
                      aria-label={`移除工具 ${tool}`}
                      onClick={() => removeTool(tool)}
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                </span>
              );
            })}
          </div>
        ) : (
          <div className="flex min-h-8 items-center text-sm text-muted-foreground">还没有选择工具</div>
        )}
      </div>

      {open ? (
        <div className="rounded-xl border border-border/75 bg-card/70 p-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-8"
              value={query}
              placeholder="搜索工具或 MCP 服务"
              onChange={(event) => setQuery(event.target.value)}
            />
          </div>
          <div className="mt-2 max-h-64 space-y-1 overflow-y-auto pr-1">
            {visibleOptions.map((option) => {
              const selected = selectedSet.has(option.name);
              return (
                <button
                  key={option.name}
                  type="button"
                  className={cn(
                    "flex w-full items-start gap-2 rounded-lg px-2 py-2 text-left transition-colors hover:bg-accent/60",
                    selected && "bg-accent/55",
                  )}
                  onClick={() => toggleTool(option.name)}
                >
                  <span
                    className={cn(
                      "mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md border",
                      selected ? "border-primary bg-primary text-primary-foreground" : "border-border bg-background/70",
                    )}
                  >
                    {selected ? <Check className="h-3.5 w-3.5" /> : null}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 flex-wrap items-center gap-1.5">
                      <span className="truncate font-medium">{option.label}</span>
                      <span className="text-xs text-muted-foreground">{option.name}</span>
                      {option.source === "mcp" ? <Badge>MCP</Badge> : null}
                      {option.readOnly ? <Badge>只读</Badge> : null}
                      {option.destructive ? <Badge>破坏性</Badge> : null}
                      {option.enabled === false ? <Badge>未启用</Badge> : null}
                    </span>
                    {option.description ? <span className="mt-0.5 block line-clamp-2 text-xs text-muted-foreground">{option.description}</span> : null}
                  </span>
                </button>
              );
            })}
            {canAddCustom ? (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm transition-colors hover:bg-accent/60"
                onClick={() => addTool(query)}
              >
                <Plus className="h-4 w-4 text-muted-foreground" />
                <span>添加自定义工具名</span>
                <span className="min-w-0 truncate font-medium">{query.trim()}</span>
              </button>
            ) : null}
            {!visibleOptions.length && !canAddCustom ? (
              <div className="px-2 py-6 text-center text-sm text-muted-foreground">没有匹配的工具</div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function ChannelCard({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return (
    <Panel>
      <PanelHeader>
        <SectionTitle icon={MessageSquare} title={title} description={description} />
      </PanelHeader>
      <PanelBody className="space-y-4">{children}</PanelBody>
    </Panel>
  );
}

export function ExtensionConfigCard({
  title,
  id,
  description,
  badges,
  metrics,
  expanded,
  onToggle,
  onAI,
  onRemove,
  children,
}: {
  title: string;
  id?: string;
  description?: string;
  badges: string[];
  metrics: Array<{ label: string; value: string }>;
  expanded: boolean;
  onToggle: () => void;
  onAI: () => void;
  onRemove: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("rounded-xl border border-border/75 bg-card/65 transition-colors", expanded && "xl:col-span-2")}>
      <div className="flex min-h-36 flex-col gap-3 p-4">
        <button type="button" className="flex min-w-0 flex-1 items-start gap-3 text-left" onClick={onToggle} aria-expanded={expanded}>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="truncate font-medium">{title}</span>
              {badges.map((badge) => <Badge key={badge}>{badge}</Badge>)}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">{id || "未设置 ID"}</div>
            <div className="mt-3 line-clamp-2 text-sm leading-6 text-muted-foreground">{description || "暂无描述"}</div>
          </div>
          <ChevronDown className={cn("mt-1 h-4 w-4 shrink-0 text-muted-foreground transition-transform", expanded && "rotate-180")} />
        </button>
        <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground sm:grid-cols-4">
          {metrics.map((metric) => <AgentSummaryMetric key={metric.label} label={metric.label} value={metric.value} />)}
        </div>
        <div className="flex items-center justify-between gap-2">
          <Button size="sm" variant={expanded ? "secondary" : "outline"} onClick={onToggle}>
            {expanded ? "收起" : "编辑"}
          </Button>
          <div className="flex items-center gap-1">
            <Button size="sm" variant="ghost" onClick={onAI}>
              <Sparkles className="h-4 w-4" />
              AI 修改
            </Button>
            <Button size="icon" variant="ghost" onClick={onRemove} aria-label="删除">
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
      {expanded ? <div className="space-y-3 border-t border-border/70 p-4">{children}</div> : null}
    </div>
  );
}

export function ConfigCard({
  title,
  aside,
  children,
  onRemove,
}: {
  title: string;
  aside?: string;
  children: React.ReactNode;
  onRemove?: () => void;
}) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-medium">{title}</div>
          {aside ? <div className="mt-0.5 text-xs text-muted-foreground">{aside}</div> : null}
        </div>
        {onRemove ? (
          <Button size="icon" variant="ghost" onClick={onRemove} aria-label="删除">
            <Trash2 className="h-4 w-4" />
          </Button>
        ) : null}
      </div>
      <div className="space-y-3">{children}</div>
    </div>
  );
}

export function SectionTitle({ icon: Icon, title, description }: { icon: typeof Bot; title: string; description: string }) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border bg-card/75">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="font-semibold">{title}</div>
        <div className="mt-1 text-sm leading-6 text-muted-foreground">{description}</div>
      </div>
    </div>
  );
}

export function SectionHeader({
  icon,
  title,
  description,
  children,
}: {
  icon: typeof Bot;
  title: string;
  description: string;
  children?: React.ReactNode;
}) {
  return (
    <PanelHeader className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
      <SectionTitle icon={icon} title={title} description={description} />
      {children ? <div className="flex shrink-0 sm:justify-end">{children}</div> : null}
    </PanelHeader>
  );
}

export function Field({ label, action, children }: { label: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="block space-y-1.5">
      <span className="flex min-h-8 items-center justify-between gap-3">
        <span className="text-sm text-muted-foreground">{label}</span>
        {action ? <span className="shrink-0">{action}</span> : null}
      </span>
      {children}
    </div>
  );
}

export function TextField({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
  disabled,
}: {
  label: string;
  value?: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: "text" | "password";
  disabled?: boolean;
}) {
  return (
    <Field label={label}>
      <Input type={type} value={value || ""} placeholder={placeholder} disabled={disabled} onChange={(event) => onChange(event.target.value)} />
    </Field>
  );
}

export function ModelAPIKeyField({ model, onChange }: { model: ModelConfig; onChange: (value: string) => void }) {
  const [editing, setEditing] = useState(false);
  const configured = Boolean(model.has_api_key);
  const hasDraftValue = Boolean(model.api_key);
  const showEditor = !configured || editing || hasDraftValue;

  useEffect(() => {
    if (configured && !hasDraftValue) {
      setEditing(false);
    }
  }, [configured, hasDraftValue]);

  if (!showEditor) {
    return (
      <Field
        label="API Key"
        action={
          <Button variant="ghost" size="sm" onClick={() => setEditing(true)}>
            修改
          </Button>
        }
      >
        <div className="sketch-inset flex h-9 w-full items-center rounded-md px-3 py-1 text-sm text-muted-foreground">已配置，留空不修改</div>
      </Field>
    );
  }

  return (
    <Field
      label={configured ? "API Key（输入新值后保存）" : "API Key"}
      action={
        configured ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              onChange("");
              setEditing(false);
            }}
          >
            取消
          </Button>
        ) : null
      }
    >
      <Input
        type="password"
        value={model.api_key || ""}
        placeholder={configured ? "输入新的 API Key" : undefined}
        autoComplete="new-password"
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

export function NumberField({
  label,
  value,
  min,
  max,
  step = 1,
  onChange,
}: {
  label: string;
  value?: number;
  min?: number;
  max?: number;
  step?: number;
  onChange: (value: number) => void;
}) {
  function parseValue(raw: string) {
    let next = Number(raw || 0);
    if (!Number.isFinite(next)) {
      next = 0;
    }
    if (min !== undefined) {
      next = Math.max(min, next);
    }
    if (max !== undefined) {
      next = Math.min(max, next);
    }
    return next;
  }

  return (
    <Field label={label}>
      <Input type="number" min={min} max={max} step={step} value={value ?? 0} onChange={(event) => onChange(parseValue(event.target.value))} />
    </Field>
  );
}

export function SelectField({
  label,
  value,
  options,
  optionLabels,
  onChange,
  disabled,
}: {
  label: string;
  value?: string;
  options: string[];
  optionLabels?: Record<string, string>;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <Field label={label}>
      <Select aria-label={label} value={value} options={options} optionLabels={optionLabels} onChange={onChange} disabled={disabled} />
    </Field>
  );
}

export function ModelUseField({ uses, onChange }: { uses: string[]; onChange: (uses: string[]) => void }) {
  const selected = new Set(uses);
  function toggle(value: string) {
    if (selected.has(value)) {
      onChange(uses.filter((item) => item !== value));
      return;
    }
    onChange([...uses, value]);
  }
  return (
    <div className="space-y-1.5 md:col-span-2">
      <div className="text-sm text-muted-foreground">用途</div>
      <div className="flex flex-wrap gap-2">
        {modelUseOptions.map((option) => (
          <button
            key={option.value}
            type="button"
            className={cn(
              "inline-flex h-8 items-center rounded-md border px-3 text-sm transition-colors",
              selected.has(option.value) ? "border-primary/60 bg-primary/10 text-primary" : "border-border/75 bg-card/70 text-muted-foreground hover:bg-accent/60",
            )}
            onClick={() => toggle(option.value)}
          >
            {option.label}
          </button>
        ))}
      </div>
    </div>
  );
}

export function ModelSelect({
  label,
  value,
  modelIDs,
  onChange,
  disabled,
}: {
  label: string;
  value?: string;
  modelIDs: string[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return <SelectField label={label} value={value} options={modelIDs.length ? modelIDs : [""]} disabled={disabled} onChange={onChange} />;
}

export function ModeField({ value, onChange }: { value?: string; onChange: (value: string) => void }) {
  return <SelectField label="运行模式" value={value} options={["team", "deep", "roundtable", "agent"]} onChange={onChange} />;
}

export function ToggleField({ label, checked, onChange, disabled }: { label: string; checked: boolean; onChange: (value: boolean) => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      className={cn(
        "flex w-full items-center justify-between gap-3 rounded-lg border border-border/75 bg-card/60 px-3 py-2 text-left transition-colors hover:bg-accent/60",
        disabled && "cursor-not-allowed opacity-70 hover:bg-card/60",
      )}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <span className="text-sm font-medium">{label}</span>
      <span className={cn("relative h-6 w-11 rounded-full border transition-colors", checked ? "border-primary/70 bg-primary" : "border-border bg-muted")}>
        <span className={cn("absolute top-0.5 h-5 w-5 rounded-full bg-card shadow transition-transform", checked ? "translate-x-5" : "translate-x-0.5")} />
      </span>
    </button>
  );
}

export function StringListField({
  label,
  values,
  onChange,
  placeholder,
  secret,
}: {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  secret?: boolean;
}) {
  const rows = values.length ? values : [""];
  return (
    <div className="space-y-2">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="space-y-2">
        {rows.map((value, index) => (
          <div key={index} className="flex gap-2">
            <Input
              type={secret ? "password" : "text"}
              value={value}
              placeholder={placeholder}
              onChange={(event) => {
                const next = [...rows];
                next[index] = event.target.value;
                onChange(next.filter((item, itemIndex) => item.trim() || itemIndex !== rows.length - 1));
              }}
            />
            <Button
              size="icon"
              variant="ghost"
              aria-label="删除"
              onClick={() => {
                const next = rows.filter((_, itemIndex) => itemIndex !== index);
                onChange(next.length ? next : []);
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>
      <Button variant="outline" size="sm" onClick={() => onChange([...values, ""])}>
        <Plus className="h-4 w-4" />
        添加
      </Button>
    </div>
  );
}

export function KeyValueMapField({
  label,
  values,
  onChange,
}: {
  label: string;
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
}) {
  const [rows, setRows] = useState<Array<[string, string]>>(() => {
    const entries = Object.entries(values);
    return entries.length ? entries : [["", ""]];
  });
  useEffect(() => {
    const entries = Object.entries(values);
    setRows(entries.length ? entries : [["", ""]]);
  }, [JSON.stringify(values)]);

  function commit(nextRows: Array<[string, string]>) {
    setRows(nextRows.length ? nextRows : [["", ""]]);
    const next: Record<string, string> = {};
    for (const [key, value] of nextRows) {
      const trimmed = key.trim();
      if (!trimmed) continue;
      next[trimmed] = value;
    }
    onChange(next);
  }
  return (
    <div className="space-y-2">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="space-y-2">
        {rows.map(([key, value], index) => (
          <div key={index} className="grid gap-2 md:grid-cols-[1fr_1fr_auto]">
            <Input
              value={key}
              placeholder="KEY"
              onChange={(event) => {
                const nextRows = [...rows];
                nextRows[index] = [event.target.value, value];
                commit(nextRows);
              }}
            />
            <Input
              value={value}
              placeholder="value"
              onChange={(event) => {
                const nextRows = [...rows];
                nextRows[index] = [key, event.target.value];
                commit(nextRows);
              }}
            />
            <Button
              size="icon"
              variant="ghost"
              aria-label="删除"
              onClick={() => {
                const nextRows = rows.filter((_, itemIndex) => itemIndex !== index);
                commit(nextRows);
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>
      <Button variant="outline" size="sm" onClick={() => setRows([...rows, ["", ""]])}>
        <Plus className="h-4 w-4" />
        添加
      </Button>
    </div>
  );
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-xl border border-dashed border-border p-8 text-center xl:col-span-2">
      <div className="font-medium">{title}</div>
      <div className="mt-1 text-sm text-muted-foreground">{description}</div>
    </div>
  );
}

export function MetricCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="mt-2 text-3xl font-semibold">{value}</div>
    </div>
  );
}

export function AgentSummaryMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg border border-border/60 bg-background/45 px-2 py-1.5">
      <div>{label}</div>
      <div className="mt-1 truncate text-foreground">{value}</div>
    </div>
  );
}
