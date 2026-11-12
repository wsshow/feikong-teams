import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let state: typeof import("@/app/store");
let ChatInput: typeof import("./ChatInput").ChatInput;
let savedGlobals: Record<string, unknown>;
let savedFetch: typeof fetch;
let requests: { url: string; body: Record<string, unknown> }[];

beforeEach(async () => {
  savedFetch = globalThis.fetch;
  const keys = ["window", "document", "location", "history", "localStorage", "HTMLElement", "Element", "Node", "requestAnimationFrame", "ResizeObserver"];
  savedGlobals = Object.fromEntries(keys.map((key) => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/assistant" });
  Object.assign(dom.window.HTMLElement.prototype, { attachEvent() {}, detachEvent() {} });
  Object.assign(globalThis, {
    window: dom.window, document: dom.window.document, location: dom.window.location, history: dom.window.history,
    localStorage: dom.window.localStorage, HTMLElement: dom.window.HTMLElement, Element: dom.window.Element, Node: dom.window.Node,
    requestAnimationFrame: (callback: FrameRequestCallback) => setTimeout(callback, 0),
    ResizeObserver: class { observe() {} disconnect() {} }, IS_REACT_ACT_ENVIRONMENT: true,
  });
  Object.assign(dom.window.HTMLDialogElement.prototype, {
    showModal(this: HTMLDialogElement) { this.setAttribute("open", ""); },
    close(this: HTMLDialogElement) { this.removeAttribute("open"); },
  });
  state = await import("@/app/store");
  ChatInput = (await import("./ChatInput")).ChatInput;
  state.store.dispatch(state.chatActions.finishRunningSession("workspace-assistant"));
  state.store.dispatch(state.chatActions.setActiveSession("workspace-assistant"));
  state.store.dispatch(state.chatActions.setSessionDetail({ session_id: "workspace-assistant", view: userView("old context"), queue: [{ queue_id: "q", kind: "follow_up", text: "pending task" }] }));
  state.store.dispatch(state.sessionsActions.upsertSession({ session_id: "workspace-assistant", title: "小空" }));
  requests = [];
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    const body = JSON.parse(String(init?.body || "{}"));
    requests.push({ url: String(url), body });
    return Response.json({ code: 0, data: { session_id: "workspace-assistant", title: body.title } });
  }) as unknown as typeof fetch;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
    await new Promise((resolve) => setTimeout(resolve, 5));
  });
  globalThis.fetch = savedFetch;
  Object.assign(globalThis, savedGlobals);
  dom.window.close();
});

async function render(variant: "hero" | "dock" = "hero") {
  await act(async () => root.render(<Provider store={state.store}><ChatInput variant={variant} /></Provider>));
}

function editor() { return container.querySelector<HTMLDivElement>('[role="textbox"]')!; }
function options() { return Array.from(container.querySelectorAll<HTMLButtonElement>('[role="option"]')); }
function dialog() { return container.querySelector<HTMLDialogElement>("dialog[open]"); }
function button(label: string) { return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find((item) => item.textContent === label)!; }

async function input(text: string) {
  await act(async () => {
    editor().focus();
    editor().textContent = text;
    const range = document.createRange();
    range.selectNodeContents(editor());
    range.collapse(false);
    const selection = dom.window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    editor().dispatchEvent(new dom.window.InputEvent("input", { bubbles: true }));
  });
}

async function key(key: string, init: KeyboardEventInit = {}) {
  await act(async () => {
    editor().dispatchEvent(new dom.window.KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init }));
    editor().dispatchEvent(new dom.window.KeyboardEvent("keyup", { key, bubbles: true, ...init }));
  });
}

async function name(value: string) {
  await act(async () => {
    const field = dialog()!.querySelector<HTMLInputElement>("input")!;
    field.focus();
    Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(field, value);
    field.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
    field.dispatchEvent(new dom.window.KeyboardEvent("keyup", { key: "a", bubbles: true }));
  });
}

test("slash menu filters commands, preserves keyboard selection, and dismisses without sending", async () => {
  await render();
  await input("/");
  expect(options()).toHaveLength(2);
  expect(editor().getAttribute("aria-controls")).toBe(container.querySelector('[role="listbox"]')!.id);
  await key("ArrowDown");
  expect(options()[1].getAttribute("aria-selected")).toBe("true");
  expect(editor().getAttribute("aria-activedescendant")).toBe(options()[1].id);
  await key("ArrowUp");
  expect(options()[0].getAttribute("aria-selected")).toBe("true");
  await key("Escape");
  expect(options()).toHaveLength(0);
  expect(editor().textContent).toBe("/");
  await input("/重命名");
  expect(options()).toHaveLength(1);
  await key("Enter", { isComposing: true });
  expect(dialog()).toBeNull();
  await key("Enter", { shiftKey: true });
  expect(dialog()).toBeNull();
  await key("Tab");
  expect(dialog()?.textContent).toContain("重命名助手");
  expect(editor().textContent).toBe("");
  expect(requests).toHaveLength(0);
});

test("ordinary text and paths keep their behavior and commands are limited to assistant", async () => {
  await render("dock");
  for (const text of ["看看 /rename", "https://example.com", "#folder/file", "/folder/file"]) {
    await input(text);
    expect(container.querySelector('[aria-label="助手命令"]')).toBeNull();
  }
  await input("/missing");
  expect(container.textContent).toContain("没有匹配命令");
  await act(async () => document.body.dispatchEvent(new dom.window.Event("pointerdown", { bubbles: true })));
  expect(container.querySelector('[aria-label="助手命令"]')).toBeNull();
  await act(async () => state.store.dispatch(state.chatActions.setActiveSession("ordinary")));
  await input("/");
  expect(options()).toHaveLength(0);
  expect(editor().hasAttribute("aria-controls")).toBe(false);
});

