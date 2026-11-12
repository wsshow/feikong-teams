import { useEffect } from "react";
import { useAppDispatch } from "@/app/hooks";
import { appActions } from "@/app/store";
import { deliverScheduleNotifications } from "./notifications";

export function ScheduleNotifications() {
  const dispatch = useAppDispatch();
  useEffect(() => {
    const controller = new AbortController();
    const shown = new Set<string>();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      try {
        if (!document.hidden) await deliverScheduleNotifications(shown, (message) => dispatch(appActions.showToast(message)), controller.signal);
      } catch {
        // 断线或服务未启动时等待下一次轮询，待送达状态保留在服务端。
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 30000);
      }
    }
    void poll();
    return () => { controller.abort(); if (timer) clearTimeout(timer); };
  }, [dispatch]);
  return null;
}
