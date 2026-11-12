import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";
import { installDialogStub } from "@/test/dialog";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let originalGlobals: Record<string, unknown>;
let originalFetch: typeof fetch;
let writes: number;
let finishSave: (response: Response) => void;
const task = { id: "weekly", task: "整理进展", cron_expr: "0 9 * * 1", status: "active" };

beforeEach(async () => {
  originalFetch = globalThis.fetch;
  originalGlobals = Object.fromEntries(["window", "document", "location", "history", "localStorage"].map((key) => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/schedules" });
  installDialogStub(dom);
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, location: dom.window.location, history: dom.window.history, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
  writes = 0;
  globalThis.fetch = (async (_path: unknown, init?: RequestInit) => {
    if (init?.method === "PUT" || init?.method === "POST") {
      writes++;
      return new Promise<Response>((resolve) => { finishSave = resolve; });
    }
    return Response.json({ code: 0, data: { tasks: [task] } });
  }) as typeof fetch;
  const { store } = await import("@/app/store");
  const { SchedulePanel } = await import("./SchedulePanel");
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => root.render(<Provider store={store}><SchedulePanel /></Provider>));
});

afterEach(async () => {
  await act(async () => root.unmount());
  globalThis.fetch = originalFetch;
  Object.assign(globalThis, originalGlobals);
  dom.window.close();
});

function button(label: string, parent: ParentNode = container) {
  return [...parent.querySelectorAll<HTMLButtonElement>("button")].find((item) => item.textContent?.trim() === label)!;
}
async function click(label: string, parent: ParentNode = container) { await act(async () => button(label, parent).click()); }

test("editing stays in a modal through failed saves and closes after persistence succeeds", async () => {
  expect(container.querySelector("textarea")).toBeNull();
  await click("编辑");
  const dialog = container.querySelector<HTMLDialogElement>("dialog[open]")!;
  expect(dialog.querySelector("textarea")?.value).toBe(task.task);
  await click("确认并保存", dialog);
  expect(writes).toBe(1);
  expect(button("取消", dialog).disabled).toBe(true);
  expect(dialog.querySelector("fieldset")?.disabled).toBe(true);
  await act(async () => finishSave(Response.json({ message: "存储失败" }, { status: 500 })));
  expect(dialog.querySelector('[role="alert"]')?.textContent).toBe("存储失败");
  expect(dialog.querySelector("textarea")?.value).toBe(task.task);
  await click("确认并保存", dialog);
  expect(writes).toBe(2);
  await act(async () => finishSave(Response.json({ code: 0, data: { task } })));
  expect(container.querySelector("dialog[open]")).toBeNull();
});

test("cancelling a new schedule leaves the list unchanged", async () => {
  await click("新建任务");
  const dialog = container.querySelector<HTMLDialogElement>("dialog[open]")!;
  expect(dialog).not.toBeNull();
  await click("取消", dialog);
  expect(writes).toBe(0);
  expect(container.querySelector("textarea")).toBeNull();
  expect(container.textContent).toContain(task.task);
});
