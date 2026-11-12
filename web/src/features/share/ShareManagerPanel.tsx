import {
  Check,
  Clock3,
  Copy,
  ExternalLink,
  FileText,
  MessageSquareText,
  RefreshCcw,
  Search,
  Share2,
  Trash2,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { deletePreviewLink, listPreviewLinks } from "@/api/files";
import { deleteSessionShare, listSessionShares, type SessionShare } from "@/api/shares";
import { appActions } from "@/app/store";
import { useAppDispatch } from "@/app/hooks";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { copyText } from "@/lib/clipboard";
import { useTimedState } from "@/lib/useTimedState";
import { cn } from "@/lib/cn";
import { formatTime, shortID } from "@/lib/format";
import type { PreviewLink } from "@/types/files";

type ShareFilter = "all" | "protected" | "open" | "permanent";

export function ShareManagerPanel() {
  const dispatch = useAppDispatch();
  const [shares, setShares] = useState<SessionShare[]>([]);
  const [fileShares, setFileShares] = useState<PreviewLink[]>([]);
  const [keyword, setKeyword] = useState("");
  const [filter, setFilter] = useState<ShareFilter>("all");
  const [loading, setLoading] = useState(false);
  const [busyKey, setBusyKey] = useState("");
  const [copiedKey, showCopiedKey] = useTimedState("");
  const [deleteTarget, setDeleteTarget] = useState<SessionShare | null>(null);
  const [deleteFileTarget, setDeleteFileTarget] = useState<PreviewLink | null>(null);
  const counts = useMemo(() => countShares(shares, fileShares), [shares, fileShares]);
  const filteredShares = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    return shares
      .filter((share) => {
        if (filter === "protected") return Boolean(share.has_password);
        if (filter === "open") return !share.has_password;
        if (filter === "permanent") return !Number(share.expires_at || 0);
        return true;
      })
      .filter((share) => {
        if (!query) return true;
        return `${shareID(share)} ${share.session_id} ${share.title || ""}`.toLowerCase().includes(query);
      })
      .sort((left, right) => Number(right.created_at || 0) - Number(left.created_at || 0));
  }, [shares, filter, keyword]);
  const filteredFileShares = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    return fileShares
      .filter((share) => {
        if (filter === "protected") return Boolean(share.has_password);
        if (filter === "open") return !share.has_password;
        if (filter === "permanent") return !Number(share.expires_at || 0);
        return true;
      })
      .filter((share) => {
        if (!query) return true;
        return `${previewLinkID(share)} ${share.file_path || ""} ${(share.file_paths || []).join(" ")}`.toLowerCase().includes(query);
      })
      .sort((left, right) => Number(right.created_at || 0) - Number(left.created_at || 0));
  }, [fileShares, filter, keyword]);

  async function load() {
    setLoading(true);
    try {
      const [sessionResult, fileResult] = await Promise.all([listSessionShares(), listPreviewLinks()]);
      setShares(Array.isArray(sessionResult) ? sessionResult : sessionResult.shares || []);
      setFileShares(Array.isArray(fileResult) ? fileResult : fileResult.links || []);
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setLoading(false);
    }
  }

  async function copyShare(share: SessionShare) {
    const id = shareID(share);
    if (!id) return;
    const url = shareURL(id);
    try {
      await copyText(url);
      showCopiedKey(shareKey("session", id));
      dispatch(appActions.showToast("分享链接已复制"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : "复制失败"));
    }
  }

  async function copyFileShare(share: PreviewLink) {
    const id = previewLinkID(share);
    if (!id) return;
    const url = previewURL(id);
    try {
      await copyText(url);
      showCopiedKey(shareKey("file", id));
      dispatch(appActions.showToast("分享链接已复制"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : "复制失败"));
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    const id = shareID(deleteTarget);
    const key = shareKey("session", id);
    if (!id || busyKey) return;
    setBusyKey(key);
    try {
      await deleteSessionShare(id);
      setShares((current) => current.filter((share) => shareID(share) !== id));
      setDeleteTarget(null);
      dispatch(appActions.showToast("分享已删除"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusyKey("");
    }
  }

  async function confirmDeleteFile() {
    if (!deleteFileTarget) return;
    const id = previewLinkID(deleteFileTarget);
    const key = shareKey("file", id);
    if (!id || busyKey) return;
    setBusyKey(key);
    try {
      await deletePreviewLink(id);
      setFileShares((current) => current.filter((share) => previewLinkID(share) !== id));
      setDeleteFileTarget(null);
      dispatch(appActions.showToast("文件分享已删除"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusyKey("");
    }
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
                <Share2 className="h-5 w-5 text-primary" />
                <h2 className="text-xl font-semibold">分享管理</h2>
              </div>
              <div className="mt-1 text-sm text-muted-foreground">管理已经创建的会话分享和文件分享，复制访问地址或撤销公开访问。</div>
            </div>
            <div className="grid w-full min-w-0 grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_auto] xl:w-[520px]">
              <Input
                className="min-w-0"
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                placeholder="搜索分享标题、会话、文件或 ID"
              />
              <Button className="min-w-20 justify-center whitespace-nowrap" variant="outline" onClick={() => void load()} disabled={loading}>
                <RefreshCcw className="h-4 w-4" />
                刷新
              </Button>
            </div>
          </PanelHeader>
          <PanelBody className="grid gap-3 border-t border-border/70 md:grid-cols-4">
            <MetricCard icon={Share2} label="全部分享" value={shares.length + fileShares.length} />
            <MetricCard icon={MessageSquareText} label="会话分享" value={shares.length} />
            <MetricCard icon={FileText} label="文件分享" value={fileShares.length} />
            <MetricCard icon={Clock3} label="永不过期" value={counts.permanent} />
          </PanelBody>
        </Panel>

        <Panel>
          <PanelHeader className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="chat-scroll flex gap-2 overflow-x-auto">
              <FilterButton active={filter === "all"} label="全部" count={shares.length + fileShares.length} onClick={() => setFilter("all")} />
              <FilterButton active={filter === "protected"} label="带密码" count={counts.protected} onClick={() => setFilter("protected")} />
              <FilterButton active={filter === "open"} label="公开" count={counts.open} onClick={() => setFilter("open")} />
              <FilterButton active={filter === "permanent"} label="永不过期" count={counts.permanent} onClick={() => setFilter("permanent")} />
            </div>
            <div className="text-sm text-muted-foreground">{filteredShares.length + filteredFileShares.length} 条分享符合当前筛选</div>
          </PanelHeader>
        </Panel>

        <Panel>
          <PanelHeader className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <MessageSquareText className="h-4 w-4 text-primary" />
              <div className="font-semibold">会话分享</div>
            </div>
            <div className="text-sm text-muted-foreground">{filteredShares.length} 条</div>
          </PanelHeader>
          <PanelBody>
            <ShareGrid
              shares={filteredShares}
              loading={loading}
              busyKey={busyKey}
              copiedKey={copiedKey}
              onCopy={(share) => void copyShare(share)}
              onDelete={setDeleteTarget}
            />
          </PanelBody>
        </Panel>

        <Panel>
          <PanelHeader className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <FileText className="h-4 w-4 text-primary" />
              <div className="font-semibold">文件分享</div>
            </div>
            <div className="text-sm text-muted-foreground">{filteredFileShares.length} 条</div>
          </PanelHeader>
          <PanelBody>
            <FileShareGrid
              shares={filteredFileShares}
              loading={loading}
              busyKey={busyKey}
              copiedKey={copiedKey}
              onCopy={(share) => void copyFileShare(share)}
              onDelete={setDeleteFileTarget}
            />
          </PanelBody>
        </Panel>
      </div>

      <ShareDeleteDialog
        share={deleteTarget}
        deleting={Boolean(deleteTarget && busyKey === shareKey("session", shareID(deleteTarget)))}
        onCancel={() => {
          if (!busyKey) setDeleteTarget(null);
        }}
        onConfirm={() => void confirmDelete()}
      />
      <FileShareDeleteDialog
        share={deleteFileTarget}
        deleting={Boolean(deleteFileTarget && busyKey === shareKey("file", previewLinkID(deleteFileTarget)))}
        onCancel={() => {
          if (!busyKey) setDeleteFileTarget(null);
        }}
        onConfirm={() => void confirmDeleteFile()}
      />
    </div>
  );
}

function ShareGrid({
  shares,
  loading,
  busyKey,
  copiedKey,
  onCopy,
  onDelete,
}: {
  shares: SessionShare[];
  loading: boolean;
  busyKey: string;
  copiedKey: string;
  onCopy: (share: SessionShare) => void;
  onDelete: (share: SessionShare) => void;
}) {
  if (shares.length === 0) {
    return (
      <div className="flex min-h-56 flex-col items-center justify-center rounded-md border border-dashed border-border bg-background/45 px-4 py-10 text-center">
        <Search className="h-8 w-8 text-muted-foreground" />
        <div className="mt-3 text-base font-semibold">{loading ? "正在加载分享" : "暂无分享链接"}</div>
        <div className="mt-1 max-w-md text-sm leading-6 text-muted-foreground">
          从会话右上角分享图标或左侧会话菜单创建分享后，会在这里集中管理。
        </div>
      </div>
    );
  }

  return (
    <div className="grid gap-3 xl:grid-cols-2">
      {shares.map((share) => {
        const id = shareID(share);
        const deleting = busyKey === shareKey("session", id);
        const copied = copiedKey === shareKey("session", id);
        return (
          <article key={id} className="rounded-md border border-border bg-card/70 p-4 shadow-[2px_3px_0_hsl(218_32%_30%/0.06)]">
            <div className="flex min-w-0 items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <MessageSquareText className="h-4 w-4 shrink-0 text-primary" />
                  <h3 className="truncate text-base font-semibold">{share.title || shortID(share.session_id)}</h3>
                </div>
                <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{id}</div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  className="flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
                  type="button"
                  aria-label="复制分享链接"
                  title="复制分享链接"
                  onClick={() => onCopy(share)}
                  disabled={!id}
                >
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                </button>
                <a
                  className="flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
                  href={shareURL(id)}
                  target="_blank"
                  rel="noreferrer"
                  aria-label="打开分享链接"
                  title="打开分享链接"
                >
                  <ExternalLink className="h-4 w-4" />
                </a>
                <button
                  className="flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:text-destructive disabled:pointer-events-none disabled:opacity-40"
                  type="button"
                  aria-label="删除分享"
                  title="删除分享"
                  onClick={() => onDelete(share)}
                  disabled={deleting}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap gap-2">
              <StatusBadge active={Boolean(share.has_password)} label={share.has_password ? "需要密码" : "无密码"} />
              <StatusBadge active={Boolean(share.allow_tool_details)} label={share.allow_tool_details ? "包含工具细节" : "隐藏工具细节"} />
              <Badge>{Number(share.message_count || 0)} 条消息</Badge>
            </div>

            <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-3">
              <ShareMeta label="会话" value={shortID(share.session_id)} mono />
              <ShareMeta label="创建时间" value={formatShareTime(share.created_at)} />
              <ShareMeta label="过期时间" value={formatExpiry(share.expires_at)} />
              <ShareMeta label="最近访问" value={formatShareTime(share.last_accessed_at) || "尚未访问"} />
              <ShareMeta className="sm:col-span-2" label="访问地址" value={shareURL(id)} mono />
            </dl>
          </article>
        );
      })}
    </div>
  );
}

function FileShareGrid({
  shares,
  loading,
  busyKey,
  copiedKey,
  onCopy,
  onDelete,
}: {
  shares: PreviewLink[];
  loading: boolean;
  busyKey: string;
  copiedKey: string;
  onCopy: (share: PreviewLink) => void;
  onDelete: (share: PreviewLink) => void;
}) {
  if (shares.length === 0) {
    return (
      <div className="flex min-h-56 flex-col items-center justify-center rounded-md border border-dashed border-border bg-background/45 px-4 py-10 text-center">
        <Search className="h-8 w-8 text-muted-foreground" />
        <div className="mt-3 text-base font-semibold">{loading ? "正在加载文件分享" : "暂无文件分享链接"}</div>
        <div className="mt-1 max-w-md text-sm leading-6 text-muted-foreground">
          从文件管理中的分享图标创建文件分享后，会在这里集中管理。
        </div>
      </div>
    );
  }

  return (
    <div className="grid gap-3 xl:grid-cols-2">
      {shares.map((share) => {
        const id = previewLinkID(share);
        const deleting = busyKey === shareKey("file", id);
        const copied = copiedKey === shareKey("file", id);
        const filePaths = share.file_paths || [];
        const title = share.file_path || filePaths[0] || id;
        return (
          <article key={id} className="rounded-md border border-border bg-card/70 p-4 shadow-[2px_3px_0_hsl(218_32%_30%/0.06)]">
            <div className="flex min-w-0 items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <FileText className="h-4 w-4 shrink-0 text-primary" />
                  <h3 className="truncate text-base font-semibold">{title}</h3>
                </div>
                <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{id}</div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  className="flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
                  type="button"
                  aria-label="复制分享链接"
                  title="复制分享链接"
                  onClick={() => onCopy(share)}
                  disabled={!id}
                >
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                </button>
                <a
                  className="flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
                  href={previewURL(id)}
                  target="_blank"
                  rel="noreferrer"
                  aria-label="打开分享链接"
                  title="打开分享链接"
                >
                  <ExternalLink className="h-4 w-4" />
                </a>
                <button
                  className="flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:text-destructive disabled:pointer-events-none disabled:opacity-40"
                  type="button"
                  aria-label="删除分享"
                  title="删除分享"
                  onClick={() => onDelete(share)}
                  disabled={deleting}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap gap-2">
              <StatusBadge active={Boolean(share.has_password)} label={share.has_password ? "需要密码" : "无密码"} />
              <Badge>{filePaths.length > 1 ? `${filePaths.length} 个文件` : "单文件"}</Badge>
            </div>

            <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-3">
              <ShareMeta className="sm:col-span-2" label="文件" value={title} mono />
              <ShareMeta label="创建时间" value={formatShareTime(share.created_at)} />
              <ShareMeta label="过期时间" value={formatExpiry(share.expires_at)} />
              <ShareMeta className="sm:col-span-2" label="访问地址" value={previewURL(id)} mono />
            </dl>
          </article>
        );
      })}
    </div>
  );
}

function ShareDeleteDialog({
  share,
  deleting,
  onCancel,
  onConfirm,
}: {
  share: SessionShare | null;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!share) return null;
  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="share-delete-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onCancel();
      }}
    >
      <div className="sketch-surface max-h-[92%] w-full max-w-md overflow-auto rounded-t-2xl bg-card/95 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:max-h-full sm:rounded-2xl shadow-[0_18px_48px_hsl(218_30%_20%/0.18)] sm:p-5">
        <div className="flex items-start gap-3">
          <div className="mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-destructive/30 bg-destructive/10 text-destructive">
            <Trash2 className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="share-delete-title" className="text-lg font-semibold text-foreground">
              删除分享
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              分享「{share.title || shortID(share.session_id)}」会立即失效，已复制的公开链接也将无法继续访问。
            </p>
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={deleting}>
            取消
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={deleting}>
            {deleting ? "删除中" : "确认删除"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function FileShareDeleteDialog({
  share,
  deleting,
  onCancel,
  onConfirm,
}: {
  share: PreviewLink | null;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!share) return null;
  const title = share.file_path || share.file_paths?.[0] || previewLinkID(share);
  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="file-share-delete-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onCancel();
      }}
    >
      <div className="sketch-surface max-h-[92%] w-full max-w-md overflow-auto rounded-t-2xl bg-card/95 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:max-h-full sm:rounded-2xl shadow-[0_18px_48px_hsl(218_30%_20%/0.18)] sm:p-5">
        <div className="flex items-start gap-3">
          <div className="mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-destructive/30 bg-destructive/10 text-destructive">
            <Trash2 className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="file-share-delete-title" className="text-lg font-semibold text-foreground">
              删除文件分享
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              文件分享「{title}」会立即失效，已复制的公开链接也将无法继续访问。
            </p>
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={deleting}>
            取消
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={deleting}>
            {deleting ? "删除中" : "确认删除"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function FilterButton({
  active,
  label,
  count,
  onClick,
}: {
  active: boolean;
  label: string;
  count: number;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "flex h-9 shrink-0 items-center gap-2 rounded-md border px-3 text-sm font-semibold transition-colors",
        active ? "border-primary/60 bg-primary/10 text-primary" : "border-border bg-card/70 text-muted-foreground hover:bg-accent/70",
      )}
      type="button"
      onClick={onClick}
    >
      <span>{label}</span>
      <span className="text-xs opacity-70">{count}</span>
    </button>
  );
}

function MetricCard({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: number }) {
  return (
    <div className="rounded-md border border-border bg-background/55 p-3">
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">{label}</div>
        <Icon className="h-4 w-4 text-primary" />
      </div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
    </div>
  );
}

function StatusBadge({ active, label }: { active: boolean; label: string }) {
  return (
    <Badge className={active ? "border-primary/35 bg-primary/10 text-primary" : undefined}>
      {label}
    </Badge>
  );
}

function ShareMeta({
  label,
  value,
  mono,
  className,
}: {
  label: string;
  value: string;
  mono?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0", className)}>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={cn("mt-1 truncate text-foreground", mono && "font-mono text-xs")}>{value || "-"}</dd>
    </div>
  );
}

function countShares(shares: SessionShare[], fileShares: PreviewLink[]) {
  const initial = { protected: 0, open: 0, permanent: 0 };
  const sessionCounts = shares.reduce(
    (counts, share) => {
      if (share.has_password) counts.protected += 1;
      else counts.open += 1;
      if (!Number(share.expires_at || 0)) counts.permanent += 1;
      return counts;
    },
    initial,
  );
  return fileShares.reduce((counts, share) => {
    if (share.has_password) counts.protected += 1;
    else counts.open += 1;
    if (!Number(share.expires_at || 0)) counts.permanent += 1;
    return counts;
  }, sessionCounts);
}

function shareID(share: SessionShare) {
  return share.id || share.share_id || "";
}

function shareURL(id: string) {
  return `${location.origin}/s/${encodeURIComponent(id)}`;
}

function previewLinkID(share: PreviewLink) {
  return share.link_id || share.id || "";
}

function previewURL(id: string) {
  return `${location.origin}/p/${encodeURIComponent(id)}`;
}

function shareKey(kind: "session" | "file", id: string) {
  return `${kind}:${id}`;
}

function formatShareTime(value?: string | number) {
  if (!value) return "";
  return formatTime(value);
}

function formatExpiry(value?: string | number) {
  return Number(value || 0) ? formatTime(value) : "永不过期";
}
