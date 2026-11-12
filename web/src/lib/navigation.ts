import type { AppPanel } from "@/app/store";
import { assistantSessionID } from "./assistant";

export function panelFromPath(path: string): AppPanel {
  if (path === "/memory") return "memory";
  if (path === "/config") return "config";
  if (path === "/files") return "files";
  if (path === "/schedules") return "schedules";
  if (path === "/shares") return "shares";
  if (path === "/skills") return "skills";
  return "chat";
}

export function chatSessionIDFromPath(path: string) {
  if (path === "/assistant") return assistantSessionID;
  const match = path.match(/^\/chat\/([^/?#]+)/);
  if (!match?.[1]) return "";
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return "";
  }
}

export function chatPath(sessionID?: string) {
  if (sessionID === assistantSessionID) return "/assistant";
  return sessionID ? `/chat/${encodeURIComponent(sessionID)}` : "/chat";
}

export function panelPath(panel: AppPanel) {
  switch (panel) {
    case "memory":
      return "/memory";
    case "config":
      return "/config";
    case "files":
      return "/files";
    case "schedules":
      return "/schedules";
    case "shares":
      return "/shares";
    case "skills":
      return "/skills";
    case "chat":
    default:
      return "/chat";
  }
}

export function loginReturnPath(search: string) {
  const next = new URLSearchParams(search).get("next") || "";
  if (!next.startsWith("/") || next.startsWith("//") || next.startsWith("/login")) return "/chat";
  return next;
}

export function pushAppPath(path: string) {
  if (location.pathname !== path) history.pushState(null, "", path);
}
