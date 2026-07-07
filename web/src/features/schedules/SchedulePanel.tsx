import {
  Ban,
  CalendarClock,
  CheckCircle2,
  Clock3,
  Pencil,
  FileText,
  History,
  PlayCircle,
  Plus,
  RefreshCcw,
  Save,
  Search,
  Trash2,
  XCircle,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { cancelSchedule, createSchedule, deleteSchedule, getScheduleHistory, getScheduleResult, listSchedules, updateSchedule } from "@/api/schedules";
import { appActions, schedulesActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/action-dialog";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { cn } from "@/lib/cn";
import { formatTime, shortID } from "@/lib/format";
import type { ScheduleHistoryEntry, ScheduleTask, ScheduleTaskPayload } from "@/types/schedules";

type ScheduleFilter = "all" | "active" | "completed" | "cancelled" | "failed";
type ScheduleFormMode = "once" | "cron";

interface ScheduleFormState {
  id?: string;
  task: string;
  mode: ScheduleFormMode;
  cronExpr: string;
  executeAt: string;
}

export function SchedulePanel() {
  const dispatch = useAppDispatch();
  const tasks = useAppSelector((state) => state.schedules.items);
  const [selectedID, setSelectedID] = useState("");
  const [resultContent, setResultContent] = useState("");
  const [historyEntries, setHistoryEntries] = useState<ScheduleHistoryEntry[]>([]);
  const [filter, setFilter] = useState<ScheduleFilter>("all");
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [busyID, setBusyID] = useState("");
  const [form, setForm] = useState<ScheduleFormState | undefined>();
  const [saving, setSaving] = useState(false);
  const [cancelTarget, setCancelTarget] = useState<ScheduleTask | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ScheduleTask | null>(null);
  const selectedTask = tasks.find((task) => task.id === selectedID);
  const counts = useMemo(() => countTasks(tasks), [tasks]);
  const filteredTasks = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    return tasks
      .filter((task) => filter === "all" || normalizeStatus(task.status) === filter)
      .filter((task) => {
        if (!query) return true;
        return `${task.id} ${task.task} ${task.status} ${task.cron_expr || ""}`.toLowerCase().includes(query);
      })
      .sort((left, right) => taskTime(right) - taskTime(left));
  }, [tasks, filter, keyword]);

  async function load() {
    setLoading(true);
    try {
      const result = await listSchedules();
      dispatch(schedulesActions.setSchedules(result.tasks || []));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setLoading(false);
    }
  }

  async function showDetail(task: ScheduleTask) {
    setSelectedID(task.id);
    setResultContent("");
    setHistoryEntries([]);
    setDetailLoading(true);
    try {
      const [result, history] = await Promise.all([
        getScheduleResult(task.id).catch(() => undefined),
        getScheduleHistory(task.id).catch(() => undefined),
      ]);
      setResultContent(result?.result || result?.content || "");
      setHistoryEntries(history?.entries || history?.history || []);
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setDetailLoading(false);
    }
  }

  async function cancel(id: string) {
    const task = tasks.find((item) => item.id === id);
    setCancelTarget(task || { id, task: "", status: "" });
  }

  async function confirmCancel() {
    if (!cancelTarget) return;
    const id = cancelTarget.id;
    setBusyID(id);
    try {
      await cancelSchedule(id);
      setCancelTarget(null);
      dispatch(appActions.showToast("任务已取消"));
      await load();
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusyID("");
    }
  }

  async function saveTask() {
    if (!form) return;
    const payload = formToPayload(form);
    setSaving(true);
    try {
      if (form.id) {
        const result = await updateSchedule(form.id, payload);
        dispatch(appActions.showToast("任务已更新"));
        setSelectedID(result.task.id);
      } else {
        const result = await createSchedule(payload);
        dispatch(appActions.showToast("任务已创建"));
        setSelectedID(result.task.id);
      }
      setForm(undefined);
      await load();
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setSaving(false);
    }
  }

  async function remove(id: string) {
    const task = tasks.find((item) => item.id === id);
    setDeleteTarget(task || { id, task: "", status: "" });
  }

  async function confirmRemove() {
    if (!deleteTarget) return;
    const id = deleteTarget.id;
    setBusyID(id);
    try {
      await deleteSchedule(id);
      if (selectedID === id) {
        setSelectedID("");
        setResultContent("");
        setHistoryEntries([]);
      }
      setDeleteTarget(null);
      dispatch(appActions.showToast("任务已删除"));
      await load();
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusyID("");
    }
  }

  function startCreate() {
    setForm({
      task: "",
      mode: "once",
      cronExpr: "0 9 * * *",
      executeAt: toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000).toISOString()),
    });
  }

  function startEdit(task: ScheduleTask) {
    setForm({
      id: task.id,
      task: task.task || "",
      mode: task.cron_expr ? "cron" : "once",
      cronExpr: task.cron_expr || "0 9 * * *",
      executeAt: toLocalDateTimeInput(task.next_run_at || new Date(Date.now() + 60 * 60 * 1000).toISOString()),
    });
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="chat-scroll h-full overflow-auto p-3 sm:p-6">
      <div className="mx-auto flex max-w-7xl flex-col gap-4">
        <Panel>
          <PanelHeader className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
            <div className="min-w-0">
              <div className="flex items-center gap-3">
                <CalendarClock className="h-5 w-5 text-primary" />
                <h2 className="text-xl font-semibold">任务</h2>
              </div>
              <div className="mt-1 text-sm text-muted-foreground">查看计划任务状态、执行结果和历史记录。</div>
            </div>
            <div className="grid w-full min-w-0 grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_auto_auto] xl:w-[640px]">
              <Input className="min-w-0" value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="搜索任务内容、ID 或状态" />
              <Button className="min-w-24 justify-center whitespace-nowrap" onClick={startCreate}>
                <Plus className="h-4 w-4" />
                新建任务
              </Button>
              <Button className="min-w-20 justify-center whitespace-nowrap" variant="outline" onClick={() => void load()} disabled={loading}>
                <RefreshCcw className="h-4 w-4" />
                刷新
              </Button>
            </div>
          </PanelHeader>
          <PanelBody className="grid gap-3 border-t border-border/70 md:grid-cols-4">
            <MetricCard icon={CalendarClock} label="全部任务" value={tasks.length} />
            <MetricCard icon={PlayCircle} label="活跃" value={counts.active} />
            <MetricCard icon={CheckCircle2} label="完成" value={counts.completed} />
            <MetricCard icon={XCircle} label="取消/失败" value={counts.cancelled + counts.failed} />
          </PanelBody>
        </Panel>

        {form ? <ScheduleEditor form={form} saving={saving} onChange={setForm} onCancel={() => setForm(undefined)} onSave={() => void saveTask()} /> : null}

        <Panel>
          <PanelHeader className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="chat-scroll flex gap-2 overflow-x-auto">
              <FilterButton active={filter === "all"} label="全部" count={tasks.length} onClick={() => setFilter("all")} />
              <FilterButton active={filter === "active"} label="活跃" count={counts.active} onClick={() => setFilter("active")} />
              <FilterButton active={filter === "completed"} label="完成" count={counts.completed} onClick={() => setFilter("completed")} />
              <FilterButton active={filter === "cancelled"} label="取消" count={counts.cancelled} onClick={() => setFilter("cancelled")} />
              <FilterButton active={filter === "failed"} label="失败" count={counts.failed} onClick={() => setFilter("failed")} />
            </div>
            <div className="text-sm text-muted-foreground">{filteredTasks.length} 个任务符合当前筛选</div>
          </PanelHeader>
          <PanelBody>
            <TaskGrid
              tasks={filteredTasks}
              selectedID={selectedID}
              busyID={busyID}
              loading={loading}
              onSelect={showDetail}
              onEdit={startEdit}
              onCancel={cancel}
              onDelete={remove}
            />
          </PanelBody>
        </Panel>

        <TaskDetail
          task={selectedTask}
          loading={detailLoading}
          busy={Boolean(selectedTask && busyID === selectedTask.id)}
          result={resultContent}
          history={historyEntries}
          onEdit={startEdit}
          onCancel={(id) => void cancel(id)}
          onDelete={(id) => void remove(id)}
        />
        <ConfirmDialog
          open={Boolean(cancelTarget)}
          title="取消任务"
          description={
            <>
              任务「<span className="font-medium text-foreground">{cancelTarget?.task || shortID(cancelTarget?.id || "")}</span>」将停止后续执行。
            </>
          }
          confirmLabel="确认取消"
          busy={Boolean(cancelTarget && busyID === cancelTarget.id)}
          onCancel={() => {
            if (!busyID) setCancelTarget(null);
          }}
          onConfirm={() => void confirmCancel()}
        />
        <ConfirmDialog
          open={Boolean(deleteTarget)}
          title="删除任务"
          description={
            <>
              任务「<span className="font-medium text-foreground">{deleteTarget?.task || shortID(deleteTarget?.id || "")}</span>」及其历史结果将被删除，无法恢复。
            </>
          }
          confirmLabel="确认删除"
          destructive
          busy={Boolean(deleteTarget && busyID === deleteTarget.id)}
          onCancel={() => {
            if (!busyID) setDeleteTarget(null);
          }}
          onConfirm={() => void confirmRemove()}
        />
      </div>
    </div>
  );
}

