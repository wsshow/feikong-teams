import { useEffect, useState } from "react";
import { accessPublicShare, getPublicShareInfo, type SessionShare } from "@/api/shares";
import { APIError } from "@/api/client";
import type { SessionDetail } from "@/types/chat";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { formatTime } from "@/lib/format";

export function SharePage() {
  const shareID = decodeURIComponent(location.pathname.split("/").filter(Boolean).pop() || "");
  const [info, setInfo] = useState<SessionShare | null>(null);
  const [title, setTitle] = useState("");
  const [password, setPassword] = useState("");
  const [detail, setDetail] = useState<SessionDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!shareID) return;
    let cancelled = false;
    async function init() {
      setLoading(true);
      setError("");
      try {
        const nextInfo = await getPublicShareInfo(shareID);
        if (cancelled) return;
        setInfo(nextInfo);
        setTitle(nextInfo.title || nextInfo.id || nextInfo.share_id || shareID);
        if (!nextInfo.has_password) {
          const nextDetail = await accessPublicShare(shareID, "");
          if (!cancelled) setDetail(nextDetail);
        }
      } catch (err) {
        if (!cancelled) setError(publicShareErrorMessage(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void init();
    return () => {
      cancelled = true;
    };
  }, [shareID]);

  async function load() {
    setError("");
    setLoading(true);
    try {
      setDetail(await accessPublicShare(shareID, password));
    } catch (err) {
      setError(publicShareErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen bg-muted/30 px-4 py-10">
      {!detail ? (
        <div className="flex min-h-[calc(100vh-5rem)] items-center justify-center">
          <Panel className="w-full max-w-xl">
            <PanelHeader className="text-center">
              <div className="text-lg font-semibold">{title || "共享会话"}</div>
              <div className="mt-1 text-sm text-muted-foreground">
                {info?.message_count ? `${info.message_count} 条消息 · ` : ""}
                {info?.expires_at ? `有效期至 ${formatTime(info.expires_at)}` : "公开只读视图"}
              </div>
            </PanelHeader>
            <PanelBody className="space-y-4">
              {info?.has_password ? (
                <div className="mx-auto flex w-full max-w-md flex-col gap-3 sm:flex-row">
                  <Input
                    className="min-w-0 flex-1"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") void load();
                    }}
                    placeholder="输入访问密码"
                    type="password"
                  />
                  <Button className="min-w-24 whitespace-nowrap" onClick={() => void load()} disabled={loading}>
                    {loading ? "查看中" : "查看分享"}
                  </Button>
                </div>
              ) : error ? (
                <div className="mx-auto max-w-md py-8 text-center">
                  <div className="text-base font-semibold text-foreground">{error}</div>
                  <div className="mt-2 text-sm leading-6 text-muted-foreground">
                    可以回到原会话重新创建分享，或联系分享创建者确认链接是否仍然有效。
                  </div>
                </div>
              ) : (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  {loading ? "正在打开分享..." : "正在准备分享内容..."}
                </div>
              )}
              {error && info?.has_password ? <div className="text-center text-sm text-destructive">{error}</div> : null}
            </PanelBody>
          </Panel>
        </div>
      ) : (
        <Panel className="mx-auto max-w-5xl">
          <PanelHeader className="text-center">
            <div className="text-lg font-semibold">{detail.title || title || "共享会话"}</div>
            <div className="mt-1 text-sm text-muted-foreground">公开只读视图</div>
          </PanelHeader>
          <PanelBody className="space-y-4">
            {detail.messages?.map((message, index) => (
              <div key={index} className="rounded-md border border-border bg-card/70 p-4">
                <div className="mb-2 text-xs text-muted-foreground">{message.agent_name || message.role}</div>
                <MarkdownContent
                  className="prose-sm"
                  content={(message.events || []).map((event) => event.content || event.tool_call?.result || "").join("\n") || message.content || ""}
                />
              </div>
            ))}
          </PanelBody>
        </Panel>
      )}
    </div>
  );
}

function publicShareErrorMessage(error: unknown) {
  if (error instanceof APIError) {
    if (error.status === 404) return "这个分享已失效或已被取消";
    if (error.status === 410) return "这个分享已过期";
    if (error.status === 401) return "访问密码不正确";
  }
  const message = error instanceof Error ? error.message : String(error);
  if (/not found/i.test(message)) return "这个分享已失效或已被取消";
  if (/expired|gone/i.test(message)) return "这个分享已过期";
  if (/password|unauthorized/i.test(message)) return "访问密码不正确";
  return "暂时无法打开这个分享";
}
