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
    await new Promise(resolve => setTimeout(resolve, 5));
    root.unmount();
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

function conversation(content: string): import("@/types/presentation").PresentationSnapshot {
  return { items: [{ id: "previous-answer", index: 0, order: 1, kind: "message", node: {
    message: { id: "previous-answer", role: "user", content, createdAt: "2026-11-12T08:33:14Z" },
    parts: [{ type: "text", key: "previous-answer:text", content }], showAgentLabel: false, showCopyAction: true, order: 1,
  } }] };
}

test("switching from a readable session to a failed one never shows the previous conversation", async () => {
  const previousAnswer = "仅属于刚才新会话的正文";
  const message = "这个会话的记录格式较旧，当前版本暂时无法打开。原有记录仍保留，请新建会话继续使用。";
  state.store.dispatch(state.chatActions.setActiveSession("healthy"));
  state.store.dispatch(state.chatActions.setSessionDetail({ session_id: "healthy", view: conversation(previousAnswer) }));
  let resolve!: (response: Response) => void;
  globalThis.fetch = (async () => new Promise<Response>(done => { resolve = done; })) as unknown as typeof fetch;
  await act(async () => {
    root.render(<Provider store={state.store}><ChatPage /></Provider>);
    await new Promise(resolve => setTimeout(resolve, 50));
  });
  expect(container.textContent).toContain(previousAnswer);
  await act(async () => state.store.dispatch(state.chatActions.setActiveSession("legacy")));
  expect(container.textContent).not.toContain(previousAnswer);
  await act(async () => resolve(Response.json({ code: 1, message }, { status: 409 })));
  expect(container.textContent).toContain(message);
  expect(container.textContent).not.toContain(previousAnswer);
  expect(container.querySelector('[role="textbox"]')).toBeNull();
});

for (const outcome of ["success", "failure"] as const) {
  test(`a late ${outcome} from a previous selection cannot replace the current session`, async () => {
    state.store.dispatch(state.chatActions.setActiveSession("slow"));
    let resolveSlow!: (response: Response) => void;
    globalThis.fetch = (async (input: string | URL | Request) => {
      if (String(input).endsWith("/slow")) return new Promise<Response>(done => { resolveSlow = done; });
      return Response.json({ code: 0, data: { session_id: "fast", view: conversation("当前会话的正文") } });
    }) as unknown as typeof fetch;
    await act(async () => root.render(<Provider store={state.store}><ChatPage /></Provider>));
    await act(async () => state.store.dispatch(state.chatActions.setActiveSession("fast")));
    expect(container.textContent).toContain("当前会话的正文");
    await act(async () => resolveSlow(outcome === "success"
      ? Response.json({ code: 0, data: { session_id: "slow", view: conversation("过期请求的正文") } })
      : Response.json({ code: 1, message: "过期请求的错误" }, { status: 409 })));
    expect(state.store.getState().chat.activeSessionID).toBe("fast");
    expect(container.textContent).toContain("当前会话的正文");
    expect(container.textContent).not.toContain("过期请求");
  });
}

test("opening a readable session after an error clears the error and restores its own view", async () => {
  globalThis.fetch = (async (input: string | URL | Request) => String(input).endsWith("/legacy")
    ? Response.json({ code: 1, message: "旧会话无法打开" }, { status: 409 })
    : Response.json({ code: 0, data: { session_id: "healthy", view: conversation("新会话正文") } })) as unknown as typeof fetch;
  await act(async () => root.render(<Provider store={state.store}><ChatPage /></Provider>));
  expect(container.textContent).toContain("旧会话无法打开");
  await act(async () => state.store.dispatch(state.chatActions.setActiveSession("healthy")));
  expect(container.textContent).toContain("新会话正文");
  expect(container.textContent).not.toContain("旧会话无法打开");
  expect(container.querySelector('[role="textbox"]')).not.toBeNull();
});

test("an unavailable session can leave for a clean new conversation", async () => {
  globalThis.fetch = (async () => Response.json({ code: 1, message: "旧会话无法打开" }, { status: 409 })) as unknown as typeof fetch;
  await act(async () => root.render(<Provider store={state.store}><ChatPage /></Provider>));
  const create = Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(button => button.textContent === "新建会话")!;
  await act(async () => create.click());
  expect(state.store.getState().chat.activeSessionID).toBe("");
  expect(state.store.getState().chat.error).toBeUndefined();
  expect(location.pathname).toBe("/chat");
  expect(container.textContent).not.toContain("旧会话无法打开");
  expect(container.querySelector('[role="textbox"]')).not.toBeNull();
});
