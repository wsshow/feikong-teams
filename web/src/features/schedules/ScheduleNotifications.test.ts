import { afterEach, expect, test } from "bun:test";
import { deliverScheduleNotifications } from "./notifications";

const originalFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = originalFetch; });

test("failed delivery acknowledgment retries without replay or duplicate toast; new run notifies", async () => {
  let runAt = "2026-11-12T10:00:00Z";
  let fail = true;
  const calls: string[] = [];
  const messages: string[] = [];
  globalThis.fetch = (async (path: string, init?: RequestInit) => {
    calls.push(`${init?.method || "GET"} ${path}`);
    if (init?.method === "POST") {
      expect(JSON.parse(init.body as string).run_at).toBe(runAt);
      if (fail) throw new Error("offline");
      return Response.json({ code: 0, data: { acknowledged: true } });
    }
    return Response.json({ code: 0, data: { tasks: [{ id: "report", task: "日报", status: "pending", last_run_status: "completed", notification_pending: true, last_run_at: runAt }] } });
  }) as unknown as typeof fetch;
  const shown = new Set<string>();
  const signal = new AbortController().signal;
  await deliverScheduleNotifications(shown, (m) => messages.push(m), signal);
  fail = false;
  await deliverScheduleNotifications(shown, (m) => messages.push(m), signal);
  expect(messages).toHaveLength(1);
  expect(messages[0]).toContain("已完成");
  runAt = "2026-11-13T10:00:00Z";
  await deliverScheduleNotifications(shown, (m) => messages.push(m), signal);
  expect(messages).toHaveLength(2);
  expect(calls.filter((c) => c.startsWith("POST"))).toEqual(Array(3).fill("POST /api/fkteams/schedules/report/acknowledge"));
});

test("aborted notification poll neither displays nor acknowledges", async () => {
  const controller = new AbortController();
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    controller.abort();
    return Response.json({ code: 0, data: { tasks: [{ id: "report", notification_pending: true, last_run_at: "2026-11-12T10:00:00Z" }] } });
  }) as unknown as typeof fetch;
  await deliverScheduleNotifications(new Set(), () => { throw new Error("displayed after unmount"); }, controller.signal);
  expect(calls).toBe(1);
});
