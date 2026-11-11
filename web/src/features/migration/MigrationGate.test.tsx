import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act, StrictMode, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MigrationGate } from "./MigrationGate";

let root: Root;
let container: HTMLDivElement;
let originalFetch: typeof fetch;
let dom: JSDOM;
beforeEach(() => {
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, location: dom.window.location, IS_REACT_ACT_ENVIRONMENT: true });
  container = document.createElement("div"); document.body.appendChild(container); root = createRoot(container);
  originalFetch = globalThis.fetch;
});
afterEach(async () => { await act(async () => root.unmount()); globalThis.fetch = originalFetch; dom.window.close(); });
const response = (ready: boolean, state: string, error?: string) => Response.json({ code: 0, data: { ready, migration: { state, error } } });
async function click(text: string) {
  const button = [...container.querySelectorAll("button")].find((button) => button.textContent === text);
  expect(button).toBeDefined(); await act(async () => button!.click());
}

test("blocks workspace effects until migration and service handoff finish, without auto-starting in StrictMode", async () => {
  let posts = 0; let workspaceStarts = 0; let ready = false; let state = "pending";
  globalThis.fetch = (async (_path: unknown, init?: RequestInit) => {
    if (init?.method === "POST") { posts++; state = "running"; }
    return response(ready, state);
  }) as unknown as typeof fetch;
  function Workspace() { useEffect(() => { workspaceStarts++; }, []); return <p>workspace</p>; }
  await act(async () => root.render(<StrictMode><MigrationGate><Workspace /></MigrationGate></StrictMode>));
  expect(posts).toBe(0); expect(workspaceStarts).toBe(0);
  await click("备份并开始升级"); expect(posts).toBe(1); expect(workspaceStarts).toBe(0);
  state = "completed";
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 1100)); });
  expect(workspaceStarts).toBe(0);
  ready = true;
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 1100)); });
  expect(container.textContent).toContain("数据升级完成"); expect(workspaceStarts).toBe(0);
  await click("开始使用"); expect(workspaceStarts).toBeGreaterThan(0);
});

test("failed migration stays blocked and offers a deliberate retry", async () => {
  let posts = 0;
  globalThis.fetch = (async (_path: unknown, init?: RequestInit) => {
    if (init?.method === "POST") { posts++; return response(false, "running"); }
    return response(false, "failed", "backup verification failed");
  }) as unknown as typeof fetch;
  await act(async () => root.render(<MigrationGate><p>workspace</p></MigrationGate>));
  expect(container.textContent).toContain("backup verification failed");
  expect(container.textContent).not.toContain("workspace"); expect(posts).toBe(0);
  await click("重试升级"); expect(posts).toBe(1);
});

test("already migrated installations enter directly", async () => {
  globalThis.fetch = (async () => response(true, "completed")) as unknown as typeof fetch;
  await act(async () => root.render(<MigrationGate><p>workspace</p></MigrationGate>));
  expect(container.textContent).toBe("workspace"); expect(container.querySelector('[role="dialog"]')).toBeNull();
});
