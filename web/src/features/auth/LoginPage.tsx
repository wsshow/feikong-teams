import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { LoginForm } from "@/features/auth/LoginForm";
import { restoreAuthentication } from "@/lib/auth-session";
import { loginReturnPath } from "@/lib/navigation";

export function LoginPage() {
  function authenticated(token: string) {
    restoreAuthentication(token);
    location.replace(loginReturnPath(location.search));
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/40 px-4 py-8 sm:px-6">
      <Panel className="w-full max-w-md">
        <PanelHeader className="px-6 py-5 sm:px-7 sm:py-6">
          <div className="flex items-center gap-4">
            <img className="h-12 w-12 shrink-0 drop-shadow-sm" src="/assets/fkteams-logo.svg" alt="" />
            <div>
              <div className="text-2xl font-semibold">非空小队</div>
              <div className="mt-1 text-base text-muted-foreground">登录后继续使用</div>
            </div>
          </div>
        </PanelHeader>
        <PanelBody className="px-6 py-6 sm:px-7">
          <LoginForm onAuthenticated={authenticated} />
        </PanelBody>
      </Panel>
    </div>
  );
}