test("rename command persists the trimmed name and refreshes assistant labels", async () => {
  await render();
  await input("/ren");
  await act(async () => options()[0].click());
  expect(dialog()!.querySelector<HTMLInputElement>("input")!.value).toBe("小空");
  await name(" 小飞 ");
  await act(async () => button("保存").click());
  expect(requests).toEqual([{ url: "/api/fkteams/sessions/rename", body: { session_id: "workspace-assistant", title: "小飞" } }]);
  expect(dialog()).toBeNull();
  expect(state.store.getState().sessions.items.find((item) => item.session_id === "workspace-assistant")?.title).toBe("小飞");
  expect(editor().getAttribute("aria-label")).toBe("给小飞发送消息");
});

test("send button executes the selected command while the menu is open", async () => {
  await render();
  await input("/");
  await key("ArrowDown");
  await act(async () => {
    const send = container.querySelector<HTMLButtonElement>('button[aria-label="发送"]')!;
    const mouseDown = new dom.window.MouseEvent("mousedown", { bubbles: true, cancelable: true });
    send.dispatchEvent(mouseDown);
    expect(mouseDown.defaultPrevented).toBe(true);
    send.click();
  });
  expect(dialog()?.textContent).toContain("重命名助手");
  expect(requests).toHaveLength(0);
});

test("invalid and failed renames keep the dialog and saved identity", async () => {
  await render();
  await input("/rename");
  await key("Enter");
  await name("名".repeat(41));
  await act(async () => button("保存").click());
  expect(requests).toHaveLength(0);
  expect(dialog()!.querySelector('[role="alert"]')?.textContent).toContain("1–40");
  globalThis.fetch = (async () => Response.json({ code: 500, message: "保存失败" }, { status: 500 })) as unknown as typeof fetch;
  await name("新名字");
  await act(async () => button("保存").click());
  expect(dialog()!.querySelector('[role="alert"]')?.textContent).toContain("保存失败");
  expect(state.store.getState().sessions.items.find((item) => item.session_id === "workspace-assistant")?.title).toBe("小空");
});

test("clear requires confirmation and only clears local history after server success", async () => {
  await render();
  await input("/clear");
  await key("Enter");
  await act(async () => button("取消").click());
  expect(requests).toHaveLength(0);
  expect(state.store.getState().chat.messages).toHaveLength(1);
  await input("/clear");
  await key("Enter");
  let resolve!: (response: Response) => void;
  globalThis.fetch = (() => { requests.push({ url: "/api/fkteams/assistant/clear", body: {} }); return new Promise<Response>((done) => { resolve = done; }); }) as unknown as typeof fetch;
  await act(async () => { button("确认清空").click(); button("确认清空").click(); });
  expect(requests).toHaveLength(1);
  expect(state.store.getState().chat.messages).toHaveLength(1);
  await act(async () => resolve(Response.json({ code: 0, data: { session_id: "workspace-assistant" } })));
  expect(dialog()).toBeNull();
  const chat = state.store.getState().chat;
  expect(chat.viewSessionID).toBe("workspace-assistant");
  expect(chat.messages).toHaveLength(0);
  expect(chat.queue).toHaveLength(0);
  expect(editor().getAttribute("aria-label")).toBe("给小空发送消息");
});

test("running assistant disables clear including direct command submission", async () => {
  await act(async () => state.store.dispatch(state.chatActions.beginRunningSession({ sessionID: "workspace-assistant", startedAt: Date.now() })));
  await render();
  await input("/clear");
  expect(options()[0].getAttribute("aria-disabled")).toBe("true");
  await key("Enter");
  expect(dialog()).toBeNull();
  await key("Escape");
  await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="发送"]')!.click());
  expect(dialog()).toBeNull();
  expect(requests).toHaveLength(0);
  await input("/rename");
  await key("Enter");
  expect(dialog()?.textContent).toContain("重命名助手");
});

test("failed clear keeps history, queue and confirmation open", async () => {
  await render();
  await input("/clear");
  await key("Enter");
  globalThis.fetch = (async () => Response.json({ code: 409, message: "session is active" }, { status: 409 })) as unknown as typeof fetch;
  await act(async () => button("确认清空").click());
  expect(dialog()!.querySelector('[role="alert"]')?.textContent).toContain("session is active");
  expect(state.store.getState().chat.messages).toHaveLength(1);
  expect(state.store.getState().chat.queue).toHaveLength(1);
});

test("clear response cannot replace a different chat opened during the request", async () => {
  await render();
  await input("/clear");
  await key("Enter");
  let resolve!: (response: Response) => void;
  globalThis.fetch = (() => new Promise<Response>((done) => { resolve = done; })) as unknown as typeof fetch;
  await act(async () => button("确认清空").click());
  await act(async () => {
    state.store.dispatch(state.chatActions.setActiveSession("ordinary"));
    state.store.dispatch(state.chatActions.setSessionDetail({ session_id: "ordinary", view: userView("other chat") }));
  });
  await act(async () => resolve(Response.json({ code: 0, data: { session_id: "workspace-assistant" } })));
  expect(state.store.getState().chat.activeSessionID).toBe("ordinary");
  expect(state.store.getState().chat.messages[0]?.content).toBe("other chat");
});

function userView(content: string): import("@/types/presentation").PresentationSnapshot {
 return { items: [{id:"user",index:0,order:0,kind:"message",node:{message:{id:"user",role:"user",content},parts:[],order:0,showAgentLabel:false,showCopyAction:false}}] };
}
