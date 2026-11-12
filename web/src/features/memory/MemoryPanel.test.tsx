import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryPanel } from "./MemoryPanel";
import { installDialogStub } from "@/test/dialog";

const entries = [
  { id: "preference", type: "preference", summary: "偏好简洁回答", detail: "", tags: [] },
  { id: "fact", type: "fact", summary: "使用 Go 开发", detail: "", tags: [] },
];
let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let originalFetch: typeof fetch;
let originalWindow: typeof window;
let originalDocument: typeof document;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  originalWindow = globalThis.window;
  originalDocument = globalThis.document;
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
  installDialogStub(dom);
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, IS_REACT_ACT_ENVIRONMENT: true });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  globalThis.fetch = (async () => Response.json({ code: 0, data: entries })) as unknown as typeof fetch;
});

afterEach(async () => {
  await act(async () => root.unmount());
  globalThis.fetch = originalFetch;
  globalThis.window = originalWindow;
  globalThis.document = originalDocument;
  dom.window.close();
});

function filter() {
  return container.querySelector<HTMLButtonElement>('[role="combobox"][aria-label="筛选记忆类型"]')!;
}

async function click(element: HTMLElement) {
  await act(async () => element.click());
}

async function key(value: string) {
  const event = new dom.window.KeyboardEvent("keydown", { key: value, bubbles: true, cancelable: true });
  await act(async () => filter().dispatchEvent(event));
  return event;
}

function option(text: string) {
  return [...container.querySelectorAll<HTMLButtonElement>('[role="option"]')].find((item) => item.textContent === text)!;
}

test("custom type filter selects categories and restores all entries with the empty option", async () => {
  await act(async () => root.render(<MemoryPanel />));
  expect(container.querySelector("select")).toBeNull();
  expect(container.querySelectorAll("li")).toHaveLength(2);
  await click(filter());
  expect(option("全部类型").getAttribute("aria-selected")).toBe("true");
  await click(option("个人信息"));
  expect(filter().textContent).toBe("个人信息");
  expect(container.querySelectorAll("li")).toHaveLength(1);
  expect(container.querySelector("li")?.textContent).toContain("使用 Go 开发");
  expect(container.querySelector('[role="listbox"]')).toBeNull();
  expect(document.activeElement).toBe(filter());
  await click(filter());
  await click(option("全部类型"));
  expect(container.querySelectorAll("li")).toHaveLength(2);
});

test("keyboard navigation commits with Enter and dismisses without changes on Escape, Tab, blur or outside click", async () => {
  await act(async () => root.render(<MemoryPanel />));
  filter().focus();
  await key("ArrowDown");
  await key("End");
  expect(document.getElementById(filter().getAttribute("aria-activedescendant")!)?.textContent).toBe("操作经验");
  await key("ArrowUp");
  await key("Enter");
  expect(filter().textContent).toBe("认知洞察");
  expect(container.textContent).toContain("没有匹配的记忆");
  await key("个");
  await key("Enter");
  expect(filter().textContent).toBe("个人信息");
  await key("用");
  await key("Enter");
  expect(filter().textContent).toBe("用户偏好");
  await key(" ");
  await key("Home");
  await key("ArrowDown");
  await key("Enter");
  expect(filter().textContent).toBe("用户偏好");
  expect(container.querySelectorAll("li")).toHaveLength(1);
  await key("ArrowDown");
  await key("ArrowDown");
  await key("Escape");
  expect(filter().textContent).toBe("用户偏好");
  expect(filter().getAttribute("aria-expanded")).toBe("false");
  expect(document.activeElement).toBe(filter());
  await click(filter());
  expect((await key("Tab")).defaultPrevented).toBe(false);
  expect(filter().getAttribute("aria-expanded")).toBe("false");
  await click(filter());
  await act(async () => container.querySelector<HTMLButtonElement>("button")!.focus());
  expect(filter().getAttribute("aria-expanded")).toBe("false");
  await click(filter());
  await act(async () => document.body.dispatchEvent(new dom.window.MouseEvent("pointerdown", { bubbles: true })));
  expect(filter().getAttribute("aria-expanded")).toBe("false");
});

test("editing saves the custom type selection and disables it while the save is pending", async () => {
  let saved: Record<string, unknown> | undefined;
  let finishSave!: (response: Response) => void;
  globalThis.fetch = (async (_path: unknown, init?: RequestInit) => {
    if (init?.method !== "PUT") return Response.json({ code: 0, data: entries });
    saved = JSON.parse(init.body as string);
    return new Promise<Response>((resolve) => { finishSave = resolve; });
  }) as unknown as typeof fetch;
  await act(async () => root.render(<MemoryPanel />));
  await click(container.querySelector<HTMLButtonElement>("li button")!);
  const typeSelect = container.querySelector<HTMLButtonElement>('[role="combobox"][aria-label="类型"]')!;
  expect(typeSelect.textContent).toBe("用户偏好");
  await click(typeSelect);
  await click(option("已确定方案"));
  expect(saved).toBeUndefined();
  const form = container.querySelector("form")!;
  expect(form.closest("dialog[open]")).not.toBeNull();
  await act(async () => form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true })));
  expect(saved?.type).toBe("decision");
  expect(typeSelect.disabled).toBe(true);
  await click(typeSelect);
  expect(container.querySelector('[role="listbox"]')).toBeNull();
  await act(async () => finishSave(Response.json({ code: 0, data: saved })));
  expect(container.querySelector("form")).toBeNull();
  expect(container.textContent).toContain("记忆已保存");
  expect(container.querySelector("li:last-child")?.textContent).toContain("已确定方案");
});

test("failed memory saves keep the modal and draft until cancelled", async () => {
  globalThis.fetch = (async (_path: unknown, init?: RequestInit) => init?.method === "PUT"
    ? Response.json({ message: "存储失败" }, { status: 500 })
    : Response.json({ code: 0, data: entries })) as typeof fetch;
  await act(async () => root.render(<MemoryPanel />));
  await click(container.querySelector<HTMLButtonElement>("li button")!);
  const dialog = container.querySelector("dialog[open]")!;
  await act(async () => dialog.querySelector("form")!.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true })));
  expect(dialog.querySelector('[role="alert"]')?.textContent).toContain("存储失败");
  expect(dialog.querySelector("input")?.value).toBe(entries[0].summary);
  const cancel = [...dialog.querySelectorAll("button")].find((item) => item.textContent === "取消")!;
  await click(cancel);
  expect(container.querySelector("dialog[open]")).toBeNull();
  expect(container.querySelectorAll("li")).toHaveLength(2);
});
