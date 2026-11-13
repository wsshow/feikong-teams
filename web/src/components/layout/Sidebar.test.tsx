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

async function renderResizableSidebar() {
  await act(async () => root.render(<Provider store={state.store}><Sidebar /></Provider>));
  const handle = container.querySelector<HTMLDivElement>('[role="separator"]')!;
  const captured = new Set<number>();
  Object.assign(handle, {
    setPointerCapture: (id: number) => captured.add(id),
    hasPointerCapture: (id: number) => captured.has(id),
    releasePointerCapture: (id: number) => captured.delete(id),
  });
  return handle;
}

async function pointer(handle: HTMLDivElement, type: string, clientX = 292, pointerId = 1) {
  await act(async () => {
    handle.dispatchEvent(new dom.window.PointerEvent(type, {
      bubbles: true, cancelable: true, isPrimary: true, button: 0, pointerId, clientX,
    }));
  });
}

async function press(handle: HTMLDivElement, key: string, shiftKey = false) {
  await act(async () => {
    handle.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key, shiftKey, bubbles: true, cancelable: true }));
  });
}

test("dragging resizes in both directions within bounds and remembers the width across collapse and remount", async () => {
  const handle = await renderResizableSidebar();
  const sidebar = container.querySelector("aside")!;
  document.body.style.cursor = "default";
  document.body.style.userSelect = "text";
  await pointer(handle, "pointerdown");
  expect(handle.hasPointerCapture(1)).toBe(true);
  expect(document.body.style.userSelect).toBe("none");
  expect(document.body.style.cursor).toBe("col-resize");
  await pointer(handle, "pointermove", 350, 2);
  expect(handle.getAttribute("aria-valuenow")).toBe("292");
  await pointer(handle, "pointermove", 350);
  expect(sidebar.style.getPropertyValue("--sidebar-width")).toBe("350px");
  await pointer(handle, "pointermove", 0);
  expect(handle.getAttribute("aria-valuenow")).toBe("240");
  await pointer(handle, "pointermove", 2000);
  expect(handle.getAttribute("aria-valuenow")).toBe("480");
  await pointer(handle, "pointermove", 350);
  await pointer(handle, "pointerup", 350);
  expect(handle.hasPointerCapture(1)).toBe(false);
  expect(document.body.style.userSelect).toBe("text");
  expect(document.body.style.cursor).toBe("default");
  expect(localStorage.getItem("fk_sidebar_width")).toBe("350");
  await pointer(handle, "pointermove", 420);
  expect(handle.getAttribute("aria-valuenow")).toBe("350");
  await act(async () => { state.store.dispatch(state.appActions.setSidebarOpen(false)); });
  expect(container.querySelector('[role="separator"]')).toBeNull();
  await act(async () => { state.store.dispatch(state.appActions.setSidebarOpen(true)); });
  expect(container.querySelector('[role="separator"]')?.getAttribute("aria-valuenow")).toBe("350");
  await act(async () => root.render(null));
  const restored = await renderResizableSidebar();
  expect(restored.getAttribute("aria-valuenow")).toBe("350");
});

test.each(["pointercancel", "lostpointercapture", "blur", "collapse", "unmount"])("%s ends dragging and restores document styles", async (reason) => {
  const handle = await renderResizableSidebar();
  await pointer(handle, "pointerdown");
  await pointer(handle, "pointermove", 330);
  if (reason === "blur") {
    await act(async () => { window.dispatchEvent(new dom.window.Event("blur")); });
  } else if (reason === "collapse") {
    await act(async () => { state.store.dispatch(state.appActions.setSidebarOpen(false)); });
  } else if (reason === "unmount") {
    await act(async () => root.render(null));
  } else {
    await pointer(handle, reason);
  }
  expect(document.body.style.userSelect).toBe("");
  expect(document.body.style.cursor).toBe("");
  await pointer(handle, "pointermove", 400);
  expect(localStorage.getItem("fk_sidebar_width")).toBe("330");
});

test("keyboard controls respect width bounds and double click restores the default", async () => {
  const handle = await renderResizableSidebar();
  await press(handle, "ArrowRight");
  expect(handle.getAttribute("aria-valuenow")).toBe("302");
  await press(handle, "ArrowLeft", true);
  expect(handle.getAttribute("aria-valuenow")).toBe("262");
  await press(handle, "Home");
  await press(handle, "ArrowLeft");
  expect(handle.getAttribute("aria-valuenow")).toBe("240");
  await press(handle, "End");
  await press(handle, "ArrowRight");
  expect(handle.getAttribute("aria-valuenow")).toBe("480");
  await act(async () => { handle.dispatchEvent(new dom.window.MouseEvent("dblclick", { bubbles: true })); });
  expect(handle.getAttribute("aria-valuenow")).toBe("292");
  expect(localStorage.getItem("fk_sidebar_width")).toBe("292");
});

test("viewport limits leave room for content without overwriting the saved desktop width", async () => {
  localStorage.setItem("fk_sidebar_width", "420");
  const handle = await renderResizableSidebar();
  async function resize(innerWidth: number) {
    await act(async () => {
      Object.assign(window, { innerWidth });
      window.dispatchEvent(new dom.window.Event("resize"));
    });
  }
  await resize(768);
  expect(handle.getAttribute("aria-valuenow")).toBe("384");
  expect(handle.getAttribute("aria-valuemax")).toBe("384");
  expect(localStorage.getItem("fk_sidebar_width")).toBe("420");
  await resize(375);
  await pointer(handle, "pointerdown");
  await pointer(handle, "pointermove", 100);
  await press(handle, "Home");
  expect(document.body.style.userSelect).toBe("");
  expect(localStorage.getItem("fk_sidebar_width")).toBe("420");
  await resize(1200);
  expect(handle.getAttribute("aria-valuenow")).toBe("420");
});

test.each(["invalid", "Infinity", "1000", "-1"])("invalid stored width %s falls back to the default", async (saved) => {
  localStorage.setItem("fk_sidebar_width", saved);
  const handle = await renderResizableSidebar();
  expect(handle.getAttribute("aria-valuenow")).toBe("292");
});

test("resizing remains usable when browser storage is unavailable", async () => {
  Object.assign(globalThis, { localStorage: {
    getItem() { throw new Error("storage unavailable"); },
    setItem() { throw new Error("storage unavailable"); },
  } });
  const handle = await renderResizableSidebar();
  await press(handle, "ArrowRight");
  expect(handle.getAttribute("aria-valuenow")).toBe("302");
});
