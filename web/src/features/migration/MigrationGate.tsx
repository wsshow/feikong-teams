import { useEffect, useRef, useState, type ReactNode } from "react";
import { APIError, get, post } from "@/api/client";
import { Button } from "@/components/ui/button";

type MigrationStatus = { state: "pending" | "running" | "failed" | "completed"; stage?: string; completed?: number; total?: number; backup_path?: string; error?: string };
type Status = { ready: boolean; migration: MigrationStatus };
const stages: Record<string, string> = { backup: "正在创建并核对备份", import: "正在迁移业务数据", verify: "正在校验迁移结果", completed: "正在启动工作区" };

export function MigrationGate({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status | null>(null);
  const [connectionError, setConnectionError] = useState("");
  const [starting, setStarting] = useState(false);
  const [entered, setEntered] = useState(false);
  const [retry, setRetry] = useState(0);
  const seenMigration = useRef(false);
  const backupPath = useRef("");
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function refresh() {
      try {
        const result = await get<Status>("/api/fkteams/storage/migration", { signal: controller.signal, authFailure: "ignore" });
        if (controller.signal.aborted) return;
        if (!result.ready) seenMigration.current = true;
        if (result.migration.backup_path) backupPath.current = result.migration.backup_path;
        setStatus(result); setConnectionError("");
        if (result.ready) return;
      } catch (error) {
        if (controller.signal.aborted) return;
        if (error instanceof APIError && error.status === 401) {
          location.assign(`/login?next=${encodeURIComponent(location.pathname)}`); return;
        }
        setConnectionError("暂时无法连接服务，正在重试。后台升级不会因页面断线而取消。");
      }
      timer = setTimeout(() => void refresh(), 1000);
    }
    void refresh();
    return () => { controller.abort(); if (timer) clearTimeout(timer); };
  }, [retry]);
  async function start() {
    setStarting(true); setConnectionError("");
    try {
      const result = await post<Status>("/api/fkteams/storage/migration");
      seenMigration.current = true; setStatus(result);
    } catch (error) { setConnectionError(error instanceof Error ? error.message : "无法开始升级，请重试"); }
    finally { setStarting(false); }
  }
  if (entered || status?.ready && !seenMigration.current) return children;
  const migration = status?.migration;
  const finished = !!status?.ready;
  const running = starting || migration?.state === "running" || migration?.state === "completed" && !finished;
  const total = migration?.total || 0;
  const completed = migration?.completed || 0;
  return <div className="flex h-[var(--app-viewport-height,100dvh)] items-center justify-center bg-background p-4 sm:p-6">
    <section role="dialog" aria-modal="true" aria-labelledby="migration-title" aria-describedby="migration-description" className="sketch-surface max-h-[90vh] w-full max-w-lg overflow-auto rounded-xl bg-card p-5 sm:p-7">
      <h1 id="migration-title" className="text-xl font-semibold">{finished ? "数据升级完成" : status ? "升级本地数据存储" : "正在检查数据存储"}</h1>
      <p id="migration-description" className="mt-3 text-sm leading-6 text-muted-foreground">{finished ? "数据已迁移并通过校验，现在可以继续使用。" : "系统将自动备份、迁移并校验旧版数据。升级期间暂停业务服务，刷新或关闭页面不会取消后台升级。"}</p>
      {migration?.state === "pending" && <div className="my-5 rounded-md border bg-background p-4 text-sm leading-7"><p>迁移会话与聊天记录、待执行消息、定时任务、分享、扩展状态和长期记忆。</p><p className="mt-2 text-muted-foreground">请先退出仍在运行的旧版实例。附件与报告文件保留原位置。升级后可在“记忆”页面管理长期记忆。</p></div>}
      {running && <div className="my-5 space-y-3" role="status" aria-live="polite"><p className="text-sm font-medium">{stages[migration?.stage || ""] || "正在准备升级"}</p>{total > 0 && <><progress className="h-2 w-full accent-primary" max={total} value={completed} aria-label="当前阶段进度" /><p className="text-right text-xs tabular-nums text-muted-foreground">{completed} / {total}</p></>}</div>}
      {migration?.state === "failed" && <div role="alert" className="my-5 rounded-md border border-destructive/30 p-3 text-sm"><p className="font-semibold text-destructive">升级未完成</p><p className="mt-2 break-words">{migration.error}</p><p className="mt-2 text-muted-foreground">原文件已保留。处理问题后可以重试。</p></div>}
      {connectionError && <p role="alert" className="my-4 text-sm text-destructive">{connectionError}</p>}
      {(migration?.backup_path || backupPath.current) && <p className="my-4 break-all text-xs leading-5 text-muted-foreground">备份位置：{migration?.backup_path || backupPath.current}</p>}
      <div className="mt-6 flex justify-end gap-2">{connectionError && <Button variant="outline" onClick={() => setRetry((value) => value + 1)}>重新连接</Button>}{finished ? <Button autoFocus onClick={() => setEntered(true)}>开始使用</Button> : (migration?.state === "pending" || migration?.state === "failed") && <Button autoFocus disabled={starting} onClick={() => void start()}>{starting ? "正在开始" : migration.state === "failed" ? "重试升级" : "备份并开始升级"}</Button>}</div>
    </section>
  </div>;
}
