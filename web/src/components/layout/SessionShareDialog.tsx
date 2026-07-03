import { Check, Copy, Share2 } from "lucide-react";
import { useEffect, useState } from "react";
import { appActions } from "@/app/store";
import { useAppDispatch } from "@/app/hooks";
import { createSessionShare, type SessionShare } from "@/api/shares";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { shortID } from "@/lib/format";
import { cn } from "@/lib/cn";

export interface ShareableSession {
  session_id: string;
  title?: string;
}

const shareExpiryOptions = [
  { label: "1 天", value: 24 * 3600 },
  { label: "7 天", value: 7 * 24 * 3600 },
  { label: "30 天", value: 30 * 24 * 3600 },
  { label: "永不过期", value: -1 },
] as const;

export function SessionShareDialog({
  session,
  onClose,
  onCreated,
}: {
  session: ShareableSession | null;
  onClose: () => void;
  onCreated?: () => void;
}) {
  const dispatch = useAppDispatch();
  const [expiresIn, setExpiresIn] = useState<number>(7 * 24 * 3600);
  const [password, setPassword] = useState("");
  const [allowToolDetails, setAllowToolDetails] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createdShare, setCreatedShare] = useState<SessionShare | null>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");

  const sessionID = session?.session_id || "";

  useEffect(() => {
    if (!sessionID) return;
    setExpiresIn(7 * 24 * 3600);
    setPassword("");
    setAllowToolDetails(false);
    setCreatedShare(null);
    setCopied(false);
    setError("");
  }, [sessionID]);

  if (!session) return null;

  const shareURL = createdShare ? `${location.origin}/s/${encodeURIComponent(shareID(createdShare))}` : "";

  async function createShare() {
    if (!session || creating) return;
    setCreating(true);
    setError("");
    try {
      const share = await createSessionShare(session.session_id, {
        password: password.trim(),
        expires_in: expiresIn,
        allow_tool_details: allowToolDetails,
      });
      setCreatedShare(share);
      onCreated?.();
      dispatch(appActions.showToast("分享已创建"));
    } catch (shareError) {
      setError(shareError instanceof Error ? shareError.message : String(shareError));
    } finally {
      setCreating(false);
    }
  }

  async function copyShareURL() {
    if (!shareURL) return;
    await navigator.clipboard?.writeText(shareURL);
    setCopied(true);
    dispatch(appActions.showToast("分享链接已复制"));
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="session-share-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !creating) onClose();
      }}
    >
      <div className="sketch-surface max-h-[calc(100dvh-1.5rem)] w-full max-w-lg overflow-auto rounded-2xl bg-card/95 p-4 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)] sm:p-5">
        <div className="flex items-start gap-3">
          <div className="mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-primary/30 bg-primary/10 text-primary">
            <Share2 className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="session-share-title" className="text-lg font-semibold text-foreground">
              分享会话
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              为「{session.title || shortID(session.session_id)}」创建可访问的分享链接。
            </p>
          </div>
        </div>

        <div className="mt-5 space-y-4">
          <div>
            <div className="mb-2 text-sm font-medium text-foreground">过期时间</div>
            <div className="grid grid-cols-4 gap-2">
              {shareExpiryOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={cn(
                    "h-9 rounded-md border px-2 text-sm font-semibold transition-colors",
                    expiresIn === option.value ? "border-primary/60 bg-primary/10 text-primary" : "border-border bg-background/60 text-muted-foreground hover:bg-muted/70",
                  )}
                  onClick={() => setExpiresIn(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-foreground" htmlFor="session-share-password">
              访问密码
            </label>
            <Input
              id="session-share-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="留空表示无需密码"
            />
          </div>

          <label className="flex items-center gap-3 rounded-md border border-border bg-background/55 px-3 py-2 text-sm text-muted-foreground">
            <input
              className="h-4 w-4 accent-primary"
              type="checkbox"
              checked={allowToolDetails}
              onChange={(event) => setAllowToolDetails(event.target.checked)}
            />
            <span>包含工具调用细节</span>
          </label>

          {createdShare ? (
            <div className="rounded-md border border-border bg-background/55 p-3">
              <div className="mb-2 text-sm font-medium text-foreground">分享链接</div>
              <div className="flex items-center gap-2">
                <Input className="font-mono text-xs" value={shareURL} readOnly />
                <Button size="icon" variant="outline" onClick={() => void copyShareURL()} aria-label="复制分享链接">
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
            {creating ? "创建中" : createdShare ? "重新创建" : "创建分享"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function shareID(share: SessionShare) {
  return share.id || share.share_id || "";
}
