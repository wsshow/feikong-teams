import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let state: typeof import("@/app/store");
let ChatPage: typeof import("./ChatPage").ChatPage;
let savedGlobals: Record<string, unknown>;
let savedFetch: typeof fetch;

beforeEach(async () => {
  savedFetch = globalThis.fetch;
  const keys = ["window", "document", "location", "history", "localStorage", "HTMLElement", "Element", "Node", "requestAnimationFrame", "ResizeObserver"];
  savedGlobals = Object.fromEntries(keys.map(key => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/chat/legacy", pretendToBeVisual: true });
  Object.assign(globalThis, {
    window: dom.window, document: dom.window.document, location: dom.window.location, history: dom.window.history,
    localStorage: dom.window.localStorage, HTMLElement: dom.window.HTMLElement, Element: dom.window.Element, Node: dom.window.Node,
    requestAnimationFrame: (callback: FrameRequestCallback) => setTimeout(callback, 0),
    ResizeObserver: class { observe() {} disconnect() {} }, IS_REACT_ACT_ENVIRONMENT: true,
  });
  state = await import("@/app/store");
  ChatPage = (await import("./ChatPage")).ChatPage;
  state.store.dispatch(state.chatActions.clearMessages());
  state.store.dispatch(state.chatActions.setActiveSession("legacy"));
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
    await new Promise(resolve => setTimeout(resolve, 5));
  });
  globalThis.fetch = savedFetch;
  Object.assign(globalThis, savedGlobals);
  dom.window.close();
});

test("session errors survive Redux serialization and show a helpful message", async () => {
  const message = "这个会话的记录格式较旧，当前版本暂时无法打开。原有记录仍保留，请新建会话继续使用。";
  globalThis.fetch = (async () => Response.json({ code: 1, message }, { status: 409 })) as unknown as typeof fetch;
  await act(async () => {
    root.render(<Provider store={state.store}><ChatPage /></Provider>);
    await new Promise(resolve => setTimeout(resolve, 30));
  });
  expect(container.textContent).toContain(message);
  expect(container.textContent).not.toContain("[object Object]");
  expect(state.store.getState().chat.error).toBe(message);
});
