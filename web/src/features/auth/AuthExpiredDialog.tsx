import { LockKeyhole } from "lucide-react";
import { LoginForm } from "@/features/auth/LoginForm";
import { restoreAuthentication } from "@/lib/auth-session";

export function AuthExpiredDialog({ open }: { open: boolean }) {
  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-background/85 p-4 backdrop-blur-md sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="auth-expired-title"
      aria-describedby="auth-expired-description"
    >
      <div className="sketch-surface w-full max-w-md rounded-2xl bg-card/95 p-6 shadow-[0_24px_72px_hsl(218_30%_20%/0.24)] sm:p-7">
        <div className="flex items-start gap-4">
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-primary/30 bg-primary/10 text-primary">
            <LockKeyhole className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="auth-expired-title" className="text-xl font-semibold text-foreground">
              登录已过期
            </h2>
            <p id="auth-expired-description" className="mt-2 text-sm leading-6 text-muted-foreground">
              当前页面和后台任务会保留。重新登录后将自动恢复连接并补齐任务输出。
            </p>
          </div>
        </div>
        <div className="mt-6">
          <LoginForm onAuthenticated={restoreAuthentication} submitLabel="重新登录" />
        </div>
      </div>
    </div>
  );
}
