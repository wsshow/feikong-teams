import { Check, Copy, FileText, Share2 } from "lucide-react";
import { useEffect, useState } from "react";
import { createPreviewLink } from "@/api/files";
import { appActions } from "@/app/store";
import { useAppDispatch } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/cn";
import { useTimedState } from "@/lib/useTimedState";
import type { FileEntry, PreviewLink } from "@/types/files";

const fileShareExpiryOptions = [
  { label: "1 天", value: 24 * 3600 },
  { label: "7 天", value: 7 * 24 * 3600 },
  { label: "30 天", value: 30 * 24 * 3600 },
  { label: "永不过期", value: -1 },
] as const;

export function FileShareDialog({
  file,
  onClose,
}: {
  file: FileEntry | null;
  onClose: () => void;
}) {
  const dispatch = useAppDispatch();
  const [expiresIn, setExpiresIn] = useState<number>(24 * 3600);
  const [password, setPassword] = useState("");
  const [creating, setCreating] = useState(false);
  const [createdLink, setCreatedLink] = useState<PreviewLink | null>(null);
  const [copied, showCopied, resetCopied] = useTimedState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!file) return;
    setExpiresIn(24 * 3600);
    setPassword("");
    setCreating(false);
    setCreatedLink(null);
    resetCopied();
    setError("");
  }, [file, resetCopied]);

  if (!file) return null;

  const previewURL = createdLink ? `${location.origin}/p/${encodeURIComponent(previewLinkID(createdLink))}` : "";

  async function createShare() {
    if (!file || creating) return;
    setCreating(true);
    setError("");
    try {
      const link = await createPreviewLink(file.path, {
        password: password.trim(),
        expires_in: expiresIn,
      });
      setCreatedLink(link);
      dispatch(appActions.showToast("文件分享已创建"));
    } catch (shareError) {
      setError(shareError instanceof Error ? shareError.message : String(shareError));
    } finally {
      setCreating(false);
    }
  }

  async function copyPreviewURL() {
    if (!previewURL) return;
    try {
      await copyText(previewURL);
      showCopied(true);
      dispatch(appActions.showToast("分享链接已复制"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : "复制失败"));
    }
  }

  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="file-share-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !creating) onClose();
      }}
    >
      <div className="sketch-surface max-h-[92%] w-full max-w-lg overflow-auto rounded-t-2xl bg-card p-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:max-h-full sm:rounded-2xl shadow-dialog sm:p-5">
        <div className="flex items-start gap-3">
          <div className="mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-primary/30 bg-accent text-primary">
            <Share2 className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="file-share-title" className="text-lg font-semibold text-foreground">
              分享文件
            </h2>
            <p className="mt-2 flex min-w-0 items-center gap-2 text-sm leading-6 text-muted-foreground">
              <FileText className="h-4 w-4 shrink-0" />
              <span className="truncate">{file.path}</span>
            </p>
          </div>
        </div>

        <div className="mt-5 space-y-4">
          <div>
            <div className="mb-2 text-sm font-medium text-foreground">过期时间</div>
            <div className="grid grid-cols-4 gap-2">
              {fileShareExpiryOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={cn(
                    "h-9 rounded-md border px-2 text-sm font-semibold transition-colors",
                    expiresIn === option.value
                      ? "border-primary/25 bg-accent text-primary"
                      : "border-border bg-background/60 text-muted-foreground hover:bg-muted/70",
                  )}
                  onClick={() => setExpiresIn(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-foreground" htmlFor="file-share-password">
              访问密码
            </label>
            <Input
              id="file-share-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="留空表示无需密码"
            />
          </div>

          {createdLink ? (
            <div className="rounded-md border border-border bg-background/55 p-3">
              <div className="mb-2 text-sm font-medium text-foreground">分享链接</div>
              <div className="flex items-center gap-2">
                <Input className="font-mono text-xs" value={previewURL} readOnly />
                <Button size="icon" variant="outline" onClick={() => void copyPreviewURL()} aria-label="复制分享链接">
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                </Button>
              </div>
            </div>
          ) : null}

          {error ? <div className="text-sm text-destructive">{error}</div> : null}
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onClose} disabled={creating}>
            关闭
          </Button>
          <Button onClick={() => void createShare()} disabled={creating}>
            {creating ? "创建中" : createdLink ? "重新创建" : "创建分享"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function previewLinkID(link: PreviewLink) {
  return link.link_id || link.id || "";
}
