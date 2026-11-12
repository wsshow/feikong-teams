import { useEffect, useId, useState } from "react";
import { get, put, del } from "@/api/client";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ConfirmDialog } from "@/components/ui/action-dialog";

const categories = { preference: "用户偏好", fact: "个人信息", feedback: "行为反馈", lesson: "避坑记录", decision: "已确定方案", insight: "认知洞察", experience: "操作经验" };
const categoryOptions = Object.keys(categories);
const filterOptions = ["", ...categoryOptions];
const filterLabels = { ...categories, "": "全部类型" };
type MemoryType = keyof typeof categories;
type Entry = { id: string; version?: number; scope?: string; type: MemoryType; summary: string; detail: string; tags: string[]; created_at?: string; hit_count?: number };
const fresh = (): Entry => ({ id: "", type: "fact", summary: "", detail: "", tags: [] });

export function MemoryPanel() {
  const typeID = useId();
  const [entries, setEntries] = useState<Entry[]>([]);
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("");
  const [draft, setDraft] = useState<Entry | null>(null);
  const [tags, setTags] = useState("");
  const [target, setTarget] = useState<Entry | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reload, setReload] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    get<Entry[]>("/api/fkteams/memory", { signal: controller.signal })
      .then((items) => { setEntries(items); setError(""); })
      .catch((e: unknown) => { if (!controller.signal.aborted) setError(String(e)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [reload]);
  function edit(entry: Entry) { setDraft({ ...entry, tags: [...entry.tags || []] }); setTags((entry.tags || []).join(", ")); setNotice(""); setError(""); }
  async function save() {
    if (!draft || busy) return;
    setBusy(true); setError("");
    try {
      const saved = await put<Entry>("/api/fkteams/memory", { ...draft, tags: tags.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean) });
      setEntries((old) => [...old.filter((entry) => entry.id !== saved.id), saved]);
      setDraft(null); setNotice("记忆已保存");
    } catch (e) { setError(String(e)); } finally { setBusy(false); }
  }
  async function remove() {
    if (!target || busy) return;
    setBusy(true); setError("");
    try {
      await del(`/api/fkteams/memory/${encodeURIComponent(target.id)}`);
      setEntries((old) => old.filter((entry) => entry.id !== target.id));
      if (draft?.id === target.id) setDraft(null);
      setTarget(null); setNotice("记忆已删除");
    } catch (e) { setError(String(e)); setTarget(null); } finally { setBusy(false); }
  }
  const visible = entries.filter((entry) => (!category || entry.type === category) && `${entry.scope || ""} ${entry.summary} ${entry.detail} ${(entry.tags || []).join(" ")}`.toLowerCase().includes(query.toLowerCase()));
  return <div className="h-full overflow-auto p-4 sm:p-6">
    <div className="mx-auto max-w-5xl space-y-5">
      <div className="flex items-center justify-between gap-3"><div><h1 className="text-xl font-semibold">长期记忆</h1><p className="mt-1 text-sm text-muted-foreground">管理助手在不同会话中记住的信息。共 {entries.length} 条。</p></div><Button className="shrink-0 whitespace-nowrap" disabled={busy || !!draft} onClick={() => edit(fresh())}>新增记忆</Button></div>
      {error && !draft && <div role="alert" className="rounded-md border border-destructive/30 p-3 text-sm text-destructive">{error}<Button variant="ghost" disabled={busy} onClick={() => setReload((value) => value + 1)}>重新加载</Button></div>}
      {notice && <p role="status" className="text-sm text-muted-foreground">{notice}</p>}
      {draft && <Dialog open title={draft.id ? "编辑记忆" : "新增记忆"} closeDisabled={busy} onOpenChange={() => setDraft(null)}><form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void save(); }}>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <div className="space-y-1 text-sm"><label htmlFor={typeID}>类型</label><Select id={typeID} aria-label="类型" value={draft.type} options={categoryOptions} optionLabels={categories} disabled={busy} onChange={(type) => setDraft({ ...draft, type: type as MemoryType })} /></div>
        <label className="block space-y-1 text-sm"><span>适用范围</span><Input aria-label="适用范围" maxLength={256} placeholder="留空适用于所有对话；也可填写项目或场景" value={draft.scope || ""} disabled={busy} onChange={(e) => setDraft({ ...draft, scope: e.target.value })} /></label>
        <label className="block space-y-1 text-sm"><span>摘要</span><Input aria-label="摘要" required maxLength={1000} value={draft.summary} disabled={busy} onChange={(e) => setDraft({ ...draft, summary: e.target.value })} /></label>
        <label className="block space-y-1 text-sm"><span>详情</span><Textarea rows={5} maxLength={16000} value={draft.detail} disabled={busy} onChange={(e) => setDraft({ ...draft, detail: e.target.value })} /></label>
        <label className="block space-y-1 text-sm"><span>标签，用逗号分隔</span><Input value={tags} disabled={busy} onChange={(e) => setTags(e.target.value)} /></label>
        <div className="flex justify-end gap-2"><Button type="button" variant="outline" disabled={busy} onClick={() => setDraft(null)}>取消</Button><Button disabled={busy || !draft.summary.trim()}>{busy ? "正在保存" : "保存记忆"}</Button></div>
      </form></Dialog>}
      <div className="flex flex-col gap-3 sm:flex-row"><Input className="min-w-0 flex-1" aria-label="搜索记忆" placeholder="搜索摘要、详情或标签" value={query} onChange={(e) => setQuery(e.target.value)} /><Select className="w-full shrink-0 sm:w-44" aria-label="筛选记忆类型" value={category} options={filterOptions} optionLabels={filterLabels} onChange={setCategory} /></div>
      {loading ? <p role="status" className="py-8 text-center text-muted-foreground">正在加载记忆</p> : !visible.length ? <p className="py-8 text-center text-muted-foreground">{entries.length ? "没有匹配的记忆" : "暂无记忆。可以直接告诉助手你的偏好，也可以在这里添加。"}</p> : <ul className="divide-y rounded-lg border bg-card shadow-surface">{visible.map((entry) => <li key={entry.id} className="space-y-2 p-4"><div className="flex items-start justify-between gap-3"><div className="min-w-0"><p className="text-xs text-muted-foreground">{categories[entry.type]} · {entry.scope || "全局"}</p><h2 className="mt-1 break-words font-semibold">{entry.summary}</h2></div><div className="flex shrink-0 gap-1"><Button size="sm" variant="ghost" disabled={busy || !!draft} onClick={() => edit(entry)}>编辑</Button><Button size="sm" variant="ghost" disabled={busy} onClick={() => setTarget(entry)}>删除</Button></div></div><p className="line-clamp-3 whitespace-pre-wrap break-words text-sm text-muted-foreground">{entry.detail}</p>{!!entry.tags?.length && <p className="text-xs text-muted-foreground">{entry.tags.join(" · ")}</p>}</li>)}</ul>}
      <ConfirmDialog open={!!target} title="删除这条记忆？" description={target?.summary} confirmLabel="删除记忆" destructive busy={busy} onCancel={() => setTarget(null)} onConfirm={() => void remove()} />
    </div>
  </div>;
}
