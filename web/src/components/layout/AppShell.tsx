import { Menu, Radio } from "lucide-react";
import { appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Sidebar } from "./Sidebar";

export function AppShell({ children }: { children: React.ReactNode }) {
  const dispatch = useAppDispatch();
  const connectionState = useAppSelector((state) => state.chat.connectionState);
  const toast = useAppSelector((state) => state.app.toast);
  const statusText = useAppSelector((state) => state.chat.statusText);
  const version = useAppSelector((state) => state.app.version);
  const connected = connectionState === "connected";
  const connectionLabel = connected ? "已连接" : connectionState === "connecting" ? "连接中" : "未连接";

  return (
    <div className="flex h-screen overflow-hidden bg-background/95 text-foreground">
      <Sidebar />
      <main className="relative flex min-w-0 flex-1 flex-col">
        <div className="pointer-events-none absolute inset-x-0 top-3 z-30 flex items-center justify-center px-16">
          <div className="pointer-events-auto inline-flex max-w-[52rem] items-center gap-2 rounded-lg bg-muted/70 px-3 py-1.5 text-sm text-muted-foreground shadow-[0_1px_0_hsl(218_30%_76%/0.36)] backdrop-blur">
            <span>{connectionLabel}</span>
            <span className="text-border">·</span>
            <span>{version?.version || "dev"}</span>
            {statusText ? (
              <>
                <span className="text-border">·</span>
                <span className="truncate">{statusText}</span>
              </>
            ) : null}
          </div>
        </div>
        <div className="pointer-events-none absolute left-4 top-3 z-30 md:hidden">
          <div className="pointer-events-auto">
            <Button
              size="icon"
              variant="ghost"
              aria-label="打开导航"
              onClick={() => dispatch(appActions.setSidebarOpen(true))}
            >
              <Menu className="h-4 w-4" />
            </Button>
          </div>
        </div>
        <div className="pointer-events-none absolute right-5 top-4 z-30 flex items-center gap-2 text-xs text-muted-foreground">
          <Radio className="h-4 w-4" />
          <span
            className={
              connected
                ? "h-2.5 w-2.5 rounded-full bg-emerald-500 shadow-[0_0_0_3px_hsl(152_70%_45%/0.12)]"
                : "h-2.5 w-2.5 rounded-full bg-amber-500 shadow-[0_0_0_3px_hsl(38_90%_45%/0.12)]"
            }
          />
        </div>
        <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
      </main>
      {toast ? (
        <div className="sketch-surface fixed bottom-4 right-4 z-50 rounded-md px-4 py-3 text-sm">
          {toast}
        </div>
      ) : null}
    </div>
  );
}
