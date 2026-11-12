import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let state: typeof import("@/app/store");
let Sidebar: typeof import("./Sidebar").Sidebar;
let originalGlobals: Record<string, unknown>;
let originalFetch: typeof fetch;

beforeEach(async () => {
  originalFetch = globalThis.fetch;
  originalGlobals = Object.fromEntries(["window", "document", "location", "history", "localStorage", "Element"].map((key) => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/chat" });
  Object.assign(globalThis, {
    window: dom.window, document: dom.window.document, location: dom.window.location,
    history: dom.window.history, localStorage: dom.window.localStorage, Element: dom.window.Element,
    IS_REACT_ACT_ENVIRONMENT: true,
  });
  Object.assign(dom.window, { matchMedia: () => ({ matches: false }) });
  state = await import("@/app/store");
  Sidebar = (await import("./Sidebar")).Sidebar;
  state.store.dispatch(state.appActions.setSidebarOpen(true));
  state.store.dispatch(state.chatActions.setActiveSession(""));
  const requestStartedAt = Date.now();
  state.store.dispatch(state.sessionsActions.beginSessionsRequest(requestStartedAt));
  state.store.dispatch(state.sessionsActions.setSessions({ items: [
    { session_id: "workspace-assistant", title: "助手" },
    { session_id: "ordinary", title: "普通会话" },
  ], requestStartedAt }));
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  globalThis.fetch = originalFetch;
  Object.assign(globalThis, originalGlobals);
  dom.window.close();
});

test("assistant stays beside new chat outside the scroll area and does not duplicate in history", async () => {
  await act(async () => root.render(<Provider store={state.store}><Sidebar /></Provider>));
  const assistant = container.querySelector<HTMLButtonElement>('button[aria-label="助手"]')!;
  const newChat = container.querySelector<HTMLButtonElement>('button[aria-label="新建会话"]')!;
  expect(assistant.closest(".overflow-y-auto")).toBeNull();
  expect(newChat.closest(".overflow-y-auto")).toBeNull();
  expect(container.querySelectorAll('button[aria-label="会话操作"]')).toHaveLength(1);
  expect(container.querySelector('button[aria-label="修改助手名称"]')).toBeNull();
  await act(async () => assistant.click());
  expect(location.pathname).toBe("/assistant");
  expect(state.store.getState().chat.activeSessionID).toBe("workspace-assistant");
  await act(async () => newChat.click());
  expect(location.pathname).toBe("/chat");
  expect(state.store.getState().chat.activeSessionID).toBe("");
});

test("refreshing persisted assistant identity updates the fixed entry without changing the open chat", async () => {
  state.store.dispatch(state.chatActions.setActiveSession("ordinary"));
  await act(async () => root.render(<Provider store={state.store}><Sidebar /></Provider>));
  globalThis.fetch = (async () => Response.json({ code: 0, data: { session_id: "workspace-assistant", title: "小空" } })) as unknown as typeof fetch;
  const { refreshAssistant } = await import("@/features/sessions/sessionThunks");
  await act(async () => { await state.store.dispatch(refreshAssistant()).unwrap(); });
  expect(container.querySelector('button[aria-label="小空"]')).not.toBeNull();
  expect(container.querySelector('button[aria-label="助手"]')).toBeNull();
  expect(state.store.getState().chat.activeSessionID).toBe("ordinary");
});

test("only ordinary chats retain the header while mobile navigation remains available on every panel", async () => {
  const { AppShell } = await import("./AppShell");
  state.store.dispatch(state.appActions.setActivePanel("chat"));
  await act(async () => root.render(<Provider store={state.store}><AppShell><div>content</div></AppShell></Provider>));
  expect(container.querySelector("main > header")).toBeNull();
  expect(container.querySelector('main button[aria-label="打开导航"]')).not.toBeNull();
  await act(async () => { state.store.dispatch(state.chatActions.setActiveSession("workspace-assistant")); });
  expect(container.querySelector("main > header")).toBeNull();
  expect(container.querySelector('button[aria-label="分享会话"]')).toBeNull();
  await act(async () => { state.store.dispatch(state.chatActions.setActiveSession("ordinary")); });
  expect(container.querySelector("main > header")?.textContent).toContain("普通会话");
  expect(container.querySelector('main button[aria-label="分享会话"]')).not.toBeNull();
  for (const panel of ["memory", "files", "schedules", "shares", "skills", "config"] as const) {
    await act(async () => { state.store.dispatch(state.appActions.setActivePanel(panel)); });
    expect(container.querySelector("main > header")).toBeNull();
    expect(container.querySelector('main button[aria-label="打开导航"]')).not.toBeNull();
    expect(container.querySelector('main button[aria-label="分享会话"]')).toBeNull();
  }
  await act(async () => { state.store.dispatch(state.appActions.setSidebarOpen(false)); });
  await act(async () => { container.querySelector<HTMLButtonElement>('main button[aria-label="打开导航"]')!.click(); });
  expect(state.store.getState().app.sidebarOpen).toBe(true);
});
