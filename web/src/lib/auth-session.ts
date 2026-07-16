import { setAuthToken } from "@/lib/storage";

export const authExpiredEvent = "fkteams:auth-expired";
export const authRestoredEvent = "fkteams:auth-restored";

export function expireAuthentication() {
  setAuthToken("");
  window.dispatchEvent(new CustomEvent(authExpiredEvent));
}

export function restoreAuthentication(token: string) {
  setAuthToken(token);
  window.dispatchEvent(new CustomEvent(authRestoredEvent));
}