function ScheduleEditor({
  form,
  saving,
  onChange,
  onCancel,
  onSave,
}: {
  form: ScheduleFormState;
  saving: boolean;
  onChange: (form: ScheduleFormState) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  const canSave = Boolean(form.task.trim() && (form.mode === "cron" ? form.cronExpr.trim() : form.executeAt));
  return (
    <Panel>
      <PanelHeader className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
        <div>
          <div className="font-semibold">{form.id ? "编辑任务" : "新建任务"}</div>
          <div className="mt-1 text-sm text-muted-foreground">一次性任务使用本地时间保存，循环任务使用 cron 表达式。</div>
        </div>
        <div className="flex gap-2">
          <Button className="min-w-20 whitespace-nowrap" variant="outline" onClick={onCancel}>
            取消
          </Button>
          <Button className="min-w-20 whitespace-nowrap" disabled={!canSave || saving} onClick={onSave}>
            <Save className="h-4 w-4" />
            保存
          </Button>
        </div>
      </PanelHeader>
      <PanelBody className="grid gap-4 border-t border-border/70 xl:grid-cols-[1fr_360px]">
        <label className="block space-y-1.5">
          <span className="text-sm text-muted-foreground">任务内容</span>
          <Textarea
            className="min-h-32 text-sm"
            value={form.task}
            onChange={(event) => onChange({ ...form, task: event.target.value })}
            placeholder="例如：每天早上汇总昨日项目进展并生成报告"
          />
        </label>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-2">
            <button
              className={cn(
                "h-10 rounded-lg border text-sm transition-colors",
                form.mode === "once" ? "border-primary/50 bg-primary/10 text-primary" : "border-border bg-card/70 hover:bg-accent/60",
              )}
              onClick={() => onChange({ ...form, mode: "once" })}
              type="button"
            >
              一次执行
            </button>
            <button
              className={cn(
                "h-10 rounded-lg border text-sm transition-colors",
                form.mode === "cron" ? "border-primary/50 bg-primary/10 text-primary" : "border-border bg-card/70 hover:bg-accent/60",
              )}
              onClick={() => onChange({ ...form, mode: "cron" })}
              type="button"
            >
              循环执行
            </button>
          </div>
          {form.mode === "once" ? (
            <label className="block space-y-1.5">
              <span className="text-sm text-muted-foreground">执行时间</span>
              <Input type="datetime-local" value={form.executeAt} onChange={(event) => onChange({ ...form, executeAt: event.target.value })} />
            </label>
          ) : (
            <label className="block space-y-1.5">
              <span className="text-sm text-muted-foreground">Cron 表达式</span>
              <Input value={form.cronExpr} onChange={(event) => onChange({ ...form, cronExpr: event.target.value })} placeholder="0 9 * * *" />
            </label>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}

function TaskGrid({
  tasks,
  selectedID,
  busyID,
  loading,
  onSelect,
  onEdit,
  onCancel,
  onDelete,
}: {
  tasks: ScheduleTask[];
  selectedID: string;
  busyID: string;
  loading: boolean;
  onSelect: (task: ScheduleTask) => Promise<void>;
  onEdit: (task: ScheduleTask) => void;
  onCancel: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}) {
  if (!tasks.length) {
    return <EmptyState title={loading ? "正在加载任务" : "暂无任务"} description="通过调度工具创建任务后会显示在这里。" />;
  }

  return (
    <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
      {tasks.map((task) => {
        const selected = selectedID === task.id;
        const cancellable = isCancellable(task.status);
        return (
          <article
            key={task.id}
            className={cn(
              "group flex min-h-48 cursor-pointer flex-col rounded-xl border bg-card/65 p-4 text-left transition-[background,border-color,box-shadow]",
              selected ? "border-primary/60 bg-primary/5 shadow-[2px_3px_0_hsl(214_45%_30%/0.12)]" : "border-border/75 hover:bg-accent/45",
            )}
            onClick={() => void onSelect(task)}
            role="button"
            tabIndex={0}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") void onSelect(task);
            }}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <StatusDot status={task.status} />
                  <span className="truncate font-semibold">{shortID(task.id)}</span>
                </div>
                <div className="mt-1 text-xs text-muted-foreground">{task.cron_expr || "once"}</div>
              </div>
              <Badge>{statusLabel(task.status)}</Badge>
            </div>
            <div className="mt-3 line-clamp-3 flex-1 text-sm leading-6 text-muted-foreground">{task.task || "未命名任务"}</div>
            <div className="mt-4 grid gap-1 text-xs text-muted-foreground">
              <TimeRow label="下次" value={task.next_run_at} />
              <TimeRow label="上次" value={task.last_run_at} />
            </div>
            <div className="mt-4 flex items-center justify-between gap-3">
              <span className="text-xs text-muted-foreground">创建 {formatTime(task.created_at)}</span>
              <div className="flex shrink-0 gap-1">
                <Button
                  className="whitespace-nowrap"
                  size="sm"
                  variant="ghost"
                  onClick={(event) => {
                    event.stopPropagation();
                    onEdit(task);
                  }}
                >
                  <Pencil className="h-4 w-4" />
                  编辑
                </Button>
                <Button
                  className="whitespace-nowrap"
                  size="sm"
                  variant="ghost"
                  disabled={busyID === task.id}
                  onClick={(event) => {
                    event.stopPropagation();
                    void onDelete(task.id);
                  }}
                >
                  <Trash2 className="h-4 w-4" />
                  删除
                </Button>
                {cancellable ? (
                  <Button
                    className="whitespace-nowrap"
                    size="sm"
                    variant="ghost"
                    disabled={busyID === task.id}
                    onClick={(event) => {
                      event.stopPropagation();
                      void onCancel(task.id);
                    }}
                  >
                    <Ban className="h-4 w-4" />
                    取消
                  </Button>
                ) : null}
              </div>
            </div>
          </article>
        );
      })}
    </div>
  );
}

function TaskDetail({
  task,
  loading,
  busy,
  result,
  history,
  onEdit,
  onCancel,
  onDelete,
}: {
  task?: ScheduleTask;
  loading: boolean;
  busy: boolean;
  result: string;
  history: ScheduleHistoryEntry[];
  onEdit: (task: ScheduleTask) => void;
  onCancel: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  if (!task) {
    return (
      <Panel>
        <PanelBody>
          <EmptyState title="选择一个任务" description="任务结果和执行历史会在这里展示。" />
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel>
      <PanelHeader className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold">任务 {shortID(task.id)}</h3>
            <Badge>{statusLabel(task.status)}</Badge>
            <Badge>{task.cron_expr || "once"}</Badge>
          </div>
          <div className="mt-2 max-w-4xl text-sm leading-6 text-muted-foreground">{task.task || "未命名任务"}</div>
          <div className="mt-3 flex flex-wrap gap-3 text-xs text-muted-foreground">
            <span>ID {task.id}</span>
            <span>创建 {formatTime(task.created_at)}</span>
            {task.next_run_at ? <span>下次 {formatTime(task.next_run_at)}</span> : null}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <Button className="min-w-20 whitespace-nowrap" variant="outline" onClick={() => onEdit(task)}>
            <Pencil className="h-4 w-4" />
            编辑
          </Button>
          <Button className="min-w-20 whitespace-nowrap" variant="outline" disabled={busy} onClick={() => onDelete(task.id)}>
            <Trash2 className="h-4 w-4" />
            删除
          </Button>
          {isCancellable(task.status) ? (
            <Button className="min-w-20 whitespace-nowrap" variant="destructive" disabled={busy} onClick={() => onCancel(task.id)}>
              <Ban className="h-4 w-4" />
              取消
            </Button>
          ) : null}
        </div>
      </PanelHeader>
      <PanelBody className="space-y-4 border-t border-border/70">
        <div className="grid gap-4 xl:grid-cols-[1fr_420px]">
          <div className="rounded-xl border border-border/75 bg-card/65">
            <div className="flex h-11 items-center gap-2 border-b border-border/70 px-4">
              <FileText className="h-4 w-4 text-muted-foreground" />
              <div className="font-medium">执行结果</div>
            </div>
            <div className="chat-scroll max-h-[52vh] min-h-72 overflow-auto p-5">
              {loading ? (
                <div className="text-sm text-muted-foreground">正在加载结果</div>
              ) : (
                <MarkdownContent className="text-base leading-8" content={result || "暂无结果"} />
              )}
            </div>
          </div>
          <div className="rounded-xl border border-border/75 bg-card/65">
            <div className="flex h-11 items-center justify-between border-b border-border/70 px-4">
              <div className="flex items-center gap-2">
                <History className="h-4 w-4 text-muted-foreground" />
                <div className="font-medium">执行历史</div>
              </div>
              <Badge>{history.length}</Badge>
            </div>
            <div className="chat-scroll max-h-[52vh] min-h-72 overflow-auto p-3">
              {history.length ? (
                <div className="space-y-2">
                  {history.map((entry, index) => (
                    <HistoryEntryCard key={`${entry.filename || "history"}-${index}`} entry={entry} />
                  ))}
                </div>
              ) : (
                <div className="p-3 text-sm text-muted-foreground">{loading ? "正在加载历史" : "暂无历史记录"}</div>
              )}
            </div>
          </div>
        </div>
      </PanelBody>
    </Panel>
  );
}

function HistoryEntryCard({ entry }: { entry: ScheduleHistoryEntry }) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/45 p-3">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0 truncate text-sm font-medium">{entry.filename || "history"}</div>
        {entry.status ? <Badge>{statusLabel(entry.status)}</Badge> : null}
      </div>
      <div className="mt-1 text-xs text-muted-foreground">{formatTime(entry.created_at || entry.time)}</div>
      {entry.content ? <div className="mt-2 line-clamp-4 text-sm leading-6 text-muted-foreground">{entry.content}</div> : null}
    </div>
  );
}

function FilterButton({ active, label, count, onClick }: { active: boolean; label: string; count: number; onClick: () => void }) {
  return (
    <button
      className={cn(
        "inline-flex h-10 shrink-0 items-center gap-2 rounded-lg border px-3 text-sm transition-colors",
        active ? "border-primary/50 bg-primary/10 text-primary" : "border-transparent text-muted-foreground hover:border-border hover:bg-card",
      )}
      onClick={onClick}
    >
      {label}
      <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">{count}</span>
    </button>
  );
}

function MetricCard({ icon: Icon, label, value }: { icon: typeof CalendarClock; label: string; value: number }) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">{label}</div>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="mt-2 text-3xl font-semibold">{value}</div>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  return <span className={cn("h-2.5 w-2.5 shrink-0 rounded-full", statusColor(status))} />;
}

function TimeRow({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex items-center gap-2">
      <Clock3 className="h-3.5 w-3.5" />
      <span className="shrink-0">{label}</span>
      <span className="truncate">{formatTime(value)}</span>
    </div>
  );
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-xl border border-dashed border-border p-10 text-center">
      <div className="font-medium">{title}</div>
      <div className="mt-1 text-sm text-muted-foreground">{description}</div>
    </div>
  );
}

