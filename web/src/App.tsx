import { lazy, Suspense, useEffect } from "react";
import { Provider } from "react-redux";
import { get } from "@/api/client";
import { listAgents } from "@/api/agents";
import { appActions, chatActions } from "@/app/store";
import { store } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { AppShell } from "@/components/layout/AppShell";
import { LoadingSurface } from "@/components/ui/loading-surface";
import { loadSessions } from "@/features/sessions/sessionThunks";
import { authExpiredEvent, authRestoredEvent } from "@/lib/auth-session";
import { chatSessionIDFromPath, panelFromPath } from "@/lib/navigation";
import type { AgentInfo, VersionInfo } from "@/types/api";

const ChatPage = lazy(() => import("@/features/chat/ChatPage").then((module) => ({ default: module.ChatPage })));
const ConfigPanel = lazy(() => import("@/features/config/ConfigPanel").then((module) => ({ default: module.ConfigPanel })));
const FileManager = lazy(() => import("@/features/files/FileManager").then((module) => ({ default: module.FileManager })));
const LoginPage = lazy(() => import("@/features/auth/LoginPage").then((module) => ({ default: module.LoginPage })));
const PreviewPage = lazy(() => import("@/features/preview/PreviewPage").then((module) => ({ default: module.PreviewPage })));
const SchedulePanel = lazy(() => import("@/features/schedules/SchedulePanel").then((module) => ({ default: module.SchedulePanel })));
const ShareManagerPanel = lazy(() => import("@/features/share/ShareManagerPanel").then((module) => ({ default: module.ShareManagerPanel })));
const SharePage = lazy(() => import("@/features/share/SharePage").then((module) => ({ default: module.SharePage })));
const SkillPanel = lazy(() => import("@/features/skills/SkillPanel").then((module) => ({ default: module.SkillPanel })));

export function App() {
  return (
    <Provider store={store}>
      <Root />
    </Provider>
  );
}

function Root() {
  const path = location.pathname;
  if (path === "/login") {
    return (
      <Suspense fallback={<RouteLoading />}>
        <LoginPage />
      </Suspense>
    );
  }
  if (path.startsWith("/p/")) {
    return (
      <Suspense fallback={<RouteLoading />}>
        <PreviewPage />
      </Suspense>
    );
  }
  if (path.startsWith("/s/")) {
    return (
      <Suspense fallback={<RouteLoading />}>
        <SharePage />
      </Suspense>
    );
  }
  return (
    <AppShell>
      <Workspace />
    </AppShell>
  );
}

function Workspace() {
  const dispatch = useAppDispatch();
  const activePanel = useAppSelector((state) => state.app.activePanel);

  useEffect(() => {
    const syncRoute = () => {
      const panel = panelFromPath(location.pathname);
      dispatch(appActions.setActivePanel(panel));
      if (panel === "chat") {
        const sessionID = chatSessionIDFromPath(location.pathname);
        dispatch(chatActions.setActiveSession(sessionID));
        if (!sessionID) dispatch(chatActions.clearMessages());
      }
    };

    const refreshWorkspace = () => {
      void dispatch(loadSessions());
      void get<VersionInfo>("/api/fkteams/version").then((version) => dispatch(appActions.setVersion(version))).catch(() => undefined);
      void listAgents()
        .then((result) => {
          const agents = Array.isArray(result) ? result : result.agents || [];
          dispatch(appActions.setAgents(agents as AgentInfo[]));
        })
        .catch(() => undefined);
    };
    const onAuthExpired = () => dispatch(appActions.setAuthExpired(true));
    const onAuthRestored = () => {
      dispatch(appActions.setAuthExpired(false));
      dispatch(appActions.showToast("已重新登录，正在恢复连接"));
      refreshWorkspace();
    };
    window.addEventListener("popstate", syncRoute);
    window.addEventListener(authExpiredEvent, onAuthExpired);
    window.addEventListener(authRestoredEvent, onAuthRestored);
    syncRoute();
    refreshWorkspace();
    return () => {
      window.removeEventListener("popstate", syncRoute);
      window.removeEventListener(authExpiredEvent, onAuthExpired);
      window.removeEventListener(authRestoredEvent, onAuthRestored);
    };
  }, [dispatch]);

  const panel = (() => {
    switch (activePanel) {
      case "config":
        return <ConfigPanel />;
      case "files":
        return <FileManager />;
      case "schedules":
        return <SchedulePanel />;
      case "shares":
        return <ShareManagerPanel />;
      case "skills":
        return <SkillPanel />;
      case "chat":
      default:
        return <ChatPage />;
    }
  })();

  return <Suspense fallback={<PanelLoading />}>{panel}</Suspense>;
}

function RouteLoading() {
  return (
    <div className="flex h-[var(--app-viewport-height,100dvh)] items-center justify-center px-4">
      <LoadingSurface label="正在加载" />
    </div>
  );
}

function PanelLoading() {
  return (
    <div className="flex h-full items-center justify-center px-4">
      <LoadingSurface label="正在打开" />
    </div>
  );
}
