import { Dialog } from "@/components/ui/dialog";
import { LoginForm } from "@/features/auth/LoginForm";
import { restoreAuthentication } from "@/lib/auth-session";

export function AuthExpiredDialog({ open }: { open: boolean }) {
  return (
    <Dialog open={open} title="登录已过期" closeDisabled hideClose className="max-w-md" onOpenChange={() => undefined}>
      <p className="text-sm leading-6 text-muted-foreground">当前页面和后台任务会保留。重新登录后将自动恢复连接并补齐任务输出。</p>
      <div className="mt-6">
        <LoginForm onAuthenticated={restoreAuthentication} submitLabel="重新登录" />
      </div>
    </Dialog>
  );
}