function countTasks(tasks: ScheduleTask[]) {
  return tasks.reduce(
    (counts, task) => {
      const status = normalizeStatus(task.status);
      counts[status] += 1;
      return counts;
    },
    { active: 0, completed: 0, cancelled: 0, failed: 0 },
  );
}

function normalizeStatus(status?: string): Exclude<ScheduleFilter, "all"> {
  const value = (status || "").toLowerCase();
  if (value.includes("cancel")) return "cancelled";
  if (value.includes("fail") || value.includes("error")) return "failed";
  if (value.includes("complete") || value.includes("done") || value.includes("success")) return "completed";
  return "active";
}

function statusLabel(status?: string) {
  const normalized = normalizeStatus(status);
  if (normalized === "active") return status || "active";
  return {
    completed: "completed",
    cancelled: "cancelled",
    failed: "failed",
  }[normalized];
}

function statusColor(status?: string) {
  const normalized = normalizeStatus(status);
  if (normalized === "completed") return "bg-emerald-500";
  if (normalized === "cancelled") return "bg-muted-foreground/45";
  if (normalized === "failed") return "bg-destructive";
  return "bg-primary";
}

function isCancellable(status?: string) {
  return normalizeStatus(status) === "active";
}

function taskTime(task: ScheduleTask) {
  return Date.parse(task.next_run_at || task.last_run_at || task.created_at || "") || 0;
}

function formToPayload(form: ScheduleFormState): ScheduleTaskPayload {
  const payload: ScheduleTaskPayload = { task: form.task.trim() };
  if (form.mode === "cron") {
    payload.cron_expr = form.cronExpr.trim();
  } else {
    payload.execute_at = localDateTimeToISO(form.executeAt);
  }
  return payload;
}

function toLocalDateTimeInput(value?: string) {
  const date = value ? new Date(value) : new Date();
  if (Number.isNaN(date.getTime())) return "";
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function localDateTimeToISO(value: string) {
  return value ? new Date(value).toISOString() : "";
}
