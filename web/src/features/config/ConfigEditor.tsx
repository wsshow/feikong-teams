import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog } from "@/components/ui/dialog";
import type { AppConfig } from "@/types/config";
import { normalizeConfig } from "./configModel";

type Mutator = (next: AppConfig) => void;
type Removal = { title: string; mutate: Mutator };
interface EditingContext {
  activeKey: string | null;
  busy: boolean;
  error: string;
  begin: (key: string, mutate?: Mutator) => void;
  cancel: () => void;
  confirm: () => Promise<void>;
  apply: (mutate: Mutator) => Promise<void>;
  remove: (title: string, mutate: Mutator) => void;
}
const Context = createContext<EditingContext | null>(null);

export function useConfigEditing() {
  const context = useContext(Context);
  if (!context) throw new Error("configuration editor requires a provider");
  return context;
}

export function ConfigEditingProvider({
  draft,
  onChange,
  onSave,
  onEditingChange,
  disabled = false,
  children,
}: {
  draft: AppConfig;
  onChange: (next: AppConfig) => void;
  onSave: (next: AppConfig) => Promise<void>;
  disabled?: boolean;
  onEditingChange?: (editing: boolean) => void;
  children: ReactNode;
}) {
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [removal, setRemoval] = useState<Removal | null>(null);
  const snapshot = useRef<AppConfig | null>(null);
  const pending = useRef(false);

  useEffect(() => {
    onEditingChange?.(activeKey !== null);
  }, [activeKey, onEditingChange]);

  async function persist(next: AppConfig) {
    if (pending.current) throw new Error("configuration save already in progress");
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      await onSave(next);
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  function begin(key: string, mutate?: Mutator) {
    if (disabled || pending.current || snapshot.current) return;
    snapshot.current = normalizeConfig(draft);
    setError("");
    setActiveKey(key);
    if (mutate) {
      const next = normalizeConfig(draft);
      mutate(next);
      onChange(next);
    }
  }

  function cancel() {
    if (pending.current) return;
    if (snapshot.current) onChange(snapshot.current);
    snapshot.current = null;
    setActiveKey(null);
    setError("");
  }

  async function confirm() {
    if (pending.current) return;
    try {
      await persist(draft);
      snapshot.current = null;
      setActiveKey(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function apply(mutate: Mutator) {
    const next = normalizeConfig(draft);
    mutate(next);
    await persist(next);
  }

  async function confirmRemove() {
    if (!removal || pending.current) return;
    try {
      await apply(removal.mutate);
      setRemoval(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Context.Provider
      value={{
        activeKey,
        busy: busy || disabled,
        error,
        begin,
        cancel,
        confirm,
        apply,
        remove: (title, mutate) => {
          if (disabled || pending.current || snapshot.current) return;
          setError("");
          setRemoval({ title, mutate });
        },
      }}
    >
      {children}
      <Dialog
        open={Boolean(removal)}
        title={`删除${removal?.title || "配置"}`}
        closeDisabled={busy}
        onOpenChange={() => {
          setRemoval(null);
          setError("");
        }}
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="outline" disabled={busy} onClick={() => setRemoval(null)}>
              取消
            </Button>
            <Button variant="destructive" disabled={busy} onClick={() => void confirmRemove()}>
              {busy ? "正在删除…" : "确认删除"}
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted-foreground">确认删除「{removal?.title}」？确认后将立即保存配置。</p>
        {error ? (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {error}
          </p>
        ) : null}
      </Dialog>
    </Context.Provider>
  );
}

export function ConfigEditDialog({ editorKey, title, children }: { editorKey: string; title: string; children: ReactNode }) {
  const editing = useConfigEditing();
  const fields = useRef<HTMLFieldSetElement>(null);
  return (
    <Dialog
      open={editing.activeKey === editorKey}
      title={title}
      closeDisabled={editing.busy}
      onOpenChange={editing.cancel}
      footer={
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">确认后立即保存，敏感字段留空保留当前值。</p>
          <div className="flex shrink-0 justify-end gap-2">
            <Button variant="outline" disabled={editing.busy} onClick={editing.cancel}>
              取消
            </Button>
            <Button
              disabled={editing.busy}
              onClick={() => {
                const controls = fields.current?.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("input, textarea") || [];
                for (const control of controls) if (!control.reportValidity()) return;
                void editing.confirm();
              }}
            >
              {editing.busy ? "正在保存…" : "确认并保存"}
            </Button>
          </div>
        </div>
      }
    >
      <fieldset ref={fields} disabled={editing.busy} className="min-w-0 space-y-4">
        {children}
      </fieldset>
      {editing.error ? (
        <p role="alert" className="mt-4 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
          {editing.error}
        </p>
      ) : null}
    </Dialog>
  );
}

export function ConfigEditCard({
  editorKey,
  title,
  description,
  badges = [],
  summary = [],
  onRemove,
  actions,
  children,
}: {
  editorKey: string;
  title: string;
  description?: string;
  badges?: string[];
  summary?: Array<{ label: string; value: string }>;
  onRemove?: () => void;
  actions?: ReactNode;
  children: ReactNode;
}) {
  const editing = useConfigEditing();
  return (
    <div className="relative min-w-0 rounded-lg border border-border/75 bg-card shadow-surface transition-colors hover:bg-accent/40 hover:shadow-raised focus-within:shadow-raised">
      <button
        type="button"
        className="absolute inset-0 w-full rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none"
        disabled={editing.busy}
        aria-haspopup="dialog"
        aria-label={`编辑${title}`}
        onClick={() => editing.begin(editorKey)}
      />
      <div className="pointer-events-none relative p-4">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="min-w-0 truncate font-semibold">{title}</span>
            {badges.map((badge) => (
              <Badge key={badge}>{badge}</Badge>
            ))}
          </div>
          {actions || onRemove ? (
            <fieldset disabled={editing.busy} className="pointer-events-auto relative z-10 flex min-w-0 shrink-0 items-center gap-1">
              {actions}
              {onRemove ? (
                <Button size="icon" variant="ghost" onClick={onRemove} aria-label={`删除${title}`}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              ) : null}
            </fieldset>
          ) : null}
        </div>
        {description ? <p className="mt-1 line-clamp-2 break-words text-sm text-muted-foreground">{description}</p> : null}
        {summary.length ? (
          <dl className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            {summary.map(({ label, value }) => (
              <div key={label} className="flex min-w-0 max-w-full gap-1">
                <dt className="shrink-0">{label}</dt>
                <dd className="truncate text-foreground" title={value}>
                  {value}
                </dd>
              </div>
            ))}
          </dl>
        ) : null}
      </div>
      <ConfigEditDialog editorKey={editorKey} title={`编辑${title}`}>
        {children}
      </ConfigEditDialog>
    </div>
  );
}
